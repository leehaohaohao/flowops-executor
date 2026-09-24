package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// ErrHelp 表示用户请求帮助信息（用法已输出，调用方应正常退出）
var ErrHelp = errors.New("help requested")

// LoadResult 配置加载结果
type LoadResult struct {
	Config     *Config        // 最终生效配置
	Sources    *ConfigSources // 各核心字段的来源
	AppEnv     string         // 最终 APP_ENV
	ConfigPath string         // 实际使用的外部配置文件路径；空表示未使用外部配置
}

// Load 一站式加载配置：
//
//	CLI → ENV → 外部 YAML → 内嵌 YAML → 逐字段解析 → 统一校验
//
// out 用于输出用法与参数错误信息（通常为 os.Stderr）。
func Load(args []string, out io.Writer) (*LoadResult, error) {
	cli, err := ParseCLI(args, out)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, ErrHelp
		}
		return nil, err
	}

	env := LoadEnv()

	// APP_ENV：--env > APP_ENV > 默认 prod
	appEnv, appEnvSrc := resolveString(
		stringCandidate{cli.Env, SourceCLI},
		stringCandidate{env.AppEnv, SourceEnv},
		stringCandidate{DefaultAppEnv, SourceDefault},
	)
	if !IsValidAppEnv(appEnv) {
		return nil, fmt.Errorf("APP_ENV 非法: %q（支持: %s / %s）", appEnv, AppEnvProd, AppEnvDev)
	}

	// 内嵌配置（最低优先级，保证可解析出完整结构）
	embedded, err := LoadEmbedded(appEnv)
	if err != nil {
		return nil, err
	}

	// 外部配置：--config > FLOWOPS_CONFIG > 默认路径
	explicitPath := firstNonEmpty(cli.ConfigPath, env.ConfigPath)
	external, path, err := LoadExternal(explicitPath, appEnv)
	if err != nil {
		return nil, err
	}

	// 逐字段解析
	final, sources := Resolve(cli, env, embedded, external)
	sources.AppEnv = appEnvSrc

	// 统一校验
	if err := Validate(final); err != nil {
		return nil, err
	}

	return &LoadResult{
		Config:     final,
		Sources:    sources,
		AppEnv:     appEnv,
		ConfigPath: path,
	}, nil
}

// firstNonEmpty 返回第一个非空（去除首尾空白后）的字符串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// LogSummary 返回可安全输出的启动配置摘要。
//
// 安全约束：不得包含 token 明文，只输出 configured / missing。
func (r *LoadResult) LogSummary() []string {
	if r == nil || r.Config == nil {
		return nil
	}

	configFile := "（未使用外部配置，使用内嵌配置）"
	if r.ConfigPath != "" {
		configFile = r.ConfigPath
	}

	return []string{
		fmt.Sprintf("APP_ENV: %s (%s)", r.AppEnv, sourceLabel(r.Sources.AppEnv)),
		fmt.Sprintf("Config File: %s", configFile),
		fmt.Sprintf("Runner ID: %s (%s)", r.Config.Runner.Id, sourceLabel(r.Sources.RunnerID)),
		fmt.Sprintf("Master Addr: %s (%s)", r.Config.Runner.MasterAddr, sourceLabel(r.Sources.MasterAddr)),
		fmt.Sprintf("Runner Version: %s (%s)", r.Config.Runner.Version, sourceLabel(r.Sources.RunnerVersion)),
		fmt.Sprintf("Runner Token: %s (%s)", tokenState(r.Config.Runner.Token), sourceLabel(r.Sources.Token)),
	}
}

func tokenState(token string) string {
	if strings.TrimSpace(token) == "" {
		return "missing"
	}
	return "configured"
}

func sourceLabel(s ValueSource) string {
	if s == SourceNone {
		return "none"
	}
	return string(s)
}
