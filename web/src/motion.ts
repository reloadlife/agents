/**
 * Motion.dev (motion) helpers for the agents SPA.
 * Vanilla DOM animations — spring / stagger / soft exits.
 */
import { animate, stagger } from "motion";

export function prefersReducedMotion(): boolean {
  try {
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  } catch {
    return false;
  }
}

const springSnappy = { type: "spring" as const, stiffness: 480, damping: 36, mass: 0.75 };
const springSoft = { type: "spring" as const, stiffness: 320, damping: 34, mass: 0.9 };
const springToast = { type: "spring" as const, stiffness: 420, damping: 30, mass: 0.7 };

function done(ctrl: { finished?: Promise<unknown> } | void): Promise<void> {
  if (!ctrl || !("finished" in ctrl) || !ctrl.finished) return Promise.resolve();
  return ctrl.finished.then(() => undefined, () => undefined);
}

/** Fade + lift overlay backdrop + spring the modal card. */
export async function animateModalIn(overlay: HTMLElement): Promise<void> {
  if (prefersReducedMotion()) {
    overlay.style.opacity = "1";
    return;
  }
  const modal = overlay.querySelector<HTMLElement>("[data-modal], .modal");
  overlay.style.opacity = "0";
  if (modal) {
    modal.style.opacity = "0";
    modal.style.transform = "translateY(14px) scale(0.97)";
  }
  void animate(overlay, { opacity: [0, 1] }, { duration: 0.2, ease: "easeOut" });
  if (modal) {
    await done(
      animate(
        modal,
        { opacity: [0, 1], y: [14, 0], scale: [0.97, 1] },
        springSnappy,
      ),
    );
  }
}

/** Soft fade + scale-down before DOM removal. */
export async function animateModalOut(overlay: HTMLElement): Promise<void> {
  if (prefersReducedMotion()) return;
  const modal = overlay.querySelector<HTMLElement>("[data-modal], .modal");
  if (modal) {
    void animate(
      modal,
      { opacity: [1, 0], y: [0, 8], scale: [1, 0.98] },
      { duration: 0.14, ease: "easeIn" },
    );
  }
  await done(animate(overlay, { opacity: [1, 0] }, { duration: 0.16, ease: "easeIn" }));
}

/** Full-page settings slide + fade. */
export async function animateSettingsIn(root: HTMLElement): Promise<void> {
  if (prefersReducedMotion()) return;
  const nav = root.querySelector<HTMLElement>(".settings-nav");
  const main = root.querySelector<HTMLElement>(".settings-main");
  root.style.opacity = "0";
  if (nav) {
    nav.style.opacity = "0";
    nav.style.transform = "translateX(-12px)";
  }
  if (main) {
    main.style.opacity = "0";
    main.style.transform = "translateY(10px) scale(0.985)";
  }
  void animate(root, { opacity: [0, 1] }, { duration: 0.18, ease: "easeOut" });
  if (nav) {
    void animate(nav, { opacity: [0, 1], x: [-12, 0] }, { ...springSoft, delay: 0.02 });
  }
  if (main) {
    await done(
      animate(
        main,
        { opacity: [0, 1], y: [10, 0], scale: [0.985, 1] },
        { ...springSoft, delay: 0.04 },
      ),
    );
  }
}

export async function animateSettingsOut(root: HTMLElement): Promise<void> {
  if (prefersReducedMotion()) return;
  const main = root.querySelector<HTMLElement>(".settings-main");
  if (main) {
    void animate(
      main,
      { opacity: [1, 0], y: [0, 6], scale: [1, 0.99] },
      { duration: 0.12, ease: "easeIn" },
    );
  }
  await done(animate(root, { opacity: [1, 0] }, { duration: 0.14, ease: "easeIn" }));
}

/** Toast slide up from corner. */
export function animateToastIn(el: HTMLElement): void {
  if (prefersReducedMotion()) return;
  el.style.opacity = "0";
  el.style.transform = "translateY(12px) scale(0.96)";
  void animate(
    el,
    { opacity: [0, 1], y: [12, 0], scale: [0.96, 1] },
    springToast,
  );
}

