package cc

import "C"
import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/pingcap/tidb/util/logutil"
	"github.com/tidb-incubator/weir/pkg/cc/service"
	"github.com/tidb-incubator/weir/pkg/config"
	"go.uber.org/zap"
	"net"
	"net/http"
	"strings"
)

// Server cc admin server
type Server struct {
	cfg *config.CCConfig

	engine   *gin.Engine
	listener net.Listener

	exitC chan struct{}
}

// NewServer constructor of Server
func NewServer(cfg *config.CCConfig) (*Server, error) {
	srv := &Server{cfg: cfg, exitC: make(chan struct{})}
	srv.engine = gin.New()
	l, err := net.Listen("tcp", cfg.CCAdminServer.Addr)
	if err != nil {
		logutil.BgLogger().Error("cc admin_server listen failed", zap.String("addr", cfg.CCAdminServer.Addr))
		return nil, err
	}
	srv.listener = l
	srv.registerURL()
	return srv, nil
}

func (s *Server) registerURL() {
	api := s.engine.Group("/cc", gin.BasicAuth(gin.Accounts{s.cfg.CCAdminServer.User: s.cfg.CCAdminServer.Password}))
	api.Use(gin.Recovery())
	api.Use(gzip.Gzip(gzip.DefaultCompression))
	api.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	})
	api.GET("/namespace/list", s.listNamespace)
	api.GET("/namespace/detail/:name",s.detailNamespace)
	api.PUT("/namespace/modify", s.modifyNamespace)
	api.PUT("/namespace/delete/:name", s.delNamespace)
}

type ListNamespaceResp struct {
	Header *CommonJsonResp `json:"header"`
	Data   []string        `json:"data"`
}

func (s *Server) listNamespace(c *gin.Context) {
	cluster := c.DefaultQuery("cluster", config.DefaultClusterName)
	data, err := service.ListNamespace(s.cfg, cluster)
	if err != nil {
		errMsg := "list names of all namespace failed"
		logutil.BgLogger().Warn(errMsg, zap.Error(err))
		c.JSON(http.StatusOK, CreateFailureJsonResp(errMsg))
		return
	}
	header := CreateSuccessJsonResp()
	c.JSON(http.StatusOK, &ListNamespaceResp{Data: data, Header: &header})
	return
}

type DetailNamespaceResp struct {
	Header *CommonJsonResp     `json:"header"`
	Data   []*config.Namespace `json:"data"`
}

func (s *Server) detailNamespace(c *gin.Context) {
	var names []string
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		c.JSON(http.StatusOK, CreateFailureJsonResp("input name is empty"))
		return
	}
	names = append(names, name)

	cluster := c.DefaultQuery("cluster", config.DefaultClusterName)
	data, err := service.QueryNamespace(names, s.cfg, cluster)
	if err != nil {
		errMsg := "query namespace failed"
		logutil.BgLogger().Warn(errMsg, zap.Error(err))
		c.JSON(http.StatusOK, CreateFailureJsonResp(errMsg))
		return
	}
	header := CreateSuccessJsonResp()
	c.JSON(http.StatusOK, &DetailNamespaceResp{Data: data, Header: &header})
	return
}

func (s *Server) modifyNamespace(c *gin.Context) {
	var namespace config.Namespace
	err := c.BindJSON(&namespace)
	errMsg := "modify namespace failed"
	if err != nil {
		logutil.BgLogger().Warn(errMsg, zap.Error(err))
		c.JSON(http.StatusBadRequest, CreateFailureJsonResp(errMsg))
		return
	}
	cluster := c.DefaultQuery("cluster", config.DefaultClusterName)
	err = service.ModifyNamespace(&namespace, s.cfg, cluster)
	if err != nil {
		logutil.BgLogger().Warn(errMsg, zap.Error(err))
		c.JSON(http.StatusOK, CreateFailureJsonResp(errMsg))
		return
	}
	c.JSON(http.StatusOK, CreateSuccessJsonResp())
	return
}

func (s *Server) delNamespace(c *gin.Context) {
	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		c.JSON(http.StatusOK, CreateFailureJsonResp("input name is empty"))
		return
	}
	cluster := c.DefaultQuery("cluster", config.DefaultClusterName)
	err := service.DelNamespace(name, s.cfg, cluster)
	if err != nil {
		errMsg := "delete namespace failed"
		logutil.BgLogger().Warn(errMsg, zap.Error(err))
		c.JSON(http.StatusOK, CreateFailureJsonResp(errMsg))
		return
	}
	c.JSON(http.StatusOK, CreateSuccessJsonResp())
}

type CommonJsonResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func CreateSuccessJsonResp() CommonJsonResp {
	return CommonJsonResp{
		Code: http.StatusOK,
		Msg:  "success",
	}
}

func CreateFailureJsonResp(msg string) CommonJsonResp {
	return CommonJsonResp{
		Code: http.StatusInternalServerError,
		Msg:  msg,
	}
}

func (s *Server) Run() {
	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {
			logutil.BgLogger().Warn("listener close failed", zap.Error(err))
		}
	}(s.listener)

	errC := make(chan error)

	go func(l net.Listener) {
		h := http.NewServeMux()
		h.Handle("/", s.engine)
		hs := &http.Server{Handler: h}
		errC <- hs.Serve(l)
	}(s.listener)

	select {
	case <-s.exitC:
		logutil.BgLogger().Info("server exit.")
		return
	case err := <-errC:
		logutil.BgLogger().Fatal("gaea cc serve failed", zap.Error(err))
		return
	}
}

func (s *Server) Close() {
	s.exitC <- struct{}{}
	return
}
