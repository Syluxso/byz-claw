# byz-claw

OpenClaw’s idea. Pico-class install. Defaults that will not burn the host.

One static Go binary. Text it on CLI / Telegram / webhook. Skills are markdown (synced to SQLite); tools are reviewed Go. Shell off; workspace jail; `doctor` included.

**Standalone / open source.** Not `byz-agent`. No Byzantine platform dependency. Future Byz = optional adapters only.

## Quick start

```bash
make build
./bin/byzclaw onboard
# optional: put API key in ~/.byzclaw/secrets/xai (mode 0600)
./bin/byzclaw run --text "write hello to workspace/hi.md"
./bin/byzclaw doctor
./bin/byzclaw gateway          # CLI REPL (+ recover incomplete runs)
./bin/byzclaw gateway --webhook  # also listen on webhook (see config)
./bin/byzclaw task list
./bin/byzclaw task show --id <id>
./bin/byzclaw task complete --id <id>
./bin/byzclaw run --text "..." --verbose  # audit JSON on stderr too
```

Audit lines append to `$BYZCLAW_HOME/data/audit.jsonl` (quiet CLI by default).  
Shell tool exists but stays **off** unless `tools.shell.enabled: true`.

Override home: `--home DIR` or `BYZCLAW_HOME`.

### Webhook

Enable in `config.yaml` (`channels.webhook.enabled: true`) or pass `--webhook`.  
Default bind `127.0.0.1:8743` path `/hook`. Public binds need `allow_public: true` and `secrets/webhook_token`.

```bash
curl -s http://127.0.0.1:8743/hook -H 'Content-Type: application/json' \
  -d '{"session_id":"demo","text":"write hi to workspace/from-hook.md"}'
```

### Gateway model (v4)

- **Inbox** is the only wake path for the agent loop.
- **Scheduler** ticks with **no LLM**: mints tasks from schedules; may enqueue inbox wakes.
- **Tasks** define finished work (`task_complete` tool / `byzclaw task complete`).
- Busy session → new inbound is **enqueued**, not parallel-handled.
- **HEARTBEAT.md** runtime removed (leftover file = doctor warn only).

## Spec & design pattern

- **Concrete plan (v4):** [`BYZCLAW_BUILD_PLAN.md`](./BYZCLAW_BUILD_PLAN.md) — schedules → tasks; inbox → wake; skills DB registry  
- **Design pattern:** [`docs/CODE_FIRST_LLM_LAST.md`](./docs/CODE_FIRST_LLM_LAST.md) — when tempted to “make it smarter with more prompt,” **this wins**

## Status

**Plan v4 §16 complete:** SQLite tasks/inbox/schedules/skills, inbox-only wake + drain, scheduler tick, doctor skill sync, task tools/CLI, HEARTBEAT runtime removed.

Still later (out of §16): Discord, browser automation, MCP — when needed.

Enable Telegram in `config.yaml` (`channels.telegram.enabled`, `allow_from: ["your_chat_id"]`) and `secrets/telegram_bot`.

Binary size target ≤ ~35MB (do not claim 7–10MB).
