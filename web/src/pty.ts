/**
 * WebSocket PTY client matching agentsd / internal/clientpty protocol.
 * Binary frames = terminal I/O; JSON text = {type:resize|ping|pong|ready|error}.
 */

export type PtyHandlers = {
  onData: (data: Uint8Array) => void;
  onReady?: (cols: number, rows: number) => void;
  onError?: (msg: string) => void;
  onClose?: (reason: string) => void;
  onReconnect?: (attempt: number) => void;
};

const MAX_RECONNECTS = 8;
const RECONNECT_BASE_MS = 500;

export class PtyClient {
  private ws: WebSocket | null = null;
  private closed = false;
  private attempt = 0;
  private intentionalClose = false;

  constructor(
    private sessionId: string,
    private token: string,
    private handlers: PtyHandlers,
  ) {}

  connect(cols: number, rows: number): void {
    this.closed = false;
    this.intentionalClose = false;
    this.open(cols, rows);
  }

  private open(cols: number, rows: number): void {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const q = new URLSearchParams({
      cols: String(Math.max(20, cols | 0)),
      rows: String(Math.max(5, rows | 0)),
    });
    if (this.token) {
      q.set("token", this.token);
    }
    const url = `${proto}//${location.host}/v1/sessions/${encodeURIComponent(this.sessionId)}/pty?${q}`;
    const ws = new WebSocket(url);
    ws.binaryType = "arraybuffer";
    this.ws = ws;

    ws.onopen = () => {
      this.attempt = 0;
    };

    ws.onmessage = (ev) => {
      if (typeof ev.data === "string") {
        try {
          const msg = JSON.parse(ev.data) as {
            type?: string;
            cols?: number;
            rows?: number;
            message?: string;
          };
          switch (msg.type) {
            case "ready":
              this.handlers.onReady?.(msg.cols ?? cols, msg.rows ?? rows);
              break;
            case "ping":
              this.sendCtrl({ type: "pong" });
              break;
            case "pong":
              break;
            case "error":
              this.handlers.onError?.(msg.message ?? "remote error");
              break;
          }
        } catch {
          /* ignore non-json text */
        }
        return;
      }
      const buf =
        ev.data instanceof ArrayBuffer
          ? new Uint8Array(ev.data)
          : new Uint8Array(0);
      if (buf.length) this.handlers.onData(buf);
    };

    ws.onerror = () => {
      /* onclose will fire */
    };

    ws.onclose = () => {
      this.ws = null;
      if (this.intentionalClose || this.closed) {
        this.handlers.onClose?.("closed");
        return;
      }
      this.attempt += 1;
      if (this.attempt > MAX_RECONNECTS) {
        this.handlers.onClose?.(
          `disconnected after ${MAX_RECONNECTS} reconnects`,
        );
        return;
      }
      const delay = RECONNECT_BASE_MS * Math.min(16, 1 << (this.attempt - 1));
      this.handlers.onReconnect?.(this.attempt);
      window.setTimeout(() => {
        if (!this.intentionalClose && !this.closed) {
          this.open(cols, rows);
        }
      }, delay);
    };
  }

  write(data: string | Uint8Array): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
    if (typeof data === "string") {
      this.ws.send(data);
    } else {
      this.ws.send(data);
    }
  }

  resize(cols: number, rows: number): void {
    this.sendCtrl({ type: "resize", cols, rows });
  }

  private sendCtrl(msg: Record<string, unknown>): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
    this.ws.send(JSON.stringify(msg));
  }

  /** Detach only — server keeps tmux session running. */
  detach(): void {
    this.intentionalClose = true;
    this.closed = true;
    if (this.ws) {
      try {
        this.ws.close();
      } catch {
        /* ignore */
      }
      this.ws = null;
    }
  }
}
