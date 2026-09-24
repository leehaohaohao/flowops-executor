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
	// 加载配置：配置错误属于明确的启动错误，与"暂时连不上主节点"区分处理，直接退出
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "prod"
	}
	configPath := fmt.Sprintf("config/config.%s.yaml", env)

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置文件失败 (%s): %v", configPath, err)
	}

	fmt.Printf("配置加载成功: runner id=%s, master=%s\n", cfg.Runner.Id, cfg.Runner.MasterAddr)

	// TODO: 初始化日志
	// TODO: 初始化数据库

	// 退出信号驱动 context：等待重试、拨号、已注册三个阶段都能快速结束
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run 阻塞运行唯一的连接管理循环：主节点未启动或运行中重启时会自动重连，不再直接退出
	r := runner.New(cfg)
	if err := r.Run(ctx); err != nil {
		log.Fatalf("Runner 运行失败: %v", err)
	}

	fmt.Printf("FlowOps Executor 已停止 (env: %s)\n", env)
}
