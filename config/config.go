package config

type Config struct {
	Runner struct {
		Id                string `yaml:"id"`                 // runner 唯一标识
		MasterAddr        string `yaml:"master_addr"`        // master 地址，如 127.0.0.1:9090
		HeartbeatInterval int    `yaml:"heartbeat_interval"` // 心跳间隔（秒），默认 10
		Version           string `yaml:"version"`            // runner 版本号
		Token             string `yaml:"token"`              // 注册令牌（L1 认证，与主节点 nexa_node 录入的一致）

		// 连接恢复参数（主节点未启动/重启时的重试行为）
		ConnectTimeout           int `yaml:"connect_timeout"`            // 单次拨号超时（秒），默认 5
		ReconnectInitialInterval int `yaml:"reconnect_initial_interval"` // 重连初始退避（秒），默认 1
		ReconnectMaxInterval     int `yaml:"reconnect_max_interval"`     // 重连最大退避（秒），默认 30
		RegisterTimeout          int `yaml:"register_timeout"`           // 注册响应超时（秒），默认 10
		AuthRetryInterval        int `yaml:"auth_retry_interval"`        // token 无效/节点未登记时的低频重试间隔（秒），默认 60
	} `yaml:"runner"`
	Database struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		DBName   string `yaml:"dbname"`
		Charset  string `yaml:"charset"`
	} `yaml:"database"`
	Log struct {
		Level      string `yaml:"level"`       // debug | info | warn | error
		FilePath   string `yaml:"file_path"`
		MaxSize    int    `yaml:"max_size"`    // 单个日志文件最大尺寸，单位MB
		MaxBackups int    `yaml:"max_backups"` // 保留旧日志文件最大数量
		MaxAge     int    `yaml:"max_age"`     // 保留旧日志文件最大天数
	} `yaml:"log"`
}
