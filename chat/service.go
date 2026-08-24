package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TokenChunk represents a streamed token or final completion metrics.
type TokenChunk struct {
	Token        string
	Done         bool
	PromptTokens int
	GenTokens    int
	TokensPerSec float64
	Err          error
}

// ChatService defines the service boundary for chat session and inference operations.
type ChatService interface {
	Stream(ctx context.Context, session *Session, userMsg string) (<-chan TokenChunk, error)
	Compact(ctx context.Context, session *Session) (*CompactionCheckpoint, error)
	EstimateTokens(session *Session) int
	ContextSizeFor(port int) (int, error)
	NewSession(name, modelPath string, port int, params ChatParams) (*Session, error)
	ListSessions() ([]*Session, error)
	LoadSession(id string) (*Session, error)
	SaveSession(session *Session) error
	DeleteSession(id string) error
	RenameSession(id, name string) error
	ChatsDir() string
}

type chatService struct {
	chatsDir string
	client   *http.Client
}

// NewService instantiates a new ChatService using the designated chats directory.
func NewService(chatsDir string) ChatService {
	return &chatService{
		chatsDir: chatsDir,
		client: &http.Client{
			Timeout: 0, // No client-level timeout for streaming
		},
	}
}

func (s *chatService) ChatsDir() string {
	return s.chatsDir
}

func (s *chatService) NewSession(name, modelPath string, port int, params ChatParams) (*Session, error) {
	return NewSession(s.chatsDir, name, modelPath, port, params)
}

func (s *chatService) ListSessions() ([]*Session, error) {
	return ListSessions(s.chatsDir)
}

func (s *chatService) LoadSession(id string) (*Session, error) {
	return LoadSession(s.chatsDir, id)
}

func (s *chatService) SaveSession(session *Session) error {
	return SaveSession(s.chatsDir, session)
}

func (s *chatService) DeleteSession(id string) error {
	return DeleteSession(s.chatsDir, id)
}

func (s *chatService) RenameSession(id, name string) error {
	return RenameSession(s.chatsDir, id, name)
}

func (s *chatService) EstimateTokens(session *Session) int {
	return EstimateTokens(session)
}

func (s *chatService) ContextSizeFor(port int) (int, error) {
	return ContextSizeFor(port)
}

func (s *chatService) Compact(ctx context.Context, session *Session) (*CompactionCheckpoint, error) {
	if session == nil {
		return nil, fmt.Errorf("cannot compact nil session")
	}
	return Compact(ctx, s.chatsDir, session, session.Port)
}

// Stream appends the user turn, constructs the 3-layer context, and streams tokens over SSE.
func (s *chatService) Stream(ctx context.Context, session *Session, userMsg string) (<-chan TokenChunk, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}
	if session.Port <= 0 {
		return nil, fmt.Errorf("invalid server port %d", session.Port)
	}

	userMsg = strings.TrimSpace(userMsg)
	if userMsg == "" {
		return nil, fmt.Errorf("cannot send empty message")
	}

	// Append user turn
	userTurn := Message{
		ID:        GenerateID("msg"),
		Role:      "user",
		Content:   userMsg,
		Timestamp: time.Now(),
		Tokens:    max(1, len(userMsg)/4),
	}
	session.Messages = append(session.Messages, userTurn)
	_ = s.SaveSession(session)

	// Build 3-layer context
	contextMsgs := BuildContext(session)

	reqPayload := map[string]interface{}{
		"model":       "local",
		"messages":    contextMsgs,
		"stream":      true,
		"temperature": session.Params.Temperature,
		"top_p":       session.Params.TopP,
		"top_k":       session.Params.TopK,
	}

	reqBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat payload: %w", err)
	}

	endpoint := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", session.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to construct chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to model server on port %d: %w", session.Port, err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("model server returned status %d: %s", resp.StatusCode, string(body))
	}

	outChan := make(chan TokenChunk, 64)

	go func() {
		defer close(outChan)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		var assistantBuf strings.Builder
		startTime := time.Now()
		var firstTokenTime time.Time
		genTokens := 0
		promptTokens := 0
		var streamErr error
		completedNormally := false

		for {
			if ctx.Err() != nil {
				streamErr = ctx.Err()
				break
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					completedNormally = true
					break
				}
				if ctx.Err() != nil {
					streamErr = ctx.Err()
				} else {
					streamErr = err
				}
				break
			}

			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "data:") {
				continue
			}

			dataContent := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if dataContent == "[DONE]" {
				completedNormally = true
				break
			}

			var sseObj struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
				Usage struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}

			if err := json.Unmarshal([]byte(dataContent), &sseObj); err != nil {
				continue
			}

			if sseObj.Usage.PromptTokens > 0 {
				promptTokens = sseObj.Usage.PromptTokens
			}

			if len(sseObj.Choices) > 0 {
				delta := sseObj.Choices[0].Delta.Content
				if delta != "" {
					if genTokens == 0 {
						firstTokenTime = time.Now()
					}
					genTokens++
					assistantBuf.WriteString(delta)

					outChan <- TokenChunk{
						Token:     delta,
						GenTokens: genTokens,
					}
				}
			}
		}

		// Calculate tokens per second
		tps := 0.0
		if !firstTokenTime.IsZero() && genTokens > 0 {
			dur := time.Since(firstTokenTime).Seconds()
			if dur > 0.05 {
				tps = float64(genTokens) / dur
			}
		} else if genTokens > 0 {
			dur := time.Since(startTime).Seconds()
			if dur > 0.05 {
				tps = float64(genTokens) / dur
			}
		}

		// Save assistant turn (partial or complete)
		fullContent := assistantBuf.String()
		if len(fullContent) > 0 {
			isPartial := (streamErr != nil || ctx.Err() != nil) && !completedNormally
			assistantTurn := Message{
				ID:        GenerateID("msg"),
				Role:      "assistant",
				Content:   fullContent,
				Timestamp: time.Now(),
				Tokens:    genTokens,
				Partial:   isPartial,
			}
			session.Messages = append(session.Messages, assistantTurn)
			_ = s.SaveSession(session)
		}

		outChan <- TokenChunk{
			Done:         true,
			PromptTokens: promptTokens,
			GenTokens:    genTokens,
			TokensPerSec: tps,
			Err:          streamErr,
		}
	}()

	return outChan, nil
}
