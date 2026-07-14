import "./styles.css";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";

import {
  clearToken,
  cloneWorkspace,
  createSession,
  deleteSSHKey,
  deleteSession,
  generateMap,
  generateSSHKey,
  getMap,
  getMapStatus,
  getStatus,
  getToken,
  killSession,
  listAgents,
  listSessions,
  listSSHKeys,
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
  type SSHKey,
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
  /** New project from git */
  formGitUrl: string;
  formGitName: string;
  formGitBranch: string;
  formGitFork: boolean;
  formGitDepth: boolean;
  statusText: string;
  statusOk: boolean;
  toast: { msg: string; kind: ToastKind } | null;
  loginError: string;
  /** bootstrap / global load */
  busy: boolean;
  /** create-session in flight */
  creating: boolean;
  createError: string;
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
  sshKeys: SSHKey[];
  sshDir: string;
  sshGenName: string;
  sshGenComment: string;
  sshBusy: boolean;
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
  formGitUrl: "",
  formGitName: "",
  formGitBranch: "",
  formGitFork: false,
  formGitDepth: true,
  statusText: "idle",
  statusOk: true,
  toast: null,
  loginError: "",
  busy: false,
  creating: false,
  createError: "",
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
  sshKeys: [],
  sshDir: "",
  sshGenName: "id_agents",
  sshGenComment: "",
  sshBusy: false,
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
let uiDelegated = false;

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

/** Toasts live on document.body above modals (z-index), not inside term-wrap. */
function paintToast(): void {
  let el = document.getElementById("app-toast");
  if (!el) {
    el = document.createElement("div");
    el.id = "app-toast";
    el.className = "app-toast";
    el.hidden = true;
    document.body.appendChild(el);
  }
  if (!state.toast) {
    el.hidden = true;
    el.textContent = "";
    el.className = "app-toast";
    return;
  }
  el.hidden = false;
  el.textContent = state.toast.msg;
  el.className = `app-toast toast-${state.toast.kind}`;
}

function shortId(id: string): string {
  if (id.length <= 12) return id;
  return id.slice(0, 8) + "…";
}

/** Map agent name → CSS class for brand colors. */
function agentClass(name: string): string {
  const k = (name || "").toLowerCase().replace(/[^a-z0-9]+/g, "");
  if (k.includes("claude") || k.includes("anthropic")) return "agent-claude";
  if (k.includes("grok") || k.includes("xai")) return "agent-grok";
  if (k.includes("codex") || k.includes("openai") || k === "gpt" || k.startsWith("gpt"))
    return "agent-codex";
  if (k.includes("cursor")) return "agent-cursor";
  if (k.includes("opencode")) return "agent-opencode";
  if (k.includes("gemini") || k.includes("google")) return "agent-gemini";
  if (k.includes("copilot")) return "agent-copilot";
  if (k.includes("aider")) return "agent-aider";
  return "agent-default";
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

function normalizeWorkspacePaths(wsRes: {
  paths?: string[];
  workspaces?: Array<string | { path?: string }>;
}): string[] {
  if (wsRes.paths && wsRes.paths.length > 0) return wsRes.paths;
  const raw = wsRes.workspaces ?? [];
  const out: string[] = [];
  for (const w of raw) {
    if (typeof w === "string") out.push(w);
    else if (w && typeof w.path === "string") out.push(w.path);
  }
  return out.length > 0 ? out : ["."];
}

async function refreshWorkspaceList(): Promise<void> {
  try {
    const wsRes = await listWorkspaces();
    state.workspaces = normalizeWorkspacePaths(wsRes);
    if (!state.workspaces.includes(state.formCwd)) {
      state.formCwd = wsRes.default_cwd || state.workspaces[0] || ".";
    }
  } catch {
    /* ignore */
  }
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
    state.workspaces = normalizeWorkspacePaths(wsRes);
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
      background: "#0c0a08",
      foreground: "#e8e2d6",
      cursor: "#c9a66b",
      cursorAccent: "#0c0a08",
      selectionBackground: "#3a3228",
      black: "#0c0a08",
      red: "#c47862",
      green: "#7f9a72",
      yellow: "#c9a66b",
      blue: "#8a9bb5",
      magenta: "#b794f6",
      cyan: "#7a9aaa",
      white: "#e8e2d6",
      brightBlack: "#6e665c",
      brightRed: "#d49484",
      brightGreen: "#96b08f",
      brightYellow: "#d4b54a",
      brightBlue: "#a0b0c8",
      brightMagenta: "#c0a4cc",
      brightCyan: "#95b4c0",
      brightWhite: "#f5f0e8",
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
        <button type="button" class="btn-sm" id="btn-kill" title="Stop agent (keeps list entry)">Stop</button>
        <button type="button" class="btn-sm danger" id="btn-delete" title="Stop and delete session">Delete</button>
      </div>`;
    document.getElementById("btn-refit")?.addEventListener("click", () => fit());
    document.getElementById("btn-kill")?.addEventListener("click", () => {
      if (state.activeId) void onKillSession(state.activeId);
    });
    document.getElementById("btn-delete")?.addEventListener("click", () => {
      if (state.activeId) void onDeleteSession(state.activeId);
    });
    if (term) disposeTerminal();
  } else if (!hasActive && hasHost) {
    detachPty();
    disposeTerminal();
    setConn("idle");
    wrap.innerHTML = emptyTermHTML();
  } else if (!hasActive && !hasHost) {
    wrap.innerHTML = emptyTermHTML();
  }
}

function emptyTermHTML(): string {
  return `
    <div class="term-empty">
      <div class="term-empty-card">
        <div class="term-empty-mark" aria-hidden="true">a</div>
        <h2>Pick a session</h2>
        <p>Open one from the rail, or start fresh. Closing a browser tab only detaches — the agent keeps running.</p>
        <div class="term-empty-actions">
          <button type="button" class="primary" data-action="new-session">New session</button>
          <button type="button" class="ghost" data-action="help">Shortcuts</button>
        </div>
      </div>
    </div>`;
}

function openPanel(p: Panel): void {
  state.panel = p;
  if (p === "new") state.createError = "";
  // Defer paint past the triggering click so a full-screen overlay is not
  // immediately hit by the same pointer event (closes as it opens).
  window.queueMicrotask(() => {
    if (state.panel !== p) return;
    try {
      paintPanel();
    } catch (e) {
      console.error("paintPanel failed", e);
      toast((e as Error).message || "failed to open panel", "err", 5000);
      return;
    }
    if (p === "new") {
      window.requestAnimationFrame(() => {
        const name = document.getElementById("sess-name") as HTMLInputElement | null;
        const agent = document.getElementById("sess-agent") as HTMLSelectElement | null;
        (name ?? agent)?.focus();
      });
    }
      if (p === "tools") {
      void refreshToolsStatus();
      void refreshSSHKeys();
    }
  });
}

function closePanel(): void {
  state.panel = null;
  state.createError = "";
  try {
    paintPanel();
  } catch (e) {
    console.error("closePanel paint failed", e);
    document.getElementById("panel-overlay")?.remove();
  }
  term?.focus();
}

function readCreateForm(): {
  agent: string;
  cwd: string;
  name: string;
  prompt: string;
  gitUrl: string;
  gitName: string;
  gitBranch: string;
  gitFork: boolean;
  gitDepth: boolean;
} {
  const agentEl = document.getElementById("sess-agent") as HTMLSelectElement | null;
  const cwdEl = document.getElementById("sess-cwd") as HTMLSelectElement | null;
  const nameEl = document.getElementById("sess-name") as HTMLInputElement | null;
  const promptEl = document.getElementById("sess-prompt") as HTMLTextAreaElement | null;
  const gitUrlEl = document.getElementById("sess-git-url") as HTMLInputElement | null;
  const gitNameEl = document.getElementById("sess-git-name") as HTMLInputElement | null;
  const gitBranchEl = document.getElementById("sess-git-branch") as HTMLInputElement | null;
  const gitForkEl = document.getElementById("sess-git-fork") as HTMLInputElement | null;
  const gitDepthEl = document.getElementById("sess-git-depth") as HTMLInputElement | null;
  const agent = (agentEl?.value || state.formAgent || state.agents[0] || "claude").trim();
  const cwd = (cwdEl?.value || state.formCwd || state.defaultCwd || ".").trim();
  const name = (nameEl?.value ?? state.formName).trim();
  const prompt = (promptEl?.value ?? state.formPrompt).trim();
  const gitUrl = (gitUrlEl?.value ?? state.formGitUrl).trim();
  const gitName = (gitNameEl?.value ?? state.formGitName).trim();
  const gitBranch = (gitBranchEl?.value ?? state.formGitBranch).trim();
  const gitFork = gitForkEl ? gitForkEl.checked : state.formGitFork;
  const gitDepth = gitDepthEl ? gitDepthEl.checked : state.formGitDepth;
  state.formAgent = agent;
  state.formCwd = cwd;
  state.formName = name;
  state.formPrompt = prompt;
  state.formGitUrl = gitUrl;
  state.formGitName = gitName;
  state.formGitBranch = gitBranch;
  state.formGitFork = gitFork;
  state.formGitDepth = gitDepth;
  return { agent, cwd, name, prompt, gitUrl, gitName, gitBranch, gitFork, gitDepth };
}

async function onCreateSession(ev?: Event): Promise<void> {
  ev?.preventDefault();
  if (state.creating) {
    toast("Already starting a session…", "info", 1500);
    return;
  }
  const form = readCreateForm();
  if (!form.agent) {
    state.createError = "Pick an agent";
    paintPanel();
    return;
  }
  state.creating = true;
  state.createError = "";
  paintPanel();
  try {
    let cwd = form.cwd || ".";
    if (form.gitUrl) {
      toast(form.gitFork ? "Forking & cloning…" : "Cloning project…", "info", 60000);
      const cloned = await cloneWorkspace({
        url: form.gitUrl,
        name: form.gitName || undefined,
        branch: form.gitBranch || undefined,
        fork: form.gitFork || undefined,
        depth: form.gitDepth ? 1 : undefined,
      });
      cwd = cloned.cwd;
      state.formCwd = cwd;
      await refreshWorkspaceList();
      toast(`Cloned → ${cwd}`, "ok", 2500);
    }
    const sess = await createSession({
      agent: form.agent,
      cwd,
      name: form.name || undefined,
      prompt: form.prompt || undefined,
    });
    state.formName = "";
    state.formPrompt = "";
    state.formGitUrl = "";
    state.formGitName = "";
    state.formGitBranch = "";
    state.formGitFork = false;
    state.creating = false;
    closePanel();
    await refreshSessions();
    openTab(sess);
    toast(`Started ${sess.agent} in ${cwd}`, "ok");
  } catch (e) {
    state.creating = false;
    const msg = (e as Error).message || "create failed";
    state.createError = msg;
    paintPanel();
    toast(msg, "err", 6000);
  }
}

function removeSessionLocally(id: string): void {
  state.sessions = state.sessions.filter((s) => s.id !== id);
  closeTab(id);
  // closeTab only switches tabs; ensure list chrome updates
  paintChrome();
  ensureTermArea();
}

async function onKillSession(id: string): Promise<void> {
  const s = state.sessions.find((x) => x.id === id);
  const label = s ? tabTitle(s) : shortId(id);
  if (!confirm(`Stop “${label}”?\n\nKills the agent process. Session stays in the list (you can resume or delete).`))
    return;
  try {
    await killSession(id);
    closeTab(id);
    await refreshSessions();
    toast("Agent stopped", "ok");
  } catch (e) {
    toast((e as Error).message || "kill failed", "err");
  }
}

async function onDeleteSession(id: string): Promise<void> {
  const s = state.sessions.find((x) => x.id === id);
  const label = s ? tabTitle(s) : shortId(id);
  const running = s?.state === "running";
  const msg = running
    ? `Delete “${label}”?\n\nStops the agent and removes it from the list. Cannot undo.`
    : `Delete “${label}”?\n\nRemoves this session from the list. Cannot undo.`;
  if (!confirm(msg)) return;
  try {
    await deleteSession(id);
    removeSessionLocally(id);
    await refreshSessions();
    toast("Session deleted", "ok");
  } catch (e) {
    toast((e as Error).message || "delete failed", "err");
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
  if (!confirm("Delete all stopped sessions from the list?\n\nRunning agents are left alone.")) return;
  try {
    const out = await pruneSessions();
    const keep = new Set(
      state.sessions.filter((s) => s.state === "running").map((s) => s.id),
    );
    state.openTabs = state.openTabs.filter((t) => keep.has(t.id));
    if (state.activeId && !state.openTabs.find((t) => t.id === state.activeId)) {
      detachPty();
      disposeTerminal();
      state.activeId = state.openTabs[0]?.id ?? null;
      attachedId = null;
    }
    await refreshSessions();
    ensureTermArea();
    if (state.activeId) void attachActive();
    else paintChrome();
    toast(`Deleted ${out.removed} stopped session(s)`, "ok");
  } catch (e) {
    toast((e as Error).message || "prune failed", "err");
  }
}

function toolsCwd(): string {
  const sel = document.getElementById("tools-cwd-select") as HTMLSelectElement | null;
  if (sel?.value) {
    state.formCwd = sel.value;
    return sel.value;
  }
  return state.formCwd || state.defaultCwd || ".";
}

async function refreshToolsStatus(): Promise<void> {
  const cwd = toolsCwd();
  try {
    const [ms, mem, pw] = await Promise.all([
      getMapStatus(cwd).catch((e: Error) => ({ status: { exists: false, reason: e.message } })),
      memoryStats(cwd).catch(() => ({ docs: 0, engine: "error" })),
      playwrightStatus().catch(() => null),
    ]);
    const st = (ms as { status?: { exists?: boolean; stale?: boolean; reason?: string } }).status ?? {};
    if (!st.exists) state.mapStatus = st.reason ? `error · ${st.reason}` : "no map";
    else if (st.stale) state.mapStatus = `stale · ${st.reason || "outdated"}`;
    else state.mapStatus = "fresh";
    const eng = (mem as { engine?: string }).engine || "fts";
    const docs = (mem as { docs?: number }).docs ?? 0;
    state.memStatus = `${docs} docs · ${eng}`;
    const m = mem as { vector_enabled?: boolean; docs_embedded?: number };
    if (m.vector_enabled) {
      state.memStatus += ` · emb ${m.docs_embedded ?? 0}`;
    }
    if (pw) {
      state.pwStatus = pw.running ? `running${pw.display ? ` · ${pw.display}` : ""}` : "stopped";
    } else {
      state.pwStatus = "unavailable";
    }
    paintTools();
  } catch (e) {
    state.mapStatus = "error";
    state.memStatus = (e as Error).message || "failed";
    paintTools();
  }
}

async function onMapGenerate(): Promise<void> {
  try {
    toast("Generating map…", "info", 6000);
    const out = await generateMap(toolsCwd());
    toast(`Map written · ${out.map_path || out.cwd}`, "ok");
    await refreshToolsStatus();
  } catch (e) {
    toast((e as Error).message || "map generate failed", "err");
  }
}

async function onMapShow(): Promise<void> {
  try {
    const out = await getMap(toolsCwd());
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
    toast("Indexing…", "info", 12000);
    const out = await memoryIndex({
      cwd: toolsCwd(),
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
  const input = document.getElementById("mem-query") as HTMLInputElement | null;
  const q = (input?.value ?? state.memQuery).trim();
  state.memQuery = q;
  if (!q) {
    toast("Enter a search query", "info");
    return;
  }
  try {
    toast("Searching…", "info", 4000);
    const out = await memorySearch({
      cwd: toolsCwd(),
      query: q,
      limit: 10,
      mode: "auto",
    });
    state.memHits = out.hits ?? [];
    paintTools();
    if (state.memHits.length === 0) toast("No memory hits", "info");
    else toast(`${state.memHits.length} hit(s)`, "ok", 1500);
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
  if (state.panel !== "tools") return;
  const mapEl = document.getElementById("map-status");
  if (mapEl) mapEl.textContent = state.mapStatus;
  const memEl = document.getElementById("mem-status");
  if (memEl) memEl.textContent = state.memStatus;
  const pwEl = document.getElementById("pw-status");
  if (pwEl) pwEl.textContent = state.pwStatus;
  const hits = document.getElementById("mem-hits");
  if (hits) hits.innerHTML = memHitsHTML();
  const sshDir = document.getElementById("ssh-dir");
  if (sshDir) sshDir.textContent = state.sshDir || "—";
  const sshList = document.getElementById("ssh-key-list");
  if (sshList) sshList.innerHTML = sshKeysHTML();
  const genBtn = document.querySelector(
    '[data-action="ssh-gen"]',
  ) as HTMLButtonElement | null;
  if (genBtn) {
    genBtn.disabled = state.sshBusy;
    genBtn.textContent = state.sshBusy ? "…" : "Generate";
  }
}

async function refreshSSHKeys(): Promise<void> {
  try {
    const out = await listSSHKeys();
    state.sshDir = out.dir || "";
    state.sshKeys = out.keys ?? [];
    paintTools();
  } catch (e) {
    state.sshDir = "error";
    state.sshKeys = [];
    paintTools();
    toast((e as Error).message || "ssh keys list failed", "err");
  }
}

function sshKeysHTML(): string {
  if (!state.sshKeys.length) {
    return `<div class="empty-soft">No keys in server ~/.ssh yet</div>`;
  }
  return state.sshKeys
    .map((k) => {
      const kind = k.has_private && k.has_public ? "pair" : k.has_public ? "pub" : "priv";
      const fp = k.fingerprint ? esc(k.fingerprint) : "";
      return `<div class="ssh-key-row">
        <div class="ssh-key-main">
          <div class="ssh-key-name"><code>${esc(k.name)}</code>
            <span class="badge">${esc(k.type || "?")}</span>
            <span class="badge">${esc(kind)}</span>
          </div>
          ${fp ? `<div class="ssh-key-fp">${fp}</div>` : ""}
          ${k.comment ? `<div class="ssh-key-comment">${esc(k.comment)}</div>` : ""}
        </div>
        <div class="ssh-key-actions">
          <button type="button" class="ghost btn-sm" data-action="ssh-copy" data-name="${esc(k.name)}" ${k.public_key ? "" : "disabled"}>Copy pub</button>
          <button type="button" class="ghost btn-sm danger-text" data-action="ssh-delete" data-name="${esc(k.name)}" ${k.protected ? "disabled" : ""}>Delete</button>
        </div>
        ${k.public_key ? `<pre class="ssh-key-pub" data-pub="${esc(k.name)}">${esc(k.public_key)}</pre>` : ""}
      </div>`;
    })
    .join("");
}

async function onSSHGenerate(): Promise<void> {
  if (state.sshBusy) return;
  const nameEl = document.getElementById("ssh-gen-name") as HTMLInputElement | null;
  const commentEl = document.getElementById("ssh-gen-comment") as HTMLInputElement | null;
  const name = (nameEl?.value || state.sshGenName || "").trim();
  const comment = (commentEl?.value || state.sshGenComment || "").trim();
  if (!name) {
    toast("Key name required", "err");
    return;
  }
  state.sshBusy = true;
  state.sshGenName = name;
  state.sshGenComment = comment;
  paintTools();
  try {
    const k = await generateSSHKey({
      name,
      type: "ed25519",
      comment: comment || undefined,
    });
    toast(`Generated ${k.name}`, "ok");
    if (k.public_key) {
      try {
        await navigator.clipboard.writeText(k.public_key);
        toast("Public key copied", "ok", 2000);
      } catch {
        /* ignore */
      }
    }
    state.sshGenName = "id_agents";
    await refreshSSHKeys();
  } catch (e) {
    toast((e as Error).message || "generate failed", "err");
  } finally {
    state.sshBusy = false;
    paintTools();
  }
}

async function onSSHCopy(name: string): Promise<void> {
  const k = state.sshKeys.find((x) => x.name === name);
  const pub = k?.public_key;
  if (!pub) {
    toast("No public key", "err");
    return;
  }
  try {
    await navigator.clipboard.writeText(pub);
    toast("Public key copied", "ok", 1500);
  } catch {
    toast(pub, "info", 8000);
  }
}

async function onSSHDelete(name: string): Promise<void> {
  if (!confirm(`Delete SSH key “${name}” from the server?\n\nRemoves private + public files. Cannot undo.`)) return;
  try {
    await deleteSSHKey(name);
    toast(`Deleted ${name}`, "ok");
    await refreshSSHKeys();
  } catch (e) {
    toast((e as Error).message || "delete failed", "err");
  }
}

function paintDrawer(): void {
  let el = document.getElementById("drawer") as HTMLDivElement | null;
  if (!state.drawer) {
    el?.remove();
    return;
  }
  if (!el) {
    el = document.createElement("div");
    el.id = "drawer";
    el.className = "overlay";
    el.style.zIndex = "1000";
    document.body.appendChild(el);
    el.addEventListener("mousedown", (ev) => {
      if (ev.target === el) {
        state.drawer = null;
        paintDrawer();
      }
    });
  }
  el.hidden = false;
  el.style.display = "grid";
  el.innerHTML = `
    <div class="modal modal-wide" role="dialog" aria-modal="true" data-modal>
      <div class="modal-head">
        <strong>${esc(state.drawer.title)}</strong>
        <button type="button" class="ghost btn-sm" data-action="close-drawer">Close</button>
      </div>
      <pre class="drawer-body">${esc(state.drawer.body)}</pre>
    </div>`;
}

function paintPanel(): void {
  let el = document.getElementById("panel-overlay") as HTMLDivElement | null;
  if (!state.panel) {
    el?.remove();
    return;
  }
  if (!el) {
    el = document.createElement("div");
    el.id = "panel-overlay";
    el.className = "overlay";
    el.setAttribute("role", "presentation");
    // Backdrop close only (not when clicking inside .modal)
    el.addEventListener("mousedown", (ev) => {
      if (ev.target === el) {
        ev.preventDefault();
        closePanel();
      }
    });
    document.body.appendChild(el);
  }
  // Force visible each open (defensive against stale CSS/DOM)
  el.hidden = false;
  el.style.display = "grid";
  el.style.visibility = "visible";
  el.style.opacity = "1";
  el.style.zIndex = "1000";

  let html = "";
  if (state.panel === "new") html = newSessionHTML();
  else if (state.panel === "tools") html = toolsHTML();
  else if (state.panel === "help") html = helpHTML();
  else html = `<div class="modal"><div class="modal-body">Unknown panel</div></div>`;
  el.innerHTML = html;
}

function agentOptionsHTML(): string {
  const agents =
    state.agents.length > 0 ? state.agents : ["claude", "grok", "codex", "opencode", "cursor"];
  const selected = agents.includes(state.formAgent) ? state.formAgent : agents[0];
  return agents
    .map(
      (a) =>
        `<option value="${esc(a)}" ${a === selected ? "selected" : ""} data-agent="${esc(agentClass(a))}">${esc(a)}</option>`,
    )
    .join("");
}

function agentSwatchHTML(name: string): string {
  return `<span class="agent-swatch ${agentClass(name)}" title="${esc(name)}" aria-hidden="true"></span>`;
}

function workspaceOptionsHTML(): string {
  // Always coerce — API may send {path} objects if an older client path is hit.
  let list = (state.workspaces ?? [])
    .map((w) => (typeof w === "string" ? w : String((w as { path?: string })?.path ?? "")))
    .filter(Boolean);
  if (list.length === 0) list = [state.defaultCwd || "."];
  const selected = list.includes(state.formCwd) ? state.formCwd : list[0];
  return list
    .map(
      (w) =>
        `<option value="${esc(w)}" ${w === selected ? "selected" : ""}>${esc(w)}</option>`,
    )
    .join("");
}

function newSessionHTML(): string {
  return `
    <div class="modal" role="dialog" aria-modal="true" aria-labelledby="new-title" data-modal>
      <div class="modal-head">
        <div>
          <div class="eyebrow">Launch</div>
          <h2 id="new-title">New session</h2>
        </div>
        <button type="button" class="ghost btn-sm" data-action="close-panel" title="Close (Esc)">✕</button>
      </div>
      <form id="create-form" class="modal-body" data-action-form="create-session">
        <div class="field-grid">
          <div class="field">
            <label for="sess-agent">Agent</label>
            <div class="agent-select-row ${agentClass(state.formAgent)}" id="sess-agent-row">
              ${agentSwatchHTML(state.formAgent)}
              <select id="sess-agent" name="agent" required ${state.creating ? "disabled" : ""} data-action-change="sess-agent">
                ${agentOptionsHTML()}
              </select>
            </div>
          </div>
          <div class="field">
            <label for="sess-cwd">Workspace</label>
            <select id="sess-cwd" name="cwd" required ${state.creating ? "disabled" : ""}>
              ${workspaceOptionsHTML()}
            </select>
          </div>
        </div>
        <div class="field">
          <label for="sess-name">Session label <span class="opt">optional</span></label>
          <input id="sess-name" name="name" value="${esc(state.formName)}" placeholder="e.g. fix-auth" autocomplete="off" ${state.creating ? "disabled" : ""} />
        </div>
        <div class="field">
          <label for="sess-prompt">Seed prompt <span class="opt">optional</span></label>
          <textarea id="sess-prompt" name="prompt" rows="2" placeholder="Typed into the TTY after start" ${state.creating ? "disabled" : ""}>${esc(state.formPrompt)}</textarea>
        </div>

        <details class="clone-block" ${state.formGitUrl ? "open" : ""}>
          <summary>New project from git</summary>
          <p class="form-hint" style="margin-top:0.5rem">Clone (or GitHub-fork) into the workspace, then start the agent there. Leave empty to use the workspace above.</p>
          <div class="field">
            <label for="sess-git-url">Repo URL or <code>owner/repo</code></label>
            <input id="sess-git-url" value="${esc(state.formGitUrl)}" placeholder="https://github.com/org/app.git  or  org/app" autocomplete="off" ${state.creating ? "disabled" : ""} />
          </div>
          <div class="field-grid">
            <div class="field">
              <label for="sess-git-name">Folder name <span class="opt">optional</span></label>
              <input id="sess-git-name" value="${esc(state.formGitName)}" placeholder="defaults to repo name" autocomplete="off" ${state.creating ? "disabled" : ""} />
            </div>
            <div class="field">
              <label for="sess-git-branch">Branch <span class="opt">optional</span></label>
              <input id="sess-git-branch" value="${esc(state.formGitBranch)}" placeholder="default branch" autocomplete="off" ${state.creating ? "disabled" : ""} />
            </div>
          </div>
          <div class="check-row">
            <label class="check">
              <input type="checkbox" id="sess-git-fork" ${state.formGitFork ? "checked" : ""} ${state.creating ? "disabled" : ""} />
              Fork on GitHub first <span class="opt">(requires <code>gh</code> auth)</span>
            </label>
            <label class="check">
              <input type="checkbox" id="sess-git-depth" ${state.formGitDepth ? "checked" : ""} ${state.creating ? "disabled" : ""} />
              Shallow clone
            </label>
          </div>
        </details>

        <p class="form-hint">Runs in server-side tmux. Closing a browser tab detaches only — Stop/Delete from the rail.</p>
        ${state.createError ? `<p class="form-error" role="alert">${esc(state.createError)}</p>` : ""}
        <div class="modal-actions">
          <button type="button" class="ghost" data-action="close-panel" ${state.creating ? "disabled" : ""}>Cancel</button>
          <button class="primary" type="submit" ${state.creating ? "disabled" : ""}>
            ${state.creating ? (state.formGitUrl ? "Cloning & starting…" : "Starting…") : state.formGitUrl ? "Clone & start" : "Start session"}
          </button>
        </div>
      </form>
    </div>`;
}

function toolsHTML(): string {
  return `
    <div class="modal modal-tools" role="dialog" aria-modal="true" aria-labelledby="tools-title" data-modal>
      <div class="modal-head">
        <div>
          <div class="eyebrow">Workspace</div>
          <h2 id="tools-title">Map · memory · browser</h2>
        </div>
        <button type="button" class="ghost btn-sm" data-action="close-panel" title="Close (Esc)">✕</button>
      </div>
      <div class="modal-body tools-body">
        <div class="field">
          <label for="tools-cwd-select">Target workspace</label>
          <select id="tools-cwd-select" data-action-change="tools-cwd">
            ${workspaceOptionsHTML()}
          </select>
        </div>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>Project map</h3>
            <span class="tool-status" id="map-status">${esc(state.mapStatus)}</span>
          </div>
          <p class="tool-desc">Orientation file at <code>.agents/PROJECT_MAP.md</code></p>
          <div class="btn-row">
            <button type="button" data-action="map-gen">Generate</button>
            <button type="button" data-action="map-show">Show</button>
          </div>
        </section>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>Memory</h3>
            <span class="tool-status" id="mem-status">${esc(state.memStatus)}</span>
          </div>
          <p class="tool-desc">Full-text (and optional vector) index for retrieval</p>
          <div class="btn-row">
            <button type="button" data-action="mem-index">Reindex</button>
          </div>
          <form class="mem-search" data-action-form="mem-search">
            <input id="mem-query" name="q" placeholder="Search docs…" value="${esc(state.memQuery)}" autocomplete="off" />
            <button type="submit" class="primary">Search</button>
          </form>
          <div id="mem-hits" class="mem-hits">${memHitsHTML()}</div>
        </section>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>Playwright</h3>
            <span class="tool-status" id="pw-status">${esc(state.pwStatus)}</span>
          </div>
          <p class="tool-desc">Headed browser stack for agents</p>
          <div class="btn-row">
            <button type="button" data-action="pw-start">Start</button>
            <button type="button" data-action="pw-stop">Stop</button>
            <button type="button" data-action="pw-restart">Restart</button>
          </div>
        </section>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>SSH keys</h3>
            <span class="tool-status" id="ssh-dir" title="Server SSH directory">${esc(state.sshDir || "—")}</span>
          </div>
          <p class="tool-desc">Identities on the agents host (public keys only — private keys never leave the server)</p>
          <div class="ssh-gen">
            <input id="ssh-gen-name" placeholder="name e.g. id_github" value="${esc(state.sshGenName)}" autocomplete="off" />
            <input id="ssh-gen-comment" placeholder="comment (optional)" value="${esc(state.sshGenComment)}" autocomplete="off" />
            <button type="button" class="primary" data-action="ssh-gen" ${state.sshBusy ? "disabled" : ""}>${state.sshBusy ? "…" : "Generate"}</button>
          </div>
          <div id="ssh-key-list" class="ssh-key-list">${sshKeysHTML()}</div>
        </section>
      </div>
    </div>`;
}

function helpHTML(): string {
  return `
    <div class="modal modal-sm" role="dialog" aria-modal="true" aria-labelledby="help-title" data-modal>
      <div class="modal-head">
        <div>
          <div class="eyebrow">Keyboard</div>
          <h2 id="help-title">Shortcuts</h2>
        </div>
        <button type="button" class="ghost btn-sm" data-action="close-panel" title="Close (Esc)">✕</button>
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
        <span class="logo-mark" aria-hidden="true">a</span>
        <div>
          <h1>agents</h1>
          <p class="sub">Remote agent desk</p>
        </div>
      </div>
      <p class="login-lede">Same bearer token as <code>agentsctl</code> — paste <code>AGENTSD_TOKEN</code> to connect.</p>
      <form id="login-form">
        <div class="field">
          <label for="token">Token</label>
          <input id="token" name="token" type="password" autocomplete="current-password" placeholder="Paste token" required autofocus />
        </div>
        <button class="primary" type="submit" style="width:100%">Connect</button>
        ${state.loginError ? `<p class="error">${esc(state.loginError)}</p>` : ""}
        ${state.busy ? `<p class="login-busy">Connecting…</p>` : ""}
      </form>
      <p class="hint">Stored in this browser only. Treat it like shell access to every agent tool on the host.</p>
    </div>
  </div>`;
}

function shellHTML(): string {
  return `
  <div class="shell${state.sidebarOpen ? " sidebar-open" : ""}">
    <div class="sidebar-backdrop" id="sidebar-backdrop" data-action="close-sidebar" hidden></div>
    <aside class="sidebar" id="sidebar">
      <div class="sidebar-header">
        <div class="brand">
          <span class="logo-mark sm" aria-hidden="true">a</span>
          <span class="brand-name">agents</span>
        </div>
        <button type="button" class="ghost btn-icon" data-action="logout" title="Log out">⎋</button>
      </div>

      <div class="sidebar-actions">
        <button type="button" class="primary sidebar-new" data-action="new-session" id="btn-new">
          <span>New session</span>
          <kbd>n</kbd>
        </button>
      </div>

      <div class="sidebar-filter">
        <input id="filter" type="search" placeholder="Filter…" value="${esc(state.filter)}" autocomplete="off" />
        <span class="sess-count" id="sess-count" title="running / total">0/0</span>
      </div>

      <div class="session-list" id="session-list">
        ${sessionListHTML()}
      </div>

      <div class="sidebar-foot">
        <button type="button" class="ghost btn-sm" data-action="prune" title="Delete all stopped sessions">Clear stopped</button>
        <button type="button" class="ghost btn-sm" data-action="tools">Tools</button>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <button type="button" class="ghost btn-icon mobile-only" data-action="toggle-sidebar" title="Sessions (w)" aria-label="Toggle sessions">☰</button>
        <div class="tabs" id="tabs">${tabsHTML()}</div>
        <div class="topbar-actions">
          <span class="conn-pill conn-${state.conn}" id="conn-pill"><span class="conn-dot"></span>idle</span>
          <button type="button" class="primary btn-sm" data-action="new-session" title="New session (n)">New</button>
          <button type="button" class="ghost btn-sm" data-action="tools" title="Tools (t)">Tools</button>
          <button type="button" class="ghost btn-sm" data-action="help" title="Shortcuts (?)">?</button>
        </div>
      </header>
      <div class="term-wrap">
        <div class="term-empty">
          <div class="term-empty-card">
            <div class="term-empty-mark" aria-hidden="true">a</div>
            <h2>Pick a session</h2>
            <p>Open one from the rail, or start fresh. Detach anytime — agents keep running in tmux.</p>
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
      <button type="button" class="primary btn-sm" data-action="new-session">New session</button>
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
      const ag = agentClass(s.agent);
      return `
      <div class="session-item ${active} ${ag}" data-open="${esc(s.id)}">
        <div class="session-item-main">
          <div class="session-top">
            <span class="session-name">${esc(s.name || shortId(s.id))}</span>
            <span class="badge ${esc(s.state)}">${esc(s.state)}</span>
          </div>
          <div class="session-meta">
            <span class="agent-chip ${ag}">${agentSwatchHTML(s.agent)}${esc(s.agent)}</span>
            <span class="cwd-chip" title="${esc(s.cwd)}">${esc(basename(s.cwd))}</span>
            ${age ? `<span class="age">${esc(age)}</span>` : ""}
          </div>
        </div>
        <div class="session-actions">
          ${
            s.state === "running"
              ? `<button type="button" class="ghost btn-sm act" data-kill="${esc(s.id)}" title="Stop agent">Stop</button>`
              : `<button type="button" class="ghost btn-sm act resume-text" data-resume="${esc(s.id)}" title="Resume agent">Resume</button>`
          }
          <button type="button" class="ghost btn-sm act danger-text" data-delete="${esc(s.id)}" title="Delete session">Delete</button>
        </div>
      </div>`;
    })
    .join("");
}

