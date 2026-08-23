package hardware

import (
	"testing"
	"time"

	"github.com/BIJJUDAMA/runora/model"
)

func TestEstimateMemoryUnified(t *testing.T) {
	// macOS Apple Silicon profile: Total RAM 16GB, Unified VRAM limit 12GB (75%)
	specs := &HardwareSpecs{
		OS:        "macOS",
		IsUnified: true,
		RAM: RAMSpecs{
			Total:     16 * 1024 * 1024 * 1024,
			Available: 10 * 1024 * 1024 * 1024,
		},
		GPU: GPUSpecs{
			Name: "Apple Silicon Integrated GPU",
			VRAM: 12 * 1024 * 1024 * 1024,
			Type: "Metal",
		},
	}

	// Case 1: Model fits fully in VRAM (e.g. 4GB file size)
	meta1 := &model.GGUFMetadata{
		FileSize:     4 * 1024 * 1024 * 1024,
		Layers:       32,
		Heads:        32,
		HeadsKV:      8,
		EmbeddingLen: 4096,
	}

	est1 := EstimateMemory(meta1, specs, 2048)
	if est1.Suitability != SuitabilityFitsVRAM {
		t.Errorf("expected FitsVRAM suitability, got %d. Reason: %s", est1.Suitability, est1.Reason)
	}
	if est1.GPUOffloadPct != 100 {
		t.Errorf("expected 100%% offload, got %d%%", est1.GPUOffloadPct)
	}

	// Case 2: Partial offload (e.g. 13GB file size -> total memory ~14GB <= 16GB RAM)
	meta2 := &model.GGUFMetadata{
		FileSize:     13 * 1024 * 1024 * 1024,
		Layers:       32,
		Heads:        32,
		HeadsKV:      8,
		EmbeddingLen: 4096,
	}
	est2 := EstimateMemory(meta2, specs, 2048)
	if est2.Suitability != SuitabilityPartialVRAM {
		t.Errorf("expected PartialVRAM suitability, got %d. Reason: %s", est2.Suitability, est2.Reason)
	}

	// Case 3: Exceeds total RAM (e.g. 18GB file size)
	meta3 := &model.GGUFMetadata{
		FileSize:     18 * 1024 * 1024 * 1024,
		Layers:       32,
		Heads:        32,
		HeadsKV:      8,
		EmbeddingLen: 4096,
	}
	est3 := EstimateMemory(meta3, specs, 2048)
	if est3.Suitability != SuitabilityExceeds {
		t.Errorf("expected Exceeds suitability, got %d. Reason: %s", est3.Suitability, est3.Reason)
	}
}

func TestEstimateMemoryDedicated(t *testing.T) {
	// Windows/Linux PC profile: System RAM 16GB, GPU VRAM 8GB
	specs := &HardwareSpecs{
		OS:        "Windows",
		IsUnified: false,
		RAM: RAMSpecs{
			Total:     16 * 1024 * 1024 * 1024,
			Available: 12 * 1024 * 1024 * 1024,
		},
		GPU: GPUSpecs{
			Name: "NVIDIA GeForce RTX 4070",
			VRAM: 8 * 1024 * 1024 * 1024,
			Type: "CUDA",
		},
	}

	// Case 1: Model fits fully in VRAM (e.g. 5GB file size)
	meta1 := &model.GGUFMetadata{
		FileSize:     5 * 1024 * 1024 * 1024,
		Layers:       32,
		Heads:        32,
		HeadsKV:      8,
		EmbeddingLen: 4096,
	}
	est1 := EstimateMemory(meta1, specs, 2048)
	if est1.Suitability != SuitabilityFitsVRAM {
		t.Errorf("expected FitsVRAM suitability, got %d. Reason: %s", est1.Suitability, est1.Reason)
	}

	// Case 2: Partial offload (e.g. 10GB file size)
	meta2 := &model.GGUFMetadata{
		FileSize:     10 * 1024 * 1024 * 1024,
		Layers:       32,
		Heads:        32,
		HeadsKV:      8,
		EmbeddingLen: 4096,
	}
	est2 := EstimateMemory(meta2, specs, 2048)
	if est2.Suitability != SuitabilityPartialVRAM {
		t.Errorf("expected PartialVRAM suitability, got %d. Reason: %s", est2.Suitability, est2.Reason)
	}

	// Case 3: Exceeds total system memory (e.g. 26GB file size)
	meta3 := &model.GGUFMetadata{
		FileSize:     26 * 1024 * 1024 * 1024,
		Layers:       32,
		Heads:        32,
		HeadsKV:      8,
		EmbeddingLen: 4096,
	}
	est3 := EstimateMemory(meta3, specs, 2048)
	if est3.Suitability != SuitabilityExceeds {
		t.Errorf("expected Exceeds suitability, got %d. Reason: %s", est3.Suitability, est3.Reason)
	}

	// Case 4: Fits in RAM only (CPU mode on GPU-less machine)
	specsCPU := &HardwareSpecs{
		OS:        "Linux",
		IsUnified: false,
		RAM: RAMSpecs{
			Total:     32 * 1024 * 1024 * 1024,
			Available: 28 * 1024 * 1024 * 1024,
		},
		GPU: GPUSpecs{
			Name: "Integrated",
			VRAM: 0,
			Type: "CPU",
		},
	}
	estCPU := EstimateMemory(meta1, specsCPU, 2048)
	if estCPU.Suitability != SuitabilityFitsRAM {
		t.Errorf("expected FitsRAM suitability, got %d. Reason: %s", estCPU.Suitability, estCPU.Reason)
	}
	if estCPU.GPUOffloadPct != 0 {
		t.Errorf("expected 0%% GPU offload on CPU mode, got %d%%", estCPU.GPUOffloadPct)
	}
}

