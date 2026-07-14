/**
 * React island: Vaul drawer host for all app popups (desktop + mobile).
 * Vanilla main.ts drives open/close via drawer-bridge.
 *
 * UX: mobile = bottom sheet + drag handle; desktop = centered dialog card.
 * Chrome: title (+ optional description) + icon close — no redundant "Close" text.
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
  isMobileViewport,
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

function useIsMobileDrawer(): boolean {
  const [mobile, setMobile] = useState(() => {
    try {
      return isMobileViewport();
    } catch {
      return true;
    }
  });
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 840px)");
    const apply = () => setMobile(mq.matches);
    apply();
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, []);
  return mobile;
}

function VaulApp() {
  const [snap, setSnap] = useState<DrawerSnapshot>(() => getDrawerSnapshot());
  // Retain body through Vaul/Radix close animation (Content unmounts after exit).
  const [heldHtml, setHeldHtml] = useState(() => {
    const s = getDrawerSnapshot();
    return s.open ? s.html : "";
  });
  const [heldMeta, setHeldMeta] = useState(() => {
    const s = getDrawerSnapshot();
    return {
      title: s.open ? s.title : "",
      description: s.open ? s.description : "",
      variant: s.open ? s.variant : ("sheet" as DrawerSnapshot["variant"]),
    };
  });
  const bodyRef = useRef<HTMLDivElement | null>(null);
  const isMobile = useIsMobileDrawer();
  const liveHtml = snap.open && snap.html ? snap.html : heldHtml;
  const title = snap.open ? snap.title : heldMeta.title;
  const description = snap.open ? snap.description : heldMeta.description;
  const variant = (snap.open ? snap.variant : heldMeta.variant) || "sheet";
  const hasDesc = Boolean(description?.trim());

  useEffect(() => subscribeDrawer(setSnap), []);

  useEffect(() => {
    if (snap.open && snap.html) {
      setHeldHtml(snap.html);
      setHeldMeta({
        title: snap.title,
        description: snap.description,
        variant: snap.variant,
      });
      return;
    }
    if (!snap.open) {
      const t = window.setTimeout(() => {
        if (!getDrawerSnapshot().open) {
          setHeldHtml("");
          setHeldMeta({ title: "", description: "", variant: "sheet" });
        }
      }, 400);
      return () => window.clearTimeout(t);
    }
  }, [snap.open, snap.html, snap.revision, snap.title, snap.description, snap.variant]);

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
      shouldScaleBackground={isMobile}
      setBackgroundColorOnScale={isMobile}
      handleOnly={isMobile}
      repositionInputs
      autoFocus={false}
    >
      <Drawer.Portal>
        <Drawer.Overlay className="vaul-overlay" />
        <Drawer.Content
          className={`vaul-content vaul-content--${variant}${isMobile ? " vaul-content--mobile" : " vaul-content--desktop"}`}
          // Suppress describedby when description is sr-only only.
          {...(hasDesc ? {} : { "aria-describedby": undefined })}
          onOpenAutoFocus={(e) => e.preventDefault()}
          onCloseAutoFocus={(e) => e.preventDefault()}
        >
          <div className="vaul-chrome">
            {/* Drag handle — mobile sheet only (desktop is a centered dialog). */}
            {isMobile ? <Drawer.Handle className="vaul-handle" /> : null}
            <div className={`vaul-header${hasDesc ? " vaul-header--with-desc" : ""}`}>
              <div className="vaul-header-text">
                <Drawer.Title className="vaul-title">
                  {title || "Dialog"}
                </Drawer.Title>
                {hasDesc ? (
                  <Drawer.Description className="vaul-desc">
                    {description}
                  </Drawer.Description>
                ) : (
                  <Drawer.Description className="vaul-desc sr-only">
                    {title || "Dialog"} panel
                  </Drawer.Description>
                )}
              </div>
              <button
                type="button"
                className="vaul-close ghost btn-icon sm"
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
