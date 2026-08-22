package model

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// LocalDemo handles a few offline patterns so onboard→run demo works without an API key.
// Prefer openai_compat when a secret is present.
type LocalDemo struct{}

var writeRe = regexp.MustCompile(`(?i)write\s+(.+?)\s+to\s+(?:workspace/)?([^\s]+)`)

func (LocalDemo) Complete(_ context.Context, req ports.CompletionRequest) (ports.CompletionResponse, error) {
	// If last message is a tool result, finish.
	if len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1]
		if last.Role == "tool" {
			return ports.CompletionResponse{Content: "Done."}, nil
		}
	}
	user := lastUserText(req.Messages)
	if m := writeRe.FindStringSubmatch(user); len(m) == 3 {
		content := strings.Trim(m[1], `"'`)
		path := strings.Trim(m[2], `"'`)
		path = strings.TrimPrefix(path, "workspace/")
		args, _ := json.Marshal(map[string]string{"path": path, "content": content})
		return ports.CompletionResponse{
			ToolCalls: []ports.ToolCall{{
				ID:   "local-write-1",
				Name: "workspace_write",
				Args: args,
			}},
		}, nil
	}
	return ports.CompletionResponse{
		Content: "Local demo model (no API key). Try: write hello to workspace/hi.md",
	}, nil
}

func lastUserText(msgs []ports.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}
