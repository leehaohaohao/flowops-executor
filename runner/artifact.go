package runner

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leehaohaohao/nexa-protocol/go/codec"
	"github.com/leehaohaohao/nexa-protocol/go/messages"
)

// artifactTransferTimeout 单次产物传输总超时（含等待首块），主节点 1MB/块流式发送
const artifactTransferTimeout = 5 * time.Minute

// maxArtifactSize 产物内容最大字节数（防异常超大产物耗尽内存，1GB 保守上限）
const maxArtifactSize = 1 << 30

// fetchArtifact 通过自定义协议向主节点拉取单个产物：
// 发 ARTIFACT_REQ → 读 ARTIFACT_DATA 分块重组 → sha256 校验 → 按类型落盘 → 回 ARTIFACT_ACK。
// 主节点传输失败时回 ACK(ok=false)，此处同步返回错误。
// 连接来自本次会话（sess），断线重连后会使用新会话的连接。
func (r *Runner) fetchArtifact(sess *session, serviceId, typ, volumeDir string) error {
	req := &messages.ArtifactRequest{
		ServiceId: serviceId,
		Type:      typ,
		Version:   0, // 0 = 最新
	}
	reqEnv := codec.BuildArtifactRequest(sess.runnerId, req)
	data, err := codec.MarshalEnvelope(reqEnv)
	if err != nil {
		return fmt.Errorf("序列化产物请求失败: %w", err)
	}
	if err := codec.WriteFrame(sess.conn, data); err != nil {
		return fmt.Errorf("发送产物请求失败: %w", err)
	}
	requestId := reqEnv.GetRequestId()
	fmt.Printf("[runner] 产物请求已发送: serviceId=%s type=%s requestId=%s\n", serviceId, typ, requestId)

	content, checksum, err := r.receiveArtifact(sess, requestId)
	if err != nil {
		r.sendArtifactAck(sess, requestId, false, err.Error())
		return err
	}
	if err := verifyChecksum(content, checksum); err != nil {
		r.sendArtifactAck(sess, requestId, false, err.Error())
		return err
	}
	if err := saveArtifact(typ, content, volumeDir); err != nil {
		r.sendArtifactAck(sess, requestId, false, err.Error())
		return err
	}

	r.sendArtifactAck(sess, requestId, true, "")
	fmt.Printf("[runner] 产物拉取完成: type=%s size=%d\n", typ, len(content))
	return nil
}

// receiveArtifact 读取产物分块直到收齐，返回 (重组内容, 末块 checksum)。
// 期间到达的非产物消息记录并跳过（当前架构任务串行处理，传输窗口内不会并发处理其他请求）。
func (r *Runner) receiveArtifact(sess *session, requestId string) ([]byte, string, error) {
	conn := sess.conn
	if err := conn.SetReadDeadline(time.Now().Add(artifactTransferTimeout)); err != nil {
		return nil, "", fmt.Errorf("设置读取超时失败: %w", err)
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }() // 恢复阻塞读，供任务循环复用

	var buf bytes.Buffer
	var checksum string
	expectedChunks := int32(-1)
	nextSeq := int32(0)

	for {
		if buf.Len() > maxArtifactSize {
			return nil, "", fmt.Errorf("产物超过大小上限: %d", maxArtifactSize)
		}
		env, err := sess.client.ReadEnvelope()
		if err != nil {
			return nil, "", fmt.Errorf("读取产物分块失败: %w", err)
		}

		switch env.GetType() {
		case messages.MessageType_ARTIFACT_DATA:
			chunk := &messages.ArtifactChunk{}
			if err := codec.UnmarshalMessage(env.GetPayload(), chunk); err != nil {
				return nil, "", fmt.Errorf("解析产物分块失败: %w", err)
			}
			if chunk.GetTransferId() != requestId {
				return nil, "", fmt.Errorf("产物传输 id 不匹配: got=%s want=%s", chunk.GetTransferId(), requestId)
			}
			if expectedChunks == -1 {
				expectedChunks = chunk.GetTotalChunks()
				if expectedChunks <= 0 {
					return nil, "", fmt.Errorf("产物分块总数非法: %d", expectedChunks)
				}
			}
			if chunk.GetTotalChunks() != expectedChunks {
				return nil, "", fmt.Errorf("产物分块总数不一致: got=%d want=%d", chunk.GetTotalChunks(), expectedChunks)
			}
			if chunk.GetSequence() != nextSeq {
				return nil, "", fmt.Errorf("产物分块序号不连续: got=%d want=%d", chunk.GetSequence(), nextSeq)
			}
			if _, err := buf.Write(chunk.GetData()); err != nil {
				return nil, "", fmt.Errorf("缓存产物分块失败: %w", err)
			}
			if chunk.GetChecksum() != "" {
				checksum = chunk.GetChecksum() // 末块携带整体 sha256
			}
			nextSeq++
			if nextSeq == expectedChunks {
				return buf.Bytes(), checksum, nil
			}

		case messages.MessageType_ARTIFACT_ACK:
			ack := &messages.ArtifactAck{}
			if err := codec.UnmarshalMessage(env.GetPayload(), ack); err != nil {
				return nil, "", fmt.Errorf("解析产物回执失败: %w", err)
			}
			if !ack.GetOk() {
				return nil, "", fmt.Errorf("主节点传输失败: %s", ack.GetError())
			}
			return nil, "", fmt.Errorf("收到意外的成功 ACK（主节点成功时不回 ACK）")

		default:
			// 传输窗口内到达的其他消息（任务下发/查询等）：记录并跳过，不中断产物传输
			fmt.Printf("[runner] 产物传输期间忽略消息: type=%v\n", env.GetType())
		}
	}
}

