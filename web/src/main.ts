import "./styles.css";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";

import {
  cwdFromProjectId,
  currentRoute,
  parsePath,
  projectIdFromCwd,
  serializeRoute,
  sessionPath,
  type ProfileTab,
  type Route,
} from "./router";

function parsePathSafe(pathname: string): Route {
  return parsePath(pathname);
}

import {
  clearToken,
  cloneWorkspace,
  consumeAuthFromURL,
  contextEnsure,
  contextPack,
  contextStatus,
  createBackup,
  createSession,
  openWorkspace,
  historySearch,
  installSkills,
  listRecordings,
  listTemplates,
  listTasks,
  createTask,
  updateTask,
  deleteTask,
  notifyTest,
  startTemplate,
  uploadImage,
  workspaceDashboard,
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
  addAgentAccount,
  listAgentAccounts,
  listAllAgentAccounts,
  listAgents,
  listSessions,
  listSSHKeys,
  createWorkspace,
  listWorkspaces,
  removeAgentAccount,
  saveAgentAccount,
  switchAgentAccount,
  formatPlaywrightStatus,
  gitCommit,
  gitDiff,
  gitPullRequest,
  gitStatus,
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
  type GitFileEntry,
  type GitStatusResult,
  type MemoryHit,
  type Session,
  type SessionTemplate,
  type SSHKey,
  type TaskStatus,
  type WorkspaceOpenResult,
  type WorkspaceTask,
} from "./api";
import {
  closeAppDrawer,
  isAppDrawerOpen,
  openAppDrawer,
} from "./drawer-bridge";
import {
  animateLoginIn,
  animateSettingsIn,
  animateSettingsOut,
  animateShellIn,
  animateToastIn,
  animateToastOut,
  bindPressMotion,
} from "./motion";
import { PtyClient } from "./pty";

let vaulReady = false;
async function ensureVaulHost(): Promise<void> {
  if (vaulReady) return;
  const { mountVaulHost } = await import("./vaul-host");
  mountVaulHost();
  vaulReady = true;
  // Wait for React to commit + subscribeDrawer before first openAppDrawer emit.
  await new Promise<void>((resolve) => {
    window.requestAnimationFrame(() => resolve());
  });
}

type OpenTab = {
  id: string;
  title: string;
  agent: string;
  cwd: string;
  state: string;
};

type Panel = null | "new" | "new-project" | "projects" | "tools" | "help" | "changes" | "remote";

type ProjectCard = {
  path: string;
  abs?: string;
  map_exists?: boolean;
  map_stale?: boolean;
  has_context?: boolean;
  memory_docs?: number;
  live_sessions?: number;
};
type SettingsTab = ProfileTab;
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
  /** When true, new session form creates a folder instead of using the select */
  formCwdNew: boolean;
  formCwdNewName: string;
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
  settingsAccountsBin: string;
  settingsAccountsError: string;
  settingsAcctId: string;
  settingsAcctLabel: string;
  settingsBusy: boolean;
  /** keyboard highlight in session list (j/k) */
  listCursorId: string | null;
  commandPalette: boolean;
  paletteQuery: string;
  /** highlighted index in spotlight results */
  paletteIndex: number;
  templates: SessionTemplate[];
  tasks: WorkspaceTask[];
  tasksDraft: string;
  tasksBusy: boolean;
  theme: "dark" | "light";
  /** Git Changes panel */
  gitStatus: GitStatusResult | null;
  gitLoading: boolean;
  gitError: string;
  /** null = all changes */
  gitSelectedPath: string | null;
  /** paths checked for commit (empty = all dirty files) */
  gitCheckedPaths: string[];
  gitDiffText: string;
  gitDiffLoading: boolean;
  gitCommitMsg: string;
  gitPrTitle: string;
  gitPrBody: string;
  gitPrBase: string;
  gitPrDraft: boolean;
  gitPrUrl: string;
  gitBusy: boolean;
  /** Filter string for git file list */
  gitFileFilter: string;
  /** PR composer expanded */
  gitPrOpen: boolean;
  /** Open remote page payload */
  remoteInfo: WorkspaceOpenResult | null;
  remoteLoading: boolean;
  remoteError: string;
  /** Projects list page */
  projectCards: ProjectCard[];
  projectsLoading: boolean;
  projectsError: string;
  projectsFilter: string;
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
  formCwdNew: false,
  formCwdNewName: "",
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
  settingsAccountsBin: "",
  settingsAccountsError: "",
  settingsAcctId: "personal",
  settingsAcctLabel: "",
  settingsBusy: false,
  listCursorId: null,
  commandPalette: false,
  paletteQuery: "",
  paletteIndex: 0,
  templates: [],
  tasks: [],
  tasksDraft: "",
  tasksBusy: false,
  theme: (localStorage.getItem("agents_theme") as "dark" | "light") || "dark",
  gitStatus: null,
  gitLoading: false,
  gitError: "",
  gitSelectedPath: null,
  gitCheckedPaths: [],
  gitDiffText: "",
  gitDiffLoading: false,
  gitCommitMsg: "",
  gitPrTitle: "",
  gitPrBody: "",
  gitPrBase: "",
  gitPrDraft: false,
  gitPrUrl: "",
  gitBusy: false,
  gitFileFilter: "",
  gitPrOpen: false,
  remoteInfo: null,
  remoteLoading: false,
  remoteError: "",
  projectCards: [],
  projectsLoading: false,
  projectsError: "",
  projectsFilter: "",
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
/** Full list paint signature (content + selection). Skip DOM rewrite when unchanged. */
let lastSessionListPaintSig = "";
/** Tabs strip signature — skip rewrite when open tabs unchanged. */
let lastTabsPaintSig = "";
/** Status bar + sess-count + nav status signature. */
let lastStatusPaintSig = "";
/** Breadcrumb signature. */
let lastCrumbPaintSig = "";
/** Debounce timer for sidebar filter input. */
let filterDebounceTimer: number | null = null;
let pressMotionBound = false;
/** True while applying a route → UI actions must not push history again. */
let applyingRoute = false;
let routerBound = false;
/** Session id for the open right-click / ⋯ context menu. */
let ctxSessionId: string | null = null;

const app = document.getElementById("app")!;

// ── Client routing ──────────────────────────────────────────────────────────

function routeFromUI(): Route {
  if (state.settingsOpen) {
    return { name: "profile", tab: state.settingsTab };
  }
  if (state.panel === "tools") {
    if (state.activeId) {
      const s =
        state.sessions.find((x) => x.id === state.activeId) ||
        state.openTabs.find((x) => x.id === state.activeId);
      if (s) {
        return {
          name: "session",
          projectId: projectIdFromCwd(s.cwd),
          sessionId: s.id,
          tools: true,
        };
      }
    }
    return { name: "tools" };
  }
  if (state.panel === "changes") return { name: "changes" };
  if (state.panel === "help") return { name: "help" };
  if (state.panel === "remote") return { name: "remote" };
  if (state.panel === "projects") return { name: "projects" };
  if (state.panel === "new-project") return { name: "new-project" };
  if (state.panel === "new") return { name: "new" };
  if (state.activeId) {
    const s =
      state.sessions.find((x) => x.id === state.activeId) ||
      state.openTabs.find((x) => x.id === state.activeId);
    if (s) {
      return {
        name: "session",
        projectId: projectIdFromCwd(s.cwd),
        sessionId: s.id,
      };
    }
  }
  return { name: "desk" };
}

/**
 * Push/replace the URL and apply the route to UI.
 * Pass `{ apply: false }` when the UI was already updated (openPanel etc.).
 */
function navigate(
  route: Route,
  opts?: { replace?: boolean; apply?: boolean },
): void {
  const path = serializeRoute(route);
  const cur = window.location.pathname || "/";
  const samePath = cur === path || (path === "/" && (cur === "" || cur === "/"));
  if (!samePath) {
    if (opts?.replace) {
      history.replaceState(route, "", path);
    } else {
      history.pushState(route, "", path);
    }
  } else if (opts?.replace) {
    history.replaceState(route, "", path);
  }
  if (opts?.apply === false || applyingRoute) return;
  void applyRoute(route, { fromHistory: false });
}

/** Keep URL in sync with current UI without re-applying. */
function syncUrlFromUI(opts?: { replace?: boolean }): void {
  if (applyingRoute) return;
  const route = routeFromUI();
  const path = serializeRoute(route);
  const cur = window.location.pathname || "/";
  if (cur === path || (path === "/" && (cur === "" || cur === "/"))) return;
  if (opts?.replace) history.replaceState(route, "", path);
  else history.pushState(route, "", path);
}

async function applyRoute(
  route: Route,
  opts?: { fromHistory?: boolean },
): Promise<void> {
  if (!state.token || !state.shellMounted) {
    // Remember path; after login bootstrap will re-apply currentRoute()
    return;
  }
  if (applyingRoute) return;
  applyingRoute = true;
  try {
    switch (route.name) {
      case "new":
        if (state.settingsOpen) await closeSettingsOnly();
        if (state.panel !== "new") openPanelOnly("new");
        break;
      case "new-project":
        if (state.settingsOpen) await closeSettingsOnly();
        if (state.panel !== "new-project") openPanelOnly("new-project");
        break;
      case "desk":
        if (state.settingsOpen) await closeSettingsOnly();
        if (state.panel) await closePanelOnly();
        // keep active session if any — desk is "no modal"
        break;
      case "tools":
        if (state.settingsOpen) await closeSettingsOnly();
        if (state.panel !== "tools") openPanelOnly("tools");
        break;
      case "changes":
        if (state.settingsOpen) await closeSettingsOnly();
        if (state.panel !== "changes") openPanelOnly("changes");
        break;
      case "remote":
        if (state.settingsOpen) await closeSettingsOnly();
        if (state.panel !== "remote") openPanelOnly("remote");
        break;
      case "projects":
        if (state.settingsOpen) await closeSettingsOnly();
        if (state.panel !== "projects") openPanelOnly("projects");
        break;
      case "help":
        if (state.settingsOpen) await closeSettingsOnly();
        if (state.panel !== "help") openPanelOnly("help");
        break;
      case "profile":
        if (state.panel) await closePanelOnly();
        if (!state.settingsOpen || state.settingsTab !== route.tab) {
          openSettingsOnly(route.tab);
        }
        break;
      case "session": {
        if (state.settingsOpen) await closeSettingsOnly();
        const id = route.sessionId;
        let sess = state.sessions.find((s) => s.id === id);
        if (!sess) {
          // Fresh list — session might exist after resume/create race
          try {
            const res = await listSessions();
            state.sessions = res.sessions ?? [];
            sess = state.sessions.find((s) => s.id === id);
          } catch {
            /* ignore */
          }
        }
        if (!sess) {
          toast(`Session ${id.slice(0, 12)}… not found`, "err");
          navigate({ name: "desk" }, { replace: true });
          break;
        }
        // Align form cwd with project from URL when possible
        const fromUrl = cwdFromProjectId(route.projectId);
        if (fromUrl && state.workspaces.includes(fromUrl)) {
          state.formCwd = fromUrl;
        } else if (sess.cwd) {
          state.formCwd = sess.cwd;
        }
        if (sess.state !== "running") {
          // Opening a stopped session via deep link → try resume
          try {
            sess = await resumeSession(id);
            await refreshSessions();
            sess = state.sessions.find((s) => s.id === id) || sess;
          } catch (e) {
            toast((e as Error).message || "resume failed", "err");
          }
        }
        openTabOnly(sess);
        if (route.tools) {
          if (state.panel !== "tools") openPanelOnly("tools");
        } else if (state.panel) {
          await closePanelOnly();
        }
        break;
      }
    }
  } finally {
    applyingRoute = false;
  }
  // Ensure URL matches (replace if we redirected)
  if (opts?.fromHistory) {
    const want = serializeRoute(route);
    if ((window.location.pathname || "/") !== want) {
      history.replaceState(route, "", want);
    }
  }
}

function ensureRouterBound(): void {
  if (routerBound) return;
  routerBound = true;
  window.addEventListener("popstate", () => {
    const route = currentRoute();
    void applyRoute(route, { fromHistory: true });
  });
}

/** Panels that render as full in-shell stages (not modals). */
function isStagePanel(p: Panel): p is Exclude<Panel, null> {
  return (
    p === "new" ||
    p === "new-project" ||
    p === "projects" ||
    p === "tools" ||
    p === "help" ||
    p === "changes" ||
    p === "remote"
  );
}

/** UI-only panel open (no history). All primary panels are in-shell stages. */
function openPanelOnly(p: Panel): void {
  if (!p) return;
  state.panel = p;
  if (p === "new" || p === "new-project") state.createError = "";
  if (state.settingsOpen) {
    state.settingsOpen = false;
    document.getElementById("settings-root")?.remove();
  }
  if (p === "tools" || p === "changes" || p === "remote") {
    const tab = state.openTabs.find((t) => t.id === state.activeId);
    if (tab?.cwd && state.workspaces.includes(tab.cwd)) {
      state.formCwd = tab.cwd;
    }
  }
  // Close leftover modal chrome — stages own the main area
  if (isAppDrawerOpen()) closeAppDrawer("programmatic");
  document.getElementById("panel-overlay")?.remove();
  paintSidebarActive();
  window.queueMicrotask(async () => {
    if (state.panel !== p) return;
    try {
      if (p === "new") {
        await refreshAgentAccountsForForm(state.formAgent);
      }
      if (p === "remote") {
        void loadRemotePage();
      }
      if (p === "projects") {
        void loadProjectsPage();
      }
      paintAppStage();
      if (p === "changes") void refreshGitChanges();
      if (p === "tools") void refreshToolsStatus();
      if (p === "new") {
        window.requestAnimationFrame(() => {
          const name = document.getElementById("sess-name") as HTMLInputElement | null;
          const agent = document.getElementById("sess-agent") as HTMLSelectElement | null;
          (name ?? agent)?.focus();
        });
      }
      if (p === "new-project") {
        window.requestAnimationFrame(() => {
          const url = document.getElementById("proj-git-url") as HTMLInputElement | null;
          url?.focus();
        });
      }
    } catch (e) {
      console.error("openPanelOnly failed", e);
      toast((e as Error).message || "failed to open page", "err", 5000);
    }
  });
}

async function closePanelOnly(): Promise<void> {
  if (isAppDrawerOpen()) closeAppDrawer("programmatic");
  const hadStage = isStagePanel(state.panel);
  state.panel = null;
  state.createError = "";
  document.getElementById("panel-overlay")?.remove();
  if (hadStage) {
    unmountAppStage();
    paintBreadcrumb(true);
  }
  paintSidebarActive();
}

function openSettingsOnly(tab?: SettingsTab): void {
  if (tab) state.settingsTab = tab;
  state.settingsOpen = true;
  state.sidebarOpen = false;
  if (state.panel) {
    state.panel = null;
    if (isAppDrawerOpen()) closeAppDrawer("programmatic");
    document.getElementById("panel-overlay")?.remove();
    unmountAppStage();
  }
  paintSidebarActive();
  void loadSettingsData().then(() => paintSettings({ animateIn: true }));
}

async function closeSettingsOnly(): Promise<void> {
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
  paintSidebarActive();
}

function openTabOnly(s: Session): void {
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
  void activateTabOnly(s.id);
}

async function activateTabOnly(id: string): Promise<void> {
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

function applyTheme(): void {
  document.documentElement.dataset.theme = state.theme;
  localStorage.setItem("agents_theme", state.theme);
}

function toggleTheme(): void {
  state.theme = state.theme === "dark" ? "light" : "dark";
  applyTheme();
  toast(`Theme: ${state.theme}`, "ok", 1400);
}

function openCommandPalette(): void {
  hideTopbarMore();
  state.commandPalette = true;
  state.paletteIndex = 0;
  paintCommandPalette();
  window.requestAnimationFrame(() => {
    const input = document.getElementById("palette-input") as HTMLInputElement | null;
    input?.focus();
    input?.select();
  });
}

function closeCommandPalette(): void {
  state.commandPalette = false;
  state.paletteQuery = "";
  state.paletteIndex = 0;
  document.getElementById("command-palette")?.remove();
}

type PaletteItem = {
  id: string;
  label: string;
  hint?: string;
  /** secondary mono meta (e.g. cwd / agent) */
  meta?: string;
  group: string;
  run: () => void;
};

function paletteItems(): PaletteItem[] {
  const q = state.paletteQuery.trim().toLowerCase();
  const items: PaletteItem[] = [];

  // Sessions first when searching — spotlight-style jump-to-session
  const sessions = [...state.sessions].sort((a, b) => {
    const ar = a.state === "running" ? 0 : 1;
    const br = b.state === "running" ? 0 : 1;
    if (ar !== br) return ar - br;
    return (b.created_at || "").localeCompare(a.created_at || "");
  });
  for (const s of sessions.slice(0, 40)) {
    const label = s.name || shortId(s.id);
    const branch = (s.git_branch || s.branch || "").trim();
    const hay = [label, s.agent, s.cwd, s.id, branch, s.state].join(" ").toLowerCase();
    if (q && !hay.includes(q)) continue;
    items.push({
      id: `sess-${s.id}`,
      label,
      meta: `${s.agent}${branch ? ` · ${branch}` : ""} · ${basename(s.cwd)}`,
      hint: s.state === "running" ? "live" : s.state,
      group: "Sessions",
      run: () => openTab(s),
    });
  }

  const commands: PaletteItem[] = [
    { id: "new", label: "New session", hint: "n", group: "Commands", run: () => openPanel("new") },
    {
      id: "new-project",
      label: "New project (clone repo)",
      hint: "⇧n",
      group: "Commands",
      run: () => openPanel("new-project"),
    },
    {
      id: "projects",
      label: "Projects",
      group: "Commands",
      run: () => openPanel("projects"),
    },
    { id: "tools", label: "Tools", hint: "t", group: "Commands", run: () => openPanel("tools") },
    {
      id: "changes",
      label: "Git changes",
      hint: "⇧g",
      group: "Commands",
      run: () => openPanel("changes"),
    },
    {
      id: "shell",
      label: "Terminal",
      hint: "⇧t",
      group: "Commands",
      run: () => {
        void openShellTerminal();
      },
    },
    {
      id: "open-remote",
      label: "Open remote editor…",
      group: "Commands",
      run: () => {
        void showOpenRemoteDrawer();
      },
    },
    {
      id: "settings",
      label: "Settings",
      hint: ",",
      group: "Commands",
      run: () => openSettings("accounts"),
    },
    {
      id: "accounts",
      label: "Agent accounts",
      hint: "a",
      group: "Commands",
      run: () => openSettings("accounts"),
    },
    { id: "help", label: "Shortcuts", hint: "?", group: "Commands", run: () => openPanel("help") },
    {
      id: "ensure",
      label: "Ensure context (active cwd)",
      group: "Workspace",
      run: () => {
        void onCtxEnsure();
      },
    },
    {
      id: "dashboard",
      label: "Workspace dashboard",
      group: "Workspace",
      run: () => {
        void showDashboardDrawer();
      },
    },
    {
      id: "tasks",
      label: "Workspace tasks",
      group: "Workspace",
      run: () => {
        void showTasksDrawer();
      },
    },
    {
      id: "templates",
      label: "Start from template…",
      group: "Workspace",
      run: () => {
        void showTemplatesDrawer();
      },
    },
    {
      id: "history",
      label: "Search history…",
      group: "Workspace",
      run: () => {
        void showHistorySearch();
      },
    },
    {
      id: "recordings",
      label: "List recordings",
      group: "Workspace",
      run: () => {
        void showRecordingsDrawer();
      },
    },
    {
      id: "backup",
      label: "Create backup",
      group: "Host",
      run: () => {
        void createBackup()
          .then((o) => toast(`Backup: ${o.path || "ok"}`, "ok", 5000))
          .catch((e) => toast((e as Error).message, "err"));
      },
    },
    {
      id: "skills",
      label: "Install skills into cwd",
      group: "Host",
      run: () => {
        void installSkills(toolsCwd())
          .then((o) => toast(`Skills: ${o.path || "ok"}`, "ok"))
          .catch((e) => toast((e as Error).message, "err"));
      },
    },
    {
      id: "notify",
      label: "Test webhook notify",
      group: "Host",
      run: () => {
        void notifyTest()
          .then(() => toast("Notify test sent", "ok"))
          .catch((e) => toast((e as Error).message, "err"));
      },
    },
    {
      id: "theme",
      label: "Toggle light/dark theme",
      group: "Appearance",
      run: () => toggleTheme(),
    },
    {
      id: "refresh",
      label: "Refresh sessions",
      group: "Navigation",
      run: () => void refreshSessionsManual(),
    },
    {
      id: "next-tab",
      label: "Next tab",
      hint: "l / ]",
      group: "Navigation",
      run: () => cycleTab(1),
    },
    {
      id: "prev-tab",
      label: "Previous tab",
      hint: "h / [",
      group: "Navigation",
      run: () => cycleTab(-1),
    },
    {
      id: "next-session",
      label: "Next session (open)",
      hint: "⇧j",
      group: "Navigation",
      run: () => stepSessionAndOpen(1),
    },
    {
      id: "prev-session",
      label: "Previous session (open)",
      hint: "⇧k",
      group: "Navigation",
      run: () => stepSessionAndOpen(-1),
    },
    {
      id: "copy-id",
      label: "Copy session id",
      hint: "y",
      group: "Navigation",
      run: () => void copyTargetSessionId(),
    },
  ];
  for (const t of state.templates.slice(0, 12)) {
    commands.push({
      id: `tpl-${t.id}`,
      label: `Template: ${t.name}`,
      hint: t.agent,
      group: "Templates",
      run: () => {
        void startTemplate(t.id)
          .then((sess) => {
            toast(`Started ${sess.agent} · ${sess.id}`, "ok");
            void refreshSessions().then(() => {
              const s = state.sessions.find((x) => x.id === sess.id);
              if (s) openTab(s);
            });
          })
          .catch((e) => toast((e as Error).message, "err"));
      },
    });
  }

  for (const i of commands) {
    if (!q) {
      items.push(i);
      continue;
    }
    if (
      i.label.toLowerCase().includes(q) ||
      i.group.toLowerCase().includes(q) ||
      (i.hint && i.hint.toLowerCase().includes(q)) ||
      (i.meta && i.meta.toLowerCase().includes(q)) ||
      i.id.includes(q)
    ) {
      items.push(i);
    }
  }

  // When idle (no query), cap session noise — show a few recent then all commands
  if (!q) {
    const sess = items.filter((x) => x.group === "Sessions").slice(0, 6);
    const rest = items.filter((x) => x.group !== "Sessions");
    return [...sess, ...rest];
  }
  return items;
}

function runPaletteIndex(idx: number): void {
  const items = paletteItems();
  const item = items[idx];
  if (!item) return;
  closeCommandPalette();
  item.run();
}

function paintCommandPalette(): void {
  if (!state.commandPalette) {
    document.getElementById("command-palette")?.remove();
    return;
  }
  let root = document.getElementById("command-palette");
  if (!root) {
    root = document.createElement("div");
    root.id = "command-palette";
    root.className = "palette-root";
    document.body.appendChild(root);
    root.addEventListener("mousedown", (ev) => {
      if (ev.target === root) closeCommandPalette();
    });
  }
  const items = paletteItems();
  if (state.paletteIndex >= items.length) state.paletteIndex = Math.max(0, items.length - 1);
  if (state.paletteIndex < 0) state.paletteIndex = 0;
  let lastGroup = "";
  const listHtml = items.length
    ? items
        .map((it, i) => {
          const head =
            it.group !== lastGroup
              ? ((lastGroup = it.group),
                `<div class="palette-group" role="presentation">${esc(it.group)}</div>`)
              : "";
          const selected = i === state.paletteIndex;
          return `${head}
          <button type="button" class="palette-item${selected ? " active" : ""}" data-palette-id="${esc(it.id)}" data-palette-idx="${i}" role="option" aria-selected="${selected ? "true" : "false"}">
            <span class="palette-item-label">${esc(it.label)}</span>
            ${it.meta ? `<span class="palette-item-meta">${esc(it.meta)}</span>` : ""}
            ${it.hint ? `<kbd class="palette-kbd">${esc(it.hint)}</kbd>` : ""}
          </button>`;
        })
        .join("")
    : `<div class="palette-empty">No matches</div>`;
  root.innerHTML = `
    <div class="palette-card" role="dialog" aria-modal="true" aria-label="Search">
      <div class="palette-input-row">
        <span class="palette-search-ico" aria-hidden="true">${iconSvg("search")}</span>
        <input id="palette-input" class="palette-input" type="search" placeholder="Search sessions, commands, templates…" value="${esc(state.paletteQuery)}" autocomplete="off" spellcheck="false" aria-autocomplete="list" aria-controls="palette-list" />
        <kbd class="palette-esc">esc</kbd>
      </div>
      <div id="palette-list" class="palette-list" role="listbox" aria-label="Results">${listHtml}</div>
      <div class="palette-foot">
        <span class="palette-foot-left">${items.length} result${items.length === 1 ? "" : "s"}</span>
        <span><kbd>↑</kbd><kbd>↓</kbd> navigate</span>
        <span><kbd>↵</kbd> open</span>
      </div>
    </div>`;
  const input = document.getElementById("palette-input") as HTMLInputElement | null;
  input?.addEventListener("input", () => {
    state.paletteQuery = input.value;
    state.paletteIndex = 0;
    paintCommandPalette();
    (document.getElementById("palette-input") as HTMLInputElement | null)?.focus();
  });
  root.querySelectorAll<HTMLElement>("[data-palette-id]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const idx = Number(btn.getAttribute("data-palette-idx") || "0");
      runPaletteIndex(idx);
    });
    btn.addEventListener("mouseenter", () => {
      const idx = Number(btn.getAttribute("data-palette-idx") || "0");
      if (idx === state.paletteIndex) return;
      state.paletteIndex = idx;
      root!.querySelectorAll(".palette-item").forEach((el, i) => {
        el.classList.toggle("active", i === idx);
        el.setAttribute("aria-selected", i === idx ? "true" : "false");
      });
    });
  });
  // Keep selection in view
  root.querySelector(".palette-item.active")?.scrollIntoView({ block: "nearest" });
}

