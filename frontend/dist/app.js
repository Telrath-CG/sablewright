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

const MAGNIFIER =
  '<svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">' +
  '<circle cx="7" cy="7" r="5" fill="none" stroke="#98a2ae" stroke-width="2"/>' +
  '<path d="M11 11l4 4" stroke="#98a2ae" stroke-width="2" stroke-linecap="round"/></svg>';

const state = {
  screen: "dashboard",
  models: { search: "", status: "All", sort: "Recent", selected: null },
  paints: { search: "", type: "All types", brand: "All brands",
            range: "All ranges", owned: "All" },
  tips:   { search: "", category: "All" },
};

/* ------------------------------------------------------------------ utils */
const $ = (sel, root = document) => root.querySelector(sel);
const content = () => $("#content");

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

function selectBox(id, options, value) {
  return `<select id="${id}">` +
    options.map(o => `<option${o === value ? " selected" : ""}>${esc(o)}</option>`).join("") +
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
    if (ok) { toast("Backup imported"); show(state.screen); }
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

  content().innerHTML = `
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
    </div>`;

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
  const models = await call(App().ListModels, { search: f.search, status: f.status, sort: f.sort });

  if (models.length && !models.some(m => m.id === f.selected)) f.selected = models[0].id;
  if (!models.length) f.selected = null;

  const rows = models.length ? models.map(m => `
    <div class="row${m.id === f.selected ? " sel" : ""}" data-id="${m.id}">
      <div>
        <div class="nm">${esc(m.name)}</div>
        <div class="sub">${esc([m.gameSystem, m.faction].filter(Boolean).join(" · ") || "—")}</div>
      </div>
      <div>${badge(m.status)}</div>
      <div class="count">${(m.paintIds || []).length}${m.favorite ? ' <span class="star">★</span>' : ""}</div>
    </div>`).join("")
    : `<div class="empty"><strong>No minis yet.</strong>Click “+ Add Mini” to start your collection.</div>`;

  content().innerHTML = `
    <div class="page-head">
      <div><h1>Models</h1><div class="sub">Your collection — ${plural(models.length, "mini")}</div></div>
      <div class="spacer"></div>
      <button class="btn" id="add-model">+&nbsp; Add Mini</button>
    </div>
    <div class="filters">
      ${searchBox("m-search", "Search minis…", f.search)}
      ${selectBox("m-status", ["All", ...STATUSES], f.status)}
      ${selectBox("m-sort", ["Recent", "Name", "Status", "Favourites"], f.sort)}
    </div>
    <div class="split">
      <div class="card list-pane">
        <div class="list-head"><div>NAME</div><div>STATUS</div><div class="r">PAINTS</div></div>
        <div class="divider"></div>
        <div class="rows" id="m-rows">${rows}</div>
      </div>
      <div class="card detail-pane" id="m-detail"></div>
    </div>`;

  const search = $("#m-search");
  search.oninput = debounce(() => { f.search = search.value; renderModels(); }, 180);
  $("#m-status").onchange = e => { f.status = e.target.value; renderModels(); };
  $("#m-sort").onchange = e => { f.sort = e.target.value; renderModels(); };
  $("#add-model").onclick = () => modelDialog(null);

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
      <img src="/photos/${encodeURIComponent(p.file)}" alt="" data-file="${esc(p.file)}">
      <div class="cap"><span class="badge" style="background:${
        p.kind === "Final" ? STATUS_COLORS["Complete"] : "#64748b"};font-size:10px">${esc(p.kind)}</span></div>
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
        ${badge(m.status)}<span style="flex:1"></span>
        <button class="btn ghost small" id="edit">Edit</button>
      </div>
      <div class="section">PHOTOS</div><div class="photos">${photos}</div>
      <div class="section">PAINTS USED</div>${chips}
      <div class="section">TECHNIQUE NOTES</div>${noteHtml}
      <div class="section">PAINTING LOG${
        sessions.length ? ` · ${plural(sessions.length, "session")}${
          totalMins ? ` · ${duration(totalMins)}` : ""}` : ""}</div>${logHtml}
      <div class="detail-foot">
        <span>Started: ${prettyDate(m.started)}</span>
        <span>Finished: ${m.completed ? prettyDate(m.completed) : "in progress"}</span>
      </div>
    </div>`;

  $("#edit").onclick = () => modelDialog(m);
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
    brand: f.brand, range: f.range, owned: f.owned, limit: ROW_LIMIT });
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
      <span class="r ${p.owned ? "yes" : "unowned"}">${p.owned ? "✓ Yes" : "☆ No"}</span>
    </div>`).join("") + (page.total > shown.length
      ? `<div class="empty">Showing the first ${shown.length} of ${page.total}.
           Search or narrow the filters to see the rest.</div>` : "")
    : (facets.total
        ? `<div class="empty">No paints match those filters.</div>`
        : `<div class="empty"><strong>Your paint rack is empty.</strong>
             Add paints one at a time, or put the built-in ranges back:
             <div style="margin-top:12px"><button class="btn ghost" id="restore-lib">
               Restore built-in paints</button></div></div>`);

  content().innerHTML = `
    <div class="page-head">
      <div><h1>Paint Inventory</h1>
        <div class="sub">${plural(facets.total, "paint")} listed · ${facets.owned} owned${
          page.total !== facets.total ? ` — ${page.total} match` : ""}</div></div>
      <div class="spacer"></div>
      <button class="btn" id="add-paint">+&nbsp; Add Paint</button>
    </div>
    <div class="filters">
      ${searchBox("p-search", "Search paints, ranges, codes…", f.search)}
      ${selectBox("p-brand", ["All brands", ...facets.brands], f.brand)}
      ${selectBox("p-range", ["All ranges", ...facets.ranges], f.range)}
      ${selectBox("p-type", ["All types", ...PAINT_TYPES], f.type)}
      ${selectBox("p-owned", ["All", "Owned only", "Not owned"], f.owned)}
    </div>
    <div class="card ptable">
      <div class="thead"><span></span><span>NAME</span><span>BRAND</span>
        <span>TYPE</span><span class="r">USED ON</span><span class="r">IN STOCK</span></div>
      <div class="divider"></div>
      <div>${body}</div>
    </div>`;

  const search = $("#p-search");
  search.oninput = debounce(() => { f.search = search.value; renderPaints(); }, 180);
  $("#p-type").onchange = e => { f.type = e.target.value; renderPaints(); };
  $("#p-brand").onchange = e => { f.brand = e.target.value; renderPaints(); };
  $("#p-range").onchange = e => { f.range = e.target.value; renderPaints(); };
  $("#p-owned").onchange = e => { f.owned = e.target.value; renderPaints(); };
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
    : `<div class="card"><div class="empty"><strong>No tips yet.</strong>
         Save your recipes here so you can find them again.</div></div>`;

  content().innerHTML = `
    <div class="page-head">
      <div><h1>Technique Tips</h1>
        <div class="sub">Your painting recipes &amp; methods — searchable</div></div>
      <div class="spacer"></div>
      <button class="btn" id="add-tip">+&nbsp; Add Tip</button>
    </div>
    <div class="filters">
      ${searchBox("t-search", "Search tips, tags, recipes…", f.search)}
      ${selectBox("t-cat", ["All", ...TIP_CATEGORIES], f.category)}
    </div>
    ${cards}`;

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
  $("#modal").innerHTML = html;
  $("#modal-backdrop").hidden = false;
  const close = $("#modal .close");
  if (close) close.onclick = closeModal;
  document.addEventListener("keydown", escClose);
}
function closeModal() {
  $("#modal-backdrop").hidden = true;
  $("#modal").innerHTML = "";
  document.removeEventListener("keydown", escClose);
}
function escClose(e) { if (e.key === "Escape") closeModal(); }

