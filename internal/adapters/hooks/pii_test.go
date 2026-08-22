package hooks

import (
	"context"
	"testing"

	"github.com/Syluxso/byzclaw/internal/ports"
)

func TestPIIRedactsEmail(t *testing.T) {
	in := &ports.Inbound{Text: "mail me at a.b@example.com please"}
	h := PII{}
	if err := h.Run(context.Background(), &ports.HookContext{Inbound: in}); err != nil {
		t.Fatal(err)
	}
	if in.Text == "mail me at a.b@example.com please" {
		t.Fatalf("expected redaction, got %q", in.Text)
	}
	if !contains(in.Text, "[redacted-email]") {
		t.Fatalf("missing marker: %q", in.Text)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
