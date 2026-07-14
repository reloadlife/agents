# Project maps

Durable orientation files so agents (and humans) skip full-tree rediscovery.

## Artifacts

Under each workspace:

```text
.agents/
  PROJECT_MAP.md      # read this first
  project_map.json    # structured
  map_meta.json       # generated_at, git_head (staleness)
```

Generated **on the server** under allowlisted paths only.

## CLI

```bash
agentsctl map generate -r .          # write map for default/allowlisted cwd
agentsctl map generate -r my-app
agentsctl map show -r my-app         # print PROJECT_MAP.md
agentsctl map status -r my-app       # exists / stale?
agentsctl map generate -r my-app --json
```

## API

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/maps` | body `{ "cwd": "my-app" }` → generate |
| GET | `/v1/maps?cwd=` | markdown (default) or `format=json` |
| GET | `/v1/maps/status?cwd=` | staleness |

## Skill

Portable skill: [skills/project-map/SKILL.md](../skills/project-map/SKILL.md)

Install into agent skill dirs as needed (Claude Code skills path, etc.), or paste
the one-liner into `AGENTS.md` / `CLAUDE.md`:

```markdown
## Project map
Read `.agents/PROJECT_MAP.md` before exploring. Regenerate: `agentsctl map generate`.
```

## What the map contains (v1)

- Stack signals (`go.mod`, `package.json`, …)
- Read-first docs
- Entrypoint heuristics (`cmd/*/main.go`, package scripts, …)
- Summarized layout (depth-limited; ignores `node_modules`, `.git`, …)

Not included: full AST/symbol index or embeddings (see memory roadmap).

## Security

Same path allowlist as sessions/jobs. Maps are plain files inside the workspace
(often fine to commit; optional `.agents/` in `.gitignore` if you prefer local-only).
