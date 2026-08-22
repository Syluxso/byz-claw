# byz-claw — Grok Build Implementation Spec (v3)

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
| **Tasks vs inbox** | **Tasks** = durable completion contracts. **Inbox** = short session pending buffer. Never call the buffer a "queue" in product docs (avoids Kafka/Rabbit confusion). |
| **Task truth** | Status in SQLite is the only completion truth. The model does not "vibe complete." |
| **Approval** | Agent-owned work that requires human gate uses policy: block parent + open `owner=user` approval task — not a separate subsystem. |

**Rule:** *The loop is not a plugin. The loop has plugins (middleware/hooks).*

**Rule:** *Inbox feeds the loop. Tasks annotate what "finished" means across time. Neither replaces `Loop.Handle`.*

---

## 1. Positioning (code and README must not violate)

### Goals
- Download → onboard → message it → real work (files, fetch, memory)
- Durable tool loop (crash-safe; no dangling `tool_call_id`s)
- Safe defaults: shell off, path jail, SSRF block, secrets redacted, localhost webhook
- Skills as `SKILL.md`; optional `SOUL.md` `MEMORY.md` `HEARTBEAT.md`
- `doctor` tells the truth before you trust the process
- Durable **tasks** (user / agent / process commitments) and **inbox** (pending inbound while a run is busy)
- Simple day-one; same ports scale to long-running and enterprise adapters later

### Non-goals v1
- Skill marketplace / ClawHub
- Browser (core)
- Electron / Node
- Kafka, gRPC control plane, K8s operator, Uber FX
- Go plugins / `.so`
- User-written `Agent.Execute()` controllers
- Feature parity with OpenClaw
- Wasm / gVisor / Firecracker (sandbox **port** may exist as `none`; real isolation later)
- Camunda / full DAG workflow engine / Jira clone
- Multi-agent swarm product
- OpenClaw-style `/queue` steer|followup|collect|interrupt mode matrix as a user feature
- Email channel (later: adapter into inbox or tasks, not "agent is Gmail")

### What we deliberately fix vs OpenClaw (complaints → byz-claw)

| OpenClaw pain | byz-claw stance |
|---|---|
| Gateway RAM / OOM | Go binary; SQLite; compaction deletes rows; no unbounded session skill snapshots in heap |
| Shell / skills / exposed gateway | Shell off; workspace jail; localhost webhook; Telegram `allow_from`; no ClawHub |
| Broken memory backends | Durable SQLite + task rows; inject **small** open-task JSON, not MEMORY.md novels |
| Install / Node upgrade hell | One static binary; onboard/doctor |
| Unclear heartbeat vs cron vs "tasks" | Heartbeat = wake; **tasks** = commitments; **inbox** = pending inbound |

---

## 2. Architectural style (say these words in README)

Coding agents retrieve patterns better when names match training data:

1. **12-factor daemon** — one process, config file + env, logs stdout
2. **Standard Go layout** — `cmd/` `internal/`
3. **Clean / hexagonal** — `core` imports only `ports` + stdlib
4. **CLI subcommands** — `onboard` `gateway` `doctor` `version` `run` `task` (cobra optional)
5. **Laravel/Spring middleware** — **named hook points on a fixed kernel**, not an onion around a custom Execute
6. **Compile-time wiring** — `main` registers tools, channels, middleware explicitly

Do **not** scan `app/Agents/*.go` to auto-register. That is reflection or codegen. Skills on disk are the dynamic extension. Tasks/inbox are **ports + SQLite**, not a second runtime.

---

## 3. Repository layout

```
byzclaw/
  README.md
  BYZCLAW_BUILD_PLAN.md
  go.mod
  Makefile
  cmd/
    byzclaw/main.go
    byzclaw-full/main.go
  internal/
    core/
      loop.go, run.go, policy.go, compact.go, heartbeat.go
      skills.go, workspace_files.go, hooks.go, tasks.go, inbox.go, types.go
    ports/
      channel.go, tool.go, model.go, store.go, task.go, inbox.go
      memory.go, secrets.go, sandbox.go, clock.go, hooks.go
    adapters/
      channel/{cli,telegram,webhook}/
      model/{openai_compat.go,local_demo.go}
      tool/{workspace,http_fetch,memory_tool,shell,task_tools}.go
      store/sqlite.go
      memory/markdown.go
      secrets/file_secrets.go
      sandbox/none.go
      hooks/{audit,pii,token_ceiling,task_approval}.go
  config/, log/, skills/example-note/SKILL.md, testdata/runs/, docs/DESIGN.md
```

