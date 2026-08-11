package runner

import (
	"math"
	"testing"
)

func TestCollectHostMetrics(t *testing.T) {
	cpu, mem := collectHostMetrics()
	if math.IsNaN(cpu) || cpu < 0 {
		t.Errorf("CPU 使用率异常: %v", cpu)
	}
	if math.IsNaN(mem) || mem < 0 {
		t.Errorf("内存使用率异常: %v", mem)
	}
}
