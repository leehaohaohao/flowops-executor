package config

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

// ErrEmbeddedNotFound 内嵌配置中不存在该环境的配置
var ErrEmbeddedNotFound = errors.New("内嵌配置不存在")

// 内嵌配置：作为最低优先级的默认配置。
//
// 使用通配模式而非逐个文件名，原因：
//   - config.dev.yaml / config.prod.yaml 为**环境私有配置，刻意不入库**（见 .gitignore），
//     逐个列出会导致 clone 后因文件缺失而构建失败；
//   - 通配模式只要目录下至少存在一个匹配文件即可构建（仓库中为 config.example.yaml）。
//
// 因此某环境的内嵌配置**可能缺失**：此时 LoadEmbedded 返回 ErrEmbeddedNotFound，
// 并提示改用外部配置 / CLI / ENV 提供取值。
//
//go:embed config.*.yaml
var embeddedFS embed.FS

// LoadEmbedded 读取内嵌配置（env 取值 prod / dev）
func LoadEmbedded(env string) (*Config, error) {
	if !IsValidAppEnv(env) {
		return nil, fmt.Errorf("APP_ENV 非法: %q（支持: %s / %s）", env, AppEnvProd, AppEnvDev)
	}

	name := fmt.Sprintf("config.%s.yaml", env)
	data, err := fs.ReadFile(embeddedFS, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s —— %s 未编译进二进制（环境私有配置不入库）；"+
				"请在 config/ 下从 config.example.yaml 创建它，或改用外部配置 / CLI / ENV 提供取值",
				ErrEmbeddedNotFound, name, name)
		}
		return nil, fmt.Errorf("读取内嵌配置失败 (%s): %w", name, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析内嵌配置失败 (%s): %w", name, err)
	}
	return &cfg, nil
}
