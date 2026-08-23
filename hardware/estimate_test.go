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

	t.Logf("Detected: OS=%s, GPU=%s, VRAM=%d bytes (%d MB), RAM=%d bytes, Threads=%d, PhysicalCores=%d, GPUsCount=%d, took %v",
		specs.OS, specs.GPU.Name, specs.GPU.VRAM, specs.GPU.VRAM/(1024*1024), specs.RAM.Total, specs.CPU.Threads, specs.CPU.PhysicalCores, len(specs.GPUs), elapsed)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Hardware detection too slow: took %v (expected < 100ms)", elapsed)
	}

	if specs.CPU.Threads <= 0 {
		t.Errorf("expected CPU threads > 0, got %d", specs.CPU.Threads)
	}
	if specs.CPU.PhysicalCores <= 0 {
		t.Errorf("expected CPU physical cores > 0, got %d", specs.CPU.PhysicalCores)
	}
	if len(specs.GPUs) == 0 {
		t.Errorf("expected at least 1 GPU in specs.GPUs")
	}
}

func TestQuantizedKVCacheMath(t *testing.T) {
	// 1. Verify multipliers
	if m := KVCacheMultiplier("f16"); m != 1.0 {
		t.Errorf("expected FP16 multiplier 1.0, got %f", m)
	}
	if m := KVCacheMultiplier("fp16"); m != 1.0 {
		t.Errorf("expected FP16 multiplier 1.0, got %f", m)
	}
	if m := KVCacheMultiplier("q8_0"); m != 0.56 {
		t.Errorf("expected Q8_0 multiplier 0.56, got %f", m)
	}
	if m := KVCacheMultiplier("q4_0"); m != 0.28 {
		t.Errorf("expected Q4_0 multiplier 0.28, got %f", m)
	}
	if m := KVCacheMultiplier("fp8"); m != 0.50 {
		t.Errorf("expected FP8 multiplier 0.50, got %f", m)
	}

	meta := &model.GGUFMetadata{
		FileSize:     4 * 1024 * 1024 * 1024,
		Layers:       32,
		Heads:        32,
		HeadsKV:      8,
		EmbeddingLen: 4096,
		HeadDim:      128,
	}

	// 2. Base FP16 KV size: 4 * 8 * 128 * 32 * 4096 = 536,870,912
	baseKV := uint64(4) * 8 * 128 * 32 * 4096
	kvFP16 := ComputeKVCacheSize(meta, 4096, "fp16")
	if kvFP16 != baseKV {
		t.Errorf("expected FP16 KV size %d, got %d", baseKV, kvFP16)
	}

	// 3. Q8_0 KV size (0.56x)
	expectedQ8 := uint64(float64(baseKV) * 0.56)
	kvQ8 := ComputeKVCacheSize(meta, 4096, "q8_0")
	if kvQ8 != expectedQ8 {
		t.Errorf("expected Q8_0 KV size %d, got %d", expectedQ8, kvQ8)
	}

	// 4. Q4_0 KV size (0.28x)
	expectedQ4 := uint64(float64(baseKV) * 0.28)
	kvQ4 := ComputeKVCacheSize(meta, 4096, "q4_0")
	if kvQ4 != expectedQ4 {
		t.Errorf("expected Q4_0 KV size %d, got %d", expectedQ4, kvQ4)
	}

	// 5. FP8 KV size (0.50x)
	expectedFP8 := uint64(float64(baseKV) * 0.50)
	kvFP8 := ComputeKVCacheSize(meta, 4096, "fp8")
	if kvFP8 != expectedFP8 {
		t.Errorf("expected FP8 KV size %d, got %d", expectedFP8, kvFP8)
	}

	// 6. Test with EstimateMemoryWithKVQuant
	specs := &HardwareSpecs{
		GPU: GPUSpecs{
			Name: "NVIDIA RTX 4070",
			VRAM: 12 * 1024 * 1024 * 1024,
			Type: "CUDA",
		},
		RAM: RAMSpecs{
			Total: 32 * 1024 * 1024 * 1024,
		},
	}
	estQ4 := EstimateMemoryWithKVQuant(meta, specs, 4096, "q4_0")
	if estQ4.KVCacheSize != expectedQ4 {
		t.Errorf("expected estimate Q4 KV size %d, got %d", expectedQ4, estQ4.KVCacheSize)
	}
	if estQ4.KVQuant != "q4_0" {
		t.Errorf("expected KVQuant q4_0, got %s", estQ4.KVQuant)
	}
	if estQ4.KVCacheMultiplier != 0.28 {
		t.Errorf("expected multiplier 0.28, got %f", estQ4.KVCacheMultiplier)
	}
}

