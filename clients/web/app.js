"use strict";
/* AnotherMUD web client — P1 shell.
 *
 * A pure-browser view over the EXISTING wire (internal/conn/ws): it sends
 * `command` envelopes, renders `text` frames (ANSI → HTML), and consumes the
 * engine GMCP packages into HUD panels. Zero server changes — this is the P1
 * "superset baseline" from docs/themes/web-client-plan.md: everything here is a
 * VIEW; every action reduces to a command the server already accepts. */

const $ = (id) => document.getElementById(id);

/* ── DOM ─────────────────────────────────────────────────────── */
const el = {
  connForm: $("conn-form"),
  url: $("conn-url"),
  connBtn: $("conn-btn"),
  status: $("conn-status"),
  statusLabel: $("conn-status").querySelector(".conn-label"),
  terminal: $("terminal"),
  inputForm: $("input-form"),
  cmd: $("cmd"),
  sendBtn: $("send-btn"),
  identity: $("p-identity"),
  identName: $("ident-name"),
  identSub: $("ident-sub"),
  vitals: $("p-vitals"),
  vitalsBars: $("vitals-bars"),
  vitalsSust: $("vitals-sustenance"),
  combat: $("p-combat"),
  combatBody: $("combat-body"),
  room: $("p-room"),
  roomName: $("room-name"),
  roomMeta: $("room-meta"),
  roomExits: $("room-exits"),
  minimap: $("minimap"),
  effects: $("p-effects"),
  effectsList: $("effects-list"),
  xp: $("p-xp"),
  xpTracks: $("xp-tracks"),
};

const escapeHtml = (s) =>
  s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

/* ── ANSI → HTML ─────────────────────────────────────────────────
 * The server emits standard SGR escapes (16-color, bold/dim/italic/underline,
 * and 24-bit truecolor `38;2;r;g;b`). We translate a text chunk into escaped
 * HTML spans, tracking SGR state within the chunk and closing any open span at
 * the end. Non-SGR CSI sequences (cursor moves, etc.) are stripped. */
