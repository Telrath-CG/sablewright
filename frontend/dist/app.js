/* ==========================================================================
   Sablewright - frontend
   Plain JavaScript, no framework and no build step. Go methods are reachable
   as window.go.main.App.* and each returns a Promise.
   ========================================================================== */

const App = () => window.go.main.App;

const STATUSES = ["Backlog", "Assembled", "Primed", "In Progress", "Complete", "Display"];
const STATUS_COLORS = {
  "Backlog": "#9aa0a6", "Assembled": "#64748b", "Primed": "#8b5cf6",
  "In Progress": "#f59e0b", "Complete": "#22a06b", "Display": "#2563eb",
};
const PAINT_TYPES = ["Base", "Layer", "Shade", "Contrast", "Dry", "Technical",
                     "Air", "Primer", "Metallic", "Wash", "Glaze", "Ink", "Other"];
// The rack ships with well over a thousand paints, and every keystroke in the
// search box rebuilds the list. Past a few hundred rows that starts to drag,
// and nobody reads row 900 anyway - so cap what's drawn and say so.
const ROW_LIMIT = 300;
const PICKER_LIMIT = 200;
const TIP_CATEGORIES = ["Priming", "Basecoating", "Highlighting", "Shading", "Blending",
                        "Metallics", "Effects", "Basing", "Varnishing", "Tools", "Other"];
const TIP_COLORS = {
  "Priming": "#8b5cf6", "Basecoating": "#0ea5e9", "Highlighting": "#f59e0b",
  "Shading": "#6366f1", "Blending": "#2563eb", "Metallics": "#c08a2d",
  "Effects": "#22a06b", "Basing": "#a16207", "Varnishing": "#0891b2",
  "Tools": "#64748b", "Other": "#6b7280",
};

// The models list can be reordered from the picker or by clicking a column
// header. Each ordering runs one way naturally - names climb A→Z, everything
// else reads from the top down - and asc records which, so the header arrow
// can point at the values rather than at the reversal flag.
const MODEL_SORTS = ["Status", "Name", "Paints", "Recent", "Favourites"];
const SORT_ASC = { Status: true, Name: true, Paints: false, Recent: false, Favourites: false };
// The three columns the list draws, in order, and the sort each one clicks to.
const MODEL_COLUMNS = [["Name", "NAME", ""], ["Status", "STATUS", ""], ["Paints", "PAINTS", "r"]];

const MAGNIFIER =
  '<svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">' +
  '<circle cx="7" cy="7" r="5" fill="none" stroke="#98a2ae" stroke-width="2"/>' +
  '<path d="M11 11l4 4" stroke="#98a2ae" stroke-width="2" stroke-linecap="round"/></svg>';

const state = {
  screen: "dashboard",
  models: { search: "", status: "All", sort: "Status", desc: false, selected: null },
  paints: { search: "", type: "All types", brand: "All brands",
            range: "All ranges", stock: "All" },
  tips:   { search: "", category: "All" },
};

/* ------------------------------------------------------------------ utils */
const $ = (sel, root = document) => root.querySelector(sel);
const content = () => $("#content");

// A screen or a modal re-renders by replacing a whole subtree, which throws
// away the very node the user is typing into. The search boxes re-render
// mid-word off a debounce, so without this the caret drops back to <body>
// after the first pause and the rest of what you type goes nowhere.
//
// Only a field inside the subtree about to be replaced is worth restoring:
// a modal opening over the models list must not yank focus back down to the
// search box behind it, which is still perfectly focused where it is.
function captureFocus(root) {
  const el = document.activeElement;
  if (!el || !el.id || !root.contains(el)) return null;
  const snap = { id: el.id, sel: null };
  try {
    if (typeof el.selectionStart === "number") {
      snap.sel = { value: el.value, start: el.selectionStart, end: el.selectionEnd };
    }
  } catch (_) { /* input types that don't support selection throw on read */ }
  return snap;
}

// The live value wins over the rendered one. The markup is built from filter
// state read *before* an await on the backend, so anything typed during that
// await is newer than the HTML and would otherwise be silently overwritten.
// The pending debounce still closes over the old, now-detached input, and
// reading .value off a detached node works, so the follow-up render
// reconciles the list with whatever ended up in the box.
function restoreFocus(snap) {
  if (!snap) return;
  const next = document.getElementById(snap.id);
  if (!next) return;
  next.focus();
  const sel = snap.sel;
  if (!sel) return;
  if (next.value !== sel.value) next.value = sel.value;
  try { next.setSelectionRange(sel.start, sel.end); } catch (_) { /* ditto */ }
}

function setContent(html) {
  const snap = captureFocus(content());
  content().innerHTML = html;
  restoreFocus(snap);
}

function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g,
    c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

function prettyDate(iso) {
  if (!iso) return "—";
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  if (!m) return iso;
  const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun",
                  "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  return `${+m[3]} ${months[+m[2] - 1]} ${m[1]}`;
}

function plural(n, one, many) { return `${n} ${n === 1 ? one : (many || one + "s")}`; }

// Counts arrive from text inputs rather than number spinners, so that a
// half-typed or pasted value is never silently mangled on the way in.
function intOf(value, fallback) {
  const n = parseInt(String(value == null ? "" : value).replace(/[^0-9]/g, ""), 10);
  return isNaN(n) ? fallback : n;
}

// A squad is one row, so the row has to show how far through it is. Nothing
// is drawn for a single mini: a 0-of-1 bar beside a status badge already
// saying as much is noise.
function progressBar(done, total) {
  if (!(total > 1)) return "";
  return `<div class="progress" title="${done} of ${total} painted">
    <div class="fill" style="width:${Math.round((done / total) * 100)}%"></div>
    <span>${done}/${total}</span>
  </div>`;
}

// Tiles draw the generated thumbnail; clicking one opens the original. A
// photo in a format the decoder couldn't read has no thumbnail and falls
// back to the full image, which is heavy but never broken.
function photoSrc(p) {
  return `/photos/${encodeURIComponent(p.thumb || p.file)}`;
}

// The shot that stands for a mini in the list: an explicit choice first, then
// the newest final photo, then the newest progress one. Mirrors CoverPhoto in
// the store, which is what every other reader of this goes through.
function coverPhoto(m) {
  const photos = m.photos || [];
  return photos.find(p => p.cover)
    || [...photos].reverse().find(p => p.kind === "Final")
    || photos[photos.length - 1]
    || null;
}

// The collection is measured in minis, and the entries are how they're filed.
// The two only need saying apart once a batch makes them differ.
function miniCount(models) {
  return models.reduce((n, m) => n + (m.count || 1), 0);
}

// 95 -> "1h 35m", 60 -> "1h", 0 -> ""
function duration(mins) {
  if (!mins) return "";
  const h = Math.floor(mins / 60), m = mins % 60;
  return (h ? `${h}h` : "") + (h && m ? " " : "") + (m ? `${m}m` : "");
}

function todayISO() { return new Date().toISOString().slice(0, 10); }

let toastTimer = null;
function toast(msg) {
  const t = $("#toast");
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, 2600);
}

// Every backend call goes through here so a failure surfaces as a message
// rather than a silently dead button.
async function call(fn, ...args) {
  try {
    return await fn(...args);
  } catch (err) {
    const msg = (err && err.message) ? err.message : String(err);
    toast(msg.charAt(0).toUpperCase() + msg.slice(1));
    throw err;
  }
}

