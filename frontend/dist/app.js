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
// Mirrors finished() in the store: the statuses that mean the painting is over.
const isFinished = status => status === "Complete" || status === "Display";
// A photo's badge. The painter's finished shot borrows the Complete green;
// the maker's reference gets a hue of its own, so it never reads as one of
// the stages of the work; anything else is a progress shot.
const PHOTO_COLORS = { "Final": STATUS_COLORS["Complete"], "Product": "#0891b2" };
const photoColor = kind => PHOTO_COLORS[kind] || "#64748b";
// Product and Final are the two named kinds; everything else, including a
// kind written by a build that predates this one, is a progress shot. Mirrors
// the default branch of CoverPhoto in the store.
const photoGroup = p => (p.kind === "Product" || p.kind === "Final") ? p.kind : "Progress";
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
  models: { search: "", status: "All", system: "All", faction: "All",
            project: "All", sort: "Status", desc: false, selected: null },
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

// The shot that stands for a mini in the list. An explicit choice wins;
// failing that a finished mini is represented by the finished article and one
// still on the desk by the maker's reference, since a row is picked out of a
// list by eye and a studio photograph is the more recognisable of the two.
// Within a kind the newest wins.
//
// Mirrors CoverPhoto in the store. The two have to agree: they draw the same
// square from the same data, and a list that disagrees with the record behind
// it is worse than either rule on its own.
function coverPhoto(m) {
  const photos = m.photos || [];
  const chosen = photos.find(p => p.cover);
  if (chosen) return chosen;
  const newest = kind => [...photos].reverse().find(p => photoGroup(p) === kind);
  const product = newest("Product"), final = newest("Final"), progress = newest("Progress");
  return (isFinished(m.status) ? [final, product, progress]
                               : [product, final, progress]).find(Boolean) || null;
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
// A toast can carry one action, which is how a delete offers to undo itself.
// It stays up longer when it does: the message is no longer just telling you
// what happened, it's waiting to hear whether you meant it.
function toast(msg, action) {
  const t = $("#toast");
  t.textContent = msg;
  if (action) {
    const btn = document.createElement("button");
    btn.className = "undo";
    btn.textContent = action.label;
    btn.onclick = () => { t.hidden = true; action.run(); };
    t.appendChild(btn);
  }
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, action ? 8000 : 2600);
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
// A free-text field that suggests what the collection already uses - the same
// trick the paint dialog plays with brands. Anything can be typed, but the
// second mini of a faction only has to be picked, which is what keeps the
// filters from filling up with three spellings of the same army.
function suggestBox(id, value, values, placeholder) {
  return `<input type="text" id="${id}" value="${esc(value || "")}" list="${id}-list"
      placeholder="${esc(placeholder)}" autocomplete="off">
    <datalist id="${id}-list">${
      (values || []).map(v => `<option value="${esc(v)}">`).join("")}</datalist>`;
}

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

/* ------------------------------------------------------------- keyboard */
// Arrow keys walk the models list and Enter opens what's highlighted, so a
// pass through the collection doesn't have to be done with the mouse.
//
// One listener on the document, registered once, rather than one per render:
// the list is rebuilt on every keystroke in the search box, and handlers
// attached to it would be attached again with it.
function initKeys() {
  document.addEventListener("keydown", e => {
    if (state.screen !== "models") return;
    if (!$("#modal-backdrop").hidden) return; // the dialog owns the keyboard
    // Typing in the search box means arrows move the caret, not the list.
    const el = document.activeElement;
    if (el && /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName)) return;

    const step = { ArrowDown: 1, ArrowUp: -1 }[e.key];
    if (!step && e.key !== "Enter") return;

    const rows = [...document.querySelectorAll("#m-rows .row")];
    if (!rows.length) return;
    const at = rows.findIndex(r => +r.dataset.id === state.models.selected);
    e.preventDefault();
    if (!step) { openModel(state.models.selected); return; }

    // A first press with nothing selected starts at the top rather than
    // jumping to whichever end the arrow happens to point away from.
    const next = rows[at < 0 ? 0 : Math.min(rows.length - 1, Math.max(0, at + step))];
    state.models.selected = +next.dataset.id;
    rows.forEach(r => r.classList.toggle("sel", r === next));
    next.scrollIntoView({ block: "nearest" });
    // Only the pane is redrawn: rebuilding the list would throw away the
    // rows this is walking, and held arrows would fight the round trip.
    renderModelDetail(state.models.selected);
  });
}

function show(screen) {
  state.screen = screen;
  ({ dashboard: renderDashboard, models: renderModels, projects: renderProjects,
     paints: renderPaints, wishlist: renderWishlist, tips: renderTips,
     time: renderTime })[screen]();
}

