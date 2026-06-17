package config

type Config struct {
	Server struct {
		Port int    `yaml:"port"`
		Mode string `yaml:"mode"` // debug | release
	} `yaml:"server"`
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
