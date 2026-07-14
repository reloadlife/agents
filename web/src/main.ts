import "./styles.css";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";

import {
  clearToken,
  cloneWorkspace,
  consumeAuthFromURL,
  contextEnsure,
  contextPack,
  contextStatus,
  createSession,
  deleteSSHKey,
  deleteSession,
  generateMap,
  generateSSHKey,
  getMap,
  getMapStatus,
  getStatus,
  getToken,
  ghAccounts,
  ghLogin,
  ghLogout,
  ghSetupGit,
  ghSwitch,
  killSession,
  listAgentAccounts,
  listAgents,
  listSessions,
  listSSHKeys,
  listWorkspaces,
  saveAgentAccount,
  switchAgentAccount,
  formatPlaywrightStatus,
  memoryIndex,
  memorySearch,
  memoryStats,
  playwrightInstall,
  playwrightRestart,
  playwrightStart,
  playwrightStatus,
  playwrightStop,
  pruneSessions,
  resumeSession,
  setToken,
  type AgentAccount,
  type AgentPlatformStatus,
  type GHAccount,
  type GHStatus,
  type MemoryHit,
  type Session,
  type SSHKey,
} from "./api";
import {
  animateLoginIn,
  animateModalIn,
  animateModalOut,
  animateSessionList,
  animateSettingsIn,
  animateSettingsOut,
  animateShellIn,
  animateToastIn,
  animateToastOut,
  bindPressMotion,
} from "./motion";
import { PtyClient } from "./pty";

type OpenTab = {
  id: string;
  title: string;
  agent: string;
  cwd: string;
  state: string;
};

type Panel = null | "new" | "tools" | "help";
type SettingsTab = "accounts" | "github" | "ssh" | "workspace" | "about";
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
  formAccount: string;
  formAccountMode: string; // isolated | global
  agentAccounts: AgentAccount[];
  agentAccountPlatform: string;
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
  ctxStatus: string;
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
  ghStatus: GHStatus | null;
  ghLoginToken: string;
  ghBusy: boolean;
  settingsOpen: boolean;
  settingsTab: SettingsTab;
  settingsPlatform: string;
  settingsPlatforms: AgentPlatformStatus[];
  settingsAcctId: string;
  settingsAcctLabel: string;
  settingsBusy: boolean;
  /** keyboard highlight in session list (j/k) */
  listCursorId: string | null;
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
  formAccount: "",
  formAccountMode: "isolated",
  agentAccounts: [],
  agentAccountPlatform: "",
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
  ctxStatus: "—",
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
  ghStatus: null,
  ghLoginToken: "",
  ghBusy: false,
  settingsOpen: false,
  settingsTab: "accounts",
  settingsPlatform: "grok",
  settingsPlatforms: [],
  settingsAcctId: "personal",
  settingsAcctLabel: "",
  settingsBusy: false,
  listCursorId: null,
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
let shellEntranceDone = false;
let lastSessionListSig = "";
let pressMotionBound = false;

const app = document.getElementById("app")!;

function toast(msg: string, kind: ToastKind = "info", ms = 3200): void {
  state.toast = { msg, kind };
  void paintToast();
  if (toastTimer) window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => {
    state.toast = null;
    void paintToast();
  }, ms);
}

/** Toasts live on document.body above modals (z-index), not inside term-wrap. */
async function paintToast(): Promise<void> {
  let el = document.getElementById("app-toast");
  if (!el) {
    el = document.createElement("div");
    el.id = "app-toast";
    el.className = "app-toast";
    el.hidden = true;
    document.body.appendChild(el);
  }
  if (!state.toast) {
    if (!el.hidden) {
      await animateToastOut(el);
    }
    el.hidden = true;
    el.textContent = "";
    el.className = "app-toast";
    return;
  }
  el.hidden = false;
  el.textContent = state.toast.msg;
  el.className = `app-toast toast-${state.toast.kind}`;
  animateToastIn(el);
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
  state.settingsOpen = false;
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
      background: "#09090b",
      foreground: "#fafafa",
      cursor: "#fafafa",
      cursorAccent: "#09090b",
      selectionBackground: "#27272a",
      black: "#09090b",
      red: "#f87171",
      green: "#4ade80",
      yellow: "#fbbf24",
      blue: "#60a5fa",
      magenta: "#c084fc",
      cyan: "#22d3ee",
      white: "#e4e4e7",
      brightBlack: "#71717a",
      brightRed: "#fca5a5",
      brightGreen: "#86efac",
      brightYellow: "#fde68a",
      brightBlue: "#93c5fd",
      brightMagenta: "#d8b4fe",
      brightCyan: "#67e8f9",
      brightWhite: "#fafafa",
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
  // Tools/settings share status widgets — don't stack overlays.
  if (p === "tools" && state.settingsOpen) {
    state.settingsOpen = false;
    document.getElementById("settings-root")?.remove();
  }
  // Prefer active session workspace when opening tools
  if (p === "tools") {
    const tab = state.openTabs.find((t) => t.id === state.activeId);
    if (tab?.cwd && state.workspaces.includes(tab.cwd)) {
      state.formCwd = tab.cwd;
    }
  }
  // Defer paint past the triggering click so a full-screen overlay is not
  // immediately hit by the same pointer event (closes as it opens).
  window.queueMicrotask(async () => {
    if (state.panel !== p) return;
    if (p === "new") {
      await refreshAgentAccountsForForm(state.formAgent);
    }
    try {
      await paintPanel({ animateIn: true });
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
    }
  });
}

async function closePanel(): Promise<void> {
  const el = document.getElementById("panel-overlay") as HTMLDivElement | null;
  if (el) {
    try {
      await animateModalOut(el);
    } catch {
      /* ignore */
    }
  }
  state.panel = null;
  state.createError = "";
  try {
    await paintPanel({ animateIn: false });
  } catch (e) {
    console.error("closePanel paint failed", e);
    document.getElementById("panel-overlay")?.remove();
  }
  term?.focus();
}

function openSettings(tab?: SettingsTab): void {
  if (tab) state.settingsTab = tab;
  state.settingsOpen = true;
  state.sidebarOpen = false;
  // Avoid stacked overlays fighting for the same tool controls.
  if (state.panel) {
    state.panel = null;
    document.getElementById("panel-overlay")?.remove();
  }
  void loadSettingsData().then(() => paintSettings({ animateIn: true }));
}

async function closeSettings(): Promise<void> {
  const root = document.getElementById("settings-root");
  if (root) {
    try {
      await animateSettingsOut(root);
    } catch {
      /* ignore */
    }
  }
  state.settingsOpen = false;
  document.getElementById("settings-root")?.remove();
  term?.focus();
}

async function loadSettingsData(): Promise<void> {
  const tasks: Promise<void>[] = [];
  if (state.settingsTab === "accounts") {
    tasks.push(
      (async () => {
        try {
          const out = (await listAgentAccounts()) as {
            platforms?: AgentPlatformStatus[];
          };
          state.settingsPlatforms = out.platforms || [];
          if (
            !state.settingsPlatforms.find((p) => p.platform === state.settingsPlatform)
          ) {
            state.settingsPlatform = state.settingsPlatforms[0]?.platform || "grok";
          }
        } catch (e) {
          state.settingsPlatforms = [];
          toast((e as Error).message || "Failed to load accounts", "err");
        }
      })(),
    );
  }
  if (state.settingsTab === "github") {
    tasks.push(refreshGHAccounts());
  }
  if (state.settingsTab === "ssh") {
    tasks.push(refreshSSHKeys());
  }
  if (state.settingsTab === "workspace") {
    tasks.push(refreshToolsStatus());
  }
  await Promise.all(tasks);
}

function paintSettings(opts?: { animateIn?: boolean }): void {
  if (!state.settingsOpen || !state.shellMounted) {
    document.getElementById("settings-root")?.remove();
    return;
  }
  let root = document.getElementById("settings-root");
  const wasMissing = !root;
  if (!root) {
    root = document.createElement("div");
    root.id = "settings-root";
    root.className = "settings-root";
    document.body.appendChild(root);
  }
  root.innerHTML = settingsHTML();
  if (opts?.animateIn || wasMissing) {
    void animateSettingsIn(root);
  }
}

function settingsHTML(): string {
  const tabs: { id: SettingsTab; label: string; hint: string }[] = [
    { id: "accounts", label: "Agent accounts", hint: "Cursor · Claude · Grok · Codex" },
    { id: "github", label: "GitHub", hint: "gh CLI logins" },
    { id: "ssh", label: "SSH keys", hint: "Host identities" },
    { id: "workspace", label: "Workspace", hint: "Map · memory · browser" },
    { id: "about", label: "About", hint: "Host & shortcuts" },
  ];
  const nav = tabs
    .map(
      (t) => `
      <button type="button" class="settings-nav-item ${state.settingsTab === t.id ? "active" : ""}" data-action="settings-tab" data-tab="${t.id}">
        <span class="settings-nav-label">${esc(t.label)}</span>
        <span class="settings-nav-hint">${esc(t.hint)}</span>
      </button>`,
    )
    .join("");

  let body = "";
  switch (state.settingsTab) {
    case "accounts":
      body = settingsAccountsHTML();
      break;
    case "github":
      body = settingsGitHubHTML();
      break;
    case "ssh":
      body = settingsSSHHTML();
      break;
    case "workspace":
      body = settingsWorkspaceHTML();
      break;
    case "about":
      body = settingsAboutHTML();
      break;
  }

  return `
    <div class="settings-shell" role="dialog" aria-modal="true" aria-labelledby="settings-title" data-sidebar="08">
      <aside class="settings-nav">
        <div class="settings-nav-head">
          <div class="eyebrow">Configuration</div>
          <h1 id="settings-title">Settings</h1>
        </div>
        <nav class="settings-nav-list">${nav}</nav>
        <button type="button" class="ghost settings-nav-close" data-action="close-settings">← Back to desk</button>
      </aside>
      <section class="settings-main">
        <header class="settings-main-head">
          <div class="topbar-lead" style="gap:0.55rem">
            <span class="eyebrow" style="margin:0">Host</span>
            <span class="topbar-sep" aria-hidden="true"></span>
            <h2 style="margin:0">${esc(tabs.find((t) => t.id === state.settingsTab)?.label || "Settings")}</h2>
          </div>
          <button type="button" class="ghost btn-icon sm" data-action="close-settings" title="Close (Esc)" aria-label="Close">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
          </button>
        </header>
        <div class="settings-body">${body}</div>
      </section>
    </div>`;
}