// Landing on a mini from somewhere else - a log entry on the dashboard, a
// paint that turned out to be on it. The sidebar has to move too, or the
// highlighted entry and the screen it points at disagree.
function jumpToMini(id) {
  state.models.selected = id;
  document.querySelectorAll("#nav li").forEach(n =>
    n.classList.toggle("active", n.dataset.screen === "models"));
  show("models");
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
    el.onclick = () => jumpToMini(+el.dataset.model);
  });
}

/* =================================================================== MODELS */
async function renderModels() {
  const f = state.models;
  const [models, facets] = await Promise.all([
    call(App().ListModels, { search: f.search, status: f.status, system: f.system,
      faction: f.faction, project: f.project, sort: f.sort, desc: f.desc }),
    call(App().ModelFacets),
  ]);

  // A filter pointing at a value nothing carries any more - the last mini of
  // that faction was renamed or deleted - would hide the whole collection
  // with no way to see why, so it falls back to showing everything.
  const facetValues = { system: facets.systems, faction: facets.factions,
                        project: facets.projects };
  let stale = false;
  Object.entries(facetValues).forEach(([key, values]) => {
    if (f[key] !== "All" && !values.includes(f[key])) { f[key] = "All"; stale = true; }
  });
  if (stale) return renderModels();

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
          <div class="sub">${esc(
            [m.gameSystem, m.faction, m.project].filter(Boolean).join(" · ") || "—")}</div>
          ${progressBar(m.done, m.count)}
        </div>
      </div>
      <div>${badge(m.status)}</div>
      <div class="count">${(m.paintIds || []).length}${m.favorite ? ' <span class="star">★</span>' : ""}</div>
    </div>`;
  }).join("")
    : `<div class="empty"><strong>No minis yet.</strong>Click “+ Add Mini” to start your collection.</div>`;

  // A filter nobody can use is clutter: one game system across the whole
  // collection means the system picker can only ever say "all of them".
  const facetBox = (id, key, label) => facetValues[key].length < 2 ? ""
    : selectBox(id, ["All", ...facetValues[key]], f[key], label + ": ");

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
      ${selectBox("m-status", ["All", ...STATUSES], f.status, "Status: ")}
      ${facetBox("m-system", "system", "System")}
      ${facetBox("m-faction", "faction", "Faction")}
      ${facetBox("m-project", "project", "Project")}
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
  Object.keys(facetValues).forEach(key => {
    const el = $(`#m-${key}`);
    if (el) el.onchange = e => { f[key] = e.target.value; renderModels(); };
  });
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

  const photos = (m.photos || []).length ? m.photos.map(p => `
    <div class="photo">
      <img src="${photoSrc(p)}" alt="" data-file="${esc(p.file)}" loading="lazy">
      <div class="cap"><span class="badge" style="background:${
        photoColor(p.kind)};font-size:10px">${esc(p.kind)}</span>${
        p.cover ? `<span class="is-cover" title="Cover shot">★</span>` : ""}</div>
    </div>`).join("")
    : `<div style="color:var(--muted);font-size:13px">No photos yet — add progress shots when you edit this mini.</div>`;

  const chips = paintChips(m.paintIds, byId)
    || `<div style="color:var(--muted);font-size:13px">None recorded yet.</div>`;

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
          <div class="sub">${esc(
            [m.gameSystem, m.faction, m.project].filter(Boolean).join(" · ") || "—")}</div>
        </div>
        <button class="star-btn${m.favorite ? " on" : ""}" id="fav" title="Favourite">${m.favorite ? "★" : "☆"}</button>
      </div>
      <div style="display:flex;align-items:center;gap:10px">
        ${badge(m.status)}${m.count > 1
          ? `<span class="batch">${plural(m.count, "mini")} · ${m.done} painted</span>` : ""}
        <span style="flex:1"></span>
        ${timerBtn}
        <button class="btn ghost small" id="export" title="Save this mini as a page or a note">Export</button>
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
  // The save dialog decides the format: an .html file carries its photos
  // inside it, an .md file writes them into a folder alongside. Cancelling
  // comes back with no path and should say nothing at all.
  $("#export").onclick = async () => {
    const path = await call(App().ExportMini, m.id);
    if (path) toast("Exported");
  };
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

