# Memory (workspace search)

Embedded **SQLite FTS5** plus optional **vector** search (OpenAI-compatible
embeddings). No separate vector DB process — vectors live as BLOBs next to FTS.

TTY agents (claude, grok, …) own their own model loop. `agentsd` cannot inject
tokens mid-generation. Memory is exposed as:

1. **CLI/API** — `agentsctl memory search "…"` (agent or skill runs this)
2. **Web UI** — Index / Search panels on the embedded SPA
3. **Files** — still prefer `.agents/PROJECT_MAP.md` for structure

Without `embed_url`, search is FTS-only (works offline, no API keys).

## CLI

```bash
# index map + README + docs (regenerates map by default)
agentsctl memory index -r my-app
agentsctl memory index -r my-app --clear --code   # include shallow code samples

agentsctl memory search -r my-app "session PTY websocket"
agentsctl memory search -r my-app --mode vector "auth middleware"
agentsctl memory stats -r my-app
```

## API

| Method | Path | Body / query |
|--------|------|----------------|
| POST | `/v1/memory/index` | `{ "cwd", "clear", "include_code", "generate_map" }` |
| POST | `/v1/memory/search` | `{ "cwd", "query", "limit", "mode": "auto\|fts\|vector" }` |
| GET | `/v1/memory/stats?cwd=` | doc counts + embed stats |

## Config

```toml
[memory]
enabled = true
# dir = "/var/lib/agents/jobs/memory"   # default: {jobs_dir}/memory

# Optional vectors (OpenAI-compatible). Index then re-index after enabling.
# embed_url = "https://api.openai.com/v1"
# embed_model = "text-embedding-3-small"
# embed_api_key_env = "AGENTS_EMBED_KEY"   # or set for Ollama-compatible local
```

`mode=auto` (default): vector search if embedder configured and docs have
embeddings; otherwise FTS.

## Security

- Paths must pass the workspace allowlist
- Content is redacted for common secret patterns before index (`internal/redact`)
- Files larger than 256 KiB are truncated
- Memory DB is local under `jobs_dir` (mode 0700 dir)

## Agent usage pattern

```text
1. Read .agents/PROJECT_MAP.md
2. agentsctl memory search -r . "relevant terms"
3. Open only the hit paths
```
