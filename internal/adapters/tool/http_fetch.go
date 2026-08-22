package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

type HTTPFetch struct {
	MaxBytes int64
	Timeout  time.Duration
	Client   *http.Client
}

func NewHTTPFetch(maxBytes int64, timeoutSec int) *HTTPFetch {
	if maxBytes <= 0 {
		maxBytes = 2_000_000
	}
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &HTTPFetch{
		MaxBytes: maxBytes,
		Timeout:  time.Duration(timeoutSec) * time.Second,
		Client:   &http.Client{Timeout: time.Duration(timeoutSec) * time.Second, CheckRedirect: rejectBadRedirect},
	}
}

func (h *HTTPFetch) Name() string { return "http_fetch" }
func (h *HTTPFetch) Description() string {
	return "Fetch a public http(s) URL and return text body (SSRF-safe: blocks private/loopback/metadata)."
}
func (h *HTTPFetch) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`)
}

func (h *HTTPFetch) Exec(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil || strings.TrimSpace(args.URL) == "" {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: "url required", IsError: true}, nil
	}
	if err := validatePublicURL(args.URL); err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, nil)
	if err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	req.Header.Set("User-Agent", "byzclaw/1.0")
	resp, err := h.Client.Do(req)
	if err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, h.MaxBytes+1))
	if err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	if int64(len(body)) > h.MaxBytes {
		body = body[:h.MaxBytes]
	}
	return ports.ToolResult{
		ID: call.ID, Name: call.Name,
		Content: fmt.Sprintf("status=%d\n%s", resp.StatusCode, string(body)),
	}, nil
}

func rejectBadRedirect(req *http.Request, _ []*http.Request) error {
	return validatePublicURL(req.URL.String())
}

func validatePublicURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https allowed")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("missing host")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "metadata.google.internal" {
		return fmt.Errorf("blocked host")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// If literal IP:
		if ip := net.ParseIP(host); ip != nil {
			if isBadIP(ip) {
				return fmt.Errorf("blocked address")
			}
			return nil
		}
		return fmt.Errorf("dns lookup failed: %w", err)
	}
	for _, ip := range ips {
		if isBadIP(ip) {
			return fmt.Errorf("blocked address")
		}
	}
	return nil
}

func isBadIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// AWS/GCP metadata
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
	}
	return false
}
