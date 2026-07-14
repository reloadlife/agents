/** REST client for agentsd /v1 API. */

export type Session = {
  id: string;
  name?: string;
  agent: string;
  mode: string;
  cwd: string;
  cwd_abs?: string;
  tmux?: string;
  state: string;
  prompt?: string;
  created_at?: string;
  pty_path?: string;
  attach_hint?: string;
};

export type AgentInfo = {
  name: string;
  bin?: string;
  available?: boolean;
  tty?: boolean;
};

export type StatusSnapshot = {
  hostname?: string;
  uptime?: string;
  load?: unknown;
  disk?: unknown;
  tty_sessions?: number;
  agents?: AgentInfo[];
  [key: string]: unknown;
};

const TOKEN_KEY = "agentsd_token";

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? "";
}

export function setToken(token: string): void {
  if (token) {
    localStorage.setItem(TOKEN_KEY, token);
  } else {
    localStorage.removeItem(TOKEN_KEY);
  }
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers = new Headers(init.headers);
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(path, { ...init, headers });
  if (res.status === 401) {
    const err = new Error("unauthorized");
    (err as Error & { status: number }).status = 401;
    throw err;
  }
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const j = (await res.json()) as { error?: string };
      if (j.error) msg = j.error;
    } catch {
      /* ignore */
    }
    const err = new Error(msg);
    (err as Error & { status: number }).status = res.status;
    throw err;
  }
  if (res.status === 204) {
    return undefined as T;
  }
  return (await res.json()) as T;
}

export function listSessions(): Promise<{ sessions: Session[] }> {
  return request("/v1/sessions");
}

