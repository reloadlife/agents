/**
 * App modal host — content-sized centered dialog (desktop) / bottom sheet (mobile).
 * Keeps the drawer-bridge API; does NOT use Vaul's bottom-drawer height model
 * (that caused empty whitespace under short forms).
 */
import { useEffect, useRef, useState } from "react";
import { createRoot, type Root } from "react-dom/client";

import {
  getDrawerSnapshot,
  subscribeDrawer,
  type DrawerSnapshot,
  closeAppDrawer,
} from "./drawer-bridge";

function CloseIcon() {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </svg>
  );
}

function AppModal() {
  const [snap, setSnap] = useState<DrawerSnapshot>(() => getDrawerSnapshot());
  const [heldHtml, setHeldHtml] = useState(() => {
    const s = getDrawerSnapshot();
    return s.open ? s.html : "";
  });
  const [heldTitle, setHeldTitle] = useState(() => {
    const s = getDrawerSnapshot();
    return s.open ? s.title : "";
  });
  const bodyRef = useRef<HTMLDivElement | null>(null);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const liveHtml = snap.open && snap.html ? snap.html : heldHtml;
  const title = snap.open ? snap.title : heldTitle;
  const variant = snap.variant || "dialog";
  const open = snap.open;

  useEffect(() => subscribeDrawer(setSnap), []);

  useEffect(() => {
    if (snap.open && snap.html) {
      setHeldHtml(snap.html);
      setHeldTitle(snap.title);
      return;
    }
    if (!snap.open) {
      const t = window.setTimeout(() => {
        if (!getDrawerSnapshot().open) {
          setHeldHtml("");
          setHeldTitle("");
        }
      }, 200);
      return () => window.clearTimeout(t);
    }
  }, [snap.open, snap.html, snap.revision, snap.title]);

  useEffect(() => {
    if (!open || !liveHtml) return;
    const el = bodyRef.current;
    if (!el) return;
    const focusable = el.querySelector<HTMLElement>(
      "input:not([type=hidden]):not([type=checkbox]):not([type=radio]), select, textarea, button.primary",
    );
    window.requestAnimationFrame(() => focusable?.focus?.());
  }, [open, liveHtml, snap.revision]);

  // Body scroll lock while open
  useEffect(() => {
    if (!open) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [open]);

  // Esc
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        closeAppDrawer("user");
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  if (!open && !liveHtml) return null;

  return (
    <div
      className={`app-modal-root${open ? " app-modal-root--open" : ""}`}
      data-state={open ? "open" : "closed"}
      aria-hidden={!open}
    >
      <div
        className="app-modal-overlay"
        onClick={() => closeAppDrawer("user")}
      />
      <div
        ref={panelRef}
        className={`app-modal app-modal--${variant}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby="app-modal-title"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="app-modal-header">
          <h2 id="app-modal-title" className="app-modal-title">
            {title || "Dialog"}
          </h2>
          <button
            type="button"
            className="app-modal-close"
            onClick={() => closeAppDrawer("user")}
            aria-label="Close"
          >
            <CloseIcon />
          </button>
        </div>
        <div
          ref={bodyRef}
          className="app-modal-body"
          dangerouslySetInnerHTML={{ __html: liveHtml }}
        />
      </div>
    </div>
  );
}

let root: Root | null = null;

/** Mount once (idempotent). Name kept for callers. */
export function mountVaulHost(): void {
  let host = document.getElementById("vaul-host");
  if (!host) {
    host = document.createElement("div");
    host.id = "vaul-host";
    document.body.appendChild(host);
  }
  if (!root) {
    root = createRoot(host);
    root.render(<AppModal />);
  }
}