function tabsHTML(): string {
  if (state.openTabs.length === 0) {
    return `<div class="tabs-empty">No tabs open</div>`;
  }
  return state.openTabs
    .map((t, i) => {
      const active = t.id === state.activeId ? "active" : "";
      const live = t.state === "running" ? "live" : "dead";
      const ag = agentClass(t.agent);
      return `
      <div class="tab ${active} ${live} ${ag}" data-tab="${esc(t.id)}" title="${esc(t.agent)} · ${esc(t.id)}">
        <span class="tab-dot" title="${esc(t.agent)}"></span>
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
  const run = state.sessions.filter((s) => s.state === "running").length;
  return `
    <span class="status-dot ${cls}"></span>
    <span>${esc(state.statusText)}</span>
    <span class="sep">·</span>
    <span>${run} live</span>
    <span class="sep">·</span>
    <span>${open} open</span>
    <span class="sep">·</span>
    <span class="status-hint">tab close detaches only</span>
  `;
}

function esc(s: unknown): string {
  const str = s == null ? "" : String(s);
  return str
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

/** Document-level delegation — survives innerHTML rewrites of lists/tabs/modals. */
function ensureUIDelegation(): void {
  if (uiDelegated) return;
  uiDelegated = true;

  document.addEventListener("click", (ev) => {
    const t = ev.target as HTMLElement | null;
    if (!t) return;
    const actionEl = t.closest<HTMLElement>("[data-action]");
    if (!actionEl) return;
    // don't steal clicks from disabled controls
    if (actionEl.hasAttribute("disabled") || (actionEl as HTMLButtonElement).disabled) return;
    const action = actionEl.getAttribute("data-action");
    if (!action) return;

    switch (action) {
      case "new-session":
        ev.preventDefault();
        ev.stopPropagation();
        if (!state.token) return;
        openPanel("new");
        break;
      case "tools":
        ev.preventDefault();
        ev.stopPropagation();
        if (!state.token) return;
        openPanel("tools");
        break;
      case "help":
        ev.preventDefault();
        ev.stopPropagation();
        if (!state.token) return;
        openPanel("help");
        break;
      case "close-panel":
        ev.preventDefault();
        ev.stopPropagation();
        closePanel();
        break;
      case "close-drawer":
        ev.preventDefault();
        ev.stopPropagation();
        state.drawer = null;
        paintDrawer();
        break;
      case "prune":
        ev.preventDefault();
        void onPrune();
        break;
      case "logout":
        ev.preventDefault();
        logout();
        break;
      case "toggle-sidebar":
        ev.preventDefault();
        state.sidebarOpen = !state.sidebarOpen;
        paintChrome();
        break;
      case "close-sidebar":
        ev.preventDefault();
        state.sidebarOpen = false;
        paintChrome();
        break;
      case "map-gen":
        ev.preventDefault();
        void onMapGenerate();
        break;
      case "map-show":
        ev.preventDefault();
        void onMapShow();
        break;
      case "mem-index":
        ev.preventDefault();
        void onMemIndex();
        break;
      case "pw-start":
        ev.preventDefault();
        void onPwAction("start");
        break;
      case "pw-stop":
        ev.preventDefault();
        void onPwAction("stop");
        break;
      case "pw-restart":
        ev.preventDefault();
        void onPwAction("restart");
        break;
      case "ssh-gen":
        ev.preventDefault();
        void onSSHGenerate();
        break;
      case "ssh-copy": {
        ev.preventDefault();
        const n = actionEl.getAttribute("data-name");
        if (n) void onSSHCopy(n);
        break;
      }
      case "ssh-delete": {
        ev.preventDefault();
        const n = actionEl.getAttribute("data-name");
        if (n) void onSSHDelete(n);
        break;
      }
      default:
        break;
    }
  });

  document.addEventListener("submit", (ev) => {
    const form = (ev.target as HTMLElement | null)?.closest?.("[data-action-form]") as
      | HTMLFormElement
      | null;
    if (!form) return;
    const kind = form.getAttribute("data-action-form");
    if (kind === "create-session") {
      ev.preventDefault();
      void onCreateSession(ev);
    } else if (kind === "mem-search") {
      ev.preventDefault();
      void onMemSearch(ev);
    }
  });

  document.addEventListener("change", (ev) => {
    const el = ev.target as HTMLElement | null;
    if (!el) return;
    const kind = el.getAttribute("data-action-change");
    if (kind === "tools-cwd" && el instanceof HTMLSelectElement) {
      state.formCwd = el.value;
      void refreshToolsStatus();
    }
    if (kind === "sess-agent" && el instanceof HTMLSelectElement) {
      state.formAgent = el.value;
      const row = document.getElementById("sess-agent-row");
      if (row) {
        row.className = `agent-select-row ${agentClass(el.value)}`;
        const sw = row.querySelector(".agent-swatch");
        if (sw) sw.className = `agent-swatch ${agentClass(el.value)}`;
      }
    }
  });
}

function bindShell(): void {
  if (shellBound) return;
  shellBound = true;
  ensureUIDelegation();
  bindKeys();

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
  document.querySelectorAll<HTMLElement>("[data-open]").forEach((el) => {
    el.onclick = (ev) => {
      const t = ev.target as HTMLElement;
      if (t.closest("[data-kill], [data-delete], [data-resume], [data-copy]")) return;
      const id = el.getAttribute("data-open");
      if (!id) return;
      const s = state.sessions.find((x) => x.id === id);
      if (!s) return;
      if (s.state !== "running") {
        toast("Not running — Resume or Delete", "info", 2200);
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

  document.querySelectorAll<HTMLElement>("[data-delete]").forEach((el) => {
    el.onclick = (ev) => {
      ev.stopPropagation();
      const id = el.getAttribute("data-delete");
      if (id) void onDeleteSession(id);
    };
  });

  document.querySelectorAll<HTMLElement>("[data-resume]").forEach((el) => {
    el.onclick = (ev) => {
      ev.stopPropagation();
      const id = el.getAttribute("data-resume");
      if (id) void onResumeSession(id);
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
