# byz-claw — Grok Build Implementation Spec (v2)

Hand this entire file to local Grok Build. It is the source of truth.
Prefer this over inventing structure, over OpenClaw clones, and over "EnterpriseClaw" MVC demos.

**Product one-liner:** A highly opinionated single-binary personal AI claw in Go. OpenClaw's *idea*, PicoClaw-class footprint, defaults that will not burn the host.

**Not:** a K8s control plane, a Laravel of N custom `Agent.Execute` runtimes, Wasm/gVisor in v1, or a stdlib demo that claims 8MB while doing no tool loop.

---

## 0. Decisions locked

| Topic | Decision |
|---|---|
| Language | Go, vanilla `net/http` (1.22+ ServeMux). No Gin/Echo/Chi required. |
| Architecture | 12-factor daemon + standard Go `cmd/` `internal/` + hexagonal ports. |
| Loop | **Fixed state machine.** Not a plugin. Hooks hang off named stages. |
| Extensibility | Skills = markdown files. Tools/channels = Go adapters, explicit register. |
| DX for coding agents | Convention-over-configuration **layout and names**, not directory auto-scan of `.go`. |
| Core vs full | Same `internal/core`. Full only registers extra adapters (UI, browser later). |
| Enterprise v1 | Policy, audit, secrets, doctor, allowlists — not SSO mothership / Vault / CRDs. |
| Size honesty | Core target ≤ ~35MB binary, idle tens of MB RAM. Do **not** claim 7–10MB. |
| Build | `CGO_ENABLED=0` `-ldflags="-s -w"`. No UPX. Prefer `modernc.org/sqlite` (no CGO). |

**Rule:** *The loop is not a plugin. The loop has plugins (middleware/hooks).*

---

## 1. Positioning (code and README must not violate)

### Goals
- Download → onboard → message it → real work (files, fetch, memory)
- Durable tool loop (crash-safe; no dangling `tool_call_id`s)
- Safe defaults: shell off, path jail, SSRF block, secrets redacted, localhost webhook
- Skills as `SKILL.md`; optional `SOUL.md` `MEMORY.md` `HEARTBEAT.md`
- `doctor` tells the truth before you trust the process

### Non-goals v1
- Skill marketplace / ClawHub
- Browser (core)
- Electron / Node
- Kafka, gRPC control plane, K8s operator, Uber FX
- Go plugins / `.so`
- User-written `Agent.Execute()` controllers
- Feature parity with OpenClaw
- Wasm / gVisor / Firecracker (sandbox **port** may exist as `none`; real isolation later)

---

## 2. Architectural style (say these words in README)

Coding agents retrieve patterns better when names match training data:

1. **12-factor daemon** — one process, config file + env, logs stdout
2. **Standard Go layout** — `cmd/` `internal/`
3. **Clean / hexagonal** — `core` imports only `ports` + stdlib
4. **CLI subcommands** — `onboard` `gateway` `doctor` `version` (cobra optional)
5. **Laravel/Spring middleware** — **named hook points on a fixed kernel**, not an onion around a custom Execute
6. **Compile-time wiring** — `main` registers tools, channels, middleware explicitly

Do **not** scan `app/Agents/*.go` to auto-register. That is reflection or codegen. Skills on disk are the dynamic extension.

---

## 3. Repository layout (create exactly this)

```
byzclaw/
  README.md
  BYZCLAW_BUILD_PLAN.md
  go.mod                     # github.com/Syluxso/byzclaw (or user module)
  Makefile
  cmd/
    byzclaw/
      main.go
    byzclaw-full/
      main.go
  internal/
    core/
      loop.go
      run.go
      policy.go
      compact.go
      heartbeat.go
      skills.go
      workspace_files.go
      hooks.go
      types.go
    ports/
      channel.go
      tool.go
      model.go
      store.go
      memory.go
      secrets.go
      sandbox.go
      clock.go
      hooks.go
    adapters/
      channel/
        cli/
        telegram/
        webhook/
      model/
        openai_compat.go
      tool/
        workspace.go
        http_fetch.go
        memory_tool.go
        shell.go
      store/
        sqlite.go
      memory/
        markdown.go
      secrets/
        file_secrets.go
      sandbox/
        none.go
      hooks/
        audit.go
        pii.go
        token_ceiling.go
  config/
    config.go
    load.go
  log/
    log.go
  skills/
    example-note/
      SKILL.md
  testdata/
    runs/
  docs/
    DESIGN.md
```

