package main

import (
	"embed"
	"fmt"
	"flowops-executor/config"
	"io/fs"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

//go:embed config/*
var configFS embed.FS

func loadConfig(path string) (*config.Config, error) {
	data, err := fs.ReadFile(configFS, path)
	if err != nil {
		return nil, err
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func main() {
	// 加载配置
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "dev"
	}
	configPath := fmt.Sprintf("config/config.%s.yaml", env)

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}

	fmt.Printf("配置加载成功: server port=%d, mode=%s\n", cfg.Server.Port, cfg.Server.Mode)

	// TODO: 初始化日志
	// TODO: 初始化数据库
	// TODO: 启动服务

	fmt.Printf("FlowOps Executor 启动在 :%d (env: %s)\n", cfg.Server.Port, env)
}
