package main

import (
	"context"
	"embed"
	"fmt"
	"flowops-executor/config"
	"flowops-executor/runner"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"

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
		env = "prod"
	}
	configPath := fmt.Sprintf("config/config.%s.yaml", env)

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}

	fmt.Printf("配置加载成功: runner id=%s, master=%s\n", cfg.Runner.Id, cfg.Runner.MasterAddr)

	// TODO: 初始化日志
	// TODO: 初始化数据库

	// 启动 Runner 客户端
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := runner.New(cfg)
	if err := r.Start(ctx, cfg.Runner.MasterAddr); err != nil {
		log.Fatalf("Runner 启动失败: %v", err)
	}

	// 信号量优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("\n收到关闭信号，正在优雅退出...")
	cancel()

	fmt.Printf("FlowOps Executor 已停止 (env: %s)\n", env)
}