function badge(status) {
  return `<span class="badge" style="background:${STATUS_COLORS[status] || "#6b7280"}">${esc(status)}</span>`;
}

function searchBox(id, placeholder, value) {
  return `<div class="search">${MAGNIFIER}
    <input type="text" id="${id}" placeholder="${esc(placeholder)}" value="${esc(value)}" spellcheck="false">
  </div>`;
}

// prefix labels the choices without becoming part of them: "Sort: Status" in
// a bar that already holds a Status filter, while the value stays "Status".
function selectBox(id, options, value, prefix = "") {
  return `<select id="${id}">` +
    options.map(o => `<option value="${esc(o)}"${o === value ? " selected" : ""}>${
      esc(prefix + o)}</option>`).join("") +
    `</select>`;
}

/* ---------------------------------------------------------------- theme */
// An entry in storage is an explicit choice and wins; with nothing stored the
// app follows the OS. index.html applies the same logic inline before first
// paint - this half only handles switching and keeps the button label honest.
const THEME_KEY = "sablewright.theme";

function storedTheme() {
  try {
    const t = localStorage.getItem(THEME_KEY);
    return t === "light" || t === "dark" ? t : null;
  } catch (e) {
    return null;
  }
}

function systemTheme() {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function currentTheme() { return storedTheme() || systemTheme(); }

function applyTheme(theme) {
  document.documentElement.setAttribute("data-theme", theme);
  // The label names what the click will do, not what is currently on.
  const label = $("#btn-theme span");
  if (label) label.textContent = theme === "dark" ? "Light mode" : "Dark mode";
}

function initTheme() {
  applyTheme(currentTheme());

  $("#btn-theme").onclick = () => {
    const next = currentTheme() === "dark" ? "light" : "dark";
    try { localStorage.setItem(THEME_KEY, next); } catch (e) { /* session only */ }
    applyTheme(next);
  };

  // Track the OS while the user has no explicit preference of their own.
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
    if (!storedTheme()) applyTheme(systemTheme());
  });
}

/* ---------------------------------------------------------------- timer */
// One desk, one painter - so one timer, and it lives in memory only. It
// outlasts the dialog it was started from and every screen change, but not
// the app itself: nothing is written to disk, and a running timer reaches the
// store only when you stop it and save the log entry it fills in.
const timer = {
  modelId: 0,     // 0 = nothing running
  modelName: "",
  since: 0,       // ms epoch this running stretch began; 0 while paused
  banked: 0,      // ms from stretches already closed off by a pause
  tick: null,
};

function timerOn() { return timer.modelId !== 0; }
function timerPaused() { return timerOn() && !timer.since; }

function timerElapsed() {
  return timer.banked + (timer.since ? Date.now() - timer.since : 0);
}

// The log stores whole minutes, and 47:40 at the desk is a 48 minute session.
function timerMinutes() { return Math.round(timerElapsed() / 60000); }

// m:ss while it's short, h:mm:ss once past the hour. duration() is the other
// half of this - it formats minutes already banked in the log, this formats a
// clock still running, so it needs the seconds ticking over to look alive.
function clock(ms) {
  const t = Math.max(0, Math.floor(ms / 1000));
  const pad = n => String(n).padStart(2, "0");
  const h = Math.floor(t / 3600), m = Math.floor(t / 60) % 60;
  return (h ? `${h}:${pad(m)}` : `${m}`) + `:${pad(t % 60)}`;
}

// Every clock face on screen updates off the one interval, and the tick only
// writes a text node - no re-render, so it can't fight the caret or rebuild a
// list under the user. Whatever is drawn fresh reads the time on its way past.
function paintClocks() {
  const face = clock(timerElapsed());
  document.querySelectorAll(".timer-clock").forEach(el => { el.textContent = face; });
}

// The mini detail pane mirrors the timer next to its Edit button, so it goes
// stale the moment anything else changes that state - and a stale "Start
// timer" would restart a running count and throw the elapsed time away.
// Skipped while a dialog is up, since the log tab has its own controls and
// closeModal catches the pane up on the way out.
function syncDetailTimer() {
  if (state.screen !== "models" || !state.models.selected) return;
  if (!$("#modal-backdrop").hidden) return;
  renderModelDetail(state.models.selected);
}

function drawTimer() {
  const el = $("#timer");
  syncDetailTimer();
  if (!timerOn()) { el.hidden = true; el.innerHTML = ""; return; }
  const paused = timerPaused();
  el.hidden = false;
  // Three stacked rows rather than one: 200px of sidebar can't hold a name,
  // a clock and two controls side by side, and spelling the buttons out beats
  // hunting for pause and stop glyphs the webview's fonts might not carry.
  el.innerHTML = `
    <div class="who"><span class="dot${paused ? " off" : ""}"></span>
      <span class="nm">${esc(timer.modelName)}</span></div>
    <div class="timer-clock">${clock(timerElapsed())}</div>
    <div class="row">
      <button id="tm-toggle">${paused ? "Resume" : "Pause"}</button>
      <button id="tm-stop" title="Stop and fill in the log entry">Stop</button>
    </div>`;
  $("#tm-toggle").onclick = () => (timerPaused() ? resumeTimer() : pauseTimer());
  $("#tm-stop").onclick = stopTimer;
}

function startTimer(id, name) {
  timer.modelId = id;
  timer.modelName = name;
  timer.banked = 0;
  timer.since = Date.now();
  clearInterval(timer.tick);
  timer.tick = setInterval(paintClocks, 1000);
  drawTimer();
}

function pauseTimer() {
  if (!timerOn() || timerPaused()) return;
  timer.banked += Date.now() - timer.since;
  timer.since = 0;
  drawTimer();
}

function resumeTimer() {
  if (!timerOn() || !timerPaused()) return;
  timer.since = Date.now();
  drawTimer();
}

function clearTimer() {
  clearInterval(timer.tick);
  timer.tick = null;
  timer.modelId = 0; timer.modelName = ""; timer.since = 0; timer.banked = 0;
  drawTimer();
}

// Stopping writes nothing by itself. The store turns away a session with no
// notes, so this hands the minutes to the log form and leaves you to say what
// you actually got done - the entry isn't real until you save it.
async function stopTimer() {
  const id = timer.modelId, mins = timerMinutes();
  clearTimer();
  const m = await call(App().GetModel, id);
  if (!m) { toast("That mini is gone — nothing logged"); return; }
  modelDialog(m, { tab: "log", session: { minutes: mins } });
}

/* ------------------------------------------------------------------ nav */
function initNav() {
  $("#nav").addEventListener("click", e => {
    const li = e.target.closest("li");
    if (!li) return;
    document.querySelectorAll("#nav li").forEach(n => n.classList.toggle("active", n === li));
    show(li.dataset.screen);
  });

  $("#btn-backup").onclick = async () => {
    const path = await call(App().Backup);
    if (path) toast("Backup saved");
  };
  $("#btn-restore").onclick = async () => {
    const ok = await call(App().Restore);
    // The whole collection has just been swapped out from under it, so a
    // running timer is pointing at a mini from the previous dataset.
    if (ok) { clearTimer(); toast("Backup imported"); show(state.screen); }
  };
  $("#btn-folder").onclick = () => call(App().OpenDataFolder);
}

function show(screen) {
  state.screen = screen;
  ({ dashboard: renderDashboard, models: renderModels,
     paints: renderPaints, tips: renderTips })[screen]();
}

