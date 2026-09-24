package config

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearEnvVars 清空所有相关环境变量（t.Setenv 在测试结束后自动恢复）
func clearEnvVars(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvAppEnv, EnvConfig, EnvRunnerID, EnvMasterAddr, EnvRunnerToken, EnvRunnerVersion} {
		t.Setenv(k, "")
	}
}

// stubExecutableDir 替换「可执行文件目录」（默认外部配置路径的定位基准）
func stubExecutableDir(t *testing.T, dir string) {
	t.Helper()
	old := executableDir
	executableDir = func() string { return dir }
	t.Cleanup(func() { executableDir = old })
}

func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
}

const validExternalYAML = `runner:
  id: runner-file
  master_addr: 10.0.0.1:9090
  version: 2.0.0
`

// ---------------------------------------------------------------- CLI 解析

func TestParseCLI(t *testing.T) {
	cli, err := ParseCLI([]string{
		"--env", "dev",
		"--config", "/tmp/executor.yaml",
		"--runner-id", "runner-a",
		"--master-addr", "10.0.0.1:9090",
		"--token", "secret-token",
		"--runner-version", "1.2.3",
	}, io.Discard)
	if err != nil {
		t.Fatalf("ParseCLI: %v", err)
	}

	checks := map[string]string{
		"env":            cli.Env,
		"config":         cli.ConfigPath,
		"runner-id":      cli.RunnerID,
		"master-addr":    cli.MasterAddr,
		"token":          cli.Token,
		"runner-version": cli.RunnerVersion,
	}
	want := map[string]string{
		"env":            "dev",
		"config":         "/tmp/executor.yaml",
		"runner-id":      "runner-a",
		"master-addr":    "10.0.0.1:9090",
		"token":          "secret-token",
		"runner-version": "1.2.3",
	}
	for k, v := range want {
		if checks[k] != v {
			t.Errorf("--%s = %q, want %q", k, checks[k], v)
		}
	}
}

func TestParseCLIEqualSignForm(t *testing.T) {
	cli, err := ParseCLI([]string{"--runner-id=runner-eq", "--master-addr=10.0.0.5:9090"}, io.Discard)
	if err != nil {
		t.Fatalf("ParseCLI: %v", err)
	}
	if cli.RunnerID != "runner-eq" || cli.MasterAddr != "10.0.0.5:9090" {
		t.Errorf("等号形式解析失败: id=%q master=%q", cli.RunnerID, cli.MasterAddr)
	}
}

func TestParseCLIHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		_, err := ParseCLI([]string{arg}, io.Discard)
		if !errors.Is(err, flag.ErrHelp) {
			t.Errorf("ParseCLI(%q) err = %v, want flag.ErrHelp", arg, err)
		}
	}
}

func TestParseCLIErrors(t *testing.T) {
	t.Run("未知选项", func(t *testing.T) {
		if _, err := ParseCLI([]string{"--nope"}, io.Discard); err == nil {
			t.Error("未知选项应报错")
		}
	})
	t.Run("多余位置参数", func(t *testing.T) {
		if _, err := ParseCLI([]string{"extra"}, io.Discard); err == nil {
			t.Error("位置参数应报错")
		}
	})
}

// ---------------------------------------------------------------- 外部配置定位

// T10：显式指定（--config / FLOWOPS_CONFIG）的文件缺失 → 失败，不 fallback
func TestLoadExternalExplicitMissing(t *testing.T) {
	_, _, err := LoadExternal(filepath.Join(t.TempDir(), "not-found.yaml"), "prod")
	if err == nil {
		t.Fatal("显式指定的配置文件缺失时应失败")
	}
}

// T12（显式路径变体）：显式指定的文件 YAML 非法 → 失败
func TestLoadExternalExplicitInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	writeYAML(t, path, "runner: [invalid\n")

	_, _, err := LoadExternal(path, "prod")
	if err == nil {
		t.Fatal("显式指定的配置文件 YAML 非法时应失败")
	}
}

// T11：默认路径不存在 → 正常，返回空表示使用内嵌配置
func TestLoadExternalDefaultMissing(t *testing.T) {
	stubExecutableDir(t, t.TempDir())

	cfg, path, err := LoadExternal("", "prod")
	if err != nil {
		t.Fatalf("默认外部配置不存在不应报错: %v", err)
	}
	if cfg != nil || path != "" {
		t.Errorf("应返回空表示未使用外部配置，实际 cfg=%v path=%q", cfg != nil, path)
	}
}

