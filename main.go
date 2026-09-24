package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"flowops-executor/config"
	"flowops-executor/runner"
)

func main() {
	// 多来源配置加载（逐字段解析）：
	//   CLI > ENV > 外部 YAML（--config / FLOWOPS_CONFIG / <exeDir>/config/config.{APP_ENV}.yaml）> 内嵌 YAML
	// 配置错误属于明确的启动错误（与"暂时连不上主节点"区分），直接退出。
	res, err := config.Load(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, config.ErrHelp) {
			// -h/--help：用法输出到 stdout，正常退出
			config.PrintUsage(os.Stdout)
			return
		}
		log.Fatalf("加载配置失败: %v", err)
	}

	// 启动日志：打印各字段来源；token 仅显示 configured/missing，不输出明文
	for _, line := range res.LogSummary() {
		fmt.Printf("[INFO] %s\n", line)
	}

	// TODO: 初始化日志
	// TODO: 初始化数据库

	// 退出信号驱动 context：等待重试、拨号、已注册三个阶段都能快速结束
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run 阻塞运行唯一的连接管理循环：主节点未启动或运行中重启时会自动重连，不再直接退出
	r := runner.New(res.Config)
	if err := r.Run(ctx); err != nil {
		log.Fatalf("Runner 运行失败: %v", err)
	}

	fmt.Printf("FlowOps Executor 已停止 (env: %s)\n", res.AppEnv)
}