function settingsAccountsHTML(): string {
  const plats = state.settingsPlatforms;
  if (!plats.length) {
    return `
      <div class="settings-empty">
        <p><strong>cursor-switch</strong> is required for multi-account profiles.</p>
        <p class="form-hint">Install from <code>github.com/reloadlife/cursor-account-switcher</code> on this host, then refresh.</p>
        <button type="button" class="primary" data-action="settings-refresh">Retry</button>
      </div>`;
  }
  const platTabs = plats
    .map(
      (p) =>
        `<button type="button" class="chip ${state.settingsPlatform === p.platform ? "active" : ""}" data-action="settings-platform" data-platform="${esc(p.platform)}">${esc(p.platform)}</button>`,
    )
    .join("");
  const cur =
    plats.find((p) => p.platform === state.settingsPlatform) || plats[0];
  const rows = (cur?.accounts || [])
    .map((a) => {
      const saved = a.saved
        ? `<span class="badge running">saved</span>`
        : `<span class="badge">empty</span>`;
      const active = a.active ? `<span class="badge running">active</span>` : "";
      return `
        <div class="settings-card">
          <div class="settings-card-main">
            <div class="settings-card-title">
              <strong>${esc(a.label || a.id)}</strong>
              <code>${esc(a.id)}</code>
              ${saved}${active}
            </div>
            <div class="settings-card-meta">
              ${a.email ? esc(a.email) : "No profile saved yet"}
              ${a.saved_at ? ` · ${esc(a.saved_at)}` : ""}
            </div>
          </div>
          <div class="settings-card-actions">
            <button type="button" class="ghost btn-sm" data-action="acct-save" data-platform="${esc(cur.platform)}" data-id="${esc(a.id)}" data-label="${esc(a.label || a.id)}" title="Save current live login as this profile">Save current</button>
            <button type="button" class="ghost btn-sm" data-action="acct-switch" data-platform="${esc(cur.platform)}" data-id="${esc(a.id)}" ${a.saved ? "" : "disabled"} title="Switch host-wide login">Global switch</button>
          </div>
        </div>`;
    })
    .join("");

  return `
    <p class="settings-lede">
      Profiles from <code>cursor-switch</code>. <strong>Isolated</strong> sessions (default in New session) run accounts in parallel via private HOME.
      <strong>Global switch</strong> changes the host-wide login for that tool.
    </p>
    <div class="chip-row">${platTabs}</div>
    <div class="settings-stat">
      <span>Live: <strong>${esc(cur?.current || "—")}</strong></span>
      <span>Active profile: <strong>${esc(cur?.active || "—")}</strong></span>
    </div>
    <div class="settings-cards">${rows || `<div class="settings-empty">No accounts for ${esc(cur?.platform || "")}</div>`}</div>
    <div class="settings-form-card">
      <h3>Add account slot</h3>
      <div class="field-grid">
        <div class="field">
          <label for="settings-acct-id">Id</label>
          <input id="settings-acct-id" value="${esc(state.settingsAcctId)}" placeholder="personal" />
        </div>
        <div class="field">
          <label for="settings-acct-label">Label</label>
          <input id="settings-acct-label" value="${esc(state.settingsAcctLabel)}" placeholder="Personal" />
        </div>
      </div>
      <div class="modal-actions" style="justify-content:flex-start">
        <button type="button" class="primary" data-action="acct-add" ${state.settingsBusy ? "disabled" : ""}>Add</button>
        <button type="button" class="ghost" data-action="settings-refresh">Refresh</button>
      </div>
      <p class="form-hint">After adding, log into that CLI on the host, then <strong>Save current</strong>.</p>
    </div>`;
}

function settingsGitHubHTML(): string {
  return `
    <p class="settings-lede">GitHub CLI accounts on this host. Tokens are write-only — never shown back.</p>
    <div class="settings-stat">
      <span>Active: <strong id="gh-active">${esc(state.ghStatus?.active || "—")}</strong></span>
    </div>
    <div id="gh-account-list" class="gh-account-list settings-stack">${ghAccountsHTML()}</div>
    <div class="settings-form-card">
      <h3>Login with token</h3>
      <div class="gh-login">
        <input id="gh-login-token" type="password" placeholder="Paste PAT / fine-grained token" value="" autocomplete="off" />
        <button type="button" class="primary" data-action="gh-login" ${state.ghBusy ? "disabled" : ""}>${state.ghBusy ? "…" : "Login"}</button>
        <button type="button" class="ghost" data-action="gh-setup-git">Setup git</button>
      </div>
    </div>`;
}

function settingsSSHHTML(): string {
  return `
    <p class="settings-lede">SSH identities under the agents host home. Public keys only — private keys never leave the server.</p>
    <div class="settings-stat">
      <span>Dir: <code id="ssh-dir">${esc(state.sshDir || "—")}</code></span>
    </div>
    <div class="settings-form-card">
      <h3>Generate key</h3>
      <div class="ssh-gen">
        <input id="ssh-gen-name" placeholder="name e.g. id_github" value="${esc(state.sshGenName)}" autocomplete="off" />
        <input id="ssh-gen-comment" placeholder="comment (optional)" value="${esc(state.sshGenComment)}" autocomplete="off" />
        <button type="button" class="primary" data-action="ssh-gen" ${state.sshBusy ? "disabled" : ""}>${state.sshBusy ? "…" : "Generate"}</button>
      </div>
    </div>
    <div id="ssh-key-list" class="ssh-key-list settings-stack">${sshKeysHTML()}</div>`;
}

function settingsWorkspaceHTML(): string {
  return `
    <p class="settings-lede">Context manager packs map + docs + memory so agents orient faster. Session start auto-ensures by default.</p>
    <div class="field">
      <label for="tools-cwd-select">Target workspace</label>
      <select id="tools-cwd-select" data-action-change="tools-cwd">
        ${workspaceOptionsHTML()}
      </select>
    </div>
    <div class="settings-cards">
      <div class="settings-card col">
        <div class="settings-card-title"><strong>Context</strong> <span class="tool-status" data-status="ctx">${esc(state.ctxStatus)}</span></div>
        <p class="tool-desc">Ensure map · memory · <code>.agents/CONTEXT.md</code> · instructions</p>
        <div class="btn-row">
          <button type="button" class="primary btn-sm" data-action="ctx-ensure">Ensure</button>
          <button type="button" class="ghost btn-sm" data-action="ctx-pack">Show pack</button>
        </div>
      </div>
      <div class="settings-card col">
        <div class="settings-card-title"><strong>Project map</strong> <span class="tool-status" data-status="map">${esc(state.mapStatus)}</span></div>
        <p class="tool-desc">Orientation file at <code>.agents/PROJECT_MAP.md</code></p>
        <div class="btn-row">
          <button type="button" class="ghost btn-sm" data-action="map-gen">Generate</button>
          <button type="button" class="ghost btn-sm" data-action="map-show">Show</button>
        </div>
      </div>
      <div class="settings-card col">
        <div class="settings-card-title"><strong>Memory</strong> <span class="tool-status" data-status="mem">${esc(state.memStatus)}</span></div>
        <p class="tool-desc">FTS (and optional vector) index</p>
        <div class="btn-row">
          <button type="button" class="ghost btn-sm" data-action="mem-index">Reindex</button>
        </div>
        <form class="mem-search" data-action-form="mem-search">
          <input id="mem-query" name="q" data-mem-query placeholder="Search docs…" value="${esc(state.memQuery)}" autocomplete="off" />
          <button type="submit" class="primary btn-sm">Search</button>
        </form>
        <div class="mem-hits" data-mem-hits>${memHitsHTML()}</div>
      </div>
      <div class="settings-card col">
        <div class="settings-card-title"><strong>Playwright</strong> <span class="tool-status" data-status="pw">${esc(state.pwStatus)}</span></div>
        <p class="tool-desc">Xvfb + docker browser stack</p>
        <div class="btn-row">
          <button type="button" class="ghost btn-sm" data-action="pw-start">Start</button>
          <button type="button" class="ghost btn-sm" data-action="pw-stop">Stop</button>
          <button type="button" class="ghost btn-sm" data-action="pw-restart">Restart</button>
          <button type="button" class="ghost btn-sm" data-action="pw-install">Install browsers</button>
        </div>
      </div>
    </div>`;
}

