package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatMessage represents a role/content message payload for OpenAI-compatible chat endpoints.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BuildContext constructs the 3-layer prompt context for inference:
// Layer 1: System Prompt (Never compacted)
// Layer 2: Compaction Checkpoints Summary Stack
// Layer 3: Recent verbatim message tail
func BuildContext(session *Session) []ChatMessage {
	if session == nil {
		return []ChatMessage{}
	}

	var ctxMsgs []ChatMessage

	// Layer 1: System prompt
	if strings.TrimSpace(session.Params.SystemPrompt) != "" {
		ctxMsgs = append(ctxMsgs, ChatMessage{
			Role:    "system",
			Content: strings.TrimSpace(session.Params.SystemPrompt),
		})
	}

	// Layer 2: Summary Checkpoint Stack
	for _, cp := range session.Checkpoints {
		if strings.TrimSpace(cp.Summary) != "" {
			ctxMsgs = append(ctxMsgs, ChatMessage{
				Role:    "assistant",
				Content: fmt.Sprintf("[Previous Conversation Summary: %s]", strings.TrimSpace(cp.Summary)),
			})
		}
	}

	// Layer 3: Verbatim message history
	for _, msg := range session.Messages {
		if strings.TrimSpace(msg.Content) != "" {
			ctxMsgs = append(ctxMsgs, ChatMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	return ctxMsgs
}

// EstimateTokens calculates an approximate token count across all messages in context.
func EstimateTokens(session *Session) int {
	if session == nil {
		return 0
	}

	total := 0
	if session.Params.SystemPrompt != "" {
		total += max(1, len(session.Params.SystemPrompt)/4)
	}

	for _, cp := range session.Checkpoints {
		if cp.TokensAfter > 0 {
			total += cp.TokensAfter
		} else {
			total += max(1, len(cp.Summary)/4)
		}
	}

	for _, msg := range session.Messages {
		if msg.Tokens > 0 {
			total += msg.Tokens
		} else {
			total += max(1, len(msg.Content)/4)
		}
	}

	return total
}

// EstimateMessagesTokens computes token count for a subset of messages.
func EstimateMessagesTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		if m.Tokens > 0 {
			total += m.Tokens
		} else {
			total += max(1, len(m.Content)/4)
		}
	}
	return total
}

// ContextSizeFor queries the running llama-server /props endpoint to discover the active context size.
func ContextSizeFor(port int) (int, error) {
	if port <= 0 {
		return 4096, nil
	}

	client := http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/props", port))
	if err != nil {
		return 4096, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 4096, fmt.Errorf("props endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 4096, err
	}

	var props struct {
		DefaultGenerationSettings struct {
			NCtx int `json:"n_ctx"`
		} `json:"default_generation_settings"`
		NCtx int `json:"n_ctx"`
	}

	if err := json.Unmarshal(body, &props); err == nil {
		if props.DefaultGenerationSettings.NCtx > 0 {
			return props.DefaultGenerationSettings.NCtx, nil
		}
		if props.NCtx > 0 {
			return props.NCtx, nil
		}
	}

	return 4096, nil
}

// Compact performs lossy context compression with protected verbatim tail:
// 1. Identifies older conversation messages eligible for compression while preserving the recent 20% tail.
// 2. Calls the running model to summarize the selected turn range.
// 3. Replaces compacted turns with a CompactionCheckpoint, retaining originals in OriginalMessages.
// 4. Atomically persists the updated session.
func Compact(ctx context.Context, chatsDir string, session *Session, port int) (*CompactionCheckpoint, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}
	if len(session.Messages) < 2 {
		return nil, nil // Nothing to compact
	}

	// Resolve total context budget
	totalCtx := session.Params.ContextSize
	if totalCtx <= 0 {
		discovered, err := ContextSizeFor(port)
		if err == nil && discovered > 0 {
			totalCtx = discovered
		} else {
			totalCtx = 4096
		}
	}

	// 20% token budget for recent protected tail
	tailTokenBudget := int(float64(totalCtx) * 0.20)
	if tailTokenBudget < 100 {
		tailTokenBudget = 100
	}

	// Scan backwards from newest message to find protected tail cut point
	splitIdx := len(session.Messages)
	accumulatedTailTokens := 0

	for i := len(session.Messages) - 1; i >= 0; i-- {
		msgTokens := session.Messages[i].Tokens
		if msgTokens <= 0 {
			msgTokens = max(1, len(session.Messages[i].Content)/4)
		}

		// Always keep at least the last 2 turns in tail
		if (len(session.Messages)-i) > 2 && (accumulatedTailTokens+msgTokens) > tailTokenBudget {
			splitIdx = i + 1
			break
		}
		accumulatedTailTokens += msgTokens
		splitIdx = i
	}

	if splitIdx <= 0 {
		// All messages fit comfortably in tail budget, nothing to compact
		return nil, nil
	}

	compactable := session.Messages[:splitIdx]
	retainedTail := session.Messages[splitIdx:]

	if len(compactable) == 0 {
		return nil, nil
	}

	// Format conversation text for summarizer
	var convBuilder strings.Builder
	for _, m := range compactable {
		convBuilder.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(m.Role), strings.TrimSpace(m.Content)))
	}

	summaryPrompt := fmt.Sprintf(
		"Summarize the following conversation history clearly and concisely. Retain all key facts, requirements, code snippets, decisions, user preferences, and named entities verbatim without filler:\n\n%s",
		convBuilder.String(),
	)

	// Call model chat completions for summarization
	reqPayload := map[string]interface{}{
		"model": "local",
		"messages": []ChatMessage{
			{
				Role:    "system",
				Content: "You are a precise summarization engine. Output only the summarized dialogue retaining all critical facts and context.",
			},
			{
				Role:    "user",
				Content: summaryPrompt,
			},
		},
		"stream":      false,
		"temperature": 0.3,
	}

	reqBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode compaction request: %w", err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create compaction request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("compaction inference call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("compaction failed with HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var respObj struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&respObj); err != nil {
		return nil, fmt.Errorf("failed to decode compaction response: %w", err)
	}

	summaryText := ""
	if len(respObj.Choices) > 0 {
		summaryText = strings.TrimSpace(respObj.Choices[0].Message.Content)
	}
	if summaryText == "" {
		return nil, fmt.Errorf("model returned empty summary")
	}

	tokensBefore := EstimateMessagesTokens(compactable)
	tokensAfter := respObj.Usage.CompletionTokens
	if tokensAfter <= 0 {
		tokensAfter = max(1, len(summaryText)/4)
	}

	checkpoint := CompactionCheckpoint{
		ID:               GenerateID("cp"),
		Summary:          summaryText,
		CoveredRange:     [2]int{0, len(compactable) - 1},
		TokensBefore:     tokensBefore,
		TokensAfter:      tokensAfter,
		CreatedAt:        time.Now(),
		OriginalMessages: compactable,
	}

	// Update session structure
	session.Checkpoints = append(session.Checkpoints, checkpoint)
	session.Messages = retainedTail

	if chatsDir != "" {
		if err := SaveSession(chatsDir, session); err != nil {
			return nil, fmt.Errorf("failed to persist compacted session: %w", err)
		}
	}

	return &checkpoint, nil
}
