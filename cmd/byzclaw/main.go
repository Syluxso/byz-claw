package main

import (
	"context"
	"encoding/json"
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
		verbose := fs.Bool("verbose", false, "echo audit JSON to stderr")
		_ = fs.Parse(args)
		if strings.TrimSpace(*text) == "" {
			fmt.Fprintln(os.Stderr, "run: --text is required")
			os.Exit(2)
		}
		app.Verbose = *verbose
		root, err := home.Resolve(*homeFlag)
		must(err)
		rt, err := app.OpenRuntime(root)
		must(err)
		defer rt.Close()
		_ = rt.Loop.RecoverIncomplete(context.Background())
		out, err := rt.Loop.Handle(context.Background(), ports.Inbound{
			Channel: "cli", SessionID: *session, UserID: "local", Text: *text, Kind: "user",
		})
		must(err)
		fmt.Println(out.Text)
	case "gateway":
		fs := flag.NewFlagSet("gateway", flag.ExitOnError)
		homeFlag := fs.String("home", "", "BYZCLAW_HOME")
		noCLI := fs.Bool("no-cli", false, "disable stdin REPL")
		webhook := fs.Bool("webhook", false, "force-enable webhook channel")
		telegram := fs.Bool("telegram", false, "force-enable telegram channel")
		verbose := fs.Bool("verbose", false, "echo audit JSON to stderr")
		_ = fs.Parse(args)
		app.Verbose = *verbose
		root, err := home.Resolve(*homeFlag)
		must(err)
		must(app.RunGateway(root, app.GatewayOptions{
			CLI: !*noCLI, Webhook: *webhook, Telegram: *telegram,
		}))
	case "task":
		must(runTask(args))
	default:
		fmt.Fprintf(os.Stderr, "byzclaw: unknown command %q\n", cmd)
		printHelp()
		os.Exit(2)
	}
}

func runTask(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: byzclaw task list|show|complete [--home DIR] ...")
	}
	sub := args[0]
	fs := flag.NewFlagSet("task", flag.ExitOnError)
	homeFlag := fs.String("home", "", "BYZCLAW_HOME")
	id := fs.String("id", "", "task id")
	_ = fs.Parse(args[1:])
	root, err := home.Resolve(*homeFlag)
	if err != nil {
		return err
	}
	rt, err := app.OpenRuntime(root)
	if err != nil {
		return err
	}
	defer rt.Close()
	ctx := context.Background()
	switch sub {
	case "list":
		list, err := rt.Tasks.List(ctx, ports.TaskFilter{Limit: 50})
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(list, "", "  ")
		fmt.Println(string(b))
	case "show":
		if *id == "" {
			return fmt.Errorf("task show requires --id")
		}
		t, err := rt.Tasks.Get(ctx, *id)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(t, "", "  ")
		fmt.Println(string(b))
	case "complete":
		if *id == "" {
			return fmt.Errorf("task complete requires --id")
		}
		t, err := rt.Tasks.Get(ctx, *id)
		if err != nil {
			return err
		}
		// CLI can complete user tasks (human gate).
		if err := rt.Tasks.Complete(ctx, t.ID); err != nil {
			return err
		}
		if t.Kind == "approval" && t.ParentID != "" {
			_ = rt.Tasks.Complete(ctx, t.ParentID)
		}
		fmt.Println("completed", t.ID)
	default:
		return fmt.Errorf("unknown task subcommand %q", sub)
	}
	return nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `byzclaw — personal AI claw (plan v4)

Usage:
  byzclaw onboard [--home DIR] [--yes]
  byzclaw doctor  [--home DIR]
  byzclaw run --text "..." [--home DIR] [--session ID] [--verbose]
  byzclaw gateway [--home DIR] [--no-cli] [--webhook] [--telegram] [--verbose]
  byzclaw task list|show|complete [--home DIR] [--id ID]
  byzclaw version

Design: docs/CODE_FIRST_LLM_LAST.md (wins over "more prompt")
Plan:   BYZCLAW_BUILD_PLAN.md

Gateway wakes only from inbox; scheduler mints tasks (no LLM).
`)
}