const CSI = /\x1b\[([0-9;]*)([A-Za-z])/g;

function ansiToHtml(input) {
  let out = "";
  let last = 0;
  let state = null; // {classes:[], style:""} or null
  let open = false;

  const closeSpan = () => {
    if (open) {
      out += "</span>";
      open = false;
    }
  };
  const openSpan = () => {
    if (!state) return;
    let cls = state.classes.join(" ");
    let attr = cls ? ` class="${cls}"` : "";
    if (state.style) attr += ` style="${state.style}"`;
    out += `<span${attr}>`;
    open = true;
  };
  const emit = (text) => {
    if (!text) return;
    if (state) {
      if (!open) openSpan();
      out += escapeHtml(text);
    } else {
      closeSpan();
      out += escapeHtml(text);
    }
  };

  CSI.lastIndex = 0;
  let m;
  while ((m = CSI.exec(input)) !== null) {
    emit(input.slice(last, m.index));
    last = CSI.lastIndex;
    if (m[2] !== "m") continue; // non-SGR CSI: strip
    const params = m[1] === "" ? [0] : m[1].split(";").map((n) => parseInt(n, 10) || 0);
    applySgr(params);
  }
  emit(input.slice(last));
  closeSpan();
  return out;

  function applySgr(params) {
    for (let i = 0; i < params.length; i++) {
      const p = params[i];
      if (p === 0) {
        // reset — a run boundary; drop the current span state
        closeSpan();
        state = null;
      } else if (p === 38 && params[i + 1] === 2) {
        setStyle(`color:rgb(${params[i + 2]},${params[i + 3]},${params[i + 4]})`);
        i += 4;
      } else if (p === 48 && params[i + 1] === 2) {
        setStyle(`background:rgb(${params[i + 2]},${params[i + 3]},${params[i + 4]})`, true);
        i += 4;
      } else if (p >= 30 && p <= 37) addClass(`fg-${p - 30}`);
      else if (p >= 90 && p <= 97) addClass(`fg-${p - 90 + 8}`);
      else if (p === 39) removeFg();
      else if (p === 1) addClass("ansi-bold");
      else if (p === 2) addClass("ansi-dim");
      else if (p === 3) addClass("ansi-italic");
      else if (p === 4) addClass("ansi-underline");
      // background/other codes we don't map are ignored (harmless)
    }
  }
  function ensure() {
    if (!state) state = { classes: [], style: "" };
    closeSpan();
  }
  function addClass(c) {
    ensure();
    if (!state.classes.includes(c)) state.classes.push(c);
  }
  function removeFg() {
    if (!state) return;
    closeSpan();
    state.classes = state.classes.filter((c) => !c.startsWith("fg-"));
    state.style = state.style.replace(/color:[^;]*;?/, "");
  }
  function setStyle(css, bg) {
    ensure();
    if (!bg) state.classes = state.classes.filter((c) => !c.startsWith("fg-"));
    state.style += css + ";";
  }
}

/* ── Terminal ────────────────────────────────────────────────── */
const MAX_LINES = 5000;

function atBottom() {
  const t = el.terminal;
  return t.scrollHeight - t.scrollTop - t.clientHeight < 40;
}
function scrollDown() {
  el.terminal.scrollTop = el.terminal.scrollHeight;
}
function trimScrollback() {
  while (el.terminal.childNodes.length > MAX_LINES) {
    el.terminal.removeChild(el.terminal.firstChild);
  }
}

// The server streams text with embedded CR-LF; we render each frame as a block
// so wrapping + ANSI runs stay intact, then let CSS `white-space: pre-wrap`
// handle the newlines.
function writeServer(text) {
  const stick = atBottom();
  const span = document.createElement("span");
  span.className = "line";
  span.innerHTML = ansiToHtml(text.replace(/\r\n/g, "\n").replace(/\r/g, "\n"));
  el.terminal.appendChild(span);
  trimScrollback();
  if (stick) scrollDown();
  maybeMaskPassword(text);
}
function writeLocalEcho(cmd) {
  const stick = atBottom();
  const span = document.createElement("span");
  span.className = "line line-local";
  span.textContent = cmd;
  el.terminal.appendChild(span);
  span.appendChild(document.createTextNode("\n"));
  if (stick) scrollDown();
}
function writeSys(msg) {
  const span = document.createElement("span");
  span.className = "line line-sys";
  span.textContent = msg + "\n";
  el.terminal.appendChild(span);
  scrollDown();
}

// WS has no telnet echo negotiation (SuppressEcho is a server no-op, §6.5), so
// we mask locally: when the server's latest prompt asks for a password, switch
// the input to a password field until the next non-password prompt.
function maybeMaskPassword(text) {
  const tail = text.replace(/\x1b\[[0-9;]*[A-Za-z]/g, "").trim().slice(-40).toLowerCase();
  if (/password|passphrase/.test(tail) && !/incorrect|wrong|mismatch/.test(tail)) {
    el.cmd.type = "password";
  } else if (tail.length > 0) {
    el.cmd.type = "text";
  }
}

/* ── GMCP dispatch ───────────────────────────────────────────── */
const gmcpHandlers = {
  "Char.Login": (d) => {
    el.identName.textContent = d.name || "—";
    el.identSub.textContent = d.account ? `account: ${d.account}` : "";
    el.identity.hidden = false;
  },
  "Char.Status": (d) => {
    const parts = [d.race, d.class].filter(Boolean).join(" · ");
    const align = d.alignment_tag ? ` — ${d.alignment_tag}` : "";
    el.identSub.textContent = (parts + align) || el.identSub.textContent;
    el.identity.hidden = false;
  },
  "Char.Vitals": renderVitals,
  "Char.Combat": renderCombat,
  "Room.Info": renderRoom,
  "Char.Effects": renderEffects,
  "Char.Experience": renderXp,
  // Char.Items.List / Char.StatusVars / Comm.Channel.Text / Char.Wizard are
  // received but not yet surfaced in P1 — dispatched to a no-op so unknown
  // packages never throw. (P2+ add panels; the wire already carries them.)
};

function dispatchGmcp(pkg, data) {
  const h = gmcpHandlers[pkg];
  if (h) {
    try {
      h(data);
    } catch (e) {
      console.warn("GMCP handler error for", pkg, e);
    }
  }
}

/* ── Panels ──────────────────────────────────────────────────── */

const POOL_META = {
  mana: { label: "MP", cls: "bar-mp" },
  movement: { label: "MV", cls: "bar-mv" },
};
function poolLabel(kind) {
  return (POOL_META[kind] && POOL_META[kind].label) || kind.replace(/_/g, " ").slice(0, 6);
}

function bar(label, cur, max, cls) {
  const pct = max > 0 ? Math.max(0, Math.min(100, (cur / max) * 100)) : 0;
  return `<div class="bar ${cls}">
    <span class="bar-label">${escapeHtml(label)}</span>
    <span class="bar-track"><span class="bar-fill" style="width:${pct}%"></span></span>
    <span class="bar-num">${cur}/${max}</span>
  </div>`;
}

function renderVitals(d) {
  const rows = [];
  rows.push(bar("HP", d.hp ?? 0, d.maxhp ?? 0, "bar-hp"));
  // The generalized pools map (kind → {cur,max}) drives the rest, so any world's
  // resources (One Power via mana, SR Essence/Stun, …) render without hardcoding.
  const pools = d.pools || {};
  const order = ["mana", "movement"]; // stable, HUD-friendly first
  const keys = Object.keys(pools).sort((a, b) => {
    const ai = order.indexOf(a), bi = order.indexOf(b);
    return (ai < 0 ? 99 : ai) - (bi < 0 ? 99 : bi) || a.localeCompare(b);
  });
  for (const k of keys) {
    const p = pools[k];
    if (!p || p.max <= 0) continue;
    const cls = (POOL_META[k] && POOL_META[k].cls) || "bar-pool";
    rows.push(bar(poolLabel(k), p.cur, p.max, cls));
  }
  el.vitalsBars.innerHTML = rows.join("");
  el.vitalsSust.textContent =
    d.sustenance != null ? `Sustenance ${d.sustenance}` : "";
  el.vitals.hidden = false;
}

function renderCombat(d) {
  el.combat.hidden = false;
  el.combat.dataset.active = String(!!d.in_combat);
  if (!d.in_combat) {
    el.combatBody.innerHTML = `<div class="combat-idle">Not in combat.</div>`;
    return;
  }
  const pct = d.target_hp_percent != null
    ? d.target_hp_percent
    : d.target_max_hp > 0
    ? Math.round((d.target_hp / d.target_max_hp) * 100)
    : 0;
  el.combatBody.innerHTML = `
    <div class="target-name">⚔ ${escapeHtml(d.target || "target")}</div>
    ${bar("HP", d.target_hp ?? pct, d.target_max_hp ?? 100, "bar-hp")}`;
}

function renderEffects(d) {
  const list = (d && d.effects) || [];
  el.effects.hidden = false;
  if (!list.length) {
    el.effectsList.innerHTML = `<li class="empty">None active.</li>`;
    return;
  }
  el.effectsList.innerHTML = list
    .map((e) => {
      const rem = e.permanent
        ? `<span class="effect-perm">∞</span>`
        : e.remaining
        ? `<span class="effect-rem">${e.remaining}s</span>`
        : "";
      return `<li class="effect"><span>${escapeHtml(e.id)}</span>${rem}</li>`;
    })
    .join("");
}

function renderXp(d) {
  const tracks = (d && d.tracks) || [];
  el.xp.hidden = false;
  if (!tracks.length) {
    el.xpTracks.innerHTML = `<div class="empty">No tracks.</div>`;
    return;
  }
  el.xpTracks.innerHTML = tracks
    .map((t) => {
      const label = t.name || t.track;
      const pct = t.at_max ? 100 : t.xpnext > 0 ? Math.min(100, (Number(t.xp) / Number(t.xpnext)) * 100) : 0;
      const lvl = t.at_max ? `Lv ${t.level} (max)` : `Lv ${t.level}`;
      return `<div class="track">
        <div class="track-head"><span class="track-name">${escapeHtml(label)}</span><span class="track-lvl">${lvl}</span></div>
        <div class="track-bar"><span class="track-fill" style="width:${pct}%"></span></div>
      </div>`;
    })
    .join("");
}

/* ── Room + minimap ──────────────────────────────────────────── */
const rooms = new Map(); // num → {x,y,z,name,exits}
let current = null;

const OPPOSITE = {
  north: "south", south: "north", east: "west", west: "east",
  up: "down", down: "up", northeast: "southwest", southwest: "northeast",
  northwest: "southeast", southeast: "northwest",
};

function renderRoom(d) {
  el.room.hidden = false;
  el.roomName.textContent = d.name || "—";

  const meta = [];
  if (d.area) meta.push(`<span class="tag">area</span> ${escapeHtml(d.area)}`);
  if (d.terrain) meta.push(`<span class="tag">terrain</span> ${escapeHtml(d.terrain)}`);
  if (d.light) meta.push(`<span class="tag">light</span> ${escapeHtml(d.light)}`);
  if (d.x != null && d.y != null) meta.push(`<span class="tag">xyz</span> ${d.x},${d.y},${d.z ?? 0}`);
  el.roomMeta.innerHTML = meta.join("");

  const exits = d.exits || {};
  el.roomExits.innerHTML = Object.keys(exits)
    .map((dir) => `<button class="exit-btn" data-dir="${escapeHtml(dir)}" type="button">${escapeHtml(dir)}</button>`)
    .join("");

  if (d.num) {
    rooms.set(d.num, {
      x: d.x, y: d.y, z: d.z ?? 0, name: d.name, exits,
    });
    current = d.num;
  }
  drawMinimap();
}

// Clicking an exit sends the matching move command — an INTENT that reduces to
// the existing command surface (the P1 authority invariant: no new server verb).
el.roomExits.addEventListener("click", (e) => {
  const b = e.target.closest(".exit-btn");
  if (b && conn.socket) sendCommand(b.dataset.dir);
});

function drawMinimap() {
  const cvs = el.minimap;
  const ctx = cvs.getContext("2d");
  const W = cvs.width, H = cvs.height;
  ctx.clearRect(0, 0, W, H);
  if (!current || !rooms.has(current)) return;
  const cz = rooms.get(current).z;

  // Rooms on the current z-plane with known coords.
  const plane = [];
  for (const [num, r] of rooms) {
    if (r.z === cz && r.x != null && r.y != null) plane.push({ num, ...r });
  }
  if (!plane.length) return;

  const xs = plane.map((r) => r.x), ys = plane.map((r) => r.y);
  const minX = Math.min(...xs), maxX = Math.max(...xs);
  const minY = Math.min(...ys), maxY = Math.max(...ys);
  const pad = 24;
  const spanX = Math.max(1, maxX - minX), spanY = Math.max(1, maxY - minY);
  const step = Math.min((W - pad * 2) / spanX, (H - pad * 2) / spanY, 34) || 20;
  const ox = W / 2 - ((minX + maxX) / 2) * step;
  const oy = H / 2 + ((minY + maxY) / 2) * step; // y grows up → invert
  const px = (r) => ox + r.x * step;
  const py = (r) => oy - r.y * step;

  const accent = getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() || "#4fd";
  const dim = getComputedStyle(document.documentElement).getPropertyValue("--line-strong").trim() || "#556";

  // Edges (only when both endpoints are known on this plane).
  ctx.strokeStyle = dim;
  ctx.lineWidth = 1.5;
  for (const r of plane) {
    for (const dir of Object.keys(r.exits || {})) {
      const target = r.exits[dir];
      const t = rooms.get(target);
      if (t && t.z === cz && t.x != null) {
        ctx.beginPath();
        ctx.moveTo(px(r), py(r));
        ctx.lineTo(px(t), py(t));
        ctx.stroke();
      }
    }
  }
  // Nodes.
  for (const r of plane) {
    const isCur = r.num === current;
    ctx.fillStyle = isCur ? accent : dim;
    ctx.beginPath();
    ctx.arc(px(r), py(r), isCur ? 5.5 : 3.5, 0, Math.PI * 2);
    ctx.fill();
    if (isCur) {
      ctx.strokeStyle = accent;
      ctx.globalAlpha = 0.35;
      ctx.beginPath();
      ctx.arc(px(r), py(r), 10, 0, Math.PI * 2);
      ctx.stroke();
      ctx.globalAlpha = 1;
    }
  }
}

/* ── Connection ──────────────────────────────────────────────── */
const conn = { socket: null };

function setStatus(state, label) {
  el.status.dataset.state = state;
  el.statusLabel.textContent = label;
}
function setConnected(on) {
  el.cmd.disabled = !on;
  el.sendBtn.disabled = !on;
  el.connBtn.textContent = on ? "Disconnect" : "Connect";
  el.url.disabled = on;
  if (on) el.cmd.focus();
}

function connect(url) {
  let socket;
  try {
    socket = new WebSocket(url);
  } catch (e) {
    writeSys(`Bad URL: ${e.message}`);
    setStatus("error", "Bad URL");
    return;
  }
  conn.socket = socket;
  setStatus("connecting", "Connecting…");
  writeSys(`Connecting to ${url} …`);

  socket.onopen = () => {
    setStatus("online", "Online");
    setConnected(true);
    writeSys("Connected. GMCP is active (WebSocket).");
  };
  socket.onmessage = (ev) => {
    let env;
    try {
      env = JSON.parse(ev.data);
    } catch {
      return; // non-JSON frame — ignore, per §6.1
    }
    if (env.type === "text") writeServer(String(env.data ?? ""));
    else if (env.type === "gmcp") dispatchGmcp(env.package, env.data);
    // unknown types ignored
  };
  socket.onerror = () => {
    setStatus("error", "Error");
    writeSys("Connection error. If the server rejects the origin, run it with ANOTHERMUD_WS_INSECURE_SKIP_VERIFY=true (dev) or add your origin to ANOTHERMUD_WS_ORIGINS.");
  };
  socket.onclose = (ev) => {
    conn.socket = null;
    setConnected(false);
    if (el.status.dataset.state !== "error") setStatus("offline", "Offline");
    writeSys(`Disconnected${ev.reason ? " — " + ev.reason : ""}.`);
  };
}

function disconnect() {
  if (conn.socket) conn.socket.close();
}

function sendCommand(text) {
  if (!conn.socket || conn.socket.readyState !== WebSocket.OPEN) return;
  conn.socket.send(JSON.stringify({ type: "command", data: text }));
  writeLocalEcho(el.cmd.type === "password" ? "•".repeat(Math.min(text.length, 12)) : text);
}

/* ── Input + history ─────────────────────────────────────────── */
const history = [];
let histIdx = -1;

el.connForm.addEventListener("submit", (e) => {
  e.preventDefault();
  if (conn.socket) disconnect();
  else connect(el.url.value.trim());
});

el.inputForm.addEventListener("submit", (e) => {
  e.preventDefault();
  const text = el.cmd.value;
  if (!conn.socket) return;
  sendCommand(text);
  if (text.trim() && el.cmd.type !== "password") {
    history.push(text);
    if (history.length > 200) history.shift();
  }
  histIdx = history.length;
  el.cmd.value = "";
});

el.cmd.addEventListener("keydown", (e) => {
  if (e.key === "ArrowUp") {
    if (histIdx > 0) {
      histIdx--;
      el.cmd.value = history[histIdx];
      queueMicrotask(() => el.cmd.setSelectionRange(el.cmd.value.length, el.cmd.value.length));
    }
    e.preventDefault();
  } else if (e.key === "ArrowDown") {
    if (histIdx < history.length - 1) {
      histIdx++;
      el.cmd.value = history[histIdx];
    } else {
      histIdx = history.length;
      el.cmd.value = "";
    }
    e.preventDefault();
  }
});

// Clicking anywhere in the terminal focuses the input (terminal-like feel),
// unless the user is selecting text.
el.terminal.addEventListener("mouseup", () => {
  if (!window.getSelection().toString()) el.cmd.focus();
});

writeSys("AnotherMUD web client — P1. Set the server URL and press Connect.");
writeSys("Start the server with ANOTHERMUD_WS_ADDR=:4001 (see clients/web/README.md).");