/* ================================================================= PROJECTS */
// A project is not a record you create - it is whatever minis say they belong
// to, exactly as a brand is whatever paints say they are. This screen reads
// that name and adds up what's behind it; the only things stored here are the
// deadline and the notes, which have nowhere else to live.
async function renderProjects() {
  const projects = await call(App().Projects);

  // "in 9 days", "today", "3 days over" - a countdown is what a deadline is
  // for, and the date alone makes you do the arithmetic yourself.
  const countdown = p => {
    if (!p.due) return "";
    const d = p.daysLeft;
    const when = d === 0 ? "due today"
      : d > 0 ? `${plural(d, "day")} left`
      : `${plural(-d, "day")} over`;
    return `<span class="due${d < 0 ? " over" : d <= 7 ? " soon" : ""}">${
      when} · ${prettyDate(p.due)}</span>`;
  };

  const cards = projects.length ? projects.map(p => `
    <div class="card project" data-name="${esc(p.name)}">
      <div class="phead">
        <h2>${esc(p.name)}</h2>
        ${countdown(p)}
        <span class="spacer"></span>
        <button class="btn ghost small" data-edit="${esc(p.name)}">Edit</button>
      </div>
      <div class="pstats">
        <span><strong>${p.done}</strong> of <strong>${p.minis}</strong> minis painted</span>
        <span>${plural(p.entries, "entry", "entries")}</span>
        ${p.minutes ? `<span>${duration(p.minutes)} across ${
          plural(p.sessions, "session")}</span>` : ""}
        ${p.minis && p.minutes ? `<span>${duration(Math.round(p.minutes / p.minis))} a mini</span>` : ""}
      </div>
      ${progressBar(p.done, Math.max(p.minis, 2))}
      ${p.notes ? `<div class="pnotes">${esc(p.notes)}</div>` : ""}
      ${p.next.length ? `<div class="section">NEXT UP</div>
        <div class="links">${p.next.map(n => `
          <span class="link to-mini" data-id="${n.id}">${esc(n.name)}${
            n.count > 1 ? `<span class="qty">${n.done}/${n.count}</span>` : ""}</span>`).join("")}</div>`
        : `<div class="pnotes">Everything filed under this is finished.</div>`}
      <div class="divider"></div>
      <div class="pfoot"><span class="open" data-open="${esc(p.name)}">
        Show these ${plural(p.entries, "entry", "entries")} in Models →</span></div>
    </div>`).join("")
    : `<div class="card"><div class="empty"><strong>No projects yet.</strong>
         Put a project name on a mini — an army, a tournament list, a boxed
         game — and it shows up here with everything else that shares it.</div></div>`;

  setContent(`
    <div class="page-head">
      <div><h1>Projects</h1>
        <div class="sub">What you're working towards${
          projects.length ? ` — ${plural(projects.length, "project")}` : ""}</div></div>
    </div>
    <div class="project-grid">${cards}</div>`);

  content().querySelectorAll("[data-edit]").forEach(b => {
    b.onclick = () => projectDialog(projects.find(p => p.name === b.dataset.edit));
  });
  content().querySelectorAll("[data-open]").forEach(b => {
    b.onclick = () => {
      // Landing in Models with only this project showing, and the other
      // filters cleared so nothing else is quietly hiding half of it.
      Object.assign(state.models, { search: "", status: "All", system: "All",
        faction: "All", project: b.dataset.open });
      document.querySelectorAll("#nav li").forEach(n =>
        n.classList.toggle("active", n.dataset.screen === "models"));
      show("models");
    };
  });
  content().querySelectorAll(".link.to-mini").forEach(el => {
    el.onclick = () => jumpToMini(+el.dataset.id);
  });
}

// Renaming is the answer to the one real weakness of grouping on free text:
// without it, fixing a typo means editing every mini that carries it.
function projectDialog(p) {
  openModal(`
    <header><h2>${esc(p.name)}</h2><button class="close">✕</button></header>
    <div class="mbody">
      <div class="grid2">
        <div class="field"><label>Name</label>
          <input type="text" id="pr-name" value="${esc(p.name)}">
          <div class="hint">Renaming re-tags all ${plural(p.entries, "entry", "entries")}.</div>
        </div>
        <div class="field"><label>Deadline <span class="opt">(optional)</span></label>
          <input type="date" id="pr-due" value="${esc(p.due)}"></div>
      </div>
      <div class="field"><label>Notes</label>
        <textarea id="pr-notes" placeholder="What this is for, what's left…">${esc(p.notes)}</textarea></div>
      <div class="note">Clearing the name ungroups these minis without deleting
        anything — they stay exactly as they are, just filed under nothing.</div>
    </div>
    <footer>
      <div class="spacer"></div>
      <button class="btn ghost" id="pr-cancel">Cancel</button>
      <button class="btn" id="pr-save">Save</button>
    </footer>`);

  $("#pr-name").focus();
  $("#pr-cancel").onclick = closeModal;
  $("#pr-save").onclick = async () => {
    const name = $("#pr-name").value.trim();
    // The rename has to land first: the metadata is keyed by name, and
    // saving it under the old one would leave it behind on a project that
    // no longer exists.
    if (name !== p.name) {
      const moved = await call(App().RenameProject, p.name, name);
      if (!name) {
        closeModal(); renderProjects();
        toast(`Ungrouped ${plural(moved, "mini", "minis")}`);
        return;
      }
    }
    await call(App().SaveProject, {
      name, due: $("#pr-due").value, notes: $("#pr-notes").value.trim(),
    });
    closeModal(); renderProjects(); toast("Saved");
  };
}

