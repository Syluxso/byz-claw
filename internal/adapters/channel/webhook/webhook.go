package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// Channel receives POSTs and forwards Inbound messages.
type Channel struct {
	Addr        string
	Path        string
	AllowPublic bool
	Token       string // optional shared secret (Authorization: Bearer … or X-Byzclaw-Token)
	server      *http.Server
}

func (c *Channel) Name() string { return "webhook" }

type payload struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
	UserID    string `json:"user_id"`
}

func (c *Channel) Start(ctx context.Context, inbox chan<- ports.Inbound) error {
	path := c.Path
	if path == "" {
		path = "/hook"
	}
	addr := c.Addr
	if addr == "" {
		addr = "127.0.0.1:8743"
	}
	if err := assertBindSafe(addr, c.AllowPublic); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if !c.authorize(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		var p payload
		if err := json.Unmarshal(raw, &p); err != nil || strings.TrimSpace(p.Text) == "" {
			http.Error(w, `need JSON {"session_id","text"}`, http.StatusBadRequest)
			return
		}
		sid := strings.TrimSpace(p.SessionID)
		if sid == "" {
			sid = "webhook:default"
		}
		if !strings.HasPrefix(sid, "webhook:") {
			sid = "webhook:" + sid
		}
		uid := strings.TrimSpace(p.UserID)
		if uid == "" {
			uid = "webhook"
		}
		msg := ports.Inbound{
			Channel:   "webhook",
			SessionID: sid,
			UserID:    uid,
			Text:      p.Text,
			Kind:      "user",
		}
		select {
		case inbox <- msg:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "session_id": sid})
		case <-r.Context().Done():
			http.Error(w, "cancelled", http.StatusRequestTimeout)
		case <-time.After(5 * time.Second):
			http.Error(w, "busy", http.StatusServiceUnavailable)
		}
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"UP","channel":"webhook"}`))
	})

	c.server = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.server.Shutdown(shut)
	}()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		_ = c.server.Serve(ln)
	}()
	return nil
}

func (c *Channel) Send(_ context.Context, _ ports.Outbound) error {
	// Fire-and-forget ingest; replies are not pushed back over HTTP in v1.
	return nil
}

func (c *Channel) authorize(r *http.Request) bool {
	if c.Token == "" {
		// Loopback-only installs may omit token; public binds must set one (enforced at start).
		return true
	}
	want := c.Token
	got := strings.TrimSpace(r.Header.Get("X-Byzclaw-Token"))
	if got == "" {
		h := r.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(h), "bearer ") {
			got = strings.TrimSpace(h[7:])
		}
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func assertBindSafe(addr string, allowPublic bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// addr might be ":8743"
		if strings.HasPrefix(addr, ":") {
			host = ""
		} else {
			return err
		}
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		if !allowPublic {
			return fmt.Errorf("webhook public bind requires allow_public: true (and a token)")
		}
	}
	return nil
}
