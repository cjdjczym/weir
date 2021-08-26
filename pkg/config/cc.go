package config

import (
	"encoding/json"
	"go.uber.org/zap"
	"log"
)

type CCConfig struct {
	CCProxyServer  CCProxyServer `yaml:"proxy_server"`
	CCAdminServer  CCAdminServer `yaml:"admin_server"`
	CCEtcdConfig   ConfigEtcd  `yaml:"config_etcd"`
}

type CCProxyServer struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

type CCAdminServer struct {
	Addr     string `yaml:"addr"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// ProxyMonitorMetric proxy register information
type ProxyMonitorMetric struct {
	Token     string `json:"token"` //目前同AdminAddr
	StartTime string `json:"start_time"`

	IP        string `json:"ip"`
	AdminPort string `json:"admin_port"`
	ProxyPort string `json:"proxy_port"`

	Pid int    `json:"pid"`
	Pwd string `json:"pwd"`
	Sys string `json:"sys"`
}

func Encode(v interface{}) []byte {
	b, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		log.Fatal("encode to json failed", zap.Error(err))
	}
	return b
}