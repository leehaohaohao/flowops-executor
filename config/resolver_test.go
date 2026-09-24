package config

import (
	"strings"
	"testing"
)

// cfgOf 构造测试用配置
func cfgOf(id, master, token, version string) *Config {
	c := &Config{}
	c.Runner.Id = id
	c.Runner.MasterAddr = master
	c.Runner.Token = token
	c.Runner.Version = version
	return c
}

// T1–T5：核心字段的逐字段优先级 CLI > ENV > External > Embedded
func TestResolveCoreFieldPrecedence(t *testing.T) {
	embedded := cfgOf("runner-embedded", "127.0.0.1:9090", "token-embedded", "1.0.0")

	t.Run("T1 仅内嵌配置", func(t *testing.T) {
		final, src := Resolve(&CLIOptions{}, EnvValues{}, embedded, nil)

		if final.Runner.Id != "runner-embedded" || src.RunnerID != SourceEmbedded {
			t.Errorf("runner.id = %q (%s), want runner-embedded (embedded)", final.Runner.Id, src.RunnerID)
		}
		if final.Runner.MasterAddr != "127.0.0.1:9090" || src.MasterAddr != SourceEmbedded {
			t.Errorf("master_addr = %q (%s), want 127.0.0.1:9090 (embedded)", final.Runner.MasterAddr, src.MasterAddr)
		}
		if final.Runner.Token != "token-embedded" || src.Token != SourceEmbedded {
			t.Errorf("token 来源 = %s, want embedded", src.Token)
		}
		if final.Runner.Version != "1.0.0" || src.RunnerVersion != SourceEmbedded {
			t.Errorf("version 来源 = %s, want embedded", src.RunnerVersion)
		}
	})

	t.Run("T2 外部覆盖内嵌（其余字段 fallback）", func(t *testing.T) {
		external := &Config{}
		external.Runner.MasterAddr = "10.0.0.1:9090"

		final, src := Resolve(&CLIOptions{}, EnvValues{}, embedded, external)

		if final.Runner.MasterAddr != "10.0.0.1:9090" || src.MasterAddr != SourceExternal {
			t.Errorf("master_addr = %q (%s), want 10.0.0.1:9090 (external)", final.Runner.MasterAddr, src.MasterAddr)
		}
		if final.Runner.Id != "runner-embedded" || src.RunnerID != SourceEmbedded {
			t.Errorf("未提供的 id 应 fallback 到内嵌，实际 %q (%s)", final.Runner.Id, src.RunnerID)
		}
		if final.Runner.Token != "token-embedded" {
			t.Errorf("未提供的 token 应 fallback 到内嵌，实际 %q", final.Runner.Token)
		}
	})

	t.Run("T3 ENV 覆盖外部", func(t *testing.T) {
		external := &Config{}
		external.Runner.MasterAddr = "10.0.0.1:9090"
		env := EnvValues{MasterAddr: "10.0.0.2:9090"}

		final, src := Resolve(&CLIOptions{}, env, embedded, external)

		if final.Runner.MasterAddr != "10.0.0.2:9090" || src.MasterAddr != SourceEnv {
			t.Errorf("master_addr = %q (%s), want 10.0.0.2:9090 (env)", final.Runner.MasterAddr, src.MasterAddr)
		}
	})

	t.Run("T4 CLI 覆盖 ENV", func(t *testing.T) {
		env := EnvValues{MasterAddr: "10.0.0.2:9090"}
		cli := &CLIOptions{MasterAddr: "10.0.0.3:9090"}

		final, src := Resolve(cli, env, embedded, nil)

		if final.Runner.MasterAddr != "10.0.0.3:9090" || src.MasterAddr != SourceCLI {
			t.Errorf("master_addr = %q (%s), want 10.0.0.3:9090 (cli)", final.Runner.MasterAddr, src.MasterAddr)
		}
	})

	t.Run("T5 四级来源同时存在", func(t *testing.T) {
		// 对应计划中的混合示例：
		//   id     → ENV
		//   master → CLI
		//   token  → ENV
		//   version→ Embedded
		emb := cfgOf("runner-default", "127.0.0.1:9090", "", "1.0.0")
		ext := &Config{}
		ext.Runner.Id = "runner-file"
		ext.Runner.MasterAddr = "10.0.0.1:9090"
		env := EnvValues{RunnerID: "runner-env", Token: "secret-123"}
		cli := &CLIOptions{MasterAddr: "10.0.0.99:9090"}

		final, src := Resolve(cli, env, emb, ext)

		checks := []struct {
			name    string
			got     string
			want    string
			wantSrc ValueSource
		}{
			{"runner.id", final.Runner.Id, "runner-env", SourceEnv},
			{"runner.master_addr", final.Runner.MasterAddr, "10.0.0.99:9090", SourceCLI},
			{"runner.token", final.Runner.Token, "secret-123", SourceEnv},
			{"runner.version", final.Runner.Version, "1.0.0", SourceEmbedded},
		}
		gotSources := map[string]ValueSource{
			"runner.id":          src.RunnerID,
			"runner.master_addr": src.MasterAddr,
			"runner.token":       src.Token,
			"runner.version":     src.RunnerVersion,
		}
		for _, c := range checks {
			if c.got != c.want {
				t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
			}
			if gotSources[c.name] != c.wantSrc {
				t.Errorf("%s 来源 = %s, want %s", c.name, gotSources[c.name], c.wantSrc)
			}
		}
	})
}

