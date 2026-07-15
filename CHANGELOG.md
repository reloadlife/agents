# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.8.10] — 2026-07-15

### Added

- **Git worktrees for agentic sessions** (full product surface)
  - New session: worktree toggle (on by default) + optional branch name
  - Session list `wt` badge when isolated
  - Projects: **Worktree** quick-start action
  - Git page: worktrees panel + start session from a linked worktree
  - API: `GET /v1/git/worktrees?cwd=`

## [0.8.9] — 2026-07-15

### Added

- **Projects list page** (`/projects`) — browse workspaces with live session counts, map/context chips, and actions (Session / Tools / Git / Remote)
  - Sidebar **Projects** entry + palette + more-menu
  - Filter + refresh; uses dashboard API with list fallback

## [0.8.8] — 2026-07-15

### Added

- **New session → new directory** — create a workspace folder under the host root when starting a session
  - API: `POST /v1/workspaces` `{ "name": "my-app" }`
  - UI: Existing / New directory toggle on the New session page

## [0.8.7] — 2026-07-15

### Changed

- **In-shell pages for Tools / New session / New project / Open remote / Help**
  - Same full-stage pattern as Git Changes (sidebar + topbar stay; terminal replaced)
  - Shared page hero, cards, and actions layout
  - Routes: `/tools`, `/`, `/project/new`, `/remote`, `/help`
  - Open remote is a page with copyable editor commands (not a text drawer)

## [0.8.6] — 2026-07-14

### Fixed

- **Web UI updates on normal refresh** after agentsd upgrade
  - `index.html` served with `Cache-Control: no-store` (no more stale shell)
  - Hashed `/assets/*` long-cached (`immutable`)
  - Injected `agents-build` fingerprint + client soft-reload when the daemon ships a new UI

## [0.8.5] — 2026-07-14

### Changed

- **Topbar search + more menu redesign**
  - Search: pill-shaped trigger with platform-aware ⌘/Ctrl+K chip
  - More: richer portaled menu with icon tiles, labels + hints, danger row
  - Spotlight palette: cleaner input row, result count, esc chip

## [0.8.4] — 2026-07-14

### Changed

- **Git Changes UI overhaul** — real in-shell product page (not a modal/overlay)
  - Hero header with branch chips, dirty/clean, +/- totals, workspace switcher
  - File rail with filter, status pills, multi-line rows, commit checkboxes
  - Diff canvas with sticky hunk headers, dual line gutters, clearer add/del
  - Bottom composer: multi-line commit message, scope, primary CTA, expandable PR

## [0.8.3] — 2026-07-14

### Added

- **Spotlight search** — topbar search pill opens a centered command palette
  - Search sessions by name / agent / cwd / branch
  - ↑↓ / ↵ / Esc keyboard navigation
- **Git Changes full page** (`/changes`, `/git`) — replaces the modal sheet
  - Files list · full-height diff with line gutters · commit & PR side cards
  - Workspace switcher, branch badges, dirty/clean + totals

### Fixed

- **Topbar ⋮ menu under terminal** — portaled body-level menu with raised z-index
- Restored command-palette and git panel CSS dropped in the Vaul→modal rewrite

### Changed

- Topbar more-menu: grouped Create / Workspace / App sections with icons

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
