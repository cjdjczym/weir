package config

type Namespace struct {
	Version     string            `yaml:"version" json:"version"`
	Namespace   string            `yaml:"namespace" json:"namespace"`
	Frontend    FrontendNamespace `yaml:"frontend" json:"frontend"`
	Backend     BackendNamespace  `yaml:"backend" json:"backend"`
	Breaker     BreakerInfo       `yaml:"breaker" json:"breaker"`
	RateLimiter RateLimiterInfo   `yaml:"rate_limiter" json:"rate_limiter"`
}

type FrontendNamespace struct {
	AllowedDBs   []string           `yaml:"allowed_dbs" json:"allowed_dbs"`
	SlowSQLTime  int                `yaml:"slow_sql_time" json:"slow_sql_time"`
	DeniedIPs    []string           `yaml:"denied_ips" json:"denied_ips"`
	IdleTimeout  int                `yaml:"idle_timeout" json:"idle_timeout"`
	Users        []FrontendUserInfo `yaml:"users" json:"users"`
	SQLBlackList []SQLInfo          `yaml:"sql_blacklist" json:"sql_blacklist"`
	SQLWhiteList []SQLInfo          `yaml:"sql_whitelist" json:"sql_whitelist"`
}

type FrontendUserInfo struct {
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

type SQLInfo struct {
	SQL string `yaml:"sql" json:"sql"`
}

type RateLimiterInfo struct {
	Scope string `yaml:"scope" json:"scope"`
	QPS   int    `yaml:"qps" json:"qps"`
}

type BackendNamespace struct {
	Username     string   `yaml:"username" json:"username"`
	Password     string   `yaml:"password" json:"password"`
	Instances    []string `yaml:"instances" json:"instances"`
	SelectorType string   `yaml:"selector_type" json:"selector_type"`
	PoolSize     int      `yaml:"pool_size" json:"pool_size"`
	IdleTimeout  int      `yaml:"idle_timeout" json:"idle_timeout"`
}

type StrategyInfo struct {
	MinQps               int64 `yaml:"min_qps" json:"min_qps"`
	SqlTimeoutMs         int64 `yaml:"sql_timeout_ms" json:"sql_timeout_ms"`
	FailureRatethreshold int64 `yaml:"failure_rate_threshold" json:"failure_rate_threshold"`
	FailureNum           int64 `yaml:"failure_num" json:"failure_num"`
	OpenStatusDurationMs int64 `yaml:"open_status_duration_ms" json:"open_status_duration_ms"`
	Size                 int64 `yaml:"size" json:"size"`
	CellIntervalMs       int64 `yaml:"cell_interval_ms" json:"cell_interval_ms"`
}

type BreakerInfo struct {
	Scope      string         `yaml:"scope" json:"scope"`
	Strategies []StrategyInfo `yaml:"strategies" json:"strategies"`
}
