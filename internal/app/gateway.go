package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Syluxso/byzclaw/internal/adapters/channel/cli"
	"github.com/Syluxso/byzclaw/internal/adapters/channel/telegram"
	"github.com/Syluxso/byzclaw/internal/adapters/channel/webhook"
	"github.com/Syluxso/byzclaw/internal/adapters/secrets"
	"github.com/Syluxso/byzclaw/internal/core"
	"github.com/Syluxso/byzclaw/internal/ports"
	clawlog "github.com/Syluxso/byzclaw/log"
)

// GatewayOptions controls which channels start.
type GatewayOptions struct {
	CLI      bool // stdin REPL unless --no-cli
	Webhook  bool // force-enable webhook
	Telegram bool // force-enable telegram
}

// RunGateway doctors, recovers incomplete runs, starts channels, and serves the inbox.
func RunGateway(homeRoot string, opts GatewayOptions) error {
	findings, err := Doctor(homeRoot)
	if err != nil {
		return err
	}
	for _, f := range findings {
		fmt.Printf("[%s] %s: %s\n", f.Level, f.Check, f.Message)
	}
	if DoctorHasCritical(findings) {
		return fmt.Errorf("doctor critical — fix before gateway")
	}

	rt, err := OpenRuntime(homeRoot)
	if err != nil {
		return err
	}
	defer rt.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rt.Loop.RecoverIncomplete(ctx); err != nil {
		clawlog.Info("recover_incomplete", map[string]any{"error": err.Error()})
	}

	inbox := make(chan ports.Inbound, 32)
	var channels []ports.Channel
	sec := &secrets.FileSecrets{Dir: rt.Home.Secrets}

	// opts.CLI is true unless --no-cli; still requires channels.cli.enabled in config.
	if opts.CLI && rt.Config.Channels.CLI.Enabled {
		ch := &cli.Channel{}
		if err := ch.Start(ctx, inbox); err != nil {
			return fmt.Errorf("cli channel: %w", err)
		}
		channels = append(channels, ch)
		rt.Loop.Channel = ch
		clawlog.Info("channel_started", map[string]any{"channel": "cli"})
	}

	whEnabled := rt.Config.Channels.Webhook.Enabled || opts.Webhook
	if whEnabled {
		token := ""
		if v, err := sec.Get(ctx, "webhook_token"); err == nil {
			token = v
		}
		addr := rt.Config.Channels.Webhook.Addr
		if rt.Config.Channels.Webhook.AllowPublic && token == "" {
			return fmt.Errorf("webhook allow_public requires secrets/webhook_token")
		}
		ch := &webhook.Channel{
			Addr:        addr,
			Path:        rt.Config.Channels.Webhook.Path,
			AllowPublic: rt.Config.Channels.Webhook.AllowPublic,
			Token:       token,
		}
		if err := ch.Start(ctx, inbox); err != nil {
			return fmt.Errorf("webhook channel: %w", err)
		}
		channels = append(channels, ch)
		clawlog.Info("channel_started", map[string]any{
			"channel": "webhook",
			"addr":    addr,
			"path":    rt.Config.Channels.Webhook.Path,
			"token":   token != "",
		})
	}

	tgEnabled := rt.Config.Channels.Telegram.Enabled || opts.Telegram
	if tgEnabled {
		if len(rt.Config.Channels.Telegram.AllowFrom) == 0 {
			return fmt.Errorf("telegram enabled but allow_from is empty")
		}
		tokName := rt.Config.Channels.Telegram.TokenSecret
		if tokName == "" {
			tokName = "telegram_bot"
		}
		token, err := sec.Get(ctx, tokName)
		if err != nil || strings.TrimSpace(token) == "" {
			return fmt.Errorf("telegram requires secrets/%s", tokName)
		}
		ch := &telegram.Channel{
			Token:     token,
			AllowFrom: rt.Config.Channels.Telegram.AllowFrom,
		}
		if err := ch.Start(ctx, inbox); err != nil {
			return fmt.Errorf("telegram channel: %w", err)
		}
		channels = append(channels, ch)
		clawlog.Info("channel_started", map[string]any{
			"channel":    "telegram",
			"allow_from": len(rt.Config.Channels.Telegram.AllowFrom),
		})
	}

	if rt.Config.Heartbeat.Enabled {
		interval, err := rt.Config.HeartbeatInterval()
		if err != nil {
			return fmt.Errorf("heartbeat interval: %w", err)
		}
		loc := time.Local
		if tz := rt.Config.Heartbeat.QuietHours.Timezone; tz != "" && !strings.EqualFold(tz, "local") {
			if l, err := time.LoadLocation(tz); err == nil {
				loc = l
			}
		}
		hb := core.HeartbeatConfig{
			Enabled:  true,
			Interval: interval,
			Start:    rt.Config.Heartbeat.QuietHours.Start,
			End:      rt.Config.Heartbeat.QuietHours.End,
			Location: loc,
			FilePath: rt.Home.Heartbeat,
			Session:  "heartbeat:local",
		}
		if err := core.StartHeartbeat(ctx, hb, inbox); err != nil {
			return fmt.Errorf("heartbeat: %w", err)
		}
		clawlog.Info("heartbeat_started", map[string]any{"interval": interval.String()})
	}

	if len(channels) == 0 && !rt.Config.Heartbeat.Enabled {
		return fmt.Errorf("no channels enabled (cli / webhook / telegram) and heartbeat disabled")
	}

	clawlog.Info("gateway_ready", map[string]any{"home": homeRoot, "channels": len(channels)})

	for {
		select {
		case <-ctx.Done():
			clawlog.Info("gateway_stop", map[string]any{"reason": "signal"})
			return nil
		case in := <-inbox:
			start := time.Now()
			out, err := rt.Loop.Handle(ctx, in)
			if err != nil {
				clawlog.Info("run_error", map[string]any{
					"session": in.SessionID,
					"error":   err.Error(),
					"ms":      time.Since(start).Milliseconds(),
				})
				continue
			}
			clawlog.Info("run_ok", map[string]any{
				"session": in.SessionID,
				"kind":    in.Kind,
				"ms":      time.Since(start).Milliseconds(),
			})
			replied := false
			for _, ch := range channels {
				if ch.Name() == in.Channel {
					_ = ch.Send(ctx, out)
					replied = true
					break
				}
			}
			if !replied && rt.Loop.Channel != nil {
				_ = rt.Loop.Channel.Send(ctx, out)
			}
		}
	}
}
