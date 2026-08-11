package runner

import (
	"context"
	"fmt"
	"flowops-executor/config"
	"net"
	"sync/atomic"
	"time"

	"github.com/leehaohaohao/nexa-protocol/go/client"
	"github.com/leehaohaohao/nexa-protocol/go/codec"
)

type Runner struct {
	client            *client.Client
	runnerId          string
	heartbeatInterval time.Duration
	runningTasks      atomic.Int32
}

func New(cfg *config.Config) *Runner {
	opts := []client.Option{
		client.WithRunnerId(cfg.Runner.Id),
		client.WithVersion(cfg.Runner.Version),
	}

	if ip := getLocalIP(); ip != "" {
		opts = append(opts, client.WithIP(ip))
	}

	return &Runner{
		client:            client.New(opts...),
		runnerId:          cfg.Runner.Id,
		heartbeatInterval: time.Duration(cfg.Runner.HeartbeatInterval) * time.Second,
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

	r.startHeartbeat(ctx)
	fmt.Println("[runner] 心跳已启动")

	r.startTaskLoop(ctx)
	fmt.Println("[runner] 任务接收循环已启动")

	return nil
}

// startHeartbeat 定时上报真实运行任务数与节点 CPU/内存使用率
func (r *Runner) startHeartbeat(ctx context.Context) {
	interval := r.heartbeatInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cpuPercent, memPercent := collectHostMetrics()
				running := r.runningTasks.Load()

				env := codec.BuildHeartbeatRequest(r.runnerId, running, cpuPercent, memPercent)
				data, err := codec.MarshalEnvelope(env)
				if err != nil {
					continue
				}
				if err := codec.WriteFrame(r.client.Conn(), data); err != nil {
					fmt.Printf("[runner] 心跳发送失败: %v\n", err)
					return
				}
			}
		}
	}()
}

func (r *Runner) Stop() {
	fmt.Println("[runner] 正在断开连接...")

	// 发送 disconnect 消息
	env := codec.BuildDisconnectRequest(r.runnerId, "shutdown")
	data, _ := codec.MarshalEnvelope(env)
	_ = codec.WriteFrame(r.client.Conn(), data)

	// 等待 TCP 发送缓冲区刷新，确保对端读取到 disconnect
	time.Sleep(200 * time.Millisecond)

	_ = r.client.Conn().Close()
	fmt.Println("[runner] 已断开连接")
}

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