/* ===================================================================== TIME */
// The dashboard reports time as one ever-growing total, which stops being
// interesting the moment it's large. This is the same log asked the questions
// that actually change: how much this month, how long a mini takes, where the
// hours went. Every figure here is derived - nothing extra is recorded.
async function renderTime() {
  const r = await call(App().TimeReport);

  if (!r.sessions) {
    setContent(`
      <div class="page-head"><div><h1>Time at the Desk</h1>
        <div class="sub">What the painting log adds up to</div></div></div>
      <div class="card"><div class="empty"><strong>No sessions logged yet.</strong>
        Open a mini, go to the Painting log tab, and write down what you got
        done. Add the minutes and this screen fills itself in.</div></div>`);
    return;
  }

  const cards = [
    [duration(r.thisMonth) || "—", "This month", "#2f7d8a"],
    [duration(r.last30) || "—", "Last 30 days", "#0ea5e9"],
    [duration(r.total), "All time", STATUS_COLORS["Complete"]],
    [plural(r.days, "day"), "Days at the desk", "#8b5cf6"],
    [duration(r.perSession) || "—", "Average session", "#f59e0b"],
    [duration(r.perMini) || "—", "Average a mini", "#c07b1f"],
  ].map(([n, l, c]) => `
    <div class="card stat">
      <div class="bar" style="background:${c}"></div>
      <div><div class="n">${n}</div><div class="l">${l}</div></div>
    </div>`).join("");

  // The tallest month sets the scale. An empty month keeps a sliver of bar so
  // the year reads as twelve columns rather than a gap with some bars in it.
  const peak = Math.max(1, ...r.months.map(m => m.minutes));
  const bars = r.months.map(m => `
    <div class="col">
      <div class="val" style="color:${m.minutes ? "var(--text)" : "var(--faint)"}">${
        duration(m.minutes) || "—"}</div>
      <div class="bar" style="height:${m.minutes ? Math.max(6, Math.round(m.minutes / peak * 100)) : 2}%;
           background:${m.minutes ? "#2f7d8a" : "#e3e8ee"}"></div>
      <div class="lbl">${esc(m.label)}</div>
    </div>`).join("");

  const busiest = r.busiest.map(b => `
    <div class="mini-row time-jump" data-model="${b.id}" style="cursor:pointer">
      <span class="name">${esc(b.name)}${
        b.count > 1 ? `<span class="qty">×${b.count}</span>` : ""}</span>
      <span class="date">${b.count > 1 ? `${duration(b.perMini)} a mini · ` : ""}${
        duration(b.minutes)}</span>
    </div>`).join("");

  setContent(`
    <div class="page-head"><div><h1>Time at the Desk</h1>
      <div class="sub">${plural(r.sessions, "session")} logged across ${
        plural(r.days, "day")}</div></div></div>
    <div class="stats">${cards}</div>
    <div class="card" style="margin-top:16px">
      <h2>The last twelve months</h2>
      <div class="chart tall">${bars}</div>
    </div>
    <div class="card" style="margin-top:16px">
      <h2>Where the hours went</h2><div class="divider"></div>
      <div style="padding:6px 0 10px">${busiest}</div>
    </div>`);

  content().querySelectorAll(".time-jump").forEach(el => {
    el.onclick = () => jumpToMini(+el.dataset.model);
  });
}

