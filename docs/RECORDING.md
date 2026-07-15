# Session recording — privacy policy

Opt-in archives of terminal pane snapshots for later browse / search.

## Default: off

Recording is **disabled** unless you set:

```toml
[sessions]
recording = true
```

When off, no archive files are written on kill/detach. Live PTY and
`GET /v1/sessions/{id}/history` (tmux capture / last `.pane` snapshot for that
session) still work independently of the recording store.

## What is stored

| Item | Location | Contents |
|------|----------|----------|
| Pane snapshot | `{jobs_dir}/recordings/<session_id>/<rec_id>.pane` | Terminal scrollback (may include ANSI color codes) |
| Metadata | `{jobs_dir}/recordings/<session_id>/<rec_id>.json` | id, session_id, agent, cwd, name, bytes, created_at, reason |

`reason` is one of: `kill`, `detach`, `periodic`, `manual` (best-effort).

File modes: directories `0700`, files `0600` (owner-only on a typical Unix host).

### What may appear in a snapshot

Anything that was visible in the agent TTY, including:

- Source code and paths under the workspace  
- Prompts you typed, agent output, tool results  
- Secrets pasted or printed into the terminal (API keys, tokens, env dumps)  
- Credentials agents echo or that tools print  

Treat the recordings directory like a **sensitive log**.

## Who can read

Anyone who authenticates to the API with a valid bearer (primary or
`extra_tokens` label) can:

| API | Purpose |
|-----|---------|
| `GET /v1/recordings` | List archives (optional `session_id`, `limit`) |
| `GET /v1/recordings/{id}` | Fetch meta + pane text |
| `POST /v1/sessions/{id}/record` | Manual snap while session is alive |
| `GET /v1/history/search?q=` | Substring search across archived panes |

There is **no** separate recording ACL. Multi-token labels are for audit/ops
identity, not isolation. Do not share tokens with people who should not see
terminal history.

CLI: `agentsctl recordings list|show|snap`.

Web UI may surface history/search when the product builds that panel; the same
API auth rules apply.

## Redaction

- Newer builds redact known secret patterns (`internal/redact`) and strip ANSI
  **before** writing archive files. This is best-effort — not a guarantee that
  every secret form is removed.
- Older builds may store the pane as captured; assume residual risk either way.
- Substring search may return snippets; treat search hits as sensitive.

If you need stronger long-term controls, keep recording off, scrub offline, or
rely on agent-native conversation stores only.

## Retention

- Prefer leaving recording **off** when you do not need archives.
- Disk under `{jobs_dir}/recordings/` grows with each snap until pruned.
- Newer builds may support prune-on-archive knobs in `[sessions]`:
  - `recording_max_per_session` — keep newest N per session (default often 20; `<0` unlimited)
  - `recording_max_age_days` — drop older than N days on archive (`0` = disabled)
- You can always delete files on disk manually or via host backup policy.
- `POST /v1/backup` may include the recordings tree — back up only if that is intentional.
- `DELETE /v1/recordings/{id}` may be available when the daemon exposes it.

Recommended ops:

1. Leave recording **off** on shared or internet-facing hosts.  
2. If enabled, set retention limits when available, or periodically clean `recordings/`.  
3. Exclude recordings from unencrypted offsite backups if they may contain secrets.

## When snapshots are taken

With `sessions.recording = true`:

- On session **kill** / stop (archive reason `kill`)  
- On detach paths that snapshot the pane (`detach`)  
- Optional **manual** capture via API / `agentsctl recordings snap`  
- Other reasons (`periodic`) if configured by future/server paths  

Exact triggers follow the session manager’s archive hook; empty panes are skipped.

## Related session history (not the same store)

| Feature | Default | Path |
|---------|---------|------|
| Live/last pane for one session | always available when session metadata exists | `jobs_dir/sessions/<id>.pane` + tmux capture |
| Opt-in multi-snapshot archive | **off** | `jobs_dir/recordings/…` |

`GET /v1/sessions/{id}/history` serves the best available scrollback for attach/preview
(live tmux or last `.pane`). That is **session history**, not the multi-id recording catalog.

## Operator checklist

- [ ] `recording` left unset or `false` unless needed  
- [ ] `jobs_dir` on disk with restricted permissions  
- [ ] Tokens not shared beyond people allowed to read TTY history  
- [ ] Retention/deletion plan if recording is on  
- [ ] Bind not public without tunnel + strong token  

See also [SECURITY.md](../SECURITY.md) and [SECURITY-OPS.md](./SECURITY-OPS.md).