// T12：默认路径存在但 YAML 非法 → 失败，不静默回退
func TestLoadExternalDefaultInvalidYAML(t *testing.T) {
	exeDir := t.TempDir()
	stubExecutableDir(t, exeDir)
	writeYAML(t, filepath.Join(exeDir, "config", "config.prod.yaml"), "runner: [invalid\n")

	_, _, err := LoadExternal("", "prod")
	if err == nil {
		t.Fatal("默认外部配置 YAML 非法时应失败（禁止静默回退）")
	}
}

// T17：默认路径基于可执行文件目录，且与当前工作目录无关
func TestLoadExternalDefaultPathIndependentOfCWD(t *testing.T) {
	exeDir := t.TempDir()
	stubExecutableDir(t, exeDir)
	expected := filepath.Join(exeDir, "config", "config.prod.yaml")
	writeYAML(t, expected, validExternalYAML)

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	cfg, path, err := LoadExternal("", "prod")
	if err != nil {
		t.Fatalf("LoadExternal: %v", err)
	}
	if cfg == nil {
		t.Fatal("应加载到默认外部配置")
	}
	if path != expected {
		t.Errorf("路径 = %q, want %q", path, expected)
	}
	if cfg.Runner.Id != "runner-file" {
		t.Errorf("runner.id = %q, want runner-file", cfg.Runner.Id)
	}
}

// APP_ENV 决定默认外部配置文件名
func TestLoadExternalDefaultPathUsesAppEnv(t *testing.T) {
	exeDir := t.TempDir()
	stubExecutableDir(t, exeDir)
	writeYAML(t, filepath.Join(exeDir, "config", "config.dev.yaml"), "runner:\n  id: runner-dev-file\n")

	cfg, path, err := LoadExternal("", "dev")
	if err != nil {
		t.Fatalf("LoadExternal: %v", err)
	}
	if cfg == nil || cfg.Runner.Id != "runner-dev-file" {
		t.Fatalf("应加载 config.dev.yaml，实际 cfg=%v path=%q", cfg != nil, path)
	}
}

// 无法定位可执行文件目录时不使用默认外部配置（不报错）
func TestLoadExternalNoExecutableDir(t *testing.T) {
	stubExecutableDir(t, "")

	cfg, path, err := LoadExternal("", "prod")
	if err != nil {
		t.Fatalf("无法定位可执行文件目录不应报错: %v", err)
	}
	if cfg != nil || path != "" {
		t.Errorf("应返回空，实际 path=%q", path)
	}
}

// ---------------------------------------------------------------- Load 集成

