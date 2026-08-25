package runner

import (
	"archive/tar"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leehaohaohao/nexa-protocol/go/messages"
)

// buildTestTar 构造 tar 字节流，entries 为 name -> content（nil 表示目录）
func buildTestTar(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range entries {
		if content == nil {
			if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
				t.Fatalf("写 tar 目录头失败: %v", err)
			}
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatalf("写 tar 文件头失败: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("写 tar 内容失败: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("关闭 tar writer 失败: %v", err)
	}
	return buf.Bytes()
}

func TestExtractTar(t *testing.T) {
	dir := t.TempDir()

	data := buildTestTar(t, map[string][]byte{
		"dist/":               nil,
		"app.jar":             []byte("fake-jar"),
		"nginx/default.conf":  []byte("server {}"),
		"docker-compose.yml":  []byte("services:\n  app:\n"),
	})
	if err := extractTar(bytes.NewReader(data), dir); err != nil {
		t.Fatalf("extractTar: %v", err)
	}

	checks := map[string]string{
		"app.jar":            "fake-jar",
		"nginx/default.conf": "server {}",
	}
	for name, want := range checks {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("文件未解压: %s, err=%v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s 内容 = %q, want %q", name, string(got), want)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "dist")); err != nil || !info.IsDir() {
		t.Errorf("目录未解压: dist, err=%v", err)
	}
}

// TestExtractTarRejectTraversal 路径穿越条目必须被拒绝
func TestExtractTarRejectTraversal(t *testing.T) {
	evilNames := []string{"../evil", "a/../../evil", "/abs/path", "C:/evil"}
	for _, name := range evilNames {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			data := buildTestTar(t, map[string][]byte{name: []byte("evil")})
			if err := extractTar(bytes.NewReader(data), dir); err == nil {
				t.Fatalf("extractTar(%q) 应拒绝，但未报错", name)
			}
			// 确认没有写出任何文件
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Errorf("extractTar(%q) 残留文件: %d", name, len(entries))
			}
		})
	}
}

// TestExtractTarSkipSymlink 符号链接条目应被跳过，不创建链接
func TestExtractTarSkipSymlink(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "evil-link",
		Typeflag: tar.TypeSymlink,
		Linkname: "/etc/passwd",
		Mode:     0o777,
	}); err != nil {
		t.Fatalf("写 tar 符号链接头失败: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("关闭 tar writer 失败: %v", err)
	}

	if err := extractTar(bytes.NewReader(buf.Bytes()), dir); err != nil {
		t.Fatalf("extractTar: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "evil-link")); err == nil {
		t.Errorf("符号链接不应被创建")
	}
}

func TestDownloadArtifact(t *testing.T) {
	dir := t.TempDir()
	data := buildTestTar(t, map[string][]byte{
		"app.jar": []byte("fake-jar"),
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-tar")
		_, _ = w.Write(data)
	}))
	defer ts.Close()

	if err := downloadArtifact(ts.URL, dir); err != nil {
		t.Fatalf("downloadArtifact: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "app.jar")); err != nil || string(got) != "fake-jar" {
		t.Errorf("产物未正确解压: content=%q err=%v", string(got), err)
	}
}

func TestDownloadArtifactHTTPError(t *testing.T) {
	dir := t.TempDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":403,"message":"产物下载令牌无效"}`, http.StatusForbidden)
	}))
	defer ts.Close()

	err := downloadArtifact(ts.URL, dir)
	if err == nil {
		t.Fatal("downloadArtifact 应报错")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("错误信息应包含 HTTP 状态码: %v", err)
	}
}

// TestDownloadThenConfigOverride 验证步骤 2 顺序语义：先解压 tar，再以 config 消息覆盖
func TestDownloadThenConfigOverride(t *testing.T) {
	dir := t.TempDir()
	tarData := buildTestTar(t, map[string][]byte{
		"docker-compose.yml": []byte("version: '3'\nservices:\n  app:\n    image: old"),
		"app.jar":            []byte("fake-jar"),
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarData)
	}))
	defer ts.Close()

	if err := downloadArtifact(ts.URL, dir); err != nil {
		t.Fatalf("downloadArtifact: %v", err)
	}

	req := &messages.TaskRequest{
		VolumeDir: dir,
		Config: map[string]string{
			"docker-compose.yml": "version: '3'\nservices:\n  app:\n    image: new",
		},
	}
	if err := writeConfigFiles(req); err != nil {
		t.Fatalf("writeConfigFiles: %v", err)
	}

	// config 消息应覆盖 tar 中的同名文件
	compose, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("读取 docker-compose.yml: %v", err)
	}
	if !strings.Contains(string(compose), "image: new") {
		t.Errorf("config 未覆盖 tar 内容: %q", string(compose))
	}
	// tar 中的产物应保留
	if _, err := os.Stat(filepath.Join(dir, "app.jar")); err != nil {
		t.Errorf("tar 中的 app.jar 应保留: %v", err)
	}
}
