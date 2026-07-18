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
  completeList: $("complete-list"),
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
  inventory: $("p-inventory"),
  invWorn: $("inv-worn"),
  invCarried: $("inv-carried"),
  recipes: $("p-recipes"),
  recipesList: $("recipes-list"),
  shop: $("p-shop"),
  shopTitle: $("shop-title"),
  shopMoney: $("shop-money"),
  shopBuy: $("shop-buy"),
  shopSell: $("shop-sell"),
  quests: $("p-quests"),
  questsCount: $("quests-count"),
  questsList: $("quests-list"),
  trade: $("p-trade"),
  tradePartner: $("trade-partner"),
  tradeMineList: $("trade-mine-list"),
  tradeMineCheck: $("trade-mine-check"),
  tradeTheirsList: $("trade-theirs-list"),
  tradeTheirsCheck: $("trade-theirs-check"),
  tradeTheirsName: $("trade-theirs-name"),
  auction: $("p-auction"),
  auctionMoney: $("auction-money"),
  auctionCollect: $("auction-collect"),
  auctionList: $("auction-list"),
  auctionMore: $("auction-more"),
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
  "Room.Map": renderRoomMap,
  "Char.Effects": renderEffects,
  "Char.Experience": renderXp,
  "Char.Inventory": renderInventory,
  "Char.Recipes": renderRecipes,
  "Char.Shop": renderShop,
  "Char.Quests": renderQuests,
  "Char.Trade": renderTrade,
  "Char.Auction": renderAuction,
  // Char.Items.List / Char.StatusVars / Comm.Channel.Text / Char.Wizard are
  // received but not yet surfaced — dispatched to a no-op so unknown packages
  // never throw. (Char.Inventory is the P3 richer superset of Char.Items.List.)
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
  one_power: { label: "OP", cls: "bar-mp" },
  essence: { label: "ESS", cls: "bar-pool" },
  stun: { label: "STUN", cls: "bar-pool" },
};
// A short label for a pool kind: a known alias, else the kind upper-cased and
// capped so it fits the fixed label column (no mid-word truncation surprises
// like "ESSENC").
function poolLabel(kind) {
  return (POOL_META[kind] && POOL_META[kind].label) || kind.replace(/_/g, " ").toUpperCase().slice(0, 5);
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

/* ── Inventory (Char.Inventory, P3) ───────────────────────────────
 * The rich structured-inventory package, mirroring the in-game inventory/
 * equipment verbs: carried + worn items, stacked counts, a mechanical detail
 * line (ammo/armor/mods), the full worn-slot layout (empties included), and
 * per-item action buttons. Each action carries its FULL command string, so a
 * click sends exactly the command a player would type (the authority
 * invariant). Nothing here computes game state — it is a view + a richer input
 * surface over the existing equip/unequip/drop/reload/load commands. */

// actionButtons renders one button per affordance; each carries the full
// command to send (data-cmd), so the client never guesses an argument.
function actionButtons(actions) {
  return (actions || [])
    .map(
      (a) =>
        `<button class="inv-btn" type="button" data-cmd="${escapeHtml(a.cmd)}">${escapeHtml(a.label)}</button>`
    )
    .join("");
}

// itemRow renders an occupied row: name, optional stack count, optional detail
// (ammo/armor/mods), and its action buttons.
function itemRow(item) {
  const qty = item.qty > 1 ? `<span class="inv-qty">×${item.qty}</span>` : "";
  const detail = item.detail ? `<span class="inv-detail">${escapeHtml(item.detail)}</span>` : "";
  return `<div class="inv-item">
    <span class="inv-name">${escapeHtml(item.name || "")}${qty}${detail}</span>
    <span class="inv-actions">${actionButtons(item.actions)}</span>
  </div>`;
}

// wornRow renders one equipment slot: the slot label plus either "(empty)" or
// the equipped item's row.
function wornRow(w) {
  const body = w.empty
    ? `<span class="inv-empty">(empty)</span>`
    : itemRow(w);
  return `<div class="inv-slot-row"><span class="inv-slot">${escapeHtml(w.slot || "")}</span>${body}</div>`;
}

function renderInventory(d) {
  el.inventory.hidden = false;
  const worn = (d && d.worn) || [];
  const carried = (d && d.carried) || [];
  el.invWorn.innerHTML = worn.length
    ? worn.map(wornRow).join("")
    : `<div class="empty">No equipment slots.</div>`;
  el.invCarried.innerHTML = carried.length
    ? carried.map(itemRow).join("")
    : `<div class="empty">Nothing carried.</div>`;
}

// An action button sends its command; cancels any active click-to-walk (a
// manual action interrupts the walk, same as typing a command).
el.inventory.addEventListener("click", (e) => {
  const b = e.target.closest(".inv-btn");
  if (b && conn.socket) {
    walkTo = null;
    sendCommand(b.dataset.cmd);
  }
});

/* ── Craft form (Char.Recipes, P3 Slice B) ────────────────────────
 * The rich craft-form package, mirroring the in-game `craft` verb: the known
 * recipes with per-ingredient have/need counts, station + skill gates, and a
 * craftable-now flag. Each row's "Craft" button carries the FULL command
 * (`craft <recipe>`), so a click sends exactly what a player would type (the
 * authority invariant). The server is the sole judge of whether a craft
 * succeeds — greying an unmakeable row is a hint, never a gate. */

// ingredientLine renders one input: name and have/need, marked short when the
// crafter lacks the required quantity.
function ingredientLine(ing) {
  const short = ing.have < ing.need ? " ingredient-short" : "";
  return `<li class="ingredient${short}"><span class="ingredient-name">${escapeHtml(
    ing.name || ""
  )}</span><span class="ingredient-qty">${ing.have}/${ing.need}</span></li>`;
}

// recipeRow renders one recipe card: name, discipline, ingredient list, and a
// Craft button (disabled + reason when the recipe isn't craftable now).
function recipeRow(r) {
  const ings = (r.ingredients || []).map(ingredientLine).join("");
  const disc = r.discipline
    ? `<span class="recipe-disc">${escapeHtml(r.discipline)}</span>`
    : "";
  const blocked = !r.craftable;
  const btn = blocked
    ? `<button class="recipe-btn" type="button" disabled title="${escapeHtml(
        r.blocked || ""
      )}">Craft</button><span class="recipe-blocked">${escapeHtml(r.blocked || "")}</span>`
    : `<button class="recipe-btn" type="button" data-cmd="${escapeHtml(
        r.cmd || ""
      )}">Craft</button>`;
  return `<div class="recipe${blocked ? " recipe-blocked-row" : ""}">
    <div class="recipe-head"><span class="recipe-name">${escapeHtml(
      r.name || ""
    )}</span>${disc}</div>
    <ul class="ingredients">${ings}</ul>
    <div class="recipe-actions">${btn}</div>
  </div>`;
}

function renderRecipes(d) {
  const recipes = (d && d.recipes) || [];
  // Hide the panel entirely for a character who knows no recipes, so a
  // non-crafter's HUD isn't cluttered with an empty Crafting section.
  if (!recipes.length) {
    el.recipes.hidden = true;
    el.recipesList.innerHTML = "";
    return;
  }
  el.recipes.hidden = false;
  el.recipesList.innerHTML = recipes.map(recipeRow).join("");
}

// A Craft button sends its command; like the inventory buttons it cancels any
// active click-to-walk (a manual action interrupts the walk).
el.recipes.addEventListener("click", (e) => {
  const b = e.target.closest(".recipe-btn");
  if (b && !b.disabled && b.dataset.cmd && conn.socket) {
    walkTo = null;
    sendCommand(b.dataset.cmd);
  }
});

/* ── Trade form (Char.Shop, P3 Slice B+) ──────────────────────────
 * The contextual shop panel, shown only when the player stands at a shop
 * (open=true). Two columns — the shop's stock to buy (greyed when unaffordable)
 * and the player's sellable items — each row a button carrying its full
 * `buy <token>` / `sell <token>` command. Nothing here decides a sale; a click
 * sends the same command a player would type, and the server is the sole judge
 * (the authority invariant). */

// shopRow renders one buy/sell offer: name, optional qty, price, and a button
// carrying the full command (disabled + labelled when unaffordable).
function shopRow(o, verb) {
  const qty = o.qty > 1 ? `<span class="shop-qty">×${o.qty}</span>` : "";
  const price = `<span class="shop-price">${escapeHtml(o.price || "")}</span>`;
  const locked = verb === "buy" && !o.affordable;
  const btn = locked
    ? `<button class="shop-btn" type="button" disabled title="You can't afford that.">${verb}</button>`
    : `<button class="shop-btn" type="button" data-cmd="${escapeHtml(o.cmd || "")}">${verb}</button>`;
  return `<div class="shop-item${locked ? " shop-locked" : ""}">
    <span class="shop-name">${escapeHtml(o.name || "")}${qty}</span>
    <span class="shop-meta">${price}${btn}</span>
  </div>`;
}

function renderShop(d) {
  // Closed (not at a shop) → hide the panel entirely.
  if (!d || !d.open) {
    el.shop.hidden = true;
    return;
  }
  el.shop.hidden = false;
  el.shopTitle.textContent = d.shopkeeper || "Shop";
  el.shopMoney.textContent = d.money || "";
  if (d.refused) {
    const closed = `<div class="empty">The shopkeeper refuses to deal with you.</div>`;
    el.shopBuy.innerHTML = closed;
    el.shopSell.innerHTML = "";
    return;
  }
  const buy = d.buy || [];
  const sell = d.sell || [];
  el.shopBuy.innerHTML = buy.length
    ? buy.map((o) => shopRow(o, "buy")).join("")
    : `<div class="empty">Nothing for sale.</div>`;
  el.shopSell.innerHTML = sell.length
    ? sell.map((o) => shopRow(o, "sell")).join("")
    : `<div class="empty">Nothing to sell.</div>`;
}

// A buy/sell button sends its command; cancels any active click-to-walk.
el.shop.addEventListener("click", (e) => {
  const b = e.target.closest(".shop-btn");
  if (b && !b.disabled && b.dataset.cmd && conn.socket) {
    walkTo = null;
    sendCommand(b.dataset.cmd);
  }
});

/* ── Quest journal (Char.Quests, P3 Slice C) ──────────────────────
 * The rich journal package, mirroring the in-game `quests` verb: the active
 * quests with the current stage's description/hint and per-objective progress
 * (have/need + a done flag). Each abandonable quest carries an Abandon button
 * with the FULL `abandon <id>` command, so a click sends exactly what a player
 * would type (the authority invariant). Turn-in is done by returning to the
 * giver (not a bare command), so an awaiting-turn-in quest shows a badge, not a
 * button. Nothing here decides quest state — it is a view over the journal. */

// objectiveLine renders one objective: description, have/need, and a checkbox
// glyph, marked complete when current >= required.
function objectiveLine(o) {
  const done = o.complete;
  const mark = done ? "☑" : "☐";
  return `<li class="quest-obj${done ? " quest-obj-done" : ""}">
    <span class="quest-obj-mark" aria-hidden="true">${mark}</span>
    <span class="quest-obj-desc">${escapeHtml(o.desc || "")}</span>
    <span class="quest-obj-qty">${o.current}/${o.required}</span>
  </li>`;
}

// questCard renders one active quest: name + classification, an optional "ready
// to turn in" badge, the current stage line + hint, the objective list, and an
// Abandon button (only when abandonable).
function questCard(q) {
  const cls = q.classification
    ? `<span class="quest-class">${escapeHtml(q.classification)}</span>`
    : "";
  const badge = q.awaitingTurnIn
    ? `<span class="quest-badge">ready to turn in</span>`
    : "";
  const stage = q.stage ? `<div class="quest-stage">${escapeHtml(q.stage)}</div>` : "";
  const hint = q.hint ? `<div class="quest-hint">${escapeHtml(q.hint)}</div>` : "";
  const objs = (q.objectives || []).map(objectiveLine).join("");
  const abandon = q.abandonable
    ? `<div class="quest-actions"><button class="quest-btn" type="button" data-cmd="${escapeHtml(
        q.abandonCmd || ""
      )}">Abandon</button></div>`
    : "";
  return `<div class="quest${q.awaitingTurnIn ? " quest-ready" : ""}">
    <div class="quest-head"><span class="quest-name">${escapeHtml(
      q.name || ""
    )}</span>${cls}${badge}</div>
    ${stage}${hint}
    <ul class="quest-objs">${objs}</ul>
    ${abandon}
  </div>`;
}

function renderQuests(d) {
  const quests = (d && d.quests) || [];
  // Hide the panel entirely for a character with no active quests, so a fresh
  // player's HUD isn't cluttered with an empty Journal section.
  if (!quests.length) {
    el.quests.hidden = true;
    el.questsList.innerHTML = "";
    el.questsCount.textContent = "";
    return;
  }
  el.quests.hidden = false;
  el.questsCount.textContent = `${quests.length} active`;
  el.questsList.innerHTML = quests.map(questCard).join("");
}

// An Abandon button sends its command; cancels any active click-to-walk.
el.quests.addEventListener("click", (e) => {
  const b = e.target.closest(".quest-btn");
  if (b && !b.disabled && b.dataset.cmd && conn.socket) {
    walkTo = null;
    sendCommand(b.dataset.cmd);
  }
});

/* ── Direct-trade form (Char.Trade, P3 Slice B++) ─────────────────
 * The live two-party trade panel, shown only while a trade is open (open=true).
 * Two columns — your staged offer and the partner's — each with items, coin, and
 * a confirmed check that ticks as either side stages value (the surface plain
 * text serves worst). Your items carry a `rescind <item>` command; the whole
 * trade is confirmed/cancelled with the fixed `confirm` / `decline` verbs. A
 * click sends exactly what a player would type (the authority invariant); the
 * server is the sole judge of the swap (both sides must confirm). */

// tradeGood renders one staged item: name, plus (on your side only) a rescind
// button carrying its full command.
function tradeGood(g) {
  const btn = g.cmd
    ? `<button class="trade-rescind" type="button" data-cmd="${escapeHtml(g.cmd)}" title="Remove from your offer">×</button>`
    : "";
  return `<div class="trade-good"><span class="trade-good-name">${escapeHtml(g.name || "")}</span>${btn}</div>`;
}

// tradeSide fills one column: the staged items (or "(nothing)"), an optional coin
// line, and the confirmed check glyph.
function tradeSide(side, listEl, checkEl) {
  const items = (side && side.items) || [];
  const rows = items.map(tradeGood);
  if (side && side.coin) {
    rows.push(`<div class="trade-good trade-coin"><span class="trade-good-name">${escapeHtml(side.coin)}</span></div>`);
  }
  listEl.innerHTML = rows.length ? rows.join("") : `<div class="empty">(nothing offered)</div>`;
  checkEl.textContent = side && side.confirmed ? "✓" : "";
  checkEl.dataset.confirmed = String(!!(side && side.confirmed));
}

function renderTrade(d) {
  // Closed (not trading) → hide the panel entirely.
  if (!d || !d.open) {
    el.trade.hidden = true;
    return;
  }
  el.trade.hidden = false;
  el.tradePartner.textContent = (d.theirs && d.theirs.party) || "";
  el.tradeTheirsName.textContent = (d.theirs && d.theirs.party) || "Partner";
  tradeSide(d.mine, el.tradeMineList, el.tradeMineCheck);
  tradeSide(d.theirs, el.tradeTheirsList, el.tradeTheirsCheck);
}

// Rescind / Confirm / Decline buttons send their command; cancel any walk.
el.trade.addEventListener("click", (e) => {
  const b = e.target.closest("[data-cmd]");
  if (b && !b.disabled && b.dataset.cmd && conn.socket) {
    walkTo = null;
    sendCommand(b.dataset.cmd);
  }
});

/* ── Auction-house form (Char.Auction, P3 Slice B++) ──────────────
 * The contextual marketplace panel, shown only when the player stands at an
 * auctioneer (open=true). The active listings (priced, with a closing-time
 * countdown), each row a Buy button carrying its `buyout <ref>` command — greyed
 * when unaffordable, or shown as "yours" with an `unlist <ref>` for your own. A
 * collect banner appears when items/proceeds wait. Nothing here decides a sale;
 * a click sends the same command a player would type (the authority invariant). */

// auctionRow renders one listing: name, seller, closing time, price, and a
// buy/unlist button (or a greyed buy when unaffordable).
function auctionRow(o) {
  const meta = [];
  if (o.seller) meta.push(`<span class="auction-seller">${escapeHtml(o.seller)}</span>`);
  if (o.closesIn) meta.push(`<span class="auction-time">${escapeHtml(o.closesIn)}</span>`);
  let btn;
  if (o.mine) {
    btn = `<button class="auction-btn auction-unlist" type="button" data-cmd="${escapeHtml(o.cmd || "")}" title="Your listing — withdraw it">unlist</button>`;
  } else if (!o.affordable) {
    btn = `<button class="auction-btn" type="button" disabled title="You can't afford that.">buy</button>`;
  } else {
    btn = `<button class="auction-btn" type="button" data-cmd="${escapeHtml(o.cmd || "")}">buy</button>`;
  }
  return `<div class="auction-item${o.mine ? " auction-mine" : ""}${!o.mine && !o.affordable ? " auction-locked" : ""}">
    <div class="auction-row1"><span class="auction-name">${escapeHtml(o.name || "")}</span><span class="auction-price">${escapeHtml(o.price || "")}</span></div>
    <div class="auction-row2"><span class="auction-meta">${meta.join(" · ")}</span>${btn}</div>
  </div>`;
}

function renderAuction(d) {
  // Closed (not at an auctioneer) → hide the panel entirely.
  if (!d || !d.open) {
    el.auction.hidden = true;
    return;
  }
  el.auction.hidden = false;
  el.auctionMoney.textContent = d.money || "";

  // Collect banner: shown only when items/proceeds wait (collect.cmd present).
  const c = d.collect || {};
  if (c.cmd) {
    const bits = [];
    if (c.items) bits.push(`${c.items} item${c.items > 1 ? "s" : ""}`);
    if (c.coin) bits.push(c.coin);
    el.auctionCollect.hidden = false;
    el.auctionCollect.innerHTML =
      `<button class="auction-btn auction-collect-btn" type="button" data-cmd="${escapeHtml(c.cmd)}">Collect ${escapeHtml(bits.join(" + "))}</button>`;
  } else {
    el.auctionCollect.hidden = true;
    el.auctionCollect.innerHTML = "";
  }

  const listings = d.listings || [];
  el.auctionList.innerHTML = listings.length
    ? listings.map(auctionRow).join("")
    : `<div class="empty">Nothing for sale.</div>`;

  // "showing N of Total" note when there are more listings than this page.
  el.auctionMore.textContent =
    d.total && d.total > listings.length ? `showing ${listings.length} of ${d.total} — use \`browse\`` : "";
}

// A buy / unlist / collect button sends its command; cancels any click-to-walk.
el.auction.addEventListener("click", (e) => {
  const b = e.target.closest(".auction-btn");
  if (b && !b.disabled && b.dataset.cmd && conn.socket) {
    walkTo = null;
    sendCommand(b.dataset.cmd);
  }
});

/* ── Room + neighbourhood map ─────────────────────────────────────
 * Room.Info drives the Location panel and a fallback minimap (visited rooms
 * only, accumulated as you walk). Room.Map (P2) is the rich additive package:
 * the server-computed neighbourhood — including rooms you can SEE but haven't
 * entered — with fog-of-war flags, so the map shows what's ahead. A click
 * walks there: the client paths on the graph and sends move commands (an intent
 * that reduces to existing commands — no new server verb). A baseline client
 * that ignores Room.Map keeps the visited-only fallback. */
const rooms = new Map(); // num → {x,y,z,name,exits}  — Room.Info accumulation (fallback)
let current = null;
let nbhd = null; // {center, nodes: Map(num → {num,x,y,z,name,exits,visited})} — Room.Map
let mapHits = []; // [{num, sx, sy}] from the last draw, for click hit-testing
let walkTo = null; // click-to-walk target room id

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
    rooms.set(d.num, { x: d.x, y: d.y, z: d.z ?? 0, name: d.name, exits });
    current = d.num;
  }
  drawMinimap();
}

