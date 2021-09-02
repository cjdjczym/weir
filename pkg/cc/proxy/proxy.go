package proxy

import (
	"fmt"
	"github.com/pingcap/tidb/util/logutil"
	"github.com/tidb-incubator/weir/pkg/config"
	"github.com/tidb-incubator/weir/pkg/util/requests"
	"go.uber.org/zap"
	"net/url"
	"time"
)

// Stats proxy stats
type Stats struct {
	Host     string `json:"host"`
	Closed   bool   `json:"closed"`
	Error    string `json:"error"`
	UnixTime int64  `json:"unixtime"`
	Timeout  bool   `json:"timeout"`
}

// GetStats return proxy status
func GetStats(p *config.ProxyMonitorMetric, cfg *config.CCConfig, timeout time.Duration) *Stats {
	bytes := config.Encode(p)
	fmt.Println(string(bytes))
	var ch = make(chan struct{})
	var host = p.IP + ":" + p.AdminPort
	fmt.Println(host)
	stats := &Stats{}

	go func(host string) {
		defer close(ch)
		stats.Host = host
		err := pingCheck(host, cfg.CCProxyServer.User, cfg.CCProxyServer.Password)
		if err != nil {
			stats.Error = err.Error()
			stats.Closed = true
		} else {
			stats.Closed = false
		}
	}(host)

	select {
	case <-ch:
		return stats
	case <-time.After(timeout):
		return &Stats{Host: host, Timeout: true}
	}
}

func pingCheck(host, user, password string) error {
	URL := EncodeURL(host, "namespace/ping")
	if _, err := requests.SendGet(URL, user, password); err != nil {
		logutil.BgLogger().Fatal("call rpc xping to proxy failed")
		return err
	}
	return nil
}

// PrepareConfig prepare phase of config change
func PrepareConfig(host, name string, cfg *config.CCConfig) error {
	err := pingCheck(host, cfg.CCProxyServer.User, cfg.CCProxyServer.Password)
	if err != nil {
		return err
	}
	URL := EncodeURL(host, "namespace/reload/prepare/%s", name)
	err = requests.SendPut(URL, cfg.CCProxyServer.User, cfg.CCProxyServer.Password)
	if err != nil {
		logutil.BgLogger().Fatal("prepare proxy config failed", zap.Error(err))
		return err
	}
	return nil
}

// CommitConfig commit phase of config change
func CommitConfig(host, name string, cfg *config.CCConfig) error {
	err := pingCheck(host, cfg.CCProxyServer.User, cfg.CCProxyServer.Password)
	if err != nil {
		return err
	}
	URL := EncodeURL(host, "namespace/reload/commit/%s", name)
	err = requests.SendPut(URL, cfg.CCProxyServer.User, cfg.CCProxyServer.Password)
	if err != nil {
		logutil.BgLogger().Fatal("commit proxy config failed", zap.Error(err))
		return err
	}
	return nil
}

// DelNamespace delete namespace
func DelNamespace(host, name string, cfg *config.CCConfig) error {
	err := pingCheck(host, cfg.CCProxyServer.User, cfg.CCProxyServer.Password)
	if err != nil {
		return err
	}
	URL := EncodeURL(host, "namespace/remove/%s", name)
	err = requests.SendPut(URL, cfg.CCProxyServer.User, cfg.CCProxyServer.Password)
	if err != nil {
		logutil.BgLogger().Warn("delete namespace in proxy failed", zap.String("name", name), zap.String("host", host), zap.Error(err))
		return err
	}
	return nil
}

func EncodeURL(host string, format string, args ...interface{}) string {
	var u url.URL
	u.Scheme = "http"
	u.Host = host
	u.Path = fmt.Sprintf(format, args...)
	return u.String()
}