function settingsAboutHTML(): string {
  return `
    <div class="settings-about">
      <div class="settings-stat">
        <span>Status: <strong>${esc(state.statusText)}</strong></span>
        <span>API: <strong>${state.statusOk ? "ok" : "error"}</strong></span>
      </div>
      <h3>Keyboard</h3>
      <p class="form-hint" style="margin-bottom:0.75rem">Bare keys when focus is outside the terminal. <kbd>Alt</kbd>+key works while the terminal is focused. Press <kbd>?</kbd> for the full list.</p>
      <dl class="keys">
        <div><dt><kbd>n</kbd> / <kbd>Alt</kbd><kbd>n</kbd></dt><dd>New session</dd></div>
        <div><dt><kbd>,</kbd> / <kbd>Alt</kbd><kbd>,</kbd></dt><dd>Settings</dd></div>
        <div><dt><kbd>j</kbd><kbd>k</kbd> · <kbd>Enter</kbd></dt><dd>Browse / open sessions</dd></div>
        <div><dt><kbd>[</kbd><kbd>]</kbd> · <kbd>1</kbd>–<kbd>9</kbd></dt><dd>Cycle / jump tabs</dd></div>
        <div><dt><kbd>x</kbd></dt><dd>Close tab (detach only)</dd></div>
        <div><dt><kbd>s</kbd> · <kbd>e</kbd> · <kbd>Shift</kbd><kbd>d</kbd></dt><dd>Stop · Resume · Delete</dd></div>
        <div><dt><kbd>Esc</kbd></dt><dd>Close overlay / settings</dd></div>
      </dl>
      <h3>Multi-account</h3>
      <p class="form-hint">Save CLI logins with <code>cursor-switch --platform grok save personal</code>, then pick the profile when starting a session (isolated = parallel).</p>
      <h3>Security</h3>
      <p class="form-hint">Bearer token is full host control. SSH private keys and GitHub tokens are never returned by the API.</p>
    </div>`;
}

async function onAcctSave(platform: string, id: string, label: string): Promise<void> {
  try {
    toast(`Saving ${platform}/${id}…`, "info", 8000);
    await saveAgentAccount({ platform, id, label: label || undefined });
    toast(`Saved ${id}`, "ok");
    await loadSettingsData();
    paintSettings();
  } catch (e) {
    toast((e as Error).message || "save failed", "err");
  }
}

async function onAcctSwitch(platform: string, id: string): Promise<void> {
  if (!confirm(`Switch host-wide ${platform} login to “${id}”?\n\nThis changes the global auth for that tool.`))
    return;
  try {
    await switchAgentAccount({ platform, id });
    toast(`Switched ${platform} → ${id}`, "ok");
    await loadSettingsData();
    paintSettings();
  } catch (e) {
    toast((e as Error).message || "switch failed", "err");
  }
}

async function onAcctAdd(): Promise<void> {
  const idEl = document.getElementById("settings-acct-id") as HTMLInputElement | null;
  const labelEl = document.getElementById("settings-acct-label") as HTMLInputElement | null;
  const id = (idEl?.value || "").trim().toLowerCase();
  const label = (labelEl?.value || "").trim() || id;
  if (!id) {
    toast("Account id required", "err");
    return;
  }
  state.settingsBusy = true;
  paintSettings();
  try {
    await requestJSON("/v1/agent-accounts/add", {
      platform: state.settingsPlatform,
      id,
      label,
    });
    toast(`Added ${id}`, "ok");
    state.settingsAcctId = "personal";
    state.settingsAcctLabel = "";
    await loadSettingsData();
    paintSettings();
  } catch (e) {
    toast((e as Error).message || "add failed", "err");
  } finally {
    state.settingsBusy = false;
    paintSettings();
  }
}

