/**
 * React island: Vaul drawer host for all app popups (desktop + mobile).
 * Vanilla main.ts drives open/close via drawer-bridge.
 */
import { useEffect, useRef, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { Drawer } from "vaul";
import "../node_modules/vaul/style.css";

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

function VaulApp() {
  const [snap, setSnap] = useState<DrawerSnapshot>(() => getDrawerSnapshot());
  // Retain body through Vaul/Radix close animation (Content unmounts after exit).
  const [heldHtml, setHeldHtml] = useState(() => {
    const s = getDrawerSnapshot();
    return s.open ? s.html : "";
  });
  const bodyRef = useRef<HTMLDivElement | null>(null);
  const liveHtml = snap.open && snap.html ? snap.html : heldHtml;
  const variant = snap.variant || "sheet";

  useEffect(() => subscribeDrawer(setSnap), []);

  useEffect(() => {
    if (snap.open && snap.html) {
      setHeldHtml(snap.html);
      return;
    }
    if (!snap.open) {
      const t = window.setTimeout(() => {
        if (!getDrawerSnapshot().open) setHeldHtml("");
      }, 400);
      return () => window.clearTimeout(t);
    }
  }, [snap.open, snap.html, snap.revision]);

  useEffect(() => {
    if (!snap.open || !liveHtml) return;
    const el = bodyRef.current;
    if (!el) return;
    // Prefer first real input; skip ghost Cancel / Close
    const focusable = el.querySelector<HTMLElement>(
      "input:not([type=hidden]):not([type=checkbox]):not([type=radio]), select, textarea, button.primary",
    );
    window.requestAnimationFrame(() => focusable?.focus?.());
  }, [snap.open, liveHtml, snap.revision]);

  // After inject: pin .modal-actions into sticky footer if present
  useEffect(() => {
    if (!snap.open || !liveHtml) return;
    const el = bodyRef.current;
    if (!el) return;
    const form = el.querySelector<HTMLElement>("form.sheet-form, .sheet-form");
    const actions = el.querySelector<HTMLElement>(".modal-actions");
    if (form && actions && !form.classList.contains("sheet-form--footed")) {
      form.classList.add("sheet-form--footed");
    }
  }, [snap.open, liveHtml, snap.revision]);

  return (
    <Drawer.Root
      open={snap.open}
      onOpenChange={(open) => {
        if (!open) closeAppDrawer("user");
      }}
      shouldScaleBackground
      setBackgroundColorOnScale
      handleOnly
      repositionInputs
      autoFocus={false}
    >
      <Drawer.Portal>
        <Drawer.Overlay className="vaul-overlay" />
        <Drawer.Content
          className={`vaul-content vaul-content--${variant}`}
          aria-describedby={undefined}
          onOpenAutoFocus={(e) => e.preventDefault()}
          onCloseAutoFocus={(e) => e.preventDefault()}
        >
          <div className="vaul-chrome">
            <Drawer.Handle className="vaul-handle" />
            <div className="vaul-header">
              <div className="vaul-header-text">
                <Drawer.Title className="vaul-title">
                  {snap.title || "Dialog"}
                </Drawer.Title>
                <Drawer.Description className="vaul-desc sr-only">
                  {snap.title || "Dialog"} panel
                </Drawer.Description>
              </div>
              <button
                type="button"
                className="vaul-close"
                onClick={() => closeAppDrawer("user")}
                aria-label="Close"
                title="Close (Esc)"
              >
                <CloseIcon />
              </button>
            </div>
            <div
              ref={bodyRef}
              className="vaul-body"
              data-vaul-no-drag=""
              dangerouslySetInnerHTML={{ __html: liveHtml }}
            />
          </div>
        </Drawer.Content>
      </Drawer.Portal>
    </Drawer.Root>
  );
}

let root: Root | null = null;

/** Mount once (idempotent). */
export function mountVaulHost(): void {
  let host = document.getElementById("vaul-host");
  if (!host) {
    host = document.createElement("div");
    host.id = "vaul-host";
    document.body.appendChild(host);
  }
  if (!root) {
    root = createRoot(host);
    root.render(<VaulApp />);
  }
}
