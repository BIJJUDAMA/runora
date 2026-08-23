package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/model"
)

type DownloaderFocus int

const (
	FocusURL DownloaderFocus = iota
	FocusFilename
	FocusQueue
	FocusFileList
)

type CuratedQuickPick struct {
	Index       string
	Name        string
	Size        string
	RepoID      string
	FileName    string
	DownloadURL string
}

var CuratedQuickPicks = []CuratedQuickPick{
	{
		Index:       "1",
		Name:        "1. Llama 3.1 8B Instruct (Q4_K_M)",
		Size:        "4.9 GB",
		RepoID:      "bartowski/Meta-Llama-3.1-8B-Instruct-GGUF",
		FileName:    "Meta-Llama-3.1-8B-Instruct-Q4_K_M.gguf",
		DownloadURL: "https://huggingface.co/bartowski/Meta-Llama-3.1-8B-Instruct-GGUF/resolve/main/Meta-Llama-3.1-8B-Instruct-Q4_K_M.gguf",
	},
	{
		Index:       "2",
		Name:        "2. Qwen 2.5 7B Instruct (Q4_K_M)",
		Size:        "4.7 GB",
		RepoID:      "Qwen/Qwen2.5-7B-Instruct-GGUF",
		FileName:    "qwen2.5-7b-instruct-q4_k_m.gguf",
		DownloadURL: "https://huggingface.co/Qwen/Qwen2.5-7B-Instruct-GGUF/resolve/main/qwen2.5-7b-instruct-q4_k_m.gguf",
	},
	{
		Index:       "3",
		Name:        "3. DeepSeek Coder 6.7B (Q4_K_M)",
		Size:        "4.1 GB",
		RepoID:      "TheBloke/deepseek-coder-6.7B-instruct-GGUF",
		FileName:    "deepseek-coder-6.7b-instruct.Q4_K_M.gguf",
		DownloadURL: "https://huggingface.co/TheBloke/deepseek-coder-6.7B-instruct-GGUF/resolve/main/deepseek-coder-6.7b-instruct.Q4_K_M.gguf",
	},
	{
		Index:       "4",
		Name:        "4. Mistral Nemo 12B (Q4_K_M)",
		Size:        "7.5 GB",
		RepoID:      "bartowski/Mistral-Nemo-Instruct-2407-GGUF",
		FileName:    "Mistral-Nemo-Instruct-2407-Q4_K_M.gguf",
		DownloadURL: "https://huggingface.co/bartowski/Mistral-Nemo-Instruct-2407-GGUF/resolve/main/Mistral-Nemo-Instruct-2407-Q4_K_M.gguf",
	},
}

type DownloaderModel struct {
	config          *config.Config
	queue           *model.DownloadQueue
	focus           DownloaderFocus
	selectedTaskIdx int
	err             error
	resolving       bool

	urlInput      textinput.Model
	filenameInput textinput.Model

	resolvedFiles   []model.HFSibling
	selectedFileIdx int
	repoID          string
}

func NewDownloaderModel(cfg *config.Config, q *model.DownloadQueue) *DownloaderModel {
	urlTi := textinput.New()
	urlTi.Placeholder = "Paste direct GGUF/ONNX model download URL (http/https)..."
	urlTi.CharLimit = 512
	urlTi.Width = 60
	urlTi.Focus()

	fileTi := textinput.New()
	fileTi.Placeholder = "Enter local filename (optional, e.g. model.gguf or model.onnx)..."
	fileTi.CharLimit = 156
	fileTi.Width = 60

	return &DownloaderModel{
		config:        cfg,
		queue:         q,
		focus:         FocusURL,
		urlInput:      urlTi,
		filenameInput: fileTi,
	}
}

