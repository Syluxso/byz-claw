# byz-claw — Grok Build Implementation Spec (v4)

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
| Extensibility | **Skills = DB registry** (+ optional body/playbook). Tools/channels = Go adapters, explicit register. |
| DX for coding agents | Convention-over-configuration **layout and names**, not directory auto-scan of `.go`. |
| Core vs full | Same `internal/core`. Full only registers extra adapters (UI, browser later). |
| Enterprise v1 | Policy, audit, secrets, doctor, allowlists — not SSO mothership / Vault / CRDs. |
| Size honesty | Core target ≤ ~35MB binary, idle tens of MB RAM. Do **not** claim 7–10MB. |
| Build | `CGO_ENABLED=0` `-ldflags="-s -w"`. No UPX. Prefer `modernc.org/sqlite` (no CGO). |
| **Tasks vs inbox** | **Tasks** = durable completion contracts. **Inbox** = agent activation / pending work. Never call the buffer a "queue" in product docs. |
| **Task truth** | Status in SQLite is the only completion truth. The model does not "vibe complete." |
| **Approval** | Agent-owned work that requires human gate: block parent + open `owner=user` approval task. |
| **Scheduler vs agent** | **Scheduler is not the agent.** No LLM. Every N seconds/minutes: mint due tasks from schedules; optionally enqueue inbox wakes. |
| **HEARTBEAT** | **Removed** as a runtime system. No `HEARTBEAT.md` pulse. Periodic work = schedules → tasks → optional inbox. |
| **Agent wake** | Agent runs **only** because inbox has work (user message, task wake, system). Empty inbox → idle. |
| **Skills** | Registered in DB. Optional short `content` and/or `md_ref` / `playbook_ref`. Loop prefers **tools**; playbook applies deterministically. |

**Rule:** *The loop is not a plugin. The loop has plugins (middleware/hooks).*

**Rule:** *Inbox activates the agent. Tasks define finished. Scheduler only mints (and may request wake).*

---

## 0b. Corrected design bullets (canonical)

Use these phrasings (not the older heartbeat-centric ones):

1. **Scheduler + tasks replace HEARTBEAT**  
   - Scheduler: time-driven, no LLM; evaluates `schedules`, mints `tasks`, may enqueue inbox.  
   - Not named "cron" as the product — cron/interval is just the tick expression.  
   - Tick interval default **15–60s** (not 1s unless justified).

2. **Skills are DB-registered**  
   - Row is source of runtime truth after doctor sync.  
   - **May** attach short `content` **or** `md_ref` (file) **or** `playbook_ref` (JSON).  
   - Prefer tools + playbook over long "LLM read this skill" prose.  
   - Git-friendly: hand-written `skills/*/SKILL.md` remain; **doctor** upserts them into the DB.

### Also required (do not miss)

| Item | Why |
|---|---|
| **Inbox ≠ task row** | Inbox item **references** `task_id`; task is not moved into inbox |
| **Not every task wakes the agent** | Reminders / far-future / user-owned may never enqueue |
| **`due_at` optional** | Null = not time-driven; past/now due = eligible on next scheduler pass |
| **Recurring = schedule template** | Each fire mints a **new** task instance + `dedupe_key` |
| **Playbook is deterministic** | Ensure schedules/tasks/inbox/artifacts — applied by runtime, not model improvisation |
| **SOUL.md / MEMORY.md stay files** | Persona + long-term notes; not the skill registry |
| **Remove StartHeartbeat / HEARTBEAT.md** | From gateway, onboard seed, doctor checks, config `heartbeat.*` |
| **Tables** | `messages`, `runs`, `tasks`, `inbox`, `schedules`, `skills` |

---

## 1. Positioning

