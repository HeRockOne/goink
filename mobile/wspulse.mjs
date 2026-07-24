// src/codec.ts
var JSONCodec = {
  binaryType: "text",
  encode(msg) {
    return JSON.stringify(msg);
  },
  decode(data) {
    const str = typeof data === "string" ? data : new TextDecoder().decode(data);
    return JSON.parse(str);
  }
};

// src/clock.ts
var defaultClock = {
  setTimeout,
  clearTimeout
};

// src/options.ts
var DEFAULT_WRITE_WAIT = 1e4;
var DEFAULT_MAX_MESSAGE_SIZE = 1 << 20;
var DEFAULT_SEND_BUFFER_SIZE = 256;
var MAX_SEND_BUFFER_SIZE = 4096;
var MAX_WRITE_WAIT = 3e4;
var MAX_MSG_SIZE_BYTES = 64 << 20;
var MAX_BASE_DELAY = 6e4;
var MAX_DELAY_LIMIT = 3e5;
var MAX_RETRIES_LIMIT = 32;
var noop = () => {
};
function validateOptions(opts) {
  if (opts.maxMessageSize !== void 0) {
    if (!Number.isFinite(opts.maxMessageSize)) {
      throw new Error("wspulse: maxMessageSize must be a finite number");
    }
    if (opts.maxMessageSize < 0) {
      throw new Error("wspulse: maxMessageSize must be non-negative");
    }
    if (opts.maxMessageSize > MAX_MSG_SIZE_BYTES) {
      throw new Error("wspulse: maxMessageSize exceeds maximum (64 MiB)");
    }
  }
  if (opts.writeWait !== void 0) {
    if (!Number.isFinite(opts.writeWait)) {
      throw new Error("wspulse: writeWait must be a finite number");
    }
    if (opts.writeWait <= 0) {
      throw new Error("wspulse: writeWait must be positive");
    }
    if (opts.writeWait > MAX_WRITE_WAIT) {
      throw new Error("wspulse: writeWait exceeds maximum (30s)");
    }
  }
  if (opts.sendBufferSize !== void 0) {
    if (!Number.isFinite(opts.sendBufferSize) || !Number.isInteger(opts.sendBufferSize)) {
      throw new Error("wspulse: sendBufferSize must be a finite integer");
    }
    if (opts.sendBufferSize < 1) {
      throw new Error("wspulse: sendBufferSize must be at least 1");
    }
    if (opts.sendBufferSize > MAX_SEND_BUFFER_SIZE) {
      throw new Error(
        `wspulse: sendBufferSize exceeds maximum (${MAX_SEND_BUFFER_SIZE})`
      );
    }
  }
  if (opts._dialer !== void 0 && typeof opts._dialer !== "function") {
    throw new Error("wspulse: _dialer must be a function");
  }
  if (opts._clock !== void 0 && typeof opts._clock !== "object") {
    throw new Error("wspulse: _clock must be an object");
  }
  if (opts.autoReconnect !== void 0) {
    const rc = opts.autoReconnect;
    if (rc.maxRetries < 0) {
      throw new Error("wspulse: autoReconnect.maxRetries must be non-negative");
    }
    if (rc.baseDelay <= 0) {
      throw new Error("wspulse: autoReconnect.baseDelay must be positive");
    }
    if (rc.baseDelay > MAX_BASE_DELAY) {
      throw new Error("wspulse: autoReconnect.baseDelay exceeds maximum (1m)");
    }
    if (rc.maxDelay < rc.baseDelay) {
      throw new Error(
        "wspulse: autoReconnect.maxDelay must be >= autoReconnect.baseDelay"
      );
    }
    if (rc.maxDelay > MAX_DELAY_LIMIT) {
      throw new Error("wspulse: autoReconnect.maxDelay exceeds maximum (5m)");
    }
    if (rc.maxRetries > 0 && rc.maxRetries > MAX_RETRIES_LIMIT) {
      throw new Error("wspulse: autoReconnect.maxRetries exceeds maximum (32)");
    }
  }
}
function resolveOptions(opts) {
  if (opts) {
    validateOptions(opts);
  }
  return {
    onMessage: opts?.onMessage ?? noop,
    onDisconnect: opts?.onDisconnect ?? noop,
    onTransportRestore: opts?.onTransportRestore ?? noop,
    onTransportDrop: opts?.onTransportDrop ?? noop,
    codec: opts?.codec ?? JSONCodec,
    autoReconnect: opts?.autoReconnect,
    writeWait: opts?.writeWait ?? DEFAULT_WRITE_WAIT,
    maxMessageSize: opts?.maxMessageSize ?? DEFAULT_MAX_MESSAGE_SIZE,
    dialHeaders: opts?.dialHeaders ?? {},
    sendBufferSize: opts?.sendBufferSize ?? DEFAULT_SEND_BUFFER_SIZE,
    _dialer: opts?._dialer,
    _clock: opts?._clock ?? defaultClock
  };
}

