package hooks

import (
	"context"
	"fmt"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// TokenCeiling fails loud if configured ceiling is exceeded (set via Notes from loop).
// Loop sets hc.Notes with "usage_total=N" after model when ceiling > 0.
type TokenCeiling struct {
	Ceiling int
}

func (t TokenCeiling) Point() ports.HookPoint { return ports.HookAfterModel }
func (t TokenCeiling) Name() string           { return "token_ceiling" }

func (t TokenCeiling) Run(_ context.Context, hc *ports.HookContext) error {
	if t.Ceiling <= 0 {
		return nil
	}
	for _, n := range hc.Notes {
		var used int
		if _, err := fmt.Sscanf(n, "usage_total=%d", &used); err == nil {
			if used > t.Ceiling {
				return ports.VetoError{Reason: fmt.Sprintf("token ceiling exceeded: %d > %d", used, t.Ceiling)}
			}
		}
	}
	return nil
}
