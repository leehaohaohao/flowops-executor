package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// executableDir 返回当前可执行文件所在目录；无法获取时返回空字符串。
// 声明为变量以便测试替换（默认路径定位依赖它，而不是当前工作目录）。
var executableDir = func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

// DefaultExternalConfigPath 返回默认外部配置路径：
//
//	<可执行文件目录>/config/config.{env}.yaml
//
// 无法定位可执行文件目录时返回空字符串（表示不使用默认外部配置）。
func DefaultExternalConfigPath(env string) string {
	dir := executableDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config", fmt.Sprintf("config.%s.yaml", env))
}

// LoadExternal 加载外部配置。
//
// 定位与失败策略：
//   - explicitPath 非空（来自 --config 或 FLOWOPS_CONFIG）：文件缺失、不可读、YAML 非法
//     一律返回错误，**不回退**到内嵌配置；
//   - explicitPath 为空：尝试默认路径，不存在视为「无外部配置」正常返回 nil；
//     存在但不可读或 YAML 非法同样返回错误，**不静默回退**。
//
// 返回 (配置, 实际路径, 错误)；配置为 nil 表示未使用外部配置。
func LoadExternal(explicitPath, env string) (*Config, string, error) {
	path := strings.TrimSpace(explicitPath)
	explicit := path != ""

	if !explicit {
		path = DefaultExternalConfigPath(env)
		if path == "" {
			return nil, "", nil
		}
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return nil, "", nil
			}
			return nil, path, fmt.Errorf("检查默认外部配置失败 (%s): %w", path, err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if explicit {
			return nil, path, fmt.Errorf("读取外部配置失败 (由 --config/FLOWOPS_CONFIG 显式指定: %s): %w", path, err)
		}
		return nil, path, fmt.Errorf("读取默认外部配置失败 (%s): %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		if explicit {
			return nil, path, fmt.Errorf("解析外部配置失败 (由 --config/FLOWOPS_CONFIG 显式指定: %s): %w", path, err)
		}
		return nil, path, fmt.Errorf("解析默认外部配置失败 (%s): %w", path, err)
	}
	return &cfg, path, nil
}
