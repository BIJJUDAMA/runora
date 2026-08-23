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

type DownloaderModel struct {
	config          *config.Config
	queue           *model.DownloadQueue
	focus           DownloaderFocus
	selectedTaskIdx int
	err             error
	resolving       bool

	urlInput        textinput.Model
	filenameInput   textinput.Model

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

						hfToken := m.config.HFToken
						if hfToken == "" {
							hfToken = os.Getenv("HF_TOKEN")
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
			if m.focus == FocusFileList {
				m.selectedFileIdx--
				if m.selectedFileIdx < 0 {
					m.selectedFileIdx = 0
				}
			} else if m.focus == FocusQueue {
				m.moveCursor(-1)
			}
		case "down", "j":
			if m.focus == FocusFileList {
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
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("DIRECT MODEL DOWNLOADER (GGUF / ONNX)")))

	// Input form panel
	var directSb strings.Builder

	urlStyle := lipgloss.NewStyle().Foreground(ColorWhite)
	fileStyle := lipgloss.NewStyle().Foreground(ColorWhite)
	if m.focus == FocusURL {
		urlStyle = urlStyle.Foreground(ColorSecondary).Bold(true)
	} else if m.focus == FocusFilename {
		fileStyle = fileStyle.Foreground(ColorSecondary).Bold(true)
	}

	directSb.WriteString("  " + urlStyle.Render("Direct URL / Hugging Face Repository:") + "\n")
	directSb.WriteString("  " + m.urlInput.View() + "\n\n")
	directSb.WriteString("  " + fileStyle.Render("Destination Filename (optional/required for repositories):") + "\n")
	directSb.WriteString("  " + m.filenameInput.View() + "\n\n")
	directSb.WriteString("  " + StyleHelp.Render("Supports direct GGUF/ONNX links or Hugging Face repositories (e.g. unsloth/gemma-4-E4B-it-GGUF).") + "\n")

	if m.resolving {
		directSb.WriteString("\n" + lipgloss.NewStyle().Foreground(ColorAccent).Render("  Fetching repository files list from Hugging Face...") + "\n")
	}

	if m.err != nil {
		directSb.WriteString("\n" + lipgloss.NewStyle().Foreground(ColorDanger).Render("  "+m.err.Error()) + "\n")
	}

	linesCount := strings.Count(directSb.String(), "\n")
	panelHeight := linesCount + 1
	if panelHeight < 7 {
		panelHeight = 7
	}

	formBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Width(width - 6).
		Height(panelHeight)
	if m.focus == FocusURL || m.focus == FocusFilename {
		formBorder = formBorder.BorderForeground(ColorPrimary)
	}

	sb.WriteString("  " + formBorder.Render(directSb.String()) + "\n\n")

	// Queue panel
	var queueSb strings.Builder
	queueHeight := height - panelHeight - 16
	if queueHeight < 4 {
		queueHeight = 4
	}

	if m.focus == FocusFileList {
		queueSb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary).Render("Select Model File (Enter to download, Esc to cancel):") + "\n")
		maxVisible := queueHeight - 2
		if maxVisible < 1 {
			maxVisible = 1
		}
		startIdx := 0
		if m.selectedFileIdx >= maxVisible {
			startIdx = m.selectedFileIdx - maxVisible + 1
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(m.resolvedFiles) {
			endIdx = len(m.resolvedFiles)
		}

		for idx := startIdx; idx < endIdx; idx++ {
			f := m.resolvedFiles[idx]
			row := fmt.Sprintf("  - %s (%s)", f.Rpath, formatSize(f.Size))
			if idx == m.selectedFileIdx {
				queueSb.WriteString(StyleSelectedListItem.Width(width - 8).Render(row) + "\n")
			} else {
				queueSb.WriteString(row + "\n")
			}
		}
	} else {
		queueSb.WriteString(lipgloss.NewStyle().Bold(true).Render("Download Queue:") + "\n")
		tasks := m.queue.GetTasks()
		if len(tasks) == 0 {
			queueSb.WriteString("  Queue is empty. Enter a GGUF/ONNX URL above to start downloading.\n")
		} else {
			for idx, t := range tasks {
				statusStr := ""
				switch t.Status {
				case model.StatusQueued:
					statusStr = StyleBadgeStopped.Render(" QUEUED ")
				case model.StatusDownloading:
					statusStr = StyleBadgeRunning.Render(" DOWNLOADING ") + fmt.Sprintf(" %.1f KB/s", t.SpeedKBps)
				case model.StatusPaused:
					statusStr = StyleBadgeStarting.Render(" PAUSED ")
				case model.StatusCompleted:
					statusStr = StyleBadgeRunning.Render(" COMPLETED ")
				case model.StatusFailed:
					statusStr = StyleBadgeFailed.Render(" FAILED ") + fmt.Sprintf(": %v", t.Error)
				case model.StatusCanceled:
					statusStr = StyleBadgeStopped.Render(" CANCELED ")
				}

				progressFraction := 0.0
				if t.TotalSize > 0 {
					progressFraction = float64(t.Downloaded) / float64(t.TotalSize)
				}
				progressBar := RenderProgressBar(progressFraction*100.0, 14, ColorProgressFilled) + fmt.Sprintf(" %3.0f%%", progressFraction*100.0)

				row := fmt.Sprintf("%-20s %s %s (%s / %s)",
					t.FileName, progressBar, statusStr, formatSize(t.Downloaded), formatSize(t.TotalSize),
				)

				if m.focus == FocusQueue && idx == m.selectedTaskIdx {
					queueSb.WriteString(StyleSelectedListItem.Width(width - 8).Render(row) + "\n")
				} else {
					queueSb.WriteString(row + "\n")
				}
			}
		}
	}

	panelBorderQueue := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Width(width - 6).
		Height(queueHeight)
	if m.focus == FocusQueue || m.focus == FocusFileList {
		panelBorderQueue = panelBorderQueue.BorderForeground(ColorPrimary)
	}

	sb.WriteString("  " + panelBorderQueue.Render(queueSb.String()) + "\n\n")

	// Help instructions
	var helpKeys []string
	if m.focus == FocusFileList {
		helpKeys = append(helpKeys, fmt.Sprintf("%s Move Selection", StyleHelpKey.Render("[Up/Down/j/k]")))
		helpKeys = append(helpKeys, fmt.Sprintf("%s Download", StyleHelpKey.Render("[Enter]")))
		helpKeys = append(helpKeys, fmt.Sprintf("%s Cancel", StyleHelpKey.Render("[Esc]")))
	} else {
		helpKeys = append(helpKeys, fmt.Sprintf("%s Navigation", StyleHelpKey.Render("[Tab/Shift-Tab]")))
		if m.focus == FocusURL || m.focus == FocusFilename {
			helpKeys = append(helpKeys, fmt.Sprintf("%s Start Download", StyleHelpKey.Render("[Enter]")))
		} else if m.focus == FocusQueue {
			helpKeys = append(helpKeys, fmt.Sprintf("%s Pause/Resume", StyleHelpKey.Render("[P]")))
			helpKeys = append(helpKeys, fmt.Sprintf("%s Cancel/Remove", StyleHelpKey.Render("[C]")))
			helpKeys = append(helpKeys, fmt.Sprintf("%s Clear Finished", StyleHelpKey.Render("[X]")))
		}
		helpKeys = append(helpKeys, fmt.Sprintf("%s Return to Browser", StyleHelpKey.Render("[Esc]")))
	}

	sb.WriteString("  " + strings.Join(helpKeys, "  ") + "\n")

	if m.focus != FocusFileList && len(m.queue.GetTasks()) > 0 {
		sb.WriteString("\n  " + StyleHelp.Render("Tip: Press [Tab] to focus the queue, use [Up/Down/j/k] to select a task, [P] to Pause/Resume, [C] to Cancel/Remove, and [X] to Clear Finished.") + "\n")
	}

	boxWidth := width - 4
	if boxWidth < 50 {
		boxWidth = 50
	}
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorPrimary).
		Width(boxWidth).
		Render(sb.String())
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
