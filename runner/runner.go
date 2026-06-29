package runner

import (
	"context"
	"fmt"
	"flowops-executor/config"
	"net"
	"time"

	"github.com/leehaohaohao/nexa-protocol/go/client"
)

type Runner struct {
	client *client.Client
}

func New(cfg *config.Config) *Runner {
	opts := []client.Option{
		client.WithRunnerId(cfg.Runner.Id),
		client.WithVersion(cfg.Runner.Version),
	}

	if cfg.Runner.HeartbeatInterval > 0 {
		opts = append(opts, client.WithHeartbeatInterval(time.Duration(cfg.Runner.HeartbeatInterval)*time.Second))
	}

	if ip := getLocalIP(); ip != "" {
		opts = append(opts, client.WithIP(ip))
	}

	return &Runner{
		client: client.New(opts...),
	}
}

func (r *Runner) Start(ctx context.Context, masterAddr string) error {
	fmt.Printf("[runner] 正在连接 Master: %s\n", masterAddr)
	if err := r.client.Connect(masterAddr); err != nil {
		return fmt.Errorf("连接 Master 失败: %w", err)
	}

	resp, err := r.client.Register()
	if err != nil {
		return fmt.Errorf("注册失败: %w", err)
	}
	fmt.Printf("[runner] 注册成功: %s\n", resp.GetMessage())

	r.client.StartHeartbeat(ctx)
	fmt.Println("[runner] 心跳已启动")

	go func() {
		<-ctx.Done()
		r.Stop()
	}()

	return nil
}

func (r *Runner) Stop() {
	fmt.Println("[runner] 正在断开连接...")
	if err := r.client.Disconnect("shutdown"); err != nil {
		fmt.Printf("[runner] 断开连接失败: %v\n", err)
	} else {
		fmt.Println("[runner] 已断开连接")
	}
}

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