---

## 4. Ports

Define first. Core never imports Telegram, SQLite, YAML adapters, etc.

```go
package ports

type Inbound struct {
    Channel   string
    SessionID string
    UserID    string
    Text      string
    Kind      string // "user" | "heartbeat" | "system"
    Meta      map[string]string
}

type Outbound struct {
    Channel   string
    SessionID string
    Text      string
    Meta      map[string]string
}

type Channel interface {
    Name() string
    Start(ctx context.Context, inbox chan<- Inbound) error
    Send(ctx context.Context, msg Outbound) error
}

type ToolCall struct {
    ID   string
    Name string
    Args json.RawMessage
}

type ToolResult struct {
    ID      string
    Name    string
    Content string
    IsError bool
}

type ToolSpec struct {
    Name        string
    Description string
    Schema      json.RawMessage
}

type Tool interface {
    Name() string
    Description() string
    Schema() json.RawMessage
    Exec(ctx context.Context, call ToolCall) (ToolResult, error)
}

type Message struct {
    ID         string
    SessionID  string
    Role       string // user | assistant | tool | system
    Content    string
    ToolCalls  []ToolCall // assistant
    ToolCallID string     // tool
    CreatedAt  time.Time
}

type RunStatus string // accepted | model | tool_pending | tool_done | completed | failed | cancelled

type Run struct {
    ID        string
    SessionID string
    Status    RunStatus
    Iteration int
    Error     string
    UpdatedAt time.Time
}

type Store interface {
    SaveMessage(ctx context.Context, m Message) error
    ListMessages(ctx context.Context, sessionID string, limit int) ([]Message, error)
    SaveRun(ctx context.Context, r Run) error
    LoadRun(ctx context.Context, id string) (Run, error)
    ListIncompleteRuns(ctx context.Context) ([]Run, error)
}

type CompletionRequest struct {
    Messages []Message
    Tools    []ToolSpec
    Model    string
}

type CompletionResponse struct {
    Content   string
    ToolCalls []ToolCall
    Usage     TokenUsage
}

type TokenUsage struct {
    Input  int
    Output int
}

type Model interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

type Policy interface {
    AllowTool(ctx context.Context, name string, args json.RawMessage) error
    AllowPath(path string, op string) error // read | write | list
}

type Skill struct {
    ID    string
    Name  string
    Body  string
    Tools []string
}

type SkillSource interface {
    List(ctx context.Context) ([]Skill, error)
    Get(ctx context.Context, id string) (Skill, error)
}

type Secrets interface {
    Get(ctx context.Context, name string) (string, error)
}

type HookPoint string

const (
    HookOnInbound   HookPoint = "on_inbound"
    HookBeforeModel HookPoint = "before_model"
    HookAfterModel  HookPoint = "after_model"
    HookBeforeTool  HookPoint = "before_tool"
    HookAfterTool   HookPoint = "after_tool"
    HookOnLimit     HookPoint = "on_limit"
    HookOnError     HookPoint = "on_error"
    HookOnComplete  HookPoint = "on_complete"
)

type HookContext struct {
    Run      *Run
    Session  string
    Inbound  *Inbound
    Notes    []string
    ToolName string
    ToolArgs json.RawMessage
}

type VetoError struct{ Reason string }

func (e VetoError) Error() string { return e.Reason }

type Hook interface {
    Point() HookPoint
    Name() string
    Run(ctx context.Context, hc *HookContext) error
}
```

Tool DX: structs with json tags; helper generates JSON Schema. Do not return schema as an untyped string.
Do not add Agent / Blueprint / Execute. The kernel is the agent.

---

## 5. Agent loop (non-negotiable)

