package app

import (
	"context"
	"encoding/json"
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
	CLI      bool
	Webhook  bool
	Telegram bool
}

// RunGateway: doctor → recover → channels + scheduler → inbox drain loop.
// Agent works only when inbox has work (Code First / plan v4).
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

	channelInbox := make(chan ports.Inbound, 32)
	var channels []ports.Channel
	sec := &secrets.FileSecrets{Dir: rt.Home.Secrets}

	if opts.CLI && rt.Config.Channels.CLI.Enabled {
		ch := &cli.Channel{}
		if err := ch.Start(ctx, channelInbox); err != nil {
			return fmt.Errorf("cli channel: %w", err)
		}
		channels = append(channels, ch)
		rt.Loop.Channel = ch
		clawlog.Info("channel_started", map[string]any{"channel": "cli"})
	}

	if rt.Config.Channels.Webhook.Enabled || opts.Webhook {
		token := ""
		if v, err := sec.Get(ctx, "webhook_token"); err == nil {
			token = v
		}
		if rt.Config.Channels.Webhook.AllowPublic && token == "" {
			return fmt.Errorf("webhook allow_public requires secrets/webhook_token")
		}
		ch := &webhook.Channel{
			Addr: rt.Config.Channels.Webhook.Addr, Path: rt.Config.Channels.Webhook.Path,
			AllowPublic: rt.Config.Channels.Webhook.AllowPublic, Token: token,
		}
		if err := ch.Start(ctx, channelInbox); err != nil {
			return fmt.Errorf("webhook channel: %w", err)
		}
		channels = append(channels, ch)
		clawlog.Info("channel_started", map[string]any{"channel": "webhook", "addr": ch.Addr})
	}

	if rt.Config.Channels.Telegram.Enabled || opts.Telegram {
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
		ch := &telegram.Channel{Token: token, AllowFrom: rt.Config.Channels.Telegram.AllowFrom}
		if err := ch.Start(ctx, channelInbox); err != nil {
			return fmt.Errorf("telegram channel: %w", err)
		}
		channels = append(channels, ch)
		clawlog.Info("channel_started", map[string]any{"channel": "telegram"})
	}

	if rt.Config.Scheduler.Enabled {
		tick, err := rt.Config.SchedulerTick()
		if err != nil {
			return fmt.Errorf("scheduler tick: %w", err)
		}
		loc := time.Local
		if tz := rt.Config.Scheduler.QuietHours.Timezone; tz != "" && !strings.EqualFold(tz, "local") {
			if l, e := time.LoadLocation(tz); e == nil {
				loc = l
			}
		}
		sch := &core.Scheduler{
			Schedules: rt.Schedules,
			Tasks:     rt.Tasks,
			Inbox:     rt.Inbox,
			Clock:     ports.RealClock{},
			Config: core.SchedulerConfig{
				Enabled: true, Tick: tick,
				QuietStart: rt.Config.Scheduler.QuietHours.Start,
				QuietEnd:   rt.Config.Scheduler.QuietHours.End,
				Location:   loc,
			},
		}
		if err := sch.Start(ctx); err != nil {
			return err
		}
		clawlog.Info("scheduler_started", map[string]any{"tick": tick.String()})
	}

	if len(channels) == 0 && !rt.Config.Scheduler.Enabled {
		return fmt.Errorf("no channels and scheduler disabled — nothing to run")
	}

	clawlog.Info("gateway_ready", map[string]any{"home": homeRoot, "channels": len(channels)})

	// Drain any pending inbox from previous process.
	drainInbox(ctx, rt, channels)

	for {
		select {
		case <-ctx.Done():
			clawlog.Info("gateway_stop", map[string]any{"reason": "signal"})
			return nil
		case in := <-channelInbox:
			acceptInbound(ctx, rt, in)
			drainInbox(ctx, rt, channels)
		case <-time.After(2 * time.Second):
			// Periodic drain (scheduler may have enqueued wakes).
			drainInbox(ctx, rt, channels)
		}
	}
}

