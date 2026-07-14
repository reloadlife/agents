import "./styles.css";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";

import {
  clearToken,
  createSession,
  generateMap,
  getMap,
  getMapStatus,
  getStatus,
  getToken,
  killSession,
  listAgents,
  listSessions,
  listWorkspaces,
  memoryIndex,
  memorySearch,
  memoryStats,
  playwrightRestart,
  playwrightStart,
  playwrightStatus,
  playwrightStop,
  pruneSessions,
  resumeSession,
  setToken,
  type MemoryHit,
  type Session,
} from "./api";
import { PtyClient } from "./pty";

type OpenTab = {
  id: string;
  title: string;
  agent: string;
  cwd: string;
  state: string;
};

type Panel = null | "new" | "tools" | "help";
type ConnState = "idle" | "connecting" | "live" | "reconnecting" | "error";
type ToastKind = "info" | "ok" | "err";

type AppState = {
  token: string;
  sessions: Session[];
  openTabs: OpenTab[];
  activeId: string | null;
  agents: string[];
  workspaces: string[];
  defaultCwd: string;
  formAgent: string;
  formCwd: string;
  formName: string;
  formPrompt: string;
  statusText: string;
  statusOk: boolean;
  toast: { msg: string; kind: ToastKind } | null;
  loginError: string;
  busy: boolean;
  shellMounted: boolean;
  mapStatus: string;
  memStatus: string;
  memQuery: string;
  memHits: MemoryHit[];
  drawer: null | { title: string; body: string };
  panel: Panel;
  filter: string;
  conn: ConnState;
  sidebarOpen: boolean;
  pwStatus: string;
};

const state: AppState = {
  token: getToken(),
  sessions: [],
  openTabs: [],
  activeId: null,
  agents: [],
  workspaces: ["."],
  defaultCwd: ".",
  formAgent: "claude",
  formCwd: ".",
  formName: "",
  formPrompt: "",
  statusText: "idle",
  statusOk: true,
  toast: null,
  loginError: "",
  busy: false,
  shellMounted: false,
  mapStatus: "—",
  memStatus: "—",
  memQuery: "",
  memHits: [],
  drawer: null,
  panel: null,
  filter: "",
  conn: "idle",
  sidebarOpen: false,
  pwStatus: "—",
};

/** Only one live PTY attach at a time (active tab). Others stay open in tmux. */
let activePty: PtyClient | null = null;
let attachedId: string | null = null;
let term: Terminal | null = null;
let fitAddon: FitAddon | null = null;
let resizeObserver: ResizeObserver | null = null;
let pollTimer: number | null = null;
let toastTimer: number | null = null;
let shellBound = false;
let keysBound = false;

const app = document.getElementById("app")!;

function toast(msg: string, kind: ToastKind = "info", ms = 3200): void {
  state.toast = { msg, kind };
  paintToast();
  if (toastTimer) window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => {
    state.toast = null;
    paintToast();
  }, ms);
}

function paintToast(): void {
  const el = document.getElementById("toast");
  if (!el) return;
  if (!state.toast) {
    el.hidden = true;
    el.textContent = "";
    el.className = "toast";
    return;
  }
  el.hidden = false;
  el.textContent = state.toast.msg;
  el.className = `toast toast-${state.toast.kind}`;
}

function shortId(id: string): string {
  if (id.length <= 12) return id;
  return id.slice(0, 8) + "…";
}

function tabTitle(s: Session | OpenTab): string {
  if ("name" in s && (s as Session).name) return (s as Session).name as string;
  const agent = "agent" in s ? s.agent : "";
  return `${agent} · ${shortId(s.id)}`;
}

