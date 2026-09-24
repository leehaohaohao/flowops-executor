package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Validate 在全部来源解析完成后统一校验最终配置。
//
// 不能因为某一来源（如外部配置）缺字段就提前失败：
// 更高优先级来源（CLI / ENV）或更低优先级来源（内嵌）可能提供该字段。
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("配置为空")
	}

	var missing []string
	if strings.TrimSpace(cfg.Runner.Id) == "" {
		missing = append(missing, "runner.id")
	}
	if strings.TrimSpace(cfg.Runner.MasterAddr) == "" {
		missing = append(missing, "runner.master_addr")
	}
	if strings.TrimSpace(cfg.Runner.Token) == "" {
		missing = append(missing, "runner.token")
	}
	if strings.TrimSpace(cfg.Runner.Version) == "" {
		missing = append(missing, "runner.version")
	}
	if len(missing) > 0 {
		return fmt.Errorf("配置校验失败，以下字段在 CLI / ENV / 外部配置 / 内嵌配置中均为空: %s", strings.Join(missing, ", "))
	}

	return validateMasterAddr(cfg.Runner.MasterAddr)
}

// validateMasterAddr 校验 host:port 格式
func validateMasterAddr(addr string) error {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("runner.master_addr 格式非法: %q（期望 host:port）", addr)
	}
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("runner.master_addr 缺少主机名: %q", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("runner.master_addr 端口非法: %q（端口范围 1-65535）", addr)
	}
	return nil
}
