package runner

import (
	"context"
	"errors"
	"flowops-executor/config"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/leehaohaohao/nexa-protocol/go/client"
	"github.com/leehaohaohao/nexa-protocol/go/codec"
	"github.com/leehaohaohao/nexa-protocol/go/messages"
)

const (
	defaultHeartbeatInterval = 10 * time.Second
	defaultConnectTimeout    = 5 * time.Second
	defaultRegisterTimeout   = 10 * time.Second
	defaultBackoffInitial    = time.Second
	defaultBackoffMax        = 30 * time.Second
	defaultAuthRetryInterval = 60 * time.Second

	// disconnectFlushWait 发送 disconnect 后等待 TCP 发送缓冲区刷新的时间
	disconnectFlushWait = 200 * time.Millisecond
)

// Runner 子节点执行器：持有连接参数与连接管理逻辑。
// Run 内维护唯一的连接管理循环，成功注册后由 session 承载一次会话的全部协程。
type Runner struct {
	runnerId   string
	masterAddr string
	version    string
	token      string
	ip         string

	heartbeatInterval time.Duration
	connectTimeout    time.Duration
	registerTimeout   time.Duration
	backoffInitial    time.Duration
	backoffMax        time.Duration
	authRetryInterval time.Duration

	runningTasks atomic.Int32
}

// session 一次成功注册的会话：心跳、消息读取、产物接收共用其生命周期与连接。
// 显式传递 session（而非从 Runner 字段读取当前连接）可防止旧会话向新连接回写；
// conn 在会话建立时缓存，会话内不变——即使 client.Close() 后引用仍有效（写已关闭连接返回错误而非 panic）。
type session struct {
	client          *client.Client
	conn            net.Conn
	runnerId        string
	registerMessage string
}

// registerRejectedError 主节点明确拒绝注册（token 无效 / 节点未登记）。
// 属于配置/认证类错误，与网络类错误区分处理，避免每秒高频重试。
// 判定依据是协议库 Register 在拒绝时返回非 nil 的响应体，不做错误字符串匹配。
type registerRejectedError struct {
	message string
}

func (e *registerRejectedError) Error() string {
	return fmt.Sprintf("主节点拒绝注册: %s", e.message)
}

func New(cfg *config.Config) *Runner {
	return &Runner{
		runnerId:          cfg.Runner.Id,
		masterAddr:        cfg.Runner.MasterAddr,
		version:           cfg.Runner.Version,
		token:             cfg.Runner.Token,
		ip:                getLocalIP(),
		heartbeatInterval: secondsOrDefault(cfg.Runner.HeartbeatInterval, defaultHeartbeatInterval),
		connectTimeout:    secondsOrDefault(cfg.Runner.ConnectTimeout, defaultConnectTimeout),
		registerTimeout:   secondsOrDefault(cfg.Runner.RegisterTimeout, defaultRegisterTimeout),
		backoffInitial:    secondsOrDefault(cfg.Runner.ReconnectInitialInterval, defaultBackoffInitial),
		backoffMax:        secondsOrDefault(cfg.Runner.ReconnectMaxInterval, defaultBackoffMax),
		authRetryInterval: secondsOrDefault(cfg.Runner.AuthRetryInterval, defaultAuthRetryInterval),
	}
}

// Run 阻塞运行连接管理循环，直到 ctx 被取消（收到退出信号）：
// 连接 → 带 token 注册 → 心跳 + 消息接收；任一环节失败即关闭本次会话，
// 按退避等待后重试。主节点晚于子节点启动、或运行中重启，均可自动恢复。
//
// 主循环只有这里一个所有者：调用方（main）只调用一次 Run，不对连接做其它管理。
func (r *Runner) Run(ctx context.Context) error {
	backoff := r.backoffInitial
	attempt := 0

	for {
		if ctx.Err() != nil {
			return nil
		}

		attempt++
		fmt.Printf("[runner] 第 %d 次尝试连接 Master: %s\n", attempt, r.masterAddr)

		err := r.runSession(ctx)
		if ctx.Err() != nil {
			return nil // 退出信号：会话已清理，直接结束
		}

		wait := backoff
		var rejected *registerRejectedError
		if errors.As(err, &rejected) {
			// 认证/登记失败：不刷请求，进入低频受控等待；主节点侧补录 token 后无需重启子节点
			fmt.Printf("[runner] %v\n", err)
			fmt.Printf("[runner] 请检查 runner.token 与主节点节点登记，%s 后重试\n", r.authRetryInterval)
			wait = r.authRetryInterval
			backoff = r.backoffInitial // 认证恢复后从初始退避重新开始
		} else {
			fmt.Printf("[runner] 会话结束: %v（%s 后重试）\n", err, wait)
			backoff = nextBackoff(backoff, r.backoffMax)
		}

		if !sleepCtx(ctx, jitter(wait)) {
			return nil // 等待期间收到退出信号
		}
	}
}

