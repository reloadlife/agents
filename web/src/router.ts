/**
 * Client-side routes for the agents web UI.
 *
 *   /                              → new session (modal open)
 *   /new                           → same
 *   /project/new · /projects/new   → new project (clone) modal
 *   /desk                          → empty desk (no modal)
 *   /tools                         → global tools
 *   /help                          → shortcuts help
 *   /profile[/:tab]                → profile / settings
 *   /settings[/:tab]               → alias for profile
 *   /project/:projectId/session/:sessionId
 *   /project/:projectId/session/:sessionId/tools
 */

export type ProfileTab = "accounts" | "github" | "ssh" | "workspace" | "about";

export type Route =
  | { name: "new" }
  | { name: "new-project" }
  | { name: "desk" }
  | { name: "tools" }
  | { name: "help" }
  | { name: "profile"; tab: ProfileTab }
  | {
      name: "session";
      projectId: string;
      sessionId: string;
      tools?: boolean;
    };

const PROFILE_TABS: ProfileTab[] = [
  "accounts",
  "github",
  "ssh",
  "workspace",
  "about",
];

/** Encode workspace cwd into a single URL path segment. */
export function projectIdFromCwd(cwd: string): string {
  const c = (cwd || ".").replace(/\\/g, "/").replace(/\/+$/, "") || ".";
  if (c === "." || c === "") return "_";
  // Join multi-segment cwds with ~ so / in path doesn't create extra segments.
  return c
    .split("/")
    .filter(Boolean)
    .map((s) => encodeURIComponent(s))
    .join("~");
}

/** Decode projectId back to workspace-relative cwd. */
export function cwdFromProjectId(projectId: string): string {
  const id = (projectId || "").trim();
  if (!id || id === "_") return ".";
  try {
    return id
      .split("~")
      .map((s) => decodeURIComponent(s))
      .join("/");
  } catch {
    return id.replace(/~/g, "/");
  }
}

export function isProfileTab(s: string | null | undefined): s is ProfileTab {
  return !!s && (PROFILE_TABS as string[]).includes(s);
}

export function parsePath(pathname: string): Route {
  let path = pathname || "/";
  // strip base if any; we serve at /
  if (!path.startsWith("/")) path = `/${path}`;
  // collapse trailing slash (except root)
  if (path.length > 1 && path.endsWith("/")) path = path.slice(0, -1);

  const parts = path.split("/").filter(Boolean);

  if (parts.length === 0) return { name: "new" };
  if (parts[0] === "new") {
    // /new/project → new project; bare /new → new session
    if (parts[1] === "project" || parts[1] === "projects") {
      return { name: "new-project" };
    }
    return { name: "new" };
  }
  if (parts[0] === "desk" || parts[0] === "sessions") return { name: "desk" };
  if (parts[0] === "tools") return { name: "tools" };
  if (parts[0] === "help" || parts[0] === "shortcuts") return { name: "help" };

  if (parts[0] === "profile" || parts[0] === "settings") {
    const tab = isProfileTab(parts[1]) ? parts[1] : "accounts";
    return { name: "profile", tab };
  }

  // /project/new | /projects/new — before session pattern
  if (
    (parts[0] === "project" || parts[0] === "projects") &&
    parts[1] === "new" &&
    parts.length === 2
  ) {
    return { name: "new-project" };
  }

  // /project/:projectId/session/:sessionId[/tools]
  if (parts[0] === "project" && parts[2] === "session" && parts[1] && parts[3]) {
    const projectId = parts[1];
    const sessionId = parts[3];
    const tools = parts[4] === "tools";
    return { name: "session", projectId, sessionId, tools };
  }

  // Unknown → desk (safe)
  return { name: "desk" };
}

export function serializeRoute(route: Route): string {
  switch (route.name) {
    case "new":
      return "/";
    case "new-project":
      return "/project/new";
    case "desk":
      return "/desk";
    case "tools":
      return "/tools";
    case "help":
      return "/help";
    case "profile":
      return route.tab === "accounts"
        ? "/profile"
        : `/profile/${route.tab}`;
    case "session": {
      const base = `/project/${route.projectId}/session/${encodeURIComponent(route.sessionId)}`;
      return route.tools ? `${base}/tools` : base;
    }
    default:
      return "/desk";
  }
}

export function sessionPath(cwd: string, sessionId: string, tools = false): string {
  return serializeRoute({
    name: "session",
    projectId: projectIdFromCwd(cwd),
    sessionId,
    tools,
  });
}

export function routesEqual(a: Route, b: Route): boolean {
  if (a.name !== b.name) return false;
  switch (a.name) {
    case "profile":
      return b.name === "profile" && a.tab === b.tab;
    case "session":
      return (
        b.name === "session" &&
        a.projectId === b.projectId &&
        a.sessionId === b.sessionId &&
        !!a.tools === !!b.tools
      );
    default:
      return true;
  }
}

/** Current location path as Route. */
export function currentRoute(): Route {
  return parsePath(window.location.pathname || "/");
}
