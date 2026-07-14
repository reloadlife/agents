/**
 * React island: Vaul bottom-sheet host for mobile drawers/panels.
 * Vanilla main.ts drives open/close via the drawer-bridge.
 */
import { useEffect, useRef, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { Drawer } from "vaul";
// vaul package.json exports block subpath "style.css" — load via relative path
import "../node_modules/vaul/style.css";

import {
  getDrawerSnapshot,
  subscribeDrawer,
  type DrawerSnapshot,
  closeMobileDrawer,
} from "./drawer-bridge";

function VaulApp() {
  const [snap, setSnap] = useState<DrawerSnapshot>(() => getDrawerSnapshot());
  const bodyRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => subscribeDrawer(setSnap), []);

  useEffect(() => {
    const el = bodyRef.current;
    if (!el) return;
    if (snap.open && snap.html) {
      el.innerHTML = snap.html;
      // Focus first focusable control for a11y / tools forms
      const focusable = el.querySelector<HTMLElement>(
        "input, select, textarea, button.primary, button",
      );
      window.requestAnimationFrame(() => focusable?.focus?.());
    } else {
      el.innerHTML = "";
    }
  }, [snap.open, snap.html, snap.revision]);

  return (
    <Drawer.Root
      open={snap.open}
      onOpenChange={(open) => {
        if (!open) closeMobileDrawer("user");
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
        >
          <div className="vaul-chrome">
            <Drawer.Handle className="vaul-handle" />
            <div className="vaul-header">
              <Drawer.Title className="vaul-title">{snap.title || "Drawer"}</Drawer.Title>
              <button
                type="button"
                className="ghost btn-sm vaul-close"
                onClick={() => closeMobileDrawer("user")}
                aria-label="Close"
              >
                Close
              </button>
            </div>
            <div
              ref={bodyRef}
              className="vaul-body"
              data-vaul-no-drag=""
            />
          </div>
        </Drawer.Content>
      </Drawer.Portal>
    </Drawer.Root>
  );
}

let root: Root | null = null;

/** Mount once on boot (idempotent). */
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
