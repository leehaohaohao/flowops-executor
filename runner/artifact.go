package runner

import (
	"archive/tar"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// artifactDownloadTimeout 产物下载超时：产物可能较大（jar/dist），放宽到 10 分钟
const artifactDownloadTimeout = 10 * time.Minute

// maxDownloadErrorBody 下载失败时读取错误响应体的最大字节数
const maxDownloadErrorBody = 512

// downloadArtifact 从主节点拉取产物 tar（整 volumeDir 打包）并解压到 volumeDir。
// artifact_url 已由主节点拼接（含共享密钥 token 查询参数），直接 GET 即可。
func downloadArtifact(url, volumeDir string) error {
	client := &http.Client{Timeout: artifactDownloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("请求产物下载端点失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxDownloadErrorBody))
		return fmt.Errorf("产物下载失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := extractTar(resp.Body, volumeDir); err != nil {
		return fmt.Errorf("产物解压失败: %w", err)
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