/* ================================================================= WISHLIST */
// The wishlist flag has existed since the rack did, reachable only as one
// option in the inventory's stock filter - a list you could see but never
// take anywhere. This is the list itself: grouped the way a shop is, and
// copyable, because the point of a shopping list is to leave the house.
//
// The second half is the query only this app can answer. A paint recorded on
// a mini but not owned is one you borrowed, used at a club, or finished, and
// it belongs on the list before you reach for the empty pot.
async function renderWishlist() {
  const page = await call(App().Wishlist);

  const groups = new Map();
  page.rows.forEach(p => {
    const brand = p.brand || "No brand";
    if (!groups.has(brand)) groups.set(brand, []);
    groups.get(brand).push(p);
  });

  const row = (p, actions) => `
    <div class="wrow">
      <span class="swatch" style="background:${esc(p.hex)}"></span>
      <span class="nm" data-paint="${p.id}">${esc(p.name)}${
        p.code ? ` <span class="code">${esc(p.code)}</span>` : ""}</span>
      <span class="meta">${esc([p.range, p.type].filter(Boolean).join(" · "))}${
        p.usedOn ? ` · on ${plural(p.usedOn, "mini", "minis")}` : ""}</span>
      ${actions}
    </div>`;

  const listed = groups.size ? [...groups].map(([brand, rows]) => `
    <div class="wgroup">
      <div class="whead">${esc(brand)}<span>${plural(rows.length, "paint")}</span></div>
      ${rows.map(p => row(p, `
        <button class="btn ghost small got" data-got="${p.id}">Got it</button>
        <span class="x" data-drop="${p.id}" title="Take off the list">✕</span>`)).join("")}
    </div>`).join("")
    : `<div class="card"><div class="empty"><strong>Nothing on the list.</strong>
         Tick “Wishlist” on any paint — or take one of the suggestions below.</div></div>`;

  const missing = page.missing.length ? `
    <div class="card" style="margin-top:16px">
      <h2>Used on your minis but not owned</h2>
      <div class="sub" style="margin:2px 0 0">
        Recorded on something you've painted, with no pot in the rack —
        borrowed, used at a club, or run dry.</div>
      <div class="divider"></div>
      ${page.missing.map(p => row(p, `
        <button class="btn ghost small want" data-want="${p.id}">Add to list</button>`)).join("")}
      <div style="padding:10px 0 2px">
        <button class="btn ghost" id="w-all">Add all ${page.missing.length}</button>
      </div>
    </div>` : "";

  setContent(`
    <div class="page-head">
      <div><h1>Wishlist</h1>
        <div class="sub">${page.rows.length
          ? `${plural(page.rows.length, "paint")} to buy` : "Nothing to buy"}${
          page.missing.length ? ` · ${page.missing.length} suggested` : ""}</div></div>
      <div class="spacer"></div>
      ${page.rows.length ? `<button class="btn" id="w-copy">Copy list</button>` : ""}
    </div>
    <div class="card">${listed}</div>
    ${missing}`);

  const flags = async (id, owned, want, msg) => {
    await call(App().SetPaintFlags, id, owned, want);
    if (msg) toast(msg);
    renderWishlist();
  };
  content().querySelectorAll("[data-got]").forEach(b => {
    // Buying it is the end of the list entry, so ticking it off does both
    // halves at once rather than leaving it owned and still wanted.
    b.onclick = () => flags(+b.dataset.got, true, false, "Added to the rack");
  });
  content().querySelectorAll("[data-drop]").forEach(b => {
    const p = page.rows.find(x => x.id === +b.dataset.drop);
    b.onclick = () => flags(+b.dataset.drop, !!(p && p.owned), false);
  });
  content().querySelectorAll("[data-want]").forEach(b => {
    b.onclick = () => flags(+b.dataset.want, false, true);
  });
  content().querySelectorAll(".wrow .nm").forEach(el => {
    el.onclick = () => {
      const p = [...page.rows, ...page.missing].find(x => x.id === +el.dataset.paint);
      if (p) paintDialog(p);
    };
  });

  const all = $("#w-all");
  if (all) all.onclick = async () => {
    for (const p of page.missing) await call(App().SetPaintFlags, p.id, false, true);
    toast(`Added ${plural(page.missing.length, "paint")}`);
    renderWishlist();
  };
  const copy = $("#w-copy");
  if (copy) copy.onclick = async () => {
    const lines = [`Sablewright wishlist — ${prettyDate(todayISO())}`, ""];
    groups.forEach((rows, brand) => {
      lines.push(brand);
      rows.forEach(p => lines.push(`  ${p.name}${
        p.range ? ` — ${p.range}` : ""}${p.code ? ` (${p.code})` : ""}`));
      lines.push("");
    });
    await call(App().CopyText, lines.join("\n").trim() + "\n");
    toast("Wishlist copied");
  };
}

