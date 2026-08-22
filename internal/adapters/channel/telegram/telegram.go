package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// Channel long-polls getUpdates and sends replies via sendMessage.
type Channel struct {
	Token     string
	AllowFrom []string // required non-empty when enabled (chat id and/or @username)
	HTTP      *http.Client
	apiBase   string
}

func (c *Channel) Name() string { return "telegram" }

func (c *Channel) api(method string) string {
	base := c.apiBase
	if base == "" {
		base = "https://api.telegram.org"
	}
	return strings.TrimRight(base, "/") + "/bot" + c.Token + "/" + method
}

func (c *Channel) Start(ctx context.Context, inbox chan<- ports.Inbound) error {
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("telegram: token required")
	}
	if len(c.AllowFrom) == 0 {
		return fmt.Errorf("telegram: allow_from must be non-empty")
	}
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 60 * time.Second}
	}
	go c.poll(ctx, inbox)
	return nil
}

func (c *Channel) poll(ctx context.Context, inbox chan<- ports.Inbound) {
	offset := int64(0)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		updates, err := c.getUpdates(ctx, offset)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			msg := u.Message
			if msg == nil || strings.TrimSpace(msg.Text) == "" {
				continue
			}
			fromID := ""
			fromUser := ""
			if msg.From != nil {
				fromID = strconv.FormatInt(msg.From.ID, 10)
				fromUser = strings.TrimPrefix(msg.From.Username, "@")
			}
			chatID := strconv.FormatInt(msg.Chat.ID, 10)
			if !allowed(c.AllowFrom, chatID, fromID, fromUser) {
				continue
			}
			in := ports.Inbound{
				Channel:   "telegram",
				SessionID: "telegram:" + chatID,
				UserID:    fromID,
				Text:      msg.Text,
				Kind:      "user",
				Meta:      map[string]string{"chat_id": chatID},
			}
			select {
			case inbox <- in:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (c *Channel) Send(ctx context.Context, msg ports.Outbound) error {
	chatID := strings.TrimPrefix(msg.SessionID, "telegram:")
	if chatID == "" || chatID == msg.SessionID {
		if msg.Meta != nil {
			chatID = msg.Meta["chat_id"]
		}
	}
	if chatID == "" {
		return fmt.Errorf("telegram: missing chat id")
	}
	body := map[string]any{
		"chat_id": chatID,
		"text":    msg.Text,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.api("sendMessage"), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram sendMessage %d: %s", resp.StatusCode, truncate(string(b), 200))
	}
	return nil
}

type tgUpdate struct {
	UpdateID int64     `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	Text string  `json:"text"`
	Chat tgChat  `json:"chat"`
	From *tgUser `json:"from"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type tgUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type tgResp struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

func (c *Channel) getUpdates(ctx context.Context, offset int64) ([]tgUpdate, error) {
	q := fmt.Sprintf("?timeout=25&offset=%d&allowed_updates=%s", offset, "%5B%22message%22%5D")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.api("getUpdates")+q, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var parsed tgResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if !parsed.OK {
		return nil, fmt.Errorf("telegram getUpdates not ok: %s", truncate(string(raw), 200))
	}
	return parsed.Result, nil
}

func allowed(allow []string, ids ...string) bool {
	set := map[string]bool{}
	for _, a := range allow {
		a = strings.TrimSpace(a)
		a = strings.TrimPrefix(a, "@")
		if a != "" {
			set[a] = true
		}
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		id = strings.TrimPrefix(id, "@")
		if id != "" && set[id] {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
