package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// LlamaCppRuntime coordinates llama.cpp model execution through the ProcessSupervisor and LlamaCppDriver.
type LlamaCppRuntime struct {
	supervisor *ProcessSupervisor
	driver     *LlamaCppDriver
}

func NewLlamaCppRuntime(logDir string) *LlamaCppRuntime {
	return &LlamaCppRuntime{
		supervisor: NewProcessSupervisor(logDir),
		driver:     NewLlamaCppDriver(),
	}
}

func NewLlamaCppRuntimeWithSupervisor(supervisor *ProcessSupervisor) *LlamaCppRuntime {
	return &LlamaCppRuntime{
		supervisor: supervisor,
		driver:     NewLlamaCppDriver(),
	}
}

// Start launches the llama-server on the specified port.
func (sr *LlamaCppRuntime) Start(modelPath string, opts StartOptions) error {
	_, err := sr.supervisor.StartInstance(sr.driver, modelPath, opts)
	return err
}

// Stop terminates all running llama.cpp servers.
func (sr *LlamaCppRuntime) Stop() error {
	return sr.supervisor.Stop()
}

// StopInstance terminates the server running on the specified port.
func (sr *LlamaCppRuntime) StopInstance(port int) error {
	return sr.supervisor.StopInstance(port)
}

// GetStatus returns the status, running model path, and port of the primary running server.
func (sr *LlamaCppRuntime) GetStatus() (ServerStatus, string, int) {
	return sr.supervisor.GetStatus()
}

// GetAllInstances returns status information for all active servers.
func (sr *LlamaCppRuntime) GetAllInstances() []InstanceInfo {
	return sr.supervisor.GetAllInstances()
}

func (sr *LlamaCppRuntime) Capabilities() []TaskType {
	return sr.driver.Capabilities()
}

// GetMemoryUsage queries physical memory usage (RSS) of a process in MB.
func GetMemoryUsage(pid int) (float64, error) {
	if pid <= 0 {
		return 0, fmt.Errorf("invalid pid")
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return 0, err
		}
		lines := strings.Split(out.String(), "\n")
		for _, line := range lines {
			if strings.Contains(line, fmt.Sprintf(`"%d"`, pid)) || strings.Contains(line, fmt.Sprintf(`,%d,`, pid)) {
				parts := strings.Split(line, ",")
				if len(parts) >= 5 {
					memStr := strings.Trim(parts[4], ` "`)
					memStr = strings.ReplaceAll(memStr, ",", "")
					memStr = strings.ReplaceAll(memStr, ".", "")
					memStr = strings.TrimSuffix(memStr, " K")
					memStr = strings.TrimSuffix(memStr, " KB")
					var kb float64
					if _, err := fmt.Sscanf(memStr, "%f", &kb); err == nil {
						return kb / 1024.0, nil
					}
				}
			}
		}
		return 0, fmt.Errorf("process not found in tasklist")
	} else {
		cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "rss=")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return 0, err
		}
		var kb float64
		if _, err := fmt.Sscanf(strings.TrimSpace(out.String()), "%f", &kb); err == nil {
			return kb / 1024.0, nil
		}
		return 0, fmt.Errorf("failed to parse ps output")
	}
}

// QueryServerRequests queries total completion requests processed via the /metrics endpoint.
func QueryServerRequests(port int) (int, error) {
	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "llama_requests_total") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				var count int
				if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &count); err == nil {
					return count, nil
				}
			}
		}
	}
	return 0, nil
}

// SlotData represents context slot details returned by llama-server /slots endpoint.
type SlotData struct {
	ID           int  `json:"id"`
	IDTask       int  `json:"id_task"`
	NCtx         int  `json:"n_ctx"`
	NPast        int  `json:"n_past"`
	IsProcessing bool `json:"is_processing"`
	State        int  `json:"state"`
}

// ServerSlotMetrics summarizes context window utilization across all slots.
type ServerSlotMetrics struct {
	TotalNCtx    int     `json:"total_n_ctx"`
	TotalNPast   int     `json:"total_n_past"`
	PctUsed      float64 `json:"pct_used"`
	ActiveSlots  int     `json:"active_slots"`
	TotalSlots   int     `json:"total_slots"`
	TokensPerSec float64 `json:"tokens_per_sec"`
}

// QueryServerSlots queries the /slots endpoint from llama-server to calculate live context window utilization.
func QueryServerSlots(port int) (*ServerSlotMetrics, error) {
	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/slots", port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("slots endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var slots []SlotData
	if err := json.Unmarshal(body, &slots); err != nil {
		var objWrapper struct {
			Slots []SlotData `json:"slots"`
		}
		if err2 := json.Unmarshal(body, &objWrapper); err2 == nil {
			slots = objWrapper.Slots
		} else {
			return nil, fmt.Errorf("failed to parse /slots JSON: %w", err)
		}
	}

	if len(slots) == 0 {
		return nil, fmt.Errorf("no slots returned")
	}

	totalNCtx := 0
	totalNPast := 0
	activeSlots := 0
	for _, slot := range slots {
		totalNCtx += slot.NCtx
		totalNPast += slot.NPast
		if slot.IsProcessing || slot.NPast > 0 || slot.State > 0 {
			activeSlots++
		}
	}

	pct := 0.0
	if totalNCtx > 0 {
		pct = (float64(totalNPast) / float64(totalNCtx)) * 100.0
	}

	return &ServerSlotMetrics{
		TotalNCtx:   totalNCtx,
		TotalNPast:  totalNPast,
		PctUsed:     pct,
		ActiveSlots: activeSlots,
		TotalSlots:  len(slots),
	}, nil
}

// QueryServerTokens queries total prompt and predicted tokens processed via the /metrics endpoint.
func QueryServerTokens(port int) (int, error) {
	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", port))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(body), "\n")
	totalTokens := 0
	found := false
	for _, line := range lines {
		if strings.HasPrefix(line, "llama_tokens_predicted_total") || strings.HasPrefix(line, "llama_tokens_evaluated_total") || strings.HasPrefix(line, "llama_prompt_tokens_total") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				var count int
				if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &count); err == nil {
					totalTokens += count
					found = true
				}
			}
		}
	}
	if found {
		return totalTokens, nil
	}
	return 0, nil
}
