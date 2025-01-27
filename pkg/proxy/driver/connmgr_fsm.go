package driver

import (
	"context"
	"errors"
	"fmt"
	"github.com/pingcap/tidb/util/logutil"
	"github.com/siddontang/go-mysql/mysql"
)

type FSMState int
type FSMEvent int

const (
	State0 FSMState = 0x00 //             |            |
	State1 FSMState = 0x01 // Transaction |            |
	State2 FSMState = 0x02 //   		  | AutoCommit |
	State3 FSMState = 0x03 // Transaction | AutoCommit |
	State4 FSMState = 0x04 //             |            | Prepare
	State5 FSMState = 0x05 // Transaction |            | Prepare
	State6 FSMState = 0x06 //             | AutoCommit | Prepare
	State7 FSMState = 0x07 // Transaction | AutoCommit | Prepare

	StateUnknown FSMState = -1
)

const (
	FSMStateFlagInTransaction = 0x01
	FSMStateFlagIsAutoCommit  = 0x02
	FSMStateFlagInPrepare     = 0x04
)

const (
	stateInitial = State2
)

const (
	EventUnknown FSMEvent = -1
	EventQuery   FSMEvent = iota
	EventBegin
	EventCommitOrRollback
	EventDisableAutoCommit
	EventEnableAutoCommit

	// currently we does not support stmt_reset
	EventStmtPrepare
	EventStmtForwardData // execute, send_long_data
	EventStmtClose
)

var ErrFsmActionNowAllowed = errors.New("fsm action not allowed")

var globalFSM = NewFSM()

func init() {
	globalFSM.Init()
}

func getGlobalFSM() *FSM {
	return globalFSM
}

type FSM struct {
	handlersV2 map[FSMState]map[FSMEvent]*FSMHandlerWrapper
}

type FSMHandlerWrapper struct {
	NewState        FSMState
	MustChangeState bool
	Handler         FSMHandler
}

type FSMHandler interface {
	Handle(conn *BackendConnManager, ctx context.Context, args ...interface{}) (interface{}, error)
}

type FSMHandlerFunc func(conn *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error)

type FSMStmtPrepareHandlerFunc func(conn *BackendConnManager, ctx context.Context, args ...interface{}) (Stmt, error)

func (f FSMHandlerFunc) Handle(conn *BackendConnManager, ctx context.Context, args ...interface{}) (interface{}, error) {
	return f(conn, ctx, args...)
}

func (f FSMStmtPrepareHandlerFunc) Handle(conn *BackendConnManager, ctx context.Context, args ...interface{}) (interface{}, error) {
	return f(conn, ctx, args...)
}

func noopHandler(conn *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	return nil, nil
}

func errHandler(conn *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	return nil, ErrFsmActionNowAllowed
}

func NewFSM() *FSM {
	return &FSM{
		handlersV2: make(map[FSMState]map[FSMEvent]*FSMHandlerWrapper),
	}
}