/* ================================================================ DASHBOARD */
async function renderDashboard() {
  const s = await call(App().GetStats);
  const max = Math.max(1, ...Object.values(s.byStatus));

  const cards = [
    [s.models, "Minis tracked", "#2f7d8a"],
    [s.inProgress, "In progress", STATUS_COLORS["In Progress"]],
    [s.finished, "Finished", STATUS_COLORS["Complete"]],
    [s.sessions, s.minutes ? `Sessions · ${duration(s.minutes)}` : "Sessions logged", "#0ea5e9"],
    [s.paintsOwned, "Paints owned", "#8b5cf6"],
    [s.paintsWishlist, "On wishlist", "#c07b1f"],
  ].map(([n, l, c]) => `
    <div class="card stat">
      <div class="bar" style="background:${c}"></div>
      <div><div class="n">${n}</div><div class="l">${l}</div></div>
    </div>`).join("");

  const bars = STATUSES.map(st => {
    const v = s.byStatus[st] || 0;
    const h = v ? Math.max(6, Math.round((v / max) * 100)) : 2;
    return `<div class="col">
      <div class="val" style="color:${v ? "var(--text)" : "var(--faint)"}">${v}</div>
      <div class="bar" style="height:${h}%;background:${v ? STATUS_COLORS[st] : "#e3e8ee"}"></div>
      <div class="lbl">${st}</div>
    </div>`;
  }).join("");

  const backlog = s.backlog.length
    ? s.backlog.map(m => `<div class="mini-row"><span class="name">${esc(m.name)}</span>${badge(m.status)}</div>`).join("")
    : `<div class="empty">Nothing waiting — nice.</div>`;

  const logs = s.recentLogs.length
    ? s.recentLogs.map(l => `<div class="mini-row log-jump" data-model="${l.modelId}" style="cursor:pointer">
        <span class="date" style="min-width:92px">${prettyDate(l.date)}</span>
        <span class="name"><strong>${esc(l.modelName)}</strong> — ${esc(l.notes)}</span>
        ${l.minutes ? `<span class="date">${duration(l.minutes)}</span>` : ""}
      </div>`).join("")
    : `<div class="empty">No sessions logged yet. Open a mini and use the
         Painting log tab to record what you did at the desk.</div>`;

  const recent = s.recent.length
    ? s.recent.map(m => `<div class="mini-row">
        <span class="dot" style="background:${STATUS_COLORS[m.status]}"></span>
        <span class="name">${esc(m.name)}</span>
        <span class="date">${m.completed ? prettyDate(m.completed) : ""}</span></div>`).join("")
    : `<div class="empty">No finished minis yet — they'll show up here.</div>`;

  setContent(`
    <div class="page-head"><div>
      <h1>Dashboard</h1><div class="sub">At a glance — your painting progress</div>
    </div></div>
    <div class="stats">${cards}</div>
    <div class="card" style="margin-top:16px">
      <h2>Collection by status</h2>
      <div class="chart">${bars}</div>
    </div>
    <div class="two-col">
      <div class="card"><h2>Backlog / to start</h2><div class="divider"></div>
        <div style="padding:6px 0 10px">${backlog}</div></div>
      <div class="card"><h2>Recently finished</h2><div class="divider"></div>
        <div style="padding:6px 0 10px">${recent}</div></div>
    </div>
    <div class="card" style="margin-top:16px">
      <h2>Latest painting sessions</h2><div class="divider"></div>
      <div style="padding:6px 0 10px">${logs}</div>
    </div>`);

  content().querySelectorAll(".log-jump").forEach(el => {
    el.onclick = () => {
      state.models.selected = +el.dataset.model;
      document.querySelectorAll("#nav li").forEach(n =>
        n.classList.toggle("active", n.dataset.screen === "models"));
      show("models");
    };
  });
}

/* =================================================================== MODELS */
async function renderModels() {
  const f = state.models;
  const models = await call(App().ListModels,
    { search: f.search, status: f.status, sort: f.sort, desc: f.desc });

  if (models.length && !models.some(m => m.id === f.selected)) f.selected = models[0].id;
  if (!models.length) f.selected = null;

  // The cover keeps its square whether or not there is a photo in it, so the
  // names stay in one column instead of stepping in and out.
  const rows = models.length ? models.map(m => {
    const cover = coverPhoto(m);
    return `
    <div class="row${m.id === f.selected ? " sel" : ""}" data-id="${m.id}">
      <div class="who">
        ${cover ? `<img class="cover" src="${photoSrc(cover)}" alt="" loading="lazy">`
                : `<span class="cover blank">▤</span>`}
        <div class="txt">
          <div class="nm">${esc(m.name)}${
            m.count > 1 ? `<span class="qty">×${m.count}</span>` : ""}</div>
          <div class="sub">${esc([m.gameSystem, m.faction].filter(Boolean).join(" · ") || "—")}</div>
          ${progressBar(m.done, m.count)}
        </div>
      </div>
      <div>${badge(m.status)}</div>
      <div class="count">${(m.paintIds || []).length}${m.favorite ? ' <span class="star">★</span>' : ""}</div>
    </div>`;
  }).join("")
    : `<div class="empty"><strong>No minis yet.</strong>Click “+ Add Mini” to start your collection.</div>`;

  // Entries and minis are the same number until a batch splits them, and the
  // heading only spends words on the difference once there is one.
  const minis = miniCount(models);
  const tally = minis === models.length ? plural(minis, "mini")
    : `${plural(minis, "mini")} in ${plural(models.length, "entry", "entries")}`;

  // The arrow follows the values, not the flag: A→Z and in-progress-first
  // both point up, while "most paints first" points down unreversed.
  const head = MODEL_COLUMNS.map(([key, label, cls]) => {
    const on = f.sort === key;
    const up = SORT_ASC[key] !== f.desc;
    const arrow = on ? `<span class="arrow">${up ? "▲" : "▼"}</span>` : "";
    return `<div class="sortable${cls ? " " + cls : ""}${on ? " on" : ""}"
      data-sort="${key}" title="Sort by ${label.toLowerCase()}">${label}${arrow}</div>`;
  }).join("");

  setContent(`
    <div class="page-head">
      <div><h1>Models</h1><div class="sub">Your collection — ${tally}</div></div>
      <div class="spacer"></div>
      <button class="btn" id="add-model">+&nbsp; Add Mini</button>
    </div>
    <div class="filters">
      ${searchBox("m-search", "Search minis…", f.search)}
      ${selectBox("m-status", ["All", ...STATUSES], f.status)}
      ${selectBox("m-sort", MODEL_SORTS, f.sort, "Sort: ")}
    </div>
    <div class="split">
      <div class="card list-pane">
        <div class="list-head" id="m-head">${head}</div>
        <div class="divider"></div>
        <div class="rows" id="m-rows">${rows}</div>
      </div>
      <div class="card detail-pane" id="m-detail"></div>
    </div>`);

  const search = $("#m-search");
  search.oninput = debounce(() => { f.search = search.value; renderModels(); }, 180);
  $("#m-status").onchange = e => { f.status = e.target.value; renderModels(); };
  // Picking an ordering fresh starts it the way round it's meant to be read.
  $("#m-sort").onchange = e => { f.sort = e.target.value; f.desc = false; renderModels(); };
  $("#add-model").onclick = () => modelDialog(null);

  // Clicking the column already sorted reverses it; clicking a different one
  // moves to it in its own natural direction rather than inheriting the last
  // column's, so a reversed name sort doesn't hand you the least-painted mini.
  $("#m-head").addEventListener("click", e => {
    const col = e.target.closest("[data-sort]");
    if (!col) return;
    if (f.sort === col.dataset.sort) f.desc = !f.desc;
    else { f.sort = col.dataset.sort; f.desc = false; }
    renderModels();
  });

  $("#m-rows").addEventListener("click", e => {
    const row = e.target.closest(".row");
    if (!row) return;
    f.selected = +row.dataset.id;
    renderModels();
  });
  $("#m-rows").addEventListener("dblclick", e => {
    const row = e.target.closest(".row");
    if (row) openModel(+row.dataset.id);
  });

  renderModelDetail(f.selected);
}

