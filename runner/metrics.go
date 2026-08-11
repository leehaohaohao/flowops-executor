package runner

import (
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

// collectHostMetrics 采集节点 CPU 使用率与内存使用率（百分比）
func collectHostMetrics() (cpuPercent, memPercent float64) {
	if percents, err := cpu.Percent(0, false); err == nil && len(percents) > 0 {
		cpuPercent = percents[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		memPercent = vm.UsedPercent
	}
	return
}