func (q *FSM) Init() {
	// in state0, txnConn must be non nil, so we don't check txnConn nil in handlers
	q.MustRegisterHandler(State0, State0, EventDisableAutoCommit, false, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State0, State0, EventCommitOrRollback, false, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State0, State1, EventBegin, false, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventBegin))
	q.MustRegisterHandler(State0, State1, EventQuery, false, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventQuery))
	q.MustRegisterHandler(State0, State2, EventEnableAutoCommit, true, FSMHandlerFunc(fsmHandler_PostReleaseConn_EventEnableAutoCommit))
	q.MustRegisterHandler(State0, State4, EventStmtPrepare, false, FSMStmtPrepareHandlerFunc(fsmHandler_NoPrepare_WithAttachedConn_EventStmtPrepare))
	q.MustRegisterHandler(State0, State0, EventStmtForwardData, true, FSMHandlerFunc(errHandler)) // TODO(eastfisher): test
	q.MustRegisterHandler(State0, State0, EventStmtClose, true, FSMHandlerFunc(noopHandler))      // TODO(eastfisher): test

	q.MustRegisterHandler(State1, State1, EventDisableAutoCommit, true, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State1, State1, EventBegin, false, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State1, State1, EventQuery, false, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventQuery))
	// TODO(eastfisher): commit error may cause state infinite loop!
	// TODO(eastfisher): upper layer should recognize network error and then close queryctx.
	q.MustRegisterHandler(State1, State0, EventCommitOrRollback, true, FSMHandlerFunc(fsmHandler_NotReleaseConn_EventCommitOrRollback))
	q.MustRegisterHandler(State1, State3, EventEnableAutoCommit, false, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventEnableAutoCommit))
	q.MustRegisterHandler(State1, State5, EventStmtPrepare, false, FSMStmtPrepareHandlerFunc(fsmHandler_NoPrepare_WithAttachedConn_EventStmtPrepare))
	q.MustRegisterHandler(State1, State1, EventStmtForwardData, true, FSMHandlerFunc(errHandler)) // TODO(eastfisher): test // ERROR 1243 (HY000): Unknown prepared statement handler (10) given to mysqld_stmt_execute
	q.MustRegisterHandler(State1, State1, EventStmtClose, true, FSMHandlerFunc(noopHandler))

	q.MustRegisterHandler(State2, State2, EventEnableAutoCommit, false, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State2, State2, EventCommitOrRollback, false, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State2, State2, EventQuery, false, FSMHandlerFunc(fsmHandler_ConnPool_EventQuery))
	q.MustRegisterHandler(State2, State0, EventDisableAutoCommit, false, FSMHandlerFunc(fsmHandler_PreFetchConn_EventDisableAutoCommit))
	q.MustRegisterHandler(State2, State3, EventBegin, false, FSMHandlerFunc(fsmHandler_PreFetchConn_EventBegin))
	q.MustRegisterHandler(State2, State6, EventStmtPrepare, false, FSMStmtPrepareHandlerFunc(fsmHandler_NoPrepare_PreFetchConn_EventStmtPrepare))
	q.MustRegisterHandler(State2, State2, EventStmtForwardData, true, FSMHandlerFunc(errHandler)) // TODO(eastfisher): test
	q.MustRegisterHandler(State2, State2, EventStmtClose, true, FSMHandlerFunc(noopHandler))

	q.MustRegisterHandler(State3, State3, EventEnableAutoCommit, false, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State3, State3, EventBegin, false, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State3, State3, EventQuery, false, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventQuery))
	q.MustRegisterHandler(State3, State1, EventDisableAutoCommit, false, FSMHandlerFunc(fsmHandler_NotReleaseConn_EventDisableAutoCommit))
	q.MustRegisterHandler(State3, State2, EventCommitOrRollback, true, FSMHandlerFunc(fsmHandler_PostReleaseConn_EventCommitOrRollback))
	q.MustRegisterHandler(State3, State7, EventStmtPrepare, false, FSMStmtPrepareHandlerFunc(fsmHandler_NoPrepare_WithAttachedConn_EventStmtPrepare))
	q.MustRegisterHandler(State3, State3, EventStmtForwardData, true, FSMHandlerFunc(errHandler)) // TODO(eastfisher): test
	q.MustRegisterHandler(State3, State3, EventStmtClose, true, FSMHandlerFunc(noopHandler))

	q.MustRegisterHandler(State4, State4, EventStmtPrepare, true, FSMStmtPrepareHandlerFunc(fsmHandler_IsPrepare_EventStmtPrepare))
	q.MustRegisterHandler(State4, State5, EventStmtForwardData, false, FSMHandlerFunc(fsmHandler_IsPrepare_EventStmtForwardData))
	q.MustRegisterHandler(State4, State0, EventStmtClose, true, FSMHandlerFunc(fsmHandler_NotReleaseConn_EventStmtClose))
	//q.MustRegisterHandler(State4, State4, EventStmtClose, true, nil)  // FIXME(eastfisher): stmt close success may change to State4 or State0
	q.MustRegisterHandler(State4, State5, EventBegin, false, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventBegin))
	q.MustRegisterHandler(State4, State4, EventCommitOrRollback, true, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State4, State4, EventDisableAutoCommit, true, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State4, State6, EventEnableAutoCommit, false, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventEnableAutoCommit))
	q.MustRegisterHandler(State4, State5, EventQuery, false, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventQuery))

	q.MustRegisterHandler(State5, State5, EventStmtPrepare, true, FSMStmtPrepareHandlerFunc(fsmHandler_IsPrepare_EventStmtPrepare))
	q.MustRegisterHandler(State5, State5, EventStmtForwardData, true, FSMHandlerFunc(fsmHandler_IsPrepare_EventStmtForwardData))
	q.MustRegisterHandler(State5, State1, EventStmtClose, true, FSMHandlerFunc(fsmHandler_NotReleaseConn_EventStmtClose))
	q.MustRegisterHandler(State5, State5, EventBegin, true, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State5, State4, EventCommitOrRollback, true, FSMHandlerFunc(fsmHandler_NotReleaseConn_EventCommitOrRollback))
	q.MustRegisterHandler(State5, State5, EventDisableAutoCommit, true, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State5, State7, EventEnableAutoCommit, false, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventEnableAutoCommit))
	q.MustRegisterHandler(State5, State5, EventQuery, true, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventQuery))

	q.MustRegisterHandler(State6, State6, EventStmtPrepare, true, FSMStmtPrepareHandlerFunc(fsmHandler_IsPrepare_EventStmtPrepare))
	q.MustRegisterHandler(State6, State6, EventStmtForwardData, true, FSMHandlerFunc(fsmHandler_IsPrepare_EventStmtForwardData))
	q.MustRegisterHandler(State6, State2, EventStmtClose, true, FSMHandlerFunc(fsmHandler_ReleaseConn_EventStmtClose))
	q.MustRegisterHandler(State6, State7, EventBegin, false, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventBegin))
	q.MustRegisterHandler(State6, State6, EventCommitOrRollback, true, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State6, State4, EventDisableAutoCommit, false, FSMHandlerFunc(fsmHandler_NotReleaseConn_EventDisableAutoCommit))
	q.MustRegisterHandler(State6, State6, EventEnableAutoCommit, true, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State6, State6, EventQuery, true, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventQuery))

	q.MustRegisterHandler(State7, State7, EventStmtPrepare, true, FSMStmtPrepareHandlerFunc(fsmHandler_IsPrepare_EventStmtPrepare))
	q.MustRegisterHandler(State7, State7, EventStmtForwardData, true, FSMHandlerFunc(fsmHandler_IsPrepare_EventStmtForwardData))
	q.MustRegisterHandler(State7, State3, EventStmtClose, true, FSMHandlerFunc(fsmHandler_NotReleaseConn_EventStmtClose))
	q.MustRegisterHandler(State7, State7, EventBegin, true, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State7, State6, EventCommitOrRollback, true, FSMHandlerFunc(fsmHandler_NotReleaseConn_EventCommitOrRollback))
	q.MustRegisterHandler(State7, State5, EventDisableAutoCommit, false, FSMHandlerFunc(fsmHandler_NotReleaseConn_EventDisableAutoCommit))
	q.MustRegisterHandler(State7, State7, EventEnableAutoCommit, true, FSMHandlerFunc(noopHandler))
	q.MustRegisterHandler(State7, State7, EventQuery, true, FSMHandlerFunc(fsmHandler_WithAttachedConn_EventQuery))
}