func acceptInbound(ctx context.Context, rt *Runtime, in ports.Inbound) {
	if in.SessionID == "" {
		in.SessionID = "cli:local"
	}
	if rt.Loop.IsBusy(in.SessionID) {
		payload, _ := json.Marshal(map[string]string{"text": in.Text, "user_id": in.UserID})
		ext := in.Meta["external_id"]
		if ext == "" && in.Text != "" {
			ext = "msg:" + hashShort(in.Text)
		}
		n, _ := rt.Inbox.CountPending(ctx, in.SessionID)
		max := rt.Config.Inbox.MaxPendingPerSession
		if max <= 0 {
			max = 20
		}
		if n >= max {
			if rt.Config.Inbox.Drop == "refuse" {
				clawlog.Info("inbox_refuse", map[string]any{"session": in.SessionID})
				return
			}
			// drop oldest
			if pending, err := rt.Inbox.ListPending(ctx, in.SessionID, 1); err == nil && len(pending) > 0 {
				_ = rt.Inbox.Drop(ctx, pending[0].ID, "cap_oldest")
			}
		}
		_, _ = rt.Inbox.Enqueue(ctx, ports.InboxItem{
			SessionID: in.SessionID, Channel: in.Channel, ExternalID: ext,
			Kind: "user_message", Payload: payload, State: ports.InboxPending,
		})
		clawlog.Info("inbox_enqueued", map[string]any{"session": in.SessionID})
		return
	}
	// Idle → synthesize inbox item and handle immediately (sole wake path still via Handle).
	payload, _ := json.Marshal(map[string]string{"text": in.Text, "user_id": in.UserID})
	item, err := rt.Inbox.Enqueue(ctx, ports.InboxItem{
		SessionID: in.SessionID, Channel: in.Channel, Kind: "user_message",
		Payload: payload, State: ports.InboxPending,
	})
	if err != nil {
		clawlog.Info("inbox_enqueue_error", map[string]any{"error": err.Error()})
		return
	}
	handleInboxItem(ctx, rt, item, nil)
}

func drainInbox(ctx context.Context, rt *Runtime, channels []ports.Channel) {
	items, err := rt.Inbox.ListPendingAny(ctx, 20)
	if err != nil {
		return
	}
	for _, item := range items {
		if rt.Loop.IsBusy(item.SessionID) {
			continue
		}
		handleInboxItem(ctx, rt, item, channels)
	}
}

func handleInboxItem(ctx context.Context, rt *Runtime, item ports.InboxItem, channels []ports.Channel) {
	text := ""
	userID := ""
	var payload map[string]string
	_ = json.Unmarshal(item.Payload, &payload)
	if payload != nil {
		text = payload["text"]
		userID = payload["user_id"]
	}
	if text == "" && item.Kind == "task_wake" {
		text = "A task needs attention. Use task_list / task_get / task_complete tools. Task id: " + item.TaskID
	}
	in := ports.Inbound{
		Channel: item.Channel, SessionID: item.SessionID, UserID: userID,
		Text: text, Kind: item.Kind,
	}
	if in.Channel == "" {
		in.Channel = "system"
	}
	rt.Loop.InboxItemID = item.ID
	start := time.Now()
	out, err := rt.Loop.Handle(ctx, in)
	rt.Loop.InboxItemID = ""
	if err != nil {
		clawlog.Info("run_error", map[string]any{"session": in.SessionID, "error": err.Error()})
		return
	}
	clawlog.Info("run_ok", map[string]any{"session": in.SessionID, "ms": time.Since(start).Milliseconds()})
	for _, ch := range channels {
		if ch.Name() == in.Channel {
			_ = ch.Send(ctx, out)
			return
		}
	}
	if rt.Loop.Channel != nil {
		_ = rt.Loop.Channel.Send(ctx, out)
	}
}

func hashShort(s string) string {
	sum := 0
	for _, r := range s {
		sum = sum*31 + int(r)
	}
	if sum < 0 {
		sum = -sum
	}
	return fmt.Sprintf("%x", sum)
}
