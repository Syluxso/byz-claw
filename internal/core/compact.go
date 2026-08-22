package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// roughTokens approximates tokens as chars/4.
func roughTokens(msgs []ports.Message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Content) / 4
		for _, tc := range m.ToolCalls {
			n += (len(tc.Name) + len(tc.Args)) / 4
		}
	}
	return n
}

// CompactIfNeeded summarizes older history when over threshold.
// Uses a single Model.Complete call (not a nested agent loop).
func (l *Loop) CompactIfNeeded(ctx context.Context, sessionID string, msgs []ports.Message) ([]ports.Message, error) {
	th := l.Config.CompactTokenThreshold
	tail := l.Config.CompactTailMessages
	if th <= 0 || len(msgs) < tail+4 {
		return msgs, nil
	}
	if roughTokens(msgs) < th {
		return msgs, nil
	}
	if tail < 2 {
		tail = 2
	}
	if len(msgs) <= tail {
		return msgs, nil
	}
	head := msgs[:len(msgs)-tail]
	keep := msgs[len(msgs)-tail:]

	var transcript strings.Builder
	for _, m := range head {
		transcript.WriteString(m.Role)
		transcript.WriteString(": ")
		transcript.WriteString(m.Content)
		if len(m.ToolCalls) > 0 {
			transcript.WriteString(" [tools]")
		}
		transcript.WriteString("\n")
	}
	summaryReq := ports.CompletionRequest{
		Messages: []ports.Message{
			{Role: "system", Content: "Summarize the conversation for continuity. Keep facts, paths written, and open tasks. No tools."},
			{Role: "user", Content: transcript.String()},
		},
	}
	resp, err := l.Model.Complete(ctx, summaryReq)
	if err != nil {
		// Compaction failure must not break the run — keep full history.
		return msgs, nil
	}
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		summary = "(prior context compacted)"
	}
	sumMsg := ports.Message{
		ID:        newID(),
		SessionID: sessionID,
		Role:      "system",
		Content:   "Conversation summary (compacted):\n" + summary,
		CreatedAt: l.Clock.Now(),
	}
	// Persist summary and drop compacted messages from store by rewriting session? 
	// Simpler: return in-memory compacted view for this Complete only; also save summary for future.
	_ = l.Store.SaveMessage(ctx, sumMsg)
	out := make([]ports.Message, 0, 1+len(keep))
	out = append(out, sumMsg)
	out = append(out, keep...)
	return out, nil
}

// Format for tests.
func compactDebug(n int) string { return fmt.Sprintf("tokens~%d", n) }
