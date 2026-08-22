package core

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// HeartbeatConfig is the runtime view of config.heartbeat.
type HeartbeatConfig struct {
	Enabled  bool
	Interval time.Duration
	Start    string // "HH:MM"
	End      string // "HH:MM"
	Location *time.Location
	FilePath string // HEARTBEAT.md
	Session  string // default heartbeat session key
}

// StartHeartbeat ticks Inbound{Kind:heartbeat} into inbox, skipping quiet hours.
func StartHeartbeat(ctx context.Context, cfg HeartbeatConfig, inbox chan<- ports.Inbound) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.Interval <= 0 {
		cfg.Interval = time.Hour
	}
	if cfg.Session == "" {
		cfg.Session = "heartbeat:local"
	}
	if cfg.Location == nil {
		cfg.Location = time.Local
	}
	go func() {
		t := time.NewTicker(cfg.Interval)
		defer t.Stop()
		// Optional immediate tick after a short delay so gateway logs settle.
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
			fireHeartbeat(ctx, cfg, inbox)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				fireHeartbeat(ctx, cfg, inbox)
			}
		}
	}()
	return nil
}

func fireHeartbeat(ctx context.Context, cfg HeartbeatConfig, inbox chan<- ports.Inbound) {
	if InQuietHours(time.Now().In(cfg.Location), cfg.Start, cfg.End) {
		return
	}
	text := "Heartbeat tick. Follow HEARTBEAT.md instructions if any."
	if cfg.FilePath != "" {
		if b, err := os.ReadFile(cfg.FilePath); err == nil && len(b) > 0 {
			text = strings.TrimSpace(string(b))
		}
	}
	msg := ports.Inbound{
		Channel:   "heartbeat",
		SessionID: cfg.Session,
		UserID:    "heartbeat",
		Text:      text,
		Kind:      "heartbeat",
	}
	select {
	case inbox <- msg:
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		// drop if inbox saturated
	}
}

// InQuietHours reports whether now is inside [start, end) local wall times (HH:MM).
// Supports overnight windows (e.g. 23:00–07:00).
func InQuietHours(now time.Time, start, end string) bool {
	if start == "" || end == "" {
		return false
	}
	sh, sm, err1 := parseHHMM(start)
	eh, em, err2 := parseHHMM(end)
	if err1 != nil || err2 != nil {
		return false
	}
	mins := now.Hour()*60 + now.Minute()
	s := sh*60 + sm
	e := eh*60 + em
	if s == e {
		return false
	}
	if s < e {
		return mins >= s && mins < e
	}
	// overnight
	return mins >= s || mins < e
}

func parseHHMM(s string) (h, m int, err error) {
	var hh, mm int
	_, err = fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &hh, &mm)
	if err != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, fmt.Errorf("bad time %q", s)
	}
	return hh, mm, nil
}
