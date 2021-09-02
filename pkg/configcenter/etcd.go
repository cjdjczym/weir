package configcenter

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"time"

	"github.com/pingcap/errors"
	"github.com/pingcap/tidb/util/logutil"
	"github.com/tidb-incubator/weir/pkg/config"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/mvcc/mvccpb"
	"go.uber.org/zap"
)

const (
	DefaultEtcdDialTimeout = 3 * time.Second
)

type EtcdConfigCenter struct {
	etcdClient  *clientv3.Client
	kv          clientv3.KV
	basePath    string
	strictParse bool
}

func CreateEtcdConfigCenter(cfg config.ConfigEtcd) (*EtcdConfigCenter, error) {
	etcdConfig := clientv3.Config{
		Endpoints:   cfg.Addrs,
		Username:    cfg.Username,
		Password:    cfg.Password,
		DialTimeout: DefaultEtcdDialTimeout,
	}
	etcdClient, err := clientv3.New(etcdConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "create etcd config center error")
	}

	center := NewEtcdConfigCenter(etcdClient, cfg.BasePath, cfg.StrictParse)
	return center, nil
}

func NewEtcdConfigCenter(etcdClient *clientv3.Client, basePath string, strictParse bool) *EtcdConfigCenter {
	return &EtcdConfigCenter{
		etcdClient:  etcdClient,
		kv:          clientv3.NewKV(etcdClient),
		basePath:    basePath,
		strictParse: strictParse,
	}
}

func (e *EtcdConfigCenter) get(ctx context.Context, key string, cluster string) (*mvccpb.KeyValue, error) {
	resp, err := e.kv.Get(ctx, path.Join(e.basePath, "namespace", cluster, key))
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("key not found")
	}
	return resp.Kvs[0], nil
}

// list the string subPath should be `namespace / proxy` + clusterName
func (e *EtcdConfigCenter) list(ctx context.Context, subPath string) ([]*mvccpb.KeyValue, error) {
	resp, err := e.kv.Get(ctx, path.Join(e.basePath, subPath), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	return resp.Kvs, nil
}

func (e *EtcdConfigCenter) GetNamespace(ns string, cluster string) (*config.Namespace, error) {
	ctx := context.Background()
	etcdKeyValue, err := e.get(ctx, ns, cluster)
	if err != nil {
		return nil, err
	}
	n := &config.Namespace{}
	err = json.Unmarshal(etcdKeyValue.Value, n)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (e *EtcdConfigCenter) ListAllNamespace(cluster string) ([]*config.Namespace, error) {
	ctx := context.Background()
	etcdKeyValues, err := e.list(ctx, path.Join("namespace", cluster))
	if err != nil {
		return nil, err
	}
	var ret []*config.Namespace
	for _, kv := range etcdKeyValues {
		n := &config.Namespace{}
		err = json.Unmarshal(kv.Value, n)
		if err != nil {
			if e.strictParse {
				return nil, err
			} else {
				logutil.BgLogger().Warn("parse namespace config error", zap.Error(err), zap.ByteString("namespace", kv.Key))
				continue
			}
		}
		ret = append(ret, n)
	}
	return ret, nil
}

func (e *EtcdConfigCenter) ListAllNamespaceStringArray(cluster string) ([]string, error) {
	ctx := context.Background()
	etcdKeyValues, err := e.list(ctx, path.Join("namespace", cluster))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, kv := range etcdKeyValues {
		files = append(files, string(kv.Value))
	}
	return files, nil
}

func (e *EtcdConfigCenter) ListProxyMonitorMetrics(cluster string) (map[string]*config.ProxyMonitorMetric, error) {
	ctx := context.Background()
	etcdKeyValues, err := e.list(ctx, path.Join("proxy", cluster))
	if err != nil {
		return nil, err
	}
	proxy := make(map[string]*config.ProxyMonitorMetric)
	for _, kv := range etcdKeyValues {
		p := &config.ProxyMonitorMetric{}
		if err := json.Unmarshal(kv.Value, &p); err != nil {
			return nil, err
		}
		proxy[p.Token] = p
	}
	return proxy, nil
}

func (e *EtcdConfigCenter) SetNamespace(ns string, value string, cluster string) error {
	ctx := context.Background()
	_, err := e.kv.Put(ctx, path.Join(e.basePath, "namespace", cluster, ns), value)
	return err
}

func (e *EtcdConfigCenter) DelNamespace(ns string, cluster string) error {
	ctx := context.Background()
	_, err := e.kv.Delete(ctx, path.Join(e.basePath, "namespace", cluster, ns))
	return err
}

func (e *EtcdConfigCenter) Close() {
	if err := e.etcdClient.Close(); err != nil {
		logutil.BgLogger().Error("close etcd client error", zap.Error(err))
	}
}
