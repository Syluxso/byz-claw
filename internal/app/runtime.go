package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Syluxso/byzclaw/config"
	"github.com/Syluxso/byzclaw/internal/adapters/model"
	"github.com/Syluxso/byzclaw/internal/adapters/secrets"
	storeadapter "github.com/Syluxso/byzclaw/internal/adapters/store"
	tooladapter "github.com/Syluxso/byzclaw/internal/adapters/tool"
	"github.com/Syluxso/byzclaw/internal/core"
	"github.com/Syluxso/byzclaw/internal/home"
	"github.com/Syluxso/byzclaw/internal/ports"
)

type Runtime struct {
	Home   home.Paths
	Config config.Config
	Store  *storeadapter.SQLite
	Loop   *core.Loop
	Model  ports.Model
}

func OpenRuntime(homeRoot string) (*Runtime, error) {
	p := home.PathsFor(homeRoot)
	cfg, err := config.Load(p.Config)
	if err != nil {
		return nil, fmt.Errorf("load config: %w (run: byzclaw onboard)", err)
	}
	if err := home.EnsureLayout(p); err != nil {
		return nil, err
	}

	st, err := storeadapter.Open(p.DB)
	if err != nil {
		return nil, err
	}

	wsRoot := filepath.Join(p.Root, cfg.Tools.Workspace.Root)
	if !filepath.IsAbs(cfg.Tools.Workspace.Root) {
		wsRoot = filepath.Join(p.Root, cfg.Tools.Workspace.Root)
	} else {
		wsRoot = cfg.Tools.Workspace.Root
	}
	wsPolicy, err := core.NewPathPolicy(wsRoot)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	wsPolicy.ShellEnabled = cfg.Tools.Shell.Enabled

	memPolicy, err := core.NewPathPolicy(p.Memory)
	if err != nil {
		_ = st.Close()
		return nil, err
	}

	tools := []ports.Tool{}
	tools = append(tools, (&tooladapter.Workspace{Policy: wsPolicy}).Tools()...)
	tools = append(tools, (&tooladapter.MemoryTools{Policy: memPolicy}).Tools()...)
	tools = append(tools, tooladapter.NewHTTPFetch(cfg.Tools.HTTPFetch.MaxBytes, cfg.Tools.HTTPFetch.TimeoutSeconds))

	sec := &secrets.FileSecrets{Dir: p.Secrets}
	apiKey, _ := sec.Get(context.Background(), cfg.Model.APIKeySecret)

	var mdl ports.Model
	switch cfg.Model.Provider {
	case "openai_compat", "":
		mdl = model.NewOpenAICompat(cfg.Model.BaseURL, apiKey, cfg.Model.Model)
	case "fake":
		mdl = &model.Fake{Queue: []ports.CompletionResponse{{Content: "fake ok"}}}
	default:
		_ = st.Close()
		return nil, fmt.Errorf("unknown model provider %q", cfg.Model.Provider)
	}

	loopCfg := core.LoopConfig{
		MaxToolIterations:     cfg.Loop.MaxToolIterations,
		CompactTokenThreshold: cfg.Loop.CompactTokenThreshold,
		CompactTailMessages:   cfg.Loop.CompactTailMessages,
		TokenCeiling:          cfg.Loop.TokenCeiling,
		Profile:               cfg.Loop.Profile,
	}
	loop := core.NewLoop(st, mdl, tools, wsPolicy, loopCfg)
	loop.SystemPrompt = buildSystemPrompt(p)

	return &Runtime{Home: p, Config: cfg, Store: st, Loop: loop, Model: mdl}, nil
}

func (r *Runtime) Close() error {
	if r == nil || r.Store == nil {
		return nil
	}
	return r.Store.Close()
}

func buildSystemPrompt(p home.Paths) string {
	var b string
	b += "You are byzclaw, a personal AI assistant with tools. Prefer tools for file and fetch work.\n"
	b += "Never claim you wrote a file or fetched a URL unless you called the matching tool.\n"
	if raw, err := os.ReadFile(p.Soul); err == nil && len(raw) > 0 {
		b += "\n# SOUL\n" + string(raw) + "\n"
	}
	if raw, err := os.ReadFile(p.MemoryMD); err == nil && len(raw) > 0 && len(raw) < 50_000 {
		b += "\n# MEMORY\n" + string(raw) + "\n"
	}
	return b
}
