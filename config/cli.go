package config

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// CommandName 用法信息中显示的程序名
const CommandName = "flowops-executor"

// CLIOptions 命令行参数（未提供的字段为零值）
type CLIOptions struct {
	Env           string // --env
	ConfigPath    string // --config
	RunnerID      string // --runner-id
	MasterAddr    string // --master-addr
	Token         string // --token
	RunnerVersion string // --runner-version
}

// ParseCLI 解析命令行参数（args 不含程序名）。
//
// 返回 flag.ErrHelp 表示用户请求帮助（用法已输出）。
func ParseCLI(args []string, out io.Writer) (*CLIOptions, error) {
	opts := &CLIOptions{}

	fs := flag.NewFlagSet(CommandName, flag.ContinueOnError)
	fs.SetOutput(out)
	// 用法由调用方决定输出目标（--help → stdout；参数错误信息仍写入 out）
	fs.Usage = func() {}

	fs.StringVar(&opts.Env, "env", "", "运行环境 prod|dev（等价于 APP_ENV）")
	fs.StringVar(&opts.ConfigPath, "config", "", "外部配置文件路径（等价于 FLOWOPS_CONFIG）")
	fs.StringVar(&opts.RunnerID, "runner-id", "", "runner 唯一标识（等价于 FLOWOPS_RUNNER_ID）")
	fs.StringVar(&opts.MasterAddr, "master-addr", "", "Master 地址 host:port（等价于 FLOWOPS_MASTER_ADDR）")
	fs.StringVar(&opts.Token, "token", "", "注册令牌（等价于 FLOWOPS_RUNNER_TOKEN）")
	fs.StringVar(&opts.RunnerVersion, "runner-version", "", "runner 版本号（等价于 FLOWOPS_RUNNER_VERSION）")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("不支持的参数: %s（使用 -h 查看用法）", strings.Join(fs.Args(), " "))
	}
	return opts, nil
}

// PrintUsage 输出用法说明
func PrintUsage(out io.Writer) {
	fmt.Fprintf(out, `%[1]s — FlowOps 子节点执行器

用法:
  %[1]s [选项]

选项:
  --env <prod|dev>           运行环境（默认 prod，等价于 APP_ENV）
  --config <path>            外部配置文件路径（等价于 FLOWOPS_CONFIG）
  --runner-id <id>           runner 唯一标识（等价于 FLOWOPS_RUNNER_ID）
  --master-addr <host:port>  Master 地址（等价于 FLOWOPS_MASTER_ADDR）
  --token <token>            注册令牌（等价于 FLOWOPS_RUNNER_TOKEN）
  --runner-version <ver>     runner 版本号（等价于 FLOWOPS_RUNNER_VERSION）
  -h, --help                 显示本帮助

配置优先级（每个字段独立解析，取第一个非空值）:
  CLI > ENV > 外部 YAML > 内嵌 YAML

外部配置文件定位:
  --config > FLOWOPS_CONFIG > <可执行文件目录>/config/config.{APP_ENV}.yaml
  · 显式指定（--config / FLOWOPS_CONFIG）的文件缺失、不可读或 YAML 非法 → 启动失败
  · 默认路径不存在 → 正常，使用内嵌配置
  · 默认路径存在但读取失败 → 启动失败（不静默回退）

示例:
  %[1]s --master-addr 10.0.0.10:9090 --runner-id runner-a --token secret
  FLOWOPS_MASTER_ADDR=10.0.0.10:9090 FLOWOPS_RUNNER_TOKEN=secret %[1]s
  %[1]s --config /etc/flowops/executor.yaml

注意: --token 便于本地调试，但可能出现在 shell history / ps / CI 日志中；
      正式部署建议使用 FLOWOPS_RUNNER_TOKEN 或外部配置文件。
`, CommandName)
}