### Goals
- Download → onboard → message it → real work (files, fetch, memory)
- Durable tool loop (crash-safe; no dangling `tool_call_id`s)
- Safe defaults: shell off, path jail, SSRF block, secrets redacted, localhost webhook
- Skills **registered in DB**; optional body/playbook; tools first
- `doctor` tells the truth (incl. skill file ↔ DB sync)
- **Schedules**, **tasks**, **inbox** — not HEARTBEAT.md
- Agent idle when inbox empty; works only when inbox has items

### Non-goals v1
- Skill marketplace / ClawHub (remote *fetch* may come later; not a store product)
- Browser (core), Electron, Node, Kafka, K8s, Go plugins, user `Agent.Execute`
- Camunda / full DAG engine
- Heartbeat ticker + HEARTBEAT.md as a parallel system
- 1-second busy-poll scheduler as default

### OpenClaw pains → byz-claw

| Pain | Stance |
|---|---|
| RAM / OOM | Go; SQLite; compaction deletes |
| Shell / exposure | Shell off; jail; localhost; allow_from |
| Fuzzy memory / "tasks" | Real task rows + inbox activation |
| Heartbeat vs cron confusion | **Scheduler mints tasks**; inbox wakes agent |

---

## 2. Architectural style

1. 12-factor daemon  
2. Standard Go `cmd/` `internal/`  
3. Hexagonal — core imports only ports  
4. CLI: `onboard` `gateway` `doctor` `version` `run` `task` `schedule` (optional)  
5. Named hook points on a fixed kernel  
6. Compile-time tool/channel wiring; skills dynamic via DB registry  

---

## 3. Layout (delta from tree)

- **Remove** runtime reliance on `heartbeat.go` / HEARTBEAT.md (file may be deleted or ignored).
- **Add** `internal/core/scheduler.go`, skill registry usage, ports for `ScheduleStore` / `SkillRegistry`.
- **DB tables:** messages, runs, tasks, inbox, schedules, skills.

---

## 4. Ports (additions / changes)

### Task (extend)

Add time fields:

- `DueAt *time.Time` — null = no due; past/now = due  
- Keep: owner, status, done_when, dedupe_key, requires_approval, parent_id, …

### InboxItem (extend)

- `TaskID string` — optional link; **task is not copied into inbox**  
- `Kind` — `user_message` | `task_wake` | `system`  
- Payload for channel text when needed  

### ScheduleStore

```text
Schedule:
  id, name, enabled
  spec          // cron expression OR interval duration
  task_template // title, owner, done_when, requires_approval, metadata…
  wake          // none | on_mint | when_due
  dedupe_template // e.g. "daily-digest-{{date}}"
  quiet_hours optional override
  last_fired_at
```

Scheduler loop: tick → list enabled schedules due → mint task (honor dedupe) → if wake policy → Inbox.Enqueue(task_id=…).

Also: each tick may **scan tasks** with `due_at <= now` and status open and wake-eligible → enqueue once (dedupe).

### SkillRegistry

```text
Skill:
  id, name, enabled, source      // local|remote
  tools_json                     // allowlist
  content                        // optional short prose (DB field)
  md_ref                         // optional path to SKILL.md
  playbook_ref or playbook_json  // optional structured ensure_*
  version, updated_at
```

**Playbook (versioned, small):** deterministic ensure schedules / tasks / inbox seeds / artifacts — applied by runtime on skill activate, **not** by free-form model invention.

**Doctor:** for each `skills/*/SKILL.md` on disk → upsert skills row; report DB-only orphans and missing files.

---

## 5. Agent loop

```text
ONLY started for an inbox item (or one-shot `run` which synthesizes a single inbox-equivalent inbound)
  → on_inbound → authorize
  → SOUL + enabled skills (short content / md) + tool allowlists
  → inject open tasks summary (tiny JSON)
  → model ↔ tools (prefer tools)
  → on_complete → mark inbox delivered
  → drain next pending OR idle
```

**No** `StartHeartbeat`. **No** parallel Handle on same session while run active — enqueue only.

---

## 5b. Tasks

