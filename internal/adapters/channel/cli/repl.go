package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// Channel is a stdin REPL that posts user lines to the inbox.
type Channel struct {
	In  io.Reader
	Out io.Writer
}

func (c *Channel) Name() string { return "cli" }

func (c *Channel) Start(ctx context.Context, inbox chan<- ports.Inbound) error {
	in := c.In
	if in == nil {
		in = os.Stdin
	}
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	go func() {
		sc := bufio.NewScanner(in)
		fmt.Fprintln(out, "byzclaw gateway CLI ready. Type a message (Ctrl+C to stop).")
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			fmt.Fprint(out, "> ")
			if !sc.Scan() {
				return
			}
			text := strings.TrimSpace(sc.Text())
			if text == "" {
				continue
			}
			if text == "/quit" || text == "/exit" {
				return
			}
			msg := ports.Inbound{
				Channel:   "cli",
				SessionID: "cli:local",
				UserID:    "local",
				Text:      text,
				Kind:      "user",
			}
			select {
			case inbox <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

func (c *Channel) Send(_ context.Context, msg ports.Outbound) error {
	out := c.Out
	if out == nil {
		out = os.Stdout
	}
	_, err := fmt.Fprintf(out, "\n%s\n> ", msg.Text)
	return err
}
