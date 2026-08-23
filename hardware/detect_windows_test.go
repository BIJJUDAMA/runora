//go:build windows

package hardware

import (
	"testing"
	"time"
)

func TestDXGIFactoryDiscovery(t *testing.T) {
	start := time.Now()
	name, vram, gpuType, err := getWindowsGPUviaDXGI()
	elapsed := time.Since(start)
	t.Logf("DXGI Discovery: Name=%q, VRAM=%d bytes, Type=%s, Err=%v, took %v", name, vram, gpuType, err, elapsed)
}