// sendArtifactAck 向主节点回传产物传输确认（成功/失败）
func (r *Runner) sendArtifactAck(sess *session, requestId string, ok bool, errMsg string) {
	ack := &messages.ArtifactAck{TransferId: requestId, Ok: ok, Error: errMsg}
	env := codec.BuildArtifactAck(requestId, sess.runnerId, ack)
	data, err := codec.MarshalEnvelope(env)
	if err != nil {
		fmt.Printf("[runner] 序列化产物 ACK 失败: %v\n", err)
		return
	}
	if err := codec.WriteFrame(sess.conn, data); err != nil {
		fmt.Printf("[runner] 发送产物 ACK 失败: %v\n", err)
	}
}

// fetchArtifacts 按服务类型拉取所需产物：
//   - backend：JAR 优先，失败回退 BINARY（二选一）
//   - frontend：DIST
//   - fullstack：后端产物（JAR/BINARY 二选一）+ DIST
func (r *Runner) fetchArtifacts(sess *session, serviceId, serviceType, volumeDir string) error {
	types := neededArtifactTypes(serviceType)
	if len(types) == 0 {
		return nil
	}

	hasJarOrBinary := false
	var jarBinaryErr error
	hasDist := false
	var distErr error

	for _, t := range types {
		err := r.fetchArtifact(sess, serviceId, t, volumeDir)
		switch t {
		case "JAR", "BINARY":
			jarBinaryErr = err
			if err == nil {
				hasJarOrBinary = true
			}
		case "DIST":
			distErr = err
			hasDist = true
		}
		// 后端产物二选一已成功且无需 DIST（backend），提前结束
		if (t == "JAR" || t == "BINARY") && hasJarOrBinary && !hasDist {
			break
		}
	}

	if (types[0] == "JAR" || types[0] == "BINARY") && !hasJarOrBinary {
		return fmt.Errorf("后端产物拉取失败（JAR/BINARY 均不可用）: %v", jarBinaryErr)
	}
	if hasDist && distErr != nil {
		return fmt.Errorf("前端产物拉取失败: %v", distErr)
	}
	return nil
}

// neededArtifactTypes 按服务类型返回需拉取的产物类型（按尝试顺序）
func neededArtifactTypes(serviceType string) []string {
	switch serviceType {
	case "backend":
		return []string{"JAR", "BINARY"}
	case "frontend":
		return []string{"DIST"}
	case "fullstack":
		return []string{"JAR", "BINARY", "DIST"}
	default:
		return nil
	}
}

// verifyChecksum 校验重组内容与末块携带的整体 sha256 是否一致
func verifyChecksum(data []byte, checksum string) error {
	if checksum == "" {
		return fmt.Errorf("产物缺少 checksum")
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, checksum) {
		return fmt.Errorf("产物 sha256 校验失败: got=%s want=%s", got, checksum)
	}
	return nil
}

// saveArtifact 按类型落盘：JAR→app.jar、BINARY→app、DIST→解压 tar 到 dist/
func saveArtifact(typ string, data []byte, volumeDir string) error {
	switch strings.ToUpper(typ) {
	case "JAR":
		return writeArtifactFile(filepath.Join(volumeDir, "app.jar"), data)
	case "BINARY":
		return writeArtifactFile(filepath.Join(volumeDir, "app"), data)
	case "DIST":
		distDir := filepath.Join(volumeDir, "dist")
		// 先清空旧目录，避免上次解压残留
		if err := os.RemoveAll(distDir); err != nil {
			return fmt.Errorf("清理旧 dist 目录失败: %w", err)
		}
		if err := os.MkdirAll(distDir, 0o755); err != nil {
			return fmt.Errorf("创建 dist 目录失败: %w", err)
		}
		return extractTar(bytes.NewReader(data), distDir)
	default:
		return fmt.Errorf("不支持的产物类型: %s", typ)
	}
}

func writeArtifactFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建产物目录失败 (%s): %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入产物失败 (%s): %w", path, err)
	}
	return nil
}

// extractTar 将 tar 流安全解压到 dst：
//   - 复用 safeJoin 拦截路径穿越（..、绝对路径、盘符前缀）
//   - 跳过符号链接/硬链接/特殊文件，避免解压出指向外部的链接
//   - 目录条目带尾部斜杠，由 safeJoin 内部的 filepath.Clean 归一化
func extractTar(r io.Reader, dst string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("读取 tar 条目失败: %w", err)
		}

		target, err := safeJoin(dst, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("创建目录失败 (%s): %w", hdr.Name, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("创建目录失败 (%s): %w", hdr.Name, err)
			}
			mode := os.FileMode(hdr.Mode & 0o777)
			if mode == 0 {
				mode = 0o644
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return fmt.Errorf("创建文件失败 (%s): %w", hdr.Name, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return fmt.Errorf("写入文件失败 (%s): %w", hdr.Name, err)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("关闭文件失败 (%s): %w", hdr.Name, err)
			}
		default:
			// 跳过符号链接/硬链接/字符设备/块设备/FIFO 等特殊条目
			fmt.Printf("[runner] 跳过 tar 特殊条目: %s (type=%c)\n", hdr.Name, hdr.Typeflag)
		}
	}
}
