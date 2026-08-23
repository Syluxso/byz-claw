# byz-claw — Design Pattern: Code First, LLM Last

Hand this to local Grok Build with `BYZCLAW_BUILD_PLAN.md`.  
**This document states the design pattern; the plan is still the concrete spec.**

When this conflicts with “make it smarter with more prompt,” **this document wins.**

---

## One sentence

Build a reliable local agent **system** (state, tools, policy, audit). Use the LLM only when hard-coding the next step would be ridiculous.

## Layering (strict)

1. **Deterministic core** — schedules, tasks, inbox, status machines, dedupe  
2. **Tools + policy** — real work, jail, allowlists, local or remote adapters  
3. **Observability** — runs, audit, task/inbox state (**always**)  
4. **LLM** — optional reasoning over structured context + tools  

Never invert this. The model does not own time, completion truth, or wake-up.

## Non-negotiable splits

| Piece | Does | Does not |
|---|---|---|
| **Scheduler** | On a tick, mint due tasks from schedules; may request wake | Call the model; run the agent loop |
| **Tasks** | Durable “done when” contracts (optional `due_at`) | Act as the chat buffer |
| **Inbox** | Sole activation path for the agent; may reference `task_id` | Store the task row itself |
| **Loop** | Drain inbox → tools → model as needed → persist | Run because a timer felt like chatting |
| **Skills** | DB registry; tool allowlists; optional short content + deterministic playbook | Long essays as the control plane |
| **HEARTBEAT.md** | Removed | Periodic synthetic chat |

Agent idle ⇔ inbox empty (for gateway). Agent works ⇔ inbox has work.

## LLM usage policy

**Must not use LLM for:**

- Is a schedule due?
- Mint task / `dedupe_key`
- Path jail, SSRF, secret redaction
- Task status transitions (`open` → `done`)
- Inbox FIFO / deliver / drop
- Playbook `ensure_*` side effects

**May use LLM for:**

- Interpreting messy natural language
- Choosing among tools when policy allows
- Drafting text from tool results
- Planning under ambiguity (still via tools; completion = task store)

If a feature can be a state machine + tool, implement that first.

## Implementation habits

1. **Structured everything** — tables and typed ports, not markdown as truth.  
2. **Tools for verbs** — prefer tool calls over “please remember to…”  
3. **Log / observe** — every run, tool allow/deny, task/inbox change.  
4. **Opinionated core** — one loop, one wake path, explicit wiring in `main`.  
5. **Modularity at ports** — swap Store/TaskStore/tools; do not fork the loop.  
6. **Degrade without model** — scheduler, inbox accept, policy, task mint still work if API is down.

## Playbooks vs prompts

- **Playbook (JSON):** runtime applies deterministically (schedules/tasks/inbox/artifacts).  
- **Skill content / md:** short guidance only.  
- Doctor syncs git `SKILL.md` → skills registry; **registry is runtime truth after sync.**

## Anti-patterns (reject in review)

- Synthetic heartbeat as a second personality path  
- Scheduler invoking `Complete` / chat  
- Task row moved into inbox  
