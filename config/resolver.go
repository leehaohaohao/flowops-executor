package config

import "strings"

// ValueSource 配置值来源
type ValueSource string

const (
	SourceCLI      ValueSource = "cli"      // 命令行参数
	SourceEnv      ValueSource = "env"      // 环境变量
	SourceExternal ValueSource = "external" // 外部 YAML
	SourceEmbedded ValueSource = "embedded" // 内嵌 YAML
	SourceDefault  ValueSource = "default"  // 内置默认值（如 APP_ENV 默认 prod）
	SourceNone     ValueSource = ""         // 所有来源均为空
)

// ConfigSources 最终值的来源记录（用于启动日志与排查）
type ConfigSources struct {
	AppEnv        ValueSource
	RunnerID      ValueSource
	MasterAddr    ValueSource
	Token         ValueSource
	RunnerVersion ValueSource
}

type stringCandidate struct {
	value  string
	source ValueSource
}

type intCandidate struct {
	value  int
	source ValueSource
}

// resolveString 返回第一个非空（去除首尾空白后）的值及其来源
func resolveString(cands ...stringCandidate) (string, ValueSource) {
	for _, c := range cands {
		if v := strings.TrimSpace(c.value); v != "" {
			return v, c.source
		}
	}
	return "", SourceNone
}

// resolveInt 返回第一个非零的值及其来源
func resolveInt(cands ...intCandidate) (int, ValueSource) {
	for _, c := range cands {
		if c.value != 0 {
			return c.value, c.source
		}
	}
	return 0, SourceNone
}

// resolveString2 两级解析：外部 > 内嵌（供非核心字段使用）
func resolveString2(external, embedded string) string {
	v, _ := resolveString(
		stringCandidate{external, SourceExternal},
		stringCandidate{embedded, SourceEmbedded},
	)
	return v
}

// resolveInt2 两级解析：外部 > 内嵌（供非核心字段使用）
func resolveInt2(external, embedded int) int {
	v, _ := resolveInt(
		intCandidate{external, SourceExternal},
		intCandidate{embedded, SourceEmbedded},
	)
	return v
}

// Resolve 逐字段解析配置，优先级：CLI > ENV > 外部 YAML > 内嵌 YAML。
//
// 核心字段（runner.id / master_addr / token / version）走四级解析；
// 其余字段当前走「外部 > 内嵌」两级（CLI/ENV 位留空，后续可按需追加）。
//
// 注意：空字符串与 0 均视为「未提供」，会继续向后 fallback。
func Resolve(cli *CLIOptions, env EnvValues, embedded, external *Config) (*Config, *ConfigSources) {
	if cli == nil {
		cli = &CLIOptions{}
	}
	emb := embedded
	if emb == nil {
		emb = &Config{}
	}
	ext := external
	if ext == nil {
		ext = &Config{}
	}

	var final Config
	src := &ConfigSources{}

	// ---- 核心字段：CLI > ENV > External > Embedded ----
	final.Runner.Id, src.RunnerID = resolveString(
		stringCandidate{cli.RunnerID, SourceCLI},
		stringCandidate{env.RunnerID, SourceEnv},
		stringCandidate{ext.Runner.Id, SourceExternal},
		stringCandidate{emb.Runner.Id, SourceEmbedded},
	)
	final.Runner.MasterAddr, src.MasterAddr = resolveString(
		stringCandidate{cli.MasterAddr, SourceCLI},
		stringCandidate{env.MasterAddr, SourceEnv},
		stringCandidate{ext.Runner.MasterAddr, SourceExternal},
		stringCandidate{emb.Runner.MasterAddr, SourceEmbedded},
	)
	final.Runner.Token, src.Token = resolveString(
		stringCandidate{cli.Token, SourceCLI},
		stringCandidate{env.Token, SourceEnv},
		stringCandidate{ext.Runner.Token, SourceExternal},
		stringCandidate{emb.Runner.Token, SourceEmbedded},
	)
	final.Runner.Version, src.RunnerVersion = resolveString(
		stringCandidate{cli.RunnerVersion, SourceCLI},
		stringCandidate{env.RunnerVersion, SourceEnv},
		stringCandidate{ext.Runner.Version, SourceExternal},
		stringCandidate{emb.Runner.Version, SourceEmbedded},
	)

	// ---- runner 非核心字段：External > Embedded ----
	final.Runner.HeartbeatInterval = resolveInt2(ext.Runner.HeartbeatInterval, emb.Runner.HeartbeatInterval)
	final.Runner.ConnectTimeout = resolveInt2(ext.Runner.ConnectTimeout, emb.Runner.ConnectTimeout)
	final.Runner.ReconnectInitialInterval = resolveInt2(ext.Runner.ReconnectInitialInterval, emb.Runner.ReconnectInitialInterval)
	final.Runner.ReconnectMaxInterval = resolveInt2(ext.Runner.ReconnectMaxInterval, emb.Runner.ReconnectMaxInterval)
	final.Runner.RegisterTimeout = resolveInt2(ext.Runner.RegisterTimeout, emb.Runner.RegisterTimeout)
	final.Runner.AuthRetryInterval = resolveInt2(ext.Runner.AuthRetryInterval, emb.Runner.AuthRetryInterval)

	// ---- database：External > Embedded ----
	final.Database.Host = resolveString2(ext.Database.Host, emb.Database.Host)
	final.Database.Port = resolveInt2(ext.Database.Port, emb.Database.Port)
	final.Database.User = resolveString2(ext.Database.User, emb.Database.User)
	final.Database.Password = resolveString2(ext.Database.Password, emb.Database.Password)
	final.Database.DBName = resolveString2(ext.Database.DBName, emb.Database.DBName)
	final.Database.Charset = resolveString2(ext.Database.Charset, emb.Database.Charset)

	// ---- log：External > Embedded ----
	final.Log.Level = resolveString2(ext.Log.Level, emb.Log.Level)
	final.Log.FilePath = resolveString2(ext.Log.FilePath, emb.Log.FilePath)
	final.Log.MaxSize = resolveInt2(ext.Log.MaxSize, emb.Log.MaxSize)
	final.Log.MaxBackups = resolveInt2(ext.Log.MaxBackups, emb.Log.MaxBackups)
	final.Log.MaxAge = resolveInt2(ext.Log.MaxAge, emb.Log.MaxAge)

	return &final, src
}