async function openModel(id) {
  const m = await call(App().GetModel, id);
  if (m) modelDialog(m);
}

async function renderModelDetail(id) {
  const pane = $("#m-detail");
  if (!pane) return;
  if (!id) {
    pane.innerHTML = `<div class="empty">Select a mini to see its details.</div>`;
    return;
  }
  const [m, allPaints] = await Promise.all([call(App().GetModel, id), call(App().AllPaints)]);
  if (!m) { pane.innerHTML = `<div class="empty">Select a mini to see its details.</div>`; return; }

  const byId = new Map((allPaints || []).map(p => [p.id, p]));
  const paints = (m.paintIds || []).map(pid => byId.get(pid)).filter(Boolean);

  const photos = (m.photos || []).length ? m.photos.map(p => `
    <div class="photo">
      <img src="${photoSrc(p)}" alt="" data-file="${esc(p.file)}" loading="lazy">
      <div class="cap"><span class="badge" style="background:${
        p.kind === "Final" ? STATUS_COLORS["Complete"] : "#64748b"};font-size:10px">${esc(p.kind)}</span>${
        p.cover ? `<span class="is-cover" title="Cover shot">★</span>` : ""}</div>
    </div>`).join("")
    : `<div style="color:var(--muted);font-size:13px">No photos yet — add progress shots when you edit this mini.</div>`;

  const chips = paints.length ? `<div class="chips">${paints.map(p => `
      <span class="chip"><span class="swatch" style="background:${esc(p.hex)}"></span>${esc(p.name)}</span>`).join("")}</div>`
    : `<div style="color:var(--muted);font-size:13px">None recorded yet.</div>`;

  const notes = (m.notes || "").trim();
  const noteHtml = notes
    ? `<div class="notes">${notes.split("\n").filter(l => l.trim())
        .map(l => `<p>${esc(l.trim())}</p>`).join("")}</div>`
    : `<div style="color:var(--muted);font-size:13px">No notes yet.</div>`;

  const sessions = m.sessions || [];
  const totalMins = sessions.reduce((a, s) => a + (s.minutes || 0), 0);
  const logHtml = sessions.length
    ? `<div class="timeline">${sessions.map(s => `
        <div class="entry">
          <div class="when">${prettyDate(s.date)}${
            s.minutes ? ` <span class="mins">${duration(s.minutes)}</span>` : ""}</div>
          <div class="what">${esc(s.notes).replace(/\n/g, "<br>")}</div>
        </div>`).join("")}</div>`
    : `<div style="color:var(--muted);font-size:13px">
         No sessions logged yet — use Edit → Painting log to add one.</div>`;

  // The right-hand foot is a date once there is one and the status until
  // then, so the label has to follow the value: "Finished: in progress" read
  // as a contradiction. And under a "Status:" label the old fixed "in
  // progress" would have been a plain lie for anything still in Backlog or
  // Primed, so say which status it actually is.
  const foot = m.completed
    ? `Finished: ${prettyDate(m.completed)}`
    : `Status: ${esc(m.status)}`;

  // Starting a session is the one thing you want without opening the dialog
  // first, so it sits next to Edit. Once something is running the sidebar
  // pill owns the controls; this just reflects the state of this mini, and
  // says nothing at all when the timer belongs to a different one.
  const timerBtn = !timerOn()
    ? `<button class="btn ghost small" id="start-timer">▶&nbsp; Start timer</button>`
    : timer.modelId === m.id
      ? `<span class="running"><span class="dot${timerPaused() ? " off" : ""}"></span>
           <span class="timer-clock">${clock(timerElapsed())}</span></span>`
      : "";

  pane.innerHTML = `
    <div class="detail">
      <div class="title-row">
        <div style="flex:1;min-width:0">
          <h2>${esc(m.name)}</h2>
          <div class="sub">${esc([m.gameSystem, m.faction].filter(Boolean).join(" · ") || "—")}</div>
        </div>
        <button class="star-btn${m.favorite ? " on" : ""}" id="fav" title="Favourite">${m.favorite ? "★" : "☆"}</button>
      </div>
      <div style="display:flex;align-items:center;gap:10px">
        ${badge(m.status)}${m.count > 1
          ? `<span class="batch">${plural(m.count, "mini")} · ${m.done} painted</span>` : ""}
        <span style="flex:1"></span>
        ${timerBtn}
        <button class="btn ghost small" id="edit">Edit</button>
      </div>
      ${progressBar(m.done, m.count)}
      <div class="section">PHOTOS</div><div class="photos">${photos}</div>
      <div class="section">PAINTS USED</div>${chips}
      <div class="section">NOTES</div>${noteHtml}
      <div class="section">PAINTING LOG${
        sessions.length ? ` · ${plural(sessions.length, "session")}${
          totalMins ? ` · ${duration(totalMins)}` : ""}` : ""}</div>${logHtml}
      <div class="detail-foot">
        <span>Started: ${prettyDate(m.started)}</span>
        <span>${foot}</span>
      </div>
    </div>`;

  $("#edit").onclick = () => modelDialog(m);
  // no redraw here: startTimer -> drawTimer -> syncDetailTimer does it
  const startBtn = $("#start-timer");
  if (startBtn) startBtn.onclick = () => startTimer(m.id, m.name);
  $("#fav").onclick = async () => {
    await call(App().SaveModel, { ...m, favorite: !m.favorite });
    renderModels();
  };
  pane.querySelectorAll(".photo img").forEach(img => {
    img.onclick = () => call(App().OpenPhoto, img.dataset.file);
  });
}

function debounce(fn, ms) {
  let t = null;
  return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); };
}