func fsmHandler_IsPrepare_EventStmtForwardData(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	_ = args[0].(int) // stmtId
	data := args[1].([]byte)
	return b.txnConn.StmtExecuteForward(data)
}

func (q *FSM) MustRegisterHandler(state FSMState, newState FSMState, event FSMEvent, mustChangeState bool, handler FSMHandler) {
	handlerWrapper := &FSMHandlerWrapper{
		NewState:        newState,
		MustChangeState: mustChangeState,
		Handler:         handler,
	}

	_, ok := q.handlersV2[state]
	if !ok {
		q.handlersV2[state] = make(map[FSMEvent]*FSMHandlerWrapper)
	}
	_, ok = q.handlersV2[state][event]
	if ok {
		logutil.BgLogger().Panic("duplicated fsm handler")
	}
	q.handlersV2[state][event] = handlerWrapper
}

func (q *FSM) Call(ctx context.Context, event FSMEvent, conn *BackendConnManager, args ...interface{}) (interface{}, error) {
	action, ok := q.getHandler(conn.state, event)
	if !ok {
		return nil, fmt.Errorf("fsm handler not found")
	}
	ret, err := action.Handler.Handle(conn, ctx, args...)
	if action.MustChangeState || err == nil {
		conn.state = action.NewState
	}
	return ret, err
}

func (q *FSM) getHandler(state FSMState, event FSMEvent) (*FSMHandlerWrapper, bool) {
	eventHandlers, ok := q.handlersV2[state]
	if !ok {
		return nil, false
	}
	eventHandler, ok := eventHandlers[event]
	return eventHandler, ok
}

func fsmHandler_WithAttachedConn_EventQuery(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	db := args[0].(string)
	sql := args[1].(string)
	return b.queryInTxn(ctx, db, sql)
}

func fsmHandler_WithAttachedConn_EventBegin(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	err := b.txnConn.Begin()
	return nil, err
}

func fsmHandler_PostReleaseConn_EventEnableAutoCommit(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	err := b.txnConn.SetAutoCommit(true)
	b.releaseAttachedConn(err)
	return nil, err
}

