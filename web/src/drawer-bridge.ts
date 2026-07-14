/**
 * Bridge between vanilla agents UI and the React+Vaul drawer host.
 */

export type DrawerVariant = "sheet" | "tall" | "content";

export type DrawerSnapshot = {
  open: boolean;
  title: string;
  html: string;
  variant: DrawerVariant;
  revision: number;
};

export type OpenDrawerOptions = {
  title: string;
  html: string;
  variant?: DrawerVariant;
  /** Called when the drawer fully closes (user dismiss or programmatic). */
  onClose?: (reason: "user" | "programmatic") => void;
};

let snapshot: DrawerSnapshot = {
  open: false,
  title: "",
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

export function openMobileDrawer(opts: OpenDrawerOptions): void {
  onCloseCb = opts.onClose ?? null;
  snapshot = {
    open: true,
    title: opts.title,
    html: opts.html,
    variant: opts.variant || "sheet",
    revision: snapshot.revision + 1,
  };
  emit();
}

export function closeMobileDrawer(reason: "user" | "programmatic" = "programmatic"): void {
  if (!snapshot.open) return;
  const cb = onCloseCb;
  onCloseCb = null;
  snapshot = {
    ...snapshot,
    open: false,
    // keep last title/html until next open so close animation isn't empty
    revision: snapshot.revision + 1,
  };
  emit();
  try {
    cb?.(reason);
  } catch {
    /* ignore */
  }
}

export function isMobileDrawerOpen(): boolean {
  return snapshot.open;
}