---

## 4. Ports

Define first. Core never imports Telegram, SQLite, YAML adapters, etc.

Same Channel / Tool / Store / Model / Hook ports as v2, **plus**:

### TaskStore (durable completion contracts)

```go
type TaskOwner string  // agent | user | run | process | system
type TaskStatus string // open | blocked | done | cancelled

type Task struct {
    ID, Title, BodyRef string
    Owner         TaskOwner
    Kind          string // goal | step | procedure | approval
    Status        TaskStatus
    BlockedReason string
    Source        string
    SessionID, RunID, ParentID, BlocksTaskID string
    DoneWhen      string
    RequiresApproval bool
    DedupeKey     string
    Metadata      json.RawMessage
    CreatedAt, UpdatedAt time.Time
    CompletedAt   *time.Time
}

type TaskStore interface {
    Create(ctx context.Context, t Task) (Task, error)
    Get(ctx context.Context, id string) (Task, error)
    List(ctx context.Context, f TaskFilter) ([]Task, error)
    ListOpen(ctx context.Context, owner TaskOwner, limit int) ([]Task, error)
    UpdateStatus(ctx context.Context, id string, status TaskStatus, reason string) error
    Complete(ctx context.Context, id string) error
    Block(ctx context.Context, id string, reason string) error
    Cancel(ctx context.Context, id string, reason string) error
}
```

### InboxStore (session pending buffer — not a "queue")

```go
type InboxState string // pending | delivered | dropped

type InboxItem struct {
    ID, SessionID, Channel, ExternalID string
    Payload        json.RawMessage
    State          InboxState
    ArrivedAt      time.Time
    DeliveredRunID string
    Attempts       int
    LastError      string
}

type InboxStore interface {
    Enqueue(ctx context.Context, item InboxItem) (InboxItem, error)
    ListPending(ctx context.Context, sessionID string, limit int) ([]InboxItem, error)
    MarkDelivered(ctx context.Context, id string, runID string) error
    Drop(ctx context.Context, id string, reason string) error
}
```

### Extra hook points

`on_task_created`, `before_task_complete`, `on_task_blocked`, `on_task_cancelled`,
`on_inbound_buffered`, `on_inbox_drained`

Tool DX: structs + JSON Schema. No Agent/Blueprint/Execute.

---

## 5. Agent loop (non-negotiable)

```
inbound (or drained inbox item)
  → on_inbound → authorize
  → session + SOUL + skills
  → inject ListOpen tasks (tiny JSON: id, owner, title, status, done_when)
  → compact → before_model → model → after_model
  → persist assistant (tool_calls) BEFORE tools
  → each tool: before_tool → policy → Exec → after_tool → persist result
  → iterate or outbound → on_complete
  → reconcile owner=run tasks
  → if idle: Inbox.Drain → next Handle
```

**Hard rules**

- Never send the model an assistant `tool_calls` message without matching tool results in store.
- `max_tool_iterations` default 12. On limit: user-visible text, `on_limit`, stop.
- Same tool + same error 3 times → abort that path.
- On gateway start: `ListIncompleteRuns`. Resume if safe else mark failed. Then drain pending inbox.
- Heartbeat = `Inbound{Kind: "heartbeat"}` + `HEARTBEAT.md`; may list open tasks, not invent completion.
- While a session has an active run, new inbound → **Inbox.Enqueue** (`pending`), not a parallel Handle.

**Compaction:** defaults threshold 8000, tail 10; must **delete** compacted rows from SQLite.

**Middleware:** audit, pii, token_ceiling; optional `task_approval` on `before_task_complete`.

---

## 5b. Tasks system

| | **Inbox** | **Tasks** |
|---|---|---|
| Job | Buffer inbound while run busy | Work not finished until explicit end state |
| Lifetime | Short | Long (across runs / heartbeats / restarts) |
| Success | Handed to a run | Contract done or cancelled |
| Name | inbox / pending | tasks |

**Owners:** `user` | `agent` | `run` | `process` | `system`  
**Status:** `open` → `blocked` → `done` | `cancelled`

### Agent-owned, user-approved (policy)

```
Given a task needs human approval and is agent-owned (RequiresApproval)
When the agent attempts complete
Then parent → blocked (awaiting_approval)
And a owner=user approval task is created (linked via ParentID / BlocksTaskID)
And on user complete → parent unblocked/completed (reject → cancel parent)
```