// Room.Map (P2): the local neighbourhood graph. Re-centres each transition;
// advances an in-progress click-to-walk toward its target.
function renderRoomMap(d) {
  const nodes = new Map();
  for (const n of d.rooms || []) nodes.set(n.num, n);
  nbhd = { center: d.center, nodes };
  current = d.center;
  advanceWalk();
  drawMinimap();
}

// Clicking an exit sends the matching move command; cancels any active walk.
el.roomExits.addEventListener("click", (e) => {
  const b = e.target.closest(".exit-btn");
  if (b && conn.socket) {
    walkTo = null;
    sendCommand(b.dataset.dir);
  }
});

// Click a map node → walk there. The path is computed client-side (a view
// concern, like tab-completion); every STEP is a move command the server
// validates independently, so a locked door simply stops the walk.
el.minimap.addEventListener("click", (e) => {
  if (!conn.socket || !mapHits.length) return;
  const rect = el.minimap.getBoundingClientRect();
  const cx = ((e.clientX - rect.left) / rect.width) * el.minimap.width;
  const cy = ((e.clientY - rect.top) / rect.height) * el.minimap.height;
  let best = null;
  let bestD = 20 * 20; // hit radius²
  for (const h of mapHits) {
    const dx = h.sx - cx, dy = h.sy - cy, dist = dx * dx + dy * dy;
    if (dist < bestD) { bestD = dist; best = h.num; }
  }
  if (best && best !== current) {
    walkTo = best;
    advanceWalk();
  }
});

