package chat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BIJJUDAMA/runora/config"
)

// Message is a single turn in a chat conversation.
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"` // "user" | "assistant" | "system"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Tokens    int       `json:"tokens"`  // prompt_eval_count or predicted tokens
	Partial   bool      `json:"partial"` // true if generation was stopped mid-stream
}

// CompactionCheckpoint replaces a range of messages in active context with a summary.
// OriginalMessages preserves the verbatim source for history inspection.
type CompactionCheckpoint struct {
	ID               string    `json:"id"`
	Summary          string    `json:"summary"`
	CoveredRange     [2]int    `json:"covered_range"` // [firstIdx, lastIdx] inclusive
	TokensBefore     int       `json:"tokens_before"`
	TokensAfter      int       `json:"tokens_after"`
	CreatedAt        time.Time `json:"created_at"`
	OriginalMessages []Message `json:"original_messages"`
}

// ChatParams are the per-session inference parameters.
type ChatParams struct {
	Temperature  float64 `json:"temperature"`
	TopP         float64 `json:"top_p"`
	TopK         int     `json:"top_k"`
	ContextSize  int     `json:"context_size"` // 0 = query /props from server
	SystemPrompt string  `json:"system_prompt"`
}

// DefaultChatParams returns standard balanced generation parameters.
func DefaultChatParams() ChatParams {
	return ChatParams{
		Temperature:  0.7,
		TopP:         0.9,
		TopK:         40,
		ContextSize:  0,
		SystemPrompt: "",
	}
}

// Session is a named conversation with a persistent identity and parameters.
type Session struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	ModelPath   string                 `json:"model_path"`
	Port        int                    `json:"port"`
	Params      ChatParams             `json:"params"`
	Messages    []Message              `json:"messages"`
	Checkpoints []CompactionCheckpoint `json:"checkpoints"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// GenerateID produces a random hexadecimal identifier.
func GenerateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	hexStr := hex.EncodeToString(b)
	if prefix != "" {
		return fmt.Sprintf("%s-%s", prefix, hexStr)
	}
	return hexStr
}

// NewSession creates and persists a new blank conversation session.
func NewSession(chatsDir string, name, modelPath string, port int, params ChatParams) (*Session, error) {
	if chatsDir == "" {
		return nil, fmt.Errorf("chats directory path cannot be empty")
	}
	if name == "" {
		name = "New Chat"
	}
	if params == (ChatParams{}) {
		params = DefaultChatParams()
	} else {
		if params.Temperature == 0 {
			params.Temperature = 0.7
		}
		if params.TopP == 0 {
			params.TopP = 0.9
		}
		if params.TopK == 0 {
			params.TopK = 40
		}
	}

	now := time.Now()
	sess := &Session{
		ID:          GenerateID("sess"),
		Name:        name,
		ModelPath:   modelPath,
		Port:        port,
		Params:      params,
		Messages:    []Message{},
		Checkpoints: []CompactionCheckpoint{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := SaveSession(chatsDir, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// sessionFilePath returns the full filesystem path for a session JSON file.
func sessionFilePath(chatsDir string, id string) string {
	cleanID := filepath.Base(id)
	if !strings.HasSuffix(cleanID, ".json") {
		cleanID = cleanID + ".json"
	}
	return filepath.Join(chatsDir, cleanID)
}

// SaveSession persists a Session struct to disk atomically.
func SaveSession(chatsDir string, session *Session) error {
	if session == nil {
		return fmt.Errorf("cannot save nil session")
	}
	if session.ID == "" {
		return fmt.Errorf("session ID is required")
	}
	if err := os.MkdirAll(chatsDir, 0755); err != nil {
		return fmt.Errorf("failed to create chats directory: %w", err)
	}

	session.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	target := sessionFilePath(chatsDir, session.ID)
	return config.AtomicWriteFile(target, data, 0644)
}

// LoadSession loads and deserializes a single session from disk by ID.
func LoadSession(chatsDir string, id string) (*Session, error) {
	target := sessionFilePath(chatsDir, id)
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file %s: %w", target, err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session JSON: %w", err)
	}
	if sess.Messages == nil {
		sess.Messages = []Message{}
	}
	if sess.Checkpoints == nil {
		sess.Checkpoints = []CompactionCheckpoint{}
	}
	return &sess, nil
}

// ListSessions scans the chats directory and returns all sessions sorted by UpdatedAt descending.
func ListSessions(chatsDir string) ([]*Session, error) {
	if _, err := os.Stat(chatsDir); os.IsNotExist(err) {
		return []*Session{}, nil
	}

	entries, err := os.ReadDir(chatsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read chats directory: %w", err)
	}

	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		sessPath := filepath.Join(chatsDir, entry.Name())
		data, err := os.ReadFile(sessPath)
		if err != nil {
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		if s.Messages == nil {
			s.Messages = []Message{}
		}
		if s.Checkpoints == nil {
			s.Checkpoints = []CompactionCheckpoint{}
		}
		sessions = append(sessions, &s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// DeleteSession permanently removes a session file from disk.
func DeleteSession(chatsDir string, id string) error {
	target := sessionFilePath(chatsDir, id)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session %s: %w", id, err)
	}
	return nil
}

// RenameSession updates the friendly display name of a session and saves it.
func RenameSession(chatsDir string, id string, newName string) error {
	sess, err := LoadSession(chatsDir, id)
	if err != nil {
		return err
	}
	sess.Name = strings.TrimSpace(newName)
	if sess.Name == "" {
		sess.Name = "Untitled Chat"
	}
	return SaveSession(chatsDir, sess)
}