// src/ring-buffer.ts
var RingBuffer = class {
  data;
  head = 0;
  size = 0;
  cap;
  constructor(capacity) {
    this.cap = capacity;
    this.data = new Array(capacity);
  }
  /** Number of elements currently in the buffer. */
  get length() {
    return this.size;
  }
  /**
   * Append an item to the back of the buffer.
   *
   * @returns `true` if the item was added, `false` if the buffer is full.
   */
  push(item) {
    if (this.size >= this.cap) return false;
    const index = (this.head + this.size) % this.cap;
    this.data[index] = item;
    this.size++;
    return true;
  }
  /**
   * Return the front item without removing it.
   *
   * @returns The oldest item, or `undefined` if the buffer is empty.
   */
  peek() {
    if (this.size === 0) return void 0;
    return this.data[this.head];
  }
  /**
   * Remove and return the front item.
   *
   * @returns The oldest item, or `undefined` if the buffer is empty.
   */
  shift() {
    if (this.size === 0) return void 0;
    const item = this.data[this.head];
    this.data[this.head] = void 0;
    this.head = (this.head + 1) % this.cap;
    this.size--;
    return item;
  }
  /** Reset the buffer to empty. Does not reallocate the underlying array. */
  clear() {
    for (let i = 0; i < this.size; i++) {
      this.data[(this.head + i) % this.cap] = void 0;
    }
    this.head = 0;
    this.size = 0;
  }
};

// src/errors.ts
var ConnectionClosedError = class extends Error {
  constructor() {
    super("wspulse: connection is closed");
    this.name = "ConnectionClosedError";
  }
};
var RetriesExhaustedError = class extends Error {
  constructor() {
    super("wspulse: max reconnect retries exhausted");
    this.name = "RetriesExhaustedError";
  }
};
var ConnectionLostError = class extends Error {
  constructor() {
    super("wspulse: connection lost");
    this.name = "ConnectionLostError";
  }
};
var SendBufferFullError = class extends Error {
  constructor() {
    super("wspulse: send buffer full");
    this.name = "SendBufferFullError";
  }
};
var ServerClosedError = class extends Error {
  code;
  reason;
  constructor(code, reason) {
    const suffix = reason === "" ? "" : `, reason=${JSON.stringify(reason)}`;
    super(`wspulse: server closed connection: code=${code}${suffix}`);
    this.name = "ServerClosedError";
    this.code = code;
    this.reason = reason;
  }
};

// src/backoff.ts
function backoff(attempt, baseDelay, maxDelay) {
  const shift = Math.min(attempt, 62);
  let delay = baseDelay * 2 ** shift;
  if (delay > maxDelay || delay <= 0) {
    delay = maxDelay;
  }
  const half = delay / 2;
  return half + Math.random() * (delay - half);
}

