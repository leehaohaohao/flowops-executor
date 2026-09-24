package runner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/leehaohaohao/nexa-protocol/go/codec"
	"github.com/leehaohaohao/nexa-protocol/go/messages"
)

// configMetadataKeys 元数据配置名：value 为 JSON 或标记值，仅主节点使用，不写盘
var configMetadataKeys = map[string]bool{
	"service_config": true,
	"port_mappings":  true,
	"service_type":   true, // 服务类型标记（backend/frontend/fullstack），子节点据此决定拉取产物
}

// maxOutputLen 回执 output 字段最大长度（字节）
const maxOutputLen = 8192

// handleTask 处理单个任务：解包 → 拉取产物 → 配置落盘 → 执行 docker compose → 回传结果。
// 断线时任务不自动重放：结果无法回传时仅记录日志，由主节点的掉线/超时机制兜底。
func (r *Runner) handleTask(sess *session, env *messages.Envelope) {
	req := &messages.TaskRequest{}
	if err := codec.UnmarshalMessage(env.GetPayload(), req); err != nil {
		fmt.Printf("[runner] 解析任务请求失败: %v\n", err)
		return
	}

	fmt.Printf("[runner] 收到任务: taskId=%s serviceId=%s deployName=%s action=%s volumeDir=%s\n",
		req.GetTaskId(), req.GetServiceId(), req.GetDeployName(), req.GetAction(), req.GetVolumeDir())

	r.runningTasks.Add(1)
	result := r.executeTask(sess, req)
	r.runningTasks.Add(-1)

	fmt.Printf("[runner] 任务执行完成: taskId=%s success=%v exitCode=%d\n",
		result.taskId, result.success, result.exitCode)

	respEnv := codec.BuildTaskDispatchResponse(sess.runnerId, result.taskId, result.success, result.exitCode, result.output, result.errMsg)
	data, err := codec.MarshalEnvelope(respEnv)
	if err != nil {
		fmt.Printf("[runner] 序列化任务回执失败: %v\n", err)
		return
	}
	if err := codec.WriteFrame(sess.conn, data); err != nil {
		fmt.Printf("[runner] 回传任务回执失败（连接可能已断开，任务不重放）: %v\n", err)
	}
}

type taskResult struct {
	taskId   string
	success  bool
	exitCode int32
	output   string
	errMsg   string
}

func (r *Runner) executeTask(sess *session, req *messages.TaskRequest) taskResult {
	res := taskResult{taskId: req.GetTaskId()}

	// START 动作需要构建：先经协议从主节点拉取产物（JAR/BINARY/DIST）落盘，
	// 再以 config 消息为准覆盖写配置（产物与配置分离，config 更可靠）
	if strings.EqualFold(req.GetAction(), "START") {
		if err := r.fetchArtifacts(sess, req.GetServiceId(), req.GetConfig()["service_type"], req.GetVolumeDir()); err != nil {
			res.errMsg = "产物拉取失败: " + err.Error()
			return res
		}
		fmt.Printf("[runner] 产物就绪: %s\n", req.GetVolumeDir())
	}

	if err := writeConfigFiles(req); err != nil {
		res.errMsg = "写配置落盘失败: " + err.Error()
		return res
	}

	output, exitCode, err := runDockerCompose(req)
	res.output = output
	res.exitCode = exitCode
	if err != nil {
		if exitCode > 0 {
			// docker 已执行但失败，详情在 output 里
			res.errMsg = fmt.Sprintf("docker compose 执行失败 (exit=%d)", exitCode)
		} else {
			res.errMsg = fmt.Sprintf("docker compose 执行失败: %v", err)
		}
		return res
	}
	res.success = true
	return res
}

// writeConfigFiles 将 config map 中的文件按 key（相对路径）写入 volume_dir
func writeConfigFiles(req *messages.TaskRequest) error {
	base := req.GetVolumeDir()
	if base == "" {
		return fmt.Errorf("volume_dir 为空")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return fmt.Errorf("创建 volume_dir 失败: %w", err)
	}

	for key, val := range req.GetConfig() {
		if key == "" {
			continue
		}
		if configMetadataKeys[key] {
			fmt.Printf("[runner] 跳过元数据配置: %s\n", key)
			continue
		}

		path, err := safeJoin(base, key)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("创建配置目录失败 (%s): %w", key, err)
		}
		if err := os.WriteFile(path, []byte(val), 0o644); err != nil {
			return fmt.Errorf("写入配置文件失败 (%s): %w", key, err)
		}
		fmt.Printf("[runner] 配置落盘: %s\n", path)
	}
	return nil
}

// safeJoin 将相对路径 key 安全拼接到 base 下，防止路径穿越
func safeJoin(base, key string) (string, error) {
	rel := filepath.Clean(filepath.FromSlash(key))
	// Windows 下 filepath.IsAbs 对无盘符的根路径（\foo）返回 false，需单独拦截卷分隔符前缀
	if rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, string(filepath.Separator)) {
		return "", fmt.Errorf("非法配置文件路径: %s", key)
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == ".." {
			return "", fmt.Errorf("非法配置文件路径: %s", key)
		}
	}
	return filepath.Join(base, rel), nil
}

// runDockerCompose 按 action 执行 docker compose，返回 (输出, exitCode, err)
func runDockerCompose(req *messages.TaskRequest) (string, int32, error) {
	composeFile := filepath.Join(req.GetVolumeDir(), "docker-compose.yml")

	args := make([]string, 0, 6)
	args = append(args, "-f", composeFile)

	switch strings.ToUpper(req.GetAction()) {
	case "START":
		args = append(args, "up", "-d", "--build")
	case "STOP":
		args = append(args, "stop")
	case "RESTART":
		args = append(args, "restart")
	case "REMOVE":
		args = append(args, "down")
	default:
		return "", -1, fmt.Errorf("不支持的 action: %s", req.GetAction())
	}

	return runCompose(req.GetVolumeDir(), args...)
}

// runCompose 在 volumeDir 下执行 docker compose（docker 优先，回退 docker-compose），
// 返回 (合并输出, exitCode, err)。供任务执行与状态/日志查询共用。
func runCompose(volumeDir string, args ...string) (string, int32, error) {
	base, isSub, err := composeBaseCmd()
	if err != nil {
		return "", -1, err
	}

	fullArgs := make([]string, 0, len(args)+1)
	if isSub {
		fullArgs = append(fullArgs, "compose")
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.Command(base, fullArgs...)
	cmd.Dir = volumeDir

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	runErr := cmd.Run()
	exitCode := int32(0)
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = int32(ee.ExitCode())
		} else {
			exitCode = -1
		}
	}

	output := buf.String()
	if len(output) > maxOutputLen {
		output = output[:maxOutputLen]
	}
	return output, exitCode, runErr
}

// composeBaseCmd 返回 docker compose 命令；优先 docker compose 子命令，回退 docker-compose
func composeBaseCmd() (name string, isSub bool, err error) {
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker", true, nil
	}
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return "docker-compose", false, nil
	}
	return "", false, fmt.Errorf("未找到 docker / docker-compose 命令")
}