async function requestJSON(path: string, body: Record<string, unknown>): Promise<void> {
  const token = getToken();
  const res = await fetch(path, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    let msg = `${res.status}`;
    try {
      const j = (await res.json()) as { error?: string };
      if (j.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
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
  account: string;
  accountMode: string;
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
  const accountEl = document.getElementById("sess-account") as HTMLSelectElement | null;
  const modeEl = document.querySelector(
    'input[name="sess-account-mode"]:checked',
  ) as HTMLInputElement | null;
  const agent = (agentEl?.value || state.formAgent || state.agents[0] || "claude").trim();
  const cwd = (cwdEl?.value || state.formCwd || state.defaultCwd || ".").trim();
  const name = (nameEl?.value ?? state.formName).trim();
  const prompt = (promptEl?.value ?? state.formPrompt).trim();
  const gitUrl = (gitUrlEl?.value ?? state.formGitUrl).trim();
  const gitName = (gitNameEl?.value ?? state.formGitName).trim();
  const gitBranch = (gitBranchEl?.value ?? state.formGitBranch).trim();
  const gitFork = gitForkEl ? gitForkEl.checked : state.formGitFork;
  const gitDepth = gitDepthEl ? gitDepthEl.checked : state.formGitDepth;
  const account = (accountEl?.value ?? state.formAccount).trim();
  const accountMode = (modeEl?.value || state.formAccountMode || "isolated").trim();
  state.formAgent = agent;
  state.formCwd = cwd;
  state.formName = name;
  state.formPrompt = prompt;
  state.formGitUrl = gitUrl;
  state.formGitName = gitName;
  state.formGitBranch = gitBranch;
  state.formGitFork = gitFork;
  state.formGitDepth = gitDepth;
  state.formAccount = account;
  state.formAccountMode = accountMode;
  return {
    agent,
    cwd,
    name,
    prompt,
    gitUrl,
    gitName,
    gitBranch,
    gitFork,
    gitDepth,
    account,
    accountMode,
  };
}

function agentPlatformFor(agent: string): string {
  const a = agent.toLowerCase();
  if (a.includes("cursor")) return "cursor";
  if (a.includes("claude")) return "claude";
  if (a.includes("codex") || a === "gpt") return "codex";
  if (a.includes("grok")) return "grok";
  if (a.includes("copilot")) return "vscode";
  return "";
}

async function refreshAgentAccountsForForm(agent?: string): Promise<void> {
  const plat = agentPlatformFor(agent || state.formAgent);
  state.agentAccountPlatform = plat;
  if (!plat) {
    state.agentAccounts = [];
    return;
  }
  try {
    const out = (await listAgentAccounts(plat)) as AgentPlatformStatus;
    state.agentAccounts = out.accounts || [];
  } catch {
    state.agentAccounts = [];
  }
}

function accountOptionsHTML(): string {
  const opts = [`<option value="">(default / none)</option>`];
  for (const a of state.agentAccounts) {
    const label = a.saved
      ? `${a.label || a.id}${a.email ? " · " + a.email : ""}`
      : `${a.label || a.id} (not saved)`;
    const sel = a.id === state.formAccount ? "selected" : "";
    opts.push(
      `<option value="${esc(a.id)}" ${sel} ${a.saved ? "" : "disabled"}>${esc(label)}</option>`,
    );
  }
  return opts.join("");
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
      account: form.account || undefined,
      account_mode: form.account ? form.accountMode || "isolated" : undefined,
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
    const accNote = form.account
      ? ` · account ${form.account} (${form.accountMode || "isolated"})`
      : "";
    toast(`Started ${sess.agent} in ${cwd}${accNote}`, "ok");
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
  // Prefer the visible tools/settings select (there may be two in rare stacks).
  const sels = document.querySelectorAll<HTMLSelectElement>(
    "select[data-action-change='tools-cwd'], #tools-cwd-select",
  );
  for (const sel of sels) {
    if (sel.value && sel.offsetParent !== null) {
      state.formCwd = sel.value;
      return sel.value;
    }
  }
  for (const sel of sels) {
    if (sel.value) {
      state.formCwd = sel.value;
      return sel.value;
    }
  }
  // Fall back to active session cwd, then form default.
  const tab = state.openTabs.find((t) => t.id === state.activeId);
  if (tab?.cwd) return tab.cwd;
  return state.formCwd || state.defaultCwd || ".";
}

function setStatusText(key: string, text: string): void {
  document
    .querySelectorAll<HTMLElement>(`[data-status="${key}"]`)
    .forEach((el) => {
      el.textContent = text;
    });
}

async function refreshToolsStatus(): Promise<void> {
  const cwd = toolsCwd();
  try {
    const [ms, mem, pw, ctx] = await Promise.all([
      getMapStatus(cwd).catch((e: Error) => ({
        status: { exists: false, reason: e.message },
      })),
      memoryStats(cwd).catch(() => ({ docs: 0, engine: "error" })),
      playwrightStatus().catch(() => null),
      contextStatus(cwd).catch(() => null),
    ]);
    const st =
      (ms as { status?: { exists?: boolean; stale?: boolean; reason?: string } })
        .status ?? {};
    if (!st.exists) {
      state.mapStatus = st.reason ? `missing · ${st.reason}` : "no map — generate";
    } else if (st.stale) {
      state.mapStatus = `stale · ${st.reason || "outdated"}`;
    } else {
      state.mapStatus = "fresh";
    }
    const eng = (mem as { engine?: string }).engine || "fts";
    const docs = (mem as { docs?: number }).docs ?? 0;
    state.memStatus = `${docs} docs · ${eng}`;
    const m = mem as { vector_enabled?: boolean; docs_embedded?: number };
    if (m.vector_enabled) {
      state.memStatus += ` · emb ${m.docs_embedded ?? 0}`;
    }
    if (ctx) {
      const bits: string[] = [];
      bits.push(ctx.ready ? "ready" : "needs ensure");
      if (ctx.has_context) bits.push("CONTEXT.md");
      bits.push(`${ctx.memory_docs ?? docs} mem`);
      state.ctxStatus = bits.join(" · ");
    } else {
      state.ctxStatus = "unavailable";
    }
    state.pwStatus = formatPlaywrightStatus(pw);
    paintToolsStatus();
  } catch (e) {
    state.mapStatus = "error";
    state.memStatus = (e as Error).message || "failed";
    state.ctxStatus = "error";
    state.pwStatus = "error";
    paintToolsStatus();
  }
}

/** Patch status labels + mem hits without wiping the open modal. */
function paintToolsStatus(): void {
  setStatusText("ctx", state.ctxStatus);
  setStatusText("map", state.mapStatus);
  setStatusText("mem", state.memStatus);
  setStatusText("pw", state.pwStatus);
  document.querySelectorAll<HTMLElement>("[data-mem-hits]").forEach((el) => {
    el.innerHTML = memHitsHTML();
  });
  // Legacy ids (older embeds / partial markup)
  const legacy: Array<[string, string]> = [
    ["ctx-status", state.ctxStatus],
    ["map-status", state.mapStatus],
    ["mem-status", state.memStatus],
    ["pw-status", state.pwStatus],
  ];
  for (const [id, text] of legacy) {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
  }
  const hits = document.getElementById("mem-hits");
  if (hits) hits.innerHTML = memHitsHTML();
}

async function onCtxEnsure(): Promise<void> {
  try {
    toast("Ensuring context…", "info", 12000);
    const out = await contextEnsure({
      cwd: toolsCwd(),
      force_map: false,
      force_index: false,
    });
    const bits = [
      out.map_generated ? "map refreshed" : "map ok",
      out.memory_indexed ? `+${out.memory_indexed} mem` : "mem ok",
      out.context_wrote ? "CONTEXT.md" : "",
    ].filter(Boolean);
    toast(`Context ready · ${bits.join(" · ")}`, "ok");
    await refreshToolsStatus();
  } catch (e) {
    toast((e as Error).message || "context ensure failed", "err");
  }
}

async function onCtxPack(): Promise<void> {
  try {
    toast("Building pack…", "info", 8000);
    const out = await contextPack({
      cwd: toolsCwd(),
      write_file: true,
      query: state.memQuery || undefined,
    });
    if (!out.markdown) {
      toast("Empty pack", "info");
      return;
    }
    state.drawer = {
      title: `Context pack · ${out.cwd || toolsCwd()}`,
      body: out.markdown,
    };
    paintDrawer();
    toast(`Pack ${out.chars ?? 0} chars${out.wrote_file ? " · wrote CONTEXT.md" : ""}`, "ok");
    await refreshToolsStatus();
  } catch (e) {
    toast((e as Error).message || "pack failed", "err");
  }
}

async function onMapGenerate(): Promise<void> {
  const cwd = toolsCwd();
  try {
    state.mapStatus = "generating…";
    paintToolsStatus();
    toast(`Generating map for ${cwd}…`, "info", 8000);
    const out = await generateMap(cwd);
    toast(`Map written · ${out.map_path || out.cwd || cwd}`, "ok");
    await refreshToolsStatus();
  } catch (e) {
    state.mapStatus = "error";
    paintToolsStatus();
    toast((e as Error).message || "map generate failed", "err");
  }
}

async function onMapShow(): Promise<void> {
  const cwd = toolsCwd();
  try {
    toast(`Loading map · ${cwd}…`, "info", 4000);
    const out = await getMap(cwd);
    if (out.error || !out.markdown) {
      toast(out.error || "No map — click Generate first", "err", 4000);
      return;
    }
    state.drawer = {
      title: `Project map · ${out.cwd || cwd}`,
      body: out.markdown,
    };
    paintDrawer();
  } catch (e) {
    toast((e as Error).message || "map show failed", "err");
  }
}

async function onMemIndex(): Promise<void> {
  const cwd = toolsCwd();
  try {
    state.memStatus = "indexing…";
    paintToolsStatus();
    toast(`Indexing memory · ${cwd}…`, "info", 15000);
    // clear:false keeps durable notes; still regenerate map for fresh FTS entry
    const out = await memoryIndex({
      cwd,
      clear: false,
      generate_map: true,
    });
    toast(`Indexed ${out.indexed ?? 0} docs (total ${out.docs_total ?? "?"})`, "ok");
    await refreshToolsStatus();
  } catch (e) {
    state.memStatus = "error";
    paintToolsStatus();
    toast((e as Error).message || "index failed", "err");
  }
}

async function onMemSearch(ev?: Event): Promise<void> {
  ev?.preventDefault();
  const inputs = document.querySelectorAll<HTMLInputElement>(
    "#mem-query, input[name='q'][data-mem-query], [data-mem-query]",
  );
  let q = state.memQuery;
  for (const input of inputs) {
    if (input.value.trim()) {
      q = input.value.trim();
      break;
    }
  }
  // Also check the focused input / form
  const active = document.activeElement;
  if (active instanceof HTMLInputElement && active.value.trim()) {
    q = active.value.trim();
  }
  const single = document.getElementById("mem-query") as HTMLInputElement | null;
  if (single?.value.trim()) q = single.value.trim();
  state.memQuery = q;
  if (!q) {
    toast("Enter a search query", "info");
    return;
  }
  const cwd = toolsCwd();
  try {
    toast(`Searching “${q}” in ${cwd}…`, "info", 5000);
    const out = await memorySearch({
      cwd,
      query: q,
      limit: 12,
      mode: "auto",
    });
    state.memHits = out.hits ?? [];
    paintToolsStatus();
    if (state.memHits.length === 0) {
      toast("No memory hits — try Reindex, or a broader query", "info", 4000);
    } else {
      toast(`${state.memHits.length} hit(s)`, "ok", 1500);
    }
  } catch (e) {
    toast((e as Error).message || "search failed", "err");
  }
}

async function onPwAction(action: "start" | "stop" | "restart" | "install"): Promise<void> {
  try {
    state.pwStatus = `${action}…`;
    paintToolsStatus();
    toast(`Playwright ${action}…`, "info", 20000);
    if (action === "install") {
      const out = await playwrightInstall();
      if (out.ok === false) {
        toast(out.error || "install failed", "err", 8000);
      } else {
        toast("Browsers install finished", "ok");
      }
    } else {
      const fn =
        action === "start"
          ? playwrightStart
          : action === "stop"
            ? playwrightStop
            : playwrightRestart;
      const out = await fn();
      if (out.ok === false) {
        toast(out.error || `${action} failed`, "err", 8000);
        if (out.status) state.pwStatus = formatPlaywrightStatus(out.status);
        paintToolsStatus();
      } else {
        toast(`Playwright ${action} ok`, "ok");
        if (out.status) state.pwStatus = formatPlaywrightStatus(out.status);
        paintToolsStatus();
      }
    }
    await refreshToolsStatus();
  } catch (e) {
    state.pwStatus = "error";
    paintToolsStatus();
    toast((e as Error).message || `${action} failed`, "err", 8000);
  }
}

/** @deprecated use paintToolsStatus — kept for SSH/GH settings patches */
function paintTools(): void {
  paintToolsStatus();
  if (state.settingsOpen && (state.settingsTab === "ssh" || state.settingsTab === "github" || state.settingsTab === "workspace")) {
    // re-render only lists that live in settings cards
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
    const ghList = document.getElementById("gh-account-list");
    if (ghList) ghList.innerHTML = ghAccountsHTML();
    const ghActive = document.getElementById("gh-active");
    if (ghActive) ghActive.textContent = state.ghStatus?.active || "—";
    const ghLoginBtn = document.querySelector(
      '[data-action="gh-login"]',
    ) as HTMLButtonElement | null;
    if (ghLoginBtn) {
      ghLoginBtn.disabled = state.ghBusy;
      ghLoginBtn.textContent = state.ghBusy ? "…" : "Login";
    }
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

async function refreshGHAccounts(): Promise<void> {
  try {
    state.ghStatus = await ghAccounts();
    paintTools();
  } catch (e) {
    state.ghStatus = {
      ok: false,
      accounts: [],
      error: (e as Error).message || "gh status failed",
    };
    paintTools();
  }
}

function ghAccountsHTML(): string {
  const st = state.ghStatus;
  if (!st) return `<div class="empty-soft">Loading…</div>`;
  if (!st.accounts?.length) {
    return `<div class="empty-soft">${esc(st.error || "No GitHub accounts logged in on the server")}</div>`;
  }
  return st.accounts
    .map((a: GHAccount) => {
      const active = a.active ? "active" : "";
      const scopes = (a.scopes || []).join(", ");
      return `<div class="gh-row ${active}">
        <div class="gh-main">
          <div class="gh-user">
            <strong>${esc(a.user)}</strong>
            ${a.active ? `<span class="badge running">active</span>` : ""}
            <span class="gh-host">${esc(a.host)}</span>
          </div>
          <div class="gh-meta">proto ${esc(a.git_protocol || "—")}${scopes ? ` · ${esc(scopes)}` : ""}</div>
        </div>
        <div class="gh-actions">
          ${
            a.active
              ? ""
              : `<button type="button" class="ghost btn-sm" data-action="gh-switch" data-user="${esc(a.user)}" data-host="${esc(a.host)}">Switch</button>`
          }
          <button type="button" class="ghost btn-sm danger-text" data-action="gh-logout" data-user="${esc(a.user)}" data-host="${esc(a.host)}">Logout</button>
        </div>
      </div>`;
    })
    .join("");
}

async function onGHLogin(): Promise<void> {
  if (state.ghBusy) return;
  const tokEl = document.getElementById("gh-login-token") as HTMLInputElement | null;
  const token = (tokEl?.value || state.ghLoginToken || "").trim();
  if (!token) {
    toast("Paste a GitHub token", "err");
    return;
  }
  state.ghBusy = true;
  paintTools();
  try {
    state.ghStatus = await ghLogin({
      token,
      host: "github.com",
      git_protocol: "https",
      insecure_storage: true,
    });
    state.ghLoginToken = "";
    if (tokEl) tokEl.value = "";
    toast(`GitHub active: ${state.ghStatus.active || "ok"}`, "ok");
    paintTools();
  } catch (e) {
    toast((e as Error).message || "gh login failed", "err");
  } finally {
    state.ghBusy = false;
    paintTools();
  }
}

async function onGHSwitch(user: string, host: string): Promise<void> {
  try {
    state.ghStatus = await ghSwitch({ user, host: host || "github.com" });
    toast(`Active: ${state.ghStatus.active}`, "ok");
    paintTools();
  } catch (e) {
    toast((e as Error).message || "switch failed", "err");
  }
}

async function onGHLogout(user: string, host: string): Promise<void> {
  if (!confirm(`Log out GitHub account “${user}” on the server?\n\nOnly removes local gh config — does not revoke the token.`))
    return;
  try {
    state.ghStatus = await ghLogout({ user, host: host || "github.com" });
    toast(`Logged out ${user}`, "ok");
    paintTools();
  } catch (e) {
    toast((e as Error).message || "logout failed", "err");
  }
}

async function onGHSetupGit(): Promise<void> {
  try {
    await ghSetupGit();
    toast("git credential helper set via gh", "ok");
    await refreshGHAccounts();
  } catch (e) {
    toast((e as Error).message || "setup-git failed", "err");
  }
}

async function paintDrawer(): Promise<void> {
  let el = document.getElementById("drawer") as HTMLDivElement | null;
  if (!state.drawer) {
    if (el) {
      try {
        await animateModalOut(el);
      } catch {
        /* ignore */
      }
      el.remove();
    }
    return;
  }
  const wasMissing = !el;
  if (!el) {
    el = document.createElement("div");
    el.id = "drawer";
    el.className = "overlay drawer-overlay";
    document.body.appendChild(el);
    el.addEventListener("mousedown", (ev) => {
      if (ev.target === el) {
        state.drawer = null;
        void paintDrawer();
      }
    });
  }
  // Always re-append so drawer stacks above tools panel
  document.body.appendChild(el);
  el.hidden = false;
  el.style.display = "grid";
  el.style.zIndex = "1200";
  el.innerHTML = `
    <div class="modal modal-wide" role="dialog" aria-modal="true" data-modal>
      <div class="modal-head">
        <strong>${esc(state.drawer.title)}</strong>
        <button type="button" class="ghost btn-sm" data-action="close-drawer">Close</button>
      </div>
      <pre class="drawer-body">${esc(state.drawer.body)}</pre>
    </div>`;
  if (wasMissing) void animateModalIn(el);
}

async function paintPanel(opts?: { animateIn?: boolean }): Promise<void> {
  let el = document.getElementById("panel-overlay") as HTMLDivElement | null;
  if (!state.panel) {
    el?.remove();
    return;
  }
  const wasMissing = !el;
  if (!el) {
    el = document.createElement("div");
    el.id = "panel-overlay";
    el.className = "overlay";
    el.setAttribute("role", "presentation");
    // Backdrop close only (not when clicking inside .modal)
    el.addEventListener("mousedown", (ev) => {
      if (ev.target === el) {
        ev.preventDefault();
        void closePanel();
      }
    });
    document.body.appendChild(el);
  }
  // Force visible each open (defensive against stale CSS/DOM)
  el.hidden = false;
  el.style.display = "grid";
  el.style.visibility = "visible";
  el.style.zIndex = "1000";

  let html = "";
  if (state.panel === "new") html = newSessionHTML();
  else if (state.panel === "tools") html = toolsHTML();
  else if (state.panel === "help") html = helpHTML();
  else html = `<div class="modal" data-modal><div class="modal-body">Unknown panel</div></div>`;
  el.innerHTML = html;

  if (opts?.animateIn || wasMissing) {
    await animateModalIn(el);
  } else {
    el.style.opacity = "1";
  }
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
          <div class="eyebrow">Sessions</div>
          <h2 id="new-title">New session</h2>
        </div>
        <button type="button" class="ghost btn-icon sm" data-action="close-panel" title="Close (Esc)" aria-label="Close">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
        </button>
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
          <label for="sess-name">Label <span class="opt">optional</span></label>
          <input id="sess-name" name="name" value="${esc(state.formName)}" placeholder="e.g. fix-auth" autocomplete="off" ${state.creating ? "disabled" : ""} />
        </div>
        ${
          state.agentAccountPlatform
            ? `<div class="field">
          <label for="sess-account">Account <span class="opt">${esc(state.agentAccountPlatform)}</span></label>
          <select id="sess-account" ${state.creating ? "disabled" : ""}>
            ${accountOptionsHTML()}
          </select>
          <div class="check-row" style="margin-top:0.55rem">
            <label class="check"><input type="radio" name="sess-account-mode" value="isolated" ${state.formAccountMode !== "global" ? "checked" : ""} /> Isolated — parallel-safe private HOME</label>
            <label class="check"><input type="radio" name="sess-account-mode" value="global" ${state.formAccountMode === "global" ? "checked" : ""} /> Global switch — host-wide login</label>
          </div>
          <p class="form-hint">Manage profiles in Settings, or: <code>cursor-switch --platform ${esc(state.agentAccountPlatform)} save personal</code></p>
        </div>`
            : ""
        }
        <div class="field">
          <label for="sess-prompt">Seed prompt <span class="opt">optional</span></label>
          <textarea id="sess-prompt" name="prompt" rows="2" placeholder="Typed into the TTY after start" ${state.creating ? "disabled" : ""}>${esc(state.formPrompt)}</textarea>
        </div>

        <details class="clone-block" ${state.formGitUrl ? "open" : ""}>
          <summary>Clone project from git</summary>
          <p class="form-hint" style="margin-top:0.35rem;margin-bottom:0.75rem">Clone or GitHub-fork into the workspace, then start the agent there. Leave empty to use the workspace above.</p>
          <div class="field">
            <label for="sess-git-url">Repo URL or owner/repo</label>
            <input id="sess-git-url" value="${esc(state.formGitUrl)}" placeholder="https://github.com/org/app.git" autocomplete="off" ${state.creating ? "disabled" : ""} />
          </div>
          <div class="field-grid">
            <div class="field">
              <label for="sess-git-name">Folder <span class="opt">optional</span></label>
              <input id="sess-git-name" value="${esc(state.formGitName)}" placeholder="repo name" autocomplete="off" ${state.creating ? "disabled" : ""} />
            </div>
            <div class="field">
              <label for="sess-git-branch">Branch <span class="opt">optional</span></label>
              <input id="sess-git-branch" value="${esc(state.formGitBranch)}" placeholder="default" autocomplete="off" ${state.creating ? "disabled" : ""} />
            </div>
          </div>
          <div class="check-row">
            <label class="check">
              <input type="checkbox" id="sess-git-fork" ${state.formGitFork ? "checked" : ""} ${state.creating ? "disabled" : ""} />
              Fork on GitHub first <span class="opt">(needs gh auth)</span>
            </label>
            <label class="check">
              <input type="checkbox" id="sess-git-depth" ${state.formGitDepth ? "checked" : ""} ${state.creating ? "disabled" : ""} />
              Shallow clone
            </label>
          </div>
        </details>

        <p class="form-hint">Runs in server-side tmux. Closing a browser tab detaches only.</p>
        ${state.createError ? `<p class="form-error" role="alert">${esc(state.createError)}</p>` : ""}
        <div class="modal-actions">
          <button type="button" class="ghost" data-action="close-panel" ${state.creating ? "disabled" : ""}>Cancel</button>
          <button class="primary" type="submit" ${state.creating ? "disabled" : ""}>
            ${state.creating ? (state.formGitUrl ? "Cloning…" : "Starting…") : state.formGitUrl ? "Clone & start" : "Start session"}
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
          <h2 id="tools-title">Quick tools</h2>
        </div>
        <button type="button" class="ghost btn-icon sm" data-action="close-panel" title="Close (Esc)" aria-label="Close">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
        </button>
      </div>
      <div class="modal-body tools-body">
        <div class="field">
          <label for="tools-cwd-select">Target workspace</label>
          <select id="tools-cwd-select" data-action-change="tools-cwd">
            ${workspaceOptionsHTML()}
          </select>
          <p class="form-hint">Actions apply to this cwd under workspace_root.</p>
        </div>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>Context</h3>
            <span class="tool-status" data-status="ctx" id="ctx-status">${esc(state.ctxStatus)}</span>
          </div>
          <p class="tool-desc">Map + memory pack · auto-ensured on new session</p>
          <div class="btn-row">
            <button type="button" class="primary btn-sm" data-action="ctx-ensure">Ensure</button>
            <button type="button" class="ghost btn-sm" data-action="ctx-pack">Show pack</button>
          </div>
        </section>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>Project map</h3>
            <span class="tool-status" data-status="map" id="map-status">${esc(state.mapStatus)}</span>
          </div>
          <p class="tool-desc">Writes <code>.agents/PROJECT_MAP.md</code> for agent orientation</p>
          <div class="btn-row">
            <button type="button" class="ghost btn-sm" data-action="map-gen">Generate</button>
            <button type="button" class="ghost btn-sm" data-action="map-show">Show</button>
          </div>
        </section>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>Memory</h3>
            <span class="tool-status" data-status="mem" id="mem-status">${esc(state.memStatus)}</span>
          </div>
          <p class="tool-desc">FTS index of map + docs (agents: <code>agentsctl memory search</code>)</p>
          <div class="btn-row">
            <button type="button" class="ghost btn-sm" data-action="mem-index">Reindex</button>
          </div>
          <form class="mem-search" data-action-form="mem-search">
            <input id="mem-query" name="q" data-mem-query placeholder="Search docs…" value="${esc(state.memQuery)}" autocomplete="off" />
            <button type="submit" class="primary btn-sm">Search</button>
          </form>
          <div id="mem-hits" class="mem-hits" data-mem-hits>${memHitsHTML()}</div>
        </section>

        <section class="tool-block">
          <div class="tool-block-head">
            <h3>Playwright</h3>
            <span class="tool-status" data-status="pw" id="pw-status">${esc(state.pwStatus)}</span>
          </div>
          <p class="tool-desc">Xvfb + docker browser stack for headed agent browsers</p>
          <div class="btn-row">
            <button type="button" class="ghost btn-sm" data-action="pw-start">Start</button>
            <button type="button" class="ghost btn-sm" data-action="pw-stop">Stop</button>
            <button type="button" class="ghost btn-sm" data-action="pw-restart">Restart</button>
            <button type="button" class="ghost btn-sm" data-action="pw-install">Install browsers</button>
          </div>
        </section>

        <p class="form-hint" style="margin-top:0.5rem">
          Accounts, GitHub, and SSH keys live in
          <button type="button" class="linkish" data-action="open-settings" data-tab="accounts">Settings</button>.
        </p>
      </div>
    </div>`;
}

function helpHTML(): string {
  return `
    <div class="modal modal-wide" role="dialog" aria-modal="true" aria-labelledby="help-title" data-modal>
      <div class="modal-head">
        <div>
          <div class="eyebrow">Reference</div>
          <h2 id="help-title">Keyboard shortcuts</h2>
        </div>
        <button type="button" class="ghost btn-icon sm" data-action="close-panel" title="Close (Esc)" aria-label="Close">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
        </button>
      </div>
      <div class="modal-body help-body">
        <p class="form-hint help-lede">Bare keys work when focus is outside the terminal (sidebar, empty desk). Prefix with <kbd>Alt</kbd> to use the same action while the TTY is focused. Inputs still eat bare keys.</p>
        <div class="help-grid">
          <section class="help-section">
            <h3>Navigate</h3>
            <dl class="keys">
              <div><dt><kbd>w</kbd></dt><dd>Toggle session rail</dd></div>
              <div><dt><kbd>/</kbd></dt><dd>Focus filter</dd></div>
              <div><dt><kbd>j</kbd> / <kbd>k</kbd></dt><dd>Move list cursor</dd></div>
              <div><dt><kbd>Enter</kbd> / <kbd>o</kbd></dt><dd>Open cursor / active</dd></div>
              <div><dt><kbd>[</kbd> / <kbd>]</kbd></dt><dd>Prev / next tab</dd></div>
              <div><dt><kbd>1</kbd>–<kbd>9</kbd></dt><dd>Jump to tab</dd></div>
              <div><dt><kbd>0</kbd></dt><dd>Last open tab</dd></div>
              <div><dt><kbd>\`</kbd></dt><dd>Focus terminal</dd></div>
              <div><dt><kbd>Esc</kbd></dt><dd>Close overlay → rail</dd></div>
            </dl>
          </section>
          <section class="help-section">
            <h3>Sessions</h3>
            <dl class="keys">
              <div><dt><kbd>n</kbd></dt><dd>New session</dd></div>
              <div><dt><kbd>x</kbd></dt><dd>Close tab (detach only)</dd></div>
              <div><dt><kbd>Shift</kbd><kbd>x</kbd></dt><dd>Close all tabs</dd></div>
              <div><dt><kbd>s</kbd></dt><dd>Stop agent (confirm)</dd></div>
              <div><dt><kbd>e</kbd></dt><dd>Resume agent</dd></div>
              <div><dt><kbd>Shift</kbd><kbd>d</kbd></dt><dd>Delete session (confirm)</dd></div>
              <div><dt><kbd>c</kbd></dt><dd>Clear stopped (confirm)</dd></div>
              <div><dt><kbd>r</kbd></dt><dd>Refresh session list</dd></div>
            </dl>
          </section>
          <section class="help-section">
            <h3>Panels</h3>
            <dl class="keys">
              <div><dt><kbd>t</kbd></dt><dd>Quick tools</dd></div>
              <div><dt><kbd>,</kbd></dt><dd>Settings</dd></div>
              <div><dt><kbd>g</kbd></dt><dd>Settings → GitHub</dd></div>
              <div><dt><kbd>Shift</kbd><kbd>k</kbd></dt><dd>Settings → SSH keys</dd></div>
              <div><dt><kbd>a</kbd></dt><dd>Settings → accounts</dd></div>
              <div><dt><kbd>f</kbd></dt><dd>Fit terminal</dd></div>
              <div><dt><kbd>?</kbd></dt><dd>This help</dd></div>
            </dl>
          </section>
          <section class="help-section">
            <h3>Over the TTY</h3>
            <dl class="keys">
              <div><dt><kbd>Alt</kbd>+ letter</dt><dd>Same as bare key</dd></div>
              <div><dt><kbd>Alt</kbd><kbd>1</kbd>–<kbd>9</kbd></dt><dd>Jump tab</dd></div>
              <div><dt><kbd>Alt</kbd><kbd>[</kbd> <kbd>]</kbd></dt><dd>Cycle tabs</dd></div>
              <div><dt><kbd>Alt</kbd><kbd>Enter</kbd></dt><dd>Focus terminal</dd></div>
              <div><dt><kbd>Alt</kbd><kbd>Shift</kbd><kbd>1</kbd>–<kbd>5</kbd></dt><dd>Settings tabs</dd></div>
            </dl>
          </section>
        </div>
        <p class="form-hint">Closing a tab never kills the agent — use Stop or Delete for that.</p>
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
    shellEntranceDone = false;
    lastSessionListSig = "";
    app.innerHTML = loginHTML();
    bindLogin();
    animateLoginIn(app);
    return;
  }
  if (!state.shellMounted) {
    app.innerHTML = shellHTML();
    state.shellMounted = true;
    shellBound = false;
    bindShell();
    ensureTermArea();
    void paintPanel({ animateIn: false });
    void paintDrawer();
    if (!shellEntranceDone) {
      shellEntranceDone = true;
      const shell = document.querySelector<HTMLElement>(".shell");
      if (shell) animateShellIn(shell);
      // first session list cascade
      lastSessionListSig = "";
      maybeAnimateSessionList();
    }
    if (!pressMotionBound) {
      pressMotionBound = true;
      bindPressMotion(document);
    }
    if (state.activeId) void attachActive();
    return;
  }
  paintChrome();
}

function maybeAnimateSessionList(): void {
  const list = document.getElementById("session-list");
  if (!list) return;
  // Signature ignores state (running/exited) so 5s polls don't re-stagger the list
  const sig = `${state.filter}|${state.sessions.map((s) => s.id).join(",")}`;
  if (sig === lastSessionListSig) return;
  lastSessionListSig = sig;
  animateSessionList(list);
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
  const crumb = document.getElementById("breadcrumb");
  if (crumb) crumb.outerHTML = breadcrumbHTML();
  const host = document.getElementById("nav-host");
  if (host) {
    host.textContent = (state.statusText || "host").split("·")[0]?.trim() || "host";
  }
  const navStatus = document.getElementById("nav-status");
  if (navStatus) navStatus.textContent = state.statusText || "connected";
  const shell = document.querySelector(".shell");
  shell?.classList.toggle("sidebar-open", state.sidebarOpen);
  paintConn();
  void paintToast();
  bindSessionList();
  bindTabs();
  maybeAnimateSessionList();
}

function loginHTML(): string {
  return `
  <div class="login">
    <div class="login-card">
      <div class="login-brand">
        <span class="logo-mark" aria-hidden="true">a</span>
        <div>
          <h1>agents</h1>
          <p class="sub">Control plane</p>
        </div>
      </div>
      <p class="login-lede">Paste the same bearer token as <code>agentsctl</code> (<code>AGENTSD_TOKEN</code>), or run <code>agentsctl web</code> for auto-login.</p>
      <form id="login-form">
        <div class="field">
          <label for="token">API token</label>
          <input id="token" name="token" type="password" autocomplete="current-password" placeholder="Paste token…" required autofocus />
        </div>
        <button class="primary" type="submit" style="width:100%">Connect</button>
        ${state.loginError ? `<p class="error">${esc(state.loginError)}</p>` : ""}
        ${state.busy ? `<p class="login-busy">Connecting…</p>` : ""}
      </form>
      <p class="hint">Stored in this browser only. Full host control — treat it like root shell access.</p>
    </div>
  </div>`;
}

function iconSvg(name: string): string {
  const common =
    'width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="menu-ico" aria-hidden="true"';
  switch (name) {
    case "terminal":
      return `<svg ${common}><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>`;
    case "settings":
      return `<svg ${common}><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>`;
    case "wrench":
      return `<svg ${common}><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>`;
    case "help":
      return `<svg ${common}><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>`;
    case "panel":
      return `<svg ${common}><rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/></svg>`;
    case "logout":
      return `<svg ${common}><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>`;
    default:
      return "";
  }
}

function breadcrumbHTML(): string {
  const tab = state.openTabs.find((t) => t.id === state.activeId);
  if (!tab) {
    return `<nav class="breadcrumb" id="breadcrumb" aria-label="Breadcrumb">
      <span>Desk</span>
      <span class="sep-chev">/</span>
      <strong>Sessions</strong>
    </nav>`;
  }
  return `<nav class="breadcrumb" id="breadcrumb" aria-label="Breadcrumb">
    <span>Sessions</span>
    <span class="sep-chev">/</span>
    <strong title="${esc(tab.agent)} · ${esc(tab.cwd)}">${esc(tab.title)}</strong>
  </nav>`;
}

function shellHTML(): string {
  const host = (state.statusText || "host").split("·")[0]?.trim() || "host";
  return `
  <div class="shell${state.sidebarOpen ? " sidebar-open" : ""}" data-sidebar="08">
    <div class="sidebar-backdrop" id="sidebar-backdrop" data-action="close-sidebar" hidden></div>

    <aside class="sidebar" id="sidebar">
      <div class="sidebar-header">
        <div class="brand">
          <span class="logo-mark sm" aria-hidden="true">a</span>
          <div class="brand-meta">
            <span class="brand-name">agents</span>
            <span class="brand-sub">control plane</span>
          </div>
        </div>
      </div>

      <div class="sidebar-content">
        <div class="sidebar-group">
          <div class="sidebar-group-label">Platform</div>
          <div class="sidebar-actions">
            <button type="button" class="primary sidebar-new" data-action="new-session" id="btn-new">
              <span>+ New session</span>
              <kbd>n</kbd>
            </button>
          </div>
          <div class="sidebar-menu">
            <button type="button" class="sidebar-menu-btn" data-action="tools" title="Quick tools (t)">
              ${iconSvg("wrench")}<span>Tools</span><kbd>t</kbd>
            </button>
            <button type="button" class="sidebar-menu-btn" data-action="open-settings" data-tab="accounts" title="Settings (,)">
              ${iconSvg("settings")}<span>Settings</span><kbd>,</kbd>
            </button>
            <button type="button" class="sidebar-menu-btn" data-action="help" title="Shortcuts (?)">
              ${iconSvg("help")}<span>Shortcuts</span><kbd>?</kbd>
            </button>
          </div>
        </div>

        <div class="sidebar-group grow">
          <div class="sidebar-group-label">Sessions</div>
          <div class="sidebar-filter">
            <input id="filter" type="search" placeholder="Filter sessions…" value="${esc(state.filter)}" autocomplete="off" />
            <span class="sess-count" id="sess-count" title="running / total">0/0</span>
          </div>
          <div class="session-list" id="session-list">
            ${sessionListHTML()}
          </div>
        </div>
      </div>

      <div class="sidebar-footer">
        <button type="button" class="nav-user" data-action="open-settings" data-tab="about" title="About this host">
          <span class="nav-user-avatar">a</span>
          <span class="nav-user-text">
            <strong id="nav-host">${esc(host)}</strong>
            <span id="nav-status">${esc(state.statusText || "connected")}</span>
          </span>
          ${iconSvg("settings")}
        </button>
        <div class="sidebar-foot-actions">
          <button type="button" class="ghost btn-sm" data-action="prune" title="Delete all stopped sessions">Clear stopped</button>
          <button type="button" class="ghost btn-sm" data-action="logout" title="Log out">${iconSvg("logout")} Logout</button>
        </div>
      </div>
    </aside>

    <main class="main">
      <header class="topbar">
        <div class="topbar-lead">
          <button type="button" class="ghost btn-icon" data-action="toggle-sidebar" title="Toggle sidebar (w)" aria-label="Toggle sidebar">
            ${iconSvg("panel")}
          </button>
          <span class="topbar-sep" aria-hidden="true"></span>
          ${breadcrumbHTML()}
        </div>
        <div class="tabs" id="tabs">${tabsHTML()}</div>
        <div class="topbar-actions">
          <span class="conn-pill conn-${state.conn}" id="conn-pill"><span class="conn-dot"></span>idle</span>
          <button type="button" class="primary btn-sm" data-action="new-session" title="New session (n)">New</button>
          <button type="button" class="ghost btn-sm" data-action="tools" title="Tools (t)">Tools</button>
          <button type="button" class="ghost btn-sm" data-action="open-settings" data-tab="accounts" title="Settings (,)">Settings</button>
        </div>
      </header>
      <div class="term-wrap">
        <div class="term-empty">
          <div class="term-empty-card">
            <div class="term-empty-mark" aria-hidden="true">a</div>
            <h2>No session selected</h2>
            <p>Open a session from the sidebar, or start a new one. Closing a tab detaches only — agents keep running in tmux.</p>
            <div class="term-empty-actions">
              <button type="button" class="primary" data-action="new-session">New session</button>
              <button type="button" class="ghost" data-action="tools">Tools</button>
            </div>
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
      <button type="button" class="primary btn-sm" data-action="new-session">Create one</button>
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
      const cursor = s.id === state.listCursorId ? "cursor" : "";
      const age = relativeTime(s.created_at);
      const ag = agentClass(s.agent);
      return `
      <div class="session-item ${active} ${cursor} ${ag}" data-open="${esc(s.id)}">
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
              ? `<button type="button" class="ghost btn-sm act" data-kill="${esc(s.id)}" title="Stop agent (s)">Stop</button>`
              : `<button type="button" class="ghost btn-sm act resume-text" data-resume="${esc(s.id)}" title="Resume agent (e)">Resume</button>`
          }
          <button type="button" class="ghost btn-sm act danger-text" data-delete="${esc(s.id)}" title="Delete session (Shift+d)">Delete</button>
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

/** xterm helper textarea — agent should get bare keys; Alt chords still work. */
function isXtermTarget(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  return (
    el.classList.contains("xterm-helper-textarea") ||
    !!el.closest(".xterm") ||
    !!el.closest(".term-host")
  );
}

const SETTINGS_TABS: SettingsTab[] = [
  "accounts",
  "github",
  "ssh",
  "workspace",
  "about",
];

function focusFilter(): void {
  state.sidebarOpen = true;
  paintChrome();
  window.queueMicrotask(() => {
    (document.getElementById("filter") as HTMLInputElement | null)?.focus();
  });
}

function focusTerminal(): void {
  if (!state.activeId) {
    toast("No session open", "info", 1600);
    return;
  }
  fit();
  term?.focus();
}

function cycleTab(delta: number): void {
  const tabs = state.openTabs;
  if (!tabs.length) {
    toast("No open tabs", "info", 1400);
    return;
  }
  const cur = tabs.findIndex((t) => t.id === state.activeId);
  const idx =
    cur < 0
      ? delta > 0
        ? 0
        : tabs.length - 1
      : (cur + delta + tabs.length) % tabs.length;
  void activateTab(tabs[idx].id);
}

function jumpTab(n: number): void {
  // 1–9 → index 0–8; 0 → last
  if (n === 0) {
    const last = state.openTabs[state.openTabs.length - 1];
    if (last) void activateTab(last.id);
    return;
  }
  const tab = state.openTabs[n - 1];
  if (tab) void activateTab(tab.id);
}

function closeAllTabs(): void {
  if (!state.openTabs.length) return;
  detachPty();
  disposeTerminal();
  attachedId = null;
  state.openTabs = [];
  state.activeId = null;
  state.conn = "idle";
  paintChrome();
  ensureTermArea();
  toast("Closed all tabs (agents still running)", "ok", 2200);
}

function targetSessionId(): string | null {
  return state.listCursorId || state.activeId;
}

function moveListCursor(delta: number): void {
  const list = filteredSessionsSorted();
  if (!list.length) {
    state.listCursorId = null;
    paintChrome();
    return;
  }
  state.sidebarOpen = true;
  let idx = list.findIndex((s) => s.id === state.listCursorId);
  if (idx < 0 && state.activeId) {
    idx = list.findIndex((s) => s.id === state.activeId);
  }
  if (idx < 0) idx = delta > 0 ? -1 : 0;
  idx = Math.max(0, Math.min(list.length - 1, idx + delta));
  state.listCursorId = list[idx].id;
  paintChrome();
  const el = document.querySelector(
    `.session-item[data-open="${CSS.escape(state.listCursorId)}"]`,
  );
  el?.scrollIntoView({ block: "nearest" });
}

function filteredSessionsSorted(): Session[] {
  const list = filteredSessions();
  return [...list].sort((a, b) => {
    if (a.state === "running" && b.state !== "running") return -1;
    if (b.state === "running" && a.state !== "running") return 1;
    const ta = a.created_at ? Date.parse(a.created_at) : 0;
    const tb = b.created_at ? Date.parse(b.created_at) : 0;
    return tb - ta;
  });
}

function openCursorOrActive(): void {
  const id = targetSessionId();
  if (!id) {
    toast("No session selected — use j/k or open a tab", "info", 2200);
    return;
  }
  const s = state.sessions.find((x) => x.id === id);
  if (!s) return;
  if (s.state === "running") {
    openTab(s);
    return;
  }
  toast("Not running — press e to resume", "info", 2200);
}

async function refreshSessionsManual(): Promise<void> {
  toast("Refreshing…", "info", 1200);
  await refreshSessions();
  toast("Sessions refreshed", "ok", 1400);
}

function openSettingsTabByIndex(i: number): void {
  const tab = SETTINGS_TABS[i];
  if (tab) openSettings(tab);
}

/**
 * Shared action map. `fromTerm` means Alt-chord over the TTY.
 * Returns true if handled.
 */
function handleShortcut(ev: KeyboardEvent, _fromTerm: boolean): boolean {
  const key = ev.key;
  const lower = key.length === 1 ? key.toLowerCase() : key;
  const shift = ev.shiftKey;

  // Modal open: don't steal bare keys (form focus can be on buttons). Alt still works.
  if ((state.panel || state.drawer) && !ev.altKey) {
    if (lower === "?" || (lower === "/" && shift)) {
      openPanel("help");
      return true;
    }
    return false;
  }

  // Settings: Alt+Shift+1–5 always; bare 1–5 while Settings is open
  if (key >= "1" && key <= "5") {
    if ((ev.altKey && shift) || (state.settingsOpen && !ev.altKey && !shift)) {
      openSettingsTabByIndex(Number(key) - 1);
      return true;
    }
  }

  // While Settings is open, prefer nav shortcuts; skip session kill/delete/etc on bare keys
  if (state.settingsOpen && !ev.altKey) {
    if (lower === "a" || lower === "," || lower === "<") {
      openSettings("accounts");
      return true;
    }
    if (lower === "g") {
      openSettings("github");
      return true;
    }
    if (lower === "k") {
      openSettings("ssh");
      return true;
    }
    if (lower === "t") {
      openSettings("workspace");
      return true;
    }
    if (lower === "?" || (lower === "/" && shift)) {
      openPanel("help");
      return true;
    }
    if (lower === "r") {
      void loadSettingsData().then(() => paintSettings());
      return true;
    }
    // block session ops while reading settings
    if (
      ["s", "e", "d", "c", "x", "j", "n", "o", "w", "f", "/", "`"].includes(lower) ||
      key === "Enter" ||
      key.startsWith("Arrow")
    ) {
      return false;
    }
  }

  // Digits: open tabs (0 = last)
  if (key >= "0" && key <= "9" && !shift) {
    jumpTab(Number(key));
    return true;
  }

  switch (key) {
    case "ArrowDown":
      moveListCursor(1);
      return true;
    case "ArrowUp":
      moveListCursor(-1);
      return true;
    case "ArrowLeft":
      cycleTab(-1);
      return true;
    case "ArrowRight":
      cycleTab(1);
      return true;
    case "Enter":
      if (ev.altKey) {
        focusTerminal();
        return true;
      }
      openCursorOrActive();
      return true;
    default:
      break;
  }

  switch (lower) {
    case "n":
      openPanel("new");
      return true;
    case "t":
      openPanel("tools");
      return true;
    case ",":
    case "<":
      openSettings("accounts");
      return true;
    case "a":
      openSettings("accounts");
      return true;
    case "g":
      openSettings("github");
      return true;
    case "k":
      if (shift) {
        openSettings("ssh");
        return true;
      }
      moveListCursor(-1);
      return true;
    case "j":
      moveListCursor(1);
      return true;
    case "?":
      openPanel("help");
      return true;
    case "/":
      if (shift) {
        openPanel("help");
        return true;
      }
      focusFilter();
      return true;
    case "f":
      if (state.activeId) fit();
      else toast("No terminal to fit", "info", 1400);
      return true;
    case "w":
      state.sidebarOpen = !state.sidebarOpen;
      paintChrome();
      return true;
    case "o":
      openCursorOrActive();
      return true;
    case "[":
      cycleTab(-1);
      return true;
    case "]":
      cycleTab(1);
      return true;
    case "x":
      if (shift) {
        closeAllTabs();
        return true;
      }
      if (state.activeId) {
        closeTab(state.activeId);
        toast("Tab closed (agent still running)", "ok", 1600);
      } else toast("No tab to close", "info", 1400);
      return true;
    case "s":
      {
        const id = targetSessionId();
        if (id) void onKillSession(id);
        else toast("No session to stop", "info", 1400);
      }
      return true;
    case "e":
      {
        const id = targetSessionId();
        if (id) void onResumeSession(id);
        else toast("No session to resume", "info", 1400);
      }
      return true;
    case "d":
      if (shift) {
        const id = targetSessionId();
        if (id) void onDeleteSession(id);
        else toast("No session to delete", "info", 1400);
        return true;
      }
      return false;
    case "c":
      void onPrune();
      return true;
    case "r":
      void refreshSessionsManual();
      return true;
    case "`":
      focusTerminal();
      return true;
    default:
      return false;
  }
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
      if (state.settingsOpen) {
        closeSettings();
        ev.preventDefault();
        return;
      }
      if (state.sidebarOpen) {
        state.sidebarOpen = false;
        paintChrome();
        ev.preventDefault();
        return;
      }
      // blur terminal → bare shortcuts work without Alt
      if (isXtermTarget(ev.target)) {
        (document.activeElement as HTMLElement | null)?.blur?.();
        toast("Shortcuts armed — press ? for help", "info", 1800);
        ev.preventDefault();
      }
      return;
    }

    const typing = isTypingTarget(ev.target);
    const inTerm = isXtermTarget(ev.target);

    // Form fields: only Escape (above). Don't steal typing.
    if (typing && !inTerm) return;

    // Ctrl/Cmd reserved for browser + terminal (copy etc.)
    if (ev.metaKey || ev.ctrlKey) return;

    // Over TTY: only Alt-chords (bare keys belong to the agent)
    if (inTerm) {
      if (!ev.altKey) return;
      if (handleShortcut(ev, true)) {
        ev.preventDefault();
        ev.stopPropagation();
      }
      return;
    }

    // Outside TTY: Alt optional; bare keys work
    // When Alt is held, still handle (same map)
    if (handleShortcut(ev, false)) {
      ev.preventDefault();
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
      case "open-settings":
        ev.preventDefault();
        ev.stopPropagation();
        if (!state.token) return;
        openSettings(
          (actionEl.getAttribute("data-tab") as SettingsTab) || "accounts",
        );
        break;
      case "close-settings":
        ev.preventDefault();
        ev.stopPropagation();
        closeSettings();
        break;
      case "settings-tab":
        ev.preventDefault();
        {
          const tab = actionEl.getAttribute("data-tab") as SettingsTab | null;
          if (tab) {
            state.settingsTab = tab;
            void loadSettingsData().then(() => paintSettings());
          }
        }
        break;
      case "settings-platform":
        ev.preventDefault();
        state.settingsPlatform = actionEl.getAttribute("data-platform") || "grok";
        paintSettings();
        break;
      case "settings-refresh":
        ev.preventDefault();
        void loadSettingsData().then(() => paintSettings());
        break;
      case "acct-save": {
        ev.preventDefault();
        const platform = actionEl.getAttribute("data-platform") || "";
        const id = actionEl.getAttribute("data-id") || "";
        const label = actionEl.getAttribute("data-label") || id;
        if (platform && id) void onAcctSave(platform, id, label);
        break;
      }
      case "acct-switch": {
        ev.preventDefault();
        const platform = actionEl.getAttribute("data-platform") || "";
        const id = actionEl.getAttribute("data-id") || "";
        if (platform && id) void onAcctSwitch(platform, id);
        break;
      }
      case "acct-add":
        ev.preventDefault();
        void onAcctAdd();
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
      case "ctx-ensure":
        ev.preventDefault();
        void onCtxEnsure();
        break;
      case "ctx-pack":
        ev.preventDefault();
        void onCtxPack();
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
      case "pw-install":
        ev.preventDefault();
        void onPwAction("install");
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
      case "gh-login":
        ev.preventDefault();
        void onGHLogin();
        break;
      case "gh-setup-git":
        ev.preventDefault();
        void onGHSetupGit();
        break;
      case "gh-switch": {
        ev.preventDefault();
        const u = actionEl.getAttribute("data-user") || "";
        const h = actionEl.getAttribute("data-host") || "github.com";
        if (u) void onGHSwitch(u, h);
        break;
      }
      case "gh-logout": {
        ev.preventDefault();
        const u = actionEl.getAttribute("data-user") || "";
        const h = actionEl.getAttribute("data-host") || "github.com";
        if (u) void onGHLogout(u, h);
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
      // reload account profiles for this agent platform
      void refreshAgentAccountsForForm(el.value).then(() => {
        if (state.panel === "new") paintPanel();
      });
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

// boot — agentsctl web passes #token=… for one-shot login
{
  const fromURL = consumeAuthFromURL();
  if (fromURL) {
    state.token = fromURL;
  }
}
if (state.token) {
  void bootstrapAuthed();
} else {
  paint();
}
