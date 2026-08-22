package hooks

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

var (
	auditMu     sync.Mutex
	auditWriter io.Writer // nil → discard unless verbose
	auditVerbose bool
)

// ConfigureAudit sets where audit JSON lines go.
// If verbose is false and w is nil, audit is discarded (quiet CLI default).
func ConfigureAudit(w io.Writer, verbose bool) {
	auditMu.Lock()
	defer auditMu.Unlock()
	auditWriter = w
	auditVerbose = verbose
}

type Audit struct {
	PointName ports.HookPoint
}

func (a Audit) Point() ports.HookPoint { return a.PointName }
func (a Audit) Name() string           { return "audit" }

func (a Audit) Run(_ context.Context, hc *ports.HookContext) error {
	rec := map[string]any{
		"ts":   time.Now().UTC().Format(time.RFC3339Nano),
		"hook": string(a.PointName),
		"name": "audit",
		"tool": hc.ToolName,
	}
	if hc.Run != nil {
		rec["run_id"] = hc.Run.ID
		rec["session"] = hc.Run.SessionID
		rec["status"] = hc.Run.Status
	}
	auditMu.Lock()
	w := auditWriter
	verbose := auditVerbose
	auditMu.Unlock()
	if w == nil {
		if !verbose {
			return nil
		}
		w = os.Stderr
	}
	_ = json.NewEncoder(w).Encode(rec)
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
