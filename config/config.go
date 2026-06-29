package config

type Config struct {
	Runner struct {
		Id                string `yaml:"id"`                 // runner 唯一标识
		MasterAddr        string `yaml:"master_addr"`        // master 地址，如 127.0.0.1:9000
		HeartbeatInterval int    `yaml:"heartbeat_interval"` // 心跳间隔（秒），默认 10
		Version           string `yaml:"version"`            // runner 版本号
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