/* ---- mini ---- */
async function modelDialog(model) {
  const isNew = !model;
  let m = model ? { ...model } : {
    id: 0, name: "", gameSystem: "", faction: "", status: "Backlog",
    favorite: false, notes: "", started: new Date().toISOString().slice(0, 10),
    completed: "", paintIds: [], photos: [],
  };
  let tab = "details";
  let paintSearch = "";
  let editSession = {};   // the log entry currently being edited, {} = new
  const allPaints = await call(App().AllPaints);

  function render() {
    const chosen = new Set(m.paintIds || []);
    const q = paintSearch.toLowerCase();
    // The rack holds the whole catalogue, so float the paints that are
    // actually on the desk — owned, or already ticked — to the top.
    const matched = allPaints
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
        <div class="field"><label>Started</label>
          <input type="date" id="f-started" value="${esc(m.started)}"></div>
        <div class="field"><label>Finished</label>
          <input type="date" id="f-done" value="${esc(m.completed)}"></div>
      </div>
      <label class="check" style="margin-bottom:14px">
        <input type="checkbox" id="f-fav"${m.favorite ? " checked" : ""}> Favourite</label>
      <div class="field"><label>Technique notes for this mini</label>
        <textarea id="f-notes" placeholder="One step per line…">${esc(m.notes)}</textarea></div>`;

    const paintsTab = allPaints.length ? `
      ${searchBox("f-psearch", "Filter paints…", paintSearch)}
      <div style="color:var(--muted);font-size:13px;margin:10px 0 8px">
        Tick every paint you used on this mini. Ones you own are listed first${
          matched.length > list.length
            ? `, and only the first ${list.length} of ${matched.length} are shown —
               keep typing to narrow it down` : ""}.</div>
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
          <img src="/photos/${encodeURIComponent(p.file)}" alt="">
          <div class="cap">
            <span class="badge" style="background:${
              p.kind === "Final" ? STATUS_COLORS["Complete"] : "#64748b"};font-size:10px">${esc(p.kind)}</span>
            <span class="x" data-photo="${p.id}">✕</span></div>
        </div>`).join("") || `<div style="color:var(--muted);font-size:13px">
          No photos yet. Add a progress shot or a final picture.</div>`}</div>`;

    const sess = m.sessions || [];
    const logTab = `
      ${!m.id ? `<div class="note">Log entries save straight away, so this mini needs
        a name first — add an entry and it'll be saved automatically.</div>` : ""}
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
      closeModal(); state.models.selected = null; renderModels();
    };

    if (tab === "details") $("#f-name").focus();

    if (tab === "paints" && allPaints.length) {
      const ps = $("#f-psearch");
      ps.oninput = debounce(() => { collect(); paintSearch = ps.value; render(); }, 150);
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
    const mins = parseInt($("#s-mins").value.replace(/[^0-9]/g, ""), 10);
    const updated = await call(App().SaveSession, m.id, {
      id: editSession.id || 0,
      date: $("#s-date").value || todayISO(),
      minutes: isNaN(mins) ? 0 : mins,
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
    renderModels();
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
    hex: "#888888", owned: true, notes: "",
  };
  // suggestions = brands already in the collection, plus the common ones,
  // but the field is free text so any brand can simply be typed in
  const facets = await call(App().Facets, p.brand || "");
  const suggestions = [...new Set([...facets.brands, ...KNOWN_BRANDS])].sort((a, b) =>
    a.toLowerCase().localeCompare(b.toLowerCase()));
  const ranges = facets.ranges;

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
      <label class="check"><input type="checkbox" id="p-owned"${p.owned ? " checked" : ""}>
        I own this paint (untick and it stays listed as one you don't)</label>
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
    await call(App().SavePaint, {
      ...p,
      name: $("#p-name").value.trim(),
      brand: $("#p-brand-f").value.trim(),
      range: $("#p-range-f").value.trim(),
      code: $("#p-code-f").value.trim(),
      type: $("#p-type-f").value,
      hex: hex.value.trim(),
      owned: $("#p-owned").checked,
    });
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
    <header><h2>${isNew ? "Add Technique Tip" : "Edit Tip"}</h2><button class="close">✕</button></header>
    <div class="mbody">
      <div class="field"><label>Title</label>
        <input type="text" id="t-title" value="${esc(t.title)}"></div>
      <div class="field"><label>Category</label>${selectBox("t-cat-f", TIP_CATEGORIES, t.category)}</div>
      <div class="field"><label>The tip / recipe (one step per line)</label>
        <textarea id="t-body" style="min-height:150px">${esc(t.body)}</textarea></div>
      <div class="field"><label>Tags (comma separated)</label>
        <input type="text" id="t-tags" value="${esc((t.tags || []).join(", "))}"></div>
    </div>
    <footer>
      ${!isNew ? `<button class="btn danger" id="t-del">Delete tip</button>` : ""}
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
    const ok = await call(App().Confirm, "Delete tip", `Delete “${t.title}”?`, "Delete");
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
  const warn = await App().StartupError();
  if (warn) toast(warn);
  show("dashboard");
});
