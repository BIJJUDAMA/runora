package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BIJJUDAMA/runora/config"
)

type Profile struct {
	Name           string `json:"name"`
	Context        uint32 `json:"context"`
	Threads        int    `json:"threads"`
	GPULayers      int    `json:"gpu_layers"`
	BatchSize      int    `json:"batch_size"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	FlashAttention bool   `json:"flash_attention"`
	CacheTypeK     string `json:"cache_type_k,omitempty"`
	CacheTypeV     string `json:"cache_type_v,omitempty"`
	CustomArgs     string `json:"custom_args,omitempty"`
}

var reservedWindowsNames = map[string]bool{
	"CON":  true,
	"PRN":  true,
	"AUX":  true,
	"NUL":  true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// IsReservedWindowsName checks if a name matches any Windows reserved device names (CON, PRN, AUX, NUL, COM1-9, LPT1-9).
func IsReservedWindowsName(name string) bool {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return false
	}
	base := clean
	if idx := strings.Index(base, "."); idx != -1 {
		base = base[:idx]
	}
	base = strings.ToUpper(strings.TrimSpace(base))
	return reservedWindowsNames[base]
}

// SanitizeProfileName removes invalid filename characters and avoids reserved Windows device names.
func SanitizeProfileName(name string) string {
	cleanName := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))

	if IsReservedWindowsName(cleanName) {
		cleanName = cleanName + "_profile"
	}
	return cleanName
}

// Validate ensures profile configuration parameters conform to valid system ranges and constraints.
func (p *Profile) Validate() error {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if IsReservedWindowsName(name) {
		return fmt.Errorf("profile name %q is a reserved Windows device name", name)
	}
	if strings.ContainsAny(name, `\/:*?"<>|`) {
		return fmt.Errorf("profile name contains invalid filename characters")
	}
	if p.Context < 256 {
		return fmt.Errorf("context size must be at least 256, got %d", p.Context)
	}
	if p.GPULayers < 0 || p.GPULayers > 999 {
		return fmt.Errorf("GPU layers must be between 0 and 999, got %d", p.GPULayers)
	}
	if p.Threads < 1 {
		return fmt.Errorf("threads must be at least 1, got %d", p.Threads)
	}
	if p.BatchSize != 0 && (p.BatchSize < 1 || p.BatchSize > 8192) {
		return fmt.Errorf("batch size must be between 1 and 8192, got %d", p.BatchSize)
	}
	if strings.TrimSpace(p.Host) == "" {
		p.Host = "127.0.0.1"
	}
	if p.Port < 1024 || p.Port > 65535 {
		return fmt.Errorf("port must be between 1024 and 65535, got %d", p.Port)
	}
	return nil
}

// DefaultProfiles generates the default profiles based on system cpu count.
func DefaultProfiles() []*Profile {
	threads := runtime.NumCPU() / 2
	if threads < 1 {
		threads = 1
	}

	return []*Profile{
		{
			Name:           "Fast",
			Context:        2048,
			Threads:        threads,
			GPULayers:      999,
			BatchSize:      512,
			Host:           "127.0.0.1",
			Port:           50505,
			FlashAttention: true,
		},
		{
			Name:           "Balanced",
			Context:        4096,
			Threads:        threads,
			GPULayers:      999,
			BatchSize:      512,
			Host:           "127.0.0.1",
			Port:           50505,
			FlashAttention: true,
		},
		{
			Name:           "High",
			Context:        8192,
			Threads:        threads,
			GPULayers:      999,
			BatchSize:      512,
			Host:           "127.0.0.1",
			Port:           50505,
			FlashAttention: true,
		},
		{
			Name:           "Long Context",
			Context:        16384,
			Threads:        threads,
			GPULayers:      999,
			BatchSize:      512,
			Host:           "127.0.0.1",
			Port:           50505,
			FlashAttention: true,
		},
		{
			Name:           "CPU",
			Context:        2048,
			Threads:        runtime.NumCPU(),
			GPULayers:      0,
			BatchSize:      512,
			Host:           "127.0.0.1",
			Port:           50505,
			FlashAttention: true,
		},
	}
}

// LoadAll reads all profiles from the specified profiles directory, 
// auto-generating defaults only if no profile files exist.
func LoadAll(profilesDir string) ([]*Profile, error) {
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		return nil, err
	}

	files, err := os.ReadDir(profilesDir)
	if err != nil {
		return nil, err
	}

	hasJSON := false
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
			hasJSON = true
			break
		}
	}

	// Auto-seed default profiles only if directory has no profiles at all
	if !hasJSON {
		defaults := DefaultProfiles()
		for _, p := range defaults {
			cleanName := SanitizeProfileName(p.Name)
			fileName := strings.ReplaceAll(strings.ToLower(cleanName), " ", "_") + ".json"
			filePath := filepath.Join(profilesDir, fileName)
			data, err := json.MarshalIndent(p, "", "  ")
			if err == nil {
				_ = config.AtomicWriteFile(filePath, data, 0644)
			}
		}
		files, _ = os.ReadDir(profilesDir)
	}

	var profiles []*Profile
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
			filePath := filepath.Join(profilesDir, file.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			var p Profile
			if err := json.Unmarshal(data, &p); err == nil {
				if err := p.Validate(); err == nil {
					profiles = append(profiles, &p)
				}
			}
		}
	}

	return profiles, nil
}

// IsDefaultProfile returns true if the profile name matches one of the built-in default profiles.
func IsDefaultProfile(name string) bool {
	clean := strings.ToLower(strings.TrimSpace(name))
	defaults := []string{"fast", "balanced", "high", "long context", "cpu"}
	for _, d := range defaults {
		if clean == d {
			return true
		}
	}
	return false
}

// SaveProfile writes a profile to JSON format in the specified profiles directory.
func SaveProfile(profilesDir string, p *Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		return err
	}
	cleanName := SanitizeProfileName(p.Name)
	fileName := strings.ReplaceAll(strings.ToLower(cleanName), " ", "_") + ".json"
	filePath := filepath.Join(profilesDir, fileName)

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return config.AtomicWriteFile(filePath, data, 0644)
}

// DeleteProfile deletes any profile file from the specified profiles directory.
func DeleteProfile(profilesDir string, name string) error {
	cleanName := SanitizeProfileName(name)
	fileName := strings.ReplaceAll(strings.ToLower(cleanName), " ", "_") + ".json"
	targetPath := filepath.Join(profilesDir, fileName)

	// Remove by filename if exists
	if _, err := os.Stat(targetPath); err == nil {
		_ = os.Remove(targetPath)
		return nil
	}

	// Also search for any JSON file whose internal profile Name matches
	files, err := os.ReadDir(profilesDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if !f.IsDir() && filepath.Ext(f.Name()) == ".json" {
			filePath := filepath.Join(profilesDir, f.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			var p Profile
			if err := json.Unmarshal(data, &p); err == nil {
				if strings.EqualFold(strings.TrimSpace(p.Name), strings.TrimSpace(name)) {
					_ = os.Remove(filePath)
					return nil
				}
			}
		}
	}
	return nil
}