function relativeTime(iso?: string): string {
  if (!iso) return "";
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return "";
  const sec = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h`;
  return `${Math.floor(sec / 86400)}d`;
}

function basename(path: string): string {
  const parts = path.replace(/\/+$/, "").split(/[/\\]/);
  return parts[parts.length - 1] || path;
}

async function bootstrapAuthed(): Promise<void> {
  state.busy = true;
  state.loginError = "";
  paint();
  try {
    const [agentsRes, wsRes, sessRes, st] = await Promise.all([
      listAgents(),
      listWorkspaces(),
      listSessions(),
      getStatus(),
    ]);
    state.agents =
      agentsRes.available?.length > 0
        ? agentsRes.available
        : (agentsRes.agents ?? []).map((a) => a.name).filter(Boolean);
    if (state.agents.length === 0) state.agents = ["claude", "grok", "codex"];
    if (!state.agents.includes(state.formAgent)) {
      state.formAgent = state.agents[0] ?? "claude";
    }
    state.workspaces = wsRes.workspaces?.length > 0 ? wsRes.workspaces : ["."];
    state.defaultCwd = wsRes.default_cwd || state.workspaces[0] || ".";
    if (!state.workspaces.includes(state.formCwd)) {
      state.formCwd = state.defaultCwd;
    }
    state.sessions = sessRes.sessions ?? [];
    state.statusText = formatStatus(st as Record<string, unknown>);
    state.statusOk = true;
    void refreshToolsStatus();
    startPolling();
  } catch (e) {
    const err = e as Error & { status?: number };
    if (err.status === 401) {
      clearToken();
      state.token = "";
      state.loginError = "Invalid token — check AGENTSD_TOKEN";
      state.shellMounted = false;
      stopPolling();
    } else {
      state.statusOk = false;
      state.statusText = err.message || "failed to load";
      state.loginError = err.message;
    }
  } finally {
    state.busy = false;
    paint();
  }
}

function formatStatus(st: Record<string, unknown>): string {
  const host = (st.hostname as string) || "host";
  const n =
    st.tty_sessions ??
    state.sessions.filter((s) => s.state === "running").length;
  return `${host} · ${n} live`;
}

function startPolling(): void {
  stopPolling();
  pollTimer = window.setInterval(() => {
    void refreshSessions();
  }, 5000);
}

function stopPolling(): void {
  if (pollTimer) {
    window.clearInterval(pollTimer);
    pollTimer = null;
  }
}

async function refreshSessions(): Promise<void> {
  if (!state.token) return;
  try {
    const [sessRes, st] = await Promise.all([listSessions(), getStatus()]);
    state.sessions = sessRes.sessions ?? [];
    for (const tab of state.openTabs) {
      const s = state.sessions.find((x) => x.id === tab.id);
      if (s) {
        tab.state = s.state;
        tab.agent = s.agent;
        tab.cwd = s.cwd;
        tab.title = tabTitle(s);
      }
    }
    state.statusText = formatStatus(st as Record<string, unknown>);
    state.statusOk = true;
    paintChrome();
  } catch (e) {
    const err = e as Error & { status?: number };
    if (err.status === 401) {
      logout();
      return;
    }
    state.statusOk = false;
    state.statusText = err.message || "poll failed";
    paintChrome();
  }
}

function logout(): void {
  detachPty();
  disposeTerminal();
  stopPolling();
  clearToken();
  state.token = "";
  state.sessions = [];
  state.openTabs = [];
  state.activeId = null;
  state.loginError = "";
  state.shellMounted = false;
  state.panel = null;
  state.conn = "idle";
  state.sidebarOpen = false;
  shellBound = false;
  paint();
}

function openTab(s: Session): void {
  if (!state.openTabs.find((t) => t.id === s.id)) {
    state.openTabs.push({
      id: s.id,
      title: tabTitle(s),
      agent: s.agent,
      cwd: s.cwd,
      state: s.state,
    });
  }
  state.sidebarOpen = false;
  void activateTab(s.id);
}

async function activateTab(id: string): Promise<void> {
  if (state.activeId === id && attachedId === id && activePty) {
    fit();
    term?.focus();
    return;
  }
  const prev = state.activeId;
  state.activeId = id;
  if (prev !== id) {
    detachPty();
    attachedId = null;
  }
  paintChrome();
  ensureTermArea();
  await attachActive();
}

function closeTab(id: string): void {
  state.openTabs = state.openTabs.filter((t) => t.id !== id);
  if (state.activeId === id) {
    detachPty();
    attachedId = null;
    state.activeId = state.openTabs[0]?.id ?? null;
    state.conn = state.activeId ? "connecting" : "idle";
    paintChrome();
    ensureTermArea();
    if (state.activeId) void attachActive();
    else {
      disposeTerminal();
      ensureTermArea();
    }
  } else {
    paintChrome();
  }
}

function detachPty(): void {
  if (activePty) {
    activePty.detach();
    activePty = null;
  }
  attachedId = null;
}

function disposeTerminal(): void {
  resizeObserver?.disconnect();
  resizeObserver = null;
  term?.dispose();
  term = null;
  fitAddon = null;
}

function ensureTerminal(container: HTMLElement): void {
  if (term) {
    if (!container.contains(term.element ?? null)) {
      term.open(container);
    }
    return;
  }
  term = new Terminal({
    cursorBlink: true,
    cursorStyle: "bar",
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Cascadia Code, monospace",
    fontSize: 13,
    lineHeight: 1.25,
    scrollback: 50000,
    convertEol: true,
    theme: {
      background: "#05070c",
      foreground: "#dce6f2",
      cursor: "#3de0c5",
      cursorAccent: "#05070c",
      selectionBackground: "#1e3a4a",
      black: "#0a0e14",
      red: "#ff6b6b",
      green: "#3de0c5",
      yellow: "#f5a524",
      blue: "#5b9fd4",
      magenta: "#c084fc",
      cyan: "#3de0c5",
      white: "#dce6f2",
      brightBlack: "#5a6a80",
      brightRed: "#ff8a8a",
      brightGreen: "#6eecd6",
      brightYellow: "#ffc14d",
      brightBlue: "#7eb8e8",
      brightMagenta: "#d4a5ff",
      brightCyan: "#6eecd6",
      brightWhite: "#ffffff",
    },
    allowProposedApi: true,
  });
  fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.loadAddon(new WebLinksAddon());
  term.open(container);
  term.onData((data) => {
    activePty?.write(data);
  });
  resizeObserver = new ResizeObserver(() => fit());
  resizeObserver.observe(container);
}

function fit(): void {
  if (!term || !fitAddon) return;
  try {
    fitAddon.fit();
    const dims = fitAddon.proposeDimensions();
    if (dims) activePty?.resize(dims.cols, dims.rows);
  } catch {
    /* ignore fit races */
  }
}

function setConn(c: ConnState): void {
  if (state.conn === c) return;
  state.conn = c;
  paintConn();
}

function paintConn(): void {
  const el = document.getElementById("conn-pill");
  if (!el) return;
  el.className = `conn-pill conn-${state.conn}`;
  const labels: Record<ConnState, string> = {
    idle: "idle",
    connecting: "connecting",
    live: "live",
    reconnecting: "reconnect…",
    error: "error",
  };
  el.innerHTML = `<span class="conn-dot"></span>${labels[state.conn]}`;
}

async function attachActive(): Promise<void> {
  const id = state.activeId;
  if (!id) {
    setConn("idle");
    return;
  }
  if (attachedId === id && activePty) {
    fit();
    setConn("live");
    return;
  }
  detachPty();
  const wrap = document.getElementById("term-host");
  if (!wrap) return;
  ensureTerminal(wrap);
  term?.reset();
  term?.focus();
  fit();
  setConn("connecting");
  const dims = fitAddon?.proposeDimensions() ?? { cols: 120, rows: 40 };

  const client = new PtyClient(id, state.token, {
    onData: (data) => {
      term?.write(data);
      if (state.conn !== "live") setConn("live");
    },
    onReady: () => setConn("live"),
    onError: (msg) => {
      setConn("error");
      toast(msg, "err");
    },
    onReconnect: (n) => {
      setConn("reconnecting");
      // Server re-seeds full scrollback on each attach — clear local buffer first.
      term?.reset();
      toast(`Reconnecting… (${n}) — agent still running`, "info", 2500);
    },
    onClose: (reason) => {
      if (reason !== "closed") {
        setConn("error");
        toast(reason, "err");
      } else if (attachedId === null) {
        setConn(state.activeId ? "connecting" : "idle");
      }
    },
  });
  activePty = client;
  attachedId = id;
  client.connect(dims.cols, dims.rows);
  window.setTimeout(() => fit(), 50);
}

/** Ensure term-wrap has host or empty state without full page redraw. */
function ensureTermArea(): void {
  const wrap = document.querySelector(".term-wrap");
  if (!wrap) return;
  const hasActive = !!state.activeId;
  const hasHost = !!document.getElementById("term-host");
  if (hasActive && !hasHost) {
    wrap.innerHTML = `
      <div id="term-host" class="term-host"></div>
      <div class="term-toolbar">
        <button type="button" class="btn-sm" id="btn-refit" title="Refit terminal (f)">Fit</button>
        <button type="button" class="btn-sm danger" id="btn-kill" title="Kill agent process">Kill</button>
      </div>
      <div class="toast" id="toast" hidden></div>`;
    document.getElementById("btn-refit")?.addEventListener("click", () => fit());
    document.getElementById("btn-kill")?.addEventListener("click", () => {
      if (state.activeId) void onKillSession(state.activeId);
    });
    if (term) disposeTerminal();
  } else if (!hasActive && hasHost) {
    detachPty();
    disposeTerminal();
    setConn("idle");
    wrap.innerHTML = emptyTermHTML();
    bindEmptyActions();
  } else if (!hasActive && !hasHost) {
    wrap.innerHTML = emptyTermHTML();
    bindEmptyActions();
  }
}

function emptyTermHTML(): string {
  return `
    <div class="term-empty">
      <div class="term-empty-card">
        <div class="term-empty-mark" aria-hidden="true">◈</div>
        <h2>No session open</h2>
        <p>Start an agent or open one from the rail. Closing a browser tab
        detaches only — the process keeps running in tmux.</p>
        <div class="term-empty-actions">
          <button type="button" class="primary" id="btn-empty-new">New session</button>
          <button type="button" class="ghost" id="btn-empty-help">Shortcuts</button>
        </div>
      </div>
      <div class="toast" id="toast" hidden></div>
    </div>`;
}

function bindEmptyActions(): void {
  document.getElementById("btn-empty-new")?.addEventListener("click", () => openPanel("new"));
  document.getElementById("btn-empty-help")?.addEventListener("click", () => openPanel("help"));
}

function openPanel(p: Panel): void {
  state.panel = p;
  paintPanel();
  if (p === "new") {
    window.setTimeout(() => {
      (document.getElementById("name") as HTMLInputElement | null)?.focus();
    }, 30);
  }
  if (p === "tools") void refreshToolsStatus();
}

function closePanel(): void {
  state.panel = null;
  paintPanel();
  term?.focus();
}

async function onCreateSession(ev: Event): Promise<void> {
  ev.preventDefault();
  if (state.busy) return;
  state.busy = true;
  const btn = document.querySelector(
    "#create-form button[type=submit]",
  ) as HTMLButtonElement | null;
  if (btn) btn.disabled = true;
  try {
    const sess = await createSession({
      agent: state.formAgent,
      cwd: state.formCwd,
      name: state.formName || undefined,
      prompt: state.formPrompt || undefined,
    });
    state.formName = "";
    state.formPrompt = "";
    closePanel();
    await refreshSessions();
    openTab(sess);
    toast(`Started ${sess.agent} — tab close keeps it running`, "ok");
  } catch (e) {
    toast((e as Error).message || "create failed", "err");
  } finally {
    state.busy = false;
    if (btn) btn.disabled = false;
  }
}

async function onKillSession(id: string): Promise<void> {
  const s = state.sessions.find((x) => x.id === id);
  const label = s ? tabTitle(s) : shortId(id);
  if (!confirm(`Kill “${label}”? This stops the agent in tmux.`)) return;
  try {
    await killSession(id);
    closeTab(id);
    await refreshSessions();
    toast("Session killed", "ok");
  } catch (e) {
    toast((e as Error).message || "kill failed", "err");
  }
}

async function onResumeSession(id: string): Promise<void> {
  try {
    toast("Resuming…", "info", 6000);
    const sess = await resumeSession(id);
    await refreshSessions();
    openTab(sess);
    toast(
      sess.state === "running"
        ? `Resumed ${sess.agent} — agent process (re)started if it was gone`
        : `Resume returned state ${sess.state}`,
      "ok",
    );
  } catch (e) {
    toast((e as Error).message || "resume failed", "err");
  }
}

async function onPrune(): Promise<void> {
  if (!confirm("Remove all non-running sessions from the list?")) return;
  try {
    const out = await pruneSessions();
    state.openTabs = state.openTabs.filter((t) => {
      const s = state.sessions.find((x) => x.id === t.id);
      return s?.state === "running";
    });
    if (state.activeId && !state.openTabs.find((t) => t.id === state.activeId)) {
      detachPty();
      disposeTerminal();
      state.activeId = state.openTabs[0]?.id ?? null;
      attachedId = null;
    }
    await refreshSessions();
    ensureTermArea();
    if (state.activeId) void attachActive();
    toast(`Pruned ${out.removed} session(s)`, "ok");
  } catch (e) {
    toast((e as Error).message || "prune failed", "err");
  }
}

async function refreshToolsStatus(): Promise<void> {
  const cwd = state.formCwd || ".";
  try {
    const [ms, mem, pw] = await Promise.all([
      getMapStatus(cwd),
      memoryStats(cwd),
      playwrightStatus().catch(() => null),
    ]);
    const st = ms.status ?? {};
    if (!st.exists) state.mapStatus = "no map";
    else if (st.stale) state.mapStatus = `stale · ${st.reason || "outdated"}`;
    else state.mapStatus = "fresh";
    const eng = mem.engine || "fts";
    state.memStatus = `${mem.docs ?? 0} docs · ${eng}`;
    if (mem.vector_enabled) {
      state.memStatus += ` · emb ${mem.docs_embedded ?? 0}`;
    }
    if (pw) {
      state.pwStatus = pw.running ? `running${pw.display ? ` · ${pw.display}` : ""}` : "stopped";
    } else {
      state.pwStatus = "unavailable";
    }
    paintTools();
  } catch {
    /* optional panels */
  }
}

async function onMapGenerate(): Promise<void> {
  try {
    const out = await generateMap(state.formCwd || ".");
    toast(`Map written · ${out.map_path || out.cwd}`, "ok");
    await refreshToolsStatus();
  } catch (e) {
    toast((e as Error).message || "map generate failed", "err");
  }
}

async function onMapShow(): Promise<void> {
  try {
    const out = await getMap(state.formCwd || ".");
    if (out.error || !out.markdown) {
      toast(out.error || "No map — generate first", "err");
      return;
    }
    state.drawer = {
      title: `Project map · ${out.cwd}`,
      body: out.markdown,
    };
    paintDrawer();
  } catch (e) {
    toast((e as Error).message || "map show failed", "err");
  }
}

async function onMemIndex(): Promise<void> {
  try {
    toast("Indexing…", "info", 8000);
    const out = await memoryIndex({
      cwd: state.formCwd || ".",
      clear: true,
      generate_map: true,
    });
    toast(`Indexed ${out.indexed} docs (total ${out.docs_total})`, "ok");
    await refreshToolsStatus();
  } catch (e) {
    toast((e as Error).message || "index failed", "err");
  }
}

async function onMemSearch(ev?: Event): Promise<void> {
  ev?.preventDefault();
  const q = state.memQuery.trim();
  if (!q) {
    toast("Enter a search query", "info");
    return;
  }
  try {
    const out = await memorySearch({
      cwd: state.formCwd || ".",
      query: q,
      limit: 10,
      mode: "auto",
    });
    state.memHits = out.hits ?? [];
    paintTools();
    if (state.memHits.length === 0) toast("No memory hits", "info");
  } catch (e) {
    toast((e as Error).message || "search failed", "err");
  }
}

async function onPwAction(action: "start" | "stop" | "restart"): Promise<void> {
  try {
    toast(`${action}ing playwright…`, "info");
    const fn =
      action === "start"
        ? playwrightStart
        : action === "stop"
          ? playwrightStop
          : playwrightRestart;
    const out = await fn();
    if (out.ok === false) {
      toast(out.error || `${action} failed`, "err");
    } else {
      toast(`Playwright ${action}ed`, "ok");
    }
    await refreshToolsStatus();
  } catch (e) {
    toast((e as Error).message || `${action} failed`, "err");
  }
}

function paintTools(): void {
  const mapEl = document.getElementById("map-status");
  if (mapEl) mapEl.textContent = state.mapStatus;
  const memEl = document.getElementById("mem-status");
  if (memEl) memEl.textContent = state.memStatus;
  const pwEl = document.getElementById("pw-status");
  if (pwEl) pwEl.textContent = state.pwStatus;
  const hits = document.getElementById("mem-hits");
  if (hits) hits.innerHTML = memHitsHTML();
  const cwdHint = document.getElementById("tools-cwd");
  if (cwdHint) cwdHint.textContent = state.formCwd || ".";
}

function paintDrawer(): void {
  let el = document.getElementById("drawer");
  if (!state.drawer) {
    el?.remove();
    return;
  }
  if (!el) {
    el = document.createElement("div");
    el.id = "drawer";
    el.className = "overlay";
    document.body.appendChild(el);
  }
  el.innerHTML = `
    <div class="modal modal-wide" role="dialog" aria-modal="true">
      <div class="modal-head">
        <strong>${esc(state.drawer.title)}</strong>
        <button type="button" class="ghost btn-sm" id="drawer-close">Close</button>
      </div>
      <pre class="drawer-body">${esc(state.drawer.body)}</pre>
    </div>`;
  el.onclick = (ev) => {
    if (ev.target === el) {
      state.drawer = null;
      paintDrawer();
    }
  };
  document.getElementById("drawer-close")?.addEventListener("click", () => {
    state.drawer = null;
    paintDrawer();
  });
}

function paintPanel(): void {
  let el = document.getElementById("panel-overlay");
  if (!state.panel) {
    el?.remove();
    return;
  }
  if (!el) {
    el = document.createElement("div");
    el.id = "panel-overlay";
    el.className = "overlay";
    document.body.appendChild(el);
  }
  if (state.panel === "new") {
    el.innerHTML = newSessionHTML();
    bindNewSessionForm();
  } else if (state.panel === "tools") {
    el.innerHTML = toolsHTML();
    bindToolsPanel();
  } else if (state.panel === "help") {
    el.innerHTML = helpHTML();
    document.getElementById("panel-close")?.addEventListener("click", () => closePanel());
  }
  el.onclick = (ev) => {
    if (ev.target === el) closePanel();
  };
}

function newSessionHTML(): string {
  return `
    <div class="modal" role="dialog" aria-modal="true" aria-labelledby="new-title">
      <div class="modal-head">
        <div>
          <div class="eyebrow">Session</div>
          <h2 id="new-title">New agent</h2>
        </div>
        <button type="button" class="ghost btn-sm" id="panel-close">Esc</button>
      </div>
      <form id="create-form" class="modal-body">
        <div class="field-grid">
          <div class="field">
            <label for="agent">Agent</label>
            <select id="agent" name="agent">
              ${state.agents
                .map(
                  (a) =>
                    `<option value="${esc(a)}" ${a === state.formAgent ? "selected" : ""}>${esc(a)}</option>`,
                )
                .join("")}
            </select>
          </div>
          <div class="field">
            <label for="cwd">Workspace</label>
            <select id="cwd" name="cwd">
              ${state.workspaces
                .map(
                  (w) =>
                    `<option value="${esc(w)}" ${w === state.formCwd ? "selected" : ""}>${esc(w)}</option>`,
                )
                .join("")}
            </select>
          </div>
        </div>
        <div class="field">
          <label for="name">Name <span class="opt">optional</span></label>
          <input id="name" name="name" value="${esc(state.formName)}" placeholder="e.g. fix-auth" autocomplete="off" />
        </div>
        <div class="field">
          <label for="prompt">Seed prompt <span class="opt">optional</span></label>
          <textarea id="prompt" name="prompt" rows="3" placeholder="Typed into the TTY after start">${esc(state.formPrompt)}</textarea>
        </div>
        <p class="form-hint">Runs in server-side tmux. Detach anytime — kill only when you mean stop.</p>
        <div class="modal-actions">
          <button type="button" class="ghost" id="panel-cancel">Cancel</button>
          <button class="primary" type="submit" ${state.busy ? "disabled" : ""}>Start session</button>
        </div>
      </form>
    </div>`;
}

function toolsHTML(): string {
  return `
    <div class="modal modal-tools" role="dialog" aria-modal="true" aria-labelledby="tools-title">
      <div class="modal-head">
        <div>
          <div class="eyebrow">Workspace tools</div>
          <h2 id="tools-title">Map · Memory · Browser</h2>
        </div>
        <button type="button" class="ghost btn-sm" id="panel-close">Esc</button>
      </div>
      <div class="modal-body tools-body">
        <p class="tools-cwd-line">Target: <code id="tools-cwd">${esc(state.formCwd || ".")}</code>
          <span class="muted"> (from last new-session workspace)</span></p>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>Project map</h3>
            <span class="tool-status" id="map-status">${esc(state.mapStatus)}</span>
          </div>
          <p class="tool-desc">Durable orientation file under <code>.agents/PROJECT_MAP.md</code>.</p>
          <div class="btn-row">
            <button type="button" id="btn-map-gen">Generate</button>
            <button type="button" id="btn-map-show">Show</button>
          </div>
        </section>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>Memory</h3>
            <span class="tool-status" id="mem-status">${esc(state.memStatus)}</span>
          </div>
          <p class="tool-desc">FTS (and optional vectors) over workspace docs for agent retrieval.</p>
          <div class="btn-row">
            <button type="button" id="btn-mem-index">Reindex</button>
          </div>
          <form id="mem-search-form" class="mem-search">
            <input id="mem-query" placeholder="Search docs…" value="${esc(state.memQuery)}" />
            <button type="submit" class="primary">Search</button>
          </form>
          <div id="mem-hits" class="mem-hits">${memHitsHTML()}</div>
        </section>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>Playwright</h3>
            <span class="tool-status" id="pw-status">${esc(state.pwStatus)}</span>
          </div>
          <p class="tool-desc">Headed browser stack for agents that need a real browser.</p>
          <div class="btn-row">
            <button type="button" id="btn-pw-start">Start</button>
            <button type="button" id="btn-pw-stop">Stop</button>
            <button type="button" id="btn-pw-restart">Restart</button>
          </div>
        </section>
      </div>
    </div>`;
}

function helpHTML(): string {
  return `
    <div class="modal modal-sm" role="dialog" aria-modal="true" aria-labelledby="help-title">
      <div class="modal-head">
        <div>
          <div class="eyebrow">Keyboard</div>
          <h2 id="help-title">Shortcuts</h2>
        </div>
        <button type="button" class="ghost btn-sm" id="panel-close">Esc</button>
      </div>
      <div class="modal-body">
        <dl class="keys">
          <div><dt><kbd>n</kbd></dt><dd>New session</dd></div>
          <div><dt><kbd>t</kbd></dt><dd>Tools panel</dd></div>
          <div><dt><kbd>/</kbd></dt><dd>Focus session filter</dd></div>
          <div><dt><kbd>f</kbd></dt><dd>Fit terminal</dd></div>
          <div><dt><kbd>?</kbd></dt><dd>This help</dd></div>
          <div><dt><kbd>Esc</kbd></dt><dd>Close panel / sidebar</dd></div>
          <div><dt><kbd>1</kbd>–<kbd>9</kbd></dt><dd>Switch open tab</dd></div>
          <div><dt><kbd>w</kbd></dt><dd>Toggle session rail</dd></div>
        </dl>
        <p class="form-hint">Shortcuts ignore events when typing in inputs. Closing a tab never kills the agent.</p>
      </div>
    </div>`;
}

function bindNewSessionForm(): void {
  document.getElementById("panel-close")?.addEventListener("click", () => closePanel());
  document.getElementById("panel-cancel")?.addEventListener("click", () => closePanel());
  const form = document.getElementById("create-form") as HTMLFormElement | null;
  form?.addEventListener("submit", (ev) => void onCreateSession(ev));
  form?.querySelector("#agent")?.addEventListener("change", (e) => {
    state.formAgent = (e.target as HTMLSelectElement).value;
  });
  form?.querySelector("#cwd")?.addEventListener("change", (e) => {
    state.formCwd = (e.target as HTMLSelectElement).value;
  });
  form?.querySelector("#name")?.addEventListener("input", (e) => {
    state.formName = (e.target as HTMLInputElement).value;
  });
  form?.querySelector("#prompt")?.addEventListener("input", (e) => {
    state.formPrompt = (e.target as HTMLTextAreaElement).value;
  });
}

function bindToolsPanel(): void {
  document.getElementById("panel-close")?.addEventListener("click", () => closePanel());
  document.getElementById("btn-map-gen")?.addEventListener("click", () => void onMapGenerate());
  document.getElementById("btn-map-show")?.addEventListener("click", () => void onMapShow());
  document.getElementById("btn-mem-index")?.addEventListener("click", () => void onMemIndex());
  document.getElementById("mem-search-form")?.addEventListener("submit", (ev) => void onMemSearch(ev));
  document.getElementById("mem-query")?.addEventListener("input", (e) => {
    state.memQuery = (e.target as HTMLInputElement).value;
  });
  document.getElementById("btn-pw-start")?.addEventListener("click", () => void onPwAction("start"));
  document.getElementById("btn-pw-stop")?.addEventListener("click", () => void onPwAction("stop"));
  document.getElementById("btn-pw-restart")?.addEventListener("click", () => void onPwAction("restart"));
}

function memHitsHTML(): string {
  if (!state.memHits.length) {
    return `<div class="empty-soft">No results yet</div>`;
  }
  return state.memHits
    .map((h) => {
      const mode = h.mode || "fts";
      return `<div class="mem-hit">
        <div class="path">${esc(h.path || h.title || "?")} <span class="badge">${esc(mode)}</span></div>
        <div class="snip">${esc(h.snippet || "")}</div>
      </div>`;
    })
    .join("");
}

function filteredSessions(): Session[] {
  const q = state.filter.trim().toLowerCase();
  if (!q) return state.sessions;
  return state.sessions.filter((s) => {
    const hay = `${s.name || ""} ${s.agent} ${s.cwd} ${s.id} ${s.state}`.toLowerCase();
    return hay.includes(q);
  });
}

/** Full paint: login or initial shell mount. */
function paint(): void {
  if (!state.token) {
    stopPolling();
    state.shellMounted = false;
    shellBound = false;
    app.innerHTML = loginHTML();
    bindLogin();
    return;
  }
  if (!state.shellMounted) {
    app.innerHTML = shellHTML();
    state.shellMounted = true;
    shellBound = false;
    bindShell();
    ensureTermArea();
    paintPanel();
    paintDrawer();
    if (state.activeId) void attachActive();
    return;
  }
  paintChrome();
}

function paintChrome(): void {
  if (!state.shellMounted) return;
  const list = document.getElementById("session-list");
  if (list) list.innerHTML = sessionListHTML();
  const tabs = document.getElementById("tabs");
  if (tabs) tabs.innerHTML = tabsHTML();
  const status = document.getElementById("status-bar");
  if (status) status.innerHTML = statusHTML();
  const count = document.getElementById("sess-count");
  if (count) {
    const run = state.sessions.filter((s) => s.state === "running").length;
    count.textContent = `${run}/${state.sessions.length}`;
  }
  const shell = document.querySelector(".shell");
  shell?.classList.toggle("sidebar-open", state.sidebarOpen);
  paintConn();
  paintToast();
  bindSessionList();
  bindTabs();
}

function loginHTML(): string {
  return `
  <div class="login">
    <div class="login-card">
      <div class="login-brand">
        <span class="logo-mark" aria-hidden="true">◈</span>
        <div>
          <h1>agents</h1>
          <p class="sub">Remote control plane</p>
        </div>
      </div>
      <p class="login-lede">Paste the same <code>AGENTSD_TOKEN</code> you use with <code>agentsctl</code>.</p>
      <form id="login-form">
        <div class="field">
          <label for="token">Bearer token</label>
          <input id="token" name="token" type="password" autocomplete="current-password" placeholder="••••••••" required autofocus />
        </div>
        <button class="primary" type="submit" style="width:100%">Connect</button>
        ${state.loginError ? `<p class="error">${esc(state.loginError)}</p>` : ""}
        ${state.busy ? `<p class="login-busy">Connecting…</p>` : ""}
      </form>
      <p class="hint">Token stays in this browser’s localStorage. Prefer localhost or a private tunnel — the token is shell access to agent tools.</p>
    </div>
  </div>`;
}

function shellHTML(): string {
  return `
  <div class="shell${state.sidebarOpen ? " sidebar-open" : ""}">
    <div class="sidebar-backdrop" id="sidebar-backdrop" hidden></div>
    <aside class="sidebar" id="sidebar">
      <div class="sidebar-header">
        <div class="brand">
          <span class="logo-mark sm" aria-hidden="true">◈</span>
          <span class="brand-name">agents</span>
        </div>
        <button type="button" class="ghost btn-icon" id="btn-logout" title="Logout">⎋</button>
      </div>

      <div class="sidebar-actions">
        <button type="button" class="primary sidebar-new" id="btn-new">
          <span>+ New session</span>
          <kbd>n</kbd>
        </button>
      </div>

      <div class="sidebar-filter">
        <input id="filter" type="search" placeholder="Filter sessions…" value="${esc(state.filter)}" autocomplete="off" />
        <span class="sess-count" id="sess-count" title="running / total">0/0</span>
      </div>

      <div class="session-list" id="session-list">
        ${sessionListHTML()}
      </div>

      <div class="sidebar-foot">
        <button type="button" class="ghost btn-sm" id="btn-prune" title="Remove non-running sessions">Prune</button>
        <button type="button" class="ghost btn-sm" id="btn-tools-side">Tools</button>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <button type="button" class="ghost btn-icon mobile-only" id="btn-sidebar" title="Sessions (w)" aria-label="Toggle sessions">☰</button>
        <div class="tabs" id="tabs">${tabsHTML()}</div>
        <div class="topbar-actions">
          <span class="conn-pill conn-${state.conn}" id="conn-pill"><span class="conn-dot"></span>idle</span>
          <button type="button" class="ghost btn-sm" id="btn-tools" title="Tools (t)">Tools</button>
          <button type="button" class="ghost btn-sm" id="btn-help" title="Shortcuts (?)">?</button>
        </div>
      </header>
      <div class="term-wrap">
        <div class="term-empty">
          <div class="term-empty-card">
            <div class="term-empty-mark" aria-hidden="true">◈</div>
            <h2>No session open</h2>
            <p>Start an agent or open one from the rail.</p>
          </div>
        </div>
      </div>
      <footer class="status-bar" id="status-bar">${statusHTML()}</footer>
    </main>
  </div>`;
}

function sessionListHTML(): string {
  const list = filteredSessions();
  if (state.sessions.length === 0) {
    return `<div class="empty-list">
      <p>No sessions yet</p>
      <button type="button" class="primary btn-sm" id="btn-list-new">Start one</button>
    </div>`;
  }
  if (list.length === 0) {
    return `<div class="empty-list"><p>No matches for “${esc(state.filter)}”</p></div>`;
  }
  // running first, then by created_at desc if present
  const sorted = [...list].sort((a, b) => {
    if (a.state === "running" && b.state !== "running") return -1;
    if (b.state === "running" && a.state !== "running") return 1;
    const ta = a.created_at ? Date.parse(a.created_at) : 0;
    const tb = b.created_at ? Date.parse(b.created_at) : 0;
    return tb - ta;
  });
  return sorted
    .map((s) => {
      const active = s.id === state.activeId ? "active" : "";
      const age = relativeTime(s.created_at);
      return `
      <div class="session-item ${active}" data-open="${esc(s.id)}">
        <div class="session-item-main">
          <div class="session-top">
            <span class="session-name">${esc(s.name || shortId(s.id))}</span>
            <span class="badge ${esc(s.state)}">${esc(s.state)}</span>
          </div>
          <div class="session-meta">
            <span class="agent-chip">${esc(s.agent)}</span>
            <span class="cwd-chip" title="${esc(s.cwd)}">${esc(basename(s.cwd))}</span>
            ${age ? `<span class="age">${esc(age)}</span>` : ""}
          </div>
        </div>
        <div class="session-actions">
          <button type="button" class="ghost btn-icon sm" data-copy="${esc(s.id)}" title="Copy id">⎘</button>
          ${
            s.state === "running"
              ? `<button type="button" class="ghost btn-icon sm danger-text" data-kill="${esc(s.id)}" title="Kill">×</button>`
              : `<button type="button" class="ghost btn-icon sm resume-text" data-resume="${esc(s.id)}" title="Resume (restart agent if dead)">↻</button>`
          }
        </div>
      </div>`;
    })
    .join("");
}

function tabsHTML(): string {
  if (state.openTabs.length === 0) {
    return `<div class="tabs-empty">Open a session from the rail →</div>`;
  }
  return state.openTabs
    .map((t, i) => {
      const active = t.id === state.activeId ? "active" : "";
      const live = t.state === "running" ? "live" : "dead";
      return `
      <div class="tab ${active} ${live}" data-tab="${esc(t.id)}" title="${esc(t.id)}">
        <span class="tab-dot"></span>
        <span class="tab-label">${esc(t.title)}</span>
        ${i < 9 ? `<span class="tab-num">${i + 1}</span>` : ""}
        <button type="button" class="tab-close" data-close="${esc(t.id)}" title="Close tab (keeps agent running)">×</button>
      </div>`;
    })
    .join("");
}

function statusHTML(): string {
  const cls = state.statusOk ? "ok" : "err";
  const open = state.openTabs.length;
  return `
    <span class="status-dot ${cls}"></span>
    <span>${esc(state.statusText)}</span>
    <span class="sep">·</span>
    <span>${open} tab${open === 1 ? "" : "s"} open</span>
    <span class="sep">·</span>
    <span class="status-hint">detach ≠ kill</span>
  `;
}

function esc(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function bindLogin(): void {
  const form = document.getElementById("login-form") as HTMLFormElement | null;
  form?.addEventListener("submit", (ev) => {
    ev.preventDefault();
    const input = document.getElementById("token") as HTMLInputElement;
    const t = input.value.trim();
    if (!t) return;
    setToken(t);
    state.token = t;
    void bootstrapAuthed();
  });
}

function isTypingTarget(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  const tag = el.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  return el.isContentEditable;
}

function bindKeys(): void {
  if (keysBound) return;
  keysBound = true;
  window.addEventListener("keydown", (ev) => {
    if (!state.token || !state.shellMounted) return;

    if (ev.key === "Escape") {
      if (state.drawer) {
        state.drawer = null;
        paintDrawer();
        ev.preventDefault();
        return;
      }
      if (state.panel) {
        closePanel();
        ev.preventDefault();
        return;
      }
      if (state.sidebarOpen) {
        state.sidebarOpen = false;
        paintChrome();
        ev.preventDefault();
      }
      return;
    }

    if (isTypingTarget(ev.target)) return;
    if (ev.metaKey || ev.ctrlKey || ev.altKey) return;

    const k = ev.key;
    if (k === "n" || k === "N") {
      ev.preventDefault();
      openPanel("new");
      return;
    }
    if (k === "t" || k === "T") {
      ev.preventDefault();
      openPanel("tools");
      return;
    }
    if (k === "?" || (k === "/" && ev.shiftKey)) {
      ev.preventDefault();
      openPanel("help");
      return;
    }
    if (k === "/") {
      ev.preventDefault();
      state.sidebarOpen = true;
      paintChrome();
      (document.getElementById("filter") as HTMLInputElement | null)?.focus();
      return;
    }
    if (k === "f" || k === "F") {
      if (state.activeId) {
        ev.preventDefault();
        fit();
      }
      return;
    }
    if (k === "w" || k === "W") {
      ev.preventDefault();
      state.sidebarOpen = !state.sidebarOpen;
      paintChrome();
      return;
    }
    if (k >= "1" && k <= "9") {
      const idx = Number(k) - 1;
      const tab = state.openTabs[idx];
      if (tab) {
        ev.preventDefault();
        void activateTab(tab.id);
      }
    }
  });
}

function bindShell(): void {
  if (shellBound) return;
  shellBound = true;
  bindKeys();

  document.getElementById("btn-logout")?.addEventListener("click", () => logout());
  document.getElementById("btn-new")?.addEventListener("click", () => openPanel("new"));
  document.getElementById("btn-tools")?.addEventListener("click", () => openPanel("tools"));
  document.getElementById("btn-tools-side")?.addEventListener("click", () => openPanel("tools"));
  document.getElementById("btn-help")?.addEventListener("click", () => openPanel("help"));
  document.getElementById("btn-prune")?.addEventListener("click", () => void onPrune());
  document.getElementById("btn-sidebar")?.addEventListener("click", () => {
    state.sidebarOpen = !state.sidebarOpen;
    paintChrome();
  });
  document.getElementById("sidebar-backdrop")?.addEventListener("click", () => {
    state.sidebarOpen = false;
    paintChrome();
  });

  const filter = document.getElementById("filter") as HTMLInputElement | null;
  filter?.addEventListener("input", (e) => {
    state.filter = (e.target as HTMLInputElement).value;
    const list = document.getElementById("session-list");
    if (list) {
      list.innerHTML = sessionListHTML();
      bindSessionList();
    }
  });

  bindSessionList();
  bindTabs();
  window.addEventListener("resize", () => fit());
}

function bindSessionList(): void {
  document.getElementById("btn-list-new")?.addEventListener("click", () => openPanel("new"));

  document.querySelectorAll<HTMLElement>("[data-open]").forEach((el) => {
    el.onclick = (ev) => {
      const t = ev.target as HTMLElement;
      if (t.closest("[data-kill], [data-copy], [data-resume]")) return;
      const id = el.getAttribute("data-open");
      if (!id) return;
      const s = state.sessions.find((x) => x.id === id);
      if (!s) return;
      if (s.state !== "running") {
        void onResumeSession(s.id);
        return;
      }
      openTab(s);
    };
  });

  document.querySelectorAll<HTMLElement>("[data-kill]").forEach((el) => {
    el.onclick = (ev) => {
      ev.stopPropagation();
      const id = el.getAttribute("data-kill");
      if (id) void onKillSession(id);
    };
  });

  document.querySelectorAll<HTMLElement>("[data-resume]").forEach((el) => {
    el.onclick = (ev) => {
      ev.stopPropagation();
      const id = el.getAttribute("data-resume");
      if (id) void onResumeSession(id);
    };
  });

  document.querySelectorAll<HTMLElement>("[data-copy]").forEach((el) => {
    el.onclick = async (ev) => {
      ev.stopPropagation();
      const id = el.getAttribute("data-copy");
      if (!id) return;
      try {
        await navigator.clipboard.writeText(id);
        toast("Session id copied", "ok", 1500);
      } catch {
        toast(id, "info", 4000);
      }
    };
  });
}

function bindTabs(): void {
  document.querySelectorAll<HTMLElement>("[data-tab]").forEach((el) => {
    el.onclick = (ev) => {
      const t = ev.target as HTMLElement;
      if (t.closest("[data-close]")) return;
      const id = el.getAttribute("data-tab");
      if (id) void activateTab(id);
    };
  });
  document.querySelectorAll<HTMLElement>("[data-close]").forEach((el) => {
    el.onclick = (ev) => {
      ev.stopPropagation();
      const id = el.getAttribute("data-close");
      if (id) closeTab(id);
    };
  });
}

// boot
if (state.token) {
  void bootstrapAuthed();
} else {
  paint();
}
