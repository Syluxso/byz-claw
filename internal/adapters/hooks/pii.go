package hooks

import (
	"context"
	"regexp"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// PII is best-effort regex redaction before the model (not compliance).
type PII struct{}

func (PII) Point() ports.HookPoint { return ports.HookOnInbound }
func (PII) Name() string           { return "pii" }

var (
	reEmail = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	reSSN   = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	reCard  = regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`)
)

func (PII) Run(_ context.Context, hc *ports.HookContext) error {
	if hc.Inbound == nil {
		return nil
	}
	t := hc.Inbound.Text
	t = reEmail.ReplaceAllString(t, "[redacted-email]")
	t = reSSN.ReplaceAllString(t, "[redacted-ssn]")
	t = reCard.ReplaceAllString(t, "[redacted-card]")
	hc.Inbound.Text = t
	return nil
}
