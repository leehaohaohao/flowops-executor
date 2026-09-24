package config

// Config 最终生效的配置。
//
// 各字段值由「多来源逐字段解析」得到，优先级固定为：
//
//	CLI > ENV > 外部 YAML > 内嵌 YAML
//
// 详见 resolver.go 与 README「配置优先级」章节。
type Config struct {
	Runner   RunnerConfig   `yaml:"runner"`
	Database DatabaseConfig `yaml:"database"`
	Log      LogConfig      `yaml:"log"`
}

// RunnerConfig runner 相关配置。
//
// 其中 id / master_addr / token / version 为「核心字段」，支持 CLI 与 ENV 覆盖；
// 其余字段当前仅支持「外部 YAML > 内嵌 YAML」两级覆盖（后续可按需追加 CLI/ENV）。
type RunnerConfig struct {
	Id                string `yaml:"id"`                 // runner 唯一标识（核心字段）
	MasterAddr        string `yaml:"master_addr"`        // master 地址 host:port（核心字段）
	HeartbeatInterval int    `yaml:"heartbeat_interval"` // 心跳间隔（秒），默认 10
	Version           string `yaml:"version"`            // runner 版本号（核心字段）
	Token             string `yaml:"token"`              // 注册令牌（核心字段）

	// 连接恢复参数（主节点未启动/重启时的重试行为）
	ConnectTimeout           int `yaml:"connect_timeout"`            // 单次拨号超时（秒），默认 5
	ReconnectInitialInterval int `yaml:"reconnect_initial_interval"` // 重连初始退避（秒），默认 1
	ReconnectMaxInterval     int `yaml:"reconnect_max_interval"`     // 重连最大退避（秒），默认 30
	RegisterTimeout          int `yaml:"register_timeout"`           // 注册响应超时（秒），默认 10
	AuthRetryInterval        int `yaml:"auth_retry_interval"`        // token 无效/节点未登记时的低频重试间隔（秒），默认 60
}

// DatabaseConfig 数据库配置（当前尚未使用，保留以兼容既有 YAML）
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	Charset  string `yaml:"charset"`
}

// LogConfig 日志配置（当前尚未使用，保留以兼容既有 YAML）
type LogConfig struct {
	Level      string `yaml:"level"` // debug | info | warn | error
	FilePath   string `yaml:"file_path"`
	MaxSize    int    `yaml:"max_size"`    // 单个日志文件最大尺寸，单位MB
	MaxBackups int    `yaml:"max_backups"` // 保留旧日志文件最大数量
	MaxAge     int    `yaml:"max_age"`     // 保留旧日志文件最大天数
}