// TODO(eastfisher): if backend conn is broken, how to exit FSM?
func fsmHandler_NotReleaseConn_EventCommitOrRollback(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	commit := args[0].(bool)
	var err error
	if commit {
		err = b.txnConn.Commit()
	} else {
		err = b.txnConn.Rollback()
	}
	return nil, err
}

func fsmHandler_WithAttachedConn_EventEnableAutoCommit(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	err := b.txnConn.SetAutoCommit(true)
	return nil, err
}

func fsmHandler_ConnPool_EventQuery(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	db := args[0].(string)
	sql := args[1].(string)

	return b.queryWithoutTxn(ctx, db, sql)
}

func fsmHandler_PreFetchConn_EventDisableAutoCommit(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	conn, err := b.ns.GetPooledConn(ctx)
	if err != nil {
		return nil, err
	}
	err = conn.SetAutoCommit(false)
	if err != nil {
		errClosePooledBackendConn(conn, b.ns.Name())
		return nil, err
	}
	b.setAttachedConn(conn)
	return nil, nil
}

func fsmHandler_PreFetchConn_EventBegin(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	conn, err := b.ns.GetPooledConn(ctx)
	if err != nil {
		return nil, err
	}
	if err = conn.Begin(); err != nil {
		errClosePooledBackendConn(conn, b.ns.Name())
		return nil, err
	}
	b.setAttachedConn(conn)
	return nil, nil
}

func fsmHandler_NotReleaseConn_EventDisableAutoCommit(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	err := b.txnConn.SetAutoCommit(false)
	return nil, err
}

func fsmHandler_PostReleaseConn_EventCommitOrRollback(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	commit := args[0].(bool)
	var err error
	if commit {
		err = b.txnConn.Commit()
	} else {
		err = b.txnConn.Rollback()
	}

	if err != nil {
		_ = b.txnConn.Rollback()
		errClosePooledBackendConn(b.txnConn, b.ns.Name())
	} else {
		b.txnConn.PutBack()
	}

	b.unsetAttachedConn()
	return nil, err
}

// TODO(eastfisher): currently we don't change db
func fsmHandler_NoPrepare_WithAttachedConn_EventStmtPrepare(b *BackendConnManager, ctx context.Context, args ...interface{}) (Stmt, error) {
	stmt, err := fsmHandler_IsPrepare_EventStmtPrepare(b, ctx, args...)
	if err != nil {
		return nil, err
	}

	b.isPrepared = true
	return stmt, nil
}

func fsmHandler_NoPrepare_PreFetchConn_EventStmtPrepare(b *BackendConnManager, ctx context.Context, args ...interface{}) (Stmt, error) {
	conn, err := b.ns.GetPooledConn(ctx)
	if err != nil {
		return nil, err
	}

	db := args[0].(string)
	sql := args[1].(string)

	if err = conn.UseDB(db); err != nil {
		errClosePooledBackendConn(conn, b.ns.Name())
		return nil, err
	}
	stmt, err := conn.StmtPrepare(sql)
	if err != nil {
		errClosePooledBackendConn(conn, b.ns.Name())
		return nil, err
	}
	b.setAttachedConn(conn)
	b.isPrepared = true
	return stmt, nil
}

func fsmHandler_IsPrepare_EventStmtPrepare(b *BackendConnManager, ctx context.Context, args ...interface{}) (Stmt, error) {
	db := args[0].(string)
	sql := args[1].(string)

	if err := b.txnConn.UseDB(db); err != nil {
		return nil, err
	}

	stmt, err := b.txnConn.StmtPrepare(sql)
	if err != nil {
		return nil, err
	}

	return stmt, nil
}

func fsmHandler_NotReleaseConn_EventStmtClose(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	stmtId := args[0].(int)
	err := b.txnConn.StmtClosePrepare(stmtId)

	b.isPrepared = false
	return nil, err
}

func fsmHandler_ReleaseConn_EventStmtClose(b *BackendConnManager, ctx context.Context, args ...interface{}) (*mysql.Result, error) {
	stmtId := args[0].(int)
	err := b.txnConn.StmtClosePrepare(stmtId)
	b.releaseAttachedConn(err)
	b.isPrepared = false
	return nil, err
}

func (f FSMState) IsAutoCommit() bool {
	return (f & FSMStateFlagIsAutoCommit) != 0
}

func (f FSMState) IsInTransaction() bool {
	return (f & FSMStateFlagInTransaction) != 0
}

func (f FSMState) IsPrepare() bool {
	return (f & FSMStateFlagInPrepare) != 0
}