/** ⌘ on Apple, Ctrl elsewhere — for kbd chips in UI. */
function metaKbd(): string {
  try {
    const p = navigator.platform || "";
    const ua = navigator.userAgent || "";
    if (/Mac|iPhone|iPad|iPod/i.test(p) || /Mac OS X/i.test(ua)) return "⌘";
  } catch {
    /* ignore */
  }
  return "Ctrl";
}

/** Body-level topbar ⋮ menu (portaled so it stacks above the terminal). */
function hideTopbarMore(): void {
  const menu = document.getElementById("topbar-more-menu");
  if (menu) {
    menu.hidden = true;
    menu.innerHTML = "";
  }
  document.querySelectorAll<HTMLElement>("[data-action='topbar-more']").forEach((b) => {
    b.setAttribute("aria-expanded", "false");
    b.classList.remove("is-open");
  });
}

function showTopbarMore(anchor: HTMLElement): void {
  hideSessionCtx();
  let menu = document.getElementById("topbar-more-menu");
  if (!menu) {
    menu = document.createElement("div");
    menu.id = "topbar-more-menu";
    menu.className = "tb-menu";
    menu.setAttribute("role", "menu");
    menu.setAttribute("aria-label", "More actions");
    document.body.appendChild(menu);
  } else {
    menu.className = "tb-menu";
  }

  const item = (
    action: string,
    icon: string,
    label: string,
    hint?: string,
    kbd?: string,
    opts?: { danger?: boolean; extra?: string },
  ) =>
    `<button type="button" role="menuitem" class="tb-menu-item${opts?.danger ? " is-danger" : ""}" data-action="${esc(action)}"${opts?.extra || ""}>
      <span class="tb-menu-ico" aria-hidden="true">${iconSvg(icon)}</span>
      <span class="tb-menu-text">
        <span class="tb-menu-label">${esc(label)}</span>
        ${hint ? `<span class="tb-menu-hint">${esc(hint)}</span>` : ""}
      </span>
      ${kbd ? `<kbd class="tb-menu-kbd">${esc(kbd)}</kbd>` : ""}
    </button>`;

  menu.innerHTML = `
    <div class="tb-menu-head" role="presentation">
      <span class="tb-menu-title">Actions</span>
    </div>
    <div class="tb-menu-section" role="presentation">
      <div class="tb-menu-section-label">Create</div>
      ${item("new-session", "plus", "New session", "Start an agent", "n")}
      ${item("new-project", "folder-git", "New project", "Clone a repository", "⇧n")}
      ${item("projects", "layers", "Projects", "Browse workspaces")}
      ${item("open-shell", "terminal", "Terminal", "Plain shell session", "⇧t")}
    </div>
    <div class="tb-menu-sep" role="separator"></div>
    <div class="tb-menu-section" role="presentation">
      <div class="tb-menu-section-label">Workspace</div>
      ${item("open-remote", "external", "Open remote", "Cursor / VS Code / Zed")}
      ${item("tools", "wrench", "Tools", "Map, memory, browser", "t")}
      ${item("git-changes", "git-branch", "Changes", "Diff, commit, PR", "⇧g")}
    </div>
    <div class="tb-menu-sep" role="separator"></div>
    <div class="tb-menu-section" role="presentation">
      <div class="tb-menu-section-label">App</div>
      ${item("open-settings", "settings", "Settings", "Accounts & host", ",", { extra: ' data-tab="accounts"' })}
      ${item("help", "help", "Keyboard shortcuts", "Cheat sheet", "?")}
      ${item("toggle-theme", "layers", "Toggle theme", state.theme === "dark" ? "Switch to light" : "Switch to dark")}
    </div>
    <div class="tb-menu-sep" role="separator"></div>
    <div class="tb-menu-section" role="presentation">
      ${item("prune", "trash", "Clear stopped", "Remove exited sessions", undefined, { danger: true })}
    </div>
  `;
  menu.hidden = false;
  anchor.setAttribute("aria-expanded", "true");
  anchor.classList.add("is-open");

  const pad = 8;
  const r = anchor.getBoundingClientRect();
  const rect = menu.getBoundingClientRect();
  const w = rect.width || 260;
  const h = rect.height || 420;
  let left = r.right - w;
  let top = r.bottom + 8;
  if (left < pad) left = pad;
  if (left + w > window.innerWidth - pad) left = window.innerWidth - w - pad;
  if (top + h > window.innerHeight - pad) top = Math.max(pad, r.top - h - 8);
  menu.style.left = `${Math.round(left)}px`;
  menu.style.top = `${Math.round(top)}px`;
}

function toggleTopbarMore(anchor: HTMLElement): void {
  const menu = document.getElementById("topbar-more-menu");
  if (menu && !menu.hidden) {
    hideTopbarMore();
    return;
  }
  showTopbarMore(anchor);
}

/** Plain shell session in the current/default workspace (no AI agent). */
async function openShellTerminal(cwd?: string): Promise<void> {
  const useCwd = (cwd || state.formCwd || state.defaultCwd || ".").trim() || ".";
  try {
    toast("Starting shell…", "info", 4000);
    const sess = await createSession({
      agent: "shell",
      cwd: useCwd,
      name: `shell · ${useCwd === "." ? "root" : useCwd.split("/").pop() || useCwd}`,
    });
    await refreshSessions();
    openTab(sess);
    toast(`Shell in ${useCwd}`, "ok");
  } catch (e) {
    toast((e as Error).message || "shell failed", "err");
  }
}

/** Open remote as full in-shell page (not a drawer). */
async function showOpenRemoteDrawer(cwd?: string): Promise<void> {
  if (cwd) {
    state.formCwd = cwd;
  }
  openPanel("remote");
}

async function loadRemotePage(): Promise<void> {
  const useCwd = (toolsCwd() || state.formCwd || state.defaultCwd || ".").trim() || ".";
  state.remoteLoading = true;
  state.remoteError = "";
  if (state.panel === "remote") paintAppStage();
  try {
    const out = await openWorkspace({ cwd: useCwd });
    state.remoteInfo = out;
    state.remoteLoading = false;
    state.remoteError = "";
    if (state.panel === "remote") paintAppStage();
    const primary = out.commands.cursor_remote || out.commands.zed_remote || out.commands.vscode_remote;
    if (primary) {
      try {
        await navigator.clipboard.writeText(primary);
        toast("Copied Cursor remote command", "ok", 2500);
      } catch {
        toast("Commands ready — copy from the page", "info", 2500);
      }
    }
  } catch (e) {
    state.remoteInfo = null;
    state.remoteLoading = false;
    state.remoteError = (e as Error).message || "open remote failed";
    if (state.panel === "remote") paintAppStage();
    toast(state.remoteError, "err");
  }
}

function remoteCmdCard(title: string, cmd: string | undefined, hint?: string): string {
  if (!cmd) return "";
  return `
    <div class="remote-cmd">
      <div class="remote-cmd-head">
        <strong>${esc(title)}</strong>
        ${hint ? `<span class="opt">${esc(hint)}</span>` : ""}
      </div>
      <pre class="remote-cmd-code">${esc(cmd)}</pre>
      <button type="button" class="ghost btn-sm" data-action="copy-text" data-text="${esc(cmd)}">Copy</button>
    </div>`;
}

function remotePageHTML(): string {
  const info = state.remoteInfo;
  const actions = `
    <label class="git-workspace">
      <span class="git-workspace-label">Workspace</span>
      <select id="remote-cwd-select" class="git-cwd-select" data-action-change="remote-cwd" ${state.remoteLoading ? "disabled" : ""}>
        ${workspaceOptionsHTML()}
      </select>
    </label>
    <button type="button" class="ghost btn-sm" data-action="remote-refresh" ${state.remoteLoading ? "disabled" : ""}>
      ${state.remoteLoading ? "Loading…" : "Refresh"}
    </button>`;

  let body = "";
  if (state.remoteLoading && !info) {
    body = `<div class="page-empty"><p class="git-empty-title">Loading commands…</p></div>`;
  } else if (state.remoteError && !info) {
    body = `<div class="page-empty"><p class="git-empty-title">Failed to load</p><p class="git-empty-hint">${esc(state.remoteError)}</p></div>`;
  } else if (info) {
    const c = info.commands || {};
    body = `
      <div class="remote-meta page-card">
        <div class="remote-meta-row"><span>Workspace</span><code>${esc(info.cwd)}</code></div>
        <div class="remote-meta-row"><span>Absolute</span><code>${esc(info.abs)}</code></div>
        ${info.ssh_host ? `<div class="remote-meta-row"><span>SSH host</span><code>${esc(info.ssh_host)}</code></div>` : ""}
        <div class="remote-meta-row"><span>Host editors</span><span>${
          info.editors?.length ? esc(info.editors.join(", ")) : "none on PATH (remote still works)"
        }</span></div>
      </div>
      <div class="remote-section">
        <h2 class="remote-section-title">On your laptop (SSH remote)</h2>
        <div class="remote-cmd-grid">
          ${remoteCmdCard("Cursor", c.cursor_remote, "recommended")}
          ${remoteCmdCard("Zed", c.zed_remote)}
          ${remoteCmdCard("VS Code", c.vscode_remote)}
        </div>
      </div>
      <div class="remote-section">
        <h2 class="remote-section-title">On this host</h2>
        <div class="remote-cmd-grid">
          ${remoteCmdCard("Cursor local", c.cursor_local)}
          ${remoteCmdCard("Zed local", c.zed_local)}
          ${remoteCmdCard("VS Code local", c.vscode_local)}
          ${remoteCmdCard("SSH shell", c.ssh)}
        </div>
      </div>`;
  } else {
    body = `<div class="page-empty"><p class="git-empty-title">No data</p></div>`;
  }

  return `
    ${pageHeroHTML("Workspace", "Open remote", "Copy commands to open this workspace in Cursor, Zed, or VS Code.", actions)}
    ${state.remoteError && info ? `<div class="git-banner" role="alert">${esc(state.remoteError)}</div>` : ""}
    <div class="page-body page-body--wide">${body}</div>`;
}

async function loadProjectsPage(): Promise<void> {
  state.projectsLoading = true;
  state.projectsError = "";
  if (state.panel === "projects") paintAppStage();
  try {
    // Prefer rich dashboard cards; fall back to listWorkspaces
    let cards: ProjectCard[] = [];
    try {
      const dash = await workspaceDashboard();
      cards = (dash.workspaces || []).map((w) => ({
        path: w.path,
        abs: w.abs,
        map_exists: w.map_exists,
        map_stale: w.map_stale,
        has_context: w.has_context,
        memory_docs: w.memory_docs,
        live_sessions: w.live_sessions,
      }));
    } catch {
      const ws = await listWorkspaces();
      const paths = normalizeWorkspacePaths(ws);
      cards = paths.map((path) => ({
        path,
        live_sessions: state.sessions.filter((s) => s.cwd === path && s.state === "running").length,
      }));
    }
    // Merge live session counts from client list when API omits them
    for (const c of cards) {
      if (typeof c.live_sessions !== "number") {
        c.live_sessions = state.sessions.filter((s) => s.cwd === c.path && s.state === "running").length;
      }
    }
    // Ensure every known workspace path appears
    const seen = new Set(cards.map((c) => c.path));
    for (const path of state.workspaces) {
      if (!seen.has(path)) {
        cards.push({
          path,
          live_sessions: state.sessions.filter((s) => s.cwd === path && s.state === "running").length,
        });
      }
    }
    cards.sort((a, b) => {
      const al = a.live_sessions || 0;
      const bl = b.live_sessions || 0;
      if (al !== bl) return bl - al;
      return a.path.localeCompare(b.path);
    });
    state.projectCards = cards;
    state.projectsLoading = false;
    // Keep flat list in sync
    const paths = cards.map((c) => c.path).filter(Boolean);
    if (paths.length) state.workspaces = paths;
  } catch (e) {
    state.projectsLoading = false;
    state.projectsError = (e as Error).message || "failed to load projects";
  }
  if (state.panel === "projects") paintAppStage();
}

function projectsListHTML(): string {
  const q = state.projectsFilter.trim().toLowerCase();
  let cards = state.projectCards || [];
  if (q) {
    cards = cards.filter(
      (c) =>
        c.path.toLowerCase().includes(q) ||
        (c.abs || "").toLowerCase().includes(q),
    );
  }
  if (state.projectsLoading && !cards.length) {
    return `<div class="page-empty"><p class="git-empty-title">Loading projects…</p></div>`;
  }
  if (state.projectsError && !cards.length) {
    return `<div class="page-empty"><p class="git-empty-title">Failed to load</p><p class="git-empty-hint">${esc(state.projectsError)}</p></div>`;
  }
  if (!cards.length) {
    return `<div class="page-empty">
      <div class="git-empty-icon" aria-hidden="true">${iconSvg("layers")}</div>
      <p class="git-empty-title">${q ? "No matching projects" : "No projects yet"}</p>
      <p class="git-empty-hint">${
        q
          ? "Try a different filter."
          : "Clone a repo or create a directory from New session."
      }</p>
      <div class="page-actions" style="justify-content:center;margin-top:0.75rem">
        <button type="button" class="primary btn-sm" data-action="new-project">${iconSvg("folder-git")} Clone project</button>
        <button type="button" class="ghost btn-sm" data-action="new-session">${iconSvg("plus")} New session</button>
      </div>
    </div>`;
  }
  return `<div class="project-list" role="list">${cards
    .map((c) => {
      const path = c.path || ".";
      const name = path === "." ? "workspace root" : basename(path);
      const live = c.live_sessions || 0;
      const sessHere = state.sessions.filter((s) => s.cwd === path);
      const total = sessHere.length;
      const chips: string[] = [];
      if (live > 0) chips.push(`<span class="git-chip git-chip--dirty">${live} live</span>`);
      else chips.push(`<span class="git-chip git-chip--muted">idle</span>`);
      if (total > live) chips.push(`<span class="git-chip">${total} sessions</span>`);
      if (c.map_exists) {
        chips.push(
          c.map_stale
            ? `<span class="git-chip git-chip--warn">map stale</span>`
            : `<span class="git-chip git-chip--clean">map</span>`,
        );
      } else {
        chips.push(`<span class="git-chip git-chip--muted">no map</span>`);
      }
      if (c.has_context) chips.push(`<span class="git-chip">context</span>`);
      if (typeof c.memory_docs === "number" && c.memory_docs > 0) {
        chips.push(`<span class="git-chip">${c.memory_docs} mem</span>`);
      }
      return `
      <article class="project-card" role="listitem" data-project-path="${esc(path)}">
        <div class="project-card-main">
          <div class="project-card-icon" aria-hidden="true">${iconSvg("folder-git")}</div>
          <div class="project-card-text">
            <h3 class="project-card-name" title="${esc(path)}">${esc(name)}</h3>
            <code class="project-card-path" title="${esc(c.abs || path)}">${esc(path)}</code>
            <div class="project-card-chips">${chips.join("")}</div>
          </div>
        </div>
        <div class="project-card-actions">
          <button type="button" class="primary btn-sm" data-action="project-new-session" data-cwd="${esc(path)}" title="New session here">Session</button>
          <button type="button" class="ghost btn-sm" data-action="project-tools" data-cwd="${esc(path)}" title="Tools">Tools</button>
          <button type="button" class="ghost btn-sm" data-action="project-git" data-cwd="${esc(path)}" title="Git changes">Git</button>
          <button type="button" class="ghost btn-sm" data-action="project-remote" data-cwd="${esc(path)}" title="Open remote">Remote</button>
          <button type="button" class="ghost btn-sm" data-action="copy-text" data-text="${esc(c.abs || path)}" title="Copy path">Copy</button>
        </div>
      </article>`;
    })
    .join("")}</div>`;
}

function projectsPageHTML(): string {
  const n = state.projectCards.length;
  const live = state.projectCards.reduce((a, c) => a + (c.live_sessions || 0), 0);
  const actions = `
    <label class="projects-filter">
      <span class="sr-only">Filter projects</span>
      ${iconSvg("search")}
      <input id="projects-filter" type="search" placeholder="Filter projects…" value="${esc(state.projectsFilter)}" autocomplete="off" spellcheck="false" />
    </label>
    <button type="button" class="ghost btn-sm" data-action="projects-refresh" ${state.projectsLoading ? "disabled" : ""}>
      ${state.projectsLoading ? "…" : "Refresh"}
    </button>
    <button type="button" class="ghost btn-sm" data-action="new-project">${iconSvg("folder-git")} Clone</button>
    <button type="button" class="primary btn-sm" data-action="new-session">${iconSvg("plus")} Session</button>`;
  return `
    ${pageHeroHTML(
      "Workspace",
      "Projects",
      `${n} project${n === 1 ? "" : "s"}${live ? ` · ${live} live session${live === 1 ? "" : "s"}` : ""} under the host workspace root.`,
      actions,
    )}
    ${state.projectsError ? `<div class="git-banner" role="alert">${esc(state.projectsError)}</div>` : ""}
    <div class="page-body page-body--wide" data-projects-list>
      ${projectsListHTML()}
    </div>`;
}

async function showDashboardDrawer(): Promise<void> {
  // Full projects page replaces the old plain-text dashboard drawer.
  openPanel("projects");
}

async function showTemplatesDrawer(): Promise<void> {
  try {
    const out = await listTemplates();
    state.templates = out.templates || [];
    const lines = state.templates
      .map((t) => `${t.id}  ${t.name}\n  agent=${t.agent}  cwd=${t.cwd}`)
      .join("\n\n");
    state.drawer = {
      title: "Templates (start via palette or agentsctl templates start)",
      body: lines || "No templates — save with agentsctl templates save --name …",
    };
    void paintDrawer();
  } catch (e) {
    toast((e as Error).message, "err");
  }
}

async function showTasksDrawer(): Promise<void> {
  await ensureVaulHost();
  await loadTasks();
  // Prefer interactive Vaul (not plain pre drawer)
  if (state.panel) {
    state.panel = null;
  }
  state.drawer = null;
  openAppDrawer({
    title: `Tasks · ${toolsCwd()}`,
    html: tasksPanelHTML(),
    variant: "sheet",
    onClose: (reason) => {
      if (reason === "user") {
        term?.focus();
      }
    },
  });
}

function tasksListHTML(): string {
  const tasks = state.tasks || [];
  if (!tasks.length) {
    return `<p class="tool-desc tasks-empty">No tasks yet — add one below. Stored in <code>.agents/tasks.json</code></p>`;
  }
  return `<ul class="task-list">${tasks
    .map((t) => {
      const done = t.status === "done";
      return `<li class="task-row ${done ? "task-row--done" : ""}" data-task-id="${esc(t.id)}">
        <button type="button" class="task-check" data-action="task-toggle" data-id="${esc(t.id)}" data-status="${esc(t.status)}" title="Toggle done" aria-label="Toggle done">
          ${done ? "✓" : t.status === "doing" ? "…" : ""}
        </button>
        <span class="task-title">${esc(t.title)}</span>
        <button type="button" class="task-status task-status--${esc(t.status)}" data-action="task-cycle" data-id="${esc(t.id)}" data-status="${esc(t.status)}" title="Cycle status">
          ${esc(t.status)}
        </button>
        <button type="button" class="ghost btn-sm danger-text task-del" data-action="task-delete" data-id="${esc(t.id)}" title="Delete" aria-label="Delete">×</button>
      </li>`;
    })
    .join("")}</ul>`;
}

function tasksPanelHTML(): string {
  const n = state.tasks.length;
  const open = state.tasks.filter((t) => t.status !== "done").length;
  return `
    <div class="tasks-panel">
      <p class="tool-desc">${open} open · ${n} total · <code>.agents/tasks.json</code></p>
      <div class="tasks-list-wrap" data-tasks-list>${tasksListHTML()}</div>
      <form class="task-add" data-action-form="task-add">
        <input name="title" data-task-draft placeholder="New task…" value="${esc(state.tasksDraft)}" autocomplete="off" ${state.tasksBusy ? "disabled" : ""} />
        <button type="submit" class="primary btn-sm" ${state.tasksBusy ? "disabled" : ""}>Add</button>
      </form>
    </div>`;
}

async function loadTasks(cwd?: string): Promise<void> {
  const use = (cwd || toolsCwd() || state.formCwd || state.defaultCwd || ".").trim() || ".";
  try {
    const out = await listTasks(use);
    state.tasks = out.tasks || [];
  } catch (e) {
    state.tasks = [];
    // soft — tools sheet still works without tasks
    console.warn("tasks load", e);
  }
}

function paintTasksList(): void {
  document.querySelectorAll("[data-tasks-list]").forEach((el) => {
    el.innerHTML = tasksListHTML();
  });
  // keep open Vaul tasks panel in sync when present
  const body = document.querySelector(".app-modal-body .tasks-panel");
  if (body && isAppDrawerOpen()) {
    const draft =
      (document.querySelector("[data-task-draft]") as HTMLInputElement | null)?.value ??
      state.tasksDraft;
    state.tasksDraft = draft;
    body.outerHTML = tasksPanelHTML();
  }
}

function nextTaskStatus(cur: TaskStatus): TaskStatus {
  if (cur === "todo") return "doing";
  if (cur === "doing") return "done";
  return "todo";
}

async function onTaskAdd(ev?: Event): Promise<void> {
  const form = (ev?.target as HTMLElement | null)?.closest?.("form") as HTMLFormElement | null;
  const input =
    (form?.querySelector("[data-task-draft], input[name='title']") as HTMLInputElement | null) ||
    (document.querySelector("[data-task-draft]") as HTMLInputElement | null);
  const title = (input?.value || state.tasksDraft || "").trim();
  if (!title) return;
  const cwd = toolsCwd();
  state.tasksBusy = true;
  try {
    const out = await createTask({ cwd, title });
    state.tasks = out.tasks || (out.task ? [out.task, ...state.tasks] : state.tasks);
    state.tasksDraft = "";
    if (input) input.value = "";
    paintTasksList();
    paintToolsStatus();
  } catch (e) {
    toast((e as Error).message || "add task failed", "err");
  } finally {
    state.tasksBusy = false;
  }
}

async function onTaskToggle(id: string, status: TaskStatus): Promise<void> {
  const next: TaskStatus = status === "done" ? "todo" : "done";
  await onTaskUpdate(id, next);
}

async function onTaskCycle(id: string, status: TaskStatus): Promise<void> {
  await onTaskUpdate(id, nextTaskStatus(status));
}

async function onTaskUpdate(id: string, status: TaskStatus): Promise<void> {
  const cwd = toolsCwd();
  state.tasksBusy = true;
  try {
    const out = await updateTask(id, { cwd, status });
    if (out.tasks) state.tasks = out.tasks;
    else {
      state.tasks = state.tasks.map((t) => (t.id === id ? { ...t, status } : t));
    }
    paintTasksList();
    paintToolsStatus();
  } catch (e) {
    toast((e as Error).message || "update task failed", "err");
  } finally {
    state.tasksBusy = false;
  }
}

async function onTaskDelete(id: string): Promise<void> {
  const cwd = toolsCwd();
  state.tasksBusy = true;
  try {
    const out = await deleteTask(id, cwd);
    state.tasks = out.tasks || state.tasks.filter((t) => t.id !== id);
    paintTasksList();
    paintToolsStatus();
  } catch (e) {
    toast((e as Error).message || "delete task failed", "err");
  } finally {
    state.tasksBusy = false;
  }
}

