package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Syluxso/byzclaw/config"
	"github.com/Syluxso/byzclaw/internal/adapters/hooks"
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
	Skills []ports.Skill
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

	wsRoot := cfg.Tools.Workspace.Root
	if !filepath.IsAbs(wsRoot) {
		wsRoot = filepath.Join(p.Root, wsRoot)
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

	allTools := []ports.Tool{}
	allTools = append(allTools, (&tooladapter.Workspace{Policy: wsPolicy}).Tools()...)
	allTools = append(allTools, (&tooladapter.MemoryTools{Policy: memPolicy}).Tools()...)
	allTools = append(allTools, tooladapter.NewHTTPFetch(cfg.Tools.HTTPFetch.MaxBytes, cfg.Tools.HTTPFetch.TimeoutSeconds))

	skillsDir := cfg.Skills.Dir
	if !filepath.IsAbs(skillsDir) {
		skillsDir = filepath.Join(p.Root, skillsDir)
	}
	// Also load bundled example skills from install cwd if home skills empty.
	skills, _ := core.LoadSkillsDir(skillsDir)
	if len(skills) == 0 {
		if bundled, err := core.LoadSkillsDir(filepath.Join(p.Root, "skills")); err == nil {
			skills = bundled
		}
	}

	toolMap := map[string]ports.Tool{}
	for _, t := range allTools {
		toolMap[t.Name()] = t
	}
	toolMap = core.FilterToolsBySkills(toolMap, skills)
	tools := make([]ports.Tool, 0, len(toolMap))
	for _, t := range toolMap {
		tools = append(tools, t)
	}

	sec := &secrets.FileSecrets{Dir: p.Secrets}
	apiKey, keyErr := sec.Get(context.Background(), cfg.Model.APIKeySecret)
	if keyErr != nil {
		apiKey = strings.TrimSpace(os.Getenv("XAI_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		}
	}

	var mdl ports.Model
	switch cfg.Model.Provider {
	case "fake":
		mdl = &model.Fake{Queue: []ports.CompletionResponse{{Content: "fake ok"}}}
	case "local_demo":
		mdl = model.LocalDemo{}
	case "openai_compat", "":
		if apiKey == "" {
			mdl = model.LocalDemo{}
		} else {
			mdl = model.NewOpenAICompat(cfg.Model.BaseURL, apiKey, cfg.Model.Model)
		}
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
	loop.SystemPrompt = buildSystemPrompt(p, skills)
	loop.Hooks = buildHooks(cfg)

	return &Runtime{Home: p, Config: cfg, Store: st, Loop: loop, Model: mdl, Skills: skills}, nil
}

func (r *Runtime) Close() error {
	if r == nil || r.Store == nil {
		return nil
	}
	return r.Store.Close()
}

func buildSystemPrompt(p home.Paths, skills []ports.Skill) string {
	var b string
	b += "You are byzclaw, a personal AI assistant with tools. Prefer tools for file and fetch work.\n"
	b += "Never claim you wrote a file or fetched a URL unless you called the matching tool.\n"
	if raw, err := os.ReadFile(p.Soul); err == nil && len(raw) > 0 {
		b += "\n# SOUL\n" + string(raw) + "\n"
	}
	if raw, err := os.ReadFile(p.MemoryMD); err == nil && len(raw) > 0 && len(raw) < 50_000 {
		b += "\n# MEMORY\n" + string(raw) + "\n"
	}
	b += core.SkillsPrompt(skills)
	return b
}

func buildHooks(cfg config.Config) []ports.Hook {
	wanted := map[string]bool{}
	for _, n := range cfg.Middleware {
		wanted[n] = true
	}
	if len(wanted) == 0 {
		wanted["audit"] = true
		wanted["pii"] = true
		wanted["token_ceiling"] = true
	}
	var out []ports.Hook
	if wanted["audit"] {
		out = append(out, hooks.AuditSuite()...)
	}
	if wanted["pii"] {
		out = append(out, hooks.PII{})
	}
	if wanted["token_ceiling"] {
		out = append(out, hooks.TokenCeiling{Ceiling: cfg.Loop.TokenCeiling})
	}
	return out
}