```
inbound
  → middleware on_inbound
  → authorize (channel allowlist)
  → load session + SOUL.md + skills → system prompt
  → compact if over threshold
  → middleware before_model
  → model.Complete
  → middleware after_model
  → persist assistant message (including tool_calls) BEFORE any tool
  → for each tool_call:
        middleware before_tool
        policy.AllowTool
        tool.Exec
        middleware after_tool
        persist tool result BEFORE next model call
  → if tool_calls present → iteration++ → model again
  → else Send outbound
  → middleware on_complete
```

**Hard rules**

- Never send the model an assistant `tool_calls` message without matching tool results in store.
- `max_tool_iterations` default 12. On limit: user-visible text, `on_limit`, stop. No silent spin.
- Same tool + same error 3 times → abort that path with explanation.
- On gateway start: `ListIncompleteRuns`. Resume if safe else mark failed.
- Heartbeat = `Inbound{Kind: "heartbeat"}` plus `HEARTBEAT.md` into the same loop.
- Profiles are knob presets: `interactive` (default), `heartbeat`.

**Compaction**

Defaults: `compact_token_threshold: 8000`, `compact_tail_messages: 10`.

**Middleware (built-ins)**

| Name | Points | Behavior |
|---|---|---|
| audit | before_tool, after_tool, on_error, on_complete | JSON lines: run id, tool, allow/deny, duration |
| pii | before_model | Conservative regex redaction before the model |
| token_ceiling | after_model, on_complete | Fail loud if over config cap |

Hooks may veto (`VetoError`) and append `Notes`. Must not skip persistence or nest another loop.

---

## 6. Home directory

`$BYZCLAW_HOME` default `~/.byzclaw` (`--home`, `BYZCLAW_HOME`).

```
$BYZCLAW_HOME/
  config.yaml
  secrets/
  workspace/
  skills/
  memory/
  data/byzclaw.db
  SOUL.md
  MEMORY.md
  HEARTBEAT.md
```

`SOUL.md` — persona in system prompt.
`MEMORY.md` — long-term notes.
`HEARTBEAT.md` — heartbeat-only instructions.

`skills/<id>/SKILL.md`:

```markdown
---
id: daily-note
name: Daily note
tools: [workspace_read, workspace_write, memory_write]
---
When the user asks to jot a note, write markdown under workspace/notes/...
```

Effective tools = skill allowlist ∩ enabled tools ∩ policy.
No multi-agent in v1.

---

## 7. Config (config.yaml)

```yaml
model:
  provider: openai_compat
  base_url: https://api.x.ai/v1
  model: grok-4
  api_key_secret: xai

channels:
  cli:
    enabled: true
  telegram:
    enabled: false
    token_secret: telegram_bot
    allow_from: []
  webhook:
    enabled: false
    addr: "127.0.0.1:8743"
    path: /hook
    allow_public: false

tools:
  workspace:
    root: workspace
  http_fetch:
    max_bytes: 2000000
    timeout_seconds: 30
  shell:
    enabled: false

loop:
  max_tool_iterations: 12
  compact_token_threshold: 8000
  compact_tail_messages: 10
  profile: interactive
  token_ceiling: 0

heartbeat:
  enabled: false
  interval: 1h
  quiet_hours:
    start: "23:00"
    end: "07:00"
    timezone: Local
  model: ""

middleware:
  - audit
  - pii
  - token_ceiling

skills:
  dir: skills
```

Secrets: `$HOME/secrets/<name>` mode 0600. Never log values.

---

## 8. Tools (core v1)

| Name | Default | Rules |
|---|---|---|
| workspace_list | on | under workspace root |
| workspace_read | on | EvalSymlinks + path stays in root |
| workspace_write | on | same jail |
| http_fetch | on | block private, loopback, link-local, metadata IPs |
| memory_read / memory_write | on | under memory/ |
| shell | off | only if enabled |

No browser in core.

---

## 9. Channels v1

- **CLI** — REPL and/or `byzclaw run --text "..."`
- **Telegram** — require `allow_from` non-empty when enabled
- **Webhook** — `POST {"session_id","text","user_id?"}; no 0.0.0.0 unless allow_public: true`