export function createSession(body: {
  agent: string;
  cwd: string;
  name?: string;
  prompt?: string;
  account?: string;
  account_mode?: string;
}): Promise<Session> {
  return request("/v1/sessions", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export type AgentAccount = {
  id: string;
  label: string;
  email?: string;
  saved?: boolean;
  active?: boolean;
  saved_at?: string;
};

export type AgentPlatformStatus = {
  platform: string;
  current?: string;
  active?: string;
  accounts: AgentAccount[];
};

export function listAgentAccounts(
  platform?: string,
): Promise<AgentPlatformStatus | { platforms: AgentPlatformStatus[]; bin?: string }> {
  const q = platform ? `?platform=${encodeURIComponent(platform)}` : "?platform=all";
  return request(`/v1/agent-accounts${q}`);
}

export function saveAgentAccount(body: {
  platform: string;
  id: string;
  label?: string;
}): Promise<{ ok: boolean; status?: AgentPlatformStatus }> {
  return request("/v1/agent-accounts/save", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function switchAgentAccount(body: {
  platform: string;
  id: string;
}): Promise<{ ok: boolean; status?: AgentPlatformStatus }> {
  return request("/v1/agent-accounts/switch", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function killSession(id: string): Promise<Session> {
  return request(`/v1/sessions/${encodeURIComponent(id)}/kill`, {
    method: "POST",
    body: "{}",
  });
}

/** Stop agent (if running) and remove session from the server list. */
export function deleteSession(id: string): Promise<{ ok: boolean; id: string; deleted: boolean }> {
  return request(`/v1/sessions/${encodeURIComponent(id)}/delete`, {
    method: "POST",
    body: "{}",
  });
}

/** Re-attach if tmux still alive; otherwise restart agent with same id/agent/cwd. */
export function resumeSession(id: string): Promise<Session> {
  return request(`/v1/sessions/${encodeURIComponent(id)}/resume`, {
    method: "POST",
    body: "{}",
  });
}

/** Terminal scrollback (live tmux or last on-disk snapshot). May include ANSI. */
export async function sessionHistory(id: string): Promise<{ text: string; source: string }> {
  const token = getToken();
  const headers = new Headers();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const res = await fetch(`/v1/sessions/${encodeURIComponent(id)}/history`, { headers });
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const j = (await res.json()) as { error?: string };
      if (j.error) msg = j.error;
    } catch {
      /* ignore */
    }
    const err = new Error(msg);
    (err as Error & { status: number }).status = res.status;
    throw err;
  }
  const text = await res.text();
  return {
    text,
    source: res.headers.get("X-Agents-History-Source") || "unknown",
  };
}

export function pruneSessions(maxAge?: string): Promise<{ removed: number }> {
  return request("/v1/sessions/prune", {
    method: "POST",
    body: JSON.stringify(maxAge ? { max_age: maxAge } : {}),
  });
}

export type PlaywrightStatus = {
  running?: boolean;
  display?: string;
  [key: string]: unknown;
};

export function playwrightStatus(): Promise<PlaywrightStatus> {
  return request("/v1/playwright");
}

export function playwrightStart(): Promise<{ ok: boolean; error?: string; status?: PlaywrightStatus }> {
  return request("/v1/playwright/start", { method: "POST", body: "{}" });
}

export function playwrightStop(): Promise<{ ok: boolean; error?: string; status?: PlaywrightStatus }> {
  return request("/v1/playwright/stop", { method: "POST", body: "{}" });
}

export function playwrightRestart(): Promise<{ ok: boolean; error?: string; status?: PlaywrightStatus }> {
  return request("/v1/playwright/restart", { method: "POST", body: "{}" });
}

export function listAgents(): Promise<{
  agents: AgentInfo[];
  available: string[];
}> {
  return request("/v1/agents");
}

export type WorkspaceEntry = {
  path: string;
  abs?: string;
  exists?: boolean;
  is_dir?: boolean;
  default?: boolean;
};

export function listWorkspaces(): Promise<{
  workspace_root: string;
  default_cwd: string;
  workspaces: Array<string | WorkspaceEntry>;
  paths?: string[];
}> {
  return request("/v1/workspaces");
}

export type SSHKey = {
  name: string;
  path?: string;
  public_path?: string;
  has_private?: boolean;
  has_public?: boolean;
  public_key?: string;
  fingerprint?: string;
  comment?: string;
  type?: string;
  bits?: string;
  protected?: boolean;
};

export function listSSHKeys(): Promise<{ dir: string; keys: SSHKey[] }> {
  return request("/v1/ssh-keys");
}

export function getSSHKey(name: string): Promise<SSHKey> {
  return request(`/v1/ssh-keys/${encodeURIComponent(name)}`);
}

export function generateSSHKey(body: {
  name: string;
  type?: string;
  comment?: string;
  overwrite?: boolean;
}): Promise<SSHKey> {
  return request("/v1/ssh-keys", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function deleteSSHKey(name: string): Promise<{ ok: boolean; name: string; deleted: boolean }> {
  return request(`/v1/ssh-keys/${encodeURIComponent(name)}/delete`, {
    method: "POST",
    body: "{}",
  });
}

export type GHAccount = {
  host: string;
  user: string;
  active: boolean;
  git_protocol?: string;
  scopes?: string[];
  config_file?: string;
};

export type GHStatus = {
  ok: boolean;
  binary?: string;
  accounts: GHAccount[];
  active?: string;
  error?: string;
};

export function ghAccounts(): Promise<GHStatus> {
  return request("/v1/gh/accounts");
}

export function ghLogin(body: {
  token: string;
  host?: string;
  git_protocol?: string;
  insecure_storage?: boolean;
}): Promise<GHStatus> {
  return request("/v1/gh/login", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function ghSwitch(body: { user: string; host?: string }): Promise<GHStatus> {
  return request("/v1/gh/switch", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function ghLogout(body: { user?: string; host?: string }): Promise<GHStatus> {
  return request("/v1/gh/logout", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function ghSetupGit(): Promise<{ ok: boolean; status?: GHStatus }> {
  return request("/v1/gh/setup-git", {
    method: "POST",
    body: "{}",
  });
}

export function cloneWorkspace(body: {
  url: string;
  name?: string;
  branch?: string;
  depth?: number;
  fork?: boolean;
}): Promise<{
  ok: boolean;
  cwd: string;
  abs: string;
  url: string;
  name: string;
  forked?: boolean;
  command?: string;
  output?: string;
}> {
  return request("/v1/workspaces/clone", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function getStatus(): Promise<StatusSnapshot> {
  return request("/v1/status");
}

export function healthz(): Promise<{ ok: boolean }> {
  return fetch("/healthz").then((r) => r.json());
}

export type MapStatus = {
  exists?: boolean;
  stale?: boolean;
  reason?: string;
  generated_at?: string;
  git_head?: string;
};

export function generateMap(cwd: string): Promise<{
  ok: boolean;
  cwd: string;
  map_path?: string;
  status?: MapStatus;
}> {
  return request("/v1/maps", {
    method: "POST",
    body: JSON.stringify({ cwd }),
  });
}

export function getMap(
  cwd: string,
): Promise<{ cwd: string; markdown?: string; status?: MapStatus; error?: string }> {
  return request(`/v1/maps?cwd=${encodeURIComponent(cwd)}`);
}

export function getMapStatus(
  cwd: string,
): Promise<{ cwd: string; status: MapStatus }> {
  return request(`/v1/maps/status?cwd=${encodeURIComponent(cwd)}`);
}

export type MemoryHit = {
  id?: string;
  path?: string;
  title?: string;
  source?: string;
  snippet?: string;
  rank?: number;
  mode?: string;
};

export function memoryIndex(body: {
  cwd: string;
  clear?: boolean;
  include_code?: boolean;
  generate_map?: boolean;
}): Promise<{ ok: boolean; indexed?: number; docs_total?: number }> {
  return request("/v1/memory/index", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function memorySearch(body: {
  cwd: string;
  query: string;
  limit?: number;
  mode?: string;
}): Promise<{ hits: MemoryHit[]; mode?: string; vector?: boolean }> {
  return request("/v1/memory/search", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function memoryStats(
  cwd: string,
): Promise<{
  docs?: number;
  docs_embedded?: number;
  engine?: string;
  vector_enabled?: boolean;
  embed_model?: string;
}> {
  return request(`/v1/memory/stats?cwd=${encodeURIComponent(cwd)}`);
}

export type ContextStatus = {
  cwd?: string;
  ready?: boolean;
  map?: MapStatus;
  memory_docs?: number;
  memory_engine?: string;
  has_context?: boolean;
  has_instructions?: boolean;
  context_path?: string;
  map_path?: string;
  hints?: string[];
  protocol?: string;
};

export function contextStatus(cwd: string): Promise<ContextStatus> {
  return request(`/v1/context/status?cwd=${encodeURIComponent(cwd)}`);
}

export function contextEnsure(body: {
  cwd: string;
  force_map?: boolean;
  force_index?: boolean;
  include_code?: boolean;
  query?: string;
}): Promise<{
  status?: ContextStatus;
  map_generated?: boolean;
  memory_indexed?: number;
  context_wrote?: boolean;
  pack_chars?: number;
  seed?: string;
}> {
  return request("/v1/context/ensure", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function contextPack(body: {
  cwd: string;
  query?: string;
  budget?: number;
  write_file?: boolean;
}): Promise<{
  cwd?: string;
  markdown?: string;
  chars?: number;
  path?: string;
  wrote_file?: boolean;
  memory_hits?: number;
}> {
  return request("/v1/context/pack", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function contextNote(body: {
  cwd: string;
  title?: string;
  text: string;
}): Promise<{ ok?: boolean; note?: { path?: string; title?: string } }> {
  return request("/v1/context/note", {
    method: "POST",
    body: JSON.stringify(body),
  });
}