/* =================================================================== PAINTS */
async function renderPaints() {
  const f = state.paints;
  // Facets first: the range list depends on the brand, and a brand can vanish
  // under us if its last paint was deleted.
  const facets = await call(App().Facets, f.brand === "All brands" ? "" : f.brand);
  if (!facets.brand) f.brand = "All brands";
  if (!facets.ranges.includes(f.range)) f.range = "All ranges";

  const page = await call(App().ListPaints, { search: f.search, type: f.type,
    brand: f.brand, range: f.range, stock: f.stock, limit: ROW_LIMIT });
  const shown = page.rows;

  const body = page.total ? shown.map(p => `
    <div class="trow" data-id="${p.id}">
      <span class="swatch" style="background:${esc(p.hex)}"></span>
      <span class="nm">${esc(p.name)}${
        p.code ? ` <span class="code">${esc(p.code)}</span>` : ""}</span>
      <span class="brand">${esc(p.brand || "—")}${
        p.range ? `<span class="range">${esc(p.range)}</span>` : ""}</span>
      <span><span class="tag">${esc(p.type)}</span></span>
      <span class="r" style="color:var(--muted)">${p.usedOn ? plural(p.usedOn, "mini", "minis") : "—"}</span>
      <span class="r ${p.owned ? "yes" : "unowned"}">${p.owned ? "✓ Yes" : "☆ No"}${
        p.wishlist ? `<span class="want">★ Wanted</span>` : ""}</span>
    </div>`).join("") + (page.total > shown.length
      ? `<div class="empty">Showing the first ${shown.length} of ${page.total}.
           Search or narrow the filters to see the rest.</div>` : "")
    : (facets.total
        ? `<div class="empty">No paints match those filters.</div>`
        : `<div class="empty"><strong>Your paint rack is empty.</strong>
             Add paints one at a time, or put the built-in ranges back:
             <div style="margin-top:12px"><button class="btn ghost" id="restore-lib">
               Restore built-in paints</button></div></div>`);

  setContent(`
    <div class="page-head">
      <div><h1>Paint Inventory</h1>
        <div class="sub">${plural(facets.total, "paint")} listed · ${facets.owned} owned${
          facets.wishlist ? ` · ${facets.wishlist} wanted` : ""}${
          page.total !== facets.total ? ` — ${page.total} match` : ""}</div></div>
      <div class="spacer"></div>
      <button class="btn" id="add-paint">+&nbsp; Add Paint</button>
    </div>
    <div class="filters">
      ${searchBox("p-search", "Search paints, ranges, codes…", f.search)}
      ${selectBox("p-brand", ["All brands", ...facets.brands], f.brand)}
      ${selectBox("p-range", ["All ranges", ...facets.ranges], f.range)}
      ${selectBox("p-type", ["All types", ...PAINT_TYPES], f.type)}
      ${selectBox("p-stock", ["All", "Owned only", "Not owned", "On wishlist"], f.stock)}
    </div>
    <div class="card ptable">
      <div class="thead"><span></span><span>NAME</span><span>BRAND</span>
        <span>TYPE</span><span class="r">USED ON</span><span class="r">IN STOCK</span></div>
      <div class="divider"></div>
      <div>${body}</div>
    </div>`);

  const search = $("#p-search");
  search.oninput = debounce(() => { f.search = search.value; renderPaints(); }, 180);
  $("#p-type").onchange = e => { f.type = e.target.value; renderPaints(); };
  $("#p-brand").onchange = e => { f.brand = e.target.value; renderPaints(); };
  $("#p-range").onchange = e => { f.range = e.target.value; renderPaints(); };
  $("#p-stock").onchange = e => { f.stock = e.target.value; renderPaints(); };
  $("#add-paint").onclick = () => paintDialog(null);

  const restore = $("#restore-lib");
  if (restore) restore.onclick = async () => {
    const n = await call(App().RestoreLibraryPaints);
    toast(`Added ${plural(n, "paint")}`);
    renderPaints();
  };

  content().querySelectorAll(".trow").forEach(row => {
    row.onclick = () => {
      const p = shown.find(x => x.id === +row.dataset.id);
      if (p) paintDialog(p);
    };
  });
}

