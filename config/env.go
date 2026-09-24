package config

import (
	"os"
	"strings"
)

// 环境变量名（第一阶段支持覆盖的字段）
const (
	EnvAppEnv        = "APP_ENV"                // 运行环境
	EnvConfig        = "FLOWOPS_CONFIG"         // 外部配置文件路径
	EnvRunnerID      = "FLOWOPS_RUNNER_ID"      // runner.id
	EnvMasterAddr    = "FLOWOPS_MASTER_ADDR"    // runner.master_addr
	EnvRunnerToken   = "FLOWOPS_RUNNER_TOKEN"   // runner.token
	EnvRunnerVersion = "FLOWOPS_RUNNER_VERSION" // runner.version
)

// 运行环境取值与默认值
const (
	AppEnvProd    = "prod"
	AppEnvDev     = "dev"
	DefaultAppEnv = AppEnvProd
)

// IsValidAppEnv 判断 APP_ENV 是否合法
func IsValidAppEnv(env string) bool {
	return env == AppEnvProd || env == AppEnvDev
}

// EnvValues 一次读取全部相关环境变量。
// 抽出为结构体便于测试注入，避免测试依赖真实进程环境。
type EnvValues struct {
	AppEnv        string
	ConfigPath    string
	RunnerID      string
	MasterAddr    string
	Token         string
	RunnerVersion string
}

// LoadEnv 从进程环境读取（值会去除首尾空白）
func LoadEnv() EnvValues {
	return EnvValues{
		AppEnv:        getenv(EnvAppEnv),
		ConfigPath:    getenv(EnvConfig),
		RunnerID:      getenv(EnvRunnerID),
		MasterAddr:    getenv(EnvMasterAddr),
		Token:         getenv(EnvRunnerToken),
		RunnerVersion: getenv(EnvRunnerVersion),
	}
}

// getenv 读取环境变量并去除首尾空白（空白视为未设置）
func getenv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