func TestExactGPULayers(t *testing.T) {
	meta := &model.GGUFMetadata{
		FileSize:     4 * 1024 * 1024 * 1024, // 4GB
		Layers:       32,
		Heads:        32,
		HeadsKV:      8,
		EmbeddingLen: 4096,
		HeadDim:      128,
	}

	// Case 1: 100% offload in 8GB VRAM (all 32 layers fit)
	specsFull := &HardwareSpecs{
		GPU: GPUSpecs{
			Name: "RTX 4070",
			VRAM: 8 * 1024 * 1024 * 1024,
			Type: "CUDA",
		},
	}
	layersFull := ExactGPULayers(meta, specsFull, 2048)
	if layersFull != 32 {
		t.Errorf("expected 32 layers offloaded, got %d", layersFull)
	}

	// Case 2: Partial offload in 2GB VRAM
	// Fixed: ~268MB (overhead + activations), leaving ~1.73GB for layers (~133.4MB/layer -> ~13-14 layers)
	specsPartial := &HardwareSpecs{
		GPU: GPUSpecs{
			Name: "GTX 1050",
			VRAM: 2 * 1024 * 1024 * 1024,
			Type: "CUDA",
		},
	}
	layersPartial := ExactGPULayers(meta, specsPartial, 2048)
	if layersPartial < 10 || layersPartial > 20 {
		t.Errorf("expected partial layer offload between 10 and 20, got %d", layersPartial)
	}

	// Case 3: Insufficient VRAM (200MB < fixed overhead) -> 0 layers
	specsTiny := &HardwareSpecs{
		GPU: GPUSpecs{
			Name: "Low VRAM",
			VRAM: 200 * 1024 * 1024,
			Type: "CUDA",
		},
	}
	layersZero := ExactGPULayers(meta, specsTiny, 2048)
	if layersZero != 0 {
		t.Errorf("expected 0 layers offloaded for tiny VRAM, got %d", layersZero)
	}

	// Case 4: Quantized KV Cache increases layer capacity
	layersFP16 := ExactGPULayers(meta, specsPartial, 8192, "fp16")
	layersQ4 := ExactGPULayers(meta, specsPartial, 8192, "q4_0")
	if layersQ4 < layersFP16 {
		t.Errorf("expected Q4_0 to allow equal or more layers than FP16 (got FP16=%d, Q4=%d)", layersFP16, layersQ4)
	}

	// Case 5: Edge cases
	if l := ExactGPULayers(nil, specsFull, 2048); l != 0 {
		t.Errorf("expected 0 layers for nil meta, got %d", l)
	}
	if l := ExactGPULayers(meta, nil, 2048); l != 0 {
		t.Errorf("expected 0 layers for nil specs, got %d", l)
	}
	if l := ExactGPULayersForVRAM(meta, 0, 2048); l != 0 {
		t.Errorf("expected 0 layers for 0 VRAM, got %d", l)
	}
}

