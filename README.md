# byz-claw

OpenClaw’s idea. Pico-class install. Defaults that will not burn the host.

One static Go binary. Text it on CLI (Telegram/webhook next). Skills are markdown; tools are reviewed Go. Shell off; workspace jail; `doctor` included.

**Standalone / open source.** Not `byz-agent`. No Byzantine platform dependency. Future Byz = optional adapters only.

## Quick start

```bash
make build
./bin/byzclaw onboard
# put API key in ~/.byzclaw/secrets/xai (mode 0600)
./bin/byzclaw run --text "write hello to workspace/hi.md"
./bin/byzclaw doctor
```

Override home: `--home DIR` or `BYZCLAW_HOME`.

## Spec

See [`BYZCLAW_BUILD_PLAN.md`](./BYZCLAW_BUILD_PLAN.md).

## Status

Core loop, SQLite store, path jail, workspace/memory/http_fetch tools, config/onboard/doctor, OpenAI-compatible model client are in. Gateway/Telegram/webhook/skills loader/compaction still in progress.

Binary size target ≤ ~35MB (do not claim 7–10MB).
