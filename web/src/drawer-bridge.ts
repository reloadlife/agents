/**
 * Bridge between vanilla agents UI and the React+Vaul host.
 * All popups (panels + content) go through Vaul on every viewport.
 */

export type DrawerVariant = "sheet" | "tall" | "content" | "dialog";

export type DrawerSnapshot = {
  open: boolean;
  title: string;
  description: string;
  html: string;
  variant: DrawerVariant;
  revision: number;
};

export type OpenDrawerOptions = {
  title: string;
  /** Optional visible subtitle under the title (shadcn DialogDescription). */
  description?: string;
  html: string;
  variant?: DrawerVariant;
  /** Called when the drawer fully closes (user dismiss or programmatic). */
  onClose?: (reason: "user" | "programmatic") => void;
};

let snapshot: DrawerSnapshot = {
  open: false,
  title: "",
  description: "",
  html: "",
  variant: "sheet",
  revision: 0,
};

let onCloseCb: OpenDrawerOptions["onClose"] | null = null;
const listeners = new Set<(s: DrawerSnapshot) => void>();

function emit(): void {
  const snap = { ...snapshot };
  for (const l of listeners) l(snap);
}

export function getDrawerSnapshot(): DrawerSnapshot {
  return { ...snapshot };
}

export function subscribeDrawer(fn: (s: DrawerSnapshot) => void): () => void {
  listeners.add(fn);
  fn({ ...snapshot });
  return () => {
    listeners.delete(fn);
  };
}

export function isMobileViewport(): boolean {
  try {
    return window.matchMedia("(max-width: 840px)").matches;
  } catch {
    return false;
  }
}

/** @deprecated use openAppDrawer — kept as alias */
export function openMobileDrawer(opts: OpenDrawerOptions): void {
  openAppDrawer(opts);
}

export function openAppDrawer(opts: OpenDrawerOptions): void {
  onCloseCb = opts.onClose ?? null;
  snapshot = {
    open: true,
    title: opts.title,
    description: opts.description?.trim() || "",
    html: opts.html,
    variant: opts.variant || "sheet",
    revision: snapshot.revision + 1,
  };
  emit();
}

/** @deprecated use closeAppDrawer */
export function closeMobileDrawer(reason: "user" | "programmatic" = "programmatic"): void {
  closeAppDrawer(reason);
}

export function closeAppDrawer(reason: "user" | "programmatic" = "programmatic"): void {
  if (!snapshot.open) return;
  const cb = onCloseCb;
  onCloseCb = null;
  snapshot = {
    ...snapshot,
    open: false,
    revision: snapshot.revision + 1,
  };
  emit();
  try {
    cb?.(reason);
  } catch {
    /* ignore */
  }
}

/** @deprecated use isAppDrawerOpen */
export function isMobileDrawerOpen(): boolean {
  return isAppDrawerOpen();
}

export function isAppDrawerOpen(): boolean {
  return snapshot.open;
}