/* ===================================================================== TIPS */
async function renderTips() {
  const f = state.tips;
  const tips = await call(App().ListTips, { search: f.search, category: f.category });

  const cards = tips.length ? `<div class="tip-grid">${tips.map(t => {
    const col = TIP_COLORS[t.category] || "#6b7280";
    const lines = (t.body || "").split("\n").filter(l => l.trim()).slice(0, 6);
    return `<div class="card tip" data-id="${t.id}">
      <div class="stripe" style="background:${col}"></div>
      <div class="body">
        <div class="head"><h3>${esc(t.title)}</h3>
          <span class="badge" style="background:${col}">${esc(t.category)}</span></div>
        <div class="notes">${lines.map(l => `<p>${esc(l.trim())}</p>`).join("")}</div>
        ${(t.tags || []).length ? `<div class="tags">${
          t.tags.slice(0, 6).map(x => `<span>#${esc(x)}</span>`).join("")}</div>` : ""}
      </div></div>`;
  }).join("")}</div>`
    : `<div class="card"><div class="empty"><strong>No notes yet.</strong>
         Save your recipes here so you can find them again.</div></div>`;

  setContent(`
    <div class="page-head">
      <div><h1>Technique Notes</h1>
        <div class="sub">Your painting recipes &amp; methods — searchable</div></div>
      <div class="spacer"></div>
      <button class="btn" id="add-tip">+&nbsp; Add Note</button>
    </div>
    <div class="filters">
      ${searchBox("t-search", "Search notes, tags, recipes…", f.search)}
      ${selectBox("t-cat", ["All", ...TIP_CATEGORIES], f.category)}
    </div>
    ${cards}`);

  const search = $("#t-search");
  search.oninput = debounce(() => { f.search = search.value; renderTips(); }, 180);
  $("#t-cat").onchange = e => { f.category = e.target.value; renderTips(); };
  $("#add-tip").onclick = () => tipDialog(null);
  content().querySelectorAll(".tip").forEach(c => {
    c.onclick = () => tipDialog(tips.find(t => t.id === +c.dataset.id));
  });
}

/* =================================================================== MODALS */
function openModal(html) {
  const modal = $("#modal");
  // A modal re-renders in place - a tab switch, or the paint picker filtering
  // as you type - so the caret needs the same treatment the screens get.
  const snap = captureFocus(modal);
  modal.innerHTML = html;
  $("#modal-backdrop").hidden = false;
  restoreFocus(snap);
  const close = $("#modal .close");
  if (close) close.onclick = closeModal;
  document.addEventListener("keydown", escClose);
}
function closeModal() {
  $("#modal-backdrop").hidden = true;
  $("#modal").innerHTML = "";
  document.removeEventListener("keydown", escClose);
  // A dialog can start or stop the timer before it closes, and Cancel and
  // Escape don't redraw anything on their way out, so the pane behind would
  // otherwise be left showing the state from before the dialog opened.
  syncDetailTimer();
}
function escClose(e) { if (e.key === "Escape") closeModal(); }

/* ---- mini ---- */
// opts lets a caller open straight onto a tab with the log form part-filled -
// stopping the timer arrives here with the minutes already counted.
async function modelDialog(model, opts = {}) {
  const isNew = !model;
  let m = model ? { ...model } : {
    id: 0, name: "", gameSystem: "", faction: "", status: "Backlog",
    count: 1, done: 0, favorite: false, notes: "",
    started: new Date().toISOString().slice(0, 10),
    completed: "", paintIds: [], photos: [],
  };
  let tab = opts.tab || "details";
  let paintSearch = "";
  let paintBrand = "All brands";
  // the log entry currently being edited, {} = new
  let editSession = opts.session ? { ...opts.session } : {};
  const allPaints = await call(App().AllPaints);
  // The whole rack is already in hand, so the brand list comes off it rather
  // than out of a second trip to the backend.
  const pickerBrands = [...new Set(allPaints.map(p => p.brand).filter(Boolean))]
    .sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));

  function render() {
    const chosen = new Set(m.paintIds || []);
    const q = paintSearch.toLowerCase();
    // The rack holds the whole catalogue, so float the paints that are
    // actually on the desk — owned, or already ticked — to the top.
    const matched = allPaints
      .filter(p => paintBrand === "All brands" || p.brand === paintBrand)
      .filter(p => !q || (p.name + " " + p.brand + " " + p.range + " " + p.code)
        .toLowerCase().includes(q))
      .sort((a, b) => (chosen.has(b.id) - chosen.has(a.id)) || (b.owned - a.owned));
    const list = matched.slice(0, PICKER_LIMIT);

    const details = `
      <div class="field"><label>Name</label>
        <input type="text" id="f-name" value="${esc(m.name)}" autofocus></div>
      <div class="grid2">
        <div class="field"><label>Game system</label>
          <input type="text" id="f-sys" value="${esc(m.gameSystem)}"></div>
        <div class="field"><label>Faction / unit</label>
          <input type="text" id="f-fac" value="${esc(m.faction)}"></div>
      </div>
      <div class="grid3">
        <div class="field"><label>Status</label>${selectBox("f-status", STATUSES, m.status)}</div>
        <div class="field"><label>How many minis</label>
          <input type="text" id="f-count" inputmode="numeric" value="${m.count || 1}">
          <div class="hint">A squad is one entry — say how many it holds.</div>
        </div>
        <div class="field"><label>Painted so far</label>
          <input type="text" id="f-painted" inputmode="numeric" value="${m.done || 0}">
          <div class="hint">Filled in for you when this goes Complete.</div>
        </div>
      </div>
      <div class="grid2">
        <div class="field"><label>Started</label>
          <input type="date" id="f-started" value="${esc(m.started)}"></div>
        <div class="field"><label>Finished</label>
          <input type="date" id="f-done" value="${esc(m.completed)}"></div>
      </div>
      <label class="check" style="margin-bottom:14px">
        <input type="checkbox" id="f-fav"${m.favorite ? " checked" : ""}> Favourite</label>
      <div class="field"><label>Notes for this mini</label>
        <textarea id="f-notes" placeholder="One step per line…">${esc(m.notes)}</textarea></div>`;

    const paintsTab = allPaints.length ? `
      <div class="filters">
        ${searchBox("f-psearch", "Filter paints…", paintSearch)}
        ${selectBox("f-pbrand", ["All brands", ...pickerBrands], paintBrand)}
      </div>
      <div style="color:var(--muted);font-size:13px;margin:0 0 8px">
        Tick every paint you used on this mini. Ones you own are listed first${
          matched.length > list.length
            ? `, and only the first ${list.length} of ${matched.length} are shown —
               keep typing to narrow it down` : ""}.${
          chosen.size ? ` ${plural(chosen.size, "paint")} ticked so far —
            narrowing the filters hides rows but never unticks them.` : ""}</div>
      <div class="picker">${list.map(p => `
        <label><input type="checkbox" data-pid="${p.id}"${chosen.has(p.id) ? " checked" : ""}>
          <span class="swatch" style="background:${esc(p.hex)};width:16px;height:16px"></span>
          <span>${esc(p.name)}</span>
          <span class="meta">${esc(p.brand)}${p.range ? " " + esc(p.range) : ""} · ${
            esc(p.type)}${p.owned ? "" : " · not owned"}</span></label>`).join("")}</div>`
      : `<div class="note">Your paint rack is empty. Add some paints in Paint Inventory
           first, then come back and tick the ones you used.</div>`;

    const photosTab = `
      ${isNew && !m.id ? `<div class="note">Photos are saved straight to disk, so this
        mini needs a name and a save first. Add a photo and it'll be saved automatically.</div>` : ""}
      <div style="display:flex;gap:8px;margin-bottom:14px">
        <button class="btn ghost" id="add-prog">+&nbsp; Add progress photo</button>
        <button class="btn ghost" id="add-final">+&nbsp; Add final photo</button>
      </div>
      <div class="photos">${(m.photos || []).map(p => `
        <div class="photo">
          <img src="${photoSrc(p)}" alt="" loading="lazy">
          <div class="cap">
            <span class="badge" style="background:${
              p.kind === "Final" ? STATUS_COLORS["Complete"] : "#64748b"};font-size:10px">${esc(p.kind)}</span>
            <span class="pick${p.cover ? " on" : ""}" data-cover="${p.id}"
              title="Use this as the cover in the list">${p.cover ? "★" : "☆"}</span>
            <span class="x" data-photo="${p.id}">✕</span></div>
        </div>`).join("") || `<div style="color:var(--muted);font-size:13px">
          No photos yet. Add a progress shot or a final picture.</div>`}</div>`;

    const sess = m.sessions || [];
    // The modal backdrop covers the sidebar, so while this dialog is open the
    // pill out there can't be clicked - the log tab has to carry its own set
    // of controls. A mini with no id yet has nothing to hang a session on.
    const timerBar = !m.id ? "" : `<div class="timer-bar">${
      !timerOn()
        ? `<button class="btn ghost" id="s-start">▶&nbsp; Start timer</button>
           <span class="hint">Counts while you paint — stop it and the minutes land below.</span>`
        : timer.modelId === m.id
          ? `<span class="dot${timerPaused() ? " off" : ""}"></span>
             <span class="timer-clock">${clock(timerElapsed())}</span>
             <button class="btn ghost" id="s-toggle">${timerPaused() ? "Resume" : "Pause"}</button>
             <button class="btn" id="s-stop">Stop &amp; fill in</button>`
          : `<span class="hint">Timer running on ${esc(timer.modelName)}.</span>`
    }</div>`;

    const logTab = `
      ${!m.id ? `<div class="note">Log entries save straight away, so this mini needs
        a name first — add an entry and it'll be saved automatically.</div>` : ""}
      ${timerBar}
      <div class="log-form">
        <div class="grid-log">
          <div class="field"><label>Date</label>
            <input type="date" id="s-date" value="${esc(editSession.date || todayISO())}"></div>
          <div class="field"><label>Minutes (optional)</label>
            <input type="text" id="s-mins" inputmode="numeric" placeholder="e.g. 90"
                   value="${editSession.minutes ? editSession.minutes : ""}"></div>
        </div>
        <div class="field"><label>What did you get done?</label>
          <textarea id="s-notes" style="min-height:80px"
            placeholder="Blocked in the armour, first highlight pass on the cloak…">${esc(editSession.notes || "")}</textarea></div>
        <div style="display:flex;gap:8px">
          <button class="btn" id="s-add">${editSession.id ? "Update entry" : "Add entry"}</button>
          ${editSession.id ? `<button class="btn ghost" id="s-cancel-edit">Cancel edit</button>` : ""}
        </div>
      </div>
      <div class="section" style="margin-top:20px">HISTORY${
        sess.length ? ` · ${plural(sess.length, "session")}` : ""}</div>
      ${sess.length ? `<div class="timeline editable">${sess.map(s => `
        <div class="entry" data-sess="${s.id}">
          <div class="when">${prettyDate(s.date)}${
            s.minutes ? ` <span class="mins">${duration(s.minutes)}</span>` : ""}
            <span class="acts"><a class="s-edit" data-id="${s.id}">edit</a>
              <a class="s-del" data-id="${s.id}">delete</a></span></div>
          <div class="what">${esc(s.notes).replace(/\n/g, "<br>")}</div>
        </div>`).join("")}</div>`
        : `<div style="color:var(--muted);font-size:13px">Nothing logged yet.</div>`}`;

    openModal(`
      <header><h2>${isNew ? "Add Mini" : "Edit Mini"}</h2>
        <button class="close">✕</button></header>
      <div class="mbody">
        <div class="tabs">
          <button data-tab="details" class="${tab === "details" ? "on" : ""}">Details</button>
          <button data-tab="log" class="${tab === "log" ? "on" : ""}">Painting log</button>
          <button data-tab="paints" class="${tab === "paints" ? "on" : ""}">Paints used</button>
          <button data-tab="photos" class="${tab === "photos" ? "on" : ""}">Photos</button>
        </div>
        ${tab === "details" ? details : tab === "log" ? logTab
          : tab === "paints" ? paintsTab : photosTab}
      </div>
      <footer>
        ${!isNew ? `<button class="btn danger" id="del">Delete mini</button>` : ""}
        <div class="spacer"></div>
        <button class="btn ghost" id="cancel">Cancel</button>
        <button class="btn" id="save">Save</button>
      </footer>`);

    $("#modal").querySelectorAll(".tabs button").forEach(b => {
      b.onclick = () => { collect(); tab = b.dataset.tab; render(); };
    });
    $("#cancel").onclick = closeModal;
    $("#save").onclick = save;
    const del = $("#del");
    if (del) del.onclick = async () => {
      const ok = await call(App().Confirm, "Delete mini",
        `Delete “${m.name}” and its photos?\nThis can't be undone.`, "Delete");
      if (!ok) return;
      await call(App().DeleteModel, m.id);
      // Nothing left to log the session against.
      if (timer.modelId === m.id) clearTimer();
      closeModal(); state.models.selected = null; show(state.screen);
    };

    // A tab switch leaves focus on the tab button, so drop the caret somewhere
    // useful. A re-render mid-typing has already put it back, and that wins.
    if (!$("#modal").contains(document.activeElement)) {
      const first = tab === "details" ? $("#f-name")
                  : tab === "paints" ? $("#f-psearch") : null;
      if (first) first.focus();
    }

    if (tab === "paints" && allPaints.length) {
      const ps = $("#f-psearch");
      ps.oninput = debounce(() => { collect(); paintSearch = ps.value; render(); }, 150);
      $("#f-pbrand").onchange = e => { paintBrand = e.target.value; render(); };
      $("#modal").querySelectorAll(".picker input").forEach(cb => {
        cb.onchange = () => {
          const id = +cb.dataset.pid;
          const set = new Set(m.paintIds || []);
          cb.checked ? set.add(id) : set.delete(id);
          m.paintIds = [...set];
        };
      });
    }

    if (tab === "log") {
      const st = $("#s-start");
      if (st) st.onclick = () => { startTimer(m.id, m.name); render(); };
      const tg = $("#s-toggle");
      if (tg) tg.onclick = () => { timerPaused() ? resumeTimer() : pauseTimer(); render(); };
      const sp = $("#s-stop");
      if (sp) sp.onclick = () => {
        // Keep whatever is already typed: the notes live in the DOM, not in
        // editSession, so a re-render would otherwise wipe them.
        editSession = { ...editSession, minutes: timerMinutes(), notes: $("#s-notes").value };
        clearTimer();
        render();
        const n = $("#s-notes");
        if (n) { n.focus(); n.setSelectionRange(n.value.length, n.value.length); }
      };

      $("#s-add").onclick = addSession;
      const ce = $("#s-cancel-edit");
      if (ce) ce.onclick = () => { editSession = {}; render(); };
      $("#modal").querySelectorAll(".s-edit").forEach(a => {
        a.onclick = () => {
          editSession = { ...(m.sessions || []).find(x => x.id === +a.dataset.id) };
          render();
        };
      });
      $("#modal").querySelectorAll(".s-del").forEach(a => {
        a.onclick = async () => {
          const updated = await call(App().DeleteSession, m.id, +a.dataset.id);
          if (updated) { m.sessions = updated.sessions || []; editSession = {}; render(); }
        };
      });
    }

    if (tab === "photos") {
      $("#add-prog").onclick = () => addPhotos("Progress");
      $("#add-final").onclick = () => addPhotos("Final");
      $("#modal").querySelectorAll(".photo .x").forEach(x => {
        x.onclick = async () => {
          const updated = await call(App().DeletePhoto, m.id, +x.dataset.photo);
          if (updated) { m.photos = updated.photos || []; render(); }
        };
      });
      $("#modal").querySelectorAll(".photo .pick").forEach(s => {
        s.onclick = async () => {
          const updated = await call(App().SetCoverPhoto, m.id, +s.dataset.cover);
          if (updated) { m.photos = updated.photos || []; render(); }
        };
      });
    }
  }

  // Pull whatever is on screen back into `m` before switching tabs or saving,
  // so edits aren't lost when the modal re-renders.
  function collect() {
    if (tab !== "details") return;
    const g = id => { const e = $("#" + id); return e ? e.value : undefined; };
    if (g("f-name") !== undefined) {
      m.name = g("f-name").trim();
      m.gameSystem = g("f-sys").trim();
      m.faction = g("f-fac").trim();
      m.status = g("f-status");
      m.count = intOf(g("f-count"), 1);
      m.done = intOf(g("f-painted"), 0);
      m.started = g("f-started");
      m.completed = g("f-done");
      m.notes = g("f-notes");
      m.favorite = $("#f-fav").checked;
    }
  }

  async function ensureSaved() {
    collect();
    if (!m.name) { tab = "details"; render(); toast("Please give this mini a name"); return false; }
    const saved = await call(App().SaveModel, m);
    m = { ...m, ...saved };
    return true;
  }

  async function addPhotos(kind) {
    if (!m.id && !(await ensureSaved())) return;
    const updated = await call(App().AddPhotos, m.id, kind);
    if (updated) { m.photos = updated.photos || []; render(); }
  }

  async function addSession() {
    const notes = $("#s-notes").value.trim();
    if (!notes) { toast("Write a line about what you did this session"); return; }
    if (!m.id && !(await ensureSaved())) return;
    const updated = await call(App().SaveSession, m.id, {
      id: editSession.id || 0,
      date: $("#s-date").value || todayISO(),
      minutes: intOf($("#s-mins").value, 0),
      notes,
    });
    if (updated) {
      m.sessions = updated.sessions || [];
      m.started = updated.started;
      editSession = {};
      render();
      toast("Session logged");
    }
  }

  async function save() {
    if (!(await ensureSaved())) return;
    closeModal();
    state.models.selected = m.id;
    // Redraw whichever screen is actually up, not the models list. Stopping
    // the timer opens this dialog from wherever you happen to be, and jumping
    // the content to Models would leave it disagreeing with the sidebar.
    show(state.screen);
    toast("Saved");
  }

  render();
}