// src/client.ts
function normalizeScheme(url) {
  const lower = url.slice(0, 8).toLowerCase();
  if (lower.startsWith("https://"))
    return "wss://" + url.slice("https://".length);
  if (lower.startsWith("http://")) return "ws://" + url.slice("http://".length);
  return url;
}
var WS_OPEN = 1;
var WS_CLOSE_NORMAL = 1e3;
var WS_CLOSE_GOING_AWAY = 1001;
async function dialWebSocket(url, opts) {
  let wsImpl = null;
  try {
    const mod = await import("ws");
    wsImpl = mod.default ?? mod;
  } catch {
  }
  const hasHeaders = Object.keys(opts.dialHeaders).length > 0;
  const wsOpts = {};
  if (hasHeaders) wsOpts.headers = opts.dialHeaders;
  return new Promise((resolve, reject) => {
    let ws;
    if (wsImpl !== null) {
      ws = new wsImpl(url, Object.keys(wsOpts).length > 0 ? wsOpts : void 0);
    } else {
      if (typeof globalThis.WebSocket === "undefined") {
        reject(
          new Error(
            "wspulse: no WebSocket implementation available (install 'ws' package or run in a browser)"
          )
        );
        return;
      }
      ws = new globalThis.WebSocket(url);
    }
    ws.onopen = () => {
      ws.onopen = null;
      ws.onerror = null;
      resolve(ws);
    };
    let lastError = null;
    if (typeof ws.on === "function") {
      ws.on("error", (err) => {
        if (err instanceof Error) lastError = err;
      });
    }
    ws.onerror = (ev) => {
      ws.onopen = null;
      ws.onerror = null;
      const msg = lastError?.message ?? ev.message ?? "connection failed";
      reject(new Error(`wspulse: dial failed: ${msg}`));
    };
  });
}
async function connect(url, opts) {
  url = normalizeScheme(url);
  const resolved = resolveOptions(opts);
  const dial = resolved._dialer ?? dialWebSocket;
  const ws = await dial(url, resolved);
  return new WspulseClient(url, resolved, ws);
}
var WspulseClient = class {
  url;
  opts;
  ws;
  /** Bounded send buffer (throws when full). */
  sendBuffer;
  /** Whether the client is permanently closed. */
  closed = false;
  /**
   * Set by the client immediately before internal `ws.close(code, reason)`
   * calls that still flow through the shared `ws.onclose` handler (currently
   * the write timeout/write error paths). That handler reads this flag to
   * decide whether to surface a {@link ServerClosedError} — a self-initiated
   * close must not be reported as if the server sent the close frame.
   */
  selfClosing = false;
  /** Fires exactly once when the client reaches CLOSED state. */
  doneResolve;
  /** Public done Promise — resolves on permanent disconnect. */
  done;
  /** Drain timer for flushing the send buffer. */
  drainTimer = null;
  /** Whether an async flush is in progress (prevents re-entry). */
  draining = false;
  /** AbortController for cancelling the reconnect loop. */
  abortController;
  /** Whether onDisconnect has been called (exactly-once guard). */
  disconnectFired = false;
  /** Whether a transport drop is being handled. Suppresses onTransportDrop(null) during shutdown. */
  reconnecting = false;
  /** Timer clock — replaced in tests for deterministic behaviour. @internal */
  clock;
  constructor(url, opts, ws) {
    this.url = url;
    this.opts = opts;
    this.ws = ws;
    this.clock = opts._clock;
    this.sendBuffer = new RingBuffer(opts.sendBufferSize);
    this.abortController = new AbortController();
    let resolve;
    this.done = new Promise((r) => {
      resolve = r;
    });
    this.doneResolve = resolve;
    if (opts.codec.binaryType === "binary") {
      ws.binaryType = "arraybuffer";
    }
    this.attachListeners(ws);
  }
  /**
   * Enqueue a Message for delivery.
   *
   * @throws {@link ConnectionClosedError} if the client is in CLOSED state.
   * @throws {@link SendBufferFullError} if the internal send buffer is full.
   */
  send(msg) {
    if (this.closed) {
      throw new ConnectionClosedError();
    }
    const data = this.opts.codec.encode(msg);
    if (!this.sendBuffer.push(data)) {
      throw new SendBufferFullError();
    }
    this.startDrain();
  }
  /**
   * Permanently terminate the connection and stop any reconnect loop.
   * Idempotent.
   */
  close() {
    if (this.closed) return;
    this.shutdown(null);
  }
  // ── internal ──────────────────────────────────────────────────────────────
  /**
   * Attach message/close/error listeners to a WebSocket instance.
   * Called on initial connect and after each successful reconnect.
   */
  attachListeners(ws) {
    ws.onmessage = (ev) => {
      if (this.closed || this.disconnectFired) return;
      const data = ev.data;
      let normalized;
      let byteLength;
      if (typeof data === "string") {
        normalized = data;
        byteLength = typeof Buffer !== "undefined" ? Buffer.byteLength(data, "utf8") : new TextEncoder().encode(data).byteLength;
      } else if (typeof Buffer !== "undefined" && Buffer.isBuffer(data)) {
        const buf = data;
        byteLength = buf.byteLength;
        normalized = new Uint8Array(buf.buffer, buf.byteOffset, buf.byteLength);
      } else if (data instanceof ArrayBuffer) {
        byteLength = data.byteLength;
        normalized = new Uint8Array(data);
      } else if (ArrayBuffer.isView(data)) {
        byteLength = data.byteLength;
        normalized = new Uint8Array(
          data.buffer,
          data.byteOffset,
          data.byteLength
        );
      } else {
        ws.onclose = null;
        ws.close(1003, "unsupported payload type");
        this.handleTransportDrop();
        return;
      }
      if (this.opts.maxMessageSize > 0 && byteLength > this.opts.maxMessageSize) {
        ws.onclose = null;
        ws.close(1009, "message too large");
        this.handleTransportDrop();
        return;
      }
      try {
        const msg = this.opts.codec.decode(normalized);
        this.opts.onMessage(msg);
      } catch (err) {
        console.warn("wspulse/client: decode failed, message dropped", err);
      }
    };
    ws.onclose = (ev) => {
      let dropErr;
      if (this.selfClosing) {
        this.selfClosing = false;
        dropErr = void 0;
      } else if (ev.code === 1006) {
        dropErr = void 0;
      } else {
        dropErr = new ServerClosedError(ev.code, ev.reason);
      }
      this.handleTransportDrop(dropErr);
    };
    ws.onerror = () => {
    };
  }
  /**
   * Handle an unexpected transport drop.
   *
   * If auto-reconnect is enabled, starts the reconnect loop.
   * Otherwise, transitions to CLOSED immediately.
   *
   * @param cause  If the drop was triggered by a server close frame, pass
   *               the {@link ServerClosedError} so onTransportDrop sees
   *               the code and reason. Leave undefined for abrupt drops
   *               (default: a generic "transport closed unexpectedly" error).
   */
  handleTransportDrop(cause) {
    if (this.closed) return;
    this.stopDrain();
    const dropErr = cause ?? new Error("wspulse: transport closed unexpectedly");
    this.reconnecting = true;
    try {
      this.opts.onTransportDrop(dropErr);
    } catch (cbErr) {
      console.warn("wspulse/client: onTransportDrop threw", cbErr);
    }
    if (this.opts.autoReconnect) {
      void this.reconnectLoop();
    } else {
      this.shutdown(new ConnectionLostError());
    }
  }
  /**
   * Reconnect loop with exponential backoff.
   *
   * Runs as an async task. Stops when:
   * - A reconnect attempt succeeds.
   * - Max retries are exhausted → CLOSED with RetriesExhaustedError.
   * - `close()` is called → CLOSED with null.
   */
  async reconnectLoop() {
    const rc = this.opts.autoReconnect;
    const signal = this.abortController.signal;
    let attempt = 0;
    while (!this.closed) {
      if (rc.maxRetries > 0 && attempt >= rc.maxRetries) {
        this.shutdown(new RetriesExhaustedError());
        return;
      }
      const delay = backoff(attempt, rc.baseDelay, rc.maxDelay);
      const aborted = await this.abortableDelay(delay, signal);
      if (aborted || this.closed) return;
      let newWs;
      try {
        const dial = this.opts._dialer ?? dialWebSocket;
        newWs = await dial(this.url, this.opts);
      } catch {
        attempt++;
        continue;
      }
      if (this.closed) {
        newWs.close();
        return;
      }
      this.ws = newWs;
      if (this.opts.codec.binaryType === "binary") {
        newWs.binaryType = "arraybuffer";
      }
      this.attachListeners(newWs);
      this.startDrain();
      this.reconnecting = false;
      try {
        this.opts.onTransportRestore();
      } catch (err) {
        console.warn("wspulse/client: onTransportRestore threw", err);
      }
      return;
    }
  }
  /**
   * Sleep for `ms` milliseconds, but resolve early with `true` if `signal`
   * is aborted (i.e. `close()` was called).
   *
   * @returns `true` if aborted, `false` if the delay completed normally.
   */
  abortableDelay(ms, signal) {
    if (signal.aborted) return Promise.resolve(true);
    return new Promise((resolve) => {
      const timer = this.clock.setTimeout(() => {
        signal.removeEventListener("abort", onAbort);
        resolve(false);
      }, ms);
      const onAbort = () => {
        this.clock.clearTimeout(timer);
        resolve(true);
      };
      signal.addEventListener("abort", onAbort, { once: true });
    });
  }
  /**
   * Start the drain timer that flushes the send buffer after a short delay.
   *
   * Uses a one-shot timer so idle clients do not incur continuous wakeups.
   * Called from send() and after a successful reconnect.
   */
  startDrain() {
    if (this.draining || this.drainTimer !== null) return;
    this.drainTimer = this.clock.setTimeout(() => {
      this.drainTimer = null;
      void this.flushSendBuffer();
    }, 5);
  }
  /**
   * Stop any scheduled drain timer.
   *
   * Does not reset `draining` — an async flush may be in progress and
   * its `finally` block is responsible for clearing the flag.
   */
  stopDrain() {
    if (this.drainTimer !== null) {
      this.clock.clearTimeout(this.drainTimer);
      this.drainTimer = null;
    }
  }
  /**
   * Flush all buffered messages to the WebSocket serially with per-write
   * timeout. On Node.js each message is sent via `sendOneMessage` so a
   * stalled socket is detected within `writeWait`. In browsers `send()`
   * is fire-and-forget (no completion callback) so no deadline applies.
   *
   * Stops draining if the socket is not open (reconnect will restart it).
   */
  async flushSendBuffer() {
    if (this.ws.readyState !== WS_OPEN) return;
    this.draining = true;
    try {
      while (this.sendBuffer.length > 0 && !this.closed) {
        if (this.ws.readyState !== WS_OPEN) return;
        const encoded = this.sendBuffer.peek();
        if (encoded === void 0) {
          this.sendBuffer.shift();
          continue;
        }
        const ok = await this.sendOneMessage(encoded);
        if (!ok) return;
        this.sendBuffer.shift();
      }
    } finally {
      this.draining = false;
      if (this.sendBuffer.length > 0 && !this.closed) {
        this.startDrain();
      }
    }
  }
  /**
   * Send a single message with write-deadline enforcement.
   *
   * On Node.js (`ws` library): uses the callback form of `send()` and
   * races it against a `writeWait` timeout. On timeout the socket is
   * closed, which triggers `handleTransportDrop`.
   *
   * In browsers: `send()` is fire-and-forget; returns `true` immediately.
   *
   * @returns `true` if the write completed, `false` if it timed out or errored.
   */
  sendOneMessage(data) {
    if (this.ws.readyState !== WS_OPEN) return Promise.resolve(false);
    const ws = this.ws;
    if (typeof ws.on !== "function") {
      try {
        ws.send(data);
      } catch {
        return Promise.resolve(false);
      }
      return Promise.resolve(true);
    }
    return new Promise((resolve) => {
      let settled = false;
      const timer = this.clock.setTimeout(() => {
        if (settled) return;
        settled = true;
        if (this.ws === ws) {
          try {
            this.selfClosing = true;
            ws.close(WS_CLOSE_GOING_AWAY, "write timeout");
          } catch {
            this.selfClosing = false;
          }
        }
        resolve(false);
      }, this.opts.writeWait);
      try {
        ws.send(data, (err) => {
          if (settled) return;
          settled = true;
          this.clock.clearTimeout(timer);
          if (err) {
            if (this.ws === ws) {
              try {
                this.selfClosing = true;
                ws.close(WS_CLOSE_GOING_AWAY, "write error");
              } catch {
                this.selfClosing = false;
              }
            }
            resolve(false);
          } else {
            resolve(true);
          }
        });
      } catch {
        if (settled) return;
        settled = true;
        this.clock.clearTimeout(timer);
        if (this.ws === ws) {
          try {
            this.selfClosing = true;
            ws.close(WS_CLOSE_GOING_AWAY, "write error");
          } catch {
            this.selfClosing = false;
          }
        }
        resolve(false);
      }
    });
  }
  /**
   * Transition to CLOSED state. Releases all resources.
   *
   * @param err `null` for clean close, an Error for abnormal disconnect.
   */
  shutdown(err) {
    if (this.closed) return;
    this.closed = true;
    this.abortController.abort();
    this.stopDrain();
    this.sendBuffer.clear();
    try {
      this.ws.onmessage = null;
      this.ws.onclose = null;
      this.ws.onerror = null;
      this.ws.close(WS_CLOSE_NORMAL, "");
    } catch {
    }
    if (err === null && !this.reconnecting) {
      try {
        this.opts.onTransportDrop(null);
      } catch (cbErr) {
        console.warn("wspulse/client: onTransportDrop threw", cbErr);
      }
    }
    this.reconnecting = false;
    if (!this.disconnectFired) {
      this.disconnectFired = true;
      try {
        this.opts.onDisconnect(err);
      } catch (cbErr) {
        console.warn("wspulse/client: onDisconnect threw", cbErr);
      }
    }
    this.doneResolve();
  }
};

