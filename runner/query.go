package runner

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/leehaohaohao/nexa-protocol/go/codec"
	"github.com/leehaohaohao/nexa-protocol/go/messages"
)

// defaultLogTail 日志查询未指定 tail 时的默认尾部行数（与主节点本机查询默认一致）
const defaultLogTail = 30

// maxQueryOutputLen 查询回执 output/content 字段最大长度（字节）
const maxQueryOutputLen = 256 * 1024

// handleContainerStatusReq 处理主节点下发的 CONTAINER_STATUS_REQ，回执回填请求 request_id
func (r *Runner) handleContainerStatusReq(env *messages.Envelope) {
	req := &messages.ContainerStatusRequest{}
	if err := codec.UnmarshalMessage(env.GetPayload(), req); err != nil {
		fmt.Printf("[runner] 解析状态查询失败: %v\n", err)
		return
	}
	fmt.Printf("[runner] 收到状态查询: serviceId=%s deployName=%s volumeDir=%s\n",
		req.GetServiceId(), req.GetDeployName(), req.GetVolumeDir())

	resp := handleContainerStatus(req)
	resp.RunnerId = r.runnerId

	r.sendQueryResponse(env.GetRequestId(), codec.BuildContainerStatusResponse(env.GetRequestId(), r.runnerId, resp))
}

// handleContainerLogsReq 处理主节点下发的 CONTAINER_LOGS_REQ，回执回填请求 request_id
func (r *Runner) handleContainerLogsReq(env *messages.Envelope) {
	req := &messages.ContainerLogsRequest{}
	if err := codec.UnmarshalMessage(env.GetPayload(), req); err != nil {
		fmt.Printf("[runner] 解析日志查询失败: %v\n", err)
		return
	}
	fmt.Printf("[runner] 收到日志查询: serviceId=%s deployName=%s volumeDir=%s tail=%d\n",
		req.GetServiceId(), req.GetDeployName(), req.GetVolumeDir(), req.GetTail())

	resp := handleContainerLogs(req)
	resp.RunnerId = r.runnerId

	r.sendQueryResponse(env.GetRequestId(), codec.BuildContainerLogsResponse(env.GetRequestId(), r.runnerId, resp))
}

// sendQueryResponse 发送查询回执；requestId 回填请求的 request_id。
//
// 注意：当前主节点（协议 v0.4.0）的 Java 回执回调签名不透传 request_id，
// QueryManager 实际按 runnerId 关联（同节点同时只允许一个查询，串行无冲突）。
// 回填 request_id 是协议层约定（codec builder 已支持），保留供未来协议透传后
// 按 request_id 关联——届时可支持同节点并发查询与断线重连的准确关联。
func (r *Runner) sendQueryResponse(requestId string, env *messages.Envelope) {
	data, err := codec.MarshalEnvelope(env)
	if err != nil {
		fmt.Printf("[runner] 序列化查询回执失败: %v\n", err)
		return
	}
	if err := codec.WriteFrame(r.client.Conn(), data); err != nil {
		fmt.Printf("[runner] 回传查询回执失败: %v\n", err)
		return
	}
	fmt.Printf("[runner] 查询回执已发送: requestId=%s\n", requestId)
}

// handleContainerStatus 执行 docker compose ps 并解析容器状态。
// 与主节点本机查询语义对齐：ps 用 --format {{json .}} 逐行 JSON，State 字段取最差状态。
func handleContainerStatus(req *messages.ContainerStatusRequest) *messages.ContainerStatusResponse {
	resp := &messages.ContainerStatusResponse{}
	if req.GetVolumeDir() == "" {
		resp.Error = "volume_dir 为空"
		return resp
	}

	output, exitCode, err := runCompose(req.GetVolumeDir(), "ps", "--format", "{{json .}}")
	if err != nil {
		resp.Error = fmt.Sprintf("docker compose ps 执行失败: %v", err)
		return resp
	}
	if exitCode != 0 {
		resp.Error = fmt.Sprintf("docker compose ps 退出码=%d: %s", exitCode, limitStr(output, maxQueryOutputLen))
		return resp
	}

	running, status := parseContainerStatus(output)
	resp.Running = running
	resp.Status = status
	resp.Output = limitStr(output, maxQueryOutputLen)
	return resp
}

// parseContainerStatus 解析 docker compose ps --format {{json .}} 的输出。
// 每个容器一行 JSON，取 State 字段；多个容器取最差状态（exited/dead > restarting > 其他 > running）。
// 无任何容器（输出为空）视为 stopped。
func parseContainerStatus(output string) (running bool, status string) {
	worst := "running"
	found := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var info struct {
			State string `json:"State"`
		}
		if err := json.Unmarshal([]byte(line), &info); err != nil {
			// 忽略非 JSON 行（老版本 docker-compose 文本表头等）
			continue
		}
		state := strings.ToLower(strings.TrimSpace(info.State))
		if state == "" {
			continue
		}
		found = true
		if stateRank(state) > stateRank(worst) {
			worst = state
		}
	}
	if !found {
		return false, "stopped"
	}
	return worst == "running", worst
}

// stateRank 状态严重度：exited/dead 最差，restarting 次之，其余非运行态再次，running 最好
func stateRank(s string) int {
	switch s {
	case "exited", "dead":
		return 3
	case "restarting":
		return 2
	case "running":
		return 0
	default:
		// paused / created / removing / stopped 等均视为未在运行
		return 1
	}
}

// handleContainerLogs 执行 docker compose logs（非流式 tail），与主节点本机日志命令对齐：
// --no-color --tail N [--timestamps] [--since] [--until]
func handleContainerLogs(req *messages.ContainerLogsRequest) *messages.ContainerLogsResponse {
	resp := &messages.ContainerLogsResponse{}
	if req.GetVolumeDir() == "" {
		resp.Error = "volume_dir 为空"
		return resp
	}

	tail := req.GetTail()
	if tail <= 0 {
		tail = defaultLogTail
	}

	args := []string{"logs", "--no-color", "--tail", strconv.FormatInt(int64(tail), 10)}
	if req.GetTimestamps() {
		args = append(args, "--timestamps")
	}
	if since := req.GetSince(); since != "" {
		args = append(args, "--since", since)
	}
	if until := req.GetUntil(); until != "" {
		args = append(args, "--until", until)
	}

	output, exitCode, err := runCompose(req.GetVolumeDir(), args...)
	if err != nil {
		resp.Error = fmt.Sprintf("docker compose logs 执行失败: %v", err)
		return resp
	}
	if exitCode != 0 {
		resp.Error = fmt.Sprintf("docker compose logs 退出码=%d: %s", exitCode, limitStr(output, maxQueryOutputLen))
		return resp
	}
	resp.Content = limitStr(output, maxQueryOutputLen)
	return resp
}

// limitStr 截断字符串到最大字节数
func limitStr(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
