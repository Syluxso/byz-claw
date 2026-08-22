package hooks

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

type Audit struct {
	PointName ports.HookPoint
}

func (a Audit) Point() ports.HookPoint { return a.PointName }
func (a Audit) Name() string           { return "audit" }

func (a Audit) Run(_ context.Context, hc *ports.HookContext) error {
	rec := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"hook":  string(a.PointName),
		"name":  "audit",
		"tool":  hc.ToolName,
		"notes": hc.Notes,
	}
	if hc.Run != nil {
		rec["run_id"] = hc.Run.ID
		rec["session"] = hc.Run.SessionID
		rec["status"] = hc.Run.Status
	}
	_ = json.NewEncoder(os.Stdout).Encode(rec)
	return nil
}

func AuditSuite() []ports.Hook {
	points := []ports.HookPoint{
		ports.HookBeforeTool, ports.HookAfterTool, ports.HookOnError, ports.HookOnComplete,
	}
	out := make([]ports.Hook, 0, len(points))
	for _, p := range points {
		out = append(out, Audit{PointName: p})
	}
	return out
}