/* ---- paint ---- */
const KNOWN_BRANDS = ["Warhammer Colour", "Vallejo", "Army Painter", "Scale75",
                      "AK Interactive", "Kimera", "Two Thin Coats", "Pro Acryl",
                      "Ionic Smart Colors", "Reaper", "Tamiya", "Golden",
                      "Turbo Dork", "Green Stuff World"];

async function paintDialog(paint) {
  const isNew = !paint;
  const p = paint ? { ...paint } : {
    id: 0, name: "", brand: "", range: "", code: "", type: "Base",
    hex: "#888888", owned: true, wishlist: false, notes: "",
  };
  // suggestions = brands already in the collection, plus the common ones,
  // but the field is free text so any brand can simply be typed in
  const facets = await call(App().Facets, p.brand || "");
  const suggestions = [...new Set([...facets.brands, ...KNOWN_BRANDS])].sort((a, b) =>
    a.toLowerCase().localeCompare(b.toLowerCase()));
  const ranges = facets.ranges;

  // Every field below carries a -f suffix. The inventory screen behind the
  // dialog stays in the document and owns the plain names for its own filter
  // bar, and it comes first, so an id shared with a filter resolves to the
  // filter and the field the user actually typed into is never read.
  openModal(`
    <header><h2>${isNew ? "Add Paint" : "Edit Paint"}</h2><button class="close">✕</button></header>
    <div class="mbody">
      <div class="field"><label>Paint name</label>
        <input type="text" id="p-name" value="${esc(p.name)}"></div>
      <div class="grid2">
        <div class="field"><label>Brand</label>
          <input type="text" id="p-brand-f" value="${esc(p.brand)}" list="brand-list"
                 placeholder="Type any brand…" autocomplete="off">
          <datalist id="brand-list">${
            suggestions.map(b => `<option value="${esc(b)}">`).join("")}</datalist>
          <div class="hint">Pick from the list or type a new one — it'll be remembered.</div>
        </div>
        <div class="field"><label>Type</label>${selectBox("p-type-f", PAINT_TYPES, p.type)}</div>
      </div>
      <div class="grid2">
        <div class="field"><label>Range <span class="opt">(optional)</span></label>
          <input type="text" id="p-range-f" value="${esc(p.range)}" list="range-list"
                 placeholder="e.g. Layer, Wave 2" autocomplete="off">
          <datalist id="range-list">${
            ranges.map(r => `<option value="${esc(r)}">`).join("")}</datalist>
        </div>
        <div class="field"><label>Code <span class="opt">(optional)</span></label>
          <input type="text" id="p-code-f" value="${esc(p.code)}"
                 placeholder="e.g. AK11001" spellcheck="false"></div>
      </div>
      <div class="field"><label>Color</label>
        <div class="color-row">
          <input type="color" id="p-color" value="${esc(p.hex)}">
          <input type="text" id="p-hex" value="${esc(p.hex)}" spellcheck="false">
        </div></div>
      <label class="check"><input type="checkbox" id="p-owned-f"${p.owned ? " checked" : ""}>
        Owned</label>
      <label class="check"><input type="checkbox" id="p-wish-f"${p.wishlist ? " checked" : ""}>
        Wishlist</label>
      ${!isNew ? `<div style="color:var(--muted);font-size:13px;margin-top:14px">
        Used on ${plural(paint.usedOn || 0, "mini", "minis")}.</div>` : ""}
    </div>
    <footer>
      ${!isNew ? `<button class="btn danger" id="p-del">Delete paint</button>` : ""}
      <div class="spacer"></div>
      <button class="btn ghost" id="p-cancel">Cancel</button>
      <button class="btn" id="p-save">Save</button>
    </footer>`);

  $("#p-name").focus();
  const color = $("#p-color"), hex = $("#p-hex");
  color.oninput = () => { hex.value = color.value; };
  hex.oninput = () => { if (/^#[0-9a-f]{6}$/i.test(hex.value)) color.value = hex.value; };

  $("#p-cancel").onclick = closeModal;
  $("#p-save").onclick = async () => {
    const saved = await call(App().SavePaint, {
      ...p,
      name: $("#p-name").value.trim(),
      brand: $("#p-brand-f").value.trim(),
      range: $("#p-range-f").value.trim(),
      code: $("#p-code-f").value.trim(),
      type: $("#p-type-f").value,
      hex: hex.value.trim(),
      owned: $("#p-owned-f").checked,
      wishlist: $("#p-wish-f").checked,
    });
    // The rack runs to well over a thousand paints and the table draws the
    // first few hundred, ordered by brand, so one just added lands off the end
    // of the list and reads as a paint that never saved. Point the screen at
    // it: the filters that could hide it come off and the search box, which is
    // on screen and says what it's doing, narrows to the new paint.
    if (isNew) {
      Object.assign(state.paints, { search: saved.name, type: "All types",
        brand: "All brands", range: "All ranges", stock: "All" });
    }
    closeModal(); renderPaints(); toast("Saved");
  };
  const del = $("#p-del");
  if (del) del.onclick = async () => {
    const used = paint.usedOn || 0;
    const ok = await call(App().Confirm, "Delete paint",
      `Delete “${p.name}”?` + (used ? `\nIt's currently linked to ${plural(used, "mini", "minis")}.` : ""),
      "Delete");
    if (!ok) return;
    await call(App().DeletePaint, p.id);
    closeModal(); renderPaints();
  };
}

/* ---- tip ---- */
function tipDialog(tip) {
  const isNew = !tip;
  const t = tip ? { ...tip } : { id: 0, title: "", category: "Other", body: "", tags: [] };

  openModal(`
    <header><h2>${isNew ? "Add Technique Note" : "Edit Note"}</h2><button class="close">✕</button></header>
    <div class="mbody">
      <div class="field"><label>Title</label>
        <input type="text" id="t-title" value="${esc(t.title)}"></div>
      <div class="field"><label>Category</label>${selectBox("t-cat-f", TIP_CATEGORIES, t.category)}</div>
      <div class="field"><label>The note / recipe (one step per line)</label>
        <textarea id="t-body" style="min-height:150px">${esc(t.body)}</textarea></div>
      <div class="field"><label>Tags (comma separated)</label>
        <input type="text" id="t-tags" value="${esc((t.tags || []).join(", "))}"></div>
    </div>
    <footer>
      ${!isNew ? `<button class="btn danger" id="t-del">Delete note</button>` : ""}
      <div class="spacer"></div>
      <button class="btn ghost" id="t-cancel">Cancel</button>
      <button class="btn" id="t-save">Save</button>
    </footer>`);

  $("#t-title").focus();
  $("#t-cancel").onclick = closeModal;
  $("#t-save").onclick = async () => {
    await call(App().SaveTip, {
      ...t,
      title: $("#t-title").value.trim(),
      category: $("#t-cat-f").value,
      body: $("#t-body").value,
      tags: $("#t-tags").value.split(",").map(s => s.trim()).filter(Boolean),
    });
    closeModal(); renderTips(); toast("Saved");
  };
  const del = $("#t-del");
  if (del) del.onclick = async () => {
    const ok = await call(App().Confirm, "Delete note", `Delete “${t.title}”?`, "Delete");
    if (!ok) return;
    await call(App().DeleteTip, t.id);
    closeModal(); renderTips();
  };
}

/* ===================================================================== boot */
function ready() {
  return new Promise(resolve => {
    const check = () => {
      if (window.go && window.go.main && window.go.main.App) return resolve();
      setTimeout(check, 30);
    };
    check();
  });
}

window.addEventListener("DOMContentLoaded", async () => {
  // Before `ready()`, which waits on the Go bindings: the theme is pure
  // frontend state and shouldn't sit behind the backend coming up.
  initTheme();
  await ready();
  initNav();
  // The build names itself rather than carrying a string the source has to
  // remember to update - the linker stamps it in, so it can't go stale.
  App().Version().then(v => { $("#version").textContent = v; });
  const warn = await App().StartupError();
  if (warn) toast(warn);
  show("dashboard");
});