func TestUniversalKVCacheMHA_GQA_MLA(t *testing.T) {
	specs := &HardwareSpecs{
		OS:        "Windows",
		IsUnified: false,
		RAM: RAMSpecs{
			Total: 64 * 1024 * 1024 * 1024,
		},
		GPU: GPUSpecs{
			VRAM: 24 * 1024 * 1024 * 1024,
		},
	}

	// 1. MHA (HeadsKV == 0 -> fallback to Heads = 32)
	metaMHA := &model.GGUFMetadata{
		FileSize:     4 * 1024 * 1024 * 1024,
		Layers:       32,
		Heads:        32,
		HeadsKV:      0, // MHA fallback
		EmbeddingLen: 4096,
		HeadDim:      128,
	}
	estMHA := EstimateMemory(metaMHA, specs, 4096)
	expectedMHAKV := uint64(4) * 32 * 128 * 32 * 4096 // 4 * headsKV * headDim * layers * ctx
	if estMHA.KVCacheSize != expectedMHAKV {
		t.Errorf("expected MHA KV cache size %d, got %d", expectedMHAKV, estMHA.KVCacheSize)
	}

	// 2. GQA (Heads = 32, HeadsKV = 8)
	metaGQA := &model.GGUFMetadata{
		FileSize:     4 * 1024 * 1024 * 1024,
		Layers:       32,
		Heads:        32,
		HeadsKV:      8,
		EmbeddingLen: 4096,
		HeadDim:      128,
	}
	estGQA := EstimateMemory(metaGQA, specs, 4096)
	expectedGQAKV := uint64(4) * 8 * 128 * 32 * 4096
	if estGQA.KVCacheSize != expectedGQAKV {
		t.Errorf("expected GQA KV cache size %d, got %d", expectedGQAKV, estGQA.KVCacheSize)
	}

	// 3. MLA (DeepSeek architecture)
	metaMLA := &model.GGUFMetadata{
		Architecture: "deepseek2",
		FileSize:     16 * 1024 * 1024 * 1024,
		Layers:       60,
		Heads:        128,
		HeadsKV:      128,
		EmbeddingLen: 7168,
	}
	estMLA := EstimateMemory(metaMLA, specs, 4096)
	// MLA: 576 * 2 bytes/layer * 60 layers * 4096 ctx = 283,115,520 bytes
	expectedMLAKV := uint64(576 * 2) * 60 * 4096
	if estMLA.KVCacheSize != expectedMLAKV {
		t.Errorf("expected MLA KV cache size %d, got %d", expectedMLAKV, estMLA.KVCacheSize)
	}

	// 4. Dynamic Activations Check: batchSize(512) * embedLen(4096) * 4.5 = 9,437,184
	expectedActivation := uint64(float64(512*4096) * 4.5)
	if estMHA.ActivationSize != expectedActivation {
		t.Errorf("expected activation size %d, got %d", expectedActivation, estMHA.ActivationSize)
	}
}

func TestAppleSiliconMetalCurve(t *testing.T) {
	// Test the dynamic piecewise curve for Apple Silicon
	gb8 := uint64(8 * 1024 * 1024 * 1024)
	vram8 := AppleSiliconMetalVRAM(gb8)
	expected8 := uint64(float64(gb8) * 0.67)
	if vram8 != expected8 {
		t.Errorf("expected 8GB VRAM %d, got %d", expected8, vram8)
	}

	gb16 := uint64(16 * 1024 * 1024 * 1024)
	vram16 := AppleSiliconMetalVRAM(gb16)
	expected16 := uint64(float64(gb16) * 0.75)
	if vram16 != expected16 {
		t.Errorf("expected 16GB VRAM %d, got %d", expected16, vram16)
	}

	gb192 := uint64(192 * 1024 * 1024 * 1024)
	vram192 := AppleSiliconMetalVRAM(gb192)
	expected192 := uint64(float64(gb192) * 0.92)
	if vram192 != expected192 {
		t.Errorf("expected 192GB VRAM %d, got %d", expected192, vram192)
	}
}

func TestWindowsGPUDiscoveryPerformance(t *testing.T) {
	start := time.Now()
	specs, err := DetectHardware()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("DetectHardware failed: %v", err)
	}

	t.Logf("Detected: OS=%s, GPU=%s, VRAM=%d bytes (%d MB), RAM=%d bytes, took %v",
		specs.OS, specs.GPU.Name, specs.GPU.VRAM, specs.GPU.VRAM/(1024*1024), specs.RAM.Total, elapsed)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Hardware detection too slow: took %v (expected < 100ms)", elapsed)
	}
}