// runSession 执行一次完整会话：连接 → 注册 → 心跳 + 消息接收，直到出错或 ctx 取消。
// 返回前确保本会话的所有协程已退出、连接已关闭，再允许下一轮尝试，
// 避免重复心跳、两个 goroutine 同时读同一连接、旧会话向新连接回写。
func (r *Runner) runSession(ctx context.Context) error {
	sess, err := r.connectAndRegister(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("[runner] 注册成功: %s\n", sess.registerMessage)

	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(2)
	go func() { defer wg.Done(); errCh <- r.heartbeatLoop(sessionCtx, sess) }()
	go func() { defer wg.Done(); errCh <- r.readLoop(sessionCtx, sess) }()
	fmt.Println("[runner] 心跳与任务接收循环已启动")

	var sessionErr error
	select {
	case <-ctx.Done():
		fmt.Println("[runner] 收到退出信号，正在断开连接...")
		sess.disconnect("shutdown")
	case sessionErr = <-errCh:
		fmt.Printf("[runner] 会话中断: %v\n", sessionErr)
	}

	cancel()                // 通知另一协程退出
	_ = sess.client.Close() // 幂等关闭连接，解除阻塞中的读写
	wg.Wait()               // 等协程真正退出再进入下一轮
	fmt.Println("[runner] 会话已清理")

	return sessionErr
}

// connectAndRegister 建立新会话：全新的 client/连接 + 带 token 注册。
// 拨号与注册都在可取消的有界超时下执行（协议 v0.6.0 的 ConnectContext/RegisterContext），
// 等待期间收到退出信号会立即中断，不会卡在单次尝试里。
func (r *Runner) connectAndRegister(ctx context.Context) (*session, error) {
	opts := []client.Option{
		client.WithRunnerId(r.runnerId),
		client.WithVersion(r.version),
		client.WithToken(r.token),
	}
	if r.ip != "" {
		opts = append(opts, client.WithIP(r.ip))
	}
	// 每次尝试都使用全新的 client/连接，不复用上一次失败会话的状态
	c := client.New(opts...)

	dialCtx, cancelDial := context.WithTimeout(ctx, r.connectTimeout)
	err := c.ConnectContext(dialCtx, r.masterAddr)
	cancelDial()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err() // 拨号期间收到退出信号
		}
		return nil, fmt.Errorf("连接 Master 失败: %w", err)
	}

	// 注册响应必须有超时：主节点接受 TCP 却不回响应时不能永久阻塞
	regCtx, cancelReg := context.WithTimeout(ctx, r.registerTimeout)
	resp, err := c.RegisterContext(regCtx)
	cancelReg()
	if err != nil {
		_ = c.Close()
		if resp != nil && !resp.GetSuccess() {
			// 主节点明确拒绝（协议库约定：被拒时返回非 nil 响应体），不做错误字符串匹配
			return nil, &registerRejectedError{message: resp.GetMessage()}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err() // 注册期间收到退出信号
		}
		return nil, fmt.Errorf("注册失败: %w", err)
	}

	conn := c.Conn()
	if conn == nil {
		_ = c.Close()
		return nil, fmt.Errorf("注册成功但连接不可用")
	}

	return &session{client: c, conn: conn, runnerId: r.runnerId, registerMessage: resp.GetMessage()}, nil
}

// heartbeatLoop 定时上报真实运行任务数与节点 CPU/内存使用率；写失败即结束会话，交回连接管理循环
func (r *Runner) heartbeatLoop(ctx context.Context, sess *session) error {
	ticker := time.NewTicker(r.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			cpuPercent, memPercent := collectHostMetrics()
			running := r.runningTasks.Load()

			env := codec.BuildHeartbeatRequest(sess.runnerId, running, cpuPercent, memPercent)
			data, err := codec.MarshalEnvelope(env)
			if err != nil {
				continue
			}
			if err := codec.WriteFrame(sess.conn, data); err != nil {
				return fmt.Errorf("心跳发送失败: %w", err)
			}
		}
	}
}

// readLoop 会话内唯一的读取者：接收任务下发与容器状态/日志查询，读失败即结束会话。
// 注册完成前不会进入此循环，节点不会在未注册状态下对外表现为在线。
func (r *Runner) readLoop(ctx context.Context, sess *session) error {
	for {
		env, err := sess.client.ReadEnvelope()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("读取消息失败: %w", err)
		}
		if ctx.Err() != nil {
			// 退出中：不再开始新的任务/查询处理（未回执由主节点超时机制兜底）
			return nil
		}

		switch env.GetType() {
		case messages.MessageType_TASK_DISPATCH_REQ:
			r.handleTask(sess, env)
		case messages.MessageType_CONTAINER_STATUS_REQ:
			r.handleContainerStatusReq(sess, env)
		case messages.MessageType_CONTAINER_LOGS_REQ:
			r.handleContainerLogsReq(sess, env)
		default:
			// 忽略注册/心跳等响应
		}
	}
}

// disconnect 尽力发送断开消息并关闭连接，让主节点及时把节点标记为离线
func (s *session) disconnect(reason string) {
	env := codec.BuildDisconnectRequest(s.runnerId, reason)
	if data, err := codec.MarshalEnvelope(env); err == nil {
		_ = codec.WriteFrame(s.conn, data)
	}
	// 等待 TCP 发送缓冲区刷新，确保对端读取到 disconnect
	time.Sleep(disconnectFlushWait)
	_ = s.client.Close()
}

// secondsOrDefault 配置秒数转 Duration，未配置（<=0）时取默认值
func secondsOrDefault(seconds int, def time.Duration) time.Duration {
	if seconds <= 0 {
		return def
	}
	return time.Duration(seconds) * time.Second
}

// nextBackoff 指数退避增长，上限为 max
func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next <= 0 || next > max {
		return max
	}
	return next
}

// jitter 在等待时长上加 0~25% 随机抖动，避免多节点同时重连形成请求尖峰
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return d + time.Duration(rand.Int63n(int64(d)/4+1))
}

// sleepCtx 等待 d，期间 ctx 取消则立即返回 false
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
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
