package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Syluxso/byzclaw/internal/app"
	"github.com/Syluxso/byzclaw/internal/home"
	"github.com/Syluxso/byzclaw/internal/ports"
)

// Version is overridden via -ldflags at release time.
var Version = "dev"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "version":
		fmt.Println(Version)
	case "help", "-h", "--help":
		printHelp()
	case "onboard":
		fs := flag.NewFlagSet("onboard", flag.ExitOnError)
		homeFlag := fs.String("home", "", "BYZCLAW_HOME (default ~/.byzclaw)")
		yes := fs.Bool("yes", false, "non-interactive (skip API key prompt)")
		_ = fs.Parse(args)
		root, err := home.Resolve(*homeFlag)
		must(err)
		must(app.Onboard(root, !*yes))
	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ExitOnError)
		homeFlag := fs.String("home", "", "BYZCLAW_HOME")
		_ = fs.Parse(args)
		root, err := home.Resolve(*homeFlag)
		must(err)
		findings, err := app.Doctor(root)
		must(err)
		for _, f := range findings {
			fmt.Printf("[%s] %s: %s\n", f.Level, f.Check, f.Message)
		}
		if app.DoctorHasCritical(findings) {
			os.Exit(1)
		}
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		homeFlag := fs.String("home", "", "BYZCLAW_HOME")
		text := fs.String("text", "", "user message")
		session := fs.String("session", "cli:local", "session id")
		_ = fs.Parse(args)
		if strings.TrimSpace(*text) == "" {
			fmt.Fprintln(os.Stderr, "run: --text is required")
			os.Exit(2)
		}
		root, err := home.Resolve(*homeFlag)
		must(err)
		rt, err := app.OpenRuntime(root)
		must(err)
		defer rt.Close()
		_ = rt.Loop.RecoverIncomplete(context.Background())
		out, err := rt.Loop.Handle(context.Background(), ports.Inbound{
			Channel:   "cli",
			SessionID: *session,
			UserID:    "local",
			Text:      *text,
			Kind:      "user",
		})
		must(err)
		fmt.Println(out.Text)
	case "gateway":
		fmt.Fprintln(os.Stderr, "gateway: not fully wired yet (use run --text for now)")
		os.Exit(2)
	default:
		fmt.Fprintf(os.Stderr, "byzclaw: unknown command %q\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `byzclaw — personal AI claw

Usage:
  byzclaw onboard [--home DIR] [--yes]
  byzclaw doctor  [--home DIR]
  byzclaw run --text "..." [--home DIR] [--session ID]
  byzclaw version
  byzclaw help

Environment:
  BYZCLAW_HOME   default ~/.byzclaw
`)
}
