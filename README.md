# byz-claw

OpenClaw’s idea. Pico-class install. Defaults that will not burn the host.

One static Go binary. Text it on CLI (Telegram/webhook next). Skills are markdown; tools are reviewed Go. Shell off; workspace jail; `doctor` included.

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
```

Override home: `--home DIR` or `BYZCLAW_HOME`.

### Webhook

Enable in `config.yaml` (`channels.webhook.enabled: true`) or pass `--webhook`.  
Default bind `127.0.0.1:8743` path `/hook`. Public binds need `allow_public: true` and `secrets/webhook_token`.

```bash
curl -s http://127.0.0.1:8743/hook -H 'Content-Type: application/json' \
  -d '{"session_id":"demo","text":"write hi to workspace/from-hook.md"}'
```

## Spec

See [`BYZCLAW_BUILD_PLAN.md`](./BYZCLAW_BUILD_PLAN.md).

## Status

Core loop, store, jail, tools, onboard/doctor/run, skills, compaction, middleware, **gateway + webhook** are in. Telegram / heartbeat ticker still next.

Binary size target ≤ ~35MB (do not claim 7–10MB).
