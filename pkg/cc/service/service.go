package service

import (
	"github.com/pingcap/tidb/util/logutil"
	"github.com/tidb-incubator/weir/pkg/cc/proxy"
	"github.com/tidb-incubator/weir/pkg/config"
	"github.com/tidb-incubator/weir/pkg/configcenter"
	"go.uber.org/zap"
)

func ListNamespace(cfg *config.CCConfig, cluster string) ([]string, error) {
	center, err := configcenter.CreateEtcdConfigCenter(cfg.CCEtcdConfig)
	if err != nil {
		logutil.BgLogger().Warn("create etcd config center failed", zap.Error(err))
		return nil, err
	}
	defer center.Close()

	return center.ListAllNamespaceStringArray(cluster)
}

func QueryNamespace(names []string, cfg *config.CCConfig, cluster string) (data []*config.Namespace, err error) {
	center, err := configcenter.CreateEtcdConfigCenter(cfg.CCEtcdConfig)
	if err != nil {
		logutil.BgLogger().Warn("create etcd config center failed", zap.Error(err))
		return nil, err
	}
	defer center.Close()

	for _, v := range names {
		namespace, err := center.GetNamespace(v, cluster)
		if err != nil {
			logutil.BgLogger().Warn("load namespace failed", zap.String("namespace", v), zap.Error(err))
			return nil, err
		}
		if namespace == nil {
			logutil.BgLogger().Warn("namespace not found", zap.String("namespace", v))
			return data, nil
		}
		data = append(data, namespace)
	}
	return data, nil
}

func ModifyNamespace(namespace *config.Namespace, cfg *config.CCConfig, cluster string) (err error) {
	center, err := configcenter.CreateEtcdConfigCenter(cfg.CCEtcdConfig)
	if err != nil {
		logutil.BgLogger().Warn("create etcd config center failed", zap.Error(err))
		return err
	}
	defer center.Close()

	bytes := config.Encode(namespace)
	err = center.SetNamespace(namespace.Namespace, string(bytes), cluster)
	if err != nil {
		logutil.BgLogger().Warn("update namespace failed", zap.Error(err))
		return err
	}
	proxies, err := center.ListProxyMonitorMetrics(cluster)
	if err != nil {
		logutil.BgLogger().Warn("list proxies failed", zap.Error(err))
		return err
	}
	for _, v := range proxies {
		err := proxy.PrepareConfig(v.IP+":"+v.AdminPort, namespace.Namespace, cfg)
		if err != nil {
			return err
		}
	}
	for _, v := range proxies {
		err := proxy.CommitConfig(v.IP+":"+v.AdminPort, namespace.Namespace, cfg)
		if err != nil {
			return err
		}
	}
	return nil
}

// DelNamespace delete namespace
func DelNamespace(name string, cfg *config.CCConfig, cluster string) error {
	center, err := configcenter.CreateEtcdConfigCenter(cfg.CCEtcdConfig)
	if err != nil {
		logutil.BgLogger().Warn("create etcd config center failed", zap.Error(err))
		return err
	}
	defer center.Close()

	if err := center.DelNamespace(name, cluster); err != nil {
		logutil.BgLogger().Warn("delete namespace failed", zap.String("name", name), zap.Error(err))
		return err
	}
	proxies, err := center.ListProxyMonitorMetrics(cluster)
	if err != nil {
		logutil.BgLogger().Warn("list proxies failed", zap.Error(err))
		return err
	}
	for _, v := range proxies {
		err := proxy.DelNamespace(v.IP+":"+v.AdminPort, name, cfg)
		if err != nil {
			return err
		}
	}
	return nil
}