async function showRecordingsDrawer(): Promise<void> {
  try {
    const out = await listRecordings({ limit: 40 });
    if (out.enabled === false) {
      toast("Recording disabled — set sessions.recording = true", "info", 5000);
    }
    const lines = (out.recordings || [])
      .map(
        (r) =>
          `${r.id}\n  sess=${r.session_id}  agent=${r.agent}  ${r.bytes}B  ${r.created_at}`,
      )
      .join("\n\n");
    state.drawer = { title: "Recordings", body: lines || "No recordings yet" };
    void paintDrawer();
  } catch (e) {
    toast((e as Error).message, "err");
  }
}

async function showHistorySearch(): Promise<void> {
  const q = window.prompt("Search session history for:");
  if (!q?.trim()) return;
  try {
    const out = await historySearch(q.trim());
    const lines = (out.hits || [])
      .map(
        (h) =>
          `${h.session_id}  ${h.agent}/${h.cwd}  [${h.source}]\n  ${h.snippet || ""}`,
      )
      .join("\n\n");
    state.drawer = {
      title: `History search: ${q}`,
      body: lines || "No hits",
    };
    void paintDrawer();
  } catch (e) {
    toast((e as Error).message, "err");
  }
}

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
    el.className = "app-toast toast";
    el.setAttribute("role", "status");
    el.setAttribute("aria-live", "polite");
    el.hidden = true;
    document.body.appendChild(el);
  }
  if (!state.toast) {
    if (!el.hidden) {
      await animateToastOut(el);
    }
    el.hidden = true;
    el.textContent = "";
    el.className = "app-toast toast";
    return;
  }
  el.hidden = false;
  el.innerHTML = `<span class="toast-msg">${esc(state.toast.msg)}</span>`;
  el.className = `app-toast toast toast-${state.toast.kind}`;
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
    // Non-critical: tools status can wait until the browser is idle.
    scheduleIdle(() => {
      void refreshToolsStatus();
    });
    startPolling();
    ensureRouterBound();
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
    // Deep-link / default route after shell is mounted
    if (state.token && state.shellMounted) {
      const route = currentRoute();
      // Normalize URL (e.g. /settings → /profile) without stacking history
      history.replaceState(route, "", serializeRoute(route));
      void applyRoute(route, { fromHistory: true });
    }
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
    // Poll path: only rewrite list/tabs/status when their display sigs change.
    paintChrome();
  } catch (e) {
    const err = e as Error & { status?: number };
    if (err.status === 401) {
      logout();
      return;
    }
    state.statusOk = false;
    state.statusText = err.message || "poll failed";
    paintStatusChrome();
  }
}

/** Schedule non-critical work when the browser is idle (tools status, etc.). */
function scheduleIdle(fn: () => void, timeout = 2000): void {
  const w = window as Window & {
    requestIdleCallback?: (cb: () => void, opts?: { timeout: number }) => number;
  };
  if (typeof w.requestIdleCallback === "function") {
    w.requestIdleCallback(fn, { timeout });
  } else {
    window.setTimeout(fn, 1);
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
  openTabOnly(s);
  syncUrlFromUI();
}

async function activateTab(id: string): Promise<void> {
  await activateTabOnly(id);
  // Close overlays when switching tabs via UI (keep tools if on tools route for same session)
  if (state.panel === "new" || state.panel === "help") {
    await closePanelOnly();
  }
  if (state.settingsOpen) await closeSettingsOnly();
  syncUrlFromUI();
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
    syncUrlFromUI({ replace: true });
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
    // Always binary frames so path-looking text never collides with JSON ctrl msgs.
    activePty?.write(
      typeof data === "string" ? new TextEncoder().encode(data) : data,
    );
  });
  ensureImagePasteBound();
  resizeObserver = new ResizeObserver(() => fit());
  resizeObserver.observe(container);
}

/** Document-level once — survives term-host innerHTML rebuilds. */
let imagePasteBound = false;

function ensureImagePasteBound(): void {
  if (imagePasteBound) return;
  imagePasteBound = true;

  // Capture phase so we beat xterm's text-only paste handler.
  document.addEventListener("paste", onDocumentImagePaste, true);
  document.addEventListener("dragover", onDocumentImageDragOver, true);
  document.addEventListener("dragleave", onDocumentImageDragLeave, true);
  document.addEventListener("drop", onDocumentImageDrop, true);
}

function termPasteTarget(ev: Event): boolean {
  const t = ev.target;
  if (!(t instanceof Node)) return false;
  if (term?.element?.contains(t)) return true;
  const host = document.getElementById("term-host");
  if (host?.contains(t)) return true;
  // Focused xterm helper textarea (sometimes event target is body on some browsers)
  const ae = document.activeElement;
  if (ae && host?.contains(ae)) return true;
  if (ae && term?.element?.contains(ae)) return true;
  return false;
}

function isImageFile(f: File): boolean {
  if (f.type && f.type.startsWith("image/")) return true;
  // Some OSes hand over clipboard images with empty MIME
  return /\.(png|jpe?g|gif|webp|bmp|svg|heic|avif|tiff?)$/i.test(f.name || "");
}

/** Pull image files from a paste/drop DataTransfer (items + files + html data URLs). */
function extractImagesFromDataTransfer(dt: DataTransfer | null | undefined): File[] {
  if (!dt) return [];
  const out: File[] = [];
  const seen = new Set<string>();

  const push = (f: File | null | undefined) => {
    if (!f || !isImageFile(f)) return;
    const key = `${f.type}|${f.size}|${f.name}|${f.lastModified}`;
    if (seen.has(key)) return;
    seen.add(key);
    out.push(f);
  };

  if (dt.files?.length) {
    for (const f of Array.from(dt.files)) push(f);
  }
  if (dt.items?.length) {
    for (const item of Array.from(dt.items)) {
      if (item.kind === "file") {
        push(item.getAsFile());
      }
    }
  }

  // Copy-from-browser often only has text/html with a data:image URL
  if (!out.length) {
    try {
      const html = dt.getData("text/html") || "";
      const m = html.match(
        /src\s*=\s*["'](data:image\/[a-zA-Z0-9.+-]+;base64,[A-Za-z0-9+/=\s]+)["']/i,
      );
      if (m?.[1]) {
        const dataUrl = m[1].replace(/\s+/g, "");
        const mime =
          dataUrl.match(/^data:(image\/[a-zA-Z0-9.+-]+);/i)?.[1] || "image/png";
        const b64 = dataUrl.split(",")[1] || "";
        const bin = atob(b64);
        const bytes = new Uint8Array(bin.length);
        for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
        const ext = mime.split("/")[1]?.replace("jpeg", "jpg") || "png";
        push(
          new File([bytes], `clipboard.${ext}`, {
            type: mime,
            lastModified: Date.now(),
          }),
        );
      }
    } catch {
      /* ignore html parse */
    }
  }

  return out;
}

function clipboardTypesHintImage(dt: DataTransfer | null | undefined): boolean {
  if (!dt) return false;
  const types = Array.from(dt.types || []);
  if (types.some((t) => t.startsWith("image/") || t === "Files")) return true;
  if (dt.items) {
    for (const item of Array.from(dt.items)) {
      if (item.type?.startsWith("image/")) return true;
      if (item.kind === "file") return true;
    }
  }
  return false;
}

async function readImagesFromClipboardAPI(): Promise<File[]> {
  // Requires secure context + permission; paste gesture usually grants it.
  const nav = navigator as Navigator & {
    clipboard?: Clipboard & {
      read?: () => Promise<ClipboardItem[]>;
    };
  };
  if (!nav.clipboard?.read) return [];
  try {
    const items = await nav.clipboard.read();
    const files: File[] = [];
    for (const item of items) {
      for (const type of item.types) {
        if (!type.startsWith("image/")) continue;
        const blob = await item.getType(type);
        const ext = type.split("/")[1]?.replace("jpeg", "jpg") || "png";
        files.push(
          new File([blob], `clipboard.${ext}`, {
            type,
            lastModified: Date.now(),
          }),
        );
      }
    }
    return files;
  } catch {
    return [];
  }
}

async function ingestImageFiles(files: File[]): Promise<void> {
  if (!files.length) return;
  if (!state.activeId) {
    toast("Open a session first to paste images", "info");
    return;
  }
  for (const file of files) {
    await pasteImageFile(file);
  }
}

function onDocumentImagePaste(ev: ClipboardEvent): void {
  if (!state.activeId || !state.token) return;
  if (!termPasteTarget(ev)) return;

  const dt = ev.clipboardData;
  const images = extractImagesFromDataTransfer(dt);
  if (images.length) {
    ev.preventDefault();
    ev.stopPropagation();
    ev.stopImmediatePropagation?.();
    void ingestImageFiles(images);
    return;
  }

  // Image-only clipboard: some browsers expose types but empty files until async read.
  const plain = (dt?.getData("text/plain") || "").trim();
  if (!plain && clipboardTypesHintImage(dt)) {
    ev.preventDefault();
    ev.stopPropagation();
    ev.stopImmediatePropagation?.();
    void (async () => {
      const asyncImgs = await readImagesFromClipboardAPI();
      if (asyncImgs.length) {
        await ingestImageFiles(asyncImgs);
      } else {
        toast("Clipboard has an image but the browser blocked access", "err");
      }
    })();
    return;
  }

  // Last resort: empty text paste (macOS screenshot clipboard, some Wayland paths).
  // Try async clipboard.read(); only claim the event if an image is found.
  if (!plain) {
    void (async () => {
      const asyncImgs = await readImagesFromClipboardAPI();
      if (!asyncImgs.length) return;
      await ingestImageFiles(asyncImgs);
    })();
  }
}

function onDocumentImageDragOver(ev: DragEvent): void {
  if (!state.activeId) return;
  const host = document.getElementById("term-host");
  if (!host) return;
  const overHost =
    ev.target instanceof Node &&
    (host.contains(ev.target) || term?.element?.contains(ev.target));
  if (!overHost) return;
  if (!ev.dataTransfer) return;
  const types = Array.from(ev.dataTransfer.types || []);
  if (!types.includes("Files") && !types.some((t) => t.startsWith("image/"))) {
    return;
  }
  ev.preventDefault();
  ev.stopPropagation();
  try {
    ev.dataTransfer.dropEffect = "copy";
  } catch {
    /* ignore */
  }
  host.classList.add("term-drop-target");
}

function onDocumentImageDragLeave(ev: DragEvent): void {
  const host = document.getElementById("term-host");
  if (!host) return;
  // Only clear when leaving the host entirely
  if (ev.target === host || (ev.relatedTarget instanceof Node && !host.contains(ev.relatedTarget))) {
    host.classList.remove("term-drop-target");
  }
}

function onDocumentImageDrop(ev: DragEvent): void {
  const host = document.getElementById("term-host");
  host?.classList.remove("term-drop-target");
  if (!state.activeId) return;
  if (!termPasteTarget(ev) && !(ev.target instanceof Node && host?.contains(ev.target))) {
    return;
  }
  const images = extractImagesFromDataTransfer(ev.dataTransfer);
  if (!images.length) return;
  ev.preventDefault();
  ev.stopPropagation();
  void ingestImageFiles(images);
}

function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result || ""));
    r.onerror = () => reject(r.error || new Error("read failed"));
    r.readAsDataURL(file);
  });
}

