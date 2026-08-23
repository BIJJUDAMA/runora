package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileDefaultsAndLoading(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-profiles-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// LoadAll on empty folder should populate default JSON files
	profiles, err := LoadAll(tempDir)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(profiles) != 5 {
		t.Errorf("expected 5 default profiles, got %d", len(profiles))
	}

	// Verify that files are written
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	if len(files) != 5 {
		t.Errorf("expected 5 JSON files in directory, got %d", len(files))
	}

	// Verify custom profile loading
	customProfile := &Profile{
		Name:      "Coding Special",
		Context:   8192,
		Threads:   4,
		GPULayers: 50,
		BatchSize: 256,
		Host:      "127.0.0.1",
		Port:      9090,
	}

	customData, err := json.Marshal(customProfile)
	if err != nil {
		t.Fatalf("failed to marshal custom profile: %v", err)
	}

	err = os.WriteFile(filepath.Join(tempDir, "coding_special.json"), customData, 0644)
	if err != nil {
		t.Fatalf("failed to write custom profile file: %v", err)
	}

	// Re-load all profiles (should load 5 defaults + 1 custom)
	reloaded, err := LoadAll(tempDir)
	if err != nil {
		t.Fatalf("second LoadAll failed: %v", err)
	}

	if len(reloaded) != 6 {
		t.Errorf("expected 6 profiles (5 defaults + 1 custom), got %d", len(reloaded))
	}

	foundCustom := false
	for _, p := range reloaded {
		if p.Name == "Coding Special" {
			foundCustom = true
			if p.Context != 8192 || p.Port != 9090 || p.GPULayers != 50 {
				t.Errorf("incorrect custom profile content parsed: %+v", p)
			}
		}
	}
	if !foundCustom {
		t.Errorf("custom profile 'Coding Special' was not loaded")
	}
}

func TestProfileValidation(t *testing.T) {
	validProfile := &Profile{
		Name:      "Valid Profile",
		Context:   2048,
		Threads:   4,
		GPULayers: 33,
		BatchSize: 512,
		Host:      "127.0.0.1",
		Port:      8080,
	}

	if err := validProfile.Validate(); err != nil {
		t.Errorf("expected valid profile to pass validation, got: %v", err)
	}

	// Empty name
	pEmptyName := *validProfile
	pEmptyName.Name = "   "
	if err := pEmptyName.Validate(); err == nil {
		t.Errorf("expected empty name to fail validation")
	}

	// Reserved Windows name
	for _, reserved := range []string{"CON", "con", "prn", "AUX", "Nul", "com1", "COM9", "lpt1", "LPT9"} {
		pReserved := *validProfile
		pReserved.Name = reserved
		if err := pReserved.Validate(); err == nil {
			t.Errorf("expected reserved name %q to fail validation", reserved)
		}
	}

	// Invalid characters
	pInvalidChars := *validProfile
	pInvalidChars.Name = "Invalid/Name:Test"
	if err := pInvalidChars.Validate(); err == nil {
		t.Errorf("expected invalid characters to fail validation")
	}

	// Context < 256
	pLowCtx := *validProfile
	pLowCtx.Context = 128
	if err := pLowCtx.Validate(); err == nil {
		t.Errorf("expected context < 256 to fail validation")
	}

	// GPU Layers out of range [0..999]
	pNegativeGPU := *validProfile
	pNegativeGPU.GPULayers = -1
	if err := pNegativeGPU.Validate(); err == nil {
		t.Errorf("expected GPU layers < 0 to fail validation")
	}
	pHighGPU := *validProfile
	pHighGPU.GPULayers = 1000
	if err := pHighGPU.Validate(); err == nil {
		t.Errorf("expected GPU layers > 999 to fail validation")
	}

	// Threads < 1
	pZeroThreads := *validProfile
	pZeroThreads.Threads = 0
	if err := pZeroThreads.Validate(); err == nil {
		t.Errorf("expected threads < 1 to fail validation")
	}

	// Port out of range [1024..65535]
	pLowPort := *validProfile
	pLowPort.Port = 80
	if err := pLowPort.Validate(); err == nil {
		t.Errorf("expected port < 1024 to fail validation")
	}
	pHighPort := *validProfile
	pHighPort.Port = 70000
	if err := pHighPort.Validate(); err == nil {
		t.Errorf("expected port > 65535 to fail validation")
	}
}

func TestReservedWindowsNames(t *testing.T) {
	if !IsReservedWindowsName("CON") {
		t.Errorf("expected CON to be reserved")
	}
	if !IsReservedWindowsName("con.json") {
		t.Errorf("expected con.json to be reserved")
	}
	if !IsReservedWindowsName("aux") {
		t.Errorf("expected aux to be reserved")
	}
	if !IsReservedWindowsName("COM5") {
		t.Errorf("expected COM5 to be reserved")
	}
	if !IsReservedWindowsName("lpt3") {
		t.Errorf("expected lpt3 to be reserved")
	}
	if IsReservedWindowsName("normal_profile") {
		t.Errorf("expected normal_profile to not be reserved")
	}
}

func TestSanitizeProfileName(t *testing.T) {
	sanitized := SanitizeProfileName("CON")
	if sanitized != "CON_profile" {
		t.Errorf("expected 'CON_profile', got %q", sanitized)
	}

	sanitizedChars := SanitizeProfileName("test/profile:name*")
	if sanitizedChars != "test_profile_name_" {
		t.Errorf("expected 'test_profile_name_', got %q", sanitizedChars)
	}
}