func TestTensorSplitAdvisor(t *testing.T) {
	// Case 1: 2 GPUs with 24GB and 8GB -> "3,1"
	gpus1 := []GPUSpecs{
		{Name: "RTX 4090", VRAM: 24 * 1024 * 1024 * 1024},
		{Name: "RTX 4060", VRAM: 8 * 1024 * 1024 * 1024},
	}
	if ratio := TensorSplitAdvisor(gpus1); ratio != "3,1" {
		t.Errorf("expected ratio '3,1', got %q", ratio)
	}

	// Case 2: 2 GPUs with 16GB and 8GB -> "2,1"
	gpus2 := []GPUSpecs{
		{Name: "RTX 4080", VRAM: 16 * 1024 * 1024 * 1024},
		{Name: "RTX 4060", VRAM: 8 * 1024 * 1024 * 1024},
	}
	if ratio := TensorSplitAdvisor(gpus2); ratio != "2,1" {
		t.Errorf("expected ratio '2,1', got %q", ratio)
	}

	// Case 3: 2 GPUs with 12GB and 8GB -> "3,2"
	gpus3 := []GPUSpecs{
		{Name: "RTX 3060", VRAM: 12 * 1024 * 1024 * 1024},
		{Name: "RTX 3070", VRAM: 8 * 1024 * 1024 * 1024},
	}
	if ratio := TensorSplitAdvisor(gpus3); ratio != "3,2" {
		t.Errorf("expected ratio '3,2', got %q", ratio)
	}

	// Case 4: 2 GPUs identical 16GB each -> "1,1"
	gpus4 := []GPUSpecs{
		{Name: "RTX 4080", VRAM: 16 * 1024 * 1024 * 1024},
		{Name: "RTX 4080", VRAM: 16 * 1024 * 1024 * 1024},
	}
	if ratio := TensorSplitAdvisor(gpus4); ratio != "1,1" {
		t.Errorf("expected ratio '1,1', got %q", ratio)
	}

	// Case 5: 3 GPUs with 24GB, 24GB, 12GB -> "2,2,1"
	gpus5 := []GPUSpecs{
		{Name: "RTX 3090 #1", VRAM: 24 * 1024 * 1024 * 1024},
		{Name: "RTX 3090 #2", VRAM: 24 * 1024 * 1024 * 1024},
		{Name: "RTX 3060", VRAM: 12 * 1024 * 1024 * 1024},
	}
	if ratio := TensorSplitAdvisor(gpus5); ratio != "2,2,1" {
		t.Errorf("expected ratio '2,2,1', got %q", ratio)
	}

	// Case 6: 1 GPU -> ""
	gpusSingle := []GPUSpecs{
		{Name: "RTX 4090", VRAM: 24 * 1024 * 1024 * 1024},
	}
	if ratio := TensorSplitAdvisor(gpusSingle); ratio != "" {
		t.Errorf("expected empty ratio for 1 GPU, got %q", ratio)
	}

	// Case 7: Specs method
	specs := &HardwareSpecs{GPUs: gpus1}
	if ratio := specs.TensorSplit(); ratio != "3,1" {
		t.Errorf("expected specs.TensorSplit() '3,1', got %q", ratio)
	}
}

func TestPhysicalCoresAndThreadTuning(t *testing.T) {
	// Case 1: Physical cores available (16 threads, 8 physical)
	specs1 := &HardwareSpecs{
		CPU: CPUSpecs{
			Threads:       16,
			PhysicalCores: 8,
		},
	}
	if rec := specs1.RecommendedThreads(); rec != 8 {
		t.Errorf("expected recommended threads 8, got %d", rec)
	}

	// Case 2: Only threads available (12 threads -> recommend 6)
	specs2 := &HardwareSpecs{
		CPU: CPUSpecs{
			Threads:       12,
			PhysicalCores: 0,
		},
	}
	if rec := specs2.RecommendedThreads(); rec != 6 {
		t.Errorf("expected recommended threads 6, got %d", rec)
	}

	// Case 3: Small core system (2 threads -> recommend 2)
	specs3 := &HardwareSpecs{
		CPU: CPUSpecs{
			Threads:       2,
			PhysicalCores: 0,
		},
	}
	if rec := specs3.RecommendedThreads(); rec != 2 {
		t.Errorf("expected recommended threads 2, got %d", rec)
	}
}

func TestMultiGPUSpecs(t *testing.T) {
	specs := &HardwareSpecs{
		GPUs: []GPUSpecs{
			{Name: "NVIDIA RTX 4090", VRAM: 24 * 1024 * 1024 * 1024, Type: "CUDA"},
			{Name: "NVIDIA RTX 4080", VRAM: 16 * 1024 * 1024 * 1024, Type: "CUDA"},
		},
	}
	specs.GPU = specs.GPUs[0]

	if specs.GPUCount() != 2 {
		t.Errorf("expected GPUCount 2, got %d", specs.GPUCount())
	}
	expectedTotal := uint64((24 + 16) * 1024 * 1024 * 1024)
	if specs.TotalVRAM() != expectedTotal {
		t.Errorf("expected TotalVRAM %d, got %d", expectedTotal, specs.TotalVRAM())
	}
	if specs.PrimaryGPU().Name != "NVIDIA RTX 4090" {
		t.Errorf("expected PrimaryGPU 'NVIDIA RTX 4090', got %q", specs.PrimaryGPU().Name)
	}
}