function shellQuote(path: string): string {
  if (!/[\s'"\\$`!]/.test(path)) return path;
  return `'${path.replace(/'/g, `'\\''`)}'`;
}

async function pasteImageFile(file: File): Promise<void> {
  if (!state.activeId) {
    toast("Open a session first to paste images", "info");
    return;
  }
  // Prefer live session cwd (relative) so pathallow accepts it
  const live = state.sessions.find((s) => s.id === state.activeId);
  const tab = state.openTabs.find((t) => t.id === state.activeId);
  const cwd = live?.cwd || tab?.cwd || state.formCwd || state.defaultCwd || ".";
  try {
    const kb = Math.max(1, Math.round(file.size / 1024));
    toast(`Uploading image (${kb} KB)…`, "info", 8000);
    const dataUrl = await fileToDataURL(file);
    if (!dataUrl.startsWith("data:image") && !dataUrl.startsWith("data:application/octet")) {
      // Still try — server normalizes MIME from body.mime
    }
    const out = await uploadImage({
      cwd,
      session_id: state.activeId,
      mime: file.type || "image/png",
      data: dataUrl,
    });
    // Prefer absolute path — unambiguous for agent Read tools
    const path = out.paste || out.abs || out.cwd_rel;
    if (!path) {
      toast("Upload succeeded but no path returned", "err");
      return;
    }
    const token = shellQuote(path);
    // Binary frame into PTY (same as typed keys after encode)
    activePty?.write(new TextEncoder().encode(token));
    term?.focus();
    toast(`Image saved → ${out.cwd_rel || path}`, "ok", 4500);
  } catch (e) {
    toast((e as Error).message || "image paste failed", "err");
  }
}

/** Hidden file picker fallback when OS clipboard won't hand over images. */
function openImageFilePicker(): void {
  if (!state.activeId) {
    toast("Open a session first to paste images", "info");
    return;
  }
  const input = document.createElement("input");
  input.type = "file";
  input.accept = "image/*";
  input.multiple = true;
  input.style.display = "none";
  document.body.appendChild(input);
  input.addEventListener("change", () => {
    const files = input.files ? Array.from(input.files).filter(isImageFile) : [];
    input.remove();
    if (files.length) void ingestImageFiles(files);
  });
  input.click();
  // Cleanup if user cancels (no change event)
  window.setTimeout(() => {
    if (input.isConnected) input.remove();
  }, 60_000);
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
  const label = labels[state.conn];
  el.title = `PTY ${label}`;
  el.innerHTML = `<span class="conn-dot" aria-hidden="true"></span><span class="conn-label">${label}</span>`;
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
        <button type="button" class="ghost btn-sm" id="btn-refit" title="Refit terminal (f)">Fit</button>
        <button type="button" class="ghost btn-sm" id="btn-paste-image" title="Upload image into session">Image</button>
        <details class="menu term-menu">
          <summary class="ghost btn-sm" title="Session actions">More</summary>
          <div class="menu-panel">
            <button type="button" id="btn-kill">Stop agent</button>
            <button type="button" class="danger-text" id="btn-delete">Delete session</button>
          </div>
        </details>
      </div>`;
    document.getElementById("btn-refit")?.addEventListener("click", () => fit());
    document.getElementById("btn-paste-image")?.addEventListener("click", () => openImageFilePicker());
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
        <h2 class="term-empty-title">No session attached</h2>
        <p class="term-empty-desc">Pick a session from the sidebar, or start something new. Closing a tab only detaches — agents keep running in tmux.</p>
        <div class="term-empty-actions">
          <button type="button" class="primary" data-action="new-session">${iconSvg("plus")} New session</button>
          <button type="button" class="ghost" data-action="open-shell">${iconSvg("terminal")} Terminal</button>
          <button type="button" class="ghost" data-action="open-remote">${iconSvg("external")} Open remote</button>
          <button type="button" class="ghost" data-action="git-changes">${iconSvg("git-branch")} Git changes</button>
        </div>
        <p class="term-empty-hint">
          <kbd>n</kbd> new · <kbd>⌘</kbd><kbd>K</kbd> commands · <kbd>?</kbd> help
        </p>
      </div>
    </div>`;
}

function openPanel(p: Panel): void {
  if (!p) return;
  openPanelOnly(p);
  // Route for this panel (session-scoped tools when a tab is active)
  if (p === "new") navigate({ name: "new" }, { apply: false });
  else if (p === "new-project") navigate({ name: "new-project" }, { apply: false });
  else if (p === "help") navigate({ name: "help" }, { apply: false });
  else if (p === "changes") navigate({ name: "changes" }, { apply: false });
  else if (p === "remote") navigate({ name: "remote" }, { apply: false });
  else if (p === "projects") navigate({ name: "projects" }, { apply: false });
  else if (p === "tools") {
    if (state.activeId) {
      const s =
        state.sessions.find((x) => x.id === state.activeId) ||
        state.openTabs.find((x) => x.id === state.activeId);
      if (s) {
        navigate(
          {
            name: "session",
            projectId: projectIdFromCwd(s.cwd),
            sessionId: s.id,
            tools: true,
          },
          { apply: false },
        );
        return;
      }
    }
    navigate({ name: "tools" }, { apply: false });
  }
}

async function closePanel(): Promise<void> {
  await closePanelOnly();
  term?.focus();
  // Drop modal from URL → session or desk (URL only; UI already closed)
  if (state.activeId) {
    const s =
      state.sessions.find((x) => x.id === state.activeId) ||
      state.openTabs.find((x) => x.id === state.activeId);
    if (s) {
      navigate(
        {
          name: "session",
          projectId: projectIdFromCwd(s.cwd),
          sessionId: s.id,
        },
        { replace: true, apply: false },
      );
      return;
    }
  }
  navigate({ name: "desk" }, { replace: true, apply: false });
}

function openSettings(tab?: SettingsTab): void {
  openSettingsOnly(tab);
  navigate(
    { name: "profile", tab: tab || state.settingsTab || "accounts" },
    { apply: false },
  );
}

async function closeSettings(): Promise<void> {
  await closeSettingsOnly();
  term?.focus();
  if (state.activeId) {
    const s =
      state.sessions.find((x) => x.id === state.activeId) ||
      state.openTabs.find((x) => x.id === state.activeId);
    if (s) {
      navigate(
        {
          name: "session",
          projectId: projectIdFromCwd(s.cwd),
          sessionId: s.id,
        },
        { replace: true, apply: false },
      );
      return;
    }
  }
  navigate({ name: "desk" }, { replace: true, apply: false });
}

async function loadSettingsData(): Promise<void> {
  const tasks: Promise<void>[] = [];
  if (state.settingsTab === "accounts") {
    tasks.push(refreshSettingsAccounts());
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
  // Drop leftover motion inline styles from a previous enter/exit so repaints stay visible.
  root.style.opacity = "";
  root.style.transform = "";
  if (opts?.animateIn || wasMissing) {
    void animateSettingsIn(root);
  }
}

function settingsHTML(): string {
  const tabs: { id: SettingsTab; label: string; hint: string; icon: string }[] = [
    { id: "accounts", label: "Agent accounts", hint: "Cursor · Claude · Grok · Codex", icon: "users" },
    { id: "github", label: "GitHub", hint: "gh CLI logins", icon: "github" },
    { id: "ssh", label: "SSH keys", hint: "Host identities", icon: "key" },
    { id: "workspace", label: "Workspace", hint: "Map · memory · browser", icon: "layers" },
    { id: "about", label: "About", hint: "Host & shortcuts", icon: "info" },
  ];
  const nav = tabs
    .map((t) => {
      const href = t.id === "accounts" ? "/profile" : `/profile/${t.id}`;
      const active = state.settingsTab === t.id;
      return `
      <a href="${href}" class="settings-nav-item ${active ? "active" : ""}" data-action="settings-tab" data-tab="${t.id}" data-nav${active ? ' aria-current="page"' : ""}>
        <span class="settings-nav-ico" aria-hidden="true">${iconSvg(t.icon)}</span>
        <span class="settings-nav-text">
          <span class="settings-nav-label">${esc(t.label)}</span>
          <span class="settings-nav-hint">${esc(t.hint)}</span>
        </span>
      </a>`;
    })
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

  const tabMeta = tabs.find((t) => t.id === state.settingsTab);
  return `
    <div class="settings-shell" role="dialog" aria-modal="true" aria-labelledby="settings-title" data-sidebar="08">
      <aside class="settings-nav">
        <div class="settings-nav-head">
          <div class="eyebrow">Host</div>
          <h1 id="settings-title">Settings</h1>
          <p class="settings-nav-sub">Profiles, keys &amp; host tools</p>
        </div>
        <nav class="settings-nav-list" aria-label="Settings sections">${nav}</nav>
        <div class="settings-nav-foot">
          <button type="button" class="ghost settings-nav-close" data-action="close-settings">${iconSvg("panel")} Back to desk</button>
        </div>
      </aside>
      <section class="settings-main">
        <header class="settings-main-head">
          <div class="settings-main-titles">
            <h2>${esc(tabMeta?.label || "Settings")}</h2>
            ${tabMeta?.hint ? `<p class="settings-main-hint">${esc(tabMeta.hint)}</p>` : ""}
          </div>
          <button type="button" class="ghost btn-icon sm" data-action="close-settings" title="Close (Esc)" aria-label="Close">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>
          </button>
        </header>
        <div class="settings-body">${body}</div>
      </section>
    </div>`;
}

async function refreshSettingsAccounts(): Promise<void> {
  try {
    const out = await listAllAgentAccounts();
    state.settingsPlatforms = out.platforms || [];
    state.settingsAccountsBin = out.bin || "";
    state.settingsAccountsError = "";
    if (
      !state.settingsPlatforms.find((p) => p.platform === state.settingsPlatform)
    ) {
      // Prefer a platform that has a live login
      const withLive =
        state.settingsPlatforms.find(
          (p) => p.current && p.current !== "not signed in",
        ) || state.settingsPlatforms[0];
      state.settingsPlatform = withLive?.platform || "grok";
    }
  } catch (e) {
    state.settingsPlatforms = [];
    state.settingsAccountsBin = "";
    state.settingsAccountsError =
      (e as Error).message || "Failed to load accounts";
    toast(state.settingsAccountsError, "err");
  }
}

function platformLabel(p: string): string {
  switch (p) {
    case "cursor":
      return "Cursor";
    case "claude":
      return "Claude";
    case "codex":
      return "Codex";
    case "grok":
      return "Grok";
    case "vscode":
      return "VS Code / Copilot";
    default:
      return p;
  }
}

function formatSavedAt(iso?: string): string {
  if (!iso) return "";
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  try {
    return new Date(t).toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

function settingsAccountsHTML(): string {
  if (state.settingsAccountsError && !state.settingsPlatforms.length) {
    return `
      <div class="settings-empty settings-empty--error" role="alert">
        <p class="settings-empty-title">Could not load accounts</p>
        <p class="settings-empty-desc">${esc(state.settingsAccountsError)}</p>
        <p class="settings-empty-desc">Install <code>cursor-switch</code> from
          <code>github.com/reloadlife/cursor-account-switcher</code> on this host.</p>
        <div class="settings-empty-actions">
          <button type="button" class="primary btn-sm" data-action="settings-refresh">Retry</button>
        </div>
      </div>`;
  }
  const plats = state.settingsPlatforms;
  if (!plats.length) {
    return `
      <div class="settings-empty">
        <p class="settings-empty-title">No platforms</p>
        <p class="settings-empty-desc"><code>cursor-switch</code> found but no platforms returned.</p>
        <div class="settings-empty-actions">
          <button type="button" class="primary btn-sm" data-action="settings-refresh">Retry</button>
        </div>
      </div>`;
  }

  const platTabs = plats
    .map((p) => {
      const liveRaw =
        p.current && p.current !== "not signed in" ? p.current : "";
      // Prefer email local-part; fall back to a short id, never a long UUID line.
      let live = "—";
      if (liveRaw.includes("@")) {
        live = liveRaw.split("@")[0] || "—";
      } else if (liveRaw) {
        live = liveRaw.length > 14 ? `${liveRaw.slice(0, 8)}…` : liveRaw;
      }
      const nSaved = (p.accounts || []).filter((a) => a.saved).length;
      return `<button type="button" class="chip chip-platform ${state.settingsPlatform === p.platform ? "active" : ""}" data-action="settings-platform" data-platform="${esc(p.platform)}" title="${esc(p.current || "not signed in")}">
        <span class="chip-name">${esc(platformLabel(p.platform))}</span>
        <span class="chip-meta">${esc(live)} · ${nSaved} saved</span>
      </button>`;
    })
    .join("");

  const cur =
    plats.find((p) => p.platform === state.settingsPlatform) || plats[0];
  const accounts = cur?.accounts || [];
  const rows = accounts
    .map((a) => {
      const isActive = !!(a.active || (cur.active && cur.active === a.id));
      const savedBadge = a.saved
        ? `<span class="badge running">saved</span>`
        : `<span class="badge">empty</span>`;
      const activeBadge = isActive
        ? `<span class="badge running">active</span>`
        : "";
      const switchBtn = isActive
        ? `<button type="button" class="primary btn-sm" disabled title="Already active">Active</button>`
        : a.saved
          ? `<button type="button" class="primary btn-sm" data-action="acct-switch" data-platform="${esc(cur.platform)}" data-id="${esc(a.id)}" title="Restore saved credentials host-wide">Switch</button>`
          : `<button type="button" class="primary btn-sm" data-action="acct-switch" data-platform="${esc(cur.platform)}" data-id="${esc(a.id)}" title="Clear live login so you can sign into a new account, then Save current">Use empty</button>`;
      return `
        <div class="settings-card acct-card ${isActive ? "acct-active" : ""} ${a.saved ? "acct-saved" : "acct-empty"}">
          <div class="settings-card-main">
            <div class="settings-card-title">
              <strong>${esc(a.label || a.id)}</strong>
              <code class="settings-id">${esc(a.id)}</code>
              ${savedBadge}${activeBadge}
            </div>
            <div class="settings-card-meta">
              ${
                a.email
                  ? `<span class="acct-email">${esc(a.email)}</span>`
                  : a.saved
                    ? `<span class="acct-email muted">Saved (no email detected)</span>`
                    : `<span class="acct-email muted">Empty — switch here to sign into a new account</span>`
              }
              ${a.saved_at ? ` · saved ${esc(formatSavedAt(a.saved_at))}` : ""}
            </div>
          </div>
          <div class="settings-card-actions settings-btn-group">
            ${switchBtn}
            <button type="button" class="ghost btn-sm" data-action="acct-save" data-platform="${esc(cur.platform)}" data-id="${esc(a.id)}" data-label="${esc(a.label || a.id)}" title="Capture live login into this slot">Save</button>
            <button type="button" class="ghost btn-sm danger-text" data-action="acct-remove" data-platform="${esc(cur.platform)}" data-id="${esc(a.id)}" title="Remove this slot">Remove</button>
          </div>
        </div>`;
    })
    .join("");

  const live = cur?.current || "—";
  const activeId = cur?.active || "—";

  return `
    <p class="settings-lede">
      Multi-account profiles via <code>cursor-switch</code>${state.settingsAccountsBin ? ` · <code class="pathish">${esc(state.settingsAccountsBin)}</code>` : ""}.
      <strong>Switch</strong> changes the host-wide login.
      New sessions default to <strong>isolated</strong> mode (private HOME per account — parallel-safe).
    </p>
    <div class="chip-row platform-chips" role="tablist" aria-label="Agent platforms">${platTabs}</div>
    <div class="settings-stat acct-live-bar">
      <div class="settings-stat-items">
        <span>Platform <strong>${esc(platformLabel(cur?.platform || ""))}</strong></span>
        <span>Live <strong title="${esc(live)}">${esc(live)}</strong></span>
        <span>Profile <strong>${esc(activeId)}</strong></span>
      </div>
      <button type="button" class="ghost btn-sm" data-action="settings-refresh" ${state.settingsBusy ? "disabled" : ""}>Refresh</button>
    </div>
    <div class="settings-cards acct-list">
      ${
        rows ||
        `<div class="settings-empty settings-empty--inline">
          <p class="settings-empty-title">No account slots</p>
          <p class="settings-empty-desc">Add a slot for ${esc(cur?.platform || "this platform")} below.</p>
        </div>`
      }
    </div>
    <div class="settings-form-card">
      <div class="settings-form-head">
        <h3>Add account slot</h3>
        <p class="form-hint">Registers a profile name. Log into the CLI as that user, then <strong>Save</strong>.</p>
      </div>
      <div class="field-grid">
        <div class="field">
          <label for="settings-acct-id">Id</label>
          <input id="settings-acct-id" value="${esc(state.settingsAcctId)}" placeholder="personal" autocomplete="off" ${state.settingsBusy ? "disabled" : ""} />
        </div>
        <div class="field">
          <label for="settings-acct-label">Label</label>
          <input id="settings-acct-label" value="${esc(state.settingsAcctLabel)}" placeholder="Personal" autocomplete="off" ${state.settingsBusy ? "disabled" : ""} />
        </div>
      </div>
      <div class="settings-form-actions">
        <button type="button" class="primary btn-sm" data-action="acct-add" ${state.settingsBusy ? "disabled" : ""}>${state.settingsBusy ? "…" : "Add account"}</button>
      </div>
    </div>
    <details class="details-block howto-card">
      <summary>Workflow tips</summary>
      <ol class="howto-list">
        <li>Add a slot (e.g. <code>work</code>).</li>
        <li>Click <strong>Use empty</strong> on that slot — clears the live login so you can sign in as a new user.</li>
        <li>On the host, log into the CLI for that tool as the new account.</li>
        <li><strong>Save</strong> on the slot (captures credentials).</li>
        <li>Later: <strong>Switch</strong> restores a saved profile; or pick the account when starting a session (isolated = parallel-safe).</li>
      </ol>
    </details>`;
}

function settingsGitHubHTML(): string {
  return `
    <p class="settings-lede">GitHub CLI accounts on this host. Tokens are write-only — never shown back.</p>
    <div class="settings-stat">
      <div class="settings-stat-items">
        <span>Active <strong id="gh-active">${esc(state.ghStatus?.active || "—")}</strong></span>
      </div>
    </div>
    <div id="gh-account-list" class="gh-account-list settings-stack">${ghAccountsHTML()}</div>
    <div class="settings-form-card">
      <div class="settings-form-head">
        <h3>Login with token</h3>
        <p class="form-hint">Paste a PAT or fine-grained token. Stored only on the host via <code>gh</code>.</p>
      </div>
      <div class="field">
        <label for="gh-login-token" class="sr-only">Token</label>
        <input id="gh-login-token" type="password" placeholder="ghp_… or github_pat_…" value="" autocomplete="off" />
      </div>
      <div class="settings-form-actions">
        <button type="button" class="ghost btn-sm" data-action="gh-setup-git">Setup git</button>
        <button type="button" class="primary btn-sm" data-action="gh-login" ${state.ghBusy ? "disabled" : ""}>${state.ghBusy ? "…" : "Login"}</button>
      </div>
    </div>`;
}

function settingsSSHHTML(): string {
  return `
    <p class="settings-lede">SSH identities under the agents host home. Public keys only — private keys never leave the server.</p>
    <div class="settings-stat">
      <div class="settings-stat-items">
        <span>Dir <code id="ssh-dir">${esc(state.sshDir || "—")}</code></span>
      </div>
    </div>
    <div class="settings-form-card">
      <div class="settings-form-head">
        <h3>Generate key</h3>
        <p class="form-hint">Creates an ed25519 key pair in the host <code>~/.ssh</code> directory.</p>
      </div>
      <div class="field-grid">
        <div class="field">
          <label for="ssh-gen-name">Name</label>
          <input id="ssh-gen-name" placeholder="id_github" value="${esc(state.sshGenName)}" autocomplete="off" />
        </div>
        <div class="field">
          <label for="ssh-gen-comment">Comment</label>
          <input id="ssh-gen-comment" placeholder="optional" value="${esc(state.sshGenComment)}" autocomplete="off" />
        </div>
      </div>
      <div class="settings-form-actions">
        <button type="button" class="primary btn-sm" data-action="ssh-gen" ${state.sshBusy ? "disabled" : ""}>${state.sshBusy ? "…" : "Generate"}</button>
      </div>
    </div>
    <div id="ssh-key-list" class="ssh-key-list settings-stack">${sshKeysHTML()}</div>`;
}

function settingsWorkspaceHTML(): string {
  return `
    <p class="settings-lede">Context pack for faster agent orientation. Ensured automatically on new sessions.</p>
    ${workspaceToolsBodyHTML({ settings: true })}`;
}

function settingsAboutHTML(): string {
  return `
    <div class="settings-about">
      <div class="settings-stat">
        <div class="settings-stat-items">
          <span>Status <strong>${esc(state.statusText)}</strong></span>
          <span>API <strong class="${state.statusOk ? "ok-text" : "danger-text"}">${state.statusOk ? "ok" : "error"}</strong></span>
        </div>
      </div>
      <div class="settings-form-card">
        <div class="settings-form-head">
          <h3>Appearance</h3>
          <p class="form-hint">Theme applies to this browser only.</p>
        </div>
        <div class="settings-form-actions">
          <button type="button" class="primary btn-sm" data-action="toggle-theme">
            Switch to ${state.theme === "dark" ? "light" : "dark"} theme
          </button>
        </div>
      </div>
      <div class="settings-form-card">
        <div class="settings-form-head">
          <h3>Keyboard</h3>
          <p class="form-hint">Bare keys outside the terminal · <kbd>Alt</kbd>+key while focused · <kbd>⌘</kbd><kbd>K</kbd> palette</p>
        </div>
        <dl class="keys">
          <div><dt><kbd>j</kbd><kbd>k</kbd> · <kbd>⇧</kbd><kbd>j</kbd><kbd>k</kbd></dt><dd>List · step+open</dd></div>
          <div><dt><kbd>h</kbd><kbd>l</kbd> · <kbd>Ctrl</kbd><kbd>Tab</kbd></dt><dd>Prev/next tab</dd></div>
          <div><dt><kbd>1</kbd>–<kbd>9</kbd></dt><dd>Jump tab</dd></div>
          <div><dt><kbd>n</kbd> · <kbd>⇧</kbd><kbd>n</kbd></dt><dd>New session / project</dd></div>
          <div><dt><kbd>y</kbd></dt><dd>Copy session id</dd></div>
          <div><dt><kbd>?</kbd></dt><dd>Full shortcuts</dd></div>
        </dl>
        <div class="settings-form-actions">
          <button type="button" class="ghost btn-sm" data-action="help">Open shortcuts sheet</button>
        </div>
      </div>
      <div class="settings-form-card">
        <div class="settings-form-head">
          <h3>Security</h3>
          <p class="form-hint">Bearer token is full host control. SSH private keys and GitHub tokens are never returned by the API.</p>
        </div>
      </div>
    </div>`;
}

function patchPlatformStatus(platform: string, st?: AgentPlatformStatus): void {
  if (!st) return;
  const i = state.settingsPlatforms.findIndex((p) => p.platform === platform);
  if (i >= 0) state.settingsPlatforms[i] = st;
  else state.settingsPlatforms.push(st);
}

async function onAcctSave(platform: string, id: string, label: string): Promise<void> {
  if (state.settingsBusy) return;
  state.settingsBusy = true;
  paintSettings();
  try {
    toast(`Saving live ${platform} login → ${id}…`, "info", 12000);
    const out = await saveAgentAccount({ platform, id, label: label || undefined });
    patchPlatformStatus(platform, out.status);
    toast(`Saved ${platform}/${id}`, "ok");
    await refreshSettingsAccounts();
  } catch (e) {
    toast((e as Error).message || "save failed", "err", 8000);
  } finally {
    state.settingsBusy = false;
    paintSettings();
  }
}

async function onAcctSwitch(platform: string, id: string): Promise<void> {
  if (state.settingsBusy) return;
  const plat = state.settingsPlatforms.find((p) => p.platform === platform);
  const acc = plat?.accounts?.find((a) => a.id === id);
  const empty = acc ? !acc.saved : false;
  const msg = empty
    ? `Use empty profile “${id}” for ${platformLabel(platform)}?\n\nClears the live login so you can sign into a new account on the host, then click Save current.`
    : `Switch host-wide ${platformLabel(platform)} login to “${id}”?\n\nRestores saved credentials for that tool on this host.`;
  if (!confirm(msg)) return;
  state.settingsBusy = true;
  paintSettings();
  try {
    toast(empty ? `Preparing empty ${platform}/${id}…` : `Switching ${platform} → ${id}…`, "info", 15000);
    const out = await switchAgentAccount({ platform, id });
    patchPlatformStatus(platform, out.status);
    toast(
      empty
        ? `Empty “${id}” active — sign into ${platformLabel(platform)}, then Save current`
        : `Switched ${platformLabel(platform)} → ${id}`,
      "ok",
      empty ? 8000 : 3000,
    );
    await refreshSettingsAccounts();
  } catch (e) {
    toast((e as Error).message || "switch failed", "err", 8000);
  } finally {
    state.settingsBusy = false;
    paintSettings();
  }
}

async function onAcctAdd(): Promise<void> {
  if (state.settingsBusy) return;
  const idEl = document.getElementById("settings-acct-id") as HTMLInputElement | null;
  const labelEl = document.getElementById("settings-acct-label") as HTMLInputElement | null;
  const id = (idEl?.value || "").trim().toLowerCase().replace(/\s+/g, "-");
  const label = (labelEl?.value || "").trim() || id;
  if (!id) {
    toast("Account id required", "err");
    return;
  }
  if (!/^[a-z0-9][a-z0-9_-]{0,31}$/.test(id)) {
    toast("Id: start with letter/number, use a-z 0-9 _ -", "err");
    return;
  }
  state.settingsBusy = true;
  paintSettings();
  try {
    const out = await addAgentAccount({
      platform: state.settingsPlatform,
      id,
      label,
    });
    patchPlatformStatus(state.settingsPlatform, out.status);
    toast(`Added ${id}`, "ok");
    state.settingsAcctId = "";
    state.settingsAcctLabel = "";
    await refreshSettingsAccounts();
  } catch (e) {
    toast((e as Error).message || "add failed", "err", 8000);
  } finally {
    state.settingsBusy = false;
    paintSettings();
  }
}

async function onAcctRemove(platform: string, id: string): Promise<void> {
  if (state.settingsBusy) return;
  if (
    !confirm(
      `Remove “${id}” from ${platformLabel(platform)}?\n\nDeletes the saved profile credentials for this slot.`,
    )
  ) {
    return;
  }
  state.settingsBusy = true;
  paintSettings();
  try {
    const out = await removeAgentAccount({ platform, id });
    patchPlatformStatus(platform, out.status);
    toast(`Removed ${platform}/${id}`, "ok");
    await refreshSettingsAccounts();
  } catch (e) {
    toast((e as Error).message || "remove failed", "err", 8000);
  } finally {
    state.settingsBusy = false;
    paintSettings();
  }
}

function readCreateForm(): {
  agent: string;
  cwd: string;
  cwdNew: boolean;
  cwdNewName: string;
  name: string;
  prompt: string;
  gitUrl: string;
  gitName: string;
  gitBranch: string;
  gitFork: boolean;
  gitDepth: boolean;
  account: string;
  accountMode: string;
  worktree: boolean;
} {
  const agentEl = document.getElementById("sess-agent") as HTMLSelectElement | null;
  const cwdEl = document.getElementById("sess-cwd") as HTMLSelectElement | null;
  const cwdNewNameEl = document.getElementById("sess-cwd-new-name") as HTMLInputElement | null;
  const cwdModeEl = document.querySelector(
    'input[name="sess-cwd-mode"]:checked',
  ) as HTMLInputElement | null;
  const nameEl = document.getElementById("sess-name") as HTMLInputElement | null;
  const promptEl = document.getElementById("sess-prompt") as HTMLTextAreaElement | null;
  const gitUrlEl = document.getElementById("sess-git-url") as HTMLInputElement | null;
  const gitNameEl = document.getElementById("sess-git-name") as HTMLInputElement | null;
  const gitBranchEl = document.getElementById("sess-git-branch") as HTMLInputElement | null;
  const gitForkEl = document.getElementById("sess-git-fork") as HTMLInputElement | null;
  const gitDepthEl = document.getElementById("sess-git-depth") as HTMLInputElement | null;
  const worktreeEl = document.getElementById("sess-worktree") as HTMLInputElement | null;
  const accountEl = document.getElementById("sess-account") as HTMLSelectElement | null;
  const modeEl = document.querySelector(
    'input[name="sess-account-mode"]:checked',
  ) as HTMLInputElement | null;
  const agent = (agentEl?.value || state.formAgent || state.agents[0] || "claude").trim();
  const cwdNew = (cwdModeEl?.value || (state.formCwdNew ? "new" : "existing")) === "new";
  const cwdNewName = (cwdNewNameEl?.value ?? state.formCwdNewName).trim().replace(/^\/+|\/+$/g, "");
  const cwdExisting = (cwdEl?.value || state.formCwd || state.defaultCwd || ".").trim();
  const cwd = cwdNew ? cwdNewName : cwdExisting;
  const name = (nameEl?.value ?? state.formName).trim();
  const prompt = (promptEl?.value ?? state.formPrompt).trim();
  const gitUrl = (gitUrlEl?.value ?? state.formGitUrl).trim();
  const gitName = (gitNameEl?.value ?? state.formGitName).trim();
  const gitBranch = (gitBranchEl?.value ?? state.formGitBranch).trim();
  const gitFork = gitForkEl ? gitForkEl.checked : state.formGitFork;
  const gitDepth = gitDepthEl ? gitDepthEl.checked : state.formGitDepth;
  const worktree = worktreeEl ? worktreeEl.checked : false;
  const account = (accountEl?.value ?? state.formAccount).trim();
  const accountMode = (modeEl?.value || state.formAccountMode || "isolated").trim();
  state.formAgent = agent;
  state.formCwdNew = cwdNew;
  state.formCwdNewName = cwdNewName;
  if (!cwdNew) state.formCwd = cwdExisting;
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
    cwdNew,
    cwdNewName,
    name,
    prompt,
    gitUrl,
    gitName,
    gitBranch,
    gitFork,
    gitDepth,
    account,
    accountMode,
    worktree,
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
  const opts = [`<option value="">(host default — no profile)</option>`];
  for (const a of state.agentAccounts) {
    const bits = [a.label || a.id];
    if (a.email) bits.push(a.email);
    if (a.active) bits.push("active");
    if (!a.saved) bits.push("not saved");
    const sel = a.id === state.formAccount ? "selected" : "";
    opts.push(
      `<option value="${esc(a.id)}" ${sel} ${a.saved ? "" : "disabled"}>${esc(bits.join(" · "))}</option>`,
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
  if (form.cwdNew) {
    const n = (form.cwdNewName || "").trim();
    if (!n) {
      state.createError = "Enter a folder name for the new directory";
      paintPanel();
      return;
    }
    if (n.includes("..") || n.startsWith("/") || n.startsWith(".")) {
      state.createError = "Folder name must be relative (no .., absolute, or hidden segments)";
      paintPanel();
      return;
    }
  } else if (!form.cwd) {
    state.createError = "Pick a workspace";
    paintPanel();
    return;
  }
  state.creating = true;
  state.createError = "";
  paintPanel();
  try {
    let cwd = form.cwd || ".";
    if (form.cwdNew) {
      const created = await createWorkspace({ name: form.cwdNewName });
      cwd = created.cwd || created.workspace?.path || form.cwdNewName;
      // Refresh workspace list so the new dir shows up next time
      try {
        const wsRes = await listWorkspaces();
        state.workspaces = normalizeWorkspacePaths(wsRes);
      } catch {
        if (!state.workspaces.includes(cwd)) state.workspaces = [...state.workspaces, cwd];
      }
      state.formCwd = cwd;
      state.formCwdNew = false;
      state.formCwdNewName = "";
    }
    const sess = await createSession({
      agent: form.agent,
      cwd,
      name: form.name || undefined,
      prompt: form.prompt || undefined,
      account: form.account || undefined,
      account_mode: form.account ? form.accountMode || "isolated" : undefined,
      worktree: form.worktree || undefined,
    });
    state.formName = "";
    state.formPrompt = "";
    state.creating = false;
    await closePanelOnly();
    await refreshSessions();
    openTab(sess); // sets /project/.../session/...
    const accNote = form.account
      ? ` · account ${form.account} (${form.accountMode || "isolated"})`
      : "";
    const wtNote = form.worktree || sess.worktree ? " · worktree" : "";
    const dirNote = form.cwdNew ? " · new dir" : "";
    toast(`Started ${sess.agent} in ${sess.cwd || cwd}${dirNote}${accNote}${wtNote}`, "ok");
  } catch (e) {
    state.creating = false;
    const msg = (e as Error).message || "create failed";
    state.createError = msg;
    paintPanel();
    toast(msg, "err", 6000);
  }
}

function readProjectForm(): {
  gitUrl: string;
  gitName: string;
  gitBranch: string;
  gitFork: boolean;
  gitDepth: boolean;
  startSession: boolean;
} {
  const urlEl = document.getElementById("proj-git-url") as HTMLInputElement | null;
  const nameEl = document.getElementById("proj-git-name") as HTMLInputElement | null;
  const branchEl = document.getElementById("proj-git-branch") as HTMLInputElement | null;
  const forkEl = document.getElementById("proj-git-fork") as HTMLInputElement | null;
  const depthEl = document.getElementById("proj-git-depth") as HTMLInputElement | null;
  const startEl = document.getElementById("proj-start-session") as HTMLInputElement | null;
  const gitUrl = (urlEl?.value ?? state.formGitUrl).trim();
  const gitName = (nameEl?.value ?? state.formGitName).trim();
  const gitBranch = (branchEl?.value ?? state.formGitBranch).trim();
  const gitFork = forkEl ? forkEl.checked : state.formGitFork;
  const gitDepth = depthEl ? depthEl.checked : state.formGitDepth;
  const startSession = startEl ? startEl.checked : true;
  state.formGitUrl = gitUrl;
  state.formGitName = gitName;
  state.formGitBranch = gitBranch;
  state.formGitFork = gitFork;
  state.formGitDepth = gitDepth;
  return { gitUrl, gitName, gitBranch, gitFork, gitDepth, startSession };
}

async function onCreateProject(ev?: Event): Promise<void> {
  ev?.preventDefault();
  if (state.creating) {
    toast("Already cloning…", "info", 1500);
    return;
  }
  const form = readProjectForm();
  if (!form.gitUrl) {
    state.createError = "Repo URL or owner/repo is required";
    paintPanel();
    return;
  }
  state.creating = true;
  state.createError = "";
  paintPanel();
  try {
    toast(form.gitFork ? "Forking & cloning…" : "Cloning project…", "info", 60000);
    const cloned = await cloneWorkspace({
      url: form.gitUrl,
      name: form.gitName || undefined,
      branch: form.gitBranch || undefined,
      fork: form.gitFork || undefined,
      depth: form.gitDepth ? 1 : undefined,
    });
    const cwd = cloned.cwd;
    state.formCwd = cwd;
    state.formGitUrl = "";
    state.formGitName = "";
    state.formGitBranch = "";
    state.formGitFork = false;
    state.creating = false;
    await refreshWorkspaceList();
    toast(`Project ready → ${cwd}`, "ok", 3500);
    if (form.startSession) {
      // Switch to new-session modal with the new workspace selected
      openPanelOnly("new");
      navigate({ name: "new" }, { apply: false, replace: true });
      window.requestAnimationFrame(() => {
        const name = document.getElementById("sess-name") as HTMLInputElement | null;
        name?.focus();
      });
    } else {
      await closePanelOnly();
      navigate({ name: "desk" }, { replace: true, apply: false });
      paintChrome();
    }
  } catch (e) {
    state.creating = false;
    const msg = (e as Error).message || "clone failed";
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
      loadTasks(cwd),
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
  const open = state.tasks.filter((t) => t.status !== "done").length;
  setStatusText("tasks", state.tasks.length ? `${open} open · ${state.tasks.length}` : "none");
  paintTasksList();
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
    return `<div class="settings-empty settings-empty--inline">
      <p class="settings-empty-title">No SSH keys</p>
      <p class="settings-empty-desc">No keys in server <code>~/.ssh</code> yet. Generate one above.</p>
    </div>`;
  }
  return state.sshKeys
    .map((k) => {
      const kind = k.has_private && k.has_public ? "pair" : k.has_public ? "pub" : "priv";
      const fp = k.fingerprint ? esc(k.fingerprint) : "";
      return `<div class="ssh-key-row settings-list-row">
        <div class="ssh-key-main">
          <div class="ssh-key-name">
            <code class="settings-id">${esc(k.name)}</code>
            <span class="badge">${esc(k.type || "?")}</span>
            <span class="badge">${esc(kind)}</span>
          </div>
          ${fp ? `<div class="ssh-key-fp" title="${fp}">${fp}</div>` : ""}
          ${k.comment ? `<div class="ssh-key-comment">${esc(k.comment)}</div>` : ""}
        </div>
        <div class="ssh-key-actions settings-btn-group">
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
  if (!st) {
    return `<div class="settings-empty settings-empty--inline">
      <p class="settings-empty-desc">Loading accounts…</p>
    </div>`;
  }
  if (!st.accounts?.length) {
    return `<div class="settings-empty settings-empty--inline">
      <p class="settings-empty-title">No GitHub accounts</p>
      <p class="settings-empty-desc">${esc(st.error || "No GitHub accounts logged in on the server. Login with a token below.")}</p>
    </div>`;
  }
  return st.accounts
    .map((a: GHAccount) => {
      const active = a.active ? "active" : "";
      const scopes = (a.scopes || []).join(", ");
      return `<div class="gh-row settings-list-row ${active}">
        <div class="gh-main">
          <div class="gh-user">
            <strong>${esc(a.user)}</strong>
            ${a.active ? `<span class="badge running">active</span>` : ""}
            <span class="gh-host">${esc(a.host)}</span>
          </div>
          <div class="gh-meta">proto ${esc(a.git_protocol || "—")}${scopes ? ` · ${esc(scopes)}` : ""}</div>
        </div>
        <div class="gh-actions settings-btn-group">
          ${
            a.active
              ? `<button type="button" class="primary btn-sm" disabled title="Already active">Active</button>`
              : `<button type="button" class="primary btn-sm" data-action="gh-switch" data-user="${esc(a.user)}" data-host="${esc(a.host)}">Switch</button>`
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
  // Legacy desktop overlay cleanup
  document.getElementById("drawer")?.remove();

  if (!state.drawer) {
    // Close Vaul only when no panel owns it
    if (isAppDrawerOpen() && !state.panel) {
      closeAppDrawer("programmatic");
    }
    return;
  }

  // Content (map / pack) always uses Vaul — desktop + mobile
  if (state.panel) {
    // Prefer content over panel if both requested
    state.panel = null;
  }
  await ensureVaulHost();
  openAppDrawer({
    title: state.drawer.title,
    html: `<pre class="drawer-body">${esc(state.drawer.body)}</pre>`,
    variant: "content",
    onClose: (reason) => {
      if (reason === "user") {
        state.drawer = null;
        term?.focus();
      }
    },
  });
}

/** Re-paint the active in-shell stage (forms, tools, git, …). */
async function paintPanel(_opts?: { animateIn?: boolean }): Promise<void> {
  document.getElementById("panel-overlay")?.remove();
  if (!state.panel) {
    if (isAppDrawerOpen() && !state.drawer) {
      closeAppDrawer("programmatic");
    }
    unmountAppStage();
    return;
  }
  if (isAppDrawerOpen() && !state.drawer) {
    closeAppDrawer("programmatic");
  }
  paintAppStage();
  if (state.panel === "tools") {
    window.setTimeout(() => void refreshToolsStatus(), 60);
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

function pageHeroHTML(kicker: string, title: string, sub?: string, actionsHtml = ""): string {
  return `
    <header class="page-hero">
      <div class="page-hero-text">
        <div class="page-hero-kicker">${esc(kicker)}</div>
        <h1 class="page-hero-title">${esc(title)}</h1>
        ${sub ? `<p class="page-hero-sub">${sub}</p>` : ""}
      </div>
      ${actionsHtml ? `<div class="page-hero-actions">${actionsHtml}</div>` : ""}
    </header>`;
}

function newSessionPageHTML(): string {
  const accountBlock = state.agentAccountPlatform
    ? `<div class="page-card">
        <div class="page-card-head"><h2>Account</h2><span class="opt">${esc(platformLabel(state.agentAccountPlatform))}</span></div>
        <div class="field">
          <label for="sess-account">Profile</label>
          <select id="sess-account" ${state.creating ? "disabled" : ""}>
            ${accountOptionsHTML()}
          </select>
        </div>
        <div class="check-row">
          <label class="check"><input type="radio" name="sess-account-mode" value="isolated" ${state.formAccountMode !== "global" ? "checked" : ""} /> Isolated</label>
          <label class="check"><input type="radio" name="sess-account-mode" value="global" ${state.formAccountMode === "global" ? "checked" : ""} /> Global</label>
        </div>
      </div>`
    : "";
  const cwdExisting = !state.formCwdNew;
  return `
    ${pageHeroHTML("Create", "New session", "Start an agent TTY in an existing workspace or a brand-new folder.")}
    <div class="page-body page-body--narrow">
      <form id="create-form" class="page-form" data-action-form="create-session">
        <div class="page-card">
          <div class="page-card-head"><h2>Session</h2></div>
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
            <span class="field-label">Workspace</span>
            <div class="cwd-mode-row" role="radiogroup" aria-label="Workspace mode">
              <label class="check">
                <input type="radio" name="sess-cwd-mode" value="existing" data-action-change="sess-cwd-mode" ${cwdExisting ? "checked" : ""} ${state.creating ? "disabled" : ""} />
                Existing
              </label>
              <label class="check">
                <input type="radio" name="sess-cwd-mode" value="new" data-action-change="sess-cwd-mode" ${!cwdExisting ? "checked" : ""} ${state.creating ? "disabled" : ""} />
                New directory
              </label>
            </div>
          </div>
          <div class="field" id="sess-cwd-existing" ${cwdExisting ? "" : "hidden"}>
            <label for="sess-cwd">Directory</label>
            <select id="sess-cwd" name="cwd" ${cwdExisting ? "required" : ""} ${state.creating ? "disabled" : ""}>
              ${workspaceOptionsHTML()}
            </select>
          </div>
          <div class="field" id="sess-cwd-new" ${cwdExisting ? "hidden" : ""}>
            <label for="sess-cwd-new-name">New folder name</label>
            <input id="sess-cwd-new-name" name="cwd_new" value="${esc(state.formCwdNewName)}" placeholder="my-project or clients/acme" autocomplete="off" spellcheck="false" ${!cwdExisting ? "required" : ""} ${state.creating ? "disabled" : ""} />
            <p class="form-hint">Created under the host workspace root. Letters, numbers, <code>.</code> <code>_</code> <code>-</code>, and <code>/</code> for nesting.</p>
          </div>
          <div class="field">
            <label for="sess-name">Label <span class="opt">optional</span></label>
            <input id="sess-name" name="name" value="${esc(state.formName)}" placeholder="e.g. fix-auth" autocomplete="off" ${state.creating ? "disabled" : ""} />
          </div>
          <div class="check-row">
            <label class="check" title="Isolated git worktree for parallel agents">
              <input type="checkbox" id="sess-worktree" ${state.creating ? "disabled" : ""} />
              Isolated worktree
            </label>
          </div>
          <div class="field">
            <label for="sess-prompt">Seed prompt <span class="opt">optional</span></label>
            <textarea id="sess-prompt" name="prompt" rows="3" placeholder="Typed into TTY after start" ${state.creating ? "disabled" : ""}>${esc(state.formPrompt)}</textarea>
          </div>
        </div>
        ${accountBlock}
        <p class="form-hint">Cloning a git repo? <a href="/project/new" data-nav data-action="new-project" class="linkish">New project</a></p>
        ${state.createError ? `<p class="form-error" role="alert">${esc(state.createError)}</p>` : ""}
        <div class="page-actions">
          <button type="button" class="ghost" data-action="close-panel">Cancel</button>
          <button class="primary" type="submit" ${state.creating ? "disabled" : ""}>
            ${state.creating ? "Starting…" : state.formCwdNew ? "Create dir & start" : "Start session"}
          </button>
        </div>
      </form>
    </div>`;
}

function newProjectPageHTML(): string {
  return `
    ${pageHeroHTML("Create", "New project", "Clone a Git repository into the workspace root.")}
    <div class="page-body page-body--narrow">
      <form id="project-form" class="page-form" data-action-form="create-project">
        <div class="page-card">
          <div class="page-card-head"><h2>Repository</h2></div>
          <div class="field">
            <label for="proj-git-url">Repo URL or owner/repo</label>
            <input id="proj-git-url" name="url" value="${esc(state.formGitUrl)}" placeholder="https://github.com/org/app.git or org/app" required autocomplete="off" ${state.creating ? "disabled" : ""} autofocus />
          </div>
          <div class="field-grid">
            <div class="field">
              <label for="proj-git-name">Folder <span class="opt">optional</span></label>
              <input id="proj-git-name" name="name" value="${esc(state.formGitName)}" placeholder="repo name" autocomplete="off" ${state.creating ? "disabled" : ""} />
            </div>
            <div class="field">
              <label for="proj-git-branch">Branch <span class="opt">optional</span></label>
              <input id="proj-git-branch" name="branch" value="${esc(state.formGitBranch)}" placeholder="default" autocomplete="off" ${state.creating ? "disabled" : ""} />
            </div>
          </div>
          <div class="check-row check-row--wrap">
            <label class="check">
              <input type="checkbox" id="proj-git-fork" ${state.formGitFork ? "checked" : ""} ${state.creating ? "disabled" : ""} />
              Fork on GitHub first
            </label>
            <label class="check">
              <input type="checkbox" id="proj-git-depth" ${state.formGitDepth ? "checked" : ""} ${state.creating ? "disabled" : ""} />
              Shallow clone
            </label>
            <label class="check">
              <input type="checkbox" id="proj-start-session" checked ${state.creating ? "disabled" : ""} />
              Open session after clone
            </label>
          </div>
        </div>
        <p class="form-hint">Auth via SSH or <a href="/profile/github" data-nav data-action="open-settings" data-tab="github" class="linkish">GitHub settings</a></p>
        ${state.createError ? `<p class="form-error" role="alert">${esc(state.createError)}</p>` : ""}
        <div class="page-actions">
          <button type="button" class="ghost" data-action="close-panel">Cancel</button>
          <button class="primary" type="submit" ${state.creating ? "disabled" : ""}>
            ${state.creating ? "Cloning…" : "Clone project"}
          </button>
        </div>
      </form>
    </div>`;
}

function workspaceToolsBodyHTML(opts?: { settings?: boolean }): string {
  const settings = !!opts?.settings;
  const wrap = settings ? "settings-card col" : "tool-block";
  const head = settings ? "settings-card-title" : "tool-block-head";
  const titleOpen = settings ? "strong" : "h3";
  const titleClose = settings ? "strong" : "h3";
  return `
    <div class="field">
      <label for="tools-cwd-select">Target workspace</label>
      <select id="tools-cwd-select" data-action-change="tools-cwd">
        ${workspaceOptionsHTML()}
      </select>
    </div>
    <div class="${settings ? "settings-cards" : "tools-stack"}">
      <section class="${wrap}">
        <div class="${head}">
          <${titleOpen}>Context</${titleClose}>
          <span class="tool-status" data-status="ctx" id="ctx-status">${esc(state.ctxStatus)}</span>
        </div>
        <p class="tool-desc">Map + memory pack · auto-ensured on new session</p>
        <div class="btn-row">
          <button type="button" class="primary btn-sm" data-action="ctx-ensure">Ensure</button>
          <button type="button" class="ghost btn-sm" data-action="ctx-pack">Show pack</button>
        </div>
      </section>
      <section class="${wrap}">
        <div class="${head}">
          <${titleOpen}>Project map</${titleClose}>
          <span class="tool-status" data-status="map" id="map-status">${esc(state.mapStatus)}</span>
        </div>
        <p class="tool-desc"><code>.agents/PROJECT_MAP.md</code></p>
        <div class="btn-row">
          <button type="button" class="ghost btn-sm" data-action="map-gen">Generate</button>
          <button type="button" class="ghost btn-sm" data-action="map-show">Show</button>
        </div>
      </section>
      <section class="${wrap}">
        <div class="${head}">
          <${titleOpen}>Memory</${titleClose}>
          <span class="tool-status" data-status="mem" id="mem-status">${esc(state.memStatus)}</span>
        </div>
        <p class="tool-desc">FTS index of map + docs</p>
        <div class="btn-row">
          <button type="button" class="ghost btn-sm" data-action="mem-index">Reindex</button>
        </div>
        <form class="mem-search" data-action-form="mem-search">
          <input id="mem-query" name="q" data-mem-query placeholder="Search docs…" value="${esc(state.memQuery)}" autocomplete="off" />
          <button type="submit" class="primary btn-sm">Search</button>
        </form>
        <div id="mem-hits" class="mem-hits" data-mem-hits>${memHitsHTML()}</div>
      </section>
      <section class="${wrap} tool-block--tasks">
        <div class="${head}">
          <${titleOpen}>Tasks</${titleClose}>
          <span class="tool-status" data-status="tasks" id="tasks-status">${esc(
            state.tasks.length
              ? `${state.tasks.filter((t) => t.status !== "done").length} open · ${state.tasks.length}`
              : "none",
          )}</span>
        </div>
        <p class="tool-desc">Workspace list · <code>.agents/tasks.json</code></p>
        <div class="tasks-list-wrap" data-tasks-list>${tasksListHTML()}</div>
        <form class="task-add" data-action-form="task-add">
          <label class="sr-only" for="task-draft-input">New task</label>
          <input id="task-draft-input" name="title" data-task-draft placeholder="New task…" value="${esc(state.tasksDraft)}" autocomplete="off" ${state.tasksBusy ? "disabled" : ""} />
          <button type="submit" class="primary btn-sm" ${state.tasksBusy ? "disabled" : ""}>Add</button>
        </form>
      </section>
      <section class="${wrap}">
        <div class="${head}">
          <${titleOpen}>Playwright</${titleClose}>
          <span class="tool-status" data-status="pw" id="pw-status">${esc(state.pwStatus)}</span>
        </div>
        <p class="tool-desc">Xvfb + docker browser stack</p>
        <div class="btn-row">
          <button type="button" class="ghost btn-sm" data-action="pw-start">Start</button>
          <button type="button" class="ghost btn-sm" data-action="pw-stop">Stop</button>
          <button type="button" class="ghost btn-sm" data-action="pw-restart">Restart</button>
          <button type="button" class="ghost btn-sm" data-action="pw-install">Install</button>
        </div>
      </section>
    </div>`;
}

function toolsPageHTML(): string {
  const actions = `
    <button type="button" class="primary btn-sm" data-action="open-shell">${iconSvg("terminal")} Terminal</button>
    <button type="button" class="ghost btn-sm" data-action="open-remote">${iconSvg("external")} Remote</button>
    <button type="button" class="ghost btn-sm" data-action="git-changes">${iconSvg("git-branch")} Changes</button>`;
  return `
    ${pageHeroHTML("Workspace", "Tools", "Context, map, memory, tasks, and browser stack for the active cwd.", actions)}
    <div class="page-body">
      <div class="page-tools">
        ${workspaceToolsBodyHTML()}
        <p class="form-hint tools-foot">
          Accounts, GitHub, SSH →
          <button type="button" class="linkish" data-action="open-settings" data-tab="accounts">Open settings</button>
        </p>
      </div>
    </div>`;
}

// ── Git Changes — in-shell stage ────────────────────────────────────────────

function gitStatusLetter(f: GitFileEntry): string {
  if (f.status && f.status.trim()) {
    const s = f.status.trim();
    return (s.length === 1 ? s : s[s.length - 1] || s[0] || "?").toUpperCase();
  }
  if (f.xy && f.xy.length >= 1) {
    const x = f.xy[0] || " ";
    const y = f.xy[1] || " ";
    if (y !== " ") return y === "?" ? "?" : y.toUpperCase();
    if (x !== " ") return x === "?" ? "?" : x.toUpperCase();
  }
  return f.staged ? "M" : "?";
}

function gitStatusLabel(letter: string): string {
  switch (letter) {
    case "M":
      return "Modified";
    case "A":
      return "Added";
    case "D":
      return "Deleted";
    case "R":
      return "Renamed";
    case "C":
      return "Copied";
    case "U":
      return "Conflict";
    case "?":
      return "Untracked";
    default:
      return letter;
  }
}

function gitLineStats(files: GitFileEntry[]): { add: number; del: number } {
  let add = 0;
  let del = 0;
  for (const f of files) {
    add += f.additions ?? f.insertions ?? 0;
    del += f.deletions ?? 0;
  }
  return { add, del };
}

function gitCommitScopeLabel(): string {
  const files = state.gitStatus?.files ?? [];
  const n = state.gitCheckedPaths.length;
  const total = files.length;
  if (!total) return "No files";
  if (n > 0) return `${n} of ${total} file${total === 1 ? "" : "s"}`;
  return `${total} file${total === 1 ? "" : "s"}`;
}

function gitIsChecked(path: string): boolean {
  return state.gitCheckedPaths.length === 0 || state.gitCheckedPaths.includes(path);
}

function gitHeaderHTML(): string {
  const st = state.gitStatus;
  const busy = state.gitBusy || state.gitLoading;
  const files = st?.files ?? [];
  const { add, del } = gitLineStats(files);

  let statusBits = "";
  if (state.gitLoading && !st) {
    statusBits = `<span class="git-chip">Loading status…</span>`;
  } else if (state.gitError && !st) {
    statusBits = `<span class="git-chip git-chip--err">${esc(state.gitError)}</span>`;
  } else if (st?.is_repo === false) {
    statusBits = `<span class="git-chip git-chip--warn">Not a git repository</span>`;
  } else if (st) {
    const branch = st.branch || st.head || "HEAD";
    const dirty = st.dirty || files.length > 0;
    statusBits = `
      <span class="git-chip git-chip--branch" title="${esc(st.upstream || branch)}">
        ${iconSvg("git-branch")}<span>${esc(branch)}</span>
      </span>
      <span class="git-chip ${dirty ? "git-chip--dirty" : "git-chip--clean"}">${dirty ? "Dirty" : "Clean"}</span>
      <span class="git-chip">${files.length} file${files.length === 1 ? "" : "s"}</span>
      ${add || del ? `<span class="git-chip git-chip--stats"><span class="git-pm-add">+${add}</span><span class="git-pm-del">−${del}</span></span>` : ""}
      ${typeof st.ahead === "number" && st.ahead > 0 ? `<span class="git-chip" title="Ahead of upstream">↑${st.ahead}</span>` : ""}
      ${typeof st.behind === "number" && st.behind > 0 ? `<span class="git-chip" title="Behind upstream">↓${st.behind}</span>` : ""}
      ${st.upstream ? `<span class="git-chip git-chip--muted" title="Upstream">${esc(st.upstream)}</span>` : ""}
    `;
  }

  return `
    <header class="git-hero">
      <div class="git-hero-text">
        <div class="git-hero-kicker">Source control</div>
        <h1 class="git-hero-title">Changes</h1>
        <div class="git-hero-chips">${statusBits}</div>
      </div>
      <div class="git-hero-actions">
        <label class="git-workspace">
          <span class="git-workspace-label">Workspace</span>
          <select id="git-cwd-select" class="git-cwd-select" data-action-change="git-cwd" ${busy ? "disabled" : ""}>
            ${workspaceOptionsHTML()}
          </select>
        </label>
        <button type="button" class="ghost btn-sm git-refresh-btn" data-action="git-refresh" title="Refresh" ${busy ? "disabled" : ""}>
          ${state.gitLoading ? "Refreshing…" : "Refresh"}
        </button>
      </div>
    </header>`;
}

function gitFileListHTML(): string {
  const allFiles = state.gitStatus?.files ?? [];
  const q = state.gitFileFilter.trim().toLowerCase();
  const files = q ? allFiles.filter((f) => (f.path || "").toLowerCase().includes(q)) : allFiles;
  const allActive = state.gitSelectedPath == null;

  const overview = `
    <button type="button" class="git-file-row git-file-overview ${allActive ? "is-active" : ""}" data-action="git-select-all">
      <span class="git-file-ico" aria-hidden="true">∑</span>
      <span class="git-file-main">
        <span class="git-file-name">All changes</span>
        <span class="git-file-sub">Combined working-tree diff</span>
      </span>
      <span class="git-file-count">${allFiles.length}</span>
    </button>`;

  if (!allFiles.length) {
    return `<div class="git-file-list" data-git-files>
      ${overview}
      <div class="git-empty">
        <div class="git-empty-icon" aria-hidden="true">${iconSvg("git-branch")}</div>
        <p class="git-empty-title">Working tree clean</p>
        <p class="git-empty-hint">No uncommitted changes in this workspace.</p>
      </div>
    </div>`;
  }

  if (!files.length) {
    return `<div class="git-file-list" data-git-files>
      ${overview}
      <div class="git-empty git-empty--sm">
        <p class="git-empty-title">No matching files</p>
        <p class="git-empty-hint">Try a different filter.</p>
      </div>
    </div>`;
  }

  const rows = files
    .map((f) => {
      const path = f.path || "";
      const letter = gitStatusLetter(f);
      const active = state.gitSelectedPath === path;
      const checked = gitIsChecked(path);
      const name = basename(path);
      const dir = path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : "";
      const ins = f.additions ?? f.insertions ?? 0;
      const del = f.deletions ?? 0;
      const stClass = letter === "?" ? "u" : letter.toLowerCase();
      return `
      <div class="git-file-row git-st-${stClass} ${active ? "is-active" : ""}" data-git-path="${esc(path)}">
        <label class="git-file-check" title="Include in commit">
          <input type="checkbox" data-action-change="git-check" data-path="${esc(path)}" ${checked ? "checked" : ""} />
        </label>
        <button type="button" class="git-file-hit" data-action="git-select-file" data-path="${esc(path)}">
          <span class="git-file-status" title="${esc(gitStatusLabel(letter))}">${esc(letter)}</span>
          <span class="git-file-main">
            <span class="git-file-name" title="${esc(path)}">${esc(name)}</span>
            ${dir ? `<span class="git-file-sub" title="${esc(dir)}">${esc(dir)}</span>` : ""}
          </span>
          <span class="git-file-stats">
            ${ins > 0 ? `<span class="git-pm-add">+${ins}</span>` : ""}
            ${del > 0 ? `<span class="git-pm-del">−${del}</span>` : ""}
          </span>
        </button>
      </div>`;
    })
    .join("");

  return `<div class="git-file-list" data-git-files>${overview}${rows}</div>`;
}

function gitDiffLinesHTML(diff: string): string {
  if (!diff) {
    if (state.gitDiffLoading) {
      return `<div class="git-empty"><p class="git-empty-title">Loading diff…</p></div>`;
    }
    if (state.gitStatus?.is_repo === false) {
      return `<div class="git-empty">
        <div class="git-empty-icon" aria-hidden="true">${iconSvg("folder-git")}</div>
        <p class="git-empty-title">Not a git repository</p>
        <p class="git-empty-hint">Choose a workspace that contains a <code>.git</code> directory.</p>
      </div>`;
    }
    if (!(state.gitStatus?.files?.length)) {
      return `<div class="git-empty">
        <div class="git-empty-icon" aria-hidden="true">${iconSvg("git-branch")}</div>
        <p class="git-empty-title">Nothing to diff</p>
        <p class="git-empty-hint">Your working tree is clean.</p>
      </div>`;
    }
    return `<div class="git-empty">
      <p class="git-empty-title">No textual diff</p>
      <p class="git-empty-hint">Binary files, renames, or mode-only changes may not render here.</p>
    </div>`;
  }

  const lines = diff.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n");
  if (lines.length && lines[lines.length - 1] === "") lines.pop();

  let oldLn = 0;
  let newLn = 0;
  const parts: string[] = [];

  for (const line of lines) {
    let kind: "meta" | "hunk" | "add" | "del" | "ctx" = "ctx";
    let gutL = "";
    let gutR = "";
    let code = line;
    let mark = " ";

    if (
      line.startsWith("diff ") ||
      line.startsWith("index ") ||
      line.startsWith("---") ||
      line.startsWith("+++") ||
      line.startsWith("new file") ||
      line.startsWith("deleted file") ||
      line.startsWith("similarity") ||
      line.startsWith("rename ") ||
      line.startsWith("\\")
    ) {
      kind = "meta";
      code = line;
    } else if (line.startsWith("@@")) {
      kind = "hunk";
      const m = /^@@\s+-(\d+)(?:,\d+)?\s+\+(\d+)/.exec(line);
      if (m) {
        oldLn = Number(m[1]) - 1;
        newLn = Number(m[2]) - 1;
      }
      code = line;
    } else if (line.startsWith("+")) {
      kind = "add";
      newLn += 1;
      gutR = String(newLn);
      mark = "+";
      code = line.slice(1);
    } else if (line.startsWith("-")) {
      kind = "del";
      oldLn += 1;
      gutL = String(oldLn);
      mark = "−";
      code = line.slice(1);
    } else {
      kind = "ctx";
      if (line.startsWith(" ")) code = line.slice(1);
      oldLn += 1;
      newLn += 1;
      gutL = String(oldLn);
      gutR = String(newLn);
    }

    if (kind === "hunk") {
      parts.push(
        `<div class="git-hunk" role="separator"><span class="git-hunk-label">${esc(line)}</span></div>`,
      );
      continue;
    }
    if (kind === "meta") {
      parts.push(`<div class="git-diff-line is-meta"><span class="git-diff-code">${esc(code)}</span></div>`);
      continue;
    }

    parts.push(
      `<div class="git-diff-line is-${kind}">` +
        `<span class="git-diff-gutter" aria-hidden="true">` +
        `<span class="git-ln">${esc(gutL)}</span>` +
        `<span class="git-ln">${esc(gutR)}</span>` +
        `<span class="git-mark">${mark}</span>` +
        `</span>` +
        `<span class="git-diff-code">${esc(code)}</span>` +
        `</div>`,
    );
  }

  return `<div class="git-diff" data-git-diff role="region" aria-label="Diff">${parts.join("")}</div>`;
}

function gitDiffPaneHTML(): string {
  const path = state.gitSelectedPath;
  const files = state.gitStatus?.files ?? [];
  const file = path ? files.find((f) => f.path === path) : null;
  const letter = file ? gitStatusLetter(file) : path ? "?" : "∗";
  const title = path || "All changes";
  const sub = path
    ? gitStatusLabel(letter)
    : files.length
      ? `${files.length} file${files.length === 1 ? "" : "s"} in working tree`
      : "No changes";
  const ins = file ? (file.additions ?? file.insertions ?? 0) : gitLineStats(files).add;
  const del = file ? (file.deletions ?? 0) : gitLineStats(files).del;

  return `
    <section class="git-diff-pane" aria-label="Diff viewer">
      <header class="git-diff-toolbar">
        <div class="git-diff-identity">
          <span class="git-file-status git-st-badge git-st-${letter === "?" ? "u" : letter === "∗" ? "all" : letter.toLowerCase()}">${esc(letter)}</span>
          <div class="git-diff-titles">
            <code class="git-diff-path" title="${esc(title)}">${esc(title)}</code>
            <span class="git-diff-sub">${esc(sub)}</span>
          </div>
        </div>
        <div class="git-diff-toolbar-meta">
          ${ins > 0 ? `<span class="git-pm-add">+${ins}</span>` : ""}
          ${del > 0 ? `<span class="git-pm-del">−${del}</span>` : ""}
        </div>
      </header>
      <div class="git-diff-wrap" data-git-diff-wrap>
        ${
          state.gitDiffLoading
            ? `<div class="git-empty"><p class="git-empty-title">Loading diff…</p></div>`
            : gitDiffLinesHTML(state.gitDiffText)
        }
      </div>
    </section>`;
}

function gitComposerHTML(): string {
  const files = state.gitStatus?.files ?? [];
  const commitDisabled = state.gitBusy || !files.length;
  const scope = gitCommitScopeLabel();
  const prOpen = state.gitPrOpen ? " open" : "";

  return `
    <footer class="git-composer" aria-label="Commit and pull request">
      <form class="git-composer-commit" data-action-form="git-commit">
        <div class="git-composer-field">
          <label for="git-commit-msg">Commit message</label>
          <textarea id="git-commit-msg" name="message" rows="3" placeholder="Summarize the change for teammates…" required ${commitDisabled ? "disabled" : ""}>${esc(state.gitCommitMsg)}</textarea>
        </div>
        <div class="git-composer-actions">
          <span class="git-composer-scope" data-git-commit-scope>${esc(scope)}</span>
          <button type="button" class="ghost btn-sm" data-action="git-toggle-pr" aria-expanded="${state.gitPrOpen ? "true" : "false"}">
            ${state.gitPrOpen ? "Hide PR" : "Pull request"}
          </button>
          <button type="submit" class="primary" ${commitDisabled ? "disabled" : ""}>
            ${state.gitBusy ? "Working…" : "Commit changes"}
          </button>
        </div>
      </form>
      <div class="git-composer-pr${prOpen}" id="git-pr-panel" ${state.gitPrOpen ? "" : "hidden"}>
        <form class="git-pr-form" data-action-form="git-pr">
          <div class="git-pr-grid">
            <div class="field">
              <label for="git-pr-title">Title</label>
              <input id="git-pr-title" name="title" value="${esc(state.gitPrTitle)}" placeholder="PR title" required autocomplete="off" ${state.gitBusy ? "disabled" : ""} />
            </div>
            <div class="field">
              <label for="git-pr-base">Base branch</label>
              <input id="git-pr-base" name="base" value="${esc(state.gitPrBase)}" placeholder="main" autocomplete="off" ${state.gitBusy ? "disabled" : ""} />
            </div>
          </div>
          <div class="field">
            <label for="git-pr-body">Description <span class="opt">optional</span></label>
            <textarea id="git-pr-body" name="body" rows="3" placeholder="What should reviewers know?" ${state.gitBusy ? "disabled" : ""}>${esc(state.gitPrBody)}</textarea>
          </div>
          <div class="git-pr-actions">
            <label class="check">
              <input type="checkbox" id="git-pr-draft" ${state.gitPrDraft ? "checked" : ""} ${state.gitBusy ? "disabled" : ""} />
              Draft PR
            </label>
            <button type="submit" class="primary btn-sm" ${state.gitBusy ? "disabled" : ""}>
              ${state.gitBusy ? "Working…" : "Create pull request"}
            </button>
          </div>
          ${
            state.gitPrUrl
              ? `<p class="git-pr-url"><a href="${esc(state.gitPrUrl)}" target="_blank" rel="noopener noreferrer">${esc(state.gitPrUrl)}</a></p>`
              : ""
          }
        </form>
      </div>
    </footer>`;
}

function unmountAppStage(): void {
  document.getElementById("app-stage")?.remove();
  document.getElementById("git-stage")?.remove(); // legacy
  document.getElementById("git-page-root")?.remove();
  const main = document.querySelector(".main");
  main?.classList.remove(
    "main--stage",
    "main--git",
    "main--tools",
    "main--form",
    "main--remote",
    "main--help",
    "main--projects",
  );
  const term = document.querySelector<HTMLElement>(".term-wrap");
  if (term) term.hidden = false;
  const tabs = document.getElementById("tabs");
  if (tabs) tabs.hidden = false;
}

/** @deprecated alias */
function unmountGitStage(): void {
  unmountAppStage();
}

function stageMeta(p: Exclude<Panel, null>): {
  title: string;
  kicker: string;
  aria: string;
  cls: string;
} {
  switch (p) {
    case "new":
      return { title: "New session", kicker: "Create", aria: "New session", cls: "main--form" };
    case "new-project":
      return { title: "New project", kicker: "Create", aria: "New project", cls: "main--form" };
    case "projects":
      return { title: "Projects", kicker: "Workspace", aria: "Project list", cls: "main--projects" };
    case "tools":
      return { title: "Tools", kicker: "Workspace", aria: "Workspace tools", cls: "main--tools" };
    case "changes":
      return { title: "Changes", kicker: "Source control", aria: "Git changes", cls: "main--git" };
    case "remote":
      return { title: "Open remote", kicker: "Workspace", aria: "Open remote editor", cls: "main--remote" };
    case "help":
      return { title: "Shortcuts", kicker: "Help", aria: "Keyboard shortcuts", cls: "main--help" };
    default:
      return { title: "Page", kicker: "App", aria: "Page", cls: "main--form" };
  }
}

function stageBodyHTML(p: Exclude<Panel, null>): string {
  switch (p) {
    case "new":
      return newSessionPageHTML();
    case "new-project":
      return newProjectPageHTML();
    case "projects":
      return projectsPageHTML();
    case "tools":
      return toolsPageHTML();
    case "changes":
      return gitPageHTML();
    case "remote":
      return remotePageHTML();
    case "help":
      return helpPageHTML();
    default:
      return "";
  }
}

/** Mount active panel as full in-shell stage under the topbar. */
function paintAppStage(): void {
  if (!state.shellMounted || !isStagePanel(state.panel)) {
    unmountAppStage();
    return;
  }
  const p = state.panel;
  const meta = stageMeta(p);
  const main = document.querySelector(".main");
  if (!main) return;

  main.classList.remove(
    "main--stage",
    "main--git",
    "main--tools",
    "main--form",
    "main--remote",
    "main--help",
    "main--projects",
  );
  main.classList.add("main--stage", meta.cls);

  const term = main.querySelector<HTMLElement>(".term-wrap");
  if (term) term.hidden = true;
  const tabs = document.getElementById("tabs");
  if (tabs) tabs.hidden = true;

  let stage = document.getElementById("app-stage");
  if (!stage) {
    stage = document.createElement("div");
    stage.id = "app-stage";
    if (term) term.before(stage);
    else {
      const status = main.querySelector(".status-bar");
      if (status) status.before(stage);
      else main.appendChild(stage);
    }
  }
  stage.className = `app-stage app-stage--${p}`;
  stage.setAttribute("role", "main");
  stage.setAttribute("aria-label", meta.aria);
  stage.hidden = false;
  stage.innerHTML = stageBodyHTML(p);
  if (p === "changes") stage.setAttribute("data-git-stage", "1");
  else stage.removeAttribute("data-git-stage");

  paintBreadcrumb(true);

  if (p === "changes") {
    const filter = document.getElementById("git-file-filter") as HTMLInputElement | null;
    filter?.addEventListener("input", () => {
      state.gitFileFilter = filter.value;
      const list = document.querySelector("#app-stage [data-git-files], #git-stage [data-git-files]");
      if (list) list.outerHTML = gitFileListHTML();
    });
  }
  if (p === "projects") {
    const filter = document.getElementById("projects-filter") as HTMLInputElement | null;
    filter?.addEventListener("input", () => {
      state.projectsFilter = filter.value;
      const list = document.querySelector("#app-stage [data-projects-list]");
      if (list) list.innerHTML = projectsListHTML();
    });
  }
}

/** @deprecated — use paintAppStage */
function paintGitPage(_opts?: { animateIn?: boolean }): void {
  paintAppStage();
}

function gitPageHTML(): string {
  const files = state.gitStatus?.files ?? [];
  return `
    ${gitHeaderHTML()}
    ${
      state.gitError && state.gitStatus
        ? `<div class="git-banner" role="alert">${esc(state.gitError)}</div>`
        : ""
    }
    <div class="git-workspace-grid">
      <aside class="git-files-pane" aria-label="Changed files">
        <div class="git-files-head">
          <div class="git-files-head-row">
            <h2>Files</h2>
            <span class="git-files-count">${files.length}</span>
          </div>
          <label class="git-files-filter">
            <span class="sr-only">Filter files</span>
            ${iconSvg("search")}
            <input id="git-file-filter" type="search" placeholder="Filter files…" value="${esc(state.gitFileFilter)}" autocomplete="off" spellcheck="false" />
          </label>
        </div>
        ${gitFileListHTML()}
      </aside>
      ${gitDiffPaneHTML()}
    </div>
    ${gitComposerHTML()}`;
}

function syncGitFormFromDOM(): void {
  const msg = document.getElementById("git-commit-msg") as HTMLTextAreaElement | null;
  if (msg) state.gitCommitMsg = msg.value;
  const title = document.getElementById("git-pr-title") as HTMLInputElement | null;
  if (title) state.gitPrTitle = title.value;
  const body = document.getElementById("git-pr-body") as HTMLTextAreaElement | null;
  if (body) state.gitPrBody = body.value;
  const base = document.getElementById("git-pr-base") as HTMLInputElement | null;
  if (base) state.gitPrBase = base.value;
  const draft = document.getElementById("git-pr-draft") as HTMLInputElement | null;
  if (draft) state.gitPrDraft = draft.checked;
  const cwdSel = document.getElementById("git-cwd-select") as HTMLSelectElement | null;
  if (cwdSel?.value) state.formCwd = cwdSel.value;
  const filter = document.getElementById("git-file-filter") as HTMLInputElement | null;
  if (filter) state.gitFileFilter = filter.value;
}

function gitCwd(): string {
  const sel = document.getElementById("git-cwd-select") as HTMLSelectElement | null;
  if (sel?.value) {
    state.formCwd = sel.value;
    return sel.value;
  }
  return toolsCwd();
}

async function refreshGitChanges(): Promise<void> {
  const cwd = gitCwd();
  state.gitLoading = true;
  state.gitError = "";
  if (state.panel === "changes") {
    const chips = document.querySelector(
      "#app-stage .git-hero-chips, #git-stage .git-hero-chips",
    );
    if (chips) chips.innerHTML = `<span class="git-chip">Loading status…</span>`;
  }
  try {
    const st = await gitStatus(cwd);
    state.gitStatus = st;
    if (st.error) state.gitError = st.error;
    const paths = new Set((st.files || []).map((f) => f.path));
    state.gitCheckedPaths = state.gitCheckedPaths.filter((p) => paths.has(p));
    if (state.gitSelectedPath && !paths.has(state.gitSelectedPath)) {
      state.gitSelectedPath = null;
    }
  } catch (e) {
    state.gitStatus = null;
    state.gitError = (e as Error).message || "git status failed";
    state.gitDiffText = "";
  } finally {
    state.gitLoading = false;
  }
  if (state.panel === "changes") {
    syncGitFormFromDOM();
    paintGitPage();
  }
  await loadGitDiff();
}

async function loadGitDiff(): Promise<void> {
  if (state.panel !== "changes") return;
  const cwd = gitCwd();
  if (state.gitStatus?.is_repo === false) {
    state.gitDiffText = "";
    state.gitDiffLoading = false;
    paintGitDiffOnly();
    return;
  }
  state.gitDiffLoading = true;
  paintGitDiffOnly();
  try {
    const out = await gitDiff({
      cwd,
      path: state.gitSelectedPath || undefined,
      staged: false,
    });
    state.gitDiffText = out.diff || out.patch || "";
    if (out.error && !state.gitDiffText) {
      state.gitDiffText = "";
      if (out.error) state.gitError = out.error;
    }
  } catch (e) {
    state.gitDiffText = "";
    state.gitError = (e as Error).message || "diff failed";
  } finally {
    state.gitDiffLoading = false;
    paintGitDiffOnly();
  }
}

function paintGitDiffOnly(): void {
  const root =
    document.querySelector<HTMLElement>("#app-stage[data-git-stage]") ||
    document.querySelector<HTMLElement>("#app-stage.app-stage--changes") ||
    document.getElementById("git-stage");
  if (!root) return;

  // Refresh diff pane header + body without wiping commit composer
  const pane = root.querySelector(".git-diff-pane");
  if (pane) {
    const tmp = document.createElement("div");
    tmp.innerHTML = gitDiffPaneHTML();
    const next = tmp.firstElementChild;
    if (next) pane.replaceWith(next);
  } else {
    const wrap = root.querySelector<HTMLElement>("[data-git-diff-wrap]");
    if (wrap) {
      wrap.innerHTML = state.gitDiffLoading
        ? `<div class="git-empty"><p class="git-empty-title">Loading diff…</p></div>`
        : gitDiffLinesHTML(state.gitDiffText);
    }
  }

  root.querySelectorAll<HTMLElement>("[data-git-path]").forEach((row) => {
    const path = row.getAttribute("data-git-path") || "";
    row.classList.toggle("is-active", state.gitSelectedPath === path);
  });
  root.querySelectorAll<HTMLElement>("[data-action='git-select-all']").forEach((btn) => {
    btn.classList.toggle("is-active", state.gitSelectedPath == null);
  });
}

async function onGitSelectFile(path: string | null): Promise<void> {
  syncGitFormFromDOM();
  state.gitSelectedPath = path;
  paintGitDiffOnly();
  await loadGitDiff();
}

async function onGitCommit(): Promise<void> {
  syncGitFormFromDOM();
  const message = state.gitCommitMsg.trim();
  if (!message) {
    toast("Commit message required", "err");
    return;
  }
  const cwd = gitCwd();
  const paths = state.gitCheckedPaths.length ? [...state.gitCheckedPaths] : undefined;
  const scope = paths
    ? `${paths.length} file(s)`
    : `${state.gitStatus?.files?.length ?? 0} change(s)`;
  if (!confirm(`Commit ${scope} in ${cwd}?\n\n${message}`)) return;
  state.gitBusy = true;
  paintGitPage();
  try {
    const out = await gitCommit({
      cwd,
      message,
      all: !paths,
      paths,
    });
    const hash = (out.commit || out.short_hash || out.hash || "").slice(0, 8);
    toast(hash ? `Committed ${hash}` : "Committed", "ok");
    state.gitCommitMsg = "";
    state.gitCheckedPaths = [];
    state.gitSelectedPath = null;
    state.gitBusy = false;
    await refreshGitChanges();
  } catch (e) {
    state.gitBusy = false;
    paintGitPage();
    toast((e as Error).message || "commit failed", "err", 6000);
  }
}

async function onGitCreatePR(): Promise<void> {
  syncGitFormFromDOM();
  const title = state.gitPrTitle.trim();
  if (!title) {
    toast("PR title required", "err");
    return;
  }
  const cwd = gitCwd();
  if (!confirm(`Create pull request “${title}” for ${cwd}?`)) return;
  state.gitBusy = true;
  state.gitPrUrl = "";
  paintGitPage();
  try {
    const out = await gitPullRequest({
      cwd,
      title,
      body: state.gitPrBody.trim() || undefined,
      base: state.gitPrBase.trim() || undefined,
      draft: state.gitPrDraft || undefined,
    });
    const url = out.url || "";
    state.gitPrUrl = url;
    state.gitBusy = false;
    state.gitPrOpen = true;
    paintGitPage();
    if (url) toast("PR created", "ok");
    else
      toast(
        out.ok === false ? out.error || "PR create failed" : "PR created",
        out.ok === false ? "err" : "ok",
      );
  } catch (e) {
    state.gitBusy = false;
    paintGitPage();
    toast((e as Error).message || "create PR failed", "err", 6000);
  }
}


function helpPageHTML(): string {
  return `
    ${pageHeroHTML("Help", "Shortcuts", "Bare keys outside the terminal · Alt+key while focused · ⌘/Ctrl+K palette")}
    <div class="page-body page-body--wide">
      <div class="help-page"><div class="help-body">
      <p class="form-hint help-lede">
        <kbd>Ctrl</kbd><kbd>Tab</kbd> always switches tabs (even over the agent).
      </p>
      <div class="help-grid">
        <section class="help-section">
          <h3>Sessions (list)</h3>
          <dl class="keys">
            <div><dt><kbd>j</kbd> <kbd>k</kbd></dt><dd>Move list cursor</dd></div>
            <div><dt><kbd>⇧</kbd><kbd>j</kbd> <kbd>⇧</kbd><kbd>k</kbd></dt><dd>Step list + open</dd></div>
            <div><dt><kbd>↑</kbd> <kbd>↓</kbd></dt><dd>Same as j/k</dd></div>
            <div><dt><kbd>⇧</kbd><kbd>↑</kbd><kbd>↓</kbd></dt><dd>Step list + open</dd></div>
            <div><dt><kbd>Enter</kbd> <kbd>o</kbd> <kbd>Space</kbd></dt><dd>Open highlighted</dd></div>
            <div><dt><kbd>/</kbd></dt><dd>Filter sessions</dd></div>
            <div><dt><kbd>y</kbd></dt><dd>Copy session id</dd></div>
            <div><dt><kbd>r</kbd></dt><dd>Refresh list</dd></div>
          </dl>
        </section>
        <section class="help-section">
          <h3>Tabs (open)</h3>
          <dl class="keys">
            <div><dt><kbd>h</kbd> <kbd>l</kbd></dt><dd>Prev / next tab</dd></div>
            <div><dt><kbd>[</kbd> <kbd>]</kbd></dt><dd>Prev / next tab</dd></div>
            <div><dt><kbd>←</kbd> <kbd>→</kbd></dt><dd>Prev / next tab</dd></div>
            <div><dt><kbd>Ctrl</kbd><kbd>Tab</kbd></dt><dd>Next tab (works in TTY)</dd></div>
            <div><dt><kbd>Ctrl</kbd><kbd>⇧</kbd><kbd>Tab</kbd></dt><dd>Prev tab</dd></div>
            <div><dt><kbd>1</kbd>–<kbd>9</kbd> <kbd>0</kbd></dt><dd>Jump tab (0 = last)</dd></div>
            <div><dt><kbd>Ctrl</kbd><kbd>1</kbd>–<kbd>9</kbd></dt><dd>Jump tab from TTY</dd></div>
            <div><dt><kbd>⇧</kbd><kbd>h</kbd> <kbd>Home</kbd></dt><dd>First tab</dd></div>
            <div><dt><kbd>⇧</kbd><kbd>l</kbd> <kbd>End</kbd></dt><dd>Last tab</dd></div>
            <div><dt><kbd>x</kbd></dt><dd>Close tab (detach only)</dd></div>
            <div><dt><kbd>⇧</kbd><kbd>x</kbd></dt><dd>Close all tabs</dd></div>
          </dl>
        </section>
        <section class="help-section">
          <h3>Actions</h3>
          <dl class="keys">
            <div><dt><kbd>n</kbd></dt><dd>New session</dd></div>
            <div><dt><kbd>⇧</kbd><kbd>n</kbd></dt><dd>New project</dd></div>
            <div><dt><kbd>s</kbd></dt><dd>Stop agent</dd></div>
            <div><dt><kbd>e</kbd></dt><dd>Resume agent</dd></div>
            <div><dt><kbd>⇧</kbd><kbd>d</kbd></dt><dd>Delete session</dd></div>
            <div><dt><kbd>⇧</kbd><kbd>s</kbd></dt><dd>Settings → SSH</dd></div>
            <div><dt><kbd>c</kbd></dt><dd>Clear stopped</dd></div>
            <div><dt><kbd>i</kbd> <kbd>\`</kbd></dt><dd>Focus terminal</dd></div>
            <div><dt><kbd>u</kbd> <kbd>Esc</kbd></dt><dd>Arm shortcuts / blur TTY</dd></div>
            <div><dt><kbd>f</kbd></dt><dd>Fit terminal</dd></div>
            <div><dt><kbd>d</kbd></dt><dd>Desk (no modal)</dd></div>
          </dl>
        </section>
        <section class="help-section">
          <h3>Panels</h3>
          <dl class="keys">
            <div><dt><kbd>t</kbd></dt><dd>Tools</dd></div>
            <div><dt><kbd>⇧</kbd><kbd>t</kbd></dt><dd>Plain shell terminal</dd></div>
            <div><dt><kbd>⇧</kbd><kbd>g</kbd></dt><dd>Git changes</dd></div>
            <div><dt><kbd>,</kbd> <kbd>a</kbd></dt><dd>Settings</dd></div>
            <div><dt><kbd>g</kbd></dt><dd>Settings → GitHub</dd></div>
            <div><dt><kbd>w</kbd></dt><dd>Toggle session rail</dd></div>
            <div><dt><kbd>p</kbd> <kbd>Ctrl</kbd><kbd>K</kbd></dt><dd>Command palette</dd></div>
            <div><dt><kbd>?</kbd></dt><dd>This help</dd></div>
            <div><dt><kbd>Esc</kbd></dt><dd>Close overlay</dd></div>
          </dl>
        </section>
      </div>
      <p class="form-hint help-foot">Tab close never kills the agent — use <kbd>s</kbd> Stop or <kbd>⇧</kbd><kbd>d</kbd> Delete.</p>
    </div>
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
    const hay =
      `${s.name || ""} ${s.agent} ${s.cwd} ${s.id} ${s.state} ${s.git_branch || ""} ${s.branch || ""}`.toLowerCase();
    return hay.includes(q);
  });
}

/** Full paint: login or initial shell mount. */
function paint(): void {
  applyTheme();
  if (!state.token) {
    stopPolling();
    state.shellMounted = false;
    shellBound = false;
    shellEntranceDone = false;
    lastSessionListPaintSig = "";
    lastTabsPaintSig = "";
    lastStatusPaintSig = "";
    lastCrumbPaintSig = "";
    hideSessionCtx();
    hideTopbarMore();
    closeCommandPalette();
    unmountGitStage();
    if (isAppDrawerOpen()) closeAppDrawer("programmatic");
    app.innerHTML = loginHTML();
    bindLogin();
    animateLoginIn(app);
    return;
  }
  if (!state.shellMounted) {
    app.innerHTML = shellHTML();
    state.shellMounted = true;
    shellBound = false;
    lastSessionListPaintSig = sessionListPaintSig();
    lastTabsPaintSig = tabsPaintSig();
    lastStatusPaintSig = statusPaintSig();
    lastCrumbPaintSig = crumbPaintSig();
    bindShell();
    ensureTermArea();
    void paintPanel({ animateIn: false });
    void paintDrawer();
    if (!shellEntranceDone) {
      shellEntranceDone = true;
      const shell = document.querySelector<HTMLElement>(".shell");
      if (shell) animateShellIn(shell);
    }
    if (!pressMotionBound) {
      pressMotionBound = true;
      bindPressMotion(document);
    }
    if (state.activeId) void attachActive();
    void listTemplates()
      .then((o) => {
        state.templates = o.templates || [];
      })
      .catch(() => {
        /* optional */
      });
    return;
  }
  paintChrome();
}

/**
 * Display-only signature for the session list.
 * Includes fields that affect row markup (not relativeTime ages — those may lag until next real change).
 * Built from the same sorted/filtered view as the HTML to avoid order-only thrash from the API.
 */
function sessionListPaintSig(): string {
  if (state.sessions.length === 0) {
    return `empty\n${state.filter}`;
  }
  const list = filteredSessionsSorted();
  if (list.length === 0) {
    return `nomatch\n${state.filter}`;
  }
  const rows = list
    .map((s) => {
      const active = s.id === state.activeId ? "1" : "0";
      const cursor = s.id === state.listCursorId ? "1" : "0";
      const branch = (s.git_branch || s.branch || "").trim();
      // Display fields only: id, state, name, agent, cwd, branch (title + meta + classes)
      return `${s.id}\0${s.state}\0${s.name || ""}\0${s.agent}\0${s.cwd}\0${branch}\0${active}\0${cursor}`;
    })
    .join("\n");
  return rows;
}

function tabsPaintSig(): string {
  if (state.openTabs.length === 0) return "empty";
  return state.openTabs
    .map((t) => {
      const active = t.id === state.activeId ? "1" : "0";
      return `${t.id}\0${t.state}\0${t.title}\0${t.agent}\0${t.cwd}\0${active}`;
    })
    .join("\n");
}

function statusPaintSig(): string {
  const run = state.sessions.filter((s) => s.state === "running").length;
  return [
    state.statusOk ? "1" : "0",
    String(run),
    String(state.sessions.length),
    String(state.openTabs.length),
    state.statusText || "",
    state.sidebarOpen ? "1" : "0",
  ].join("\0");
}

function crumbPaintSig(): string {
  const tab = state.openTabs.find((t) => t.id === state.activeId);
  return [
    state.activeId || "",
    state.panel || "",
    tab?.title || "",
    tab?.cwd || "",
    tab?.agent || "",
  ].join("\0");
}

function paintSessionList(force = false): void {
  const list = document.getElementById("session-list");
  if (!list) return;
  const sig = sessionListPaintSig();
  if (!force && sig === lastSessionListPaintSig) return;
  lastSessionListPaintSig = sig;
  list.innerHTML = sessionListHTML();
}

function paintTabs(force = false): void {
  const tabs = document.getElementById("tabs");
  if (!tabs) return;
  const sig = tabsPaintSig();
  if (!force && sig === lastTabsPaintSig) return;
  lastTabsPaintSig = sig;
  tabs.innerHTML = tabsHTML();
}

/** Lightweight status bits: sess-count, status-bar, nav labels, sidebar class. */
function paintStatusChrome(force = false): void {
  const sig = statusPaintSig();
  if (!force && sig === lastStatusPaintSig) {
    // Still allow conn pill updates (separate path) — nothing else.
    return;
  }
  lastStatusPaintSig = sig;
  const status = document.getElementById("status-bar");
  if (status) status.innerHTML = statusHTML();
  const count = document.getElementById("sess-count");
  if (count) {
    const run = state.sessions.filter((s) => s.state === "running").length;
    count.textContent = `${run}/${state.sessions.length}`;
  }
  const host = document.getElementById("nav-host");
  if (host) {
    host.textContent = (state.statusText || "host").split("·")[0]?.trim() || "host";
  }
  const navStatus = document.getElementById("nav-status");
  if (navStatus) navStatus.textContent = state.statusText || "connected";
  const shell = document.querySelector(".shell");
  shell?.classList.toggle("sidebar-open", state.sidebarOpen);
}

function paintBreadcrumb(force = false): void {
  const crumb = document.getElementById("breadcrumb");
  if (!crumb) return;
  const sig = crumbPaintSig();
  if (!force && sig === lastCrumbPaintSig) return;
  lastCrumbPaintSig = sig;
  crumb.outerHTML = breadcrumbHTML();
}

/** Sync data-active on sidebar tools when panel/settings open. */
function paintSidebarActive(): void {
  const root = document.getElementById("sidebar");
  if (!root) return;
  const active: Record<string, boolean> = {
    "new-session": state.panel === "new",
    "new-project": state.panel === "new-project",
    projects: state.panel === "projects",
    tools: state.panel === "tools",
    "git-changes": state.panel === "changes",
    help: state.panel === "help",
    "open-settings": state.settingsOpen,
  };
  root.querySelectorAll<HTMLElement>("[data-action]").forEach((el) => {
    const action = el.getAttribute("data-action") || "";
    if (!(action in active)) return;
    if (active[action]) el.setAttribute("data-active", "true");
    else el.removeAttribute("data-active");
  });
}

function paintChrome(): void {
  if (!state.shellMounted) return;
  paintSessionList();
  paintTabs();
  paintStatusChrome();
  paintBreadcrumb();
  paintSidebarActive();
  paintConn();
  // Do not re-paint toast here — poll/chrome would re-trigger entrance animation.
}

function loginHTML(): string {
  return `
  <div class="login">
    <div class="login-card">
      <div class="login-brand">
        <span class="logo-mark" aria-hidden="true">a</span>
        <div class="login-brand-text">
          <h1>agents</h1>
          <p class="sub">Session control plane</p>
        </div>
      </div>
      <p class="login-lede">Sign in with your host API token to open agent TTYs, workspaces, and tools.</p>
      <form id="login-form" class="login-form">
        <div class="field">
          <label for="token">API token</label>
          <input id="token" name="token" type="password" autocomplete="current-password" placeholder="AGENTSD_TOKEN" required autofocus />
        </div>
        ${state.loginError ? `<p class="error form-error" role="alert">${esc(state.loginError)}</p>` : ""}
        ${state.busy ? `<p class="login-busy" aria-live="polite">Connecting to host…</p>` : ""}
        <button class="primary login-submit" type="submit" ${state.busy ? "disabled" : ""}>
          ${state.busy ? "Connecting…" : "Connect"}
        </button>
      </form>
      <p class="hint login-hint">Same bearer as <code>agentsctl</code>. Run <code>agentsctl web</code> for one-click login. Token stays in this browser only.</p>
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
    case "search":
      return `<svg ${common}><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>`;
    case "more":
      return `<svg ${common}><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/></svg>`;
    case "plus":
      return `<svg ${common}><path d="M5 12h14"/><path d="M12 5v14"/></svg>`;
    case "folder-git":
      return `<svg ${common}><path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/><circle cx="12" cy="13" r="2"/><path d="M14 13.5V17"/><path d="M10 13.5V17"/></svg>`;
    case "git-branch":
      return `<svg ${common}><line x1="6" x2="6" y1="3" y2="15"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/></svg>`;
    case "git":
      return `<svg ${common}><circle cx="12" cy="12" r="3"/><line x1="3" x2="9" y1="12" y2="12"/><line x1="15" x2="21" y1="12" y2="12"/><path d="M18 6v3"/><path d="M18 15v3"/><path d="M6 6v3"/><path d="M6 15v3"/></svg>`;
    case "external":
      return `<svg ${common}><path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/></svg>`;
    case "chevrons-up-down":
      return `<svg ${common}><path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/></svg>`;
    case "trash":
      return `<svg ${common}><path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/></svg>`;
    case "chevron-right":
      return `<svg ${common}><path d="m9 18 6-6-6-6"/></svg>`;
    case "x":
      return `<svg ${common}><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`;
    case "users":
      return `<svg ${common}><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>`;
    case "github":
      return `<svg ${common}><path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.403 5.403 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65-.17.6-.22 1.23-.15 1.85v4"/><path d="M9 18c-4.51 2-5-2-7-2"/></svg>`;
    case "key":
      return `<svg ${common}><circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6"/><path d="m15.5 7.5 3 3L22 7l-3-3"/></svg>`;
    case "layers":
      return `<svg ${common}><path d="m12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83Z"/><path d="m22 17.65-9.17 4.16a2 2 0 0 1-1.66 0L2 17.65"/><path d="m22 12.65-9.17 4.16a2 2 0 0 1-1.66 0L2 12.65"/></svg>`;
    case "info":
      return `<svg ${common}><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>`;
    default:
      return "";
  }
}

/** Compact chevron separator for shadcn-style breadcrumbs. */
function crumbSep(): string {
  return `<span class="sep-chev" aria-hidden="true"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg></span>`;
}

function breadcrumbHTML(): string {
  const stageCrumb: Partial<Record<Exclude<Panel, null>, string>> = {
    changes: "Changes",
    tools: "Tools",
    new: "New session",
    "new-project": "New project",
    projects: "Projects",
    remote: "Open remote",
    help: "Shortcuts",
  };
  if (state.panel && stageCrumb[state.panel]) {
    // Session-scoped tools keep project → session → Tools trail
    if (state.panel === "tools" && state.activeId) {
      const tab = state.openTabs.find((t) => t.id === state.activeId);
      if (tab) {
        const sessHref = sessionPath(tab.cwd, tab.id);
        const toolsHref = sessionPath(tab.cwd, tab.id, true);
        return `<nav class="breadcrumb" id="breadcrumb" aria-label="Breadcrumb">
          <a href="/desk" data-nav data-route="desk">Desk</a>
          ${crumbSep()}
          <a href="${esc(sessHref)}" data-nav title="${esc(tab.agent)} · ${esc(tab.cwd)}">${esc(basename(tab.cwd))}</a>
          ${crumbSep()}
          <a href="${esc(sessHref)}" data-nav title="${esc(tab.id)}">${esc(tab.title)}</a>
          ${crumbSep()}
          <a href="${esc(toolsHref)}" data-nav class="current" aria-current="page"><strong>Tools</strong></a>
        </nav>`;
      }
    }
    return `<nav class="breadcrumb" id="breadcrumb" aria-label="Breadcrumb">
      <a href="/desk" data-nav data-route="desk">Desk</a>
      ${crumbSep()}
      <span class="current" aria-current="page"><strong>${esc(stageCrumb[state.panel]!)}</strong></span>
    </nav>`;
  }
  const tab = state.openTabs.find((t) => t.id === state.activeId);
  if (!tab) {
    return `<nav class="breadcrumb" id="breadcrumb" aria-label="Breadcrumb">
      <a href="/desk" data-nav data-route="desk">Desk</a>
      ${crumbSep()}
      <span class="current" aria-current="page"><strong>Sessions</strong></span>
    </nav>`;
  }
  const sessHref = sessionPath(tab.cwd, tab.id);
  return `<nav class="breadcrumb" id="breadcrumb" aria-label="Breadcrumb">
    <a href="/desk" data-nav data-route="desk">Desk</a>
    ${crumbSep()}
    <a href="${esc(sessHref)}" data-nav title="${esc(tab.agent)} · ${esc(tab.cwd)}">${esc(basename(tab.cwd))}</a>
    ${crumbSep()}
    <a href="${esc(sessHref)}" data-nav class="current" aria-current="page" title="${esc(tab.id)}"><strong>${esc(tab.title)}</strong></a>
  </nav>`;
}

function sbNavActiveAttr(action: string): string {
  const on =
    (action === "new-session" && state.panel === "new") ||
    (action === "new-project" && state.panel === "new-project") ||
    (action === "projects" && state.panel === "projects") ||
    (action === "tools" && state.panel === "tools") ||
    (action === "git-changes" && state.panel === "changes") ||
    (action === "help" && state.panel === "help") ||
    (action === "open-settings" && state.settingsOpen);
  return on ? ' data-active="true"' : "";
}

function shellHTML(): string {
  const host = (state.statusText || "host").split("·")[0]?.trim() || "host";
  return `
  <div class="shell${state.sidebarOpen ? " sidebar-open" : ""}" data-sidebar="08">
    <div class="sidebar-backdrop" id="sidebar-backdrop" data-action="close-sidebar" hidden></div>

    <aside class="sidebar" id="sidebar" data-sidebar="sidebar">
      <div class="sidebar-inner">
        <header class="sb-head" data-sidebar="header">
          <a href="/desk" class="sb-brand" data-nav data-route="desk" title="Desk" aria-label="agents desk">
            <span class="sb-brand-mark" aria-hidden="true">a</span>
            <span class="sb-brand-name">agents</span>
          </a>
          <a href="/new" class="sb-cta" data-action="new-session" data-nav id="btn-new" title="New session (n)" aria-label="New session"${sbNavActiveAttr("new-session")}>
            ${iconSvg("plus")}<span>New</span><kbd>n</kbd>
          </a>
        </header>

        <div class="sb-group" data-sidebar="group">
          <div class="sb-sessions-head sb-group-head">
            <span class="sb-group-label sb-sessions-label" id="sb-create-label">Create</span>
          </div>
          <nav class="sb-tools" aria-labelledby="sb-create-label">
            <a href="/projects" class="sb-tool" data-action="projects" data-nav id="btn-projects" title="Projects" aria-label="Projects"${sbNavActiveAttr("projects")}>${iconSvg("layers")}<span class="sb-tool-label">Projects</span></a>
            <a href="/project/new" class="sb-tool" data-action="new-project" data-nav id="btn-new-project" title="New project (⇧n)" aria-label="New project"${sbNavActiveAttr("new-project")}>${iconSvg("folder-git")}<span class="sb-tool-label">Clone</span></a>
            <button type="button" class="sb-tool" data-action="open-shell" title="Terminal (⇧t)" aria-label="Open terminal">${iconSvg("terminal")}<span class="sb-tool-label">Shell</span></button>
            <button type="button" class="sb-tool" data-action="open-remote" title="Open remote editor" aria-label="Open remote editor">${iconSvg("external")}<span class="sb-tool-label">Remote</span></button>
          </nav>
        </div>

        <div class="sb-group" data-sidebar="group">
          <div class="sb-sessions-head sb-group-head">
            <span class="sb-group-label sb-sessions-label" id="sb-ws-label">Workspace</span>
          </div>
          <nav class="sb-tools" aria-labelledby="sb-ws-label">
            <a href="/tools" class="sb-tool" data-action="tools" data-nav title="Tools (t)" aria-label="Tools"${sbNavActiveAttr("tools")}>${iconSvg("wrench")}<span class="sb-tool-label">Tools</span></a>
            <a href="/changes" class="sb-tool" data-action="git-changes" data-nav title="Git changes (⇧g)" aria-label="Git changes"${sbNavActiveAttr("git-changes")}>${iconSvg("git-branch")}<span class="sb-tool-label">Git</span></a>
            <a href="/profile" class="sb-tool" data-action="open-settings" data-tab="accounts" data-nav title="Settings (,)" aria-label="Settings"${sbNavActiveAttr("open-settings")}>${iconSvg("settings")}<span class="sb-tool-label">Settings</span></a>
          </nav>
        </div>

        <section class="sb-sessions" data-sidebar="content" aria-label="Sessions">
          <div class="sb-sessions-head">
            <span class="sb-sessions-label" id="sb-sessions-label">Sessions</span>
            <span class="sb-sessions-count" id="sess-count" title="running / total">0/0</span>
          </div>
          <div class="sb-filter-wrap">
            <span class="sb-filter-ico" aria-hidden="true">${iconSvg("search")}</span>
            <input id="filter" class="sb-filter" type="search" placeholder="Filter…" value="${esc(state.filter)}" autocomplete="off" spellcheck="false" aria-label="Filter sessions" aria-controls="session-list" />
          </div>
          <div class="session-list" id="session-list" role="list" aria-labelledby="sb-sessions-label">
            ${sessionListHTML()}
          </div>
        </section>

        <footer class="sb-foot" data-sidebar="footer">
          <button type="button" class="sb-host" data-action="open-settings" data-tab="about" title="Host status &amp; settings" aria-label="Host status and settings"${sbNavActiveAttr("open-settings")}>
            <span class="sb-host-dot" aria-hidden="true"></span>
            <span class="sb-host-text">
              <strong id="nav-host">${esc(host)}</strong>
              <span id="nav-status">${esc(state.statusText || "connected")}</span>
            </span>
            ${iconSvg("chevrons-up-down")}
          </button>
          <div class="sb-foot-actions" role="toolbar" aria-label="Utilities">
            <button type="button" class="sb-tool" data-action="help" title="Shortcuts (?)" aria-label="Keyboard shortcuts"${sbNavActiveAttr("help")}>${iconSvg("help")}</button>
            <button type="button" class="sb-tool" data-action="prune" title="Clear stopped sessions" aria-label="Clear stopped sessions">${iconSvg("trash")}</button>
            <button type="button" class="sb-tool" data-action="logout" title="Log out" aria-label="Log out">${iconSvg("logout")}</button>
          </div>
        </footer>
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
        <div class="tabs" id="tabs" role="tablist" aria-label="Open sessions">${tabsHTML()}</div>
        <div class="topbar-actions">
          <span class="conn-pill conn-${state.conn}" id="conn-pill" title="PTY connection" aria-live="polite"><span class="conn-dot" aria-hidden="true"></span><span class="conn-label">idle</span></span>
          <button type="button" class="tb-search" data-action="open-palette" title="Search (${esc(metaKbd())}K)" aria-label="Search commands and sessions">
            <span class="tb-search-ico" aria-hidden="true">${iconSvg("search")}</span>
            <span class="tb-search-placeholder">Search</span>
            <kbd class="tb-search-kbd"><span class="tb-mod">${esc(metaKbd())}</span>K</kbd>
          </button>
          <button type="button" class="tb-more" data-action="topbar-more" title="More actions" aria-label="More actions" aria-haspopup="menu" aria-expanded="false">
            ${iconSvg("more")}
          </button>
        </div>
      </header>
      <div class="term-wrap">
        ${emptyTermHTML()}
      </div>
      <footer class="status-bar" id="status-bar">${statusHTML()}</footer>
    </main>

    <div id="ctx-menu" class="ctx-menu" hidden role="menu" aria-label="Session actions"></div>
  </div>`;
}

function sessionListHTML(): string {
  if (state.sessions.length === 0) {
    return `<div class="empty-list" role="status">
      <p class="empty-list-title">No sessions</p>
      <p class="empty-list-hint">Start an agent to begin.</p>
      <button type="button" class="primary btn-sm" data-action="new-session">${iconSvg("plus")} New session</button>
    </div>`;
  }
  const sorted = filteredSessionsSorted();
  if (sorted.length === 0) {
    return `<div class="empty-list" role="status">
      <p class="empty-list-title">No matches</p>
      <p class="empty-list-hint">Try a different filter.</p>
    </div>`;
  }
  return sorted
    .map((s) => {
      const isActive = s.id === state.activeId;
      const cursor = s.id === state.listCursorId ? " cursor" : "";
      const ag = agentClass(s.agent);
      const href = sessionPath(s.cwd, s.id);
      const label = s.name || shortId(s.id);
      const live = s.state === "running" ? "live" : "dead";
      const branch = (s.git_branch || s.branch || "").trim();
      const cwdBase = basename(s.cwd);
      const age = relativeTime(s.created_at);
      const tipBits = [s.agent, s.cwd];
      if (branch) tipBits.push(branch);
      if (age) tipBits.push(age);
      tipBits.push(s.state, "right-click for actions");
      const stateLabel =
        s.state === "running" ? "Live" : s.state === "exited" ? "Exited" : s.state;
      const activeAttr = isActive ? ' data-active="true" aria-current="page"' : "";
      return `<a class="session-item ${live}${isActive ? " active" : ""}${cursor} ${ag}" href="${esc(href)}" role="listitem" data-open="${esc(s.id)}" data-state="${esc(s.state)}" data-nav${activeAttr} title="${esc(tipBits.join(" · "))}">
  <span class="session-dot ${ag}" aria-hidden="true"></span>
  <span class="session-body">
    <span class="session-name">${esc(label)}</span>
    <span class="session-meta"><span class="session-agent">${esc(s.agent)}</span><span class="session-sep" aria-hidden="true"> · </span><span class="session-cwd">${esc(cwdBase)}</span>${
      branch
        ? `<span class="session-sep" aria-hidden="true"> · </span><span class="session-branch">${esc(branch)}</span>`
        : ""
    }</span>
  </span>
  <span class="session-state ${esc(s.state)}" title="${esc(s.state)}">${esc(stateLabel)}</span>
  <button type="button" class="session-more" data-ctx="${esc(s.id)}" title="Actions" aria-label="Actions for ${esc(label)}" tabindex="-1">${iconSvg("more")}</button>
</a>`;
    })
    .join("");
}

function hideSessionCtx(): void {
  ctxSessionId = null;
  const menu = document.getElementById("ctx-menu");
  if (!menu) return;
  menu.hidden = true;
  menu.innerHTML = "";
}

function showSessionCtx(id: string, x: number, y: number): void {
  const s = state.sessions.find((x) => x.id === id);
  if (!s) return;
  const menu = document.getElementById("ctx-menu");
  if (!menu) return;
  ctxSessionId = id;
  const running = s.state === "running";
  const label = s.name || shortId(s.id);
  menu.innerHTML = `
    <div class="ctx-label" role="presentation">${esc(label)}</div>
    <button type="button" role="menuitem" data-ctx-act="open" data-ctx-id="${esc(id)}">Open</button>
    ${
      running
        ? `<button type="button" role="menuitem" data-ctx-act="stop" data-ctx-id="${esc(id)}">Stop</button>`
        : `<button type="button" role="menuitem" data-ctx-act="resume" data-ctx-id="${esc(id)}">Resume</button>`
    }
    <button type="button" role="menuitem" data-ctx-act="copy-id" data-ctx-id="${esc(id)}">Copy ID</button>
    <div class="ctx-sep" role="separator"></div>
    <button type="button" role="menuitem" class="danger-text" data-ctx-act="delete" data-ctx-id="${esc(id)}">Delete</button>
  `;
  menu.hidden = false;
  // Measure then clamp to viewport
  const pad = 8;
  const rect = menu.getBoundingClientRect();
  const w = rect.width || 180;
  const h = rect.height || 160;
  let left = x;
  let top = y;
  if (left + w > window.innerWidth - pad) left = window.innerWidth - w - pad;
  if (top + h > window.innerHeight - pad) top = window.innerHeight - h - pad;
  if (left < pad) left = pad;
  if (top < pad) top = pad;
  menu.style.left = `${left}px`;
  menu.style.top = `${top}px`;
}

async function runSessionCtx(act: string, id: string): Promise<void> {
  hideSessionCtx();
  const s = state.sessions.find((x) => x.id === id);
  switch (act) {
    case "open":
      if (s) openTab(s);
      break;
    case "stop":
      await onKillSession(id);
      break;
    case "resume":
      await onResumeSession(id);
      break;
    case "delete":
      await onDeleteSession(id);
      break;
    case "copy-id":
      try {
        await navigator.clipboard.writeText(id);
        toast("Session ID copied", "ok", 1400);
      } catch {
        toast(id, "info", 4000);
      }
      break;
    default:
      break;
  }
}

function tabsHTML(): string {
  if (state.openTabs.length === 0) {
    return `<div class="tabs-empty" role="presentation">No open tabs</div>`;
  }
  return state.openTabs
    .map((t, i) => {
      const isActive = t.id === state.activeId;
      const active = isActive ? "active" : "";
      const live = t.state === "running" ? "live" : "dead";
      const ag = agentClass(t.agent);
      const href = sessionPath(t.cwd, t.id);
      const sess = state.sessions.find((x) => x.id === t.id);
      const branch = (sess?.git_branch || sess?.branch || "").trim();
      // Branch only on hover title — keep tab label quiet.
      const tip = branch
        ? `${t.agent} · ${branch} · ${t.id}`
        : `${t.agent} · ${t.id}`;
      const num =
        i < 9
          ? `<span class="tab-num" aria-hidden="true">${i + 1}</span>`
          : "";
      const closeIco =
        '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>';
      return `<a class="tab ${active} ${live} ${ag}" href="${esc(href)}" role="tab" aria-selected="${isActive}" tabindex="${isActive ? "0" : "-1"}" data-tab="${esc(t.id)}" data-nav title="${esc(tip)}"><span class="tab-dot" aria-hidden="true" title="${esc(t.agent)}"></span><span class="tab-label">${esc(t.title)}</span>${num}<button type="button" class="tab-close" data-close="${esc(t.id)}" title="Close tab (keeps agent running)" aria-label="Close tab">${closeIco}</button></a>`;
    })
    .join("");
}

function statusHTML(): string {
  const cls = state.statusOk ? "ok" : "err";
  const open = state.openTabs.length;
  const total = state.sessions.length;
  const run = state.sessions.filter((s) => s.state === "running").length;
  const host = (state.statusText || "").split("·")[0]?.trim() || "";
  return `
    <span class="status-dot ${cls}" title="${state.statusOk ? "Host ok" : "Host error"}" aria-hidden="true"></span>
    <span class="status-metric"><strong>${run}</strong> live</span>
    <span class="sep" aria-hidden="true">·</span>
    <span class="status-metric"><strong>${open}</strong> open</span>
    <span class="sep" aria-hidden="true">·</span>
    <span class="status-metric"><strong>${total}</strong> total</span>
    ${host ? `<span class="sep" aria-hidden="true">·</span><span class="status-metric status-host" title="${esc(state.statusText || "")}">${esc(host)}</span>` : ""}
    <span class="status-hint">tab close detaches</span>
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
    toast("No open tabs — open a session first", "info", 1600);
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
  if (!state.openTabs.length) {
    toast("No open tabs", "info", 1400);
    return;
  }
  if (n === 0) {
    const last = state.openTabs[state.openTabs.length - 1];
    if (last) void activateTab(last.id);
    return;
  }
  const tab = state.openTabs[n - 1];
  if (tab) void activateTab(tab.id);
  else toast(`No tab ${n}`, "info", 1200);
}

function jumpTabEdge(which: "first" | "last"): void {
  const tabs = state.openTabs;
  if (!tabs.length) {
    toast("No open tabs", "info", 1400);
    return;
  }
  const t = which === "first" ? tabs[0] : tabs[tabs.length - 1];
  void activateTab(t.id);
}

/** Move list cursor and open (or prompt to resume) — quick session switch. */
function stepSessionAndOpen(delta: number): void {
  moveListCursor(delta);
  const id = state.listCursorId;
  if (!id) return;
  const s = state.sessions.find((x) => x.id === id);
  if (!s) return;
  if (s.state === "running") {
    openTab(s);
    return;
  }
  // Open stopped session into a tab so user can resume with e
  openTab(s);
}

async function copyTargetSessionId(): Promise<void> {
  const id = targetSessionId();
  if (!id) {
    toast("No session to copy", "info", 1400);
    return;
  }
  try {
    await navigator.clipboard.writeText(id);
    toast(`Copied ${id.slice(0, 12)}…`, "ok", 1600);
  } catch {
    toast(id, "info", 6000);
  }
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
      if (shift) {
        openPanel("changes");
        return true;
      }
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
      if (shift) {
        stepSessionAndOpen(1);
        return true;
      }
      moveListCursor(1);
      return true;
    case "ArrowUp":
      if (shift) {
        stepSessionAndOpen(-1);
        return true;
      }
      moveListCursor(-1);
      return true;
    case "ArrowLeft":
      cycleTab(-1);
      return true;
    case "ArrowRight":
      cycleTab(1);
      return true;
    case "Home":
      jumpTabEdge("first");
      return true;
    case "End":
      jumpTabEdge("last");
      return true;
    case "PageDown":
      cycleTab(1);
      return true;
    case "PageUp":
      cycleTab(-1);
      return true;
    case " ":
    case "Spacebar":
      // Space opens highlighted session (like Enter)
      openCursorOrActive();
      return true;
    case "Enter":
      if (ev.altKey) {
        focusTerminal();
        return true;
      }
      openCursorOrActive();
      return true;
    case "Tab":
      // Bare Tab cycles open tabs when not typing (see bindKeys for Ctrl+Tab)
      cycleTab(shift ? -1 : 1);
      return true;
    default:
      break;
  }

  switch (lower) {
    case "n":
      if (shift) {
        openPanel("new-project");
        return true;
      }
      openPanel("new");
      return true;
    case "t":
      if (shift) {
        void openShellTerminal();
        return true;
      }
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
      if (shift) {
        openPanel("changes");
        return true;
      }
      openSettings("github");
      return true;
    case "k":
      if (shift) {
        // Shift+K: step up list and open
        stepSessionAndOpen(-1);
        return true;
      }
      moveListCursor(-1);
      return true;
    case "j":
      if (shift) {
        // Shift+J: step down list and open
        stepSessionAndOpen(1);
        return true;
      }
      moveListCursor(1);
      return true;
    case "h":
      if (shift) {
        jumpTabEdge("first");
        return true;
      }
      cycleTab(-1);
      return true;
    case "l":
      if (shift) {
        jumpTabEdge("last");
        return true;
      }
      cycleTab(1);
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
      if (shift) {
        jumpTabEdge("first");
        return true;
      }
      cycleTab(-1);
      return true;
    case "]":
      if (shift) {
        jumpTabEdge("last");
        return true;
      }
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
      if (shift) {
        openSettings("ssh");
        return true;
      }
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
      // bare d → desk (no modal); keep active session tabs
      if (state.panel) void closePanel();
      if (state.settingsOpen) void closeSettingsOnly();
      navigate({ name: "desk" }, { apply: false });
      syncUrlFromUI({ replace: true });
      return true;
    case "c":
      void onPrune();
      return true;
    case "y":
      void copyTargetSessionId();
      return true;
    case "r":
      void refreshSessionsManual();
      return true;
    case "p":
      openCommandPalette();
      return true;
    case "i":
    case "`":
      focusTerminal();
      return true;
    case "u":
      // "undo" focus — blur term / arm shortcuts
      (document.activeElement as HTMLElement | null)?.blur?.();
      toast("Shortcuts armed — j/k sessions · h/l tabs · ?", "info", 2000);
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
      if (state.commandPalette) {
        closeCommandPalette();
        ev.preventDefault();
        return;
      }
      const moreOpen = document.getElementById("topbar-more-menu");
      if (moreOpen && !moreOpen.hidden) {
        hideTopbarMore();
        ev.preventDefault();
        return;
      }
      if (state.drawer) {
        state.drawer = null;
        void paintDrawer();
        ev.preventDefault();
        return;
      }
      // Full-page git or any panel
      if (state.panel) {
        void closePanel();
        ev.preventDefault();
        return;
      }
      if (state.settingsOpen) {
        void closeSettings();
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

    // Spotlight keyboard navigation (works while input is focused)
    if (state.commandPalette) {
      if (ev.key === "ArrowDown" || (ev.key === "n" && ev.ctrlKey)) {
        ev.preventDefault();
        const n = paletteItems().length;
        if (n) {
          state.paletteIndex = (state.paletteIndex + 1) % n;
          paintCommandPalette();
          (document.getElementById("palette-input") as HTMLInputElement | null)?.focus();
        }
        return;
      }
      if (ev.key === "ArrowUp" || (ev.key === "p" && ev.ctrlKey)) {
        ev.preventDefault();
        const n = paletteItems().length;
        if (n) {
          state.paletteIndex = (state.paletteIndex - 1 + n) % n;
          paintCommandPalette();
          (document.getElementById("palette-input") as HTMLInputElement | null)?.focus();
        }
        return;
      }
      if (ev.key === "Enter") {
        ev.preventDefault();
        runPaletteIndex(state.paletteIndex);
        return;
      }
      // Ctrl/Cmd+K while open still toggles closed below
    }

    const typing = isTypingTarget(ev.target);
    const inTerm = isXtermTarget(ev.target);

    // Form fields: only Escape (above) + spotlight nav. Don't steal typing.
    if (typing && !inTerm && !state.commandPalette) return;

    // Spotlight: Ctrl/Cmd+K (and Ctrl/Cmd+P)
    if (
      (ev.metaKey || ev.ctrlKey) &&
      !ev.altKey &&
      (ev.key === "k" || ev.key === "K" || ev.key === "p" || ev.key === "P")
    ) {
      ev.preventDefault();
      if (state.commandPalette) closeCommandPalette();
      else openCommandPalette();
      return;
    }

    // If spotlight is open, don't run bare shortcuts over the search field
    if (state.commandPalette) return;

    // Tab switching with modifiers — works even over TTY (agent doesn't get these)
    if (ev.metaKey || ev.ctrlKey) {
      // Ctrl/Cmd+Tab · Ctrl/Cmd+Shift+Tab
      if (ev.key === "Tab") {
        ev.preventDefault();
        cycleTab(ev.shiftKey ? -1 : 1);
        return;
      }
      // Ctrl/Cmd+PageDown / PageUp
      if (ev.key === "PageDown") {
        ev.preventDefault();
        cycleTab(1);
        return;
      }
      if (ev.key === "PageUp") {
        ev.preventDefault();
        cycleTab(-1);
        return;
      }
      // Ctrl/Cmd+1…9 / 0 — jump open tab (browser-style)
      if (ev.key >= "0" && ev.key <= "9" && !ev.altKey) {
        ev.preventDefault();
        jumpTab(Number(ev.key));
        return;
      }
      // Ctrl/Cmd+[ / ] — prev/next tab
      if (ev.key === "[" || ev.key === "]") {
        ev.preventDefault();
        cycleTab(ev.key === "[" ? -1 : 1);
        return;
      }
      // leave other Ctrl/Cmd to browser / xterm (copy, paste, …)
      return;
    }

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
    if (handleShortcut(ev, false)) {
      ev.preventDefault();
    }
  });
}

/** Document-level delegation — survives innerHTML rewrites of lists/tabs/modals. */
function ensureUIDelegation(): void {
  if (uiDelegated) return;
  uiDelegated = true;

  // Session context menu (right-click)
  document.addEventListener("contextmenu", (ev) => {
    const t = ev.target as HTMLElement | null;
    if (!t) return;
    if (t.closest("#ctx-menu")) {
      ev.preventDefault();
      return;
    }
    const row = t.closest<HTMLElement>(".session-item[data-open]");
    if (!row) {
      hideSessionCtx();
      return;
    }
    ev.preventDefault();
    const id = row.getAttribute("data-open");
    if (id) showSessionCtx(id, ev.clientX, ev.clientY);
  });

  document.addEventListener(
    "keydown",
    (ev) => {
      if (ev.key === "Escape" && ctxSessionId) {
        hideSessionCtx();
      }
    },
    true,
  );

  document.addEventListener("click", (ev) => {
    const t = ev.target as HTMLElement | null;
    if (!t) return;

    // Tab close buttons (delegated — survives tabs.innerHTML rewrites)
    const closeBtn = t.closest<HTMLElement>("[data-close]");
    if (closeBtn) {
      ev.preventDefault();
      ev.stopPropagation();
      const id = closeBtn.getAttribute("data-close");
      if (id) closeTab(id);
      return;
    }

    // Context menu actions
    const ctxAct = t.closest<HTMLElement>("[data-ctx-act]");
    if (ctxAct) {
      ev.preventDefault();
      ev.stopPropagation();
      const act = ctxAct.getAttribute("data-ctx-act") || "";
      const id = ctxAct.getAttribute("data-ctx-id") || ctxSessionId || "";
      if (act && id) void runSessionCtx(act, id);
      return;
    }

    // ⋯ button on session row → open menu at button
    const moreBtn = t.closest<HTMLElement>("[data-ctx]");
    if (moreBtn) {
      ev.preventDefault();
      ev.stopPropagation();
      const id = moreBtn.getAttribute("data-ctx");
      if (id) {
        const r = moreBtn.getBoundingClientRect();
        showSessionCtx(id, r.right, r.bottom + 4);
      }
      return;
    }

    // Click outside closes context menu / topbar more
    if (ctxSessionId && !t.closest("#ctx-menu")) {
      hideSessionCtx();
    }
    const moreMenu = document.getElementById("topbar-more-menu");
    if (
      moreMenu &&
      !moreMenu.hidden &&
      !t.closest("#topbar-more-menu") &&
      !t.closest("[data-action='topbar-more']")
    ) {
      hideTopbarMore();
    }

    // In-app <a data-nav> links (session list, breadcrumbs, etc.)
    const navEl = t.closest<HTMLAnchorElement>("a[data-nav]");
    if (navEl && navEl.href && !ev.metaKey && !ev.ctrlKey && !ev.shiftKey && ev.button === 0) {
      // Don't hijack action buttons / menus inside the link
      if (
        t.closest(
          "button, summary, details.menu, [data-kill], [data-delete], [data-resume], [data-action], [data-ctx], [data-ctx-act], [data-close]",
        )
      ) {
        /* fall through to action handlers */
      } else {
        const url = new URL(navEl.href, window.location.origin);
        if (url.origin === window.location.origin) {
          ev.preventDefault();
          const route = parsePathSafe(url.pathname);
          navigate(route);
          return;
        }
      }
    }

    // Session menus live inside <a data-nav> — don't navigate when using the menu
    if (t.closest("a[data-nav] details.menu")) {
      ev.preventDefault();
    }

    const actionEl = t.closest<HTMLElement>("[data-action]");
    if (!actionEl) return;
    // don't steal clicks from disabled controls
    if (actionEl.hasAttribute("disabled") || (actionEl as HTMLButtonElement).disabled) return;
    const action = actionEl.getAttribute("data-action");
    if (!action) return;

    // Any menu action closes the portaled ⋮ menu (except the toggle itself)
    if (action !== "topbar-more") hideTopbarMore();

    switch (action) {
      case "new-session":
        ev.preventDefault();
        ev.stopPropagation();
        if (!state.token) return;
        openPanel("new");
        break;
      case "new-project":
        ev.preventDefault();
        ev.stopPropagation();
        if (!state.token) return;
        openPanel("new-project");
        break;
      case "tools":
        ev.preventDefault();
        ev.stopPropagation();
        if (!state.token) return;
        openPanel("tools");
        break;
      case "git-changes":
        ev.preventDefault();
        ev.stopPropagation();
        if (!state.token) return;
        openPanel("changes");
        break;
      case "git-refresh":
        ev.preventDefault();
        void refreshGitChanges();
        break;
      case "git-select-all":
        ev.preventDefault();
        void onGitSelectFile(null);
        break;
      case "git-select-file": {
        ev.preventDefault();
        const path = actionEl.getAttribute("data-path") || "";
        void onGitSelectFile(path || null);
        break;
      }
      case "git-toggle-pr":
        ev.preventDefault();
        syncGitFormFromDOM();
        state.gitPrOpen = !state.gitPrOpen;
        paintGitPage();
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
      case "close-git-page":
        ev.preventDefault();
        ev.stopPropagation();
        void closePanel();
        break;
      case "settings-tab":
        ev.preventDefault();
        {
          const tab = actionEl.getAttribute("data-tab") as SettingsTab | null;
          if (tab) {
            state.settingsTab = tab;
            navigate({ name: "profile", tab }, { apply: false });
            void loadSettingsData().then(() => paintSettings());
          }
        }
        break;
      case "settings-platform":
        ev.preventDefault();
        state.settingsPlatform = actionEl.getAttribute("data-platform") || "grok";
        // Persist id/label draft fields
        {
          const idEl = document.getElementById("settings-acct-id") as HTMLInputElement | null;
          const labelEl = document.getElementById(
            "settings-acct-label",
          ) as HTMLInputElement | null;
          if (idEl) state.settingsAcctId = idEl.value;
          if (labelEl) state.settingsAcctLabel = labelEl.value;
        }
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
      case "acct-remove": {
        ev.preventDefault();
        const platform = actionEl.getAttribute("data-platform") || "";
        const id = actionEl.getAttribute("data-id") || "";
        if (platform && id) void onAcctRemove(platform, id);
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
        if (isAppDrawerOpen()) closeAppDrawer("programmatic");
        void paintDrawer();
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
      case "open-shell":
        ev.preventDefault();
        document.querySelectorAll("details.menu[open]").forEach((d) => d.removeAttribute("open"));
        void openShellTerminal();
        break;
      case "open-remote":
        ev.preventDefault();
        document.querySelectorAll("details.menu[open]").forEach((d) => d.removeAttribute("open"));
        void showOpenRemoteDrawer();
        break;
      case "remote-refresh":
        ev.preventDefault();
        void loadRemotePage();
        break;
      case "projects":
        ev.preventDefault();
        ev.stopPropagation();
        if (!state.token) return;
        openPanel("projects");
        break;
      case "projects-refresh":
        ev.preventDefault();
        void loadProjectsPage();
        break;
      case "project-new-session": {
        ev.preventDefault();
        const cwd = actionEl.getAttribute("data-cwd") || ".";
        state.formCwd = cwd;
        state.formCwdNew = false;
        openPanel("new");
        break;
      }
      case "project-tools": {
        ev.preventDefault();
        const cwd = actionEl.getAttribute("data-cwd") || ".";
        state.formCwd = cwd;
        openPanel("tools");
        break;
      }
      case "project-git": {
        ev.preventDefault();
        const cwd = actionEl.getAttribute("data-cwd") || ".";
        state.formCwd = cwd;
        openPanel("changes");
        break;
      }
      case "project-remote": {
        ev.preventDefault();
        const cwd = actionEl.getAttribute("data-cwd") || ".";
        void showOpenRemoteDrawer(cwd);
        break;
      }
      case "copy-text": {
        ev.preventDefault();
        const text = actionEl.getAttribute("data-text") || "";
        if (!text) break;
        void navigator.clipboard.writeText(text).then(
          () => toast("Copied", "ok", 1400),
          () => toast("Copy failed", "err"),
        );
        break;
      }
      case "open-palette":
        ev.preventDefault();
        ev.stopPropagation();
        document.querySelectorAll("details.menu[open]").forEach((d) => d.removeAttribute("open"));
        hideTopbarMore();
        openCommandPalette();
        break;
      case "topbar-more":
        ev.preventDefault();
        ev.stopPropagation();
        document.querySelectorAll("details.menu[open]").forEach((d) => d.removeAttribute("open"));
        toggleTopbarMore(actionEl);
        break;
      case "toggle-theme":
        ev.preventDefault();
        document.querySelectorAll("details.menu[open]").forEach((d) => d.removeAttribute("open"));
        hideTopbarMore();
        toggleTheme();
        if (state.settingsOpen && state.settingsTab === "about") paintSettings();
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
      case "task-toggle": {
        ev.preventDefault();
        const id = actionEl.getAttribute("data-id") || "";
        const st = (actionEl.getAttribute("data-status") || "todo") as TaskStatus;
        if (id) void onTaskToggle(id, st);
        break;
      }
      case "task-cycle": {
        ev.preventDefault();
        const id = actionEl.getAttribute("data-id") || "";
        const st = (actionEl.getAttribute("data-status") || "todo") as TaskStatus;
        if (id) void onTaskCycle(id, st);
        break;
      }
      case "task-delete": {
        ev.preventDefault();
        const id = actionEl.getAttribute("data-id") || "";
        if (id) void onTaskDelete(id);
        break;
      }
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
    } else if (kind === "create-project") {
      ev.preventDefault();
      void onCreateProject(ev);
    } else if (kind === "mem-search") {
      ev.preventDefault();
      void onMemSearch(ev);
    } else if (kind === "task-add") {
      ev.preventDefault();
      void onTaskAdd(ev);
    } else if (kind === "git-commit") {
      ev.preventDefault();
      void onGitCommit();
    } else if (kind === "git-pr") {
      ev.preventDefault();
      void onGitCreatePR();
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
    if (kind === "git-cwd" && el instanceof HTMLSelectElement) {
      state.formCwd = el.value;
      state.gitSelectedPath = null;
      state.gitCheckedPaths = [];
      state.gitDiffText = "";
      state.gitPrUrl = "";
      void refreshGitChanges();
    }
    if (kind === "remote-cwd" && el instanceof HTMLSelectElement) {
      state.formCwd = el.value;
      void loadRemotePage();
    }
    if (kind === "sess-cwd-mode" && el instanceof HTMLInputElement) {
      state.formCwdNew = el.value === "new";
      const nameEl = document.getElementById("sess-cwd-new-name") as HTMLInputElement | null;
      if (nameEl) state.formCwdNewName = nameEl.value;
      // Toggle fields without wiping the rest of the form
      const existing = document.getElementById("sess-cwd-existing");
      const neu = document.getElementById("sess-cwd-new");
      const sel = document.getElementById("sess-cwd") as HTMLSelectElement | null;
      const nameInput = document.getElementById("sess-cwd-new-name") as HTMLInputElement | null;
      if (existing) existing.hidden = state.formCwdNew;
      if (neu) neu.hidden = !state.formCwdNew;
      if (sel) {
        if (state.formCwdNew) sel.removeAttribute("required");
        else sel.setAttribute("required", "");
      }
      if (nameInput) {
        if (state.formCwdNew) nameInput.setAttribute("required", "");
        else nameInput.removeAttribute("required");
        if (state.formCwdNew) nameInput.focus();
      }
      const submit = document.querySelector(
        '#create-form button[type="submit"]',
      ) as HTMLButtonElement | null;
      if (submit && !state.creating) {
        submit.textContent = state.formCwdNew ? "Create dir & start" : "Start session";
      }
    }
    if (kind === "git-check" && el instanceof HTMLInputElement) {
      const path = el.getAttribute("data-path") || "";
      if (!path) return;
      const files = (state.gitStatus?.files || []).map((f) => f.path).filter(Boolean);
      // empty list means "all selected" — materialize before a deselect
      let checked =
        state.gitCheckedPaths.length === 0 ? [...files] : [...state.gitCheckedPaths];
      if (el.checked) {
        if (!checked.includes(path)) checked.push(path);
      } else {
        checked = checked.filter((p) => p !== path);
      }
      // collapse full selection back to "all"
      state.gitCheckedPaths = checked.length === files.length ? [] : checked;
      const scopeEl = document.querySelector("[data-git-commit-scope]");
      if (scopeEl) scopeEl.textContent = gitCommitScopeLabel();
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
    if (filterDebounceTimer != null) window.clearTimeout(filterDebounceTimer);
    filterDebounceTimer = window.setTimeout(() => {
      filterDebounceTimer = null;
      // Sig includes filter; no force needed — only rewrites when matches change.
      paintSessionList();
    }, 120);
  });

  // Tab close is delegated in ensureUIDelegation (no per-paint rebind).
  window.addEventListener("resize", () => {
    fit();
    hideSessionCtx();
  });
}

// ── SPA update detection ─────────────────────────────────────────────────────
// After agentsd self-update, a normal refresh must pick up the new UI.
// Server serves index.html with Cache-Control: no-store; we also poll the shell
// stamp and soft-reload when the running daemon ships a different build.

function currentWebBuildStamp(): string {
  const meta = document.querySelector('meta[name="agents-build"]');
  const fromMeta = meta?.getAttribute("content")?.trim() || "";
  if (fromMeta) return fromMeta;
  const scripts = Array.from(document.querySelectorAll("script[src]"));
  for (const s of scripts) {
    const src = s.getAttribute("src") || "";
    const m = src.match(/index-([A-Za-z0-9_-]+)\.js/);
    if (m) return m[1];
  }
  const links = Array.from(document.querySelectorAll('link[rel="stylesheet"][href]'));
  for (const l of links) {
    const href = l.getAttribute("href") || "";
    const m = href.match(/index-([A-Za-z0-9_-]+)\.css/);
    if (m) return m[1];
  }
  return "";
}

let bootWebStamp = currentWebBuildStamp();
let webUpdateCheckBusy = false;
let webUpdateReloadArmed = false;

async function checkForWebUpdate(reason: string): Promise<void> {
  if (webUpdateCheckBusy || webUpdateReloadArmed) return;
  webUpdateCheckBusy = true;
  try {
    // Bust any intermediary cache; server also sends no-store for index.
    const res = await fetch(`/?_wb=${Date.now()}`, {
      method: "GET",
      cache: "no-store",
      headers: { Accept: "text/html", "Cache-Control": "no-cache" },
      credentials: "same-origin",
    });
    if (!res.ok) return;
    const html = await res.text();
    const metaM = html.match(/name=["']agents-build["']\s+content=["']([^"']+)["']/i)
      || html.match(/content=["']([^"']+)["']\s+name=["']agents-build["']/i);
    const headerBuild = res.headers.get("X-Agents-Build") || "";
    const assetM = html.match(/\/assets\/index-([A-Za-z0-9_-]+)\.js/);
    const remote =
      (metaM && metaM[1]) || headerBuild || (assetM && assetM[1]) || "";
    if (!remote) return;
    if (!bootWebStamp) {
      bootWebStamp = remote;
      return;
    }
    if (remote !== bootWebStamp) {
      webUpdateReloadArmed = true;
      // Soft reload is enough once index is no-store.
      console.info(`[agents] web UI updated (${reason}): ${bootWebStamp} → ${remote}`);
      window.location.reload();
    }
  } catch {
    /* offline / transient — ignore */
  } finally {
    webUpdateCheckBusy = false;
  }
}

function startWebUpdateWatcher(): void {
  // When tab becomes visible again (e.g. after agentsd update + user returns).
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") void checkForWebUpdate("visible");
  });
  // BFCache restore
  window.addEventListener("pageshow", (ev) => {
    if (ev.persisted) void checkForWebUpdate("pageshow");
  });
  // Focus of the window
  window.addEventListener("focus", () => {
    void checkForWebUpdate("focus");
  });
  // Periodic check for long-lived tabs (daemon may have self-updated).
  window.setInterval(() => {
    if (document.visibilityState === "visible") void checkForWebUpdate("poll");
  }, 60_000);
  // One check shortly after boot (covers mid-deploy race).
  window.setTimeout(() => void checkForWebUpdate("boot"), 4_000);
}

// boot — agentsctl web passes #token=… for one-shot login
{
  const fromURL = consumeAuthFromURL();
  if (fromURL) {
    state.token = fromURL;
  }
}
startWebUpdateWatcher();
if (state.token) {
  void bootstrapAuthed();
} else {
  paint();
}