Unchanged intent from v3: owners, status machine, approval policy, tools, CLI.

Plus: **due_at**; far-future OK; never-wake OK; due past/now → scheduler may push inbox.

---

## 5c. Inbox = sole wake path

| State | Meaning |
|---|---|
| pending items | Agent must work (drain) |
| empty | Agent idle |

Wake sources: user channels, task_wake from scheduler/dispatcher, system.  
Shutdown: finish/deliver items or audited purge.

---

## 5d. Scheduler (replaces heartbeat)

**One job:** on an interval, mint due work from schedules / due tasks; optionally enqueue inbox.

- Not the agent loop  
- Not LLM  
- Default tick: 15–60s  
- Quiet hours supported  
- Gateway process-local is enough for v1  

---

## 5e. Skills (DB registry)

- Runtime lists **enabled** skills from DB  
- Effective tools = skill allowlist ∩ enabled tools ∩ policy  
- Optional content/md for brief guidance  
- Playbook applied on activate when present  
- Files under `skills/` are authoring + git; doctor syncs to DB  

---

## 6. Home directory

```text
$BYZCLAW_HOME/
  config.yaml
  secrets/
  workspace/
  skills/           # optional SKILL.md sources for doctor sync
  memory/
  data/byzclaw.db   # messages, runs, tasks, inbox, schedules, skills
  data/audit.jsonl
  SOUL.md
  MEMORY.md
  # no HEARTBEAT.md
```

---

## 7. Config

Remove `heartbeat.*`. Add:

```yaml
scheduler:
  enabled: true
  tick: 30s
  quiet_hours:
    start: "23:00"
    end: "07:00"
    timezone: Local

inbox:
  max_pending_per_session: 20
  drop: oldest

tasks:
  inject_open_limit: 20
  run_end_policy: block
  default_requires_approval: false

skills:
  dir: skills
  doctor_sync: true
```

---

## 8. Tools

workspace_*, http_fetch, memory_*, task_*, shell off by default.  
Skill activation may be a tool later; playbook ensure is runtime, not model-authored SQL.

---

## 13. Wiring

```text
OpenRuntime → doctor (incl skill sync)
→ gateway: channels + scheduler tick + inbox drain loop
→ recover incomplete runs → drain inbox → SIGINT
```

---

## 14. Tests (add)

- Scheduler mints once per dedupe_key  
- Due task enqueues inbox once  
- Task without wake never appears in inbox  
- Empty inbox → no Handle  
- Doctor upserts SKILL.md into skills table  
- No HEARTBEAT path in gateway  

---

## 16. Implementation order

**Done:** loop, jail, tools, onboard/doctor/run, file skills (migrate), compaction, middleware, gateway, channels, **legacy heartbeat (to remove)**.

**Next:**
1. tables: tasks, inbox, schedules, skills  
2. TaskStore / InboxStore / ScheduleStore / SkillRegistry  
3. Inbox-only wake + drain in gateway  
4. Scheduler tick (replace StartHeartbeat)  
5. Doctor skill sync; skill load from DB  
6. task tools/CLI; due_at; approval policy  
7. Remove HEARTBEAT.md from onboard/doctor/config  
8. Tests + README  

---

## 18. Anti-patterns

- HEARTBEAT.md or synthetic heartbeat Kind as ongoing design  
- Scheduler calling the model  
- Moving task rows into inbox  
- Every minted task auto-waking the agent  
- Long skill novels every turn instead of tools + short content  
- Model inventing playbook side effects  
- Calling inbox a "queue" in product docs  
- Second agent engine  

---

## 21. Design origins

- Gears: SQLite index + optional file bodies  
- OpenClaw: lessons, not defaults  
- byz-claw: fixed loop; **scheduler → tasks; inbox → wake; skills registry + tools**  

---

*End of spec v4. Implement strictly. When ambiguous, choose the stricter security interpretation.*