// advanceWalk sends the next step toward walkTo, re-pathing from wherever we
// actually are (each successful move produces a fresh Room.Map that re-invokes
// this). A blocked move produces no Room.Map, so the walk simply pauses.
function advanceWalk() {
  if (!walkTo || !nbhd) return;
  if (walkTo === nbhd.center) { walkTo = null; return; } // arrived
  const dir = firstStep(nbhd, nbhd.center, walkTo);
  if (!dir) { walkTo = null; return; } // no longer reachable within the map window
  sendCommand(dir);
}

// firstStep BFS-es the neighbourhood graph and returns the short direction of
// the first move on a shortest path from → to, or null if unreachable in-window.
function firstStep(nb, from, to) {
  const seen = new Set([from]);
  const queue = [[from, null]];
  while (queue.length) {
    const [num, firstDir] = queue.shift();
    const node = nb.nodes.get(num);
    if (!node || !node.exits) continue;
    for (const dir of Object.keys(node.exits)) {
      const target = node.exits[dir];
      if (seen.has(target)) continue;
      const step = firstDir || dir;
      if (target === to) return step;
      if (nb.nodes.has(target)) { seen.add(target); queue.push([target, step]); }
    }
  }
  return null;
}

function drawMinimap() {
  const cvs = el.minimap;
  const ctx = cvs.getContext("2d");
  const W = cvs.width, H = cvs.height;
  ctx.clearRect(0, 0, W, H);
  mapHits = [];
  if (!current) return;

  // Prefer the rich Room.Map neighbourhood (has fog flags + unvisited rooms);
  // fall back to the visited-only Room.Info accumulation.
  let source, hasFog;
  if (nbhd && nbhd.nodes.has(current)) {
    source = [...nbhd.nodes.values()];
    hasFog = true;
  } else {
    source = [...rooms.entries()].map(([num, r]) => ({ num, ...r, visited: true }));
    hasFog = false;
  }
  const cz = (nbhd && nbhd.nodes.get(current)) ? nbhd.nodes.get(current).z : (rooms.get(current) || {}).z ?? 0;

  const plane = source.filter((r) => (r.z ?? 0) === cz && r.x != null && r.y != null);
  if (!plane.length) return;
  const byNum = new Map(plane.map((r) => [r.num, r]));

  const xs = plane.map((r) => r.x), ys = plane.map((r) => r.y);
  const minX = Math.min(...xs), maxX = Math.max(...xs);
  const minY = Math.min(...ys), maxY = Math.max(...ys);
  const pad = 24;
  const spanX = Math.max(1, maxX - minX), spanY = Math.max(1, maxY - minY);
  const stepPx = Math.min((W - pad * 2) / spanX, (H - pad * 2) / spanY, 34) || 20;
  const ox = W / 2 - ((minX + maxX) / 2) * stepPx;
  const oy = H / 2 + ((minY + maxY) / 2) * stepPx; // y grows up → invert
  const px = (r) => ox + r.x * stepPx;
  const py = (r) => oy - r.y * stepPx;

  const cssVar = (n, fb) => getComputedStyle(document.documentElement).getPropertyValue(n).trim() || fb;
  const accent = cssVar("--accent", "#4fd");
  const dim = cssVar("--line-strong", "#556");
  const text = cssVar("--text-faint", "#889");

  // Edges (both endpoints on this plane).
  ctx.strokeStyle = dim;
  ctx.lineWidth = 1.5;
  for (const r of plane) {
    for (const dir of Object.keys(r.exits || {})) {
      const t = byNum.get(r.exits[dir]);
      if (t) {
        ctx.beginPath();
        ctx.moveTo(px(r), py(r));
        ctx.lineTo(px(t), py(t));
        ctx.stroke();
      }
    }
  }
  // Nodes: current highlighted; visited solid; unvisited (fog) hollow.
  for (const r of plane) {
    const sx = px(r), sy = py(r);
    mapHits.push({ num: r.num, sx, sy });
    const isCur = r.num === current;
    const visited = r.visited !== false;
    if (isCur) {
      ctx.fillStyle = accent;
      ctx.beginPath();
      ctx.arc(sx, sy, 5.5, 0, Math.PI * 2);
      ctx.fill();
      ctx.strokeStyle = accent;
      ctx.globalAlpha = 0.35;
      ctx.beginPath();
      ctx.arc(sx, sy, 10, 0, Math.PI * 2);
      ctx.stroke();
      ctx.globalAlpha = 1;
    } else if (visited || !hasFog) {
      ctx.fillStyle = dim;
      ctx.beginPath();
      ctx.arc(sx, sy, 3.5, 0, Math.PI * 2);
      ctx.fill();
    } else {
      // Fog: seen on the map but not yet entered — hollow.
      ctx.strokeStyle = text;
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.arc(sx, sy, 3.5, 0, Math.PI * 2);
      ctx.stroke();
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
    closeComplete();
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

function sendGmcp(pkg, data) {
  if (!conn.socket || conn.socket.readyState !== WebSocket.OPEN) return;
  conn.socket.send(JSON.stringify({ type: "gmcp", package: pkg, data }));
}

/* ── Autocomplete (Input.Complete GMCP, tab-completion §12/§13) ──
 * The server owns completion; this client only *requests* the candidate set
 * for the token under the caret and renders it. Requests are debounced so we
 * stay under the server's per-connection inbound-GMCP rate limit. Keyboard:
 * when the dropdown is open, ↑/↓ navigate, Tab completes (common prefix first,
 * then the selected/first candidate), Esc closes, and Enter accepts only if the
 * user explicitly navigated — otherwise Enter submits the command as normal, so
 * the command line never gets slower. */
const comp = { open: false, items: [], sel: -1, common: "", reqLine: "", timer: 0 };
const COMPLETE_DEBOUNCE_MS = 120;

// The command line up to the caret — exactly what the server completes (§13).
function lineToCursor() {
  return el.cmd.value.slice(0, el.cmd.selectionStart ?? el.cmd.value.length);
}
// Start index of the whitespace-delimited token under the caret (mirrors the
// server's lastToken semantics), so an accepted value replaces that token.
function tokenStart(line) {
  const i = line.lastIndexOf(" ");
  return i < 0 ? 0 : i + 1;
}

function requestComplete(immediate) {
  clearTimeout(comp.timer);
  if (!conn.socket || el.cmd.type === "password") return closeComplete();
  const line = lineToCursor();
  if (!line.trim()) return closeComplete();
  comp.reqLine = line;
  const fire = () => sendGmcp("Input.Complete", { line: comp.reqLine });
  if (immediate) fire();
  else comp.timer = setTimeout(fire, COMPLETE_DEBOUNCE_MS);
}

function onCompleteList(d) {
  // Drop a stale/late reply: it must match the last request AND still match
  // what's under the caret right now — so a reply that lands after the dropdown
  // was dismissed (Esc / submit / blur) or after the caret moved can't re-open
  // or mis-populate the list.
  if (!d || d.line !== comp.reqLine || d.line !== lineToCursor()) return;
  const items = Array.isArray(d.candidates) ? d.candidates : [];
  if (!items.length) return closeComplete();
  comp.items = items;
  comp.common = d.common || "";
  comp.sel = -1;
  comp.open = true;
  renderComplete();
}
gmcpHandlers["Input.Complete.List"] = onCompleteList;

function renderComplete() {
  el.completeList.innerHTML = comp.items
    .map((c, i) => {
      const sel = i === comp.sel;
      const kind = c.kind
        ? `<span class="complete-kind">${escapeHtml(c.kind)}</span>`
        : "";
      const disp =
        c.display && c.display !== c.value
          ? `<span class="complete-disp">${escapeHtml(c.display)}</span>`
          : "";
      return `<li class="complete-item${sel ? " is-sel" : ""}" role="option" aria-selected="${sel}" data-val="${escapeHtml(c.value)}"><span class="complete-val">${escapeHtml(c.value)}</span>${disp}${kind}</li>`;
    })
    .join("");
  el.completeList.hidden = false;
  el.cmd.setAttribute("aria-expanded", "true");
}

function closeComplete() {
  clearTimeout(comp.timer);
  comp.open = false;
  comp.items = [];
  comp.sel = -1;
  comp.common = "";
  comp.reqLine = ""; // so an in-flight reply can't re-open a dismissed dropdown
  el.completeList.hidden = true;
  el.completeList.innerHTML = "";
  el.cmd.setAttribute("aria-expanded", "false");
}

function moveSel(delta) {
  const n = comp.items.length;
  if (!n) return;
  comp.sel = comp.sel < 0 ? (delta > 0 ? 0 : n - 1) : (comp.sel + delta + n) % n;
  renderComplete();
  const li = el.completeList.children[comp.sel];
  if (li) li.scrollIntoView({ block: "nearest" });
}

// Replace the caret's token with `value`; addSpace appends a trailing space so
// the next argument can be typed straight away (shell-style on a full accept).
function insertToken(value, addSpace) {
  const full = el.cmd.value;
  const caret = el.cmd.selectionStart ?? full.length;
  const head = full.slice(0, caret);
  const tail = full.slice(caret);
  const start = tokenStart(head);
  // Consume the rest of the word after the caret too, so completing mid-word
  // (`get sw|ord` → accept `sword`) yields `get sword `, not `get sword ord`.
  const tailWord = (tail.match(/^\S*/) || [""])[0];
  const insert = value + (addSpace ? " " : "");
  el.cmd.value = head.slice(0, start) + insert + tail.slice(tailWord.length);
  const pos = start + insert.length;
  el.cmd.setSelectionRange(pos, pos);
}

function acceptCandidate(idx) {
  const c = comp.items[idx >= 0 ? idx : 0];
  if (!c) return;
  insertToken(c.value, true);
  closeComplete();
}

// Tab: if the longest-common-prefix extends the typed token and the user hasn't
// picked a specific item, complete to the common prefix first (§12); otherwise
// accept the selected (or first) candidate.
function tabComplete() {
  const head = lineToCursor();
  const token = head.slice(tokenStart(head));
  if (
    comp.sel < 0 &&
    comp.common.length > token.length &&
    comp.common.startsWith(token)
  ) {
    insertToken(comp.common, false);
    requestComplete(true); // refine the list against the extended token
    return;
  }
  acceptCandidate(comp.sel);
}

el.cmd.addEventListener("input", () => requestComplete(false));

// mousedown (not click) so the accept runs before the input's blur fires.
el.completeList.addEventListener("mousedown", (e) => {
  const li = e.target.closest(".complete-item");
  if (!li) return;
  e.preventDefault();
  insertToken(li.dataset.val, true);
  closeComplete();
  el.cmd.focus();
});

// Close on blur, but after a beat so a candidate mousedown lands first.
el.cmd.addEventListener("blur", () => setTimeout(closeComplete, 120));

// A click in the input repositions the caret to a possibly-different token, so
// the open list is stale — dismiss it (typing will re-request).
el.cmd.addEventListener("click", () => {
  if (comp.open) closeComplete();
});

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
  walkTo = null; // a manual command cancels an in-progress click-to-walk
  closeComplete();
  sendCommand(text);
  if (text.trim() && el.cmd.type !== "password") {
    history.push(text);
    if (history.length > 200) history.shift();
  }
  histIdx = history.length;
  el.cmd.value = "";
});

el.cmd.addEventListener("keydown", (e) => {
  // Autocomplete steers the keys while the dropdown is open.
  if (comp.open) {
    if (e.key === "ArrowDown") return e.preventDefault(), moveSel(1);
    if (e.key === "ArrowUp") return e.preventDefault(), moveSel(-1);
    if (e.key === "Tab") return e.preventDefault(), tabComplete();
    if (e.key === "Escape") return e.preventDefault(), closeComplete();
    if (e.key === "Enter" && comp.sel >= 0)
      return e.preventDefault(), acceptCandidate(comp.sel);
    // Caret-moving keys change which token is under the caret — the current
    // candidate list is now stale, so dismiss it (let the caret move normally).
    if (["ArrowLeft", "ArrowRight", "Home", "End"].includes(e.key)) closeComplete();
    // Enter with no explicit selection, or typing, falls through.
  } else if (e.key === "Tab" && el.cmd.type !== "password" && lineToCursor().trim()) {
    // Closed dropdown: an explicit Tab requests completion immediately.
    // (Suppressed in password prompts so Tab isn't a focus trap.)
    e.preventDefault();
    requestComplete(true);
    return;
  }

  // Command history (arrows only reach here when the dropdown isn't open).
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
