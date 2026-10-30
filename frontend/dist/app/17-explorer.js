// 17-explorer.js — Explorateur de fichiers UNIQUE (1.18.3)
// Tronc commun de TOUTES les navigations d'arbre (fichiers git, cloud CORPUS, sélection de fichier
// run, droits IA). Anatomie de rangée invariante, un seul fil d'Ariane, un seul filtre, le registre
// d'icônes unique (ico()), des états vide/chargement unifiés, et la réactivité (instantané + shimmer).
//
// mountExplorer(container, opts) — opts :
//   load(path) -> Promise<[{name, path, type:"dir"|"file", size?, date?}]>   REQUIS (source autoritaire)
//   mode        "browse" (défaut) | "multi" | "single" | "rights"
//   root        libellé du fil d'Ariane racine (défaut "racine")
//   startPath   chemin initial ("")
//   instant(path) -> Promise<entries>   source RAPIDE/LOCALE peinte d'abord (ex. clone local) — option
//   dates(entries) -> Promise<map path->ISO>   dates de dernière publication, remplies en différé (shimmer)
//   cacheKey(path) -> string            clé swr (sinon pas de cache) ; swrPersist (défaut true)
//   onOpenFile(entry) / onPick(entry)   clic fichier (browse) / sélection (single)
//   selectedSet() -> Set                chemins cochés (multi) / visibles (rights)
//   onToggle(entry, on)                 bascule case (multi) / œil (rights)
//   rowActions(entry) -> html / onAction(entry, action)   actions à droite (browse)
//   iconOf(entry) -> name ; metaOf(entry) -> html         surcharges d'affichage
//   filter (défaut true) ; emptyText
// Renvoie { goTo, refresh, el }.
function mountExplorer(container, opts) {
  opts = opts || {};
  const mode = opts.mode || "browse";
  const rootLabel = opts.root || "racine";
  const wantFilter = opts.filter !== false;
  let path = opts.startPath || "";
  let entries = [];
  let q = "";

  container.classList.add("xp");
  container.innerHTML = `
    <div class="xp-bar">
      <div class="xp-crumb" id="xp-crumb"></div>
      ${opts.totals ? `<span class="xp-total" id="xp-total"></span>` : ""}
      ${wantFilter ? `<div class="xp-search">${ico("search", 14)}<input type="text" id="xp-q" placeholder="Filtrer ce dossier…" autocomplete="off" aria-label="Filtrer" /></div>` : ""}
    </div>
    <div class="xp-list" id="xp-list"><div class="skeleton"></div></div>`;
  const listEl = () => container.querySelector("#xp-list");
  const crumbEl = () => container.querySelector("#xp-crumb");

  function crumb() {
    const segs = path ? path.split("/").filter(Boolean) : [];
    let acc = "", html = `<button class="xp-crumb-seg" data-p="">${ico("folder", 14)}<span>${esc(rootLabel)}</span></button>`;
    segs.forEach((s) => {
      acc = acc ? acc + "/" + s : s;
      html += `<span class="xp-crumb-sep">${ico("chevronRight", 12)}</span><button class="xp-crumb-seg" data-p="${esc(acc)}">${esc(s)}</button>`;
    });
    crumbEl().innerHTML = html;
    crumbEl().querySelectorAll(".xp-crumb-seg").forEach((b) => (b.onclick = () => goTo(b.dataset.p)));
  }

  function fmtDate(iso) {
    if (!iso) return "";
    const d = new Date(iso); if (isNaN(d)) return "";
    return d.toLocaleDateString() + " " + d.toLocaleTimeString().replace(/:\d\d$/, "");
  }

  function metaHTML(e) {
    if (opts.metaOf) return opts.metaOf(e);
    let size;
    if (e.size) size = `<span class="xp-size">${fmtSize(e.size)}</span>`;
    else if (e.type === "dir" && opts.totals) size = `<span class="xp-size" data-sizecell><span class="sl-cell-load" style="--w:52px"></span></span>`;
    else size = "";
    let date;
    if (e.date) date = `<span class="xp-date">${esc(fmtDate(e.date))}</span>`;
    else if (opts.dates) date = `<span class="xp-date" data-datecell><span class="sl-cell-load" style="--w:86px"></span></span>`;
    else date = "";
    return size + date;
  }

  function rowHTML(e, i) {
    const isDir = e.type === "dir";
    const iconName = opts.iconOf ? opts.iconOf(e) : (isDir ? "folder" : "file");
    let selector = "";
    if (mode === "multi") {
      const on = opts.selectedSet && opts.selectedSet().has(e.path);
      selector = `<label class="xp-check" title="Relier"><input type="checkbox" data-sel="${esc(e.path)}"${on ? " checked" : ""} /><span class="xp-tick">${ico("check", 12)}</span></label>`;
    } else if (mode === "rights") {
      const on = opts.selectedSet && opts.selectedSet().has(e.path);
      selector = `<button type="button" class="xp-eye${on ? " on" : ""}" data-eye="${esc(e.path)}" title="${on ? "Visible par l'IA" : "Masqué"}">${ico(on ? "eye" : "eyeOff", 15)}</button>`;
    }
    const acts = opts.rowActions ? `<span class="xp-acts">${opts.rowActions(e)}</span>` : "";
    const nav = (isDir && (mode !== "single" || true)) ? `<span class="xp-chev" data-nav="${esc(e.path)}">${ico("chevronRight", 15)}</span>` : "";
    return `<div class="xp-row" data-i="${i}" data-path="${esc(e.path)}" data-type="${e.type}" role="button" tabindex="0">
      ${selector}<span class="xp-ico">${ico(iconName, 17)}</span>
      <span class="xp-name">${esc(e.name)}</span>${e.shared ? ` <span class="shared-badge" title="Partagé avec vous">${ico("shared", 11)}</span>` : ""}
      <span class="xp-meta">${metaHTML(e)}</span>${acts}${nav}</div>`;
  }

  function paint(list, how) {
    entries = list || [];
    const box = listEl(); if (!box) return;
    const shown = q ? entries.filter((e) => e.name.toLowerCase().includes(q)) : entries;
    if (!shown.length) {
      box.innerHTML = `<div class="xp-empty">${esc(q ? "Aucun élément ne correspond au filtre." : (opts.emptyText || "Dossier vide."))}</div>`;
      return;
    }
    box.innerHTML = shown.map((e, i) => rowHTML(e, i)).join("");
    if (how && how.stale === false) box.classList.add("sl-settle");
    bindRows(box, shown);
    fillDates(shown);
    fillTotals(shown);
  }

  function bindRows(box, shown) {
    box.querySelectorAll(".xp-row").forEach((row) => {
      const e = shown[+row.dataset.i];
      const activate = () => {
        // Cliquer un dossier l'ouvre (tous modes). En multi/rights, le sélecteur (case/œil) reste une
        // cible distincte, non interceptée ici. Cliquer un fichier le choisit/ouvre selon le mode.
        if (e.type === "dir") return goTo(e.path);
        if (mode === "single" && opts.onPick) return opts.onPick(e);
        if (mode === "browse" && opts.onOpenFile) return opts.onOpenFile(e);
      };
      row.onclick = (ev) => {
        const nav = ev.target.closest("[data-nav]");
        if (nav) return goTo(nav.dataset.nav);
        if (ev.target.closest("[data-sel]") || ev.target.closest(".xp-check")) return;
        if (ev.target.closest("[data-eye]")) return;
        if (ev.target.closest("[data-action]")) return;
        activate();
      };
      row.onkeydown = (ev) => { if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); activate(); } };
    });
    box.querySelectorAll("[data-sel]").forEach((c) => (c.onchange = () => {
      const e = shown.find((x) => x.path === c.getAttribute("data-sel"));
      if (e && opts.onToggle) opts.onToggle(e, c.checked);
    }));
    box.querySelectorAll("[data-eye]").forEach((b) => (b.onclick = () => {
      const e = shown.find((x) => x.path === b.getAttribute("data-eye"));
      const on = !b.classList.contains("on");
      if (e && opts.onToggle) opts.onToggle(e, on);
      paint(entries); // repeint l'état des yeux
    }));
    if (opts.onAction) box.querySelectorAll("[data-action]").forEach((b) => (b.onclick = (ev) => {
      ev.stopPropagation();
      const row = b.closest(".xp-row"); const e = shown[+row.dataset.i];
      opts.onAction(e, b.getAttribute("data-action"));
    }));
  }

  async function fillDates(shown) {
    if (!opts.dates) return;
    const need = shown.filter((e) => !e.date).map((e) => e.path);
    if (!need.length) return;
    let map = {};
    try { map = (await opts.dates(need)) || {}; } catch (_) { return; }
    const box = listEl(); if (!box) return;
    shown.forEach((e, i) => {
      if (map[e.path]) e.date = map[e.path];
      const cell = box.querySelector(`.xp-row[data-i="${i}"] [data-datecell]`);
      if (cell) cell.innerHTML = e.date ? `<span class="sl-settle">${esc(fmtDate(e.date))}</span>` : "";
    });
  }

  async function fillTotals(shown) {
    if (!opts.totals) return;
    let t;
    try { t = await opts.totals(path); } catch (_) { return; }
    const box = listEl(); if (!box || !t) return;
    const sizes = t.sizes || {};
    shown.forEach((e, i) => {
      const cell = box.querySelector(`.xp-row[data-i="${i}"] [data-sizecell]`);
      if (!cell) return;
      if (e.type === "dir" && sizes[e.path]) { e.size = sizes[e.path]; cell.innerHTML = `<span class="sl-settle">${fmtSize(sizes[e.path])}</span>`; }
      else cell.innerHTML = "";
    });
    const tot = container.querySelector("#xp-total");
    if (tot) tot.innerHTML = `<span class="sl-settle">Total : <b>${fmtSize(t.total)}</b></span>`;
  }

  async function goTo(p) {
    path = p || "";
    if (opts.onNav) opts.onNav(path);
    q = ""; const qi = container.querySelector("#xp-q"); if (qi) qi.value = "";
    crumb();
    const box = listEl(); if (box) box.innerHTML = `<div class="skeleton"></div>`;
    const alive = () => container.isConnected;
    // 1) Source rapide/locale (ex. clone local) peinte d'abord, si rien encore à l'écran.
    if (opts.instant) {
      opts.instant(path).then((loc) => { if (alive() && path === p) paint(loc, { stale: true }); }).catch(() => {});
    }
    // 2) Source autoritaire via swr (cache persistant optionnel) ou appel direct.
    try {
      if (opts.cacheKey) {
        await swr(opts.cacheKey(path), () => opts.load(path),
          (list, how) => { if (alive() && path === p) paint(list, how); },
          { alive: () => alive() && path === p, persist: opts.swrPersist !== false });
      } else {
        const list = await opts.load(path);
        if (alive() && path === p) paint(list, { stale: false });
      }
    } catch (e) {
      const b = listEl(); if (b && !entries.length) b.innerHTML = `<div class="xp-empty">${esc(cleanErr(e))}</div>`;
    }
  }

  if (wantFilter) {
    const qi = container.querySelector("#xp-q");
    qi.oninput = () => { q = qi.value.trim().toLowerCase(); paint(entries); };
  }
  goTo(path);
  return { goTo, refresh: () => goTo(path), get path() { return path; }, el: container };
}
