# Context manager

Orientation layer so agents start **faster** with **better workspace data**.

TTY agents own their model loop — `agentsd` cannot inject tokens mid-generation.
Instead the context manager:

1. **Ensures** durable files under `<cwd>/.agents/`
2. **Indexes** workspace docs into FTS memory (when empty / after map regen)
3. **Seeds** a short protocol into the first TTY keystrokes on session create

## Artifacts

```text
.agents/
  PROJECT_MAP.md     # layout / stack / read-first (projmap)
  project_map.json
  map_meta.json
  CONTEXT.md         # budgeted pack (map + AGENTS/README + optional hits)
  INSTRUCTIONS.md    # standing protocol for agents
```

## Defaults (config)

```toml
[context]
ensure_on_session = true   # on POST /v1/sessions
seed_orientation = true    # prepend short protocol to TTY seed
auto_index = true          # index memory when empty / after map regen
write_files = true         # write CONTEXT.md + INSTRUCTIONS.md
# seed_budget = 1800
# pack_budget = 12000
```

## CLI

```bash
agentsctl context status -r my-app
agentsctl context ensure -r my-app              # map + memory + CONTEXT.md
agentsctl context ensure -r my-app --force-map --force-index --code
agentsctl context pack -r my-app -q "auth middleware"
agentsctl context note -r my-app -title decision "Use JWT for API auth"
```

## API

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/v1/context/status?cwd=` | ready / map / memory / hints |
| POST | `/v1/context/ensure` | `{ cwd, force_map, force_index, include_code, query }` |
| POST | `/v1/context/pack` | `{ cwd, query, budget, write_file }` → markdown pack |
| POST | `/v1/context/note` | `{ cwd, title, text }` → durable memory note |

## Session create

When `ensure_on_session` is true:

1. Resolve allowlisted `cwd`
2. Ensure map (missing/stale) + memory index + write `CONTEXT.md`
3. If `seed_orientation`, merge short protocol into the seed prompt
4. Start tmux agent as usual

Clone (`POST /v1/workspaces/clone`) also runs ensure best-effort.

## Agent usage

```text
1. Read .agents/PROJECT_MAP.md
2. Read .agents/CONTEXT.md
3. agentsctl memory search -r . "topic"
4. Open only hit paths
```

## Web UI

**Tools** (`t`) and **Settings → Workspace** expose:

- Context status (ready / needs ensure)
- **Ensure** — full refresh
- **Show pack** — drawer with CONTEXT.md contents