// 非核心字段：外部 > 内嵌（当前不支持 CLI/ENV）
func TestResolveNonCoreFieldsExternalOverEmbedded(t *testing.T) {
	embedded := &Config{}
	embedded.Runner.HeartbeatInterval = 10
	embedded.Runner.ConnectTimeout = 5
	embedded.Runner.AuthRetryInterval = 60
	embedded.Log.Level = "info"
	embedded.Log.MaxSize = 100

	external := &Config{}
	external.Runner.HeartbeatInterval = 20 // 仅覆盖心跳
	external.Log.Level = "debug"

	final, _ := Resolve(&CLIOptions{}, EnvValues{}, embedded, external)

	if final.Runner.HeartbeatInterval != 20 {
		t.Errorf("heartbeat_interval = %d, want 20（外部覆盖）", final.Runner.HeartbeatInterval)
	}
	if final.Runner.ConnectTimeout != 5 {
		t.Errorf("connect_timeout = %d, want 5（外部未提供，fallback 内嵌）", final.Runner.ConnectTimeout)
	}
	if final.Runner.AuthRetryInterval != 60 {
		t.Errorf("auth_retry_interval = %d, want 60（fallback 内嵌）", final.Runner.AuthRetryInterval)
	}
	if final.Log.Level != "debug" {
		t.Errorf("log.level = %q, want debug", final.Log.Level)
	}
	if final.Log.MaxSize != 100 {
		t.Errorf("log.max_size = %d, want 100（fallback 内嵌）", final.Log.MaxSize)
	}
}

// 空白值视为「未提供」，继续 fallback
func TestResolveTreatsBlankAsUnset(t *testing.T) {
	embedded := cfgOf("runner-embedded", "127.0.0.1:9090", "token-embedded", "1.0.0")
	cli := &CLIOptions{RunnerID: "   ", MasterAddr: ""}
	env := EnvValues{RunnerID: "\t", Token: "  "}

	final, src := Resolve(cli, env, embedded, nil)

	if final.Runner.Id != "runner-embedded" || src.RunnerID != SourceEmbedded {
		t.Errorf("空白 CLI/ENV 值应视为未提供，实际 id=%q 来源=%s", final.Runner.Id, src.RunnerID)
	}
	if final.Runner.Token != "token-embedded" {
		t.Errorf("空白 token 应视为未提供，实际 %q", final.Runner.Token)
	}
}

