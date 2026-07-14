# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.8.0] — 2026-07-14

### Changed

- **Full shadcn/ui UX pass** (6 parallel UI agents)
  - Zinc design tokens, button/input/popover primitives
  - Sidebar-08 style rail: Create / Workspace groups + dense sessions
  - Topbar breadcrumbs, session tabs, empty-desk card
  - Vaul sheets: dialog chrome, descriptions, sticky form actions
  - Settings page: nav rail + account/GitHub/SSH cards
  - Palette, context menu, toast, login overlays

## [0.7.9] — 2026-07-14

### Added

- **Git Changes UI polish** — worktree checkbox on New session; API field alignment (`additions` / `commit` SHA)
- Embedded web dist rebuild for Changes panel + worktree form

### Changed

- Changelog organized per ship step (v0.7.2–v0.7.9)

## [0.7.8] — 2026-07-14

### Added

- **Workspace tasks** — lightweight list at `<cwd>/.agents/tasks.json`
  - API: `GET/POST /v1/tasks`, `PATCH/DELETE /v1/tasks/{id}` (`todo`|`doing`|`done`)
  - Web: Tools sheet section + palette “Workspace tasks” panel

## [0.7.7] — 2026-07-14

### Added

- **Session git branch** — `GET /v1/sessions` includes best-effort `git_branch`
  - Short-timeout `git rev-parse`; deduped per cwd; worktree branch fallback
  - Web list meta: `agent · cwd · branch · age`; filter matches branch

## [0.7.6] — 2026-07-14

### Changed

- **Web performance**
  - Session list / tabs / status paint only when display signatures change
  - Debounced filter (120ms); poll no longer thrashing full chrome
  - Lighter motion (no stuck opacity); `content-visibility` on session rows
  - Tab-close via document delegation

## [0.7.5] — 2026-07-14

### Added

- **Git write APIs** (never auto-push)
  - `POST /v1/git/commit` — `{ cwd, message, all?, paths? }`
  - `POST /v1/git/pull-request` — `{ cwd, title, body?, base?, draft? }` via `gh pr create`

## [0.7.4] — 2026-07-14

### Added

- **Git read APIs** for workspace review
  - `GET /v1/git/status?cwd=` — branch, dirty, files, summary
  - `GET /v1/git/diff?cwd=&path=&staged=&base=` — unified diff (1.5 MiB cap)
  - `GET /v1/git/file?cwd=&path=&ref=` — blob at ref
- Web **Changes** panel (`/changes`, sidebar, `⇧g`) — file list + diff + commit/PR forms

## [0.7.3] — 2026-07-14

### Added

- **Isolated git worktrees** for parallel agents
  - `POST /v1/sessions` `{ "worktree": true, "worktree_branch"?: "…" }`
  - Worktree under `.agents/worktrees/<id>`; session `cwd` points at worktree
  - Delete best-effort `git worktree remove` (never fails session delete)

## [0.7.2] — 2026-07-14

### Added

- **Open remote file + line**
  - `POST /v1/workspaces/open` accepts `path` / `line`
  - Cursor / VS Code / Zed remote and local goto-style commands

## [0.7.1] — 2026-07-14

### Fixed

- **Image paste into the web terminal**
  - Paste or drag-drop images onto the PTY
  - Uploads to `<cwd>/.agents/pastes/` via `POST /v1/uploads/image`
  - Inserts the absolute file path into the terminal so agents can `Read` it

## [0.7.0] — 2026-07-14

### Added

- **Session recording** (`sessions.recording`) — archive pane snapshots under `jobs_dir/recordings`
- **History search** — `GET /v1/history/search?q=`
- Templates, audit, notify, command palette foundations

## Earlier

See git tags `v0.6.x` … `v0.2.x` and commit history for prior releases.