export async function animateToastOut(el: HTMLElement): Promise<void> {
  if (prefersReducedMotion()) return;
  await done(
    animate(
      el,
      { opacity: [1, 0], y: [0, 8], scale: [1, 0.97] },
      { duration: 0.15, ease: "easeIn" },
    ),
  );
}

/**
 * Shell first paint — sidebar + inset card cascade.
 * Only call once on mount (not every chrome paint).
 */
export function animateShellIn(shell: HTMLElement): void {
  if (prefersReducedMotion()) return;
  const sidebar = shell.querySelector<HTMLElement>(".sidebar");
  const main = shell.querySelector<HTMLElement>(".main");
  if (sidebar) {
    sidebar.style.opacity = "0";
    sidebar.style.transform = "translateX(-16px)";
    void animate(
      sidebar,
      { opacity: [0, 1], x: [-16, 0] },
      { ...springSoft, delay: 0.02 },
    );
  }
  if (main) {
    main.style.opacity = "0";
    main.style.transform = "translateY(12px) scale(0.985)";
    void animate(
      main,
      { opacity: [0, 1], y: [12, 0], scale: [0.985, 1] },
      { ...springSoft, delay: 0.06 },
    );
  }
  const menuBtns = shell.querySelectorAll<HTMLElement>(".sidebar-menu-btn, .sidebar-new");
  if (menuBtns.length) {
    for (const b of menuBtns) {
      b.style.opacity = "0";
      b.style.transform = "translateY(6px)";
    }
    void animate(
      menuBtns,
      { opacity: [0, 1], y: [6, 0] },
      { ...springSnappy, delay: stagger(0.035, { startDelay: 0.08 }) },
    );
  }
}

/** Stagger session list items (call after list HTML rewrite). */
export function animateSessionList(list: HTMLElement): void {
  if (prefersReducedMotion()) return;
  const items = list.querySelectorAll<HTMLElement>(".session-item");
  if (!items.length) return;
  // Cap so large lists don't take forever
  const max = 18;
  const targets = Array.from(items).slice(0, max);
  for (const el of targets) {
    el.style.opacity = "0";
    el.style.transform = "translateX(-8px)";
  }
  void animate(
    targets,
    { opacity: [0, 1], x: [-8, 0] },
    { ...springSnappy, delay: stagger(0.028, { startDelay: 0.02 }) },
  );
}

/** Subtle press feedback on interactive chrome (optional). */
export function bindPressMotion(root: ParentNode = document): void {
  if (prefersReducedMotion()) return;
  root.addEventListener(
    "pointerdown",
    (ev) => {
      const t = (ev.target as HTMLElement | null)?.closest?.(
        "button.primary, button.sidebar-new, .sidebar-menu-btn, .session-item",
      ) as HTMLElement | null;
      if (!t || t.hasAttribute("disabled")) return;
      void animate(t, { scale: 0.97 }, { duration: 0.08 });
    },
    true,
  );
  root.addEventListener(
    "pointerup",
    (ev) => {
      const t = (ev.target as HTMLElement | null)?.closest?.(
        "button.primary, button.sidebar-new, .sidebar-menu-btn, .session-item",
      ) as HTMLElement | null;
      if (!t) return;
      void animate(t, { scale: 1 }, springSnappy);
    },
    true,
  );
  root.addEventListener(
    "pointercancel",
    (ev) => {
      const t = (ev.target as HTMLElement | null)?.closest?.(
        "button.primary, button.sidebar-new, .sidebar-menu-btn, .session-item",
      ) as HTMLElement | null;
      if (!t) return;
      void animate(t, { scale: 1 }, { duration: 0.1 });
    },
    true,
  );
}

/** Login card entrance. */
export function animateLoginIn(root: HTMLElement): void {
  if (prefersReducedMotion()) return;
  const card = root.querySelector<HTMLElement>(".login-card");
  if (!card) return;
  card.style.opacity = "0";
  card.style.transform = "translateY(16px) scale(0.97)";
  void animate(
    card,
    { opacity: [0, 1], y: [16, 0], scale: [0.97, 1] },
    springSoft,
  );
}