// T13 + master_addr 格式校验（统一在全部来源解析之后执行）
func TestValidate(t *testing.T) {
	t.Run("T13 所有来源 token 为空 → 失败", func(t *testing.T) {
		cfg := cfgOf("runner-1", "10.0.0.1:9090", "", "1.0.0")
		err := Validate(cfg)
		if err == nil {
			t.Fatal("token 为空时应校验失败")
		}
		if !strings.Contains(err.Error(), "runner.token") {
			t.Errorf("错误信息应指出缺少 runner.token，实际: %v", err)
		}
	})

	t.Run("核心字段多个为空 → 全部列出", func(t *testing.T) {
		cfg := &Config{}
		err := Validate(cfg)
		if err == nil {
			t.Fatal("应校验失败")
		}
		for _, field := range []string{"runner.id", "runner.master_addr", "runner.token", "runner.version"} {
			if !strings.Contains(err.Error(), field) {
				t.Errorf("错误信息应包含 %s，实际: %v", field, err)
			}
		}
	})

	t.Run("master_addr 格式非法", func(t *testing.T) {
		cases := []string{"10.0.0.1", "10.0.0.1:", ":9090", "10.0.0.1:0", "10.0.0.1:70000", "10.0.0.1:abc"}
		for _, addr := range cases {
			cfg := cfgOf("runner-1", addr, "token", "1.0.0")
			if err := Validate(cfg); err == nil {
				t.Errorf("master_addr=%q 应校验失败", addr)
			}
		}
	})

	t.Run("合法配置通过", func(t *testing.T) {
		for _, addr := range []string{"10.0.0.1:9090", "127.0.0.1:9090", "master.example.com:9090", "[::1]:9090"} {
			cfg := cfgOf("runner-1", addr, "token", "1.0.0")
			if err := Validate(cfg); err != nil {
				t.Errorf("master_addr=%q 应校验通过: %v", addr, err)
			}
		}
	})
}

// T16：日志摘要不得包含 token 明文
func TestLogSummaryHidesToken(t *testing.T) {
	const secret = "super-secret-token-value"

	cfg := cfgOf("runner-1", "10.0.0.1:9090", secret, "1.0.0")
	res := &LoadResult{
		Config:     cfg,
		Sources:    &ConfigSources{RunnerID: SourceCLI, MasterAddr: SourceEnv, Token: SourceEnv, RunnerVersion: SourceEmbedded, AppEnv: SourceDefault},
		AppEnv:     "prod",
		ConfigPath: "/etc/flowops/executor.yaml",
	}

	lines := res.LogSummary()
	if len(lines) == 0 {
		t.Fatal("日志摘要不应为空")
	}
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, secret) {
		t.Fatalf("日志摘要泄露了 token 明文:\n%s", joined)
	}
	if !strings.Contains(joined, "configured (env)") {
		t.Errorf("应输出 token 状态与来源，实际:\n%s", joined)
	}
	// 其他字段与来源应正常输出
	for _, want := range []string{"APP_ENV: prod", "runner-1 (cli)", "10.0.0.1:9090 (env)", "1.0.0 (embedded)", "/etc/flowops/executor.yaml"} {
		if !strings.Contains(joined, want) {
			t.Errorf("日志摘要缺少 %q，实际:\n%s", want, joined)
		}
	}
}

// token 缺失时摘要显示 missing
func TestLogSummaryTokenMissing(t *testing.T) {
	res := &LoadResult{
		Config:  cfgOf("r", "10.0.0.1:9090", "", "1.0.0"),
		Sources: &ConfigSources{},
		AppEnv:  "prod",
	}
	joined := strings.Join(res.LogSummary(), "\n")
	if !strings.Contains(joined, "Runner Token: missing") {
		t.Errorf("token 缺失应显示 missing，实际:\n%s", joined)
	}
}
