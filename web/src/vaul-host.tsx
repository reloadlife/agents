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

function VaulApp() {
  const [snap, setSnap] = useState<DrawerSnapshot>(() => getDrawerSnapshot());
  // Retain body through Vaul/Radix close animation (Content unmounts after exit).
  const [heldHtml, setHeldHtml] = useState(() => {
    const s = getDrawerSnapshot();
    return s.open ? s.html : "";
  });
  const bodyRef = useRef<HTMLDivElement | null>(null);
  const liveHtml = snap.open && snap.html ? snap.html : heldHtml;

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
    const focusable = el.querySelector<HTMLElement>(
      "input:not([type=hidden]), select, textarea, button.primary, button",
    );
    window.requestAnimationFrame(() => focusable?.focus?.());
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
          className={`vaul-content vaul-content--${snap.variant || "sheet"}`}
          aria-describedby={undefined}
          // Keep DOM mounted during open so body HTML is present on first paint.
          onOpenAutoFocus={(e) => e.preventDefault()}
        >
          <div className="vaul-chrome">
            <Drawer.Handle className="vaul-handle" />
            <div className="vaul-header">
              <Drawer.Title className="vaul-title">
                {snap.title || "Dialog"}
              </Drawer.Title>
              <button
                type="button"
                className="ghost btn-sm vaul-close"
                onClick={() => closeAppDrawer("user")}
                aria-label="Close"
              >
                Close
              </button>
            </div>
            <div
              ref={bodyRef}
              className="vaul-body"
              data-vaul-no-drag=""
              // Presence remounts Content on open — inject in render, not a post-mount effect race.
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
