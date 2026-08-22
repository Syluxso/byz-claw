package app

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Syluxso/byzclaw/config"
	"github.com/Syluxso/byzclaw/internal/adapters/secrets"
	"github.com/Syluxso/byzclaw/internal/home"
)

func Onboard(homeRoot string, interactive bool) error {
	p := home.PathsFor(homeRoot)
	if err := home.EnsureLayout(p); err != nil {
		return err
	}

	cfg := config.Default()
	if _, err := os.Stat(p.Config); err == nil {
		fmt.Println("config.yaml already exists — keeping it")
		existing, err := config.Load(p.Config)
		if err == nil {
			cfg = existing
		}
	} else {
		if err := config.Write(p.Config, cfg); err != nil {
			return err
		}
		fmt.Println("wrote", p.Config)
	}

	writeIfMissing(p.Soul, "# Soul\n\nYou are a helpful personal assistant running as byzclaw.\n")
	writeIfMissing(p.MemoryMD, "# Memory\n\nLong-term notes live here and under memory/.\n")
	writeIfMissing(p.Heartbeat, "# Heartbeat\n\nInstructions for periodic heartbeat runs.\n")

	sec := &secrets.FileSecrets{Dir: p.Secrets}
	secretPath := p.Secrets + string(os.PathSeparator) + cfg.Model.APIKeySecret
	if _, err := os.Stat(secretPath); err != nil && interactive {
		fmt.Printf("Enter API key for model secret %q (leave blank to skip): ", cfg.Model.APIKeySecret)
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			if err := sec.Put(cfg.Model.APIKeySecret, line); err != nil {
				return err
			}
			fmt.Println("saved secret (0600)")
		}
	} else if _, err := os.Stat(secretPath); err != nil {
		fmt.Println("skipped API key prompt (non-interactive); set secrets/" + cfg.Model.APIKeySecret)
	}

	findings, err := Doctor(homeRoot)
	if err != nil {
		return err
	}
	fmt.Println("\ndoctor:")
	for _, f := range findings {
		fmt.Printf("  [%s] %s: %s\n", f.Level, f.Check, f.Message)
	}
	if DoctorHasCritical(findings) {
		return fmt.Errorf("doctor reported critical issues")
	}
	fmt.Println("\nonboard complete. Try: byzclaw run --text \"write hello to workspace/hi.md\"")
	return nil
}

func writeIfMissing(path, content string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	_ = os.WriteFile(path, []byte(content), 0o644)
	fmt.Println("wrote", path)
}