func (m *DownloaderModel) Update(msg tea.Msg) (*DownloaderModel, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch m.focus {
	case FocusURL:
		m.urlInput, cmd = m.urlInput.Update(msg)
		cmds = append(cmds, cmd)
	case FocusFilename:
		m.filenameInput, cmd = m.filenameInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case hfResolveMsg:
		m.resolving = false
		if msg.err != nil {
			m.queue.AddFailedTask(msg.repoID, "Hugging Face Repo", fmt.Errorf("failed to fetch Hugging Face repo info: %v", msg.err))
			m.focus = FocusURL
			m.urlInput.Focus()
			m.selectedTaskIdx = len(m.queue.GetTasks()) - 1
		} else if len(msg.files) == 0 {
			m.queue.AddFailedTask(msg.repoID, "Hugging Face Repo", fmt.Errorf("no GGUF or ONNX files found in repository '%s'", msg.repoID))
			m.focus = FocusURL
			m.urlInput.Focus()
			m.selectedTaskIdx = len(m.queue.GetTasks()) - 1
		} else if len(msg.files) == 1 {
			filename := msg.files[0].Rpath
			modelName := filename
			ext := strings.ToLower(filepath.Ext(modelName))
			if ext == ".gguf" || ext == ".onnx" {
				modelName = modelName[:len(modelName)-len(ext)]
			}
			downloadURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", msg.repoID, filename)
			m.queue.AddTask(modelName, filename, msg.files[0].Size, downloadURL)
			m.urlInput.SetValue("")
			m.filenameInput.SetValue("")
			m.focus = FocusURL
			m.urlInput.Focus()
			m.filenameInput.Blur()
			m.selectedTaskIdx = len(m.queue.GetTasks()) - 1
		} else {
			m.resolvedFiles = msg.files
			m.selectedFileIdx = 0
			m.repoID = msg.repoID
			m.focus = FocusFileList
			m.urlInput.Blur()
			m.filenameInput.Blur()
		}

	case tea.KeyMsg:
		if m.resolving {
			if msg.String() == "esc" {
				// Allow escape to flow
			} else {
				return m, nil
			}
		}
		m.err = nil // Clear error on key input
		switch msg.String() {
		case "ctrl+v":
			if m.focus == FocusURL {
				pasteFromClipboard(&m.urlInput)
			} else if m.focus == FocusFilename {
				pasteFromClipboard(&m.filenameInput)
			}
		case "tab":
			m.nextFocus()
		case "shift+tab":
			m.prevFocus()

		case "enter":
			if m.focus == FocusFileList {
				if len(m.resolvedFiles) > 0 && m.selectedFileIdx >= 0 && m.selectedFileIdx < len(m.resolvedFiles) {
					selectedFile := m.resolvedFiles[m.selectedFileIdx]
					filename := selectedFile.Rpath
					modelName := filename
					ext := strings.ToLower(filepath.Ext(modelName))
					if ext == ".gguf" || ext == ".onnx" {
						modelName = modelName[:len(modelName)-len(ext)]
					}
					parts := strings.Split(filename, "/")
					baseName := parts[len(parts)-1]

					downloadURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", m.repoID, filename)
					m.queue.AddTask(modelName, baseName, selectedFile.Size, downloadURL)

					m.resolvedFiles = nil
					m.repoID = ""
					m.urlInput.SetValue("")
					m.filenameInput.SetValue("")
					m.focus = FocusURL
					m.urlInput.Focus()
					m.selectedTaskIdx = len(m.queue.GetTasks()) - 1
				}
			} else if m.focus == FocusURL || m.focus == FocusFilename {
				urlStr := strings.TrimSpace(m.urlInput.Value())
				if urlStr != "" {
					filename := strings.TrimSpace(m.filenameInput.Value())

					isHFRepo := false
					repoID := urlStr

					if strings.Contains(urlStr, "huggingface.co/") && !strings.Contains(urlStr, "/resolve/") {
						isHFRepo = true
						idx := strings.Index(repoID, "huggingface.co/")
						repoID = repoID[idx+len("huggingface.co/"):]
					} else if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") && strings.Contains(urlStr, "/") {
						isHFRepo = true
					}

					if isHFRepo {
						repoID = strings.Trim(repoID, "/")
						parts := strings.Split(repoID, "/")
						if len(parts) >= 2 {
							repoID = parts[0] + "/" + parts[1]
						}

						if filename != "" {
							downloadURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repoID, filename)
							modelName := filename
							ext := strings.ToLower(filepath.Ext(modelName))
							if ext == ".gguf" || ext == ".onnx" {
								modelName = modelName[:len(modelName)-len(ext)]
							}
							m.queue.AddTask(modelName, filename, 0, downloadURL)
							m.urlInput.SetValue("")
							m.filenameInput.SetValue("")
							m.focus = FocusURL
							m.urlInput.Focus()
							m.filenameInput.Blur()
							m.selectedTaskIdx = len(m.queue.GetTasks()) - 1
						} else {
							m.resolving = true
							m.err = nil
							m.urlInput.Blur()
							m.filenameInput.Blur()
							cmds = append(cmds, m.resolveHFRepo(repoID))
						}
					} else {
						if filename == "" {
							parts := strings.Split(urlStr, "/")
							if len(parts) > 0 {
								filename = parts[len(parts)-1]
								if qIdx := strings.Index(filename, "?"); qIdx != -1 {
									filename = filename[:qIdx]
								}
							}
						}
						if filename == "" {
							if strings.Contains(strings.ToLower(urlStr), ".onnx") {
								filename = "downloaded_model.onnx"
							} else {
								filename = "downloaded_model.gguf"
							}
						}

						modelName := filename
						ext := strings.ToLower(filepath.Ext(modelName))
						if ext == ".gguf" || ext == ".onnx" {
							modelName = modelName[:len(modelName)-len(ext)]
						}

						m.queue.AddTask(modelName, filename, 0, urlStr)
						m.urlInput.SetValue("")
						m.filenameInput.SetValue("")
						m.focus = FocusURL
						m.urlInput.Focus()
						m.filenameInput.Blur()
						m.selectedTaskIdx = len(m.queue.GetTasks()) - 1
					}
				}
			}

		case "up", "k":
			if m.focus == FocusURL || m.focus == FocusFilename {
				m.prevFocus()
			} else if m.focus == FocusFileList {
				m.selectedFileIdx--
				if m.selectedFileIdx < 0 {
					m.selectedFileIdx = 0
				}
			} else if m.focus == FocusQueue {
				m.moveCursor(-1)
			}
		case "down", "j":
			if m.focus == FocusURL || m.focus == FocusFilename {
				m.nextFocus()
			} else if m.focus == FocusFileList {
				m.selectedFileIdx++
				if m.selectedFileIdx >= len(m.resolvedFiles) {
					m.selectedFileIdx = len(m.resolvedFiles) - 1
				}
			} else if m.focus == FocusQueue {
				m.moveCursor(1)
			}

		case "p", "P":
			if m.focus == FocusQueue {
				tasks := m.queue.GetTasks()
				if len(tasks) > 0 && m.selectedTaskIdx >= 0 && m.selectedTaskIdx < len(tasks) {
					t := tasks[m.selectedTaskIdx]
					if t.Status == model.StatusDownloading {
						m.queue.PauseTask(t)
					} else {
						m.queue.ResumeTask(t)
					}
				}
			}

		case "c", "C":
			if m.focus == FocusQueue {
				tasks := m.queue.GetTasks()
				if len(tasks) > 0 && m.selectedTaskIdx >= 0 && m.selectedTaskIdx < len(tasks) {
					t := tasks[m.selectedTaskIdx]
					if t.Status == model.StatusCompleted || t.Status == model.StatusFailed || t.Status == model.StatusCanceled {
						m.queue.RemoveTask(t)
					} else {
						m.queue.CancelTask(t)
					}
					if m.selectedTaskIdx >= len(m.queue.GetTasks()) {
						m.selectedTaskIdx = len(m.queue.GetTasks()) - 1
					}
					if m.selectedTaskIdx < 0 {
						m.selectedTaskIdx = 0
					}
				}
			}

		case "x", "X":
			if m.focus == FocusQueue {
				m.queue.ClearFinishedTasks()
				if m.selectedTaskIdx >= len(m.queue.GetTasks()) {
					m.selectedTaskIdx = len(m.queue.GetTasks()) - 1
				}
				if m.selectedTaskIdx < 0 {
					m.selectedTaskIdx = 0
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *DownloaderModel) nextFocus() {
	if m.focus == FocusFileList {
		return
	}
	m.urlInput.Blur()
	m.filenameInput.Blur()
	switch m.focus {
	case FocusURL:
		m.focus = FocusFilename
		m.filenameInput.Focus()
	case FocusFilename:
		m.focus = FocusQueue
	case FocusQueue:
		m.focus = FocusURL
		m.urlInput.Focus()
	}
}

func (m *DownloaderModel) prevFocus() {
	if m.focus == FocusFileList {
		return
	}
	m.urlInput.Blur()
	m.filenameInput.Blur()
	switch m.focus {
	case FocusURL:
		m.focus = FocusQueue
	case FocusFilename:
		m.focus = FocusURL
		m.urlInput.Focus()
	case FocusQueue:
		m.focus = FocusFilename
		m.filenameInput.Focus()
	}
}

func (m *DownloaderModel) moveCursor(dir int) {
	if m.focus == FocusQueue {
		tasks := m.queue.GetTasks()
		if len(tasks) == 0 {
			return
		}
		m.selectedTaskIdx += dir
		if m.selectedTaskIdx < 0 {
			m.selectedTaskIdx = 0
		}
		if m.selectedTaskIdx >= len(tasks) {
			m.selectedTaskIdx = len(tasks) - 1
		}
	}
}

func (m *DownloaderModel) View(width int, height int) string {
	// Top Bento Card: Direct Model Download
	var inputSb strings.Builder

	urlLabel := "Direct URL / Hugging Face Repository:"
	fileLabel := "Destination Filename (optional/required for repositories):"

	urlStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	fileStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	if m.focus == FocusURL {
		urlStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	} else if m.focus == FocusFilename {
		fileStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	}

	inputSb.WriteString("  " + urlStyle.Render(urlLabel) + "\n")
	inputSb.WriteString("  " + m.urlInput.View() + "\n\n")
	inputSb.WriteString("  " + fileStyle.Render(fileLabel) + "\n")
	inputSb.WriteString("  " + m.filenameInput.View() + "\n\n")

	var tokenBadge string
	hfToken := ""
	if m.config != nil {
		hfToken = m.config.HFToken
	}
	if hfToken == "" {
		hfToken = os.Getenv("HF_TOKEN")
	}
	if hfToken != "" {
		tokenBadge = StyleBadgeFits.Render("[Token Active]")
	} else {
		tokenBadge = StyleBadgeStopped.Render("[No HF Token]")
	}

	hintText := StyleHelp.Render("Supports direct GGUF/ONNX links or Hugging Face repositories (e.g. unsloth/gemma-4-E4B-it-GGUF).")
	inputSb.WriteString("  " + hintText + "  " + tokenBadge)

	if m.resolving {
		inputSb.WriteString("\n\n  " + lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("Fetching repository files list from Hugging Face..."))
	}

	if m.err != nil {
		inputSb.WriteString("\n\n  " + lipgloss.NewStyle().Foreground(ColorDanger).Bold(true).Render(m.err.Error()))
	}

	// Bottom Bento Card: Download Queue & Progress
	var queueSb strings.Builder

	if m.focus == FocusFileList {
		queueSb.WriteString("  " + lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary).Render("Select Model File (Enter to download, Esc to cancel):") + "\n\n")
		for idx, f := range m.resolvedFiles {
			row := fmt.Sprintf("  - %s (%s)", f.Rpath, formatSize(f.Size))
			if idx == m.selectedFileIdx {
				queueSb.WriteString(StyleSelectedListItem.Width(max(20, width-8)).Render(row) + "\n")
			} else {
				queueSb.WriteString(row + "\n")
			}
		}
	} else {
		tasks := m.queue.GetTasks()
		if len(tasks) == 0 {
			queueSb.WriteString("  " + StyleMuted.Render("Queue is empty. Enter a direct GGUF/ONNX download URL or Hugging Face repo above.") + "\n")
		} else {
			startHex := ThemeGradientStart
			if startHex == "" {
				startHex = "#7D56F4"
			}
			endHex := ThemeGradientEnd
			if endHex == "" {
				endHex = "#FF5F87"
			}

			barWidth := 14
			if width > 120 {
				barWidth = 18
			} else if width < 80 {
				barWidth = 10
			}

			for idx, t := range tasks {
				var statusStr string
				switch t.Status {
				case model.StatusQueued:
					statusStr = StyleBadgeStopped.Render("[Queued]")
				case model.StatusDownloading:
					statusStr = StyleBadgeRunning.Render("[Downloading]")
				case model.StatusPaused:
					statusStr = StyleBadgeStarting.Render("[Paused]")
				case model.StatusCompleted:
					statusStr = StyleBadgeFits.Render("[Completed]")
				case model.StatusFailed:
					statusStr = StyleBadgeFailed.Render("[Failed]")
				case model.StatusCanceled:
					statusStr = StyleBadgeStopped.Render("[Canceled]")
				}

				progressPct := 0.0
				if t.TotalSize > 0 {
					progressPct = (float64(t.Downloaded) / float64(t.TotalSize)) * 100.0
					if progressPct > 100.0 {
						progressPct = 100.0
					}
				} else if t.Status == model.StatusCompleted {
					progressPct = 100.0
				}

				progressBar := RenderGradientBar(progressPct, barWidth, startHex, endHex)

				speedStr := ""
				if t.Status == model.StatusDownloading {
					if t.SpeedKBps >= 1024.0 {
						speedStr = fmt.Sprintf("%.2f MB/s", t.SpeedKBps/1024.0)
					} else {
						speedStr = fmt.Sprintf("%.1f KB/s", t.SpeedKBps)
					}
				}

				etaStr := ""
				if t.Status == model.StatusDownloading && t.SpeedKBps > 0 && t.TotalSize > t.Downloaded {
					remBytes := t.TotalSize - t.Downloaded
					remSecs := float64(remBytes) / (t.SpeedKBps * 1024.0)
					if remSecs < 60 {
						etaStr = fmt.Sprintf("ETA: %ds", int(remSecs))
					} else if remSecs < 3600 {
						etaStr = fmt.Sprintf("ETA: %dm %ds", int(remSecs)/60, int(remSecs)%60)
					} else {
						etaStr = fmt.Sprintf("ETA: %dh %dm", int(remSecs)/3600, (int(remSecs)%3600)/60)
					}
				}

				sizeStr := fmt.Sprintf("%s / %s", formatSize(t.Downloaded), formatSize(t.TotalSize))

				nameColWidth := 20
				if width > 120 {
					nameColWidth = 26
				}
				fileNameTrunc := TruncateVisual(t.FileName, nameColWidth, "...")

				var row strings.Builder
				prefix := "  "
				if m.focus == FocusQueue && idx == m.selectedTaskIdx {
					prefix = "> "
				}
				row.WriteString(prefix)
				row.WriteString(fmt.Sprintf("%-*s %s  %3.0f%% [%s]  %-16s",
					nameColWidth,
					fileNameTrunc,
					statusStr,
					progressPct,
					progressBar,
					sizeStr,
				))

				if speedStr != "" {
					row.WriteString("  " + lipgloss.NewStyle().Foreground(ColorSecondary).Render(speedStr))
				}
				if etaStr != "" {
					row.WriteString("  " + lipgloss.NewStyle().Foreground(ColorTextMuted).Render(etaStr))
				}
				if t.Status == model.StatusFailed && t.Error != nil {
					row.WriteString("  " + lipgloss.NewStyle().Foreground(ColorDanger).Render(fmt.Sprintf("(%v)", t.Error)))
				}

				renderedRow := row.String()
				if m.focus == FocusQueue && idx == m.selectedTaskIdx {
					rowWidth := width - 8
					if rowWidth < 20 {
						rowWidth = 20
					}
					queueSb.WriteString(StyleSelectedListItem.Width(rowWidth).Render(renderedRow) + "\n")
				} else {
					queueSb.WriteString(renderedRow + "\n")
				}
			}
		}
	}

	topContent := strings.TrimRight(inputSb.String(), "\n")
	botContent := strings.TrimRight(queueSb.String(), "\n")

	// Calculate balanced card height to fill the available height
	topHeight := 0
	botHeight := 0
	if height > 0 {
		if height > 14 {
			topHeight = 7
			botHeight = height - topHeight
		}
	}

	topCard := SurfaceCardWithHeight("Direct Model Download", topContent, width, topHeight, m.focus == FocusURL || m.focus == FocusFilename, "HuggingFace / Direct")
	bottomCard := SurfaceCardWithHeight("Download Queue & Progress", botContent, width, botHeight, m.focus == FocusQueue || m.focus == FocusFileList, fmt.Sprintf("%d active", len(m.queue.GetTasks())))

	return lipgloss.JoinVertical(lipgloss.Left,
		topCard,
		bottomCard,
	)
}

type hfResolveMsg struct {
	repoID string
	files  []model.HFSibling
	err    error
}

func (m *DownloaderModel) resolveHFRepo(repoID string) tea.Cmd {
	token := m.config.HFToken
	return func() tea.Msg {
		files, err := model.ListHFModelFiles(repoID, token)
		if err != nil {
			return hfResolveMsg{repoID: repoID, err: err}
		}
		return hfResolveMsg{repoID: repoID, files: files}
	}
}
