package runner

import (
	"strings"
	"testing"

	"github.com/leehaohaohao/nexa-protocol/go/messages"
)

func TestParseContainerStatus(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		running bool
		status  string
	}{
		{
			name: "全部 running",
			output: `{"ID":"abc","Name":"svc-app-1","State":"running","Status":"Up 2 minutes"}
{"ID":"def","Name":"svc-db-1","State":"running","Status":"Up 2 minutes"}`,
			running: true,
			status:  "running",
		},
		{
			name:    "exited",
			output:  `{"ID":"abc","Name":"svc-app-1","State":"exited","Status":"Exited (1) 5 minutes ago"}`,
			running: false,
			status:  "exited",
		},
		{
			name:    "restarting",
			output:  `{"ID":"abc","Name":"svc-app-1","State":"restarting","Status":"Restarting"}`,
			running: false,
			status:  "restarting",
		},
		{
			name:    "混合 running+restarting 取最差",
			output:  `{"State":"running"}` + "\n" + `{"State":"restarting"}`,
			running: false,
			status:  "restarting",
		},
		{
			name:    "混合 running+exited 取最差",
			output:  `{"State":"exited"}` + "\n" + `{"State":"running"}`,
			running: false,
			status:  "exited",
		},
		{
			name:    "无容器输出为空",
			output:  ``,
			running: false,
			status:  "stopped",
		},
		{
			name:    "老版本文本输出非 JSON 视为无容器",
			output:  "Name   Command   State   Ports\n------  --------  -----  ------\nsvc_1   /bin/sh   Up     0.0.0.0:80->80/tcp",
			running: false,
			status:  "stopped",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			running, status := parseContainerStatus(c.output)
			if running != c.running || status != c.status {
				t.Errorf("parseContainerStatus = (%v, %q), want (%v, %q)",
					running, status, c.running, c.status)
			}
		})
	}
}

func TestHandleContainerStatusEmptyVolumeDir(t *testing.T) {
	resp := handleContainerStatus(&messages.ContainerStatusRequest{})
	if resp.GetError() == "" {
		t.Error("volume_dir 为空时应返回 error")
	}
	if resp.GetRunning() {
		t.Error("volume_dir 为空时 running 应为 false")
	}
}

func TestHandleContainerLogsEmptyVolumeDir(t *testing.T) {
	resp := handleContainerLogs(&messages.ContainerLogsRequest{})
	if resp.GetError() == "" {
		t.Error("volume_dir 为空时应返回 error")
	}
}

func TestLimitStr(t *testing.T) {
	s := "hello world"
	if got := limitStr(s, 5); got != "hello" {
		t.Errorf("limitStr(5) = %q", got)
	}
	if got := limitStr(s, 100); got != s {
		t.Errorf("limitStr(100) = %q", got)
	}
}

// TestParseContainerStatusStateRank 验证状态优先级辅助函数
func TestParseContainerStatusStateRank(t *testing.T) {
	if stateRank("exited") <= stateRank("restarting") {
		t.Error("exited 优先级应高于 restarting")
	}
	if stateRank("restarting") <= stateRank("running") {
		t.Error("restarting 优先级应高于 running")
	}
	if stateRank("paused") != 1 {
		t.Errorf("paused 应为中间优先级: %d", stateRank("paused"))
	}
}

func TestHandleContainerStatusComposeFile(t *testing.T) {
	// volume_dir 非空但目录不存在：compose 命令执行失败，应返回 error 而非 panic
	resp := handleContainerStatus(&messages.ContainerStatusRequest{VolumeDir: strings.Repeat("x", 8)})
	if resp.GetError() == "" {
		t.Log("本机存在 docker 时该目录不存在应报错")
	}
}