Default: agent may **not** complete `owner=user` tasks.

Procedures: same table + `parent_id` (flat first is OK). No DAG engine in v1.

**Tools:** `task_create`, `task_list`, `task_get`, `task_complete`, `task_block`  
**CLI:** `byzclaw task list|show|complete`

**Fault tolerance:** persist before model sees task; `dedupe_key`; reconcile `owner=run` on gateway start.

**Scale:** same `TaskStore` port; swap backend later; enterprise = adapters + hooks.

---

## 5c. Inbox (session pending)

- Run idle → Handle immediately
- Run active → Enqueue pending (external_id dedupe)
- On complete / idle → drain FIFO
- Cap + drop policy in config
- Product name **inbox**, state **pending** — never public "queue"

---

## 6. Home directory

`$BYZCLAW_HOME` default `~/.byzclaw`.

```
$BYZCLAW_HOME/
  config.yaml, secrets/, workspace/, skills/, memory/
  data/byzclaw.db    # messages, runs, tasks, inbox
  data/audit.jsonl
  SOUL.md, MEMORY.md, HEARTBEAT.md
```

Effective tools = skill allowlist ∩ enabled tools ∩ policy. No multi-agent in v1.

---

## 7. Config (additions)

```yaml
tools:
  tasks:
    enabled: true

inbox:
  max_pending_per_session: 20
  drop: oldest   # oldest | refuse

tasks:
  inject_open_limit: 20
  run_end_policy: block   # block | cancel for owner=run leftovers
  default_requires_approval: false
```

(Other sections unchanged from v2: model, channels, loop, heartbeat, middleware, skills.)

---

## 8. Tools (core v1)

workspace_list/read/write, http_fetch, memory_read/write, task_* (when enabled), shell **off** by default. No browser in core.

---

## 9–12. Channels, CLI, core/full, model

As v2, plus busy session → inbox; CLI adds `byzclaw task …`; LocalDemo when no API key.

---

## 13. Wiring

```
parse flags → load config → secrets/store/taskstore/inboxstore/policy
→ register tools/channels/middleware
→ Loop → doctor → channels/heartbeat/webhook
→ inbound: busy? enqueue : Handle
→ recover incomplete runs → drain pending → SIGINT
```

---

## 14. Tests (required)

Path jail, crash resume, max iterations, SSRF, golden tool round-trip, PII,
**tasks** (create/list/complete; agent cannot complete owner=user; requires_approval flow),
**inbox** (enqueue while busy; drain; dedupe; no double-deliver after restart),
**run end** (owner=run policy).

---

## 15. Makefile

Unchanged: `CGO_ENABLED=0`, `-ldflags="-s -w"`, no UPX, modernc sqlite.

---

## 16. Implementation order

**Done (do not regress):** loop, jail, tools, onboard/doctor/run, skills, compaction,
middleware, gateway, webhook, Telegram, heartbeat, audit.

**Next:** tasks + inbox tables/ports → gateway buffer/drain → task tools/CLI →
inject open tasks → run-end reconcile → optional approval hook → tests → README naming.

**Later:** Discord, browser, full UI, MCP, email, external TaskStore.

---

## 17. Demo bar

```
make build && ./bin/byzclaw onboard
./bin/byzclaw run --text "write hello to workspace/hi.md"
```

Plus when tasks/inbox land: create user task, `byzclaw task list`, second message while busy drains after.

---

## 18. Anti-patterns

Prior list, plus:
- Calling the session pending buffer a "queue" in user-facing docs
- Storing todos only in HEARTBEAT.md / MEMORY.md
- Second registered agent engine / pluggable Loop.Handle
- Approval as a separate database or workflow product
- Merging inbox notifications with task completion state

---

## 19–20. README / not from EnterpriseClaw

Unchanged intent. Later enterprise = adapters on this same core.

---

## 21. Design origins

- **Gears:** SQLite for queryable facts; avoid stuffing the model with large files
- **OpenClaw:** durability lessons; do not copy RAM/skill-hub/security defaults
- **PicoClaw:** small binary; HEARTBEAT.md as standing orders only
- **byz-claw:** fixed loop + hooks; tasks = completion contracts; inbox = pending

---

*End of spec v3. Implement strictly. When ambiguous, choose the stricter security interpretation.*