// T14（Docker ENV 等价）：仅通过 ENV 覆盖全部核心字段
func TestLoadEnvOnlyOverridesEmbedded(t *testing.T) {
	clearEnvVars(t)
	stubExecutableDir(t, t.TempDir()) // 无默认外部配置

	t.Setenv(EnvAppEnv, "prod")
	t.Setenv(EnvRunnerID, "runner-env")
	t.Setenv(EnvMasterAddr, "10.9.9.9:9090")
	t.Setenv(EnvRunnerToken, "env-token")
	t.Setenv(EnvRunnerVersion, "3.0.0")

	res, err := Load(nil, io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if res.Config.Runner.Id != "runner-env" || res.Sources.RunnerID != SourceEnv {
		t.Errorf("runner.id = %q (%s), want runner-env (env)", res.Config.Runner.Id, res.Sources.RunnerID)
	}
	if res.Config.Runner.MasterAddr != "10.9.9.9:9090" || res.Sources.MasterAddr != SourceEnv {
		t.Errorf("master_addr = %q (%s), want 10.9.9.9:9090 (env)", res.Config.Runner.MasterAddr, res.Sources.MasterAddr)
	}
	if res.Config.Runner.Version != "3.0.0" || res.Sources.RunnerVersion != SourceEnv {
		t.Errorf("version = %q (%s), want 3.0.0 (env)", res.Config.Runner.Version, res.Sources.RunnerVersion)
	}
	if res.ConfigPath != "" {
		t.Errorf("未使用外部配置时 ConfigPath 应为空，实际 %q", res.ConfigPath)
	}
}

// T8：FLOWOPS_CONFIG 指定外部配置文件
func TestLoadEnvConfigPath(t *testing.T) {
	clearEnvVars(t)
	path := filepath.Join(t.TempDir(), "executor.yaml")
	writeYAML(t, path, validExternalYAML)

	t.Setenv(EnvAppEnv, "prod")
	t.Setenv(EnvConfig, path)
	t.Setenv(EnvRunnerToken, "env-token")

	res, err := Load(nil, io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if res.ConfigPath != path {
		t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, path)
	}
	if res.Config.Runner.Id != "runner-file" || res.Sources.RunnerID != SourceExternal {
		t.Errorf("runner.id = %q (%s), want runner-file (external)", res.Config.Runner.Id, res.Sources.RunnerID)
	}
	if res.Config.Runner.Token != "env-token" || res.Sources.Token != SourceEnv {
		t.Errorf("token 来源 = %s, want env（外部 YAML 未提供 token）", res.Sources.Token)
	}
	if res.Config.Runner.Version != "2.0.0" || res.Sources.RunnerVersion != SourceExternal {
		t.Errorf("version = %q (%s), want 2.0.0 (external)", res.Config.Runner.Version, res.Sources.RunnerVersion)
	}
}

// T9：--config 优先于 FLOWOPS_CONFIG
func TestLoadCLIConfigOverridesEnvConfig(t *testing.T) {
	clearEnvVars(t)
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env.yaml")
	cliPath := filepath.Join(dir, "cli.yaml")
	writeYAML(t, envPath, "runner:\n  id: runner-from-env-config\n")
	writeYAML(t, cliPath, validExternalYAML)

	t.Setenv(EnvAppEnv, "prod")
	t.Setenv(EnvConfig, envPath)
	t.Setenv(EnvRunnerToken, "env-token")

	res, err := Load([]string{"--config", cliPath}, io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if res.ConfigPath != cliPath {
		t.Errorf("ConfigPath = %q, want %q（--config 应优先）", res.ConfigPath, cliPath)
	}
	if res.Config.Runner.Id != "runner-file" {
		t.Errorf("runner.id = %q, want runner-file（来自 --config 文件）", res.Config.Runner.Id)
	}
}

// prod 环境未提供任何外部来源时的行为（不依赖私有配置内容）：
//   - 内嵌 prod 配置自带 token → 全部取值来自 embedded，启动通过
//   - 内嵌 prod 配置缺 token（或缺失）→ 校验失败/报缺配置
//
// 「所有来源都为空 → 校验失败」的核心语义由 TestValidate 覆盖。
func TestLoadProdNoExternalSources(t *testing.T) {
	clearEnvVars(t)
	stubExecutableDir(t, t.TempDir())

	t.Setenv(EnvAppEnv, "prod")

	res, err := Load(nil, io.Discard)
	if errors.Is(err, ErrEmbeddedNotFound) {
		t.Skip("内嵌 prod 配置未纳入本次构建（config.prod.yaml 不存在时）")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "runner.token") {
			t.Fatalf("校验失败时应指出缺少 runner.token，实际: %v", err)
		}
		return // 内嵌配置无 token → 启动失败，符合预期
	}

	// 内嵌配置自带 token（环境私有配置的常见情况）→ 各字段均来自 embedded
	if res.Sources.Token != SourceEmbedded || res.Sources.RunnerID != SourceEmbedded {
		t.Errorf("未提供外部来源时核心字段应来自内嵌配置，实际 id=%s token=%s",
			res.Sources.RunnerID, res.Sources.Token)
	}
	if res.ConfigPath != "" {
		t.Errorf("未使用外部配置时 ConfigPath 应为空，实际 %q", res.ConfigPath)
	}
}

// dev 内嵌配置可独立启动（本地开发开箱即用）
//
// 注意：config.dev.yaml 属环境私有配置、不入库；若当前构建未把它纳入 embed
// （例如 clone 后未创建），本用例会跳过而非失败。
func TestLoadDevEmbeddedSucceeds(t *testing.T) {
	clearEnvVars(t)
	stubExecutableDir(t, t.TempDir())

	t.Setenv(EnvAppEnv, "dev")

	res, err := Load(nil, io.Discard)
	if errors.Is(err, ErrEmbeddedNotFound) {
		t.Skip("内嵌 dev 配置未纳入本次构建（config.dev.yaml 存在时才可用）")
	}
	if err != nil {
		t.Fatalf("dev 内嵌配置应可直接使用: %v", err)
	}
	if res.Config.Runner.Id != "runner-dev-1" || res.Sources.RunnerID != SourceEmbedded {
		t.Errorf("runner.id = %q (%s), want runner-dev-1 (embedded)", res.Config.Runner.Id, res.Sources.RunnerID)
	}
	if res.Config.Runner.Token != "dev-runner-token" {
		t.Errorf("dev token 应来自内嵌配置，实际 %q", res.Config.Runner.Token)
	}
	if res.Sources.AppEnv != SourceEnv {
		t.Errorf("APP_ENV 来源 = %s, want env", res.Sources.AppEnv)
	}
}

// --env 优先于 APP_ENV
func TestLoadCliEnvOverridesAppEnv(t *testing.T) {
	clearEnvVars(t)
	stubExecutableDir(t, t.TempDir())

	t.Setenv(EnvAppEnv, "prod")
	t.Setenv(EnvRunnerToken, "t") // 让 prod 也能通过校验

	res, err := Load([]string{"--env", "dev"}, io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if res.AppEnv != "dev" || res.Sources.AppEnv != SourceCLI {
		t.Errorf("APP_ENV = %q (%s), want dev (cli)", res.AppEnv, res.Sources.AppEnv)
	}
	if res.Config.Runner.Id != "runner-dev-1" {
		t.Errorf("应使用 dev 内嵌配置，实际 id=%q", res.Config.Runner.Id)
	}
}

// 非法 APP_ENV → 失败
func TestLoadInvalidAppEnvFails(t *testing.T) {
	clearEnvVars(t)
	stubExecutableDir(t, t.TempDir())

	t.Setenv(EnvAppEnv, "staging")

	if _, err := Load(nil, io.Discard); err == nil {
		t.Fatal("非法 APP_ENV 应失败")
	}
}

// --help 返回 ErrHelp（用法由调用方打印）
func TestLoadHelp(t *testing.T) {
	_, err := Load([]string{"--help"}, io.Discard)
	if !errors.Is(err, ErrHelp) {
		t.Fatalf("Load(--help) err = %v, want ErrHelp", err)
	}
}

// CLI 全字段覆盖（四级优先级的最上一层）
func TestLoadCLIOverridesEverything(t *testing.T) {
	clearEnvVars(t)
	path := filepath.Join(t.TempDir(), "executor.yaml")
	writeYAML(t, path, validExternalYAML)

	t.Setenv(EnvAppEnv, "prod")
	t.Setenv(EnvConfig, path)
	t.Setenv(EnvRunnerID, "runner-env")
	t.Setenv(EnvMasterAddr, "10.0.0.2:9090")
	t.Setenv(EnvRunnerToken, "env-token")
	t.Setenv(EnvRunnerVersion, "9.9.9")

	res, err := Load([]string{
		"--runner-id", "runner-cli",
		"--master-addr", "10.0.0.3:9090",
		"--runner-version", "1.1.1",
	}, io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := []struct {
		name    string
		got     string
		want    string
		wantSrc ValueSource
	}{
		{"runner.id", res.Config.Runner.Id, "runner-cli", SourceCLI},
		{"master_addr", res.Config.Runner.MasterAddr, "10.0.0.3:9090", SourceCLI},
		{"version", res.Config.Runner.Version, "1.1.1", SourceCLI},
		{"token", res.Config.Runner.Token, "env-token", SourceEnv}, // CLI 未提供 → ENV
	}
	gotSrc := map[string]ValueSource{
		"runner.id":   res.Sources.RunnerID,
		"master_addr": res.Sources.MasterAddr,
		"version":     res.Sources.RunnerVersion,
		"token":       res.Sources.Token,
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
		if gotSrc[c.name] != c.wantSrc {
			t.Errorf("%s 来源 = %s, want %s", c.name, gotSrc[c.name], c.wantSrc)
		}
	}
}

// T16（集成）：Load 结果的日志摘要不含 token 明文
func TestLoadLogSummaryHidesToken(t *testing.T) {
	clearEnvVars(t)
	stubExecutableDir(t, t.TempDir())

	const secret = "integration-secret-token"
	t.Setenv(EnvAppEnv, "prod")
	t.Setenv(EnvRunnerToken, secret)
	t.Setenv(EnvRunnerID, "runner-1")
	t.Setenv(EnvMasterAddr, "10.0.0.1:9090")
	t.Setenv(EnvRunnerVersion, "1.0.0")

	res, err := Load(nil, io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	joined := strings.Join(res.LogSummary(), "\n")
	if strings.Contains(joined, secret) {
		t.Fatalf("日志摘要泄露 token 明文:\n%s", joined)
	}
}