func TestMemoryEstimateBoundaryAndZeroValues(t *testing.T) {
	specs := &HardwareSpecs{
		OS: "Windows",
		RAM: RAMSpecs{
			Total:     32 * 1024 * 1024 * 1024,
			Available: 24 * 1024 * 1024 * 1024,
		},
		GPU: GPUSpecs{
			Name: "RTX 4070",
			VRAM: 12 * 1024 * 1024 * 1024,
			Type: "CUDA",
		},
	}

	// 1. Model with zero layers and heads (fallback heuristics should not divide by zero or panic)
	zeroMeta := &model.GGUFMetadata{
		FileSize:     4 * 1024 * 1024 * 1024,
		Layers:       0,
		Heads:        0,
		HeadsKV:      0,
		EmbeddingLen: 0,
	}
	estZero := EstimateMemory(zeroMeta, specs, 4096)
	if estZero.TotalMemory == 0 {
		t.Errorf("expected non-zero total memory estimate even with zero metadata fields")
	}
	if estZero.Suitability == SuitabilityExceeds {
		t.Errorf("4GB model on 12GB VRAM should fit VRAM, got %d", estZero.Suitability)
	}

	// 2. Extreme context size (128k tokens)
	metaLarge := &model.GGUFMetadata{
		FileSize:     8 * 1024 * 1024 * 1024,
		Layers:       32,
		Heads:        32,
		HeadsKV:      8,
		EmbeddingLen: 4096,
	}
	est128k := EstimateMemory(metaLarge, specs, 131072)
	if est128k.KVCacheSize <= estZero.KVCacheSize {
		t.Errorf("128k context KV cache size (%d) should be significantly larger than 4k (%d)", est128k.KVCacheSize, estZero.KVCacheSize)
	}

	// 3. Zero RAM / Zero VRAM specs (should classify as Exceeds or CPU-only without crashing)
	zeroSpecs := &HardwareSpecs{}
	estNoHW := EstimateMemory(metaLarge, zeroSpecs, 2048)
	if estNoHW.Suitability != SuitabilityExceeds {
		t.Errorf("expected SuitabilityExceeds for zero hardware specs, got %d", estNoHW.Suitability)
	}
}

func TestMultiGPUTensorSplitAdvisorEdgeCases(t *testing.T) {
	// 1. 3-GPU setup: 16GB, 12GB, 8GB (GCD = 4GB -> "4,3,2")
	gpus3 := []GPUSpecs{
		{Name: "GPU 1", VRAM: 16 * 1024 * 1024 * 1024},
		{Name: "GPU 2", VRAM: 12 * 1024 * 1024 * 1024},
		{Name: "GPU 3", VRAM: 8 * 1024 * 1024 * 1024},
	}
	if ratio := TensorSplitAdvisor(gpus3); ratio != "4,3,2" {
		t.Errorf("expected ratio '4,3,2', got %q", ratio)
	}

	// 2. 4 identical GPUs: 8GB each -> "1,1,1,1"
	gpus4Identical := []GPUSpecs{
		{Name: "GPU 1", VRAM: 8 * 1024 * 1024 * 1024},
		{Name: "GPU 2", VRAM: 8 * 1024 * 1024 * 1024},
		{Name: "GPU 3", VRAM: 8 * 1024 * 1024 * 1024},
		{Name: "GPU 4", VRAM: 8 * 1024 * 1024 * 1024},
	}
	if ratio := TensorSplitAdvisor(gpus4Identical); ratio != "1,1,1,1" {
		t.Errorf("expected ratio '1,1,1,1', got %q", ratio)
	}

	// 3. One GPU with 0 VRAM -> should compute "1,0" to allocate all weights to GPU 1
	gpusWithZero := []GPUSpecs{
		{Name: "GPU 1", VRAM: 16 * 1024 * 1024 * 1024},
		{Name: "GPU 2 (Display Only)", VRAM: 0},
	}
	ratio := TensorSplitAdvisor(gpusWithZero)
	if ratio != "1,0" {
		t.Errorf("expected ratio '1,0' for 16GB + 0GB, got %q", ratio)
	}
}