// src/status.ts
var StatusCode = {
  /** Normal, intentional close (1000). */
  NormalClosure: 1e3,
  /** Endpoint is going away — server shutting down or browser tab closing (1001). */
  GoingAway: 1001,
  /** Protocol error (1002). */
  ProtocolError: 1002,
  /** Endpoint received a frame type it cannot accept (1003). */
  UnsupportedData: 1003,
  /** Received data not consistent with the message type (1007). */
  InvalidFramePayloadData: 1007,
  /** Endpoint policy violation (1008). */
  PolicyViolation: 1008,
  /** Message too large to process (1009). */
  MessageTooBig: 1009,
  /** Client expected a required extension the server did not return (1010). */
  MandatoryExtension: 1010,
  /** Server encountered an unexpected condition (1011). */
  InternalError: 1011,
  // --- Local-only sentinels (MUST NOT be sent on the wire, per RFC 6455 §7.4.1) ---
  /** No status code was present in the close frame (1005, local-only). */
  NoStatusReceived: 1005,
  /** Connection closed abnormally without a close frame (1006, local-only). */
  AbnormalClosure: 1006,
  /** TLS handshake failure (1015, local-only). */
  TLSHandshake: 1015
};
export {
  ConnectionClosedError,
  ConnectionLostError,
  JSONCodec,
  RetriesExhaustedError,
  SendBufferFullError,
  ServerClosedError,
  StatusCode,
  backoff,
  connect
};
//# sourceMappingURL=index.js.map