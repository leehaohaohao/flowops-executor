package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leehaohaohao/nexa-protocol/go/messages"
)

func TestSafeJoin(t *testing.T) {
	base := "C:\\volume\\svc"

	cases := []struct {
		key  string
		want string
		ok   bool
	}{
		{"Dockerfile", "C:\\volume\\svc\\Dockerfile", true},
		{"nginx/default.conf", "C:\\volume\\svc\\nginx\\default.conf", true},
		{"../evil", "", false},
		{"a/../../evil", "", false},
		{"/abs/path", "", false},
	}
	for _, c := range cases {
		got, err := safeJoin(base, c.key)
		if c.ok != (err == nil) {
			t.Errorf("safeJoin(%q) err=%v, want ok=%v", c.key, err, c.ok)
			continue
		}
		if c.ok && got != c.want {
			t.Errorf("safeJoin(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}

func TestWriteConfigFiles(t *testing.T) {
	dir := t.TempDir()

	req := &messages.TaskRequest{
		VolumeDir: dir,
		Config: map[string]string{
			"Dockerfile":        "FROM openjdk:17",
			"nginx/default.conf": "server {}",
			"docker-compose.yml": "services:\n  app:\n",
			"service_config":     `{"runtime":"java"}`,
			"port_mappings":      `[{"containerPort":8080}]`,
		},
	}

	if err := writeConfigFiles(req); err != nil {
		t.Fatalf("writeConfigFiles: %v", err)
	}

	checks := []string{"Dockerfile", "nginx/default.conf", "docker-compose.yml"}
	for _, f := range checks {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("配置文件未落盘: %s, err=%v", f, err)
		}
	}

	for _, meta := range []string{"service_config", "port_mappings"} {
		if _, err := os.Stat(filepath.Join(dir, meta)); err == nil {
			t.Errorf("元数据不应写盘: %s", meta)
		}
	}
}