/* ===================================================================== TIPS */
async function renderTips() {
  const f = state.tips;
  const [tips, allPaints] = await Promise.all([
    call(App().ListTips, { search: f.search, category: f.category }),
    call(App().AllPaints),
  ]);
  const byId = new Map((allPaints || []).map(p => [p.id, p]));

  const cards = tips.length ? `<div class="tip-grid">${tips.map(t => {
    const col = TIP_COLORS[t.category] || "#6b7280";
    const lines = (t.body || "").split("\n").filter(l => l.trim()).slice(0, 6);
    return `<div class="card tip" data-id="${t.id}">
      <div class="stripe" style="background:${col}"></div>
      <div class="body">
        <div class="head"><h3>${esc(t.title)}</h3>
          <span class="badge" style="background:${col}">${esc(t.category)}</span></div>
        <div class="notes">${lines.map(l => `<p>${esc(l.trim())}</p>`).join("")}</div>
        ${paintChips(t.paintIds, byId)}
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

/* ---------------------------------------------------------- paint picker */
// Two dialogs pick paints the same way - the ones used on a mini, and the
// ones a recipe calls for - so the control is built once here. The caller
// owns the state; this turns it into markup and says what was clicked.
//
// prefix namespaces the element ids, because the screen behind the dialog
// keeps its own filter bar in the document and an id shared with it resolves
// to whichever came first.
function paintPicker(prefix, allPaints, chosen, filter, note) {
  if (!allPaints.length) return `<div class="note">${note.empty}</div>`;

  const q = filter.search.toLowerCase();
  const brands = [...new Set(allPaints.map(p => p.brand).filter(Boolean))]
    .sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
  // The rack holds the whole catalogue, so float the paints that are actually
  // on the desk - owned, or already ticked - to the top.
  const matched = allPaints
    .filter(p => filter.brand === "All brands" || p.brand === filter.brand)
    .filter(p => !q || (p.name + " " + p.brand + " " + p.range + " " + p.code)
      .toLowerCase().includes(q))
    .sort((a, b) => (chosen.has(b.id) - chosen.has(a.id)) || (b.owned - a.owned));
  const list = matched.slice(0, PICKER_LIMIT);

  return `
    <div class="filters">
      ${searchBox(`${prefix}-psearch`, "Filter paints…", filter.search)}
      ${selectBox(`${prefix}-pbrand`, ["All brands", ...brands], filter.brand)}
    </div>
    <div style="color:var(--muted);font-size:13px;margin:0 0 8px">
      ${note.lead} Ones you own are listed first${
        matched.length > list.length
          ? `, and only the first ${list.length} of ${matched.length} are shown —
             keep typing to narrow it down` : ""}.${
        chosen.size ? ` ${plural(chosen.size, "paint")} ticked so far —
          narrowing the filters hides rows but never unticks them.` : ""}</div>
    <div class="picker" id="${prefix}-picker">${list.map(p => `
      <label><input type="checkbox" data-pid="${p.id}"${chosen.has(p.id) ? " checked" : ""}>
        <span class="swatch" style="background:${esc(p.hex)};width:16px;height:16px"></span>
        <span>${esc(p.name)}</span>
        <span class="meta">${esc(p.brand)}${p.range ? " " + esc(p.range) : ""} · ${
          esc(p.type)}${p.owned ? "" : " · not owned"}</span></label>`).join("")}</div>`;
}

// onFilter re-renders the dialog around the picker; onToggle is handed the
// paint that was ticked or unticked and the state it landed in.
function wirePaintPicker(prefix, filter, onFilter, onToggle) {
  const search = $(`#${prefix}-psearch`);
  if (!search) return; // an empty rack draws the note instead
  search.oninput = debounce(() => { filter.search = search.value; onFilter(); }, 150);
  $(`#${prefix}-pbrand`).onchange = e => { filter.brand = e.target.value; onFilter(); };
  document.querySelectorAll(`#${prefix}-picker input`).forEach(cb => {
    cb.onchange = () => onToggle(+cb.dataset.pid, cb.checked);
  });
}

// The paints on a mini or in a recipe, drawn as swatches. ids are resolved
// against the rack rather than stored alongside the name, so renaming a paint
// or correcting its color updates every place it is used.
function paintChips(ids, byId) {
  const chosen = (ids || []).map(id => byId.get(id)).filter(Boolean);
  if (!chosen.length) return "";
  return `<div class="chips">${chosen.map(p => `
    <span class="chip"><span class="swatch" style="background:${esc(p.hex)}"></span>${
      esc(p.name)}</span>`).join("")}</div>`;
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
    id: 0, name: "", gameSystem: "", faction: "", project: "", status: "Backlog",
    count: 1, done: 0, favorite: false, notes: "",
    started: new Date().toISOString().slice(0, 10),
    completed: "", paintIds: [], photos: [],
  };
  let tab = opts.tab || "details";
  const paintFilter = { search: "", brand: "All brands" };
  // the log entry currently being edited, {} = new
  let editSession = opts.session ? { ...opts.session } : {};
  const [allPaints, facets] = await Promise.all([
    call(App().AllPaints), call(App().ModelFacets),
  ]);

  function render() {
    const chosen = new Set(m.paintIds || []);

    const details = `
      <div class="field"><label>Name</label>
        <input type="text" id="f-name" value="${esc(m.name)}" autofocus></div>
      <div class="grid3">
        <div class="field"><label>Game system</label>
          ${suggestBox("f-sys", m.gameSystem, facets.systems, "e.g. Warhammer 40,000")}</div>
        <div class="field"><label>Faction / unit</label>
          ${suggestBox("f-fac", m.faction, facets.factions, "e.g. Death Guard")}</div>
        <div class="field"><label>Project <span class="opt">(optional)</span></label>
          ${suggestBox("f-proj", m.project, facets.projects, "e.g. Tournament list")}</div>
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

    const paintsTab = paintPicker("f", allPaints, chosen, paintFilter, {
      lead: "Tick every paint you used on this mini.",
      empty: `Your paint rack is empty. Add some paints in Paint Inventory
              first, then come back and tick the ones you used.`,
    });

    const photoTile = p => `
        <div class="photo">
          <img src="${photoSrc(p)}" alt="" loading="lazy">
          <div class="cap">
            <span class="badge" style="background:${
              photoColor(p.kind)};font-size:10px">${esc(p.kind)}</span>
            <span class="pick${p.cover ? " on" : ""}" data-cover="${p.id}"
              title="Use this as the cover in the list">${p.cover ? "★" : "☆"}</span>
            <span class="x" data-photo="${p.id}">✕</span></div>
        </div>`;

    // Grouped rather than run together: the reference is the maker's
    // photograph and everything else is the painter's own work, and a gallery
    // that mixes the two invites them being read as the same thing. A group
    // with nothing in it is left out entirely rather than drawn empty.
    const photoSections = [
      ["Product", "Reference", `The maker's painted example, to work towards. Stands in as
        the cover in the list until this mini is finished, and is left out of exports.`],
      ["Progress", "Progress", ""],
      ["Final", "Final", ""],
    ].map(([kind, title, blurb]) => {
      const inKind = (m.photos || []).filter(p => photoGroup(p) === kind);
      if (!inKind.length) return "";
      return `<div class="section">${title.toUpperCase()}</div>${
        blurb ? `<div class="note" style="margin-bottom:10px">${blurb}</div>` : ""
      }<div class="photos" style="margin-bottom:16px">${inKind.map(photoTile).join("")}</div>`;
    }).join("");

    const photosTab = `
      ${isNew && !m.id ? `<div class="note">Photos are saved straight to disk, so this
        mini needs a name and a save first. Add a photo and it'll be saved automatically.</div>` : ""}
      <div style="display:flex;gap:8px;margin-bottom:14px;flex-wrap:wrap">
        <button class="btn ghost" id="add-product">+&nbsp; Add product image</button>
        <button class="btn ghost" id="add-prog">+&nbsp; Add progress photo</button>
        <button class="btn ghost" id="add-final">+&nbsp; Add final photo</button>
      </div>
      ${photoSections || `<div style="color:var(--muted);font-size:13px">
          No photos yet. Add the maker's product image so this mini is recognisable in
          the list, or a progress shot to start the record.</div>`}`;

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
        `Delete “${m.name}”?\nIt goes to the trash for 30 days, photos and all.`,
        "Delete");
      if (!ok) return;
      const gone = m.id, name = m.name;
      await call(App().DeleteModel, gone);
      // Nothing left to log the session against.
      if (timer.modelId === gone) clearTimer();
      closeModal(); state.models.selected = null; show(state.screen);
      toast(`Deleted “${name}”`, { label: "Undo", run: async () => {
        await call(App().UndoDeleteModel, gone);
        state.models.selected = gone;
        show(state.screen);
        toast("Put back");
      } });
    };

    // A tab switch leaves focus on the tab button, so drop the caret somewhere
    // useful. A re-render mid-typing has already put it back, and that wins.
    if (!$("#modal").contains(document.activeElement)) {
      const first = tab === "details" ? $("#f-name")
                  : tab === "paints" ? $("#f-psearch") : null;
      if (first) first.focus();
    }

    if (tab === "paints") {
      wirePaintPicker("f", paintFilter, () => { collect(); render(); }, (id, on) => {
        const set = new Set(m.paintIds || []);
        on ? set.add(id) : set.delete(id);
        m.paintIds = [...set];
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
      $("#add-product").onclick = () => addPhotos("Product");
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
      m.project = g("f-proj").trim();
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

  // "Used on 3 minis" is a dead number: which three is the question actually
  // being asked, and it is answerable from what's already recorded. Same for
  // the recipes - a paint is a way into the notes that call for it.
  const found = isNew ? { minis: [], tips: [] } : await call(App().PaintLinks, p.id);
  const linkList = (items, cls, label) => !items.length ? "" : `
    <div class="section">${label}</div>
    <div class="links">${items.map(x => `
      <span class="link ${cls}" data-id="${x.id}">${esc(x.name || x.title)}${
        x.count > 1 ? `<span class="qty">×${x.count}</span>` : ""}</span>`).join("")}</div>`;
  const links = isNew ? "" : `
    ${linkList(found.minis, "to-mini", `USED ON ${plural(found.minis.length, "MINI", "MINIS")}`)}
    ${linkList(found.tips, "to-tip", `IN ${plural(found.tips.length, "RECIPE")}`)}
    ${!found.minis.length && !found.tips.length
      ? `<div style="color:var(--muted);font-size:13px;margin-top:14px">
           Not recorded on any mini or in any recipe yet.</div>` : ""}`;

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
      ${links}
    </div>
    <footer>
      ${!isNew ? `<button class="btn danger" id="p-del">Delete paint</button>` : ""}
      <div class="spacer"></div>
      <button class="btn ghost" id="p-cancel">Cancel</button>
      <button class="btn" id="p-save">Save</button>
    </footer>`);

  $("#p-name").focus();
  // Following a link leaves the paint behind, so nothing here is saved on the
  // way out - the dialog closes exactly as Cancel would.
  $("#modal").querySelectorAll(".link.to-mini").forEach(el => {
    el.onclick = () => { closeModal(); jumpToMini(+el.dataset.id); };
  });
  $("#modal").querySelectorAll(".link.to-tip").forEach(el => {
    el.onclick = async () => {
      const tip = await call(App().GetTip, +el.dataset.id);
      closeModal();
      if (tip) tipDialog(tip);
    };
  });

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
// A recipe written as prose is a recipe you can't search by paint, so the
// note carries the actual paints alongside the words. The dialog re-renders
// to filter that picker, which means the text fields have to be read back
// into `t` first - the same collect-before-render the mini dialog does.
async function tipDialog(tip) {
  const isNew = !tip;
  const t = tip ? { ...tip } : {
    id: 0, title: "", category: "Other", body: "", tags: [], paintIds: [],
  };
  const paintFilter = { search: "", brand: "All brands" };
  const allPaints = await call(App().AllPaints);

  function collect() {
    const g = id => { const e = $("#" + id); return e ? e.value : undefined; };
    if (g("t-title") === undefined) return;
    t.title = g("t-title").trim();
    t.category = g("t-cat-f");
    t.body = g("t-body");
    t.tags = g("t-tags").split(",").map(s => s.trim()).filter(Boolean);
  }

  function render() {
    const chosen = new Set(t.paintIds || []);
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
        <div class="section">PAINTS IN THIS RECIPE${
          chosen.size ? ` · ${plural(chosen.size, "paint")}` : ""}</div>
        ${paintPicker("t", allPaints, chosen, paintFilter, {
          lead: "Tick the paints this recipe calls for.",
          empty: `Your paint rack is empty. Add some paints in Paint Inventory
                  first, then come back and tick the ones this recipe uses.`,
        })}
      </div>
      <footer>
        ${!isNew ? `<button class="btn danger" id="t-del">Delete note</button>` : ""}
        <div class="spacer"></div>
        <button class="btn ghost" id="t-cancel">Cancel</button>
        <button class="btn" id="t-save">Save</button>
      </footer>`);

    if (!$("#modal").contains(document.activeElement)) $("#t-title").focus();

    wirePaintPicker("t", paintFilter, () => { collect(); render(); }, (id, on) => {
      const set = new Set(t.paintIds || []);
      on ? set.add(id) : set.delete(id);
      t.paintIds = [...set];
    });

    $("#t-cancel").onclick = closeModal;
    $("#t-save").onclick = async () => {
      collect();
      await call(App().SaveTip, t);
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

  render();
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
  initKeys();
  // The build names itself rather than carrying a string the source has to
  // remember to update - the linker stamps it in, so it can't go stale.
  App().Version().then(v => { $("#version").textContent = v; });
  const warn = await App().StartupError();
  if (warn) toast(warn);
  show("dashboard");
});