Session keys: `cli:local`, `telegram:<chat_id>`, `webhook:<session_id>`.

---

## 10. CLI

```
byzclaw onboard
byzclaw gateway
byzclaw doctor
byzclaw version
byzclaw run --text "..."
```

Doctor: config parse, secrets 0600, sqlite writable, workspace, telegram allowlist, webhook bind, shell-off check, model secret, MEMORY/SOUL size warn.
Onboard creates dirs, writes config, prompts keys, runs doctor. Gateway doctors at start; abort on critical.

---

## 11. Core vs full

Same `internal/core`. Full only registers extra adapters later (UI, browser). One config schema.

---

## 12. Model adapter

One `openai_compat` client: chat completions + tools. xAI / OpenAI / Ollama-compatible.

---

## 13. Wiring

```
parse flags → load config → secrets/store/policy → register tools/channels/middleware
→ Loop → doctor → channels/heartbeat/webhook → Handle inbox
→ recover incomplete runs → SIGINT
```

---

## 14. Tests (required)

- Path jail: `..`, symlink out, root boundary
- Crash after assistant `tool_calls`, before tool result → restart safe
- Max iterations stops
- SSRF: 169.254.169.254, 127.0.0.1, 10.0.0.1
- Golden fixture tool round-trip with fake model
- PII middleware unit test

---

## 15. Makefile

```makefile
BINARY_NAME=byzclaw
BUILD_FLAGS=-ldflags="-s -w" -trimpath

.PHONY: build test vet

build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o bin/$(BINARY_NAME) ./cmd/byzclaw

test:
	go test ./...

vet:
	go vet ./...
```

No UPX. Prefer `modernc.org/sqlite`.

---

## 16. Implementation order

1. Module + layout + ports + types
2. SQLite store + policy path jail + tests
3. Fake model + core/loop + workspace tools + loop tests
4. Config + secrets + CLI + doctor + onboard
5. `openai_compat` + `run --text` / CLI REPL
6. Telegram + webhook
7. Skills + SOUL/MEMORY/HEARTBEAT + compaction
8. Heartbeat ticker + audit/pii/token_ceiling
9. Incomplete run recovery
10. README

Do not start Discord, browser, UI chrome, MCP, vector DB, or gRPC.

---

## 17. Demo bar ("done")

```
make build && ./bin/byzclaw onboard
./bin/byzclaw run --text "write hello to workspace/hi.md"
```

- File exists under workspace jail
- Kill -9 gateway; restart; no `tool_call_id` corruption
- doctor passes with telegram off and shell off

---

## 18. Anti-patterns

- `core` importing adapters
- String-prefix path checks without `EvalSymlinks`
- Shell or public bind on by default
- `Agent` interface with user `Execute`
- Onion middleware wrapping a custom agent
- Schema as free-form strings only
- Claiming sub-10MB / 8MB idle
- UPX, CGO sqlite, Node, Gin
- Scanning Go packages to register tools
- Nested LLM loops in hooks
- Control plane, Vault, K8s CRDs, Wasm in v1
- Unbounded history to the model

---

## 19. README skeleton

```
byz-claw — OpenClaw's idea. Pico-class install. Defaults that will not burn the host.

One static Go binary
Text it on CLI or Telegram
Skills are markdown; tools are reviewed Go
Shell off; workspace jail; doctor included
```

Headline: one loop, one layout, safe defaults. Not "Laravel of agents."

---

## 20. Not from the EnterpriseClaw/Gemini draft

Do not implement: split control plane, `internal/app/agents` MVC, Kernel `BuildPipeline` around `Execute`, required Vault/Bedrock, gVisor/Firecracker/wazero v1, UPX, CRDs, stub `Execute` as the product.

Later enterprise = adapters on this same core.

---

*End of spec. Implement strictly. When ambiguous, choose the stricter security interpretation.*

Save as `BYZCLAW_BUILD_PLAN.md` and hand that file to local Grok Build.
