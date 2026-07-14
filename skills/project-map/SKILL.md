---
name: project-map
description: >
  Use the durable project map at .agents/PROJECT_MAP.md before exploring a
  codebase. Generate or refresh with agentsctl map generate. Prefer the map
  plus targeted reads over recursive full-tree walks.
---

# Project map skill

## When to use

At the start of work in a workspace (or after large structural changes), orient
from the **project map** instead of re-walking the entire tree.

## Steps

1. **Check** for `.agents/PROJECT_MAP.md` in the workspace root (cwd).
2. If **missing** or clearly stale:
   - Prefer: `agentsctl map generate -r <cwd>` (when agentsd is available)
   - Or tell the user to regenerate, then continue with minimal exploration
3. **Read** `.agents/PROJECT_MAP.md` (and optionally `.agents/project_map.json`).
4. Use **Read these first** and **Layout** to open only the files you need.
5. After major moves/renames/new packages: regenerate the map.

## Staleness

`.agents/map_meta.json` records `git_head` and `generated_at`.

- If current `git rev-parse HEAD` ≠ `git_head` → regenerate
- If older than ~14 days → regenerate
- `agentsctl map status -r <cwd>` reports `stale` + reason

## Context manager (preferred)

One shot refresh of map + packed `CONTEXT.md` + memory index:

```bash
agentsctl context ensure -r .
agentsctl context status -r .
```

Session start auto-runs ensure when `context.ensure_on_session = true` (default).

## Optional: memory search

After reading the map / CONTEXT.md, for topic-specific docs:

```bash
agentsctl memory search -r . "relevant terms"
```

Index first (once per workspace, or after big doc changes):

```bash
agentsctl memory index -r .
# or: agentsctl context ensure -r .
```

## Do not

- Dump entire monorepos into context when a map exists
- Ignore `AGENTS.md` / `CLAUDE.md` / README listed under “Read these first”
- Treat the map as authoritative for line-level code — still open real files

## One-liner for project instructions

```markdown
## Project map
Read `.agents/PROJECT_MAP.md` before exploring. Regenerate: `agentsctl map generate`.
```
