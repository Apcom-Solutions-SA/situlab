/* ---------------- BACKUP : tunnel clé-en-main (v1.14) ----------------
 * Menu dédié. Un PLAN dit QUOI sauvegarder (générique = portée dynamique, ou ciblé = liste figée),
 * SOUS QUELLE FORME (bundle git léger / miroir de fichiers lisibles), OÙ (cloud + dossier), et QUAND
 * (manuel / à l'ouverture / à la fermeture). Tout se choisit : seul le NOM du plan se tape.
 * Un JOURNAL trace chaque exécution. Les plans hérités (mono-dépôt v1.10) restent lisibles/exécutables.
 */

const BK_SCOPES = [
  { k: "kernels", label: "Tous mes KERNEL" },
  { k: "spokes", label: "Tous mes spokes" },
  { k: "kernels_spokes", label: "KERNEL + spokes" },
  { k: "repos", label: "Tous mes dépôts" },
];
// Picto d'un rôle/scope : réutilise les SVG du menu (chargés au rendu). Jamais d'ampoule.
function bkPicto(role) {
  const N = (typeof NAV_ICO !== "undefined") ? NAV_ICO : {};
  if (role === "kernels" || role === "kernel") return N.kernel || "";
  if (role === "spokes" || role === "spoke") return N.spokes || "";
  if (role === "kernels_spokes") return (N.kernel || "") + (N.spokes || "");
  return N.git || "";
}

async function renderBackup() {
  const view = el("view");
  view.innerHTML = `
    <div class="view-head view-head-row"><div><div class="view-title">BACKUP</div>
      <div class="view-sub">Sauvegardez vos dépôts vers un cloud souverain — clé en main. Seul le nom du plan se tape.</div></div>
      <div class="view-head-actions"><button class="btn btn-ghost btn-sm" id="bk-journal-link">🧾 Journal</button><button class="btn btn-primary btn-sm" id="bk-new">＋ Nouveau plan</button></div></div>
    <div id="bk-list"><div class="skeleton"></div></div>`;
  el("bk-new").onclick = () => backupTunnel(null);
  el("bk-journal-link").onclick = () => navTo("bkjournal");
  try { state.corpusClouds = (await App().FileClouds()) || []; } catch (e) { if (isLocked(e)) return handleLock(); }
  let plans = [];
  try { plans = (await App().Backups()) || []; } catch (e) {
    if (isLocked(e)) return handleLock();
    el("bk-list").innerHTML = blankslate("⚠", "Sauvegardes indisponibles", cleanErr(e)); return;
  }
  const box = el("bk-list");
  if (!plans.length) {
    box.innerHTML = blankslate("🛟", "Aucun plan de sauvegarde", "Créez un plan en quelques clics : quoi sauvegarder, sous quelle forme, vers quel cloud, et quand.") +
      `<div class="cc-center"><button class="btn btn-primary" id="bk-new2">＋ Nouveau plan</button></div>`;
    el("bk-new2").onclick = () => backupTunnel(null);
  } else {
    box.innerHTML = plans.map(bkCardHTML).join("");
    box.querySelectorAll("[data-run]").forEach((b) => b.onclick = () => runBackupFlow(b.dataset.run));
    box.querySelectorAll("[data-edit]").forEach((b) => b.onclick = () => { const p = plans.find((x) => x.name === b.dataset.edit); if (p) backupTunnel(p); });
    box.querySelectorAll("[data-rm]").forEach((b) => b.onclick = async () => {
      try { await App().RemoveBackupPlan(b.dataset.rm); renderBackup(); } catch (e) { toast(cleanErr(e), "err"); }
    });
  }
}

// renderBkJournal : page dédiée au JOURNAL des sauvegardes (lien depuis BACKUP).
async function renderBkJournal() {
  const view = el("view");
  view.innerHTML = `
    <div class="view-head view-head-row">
      <div><button class="btn btn-ghost btn-sm" id="bkj-back">‹ BACKUP</button>
      <div class="view-title" style="margin-top:6px">🧾 Journal des sauvegardes</div>
      <div class="view-sub">L'historique de vos exécutions, avec leur statut.</div></div></div>
    <div id="bkj-list"><div class="skeleton"></div></div>`;
  el("bkj-back").onclick = () => navTo("backup");
  let runs = [];
  try { runs = (await App().BackupRuns()) || []; } catch (e) { if (isLocked(e)) return handleLock(); el("bkj-list").innerHTML = blankslate("⚠", "Journal indisponible", cleanErr(e)); return; }
  if (!runs.length) { el("bkj-list").innerHTML = blankslate("🧾", "Aucune sauvegarde", "Lancez un plan depuis BACKUP — chaque exécution s'inscrira ici."); return; }
  el("bkj-list").innerHTML = `<div class="bk-runs">${runs.map((r) => `
    <div class="bk-run ${r.ok ? "ok" : "ko"}">
      <span class="bk-run-dot">${r.ok ? "✓" : "✕"}</span>
      <span class="bk-run-plan">${esc(r.plan)}</span>
      <span class="bk-run-info">${r.repos || 0} dépôt(s)${r.failed ? ` · ${r.failed} échec(s)` : ""} · ${esc(bkFormLabel(r.form))} · ${esc(bkRoutineLabel(r.trigger))}</span>
      <span class="bk-run-when">${esc(fmtWhen(r.when))}</span>
      ${r.detail ? `<span class="bk-run-detail">${esc(r.detail)}</span>` : ""}
    </div>`).join("")}</div>`;
}

function bkWhat(b) {
  if (!b.type) return `<code>${esc(b.repo_path || "")}</code>`; // hérité mono-dépôt
  if (b.type === "targeted") return `${(b.repos || []).length} dépôt(s) choisi(s)`;
  const sc = BK_SCOPES.find((s) => s.k === b.scope);
  return `<span class="bk-dyn">↻ ${esc(sc ? sc.label : (b.scope || "dépôts"))}</span>`;
}
function bkFormLabel(f) { return f === "mirror" ? "miroir (fichiers lisibles)" : "bundle git"; }
function bkRoutineLabel(r) { return r === "on_open" ? "à l'ouverture" : r === "on_close" ? "à la fermeture" : "manuel"; }

function bkCardHTML(b) {
  const legacy = !b.type;
  return `<div class="bk-card">
    <div class="bk-main"><b>${esc(b.name)}</b>
      <span class="bk-flow">${bkWhat(b)} → <b>${esc(b.cloud)}</b><code>${esc(b.dest || "/")}</code></span>
      <span class="bk-meta">${legacy ? "plan simple" : `${esc(bkFormLabel(b.form))} · ${esc(b.layer || "local")} · ${esc(bkRoutineLabel(b.routine))}`} · ${b.last_run ? "dernière : " + esc(fmtWhen(b.last_run)) : "jamais exécutée"}</span></div>
    <div class="bk-act"><button class="btn btn-primary btn-sm" data-run="${esc(b.name)}">Sauvegarder maintenant</button>
      ${legacy ? "" : `<button class="btn btn-ghost btn-sm" data-edit="${esc(b.name)}">Modifier</button>`}
      <button class="btn btn-ghost btn-sm" data-rm="${esc(b.name)}">Retirer</button></div></div>`;
}

// backupTunnel : assistant ÉTAPE PAR ÉTAPE (tunnel). plan != null → édition (pré-rempli). Tout se
// choisit ; seul le nom se tape. On avance Suivant › / ‹ Précédent, récap final avant de créer.
function backupTunnel(plan) {
  const clouds = state.corpusClouds || [];
  if (!clouds.length) { toast("Connectez d'abord un cloud dans CORPUS", "err"); return; }
  const W = {
    name: (plan && plan.name) || "",
    type: (plan && plan.type) || "generic",
    scope: (plan && plan.scope) || "kernels",
    repos: (plan && plan.repos) ? plan.repos.slice() : [],
    layer: (plan && plan.layer) || "local",
    form: (plan && plan.form) || "bundle",
    routine: (plan && plan.routine) || "manual",
    cloud: (plan && plan.cloud) || clouds[0].name,
    dest: (plan && plan.dest) || "/",
    editing: !!plan,
  };
  const STEPS = [
    { k: "name", t: "Nom & type" },
    { k: "what", t: "Que sauvegarder" },
    { k: "layer", t: "Quelle couche" },
    { k: "form", t: "Forme" },
    { k: "dest", t: "Destination" },
    { k: "routine", t: "Routine" },
    { k: "review", t: "Récapitulatif" },
  ];
  let si = 0;
  const seg = (name, cur, opts) => `<div class="seg bk-seg" data-seg="${name}">${opts.map((o) => `<button type="button" data-v="${o.v}" class="${cur === o.v ? "on" : ""}">${o.l}</button>`).join("")}</div>`;
  const scopeLabel = (k) => (BK_SCOPES.find((s) => s.k === k) || {}).label || k;
  const helps = () => {
    const th = el("bk-type-help"); if (th) th.textContent = W.type === "generic" ? "Portée dynamique : tout nouveau dépôt correspondant entrera automatiquement." : "Liste figée : seuls les dépôts inscrits sont sauvegardés.";
    const fh = el("bk-form-help"); if (fh) fh.textContent = W.form === "mirror" ? "Miroir : fichiers déposés lisibles dans le cloud (plus lourd)." : "Bundle : un .bundle par dépôt (léger, re-clonable, illisible sans git).";
  };
  const drawScope = () => {
    const ui = el("bk-scope-ui"); if (!ui) return;
    if (W.type === "generic") {
      ui.innerHTML = `<div class="sub" style="margin-bottom:6px">Portée (dynamique) :</div><div class="bk-scope-grid">${BK_SCOPES.map((s) => `<button type="button" class="bk-scope ${W.scope === s.k ? "on" : ""}" data-scope="${s.k}"><span class="bk-scope-ico">${bkPicto(s.k)}</span>${esc(s.label)}</button>`).join("")}</div>`;
      ui.querySelectorAll("[data-scope]").forEach((b) => b.onclick = () => { W.scope = b.dataset.scope; drawScope(); });
    } else {
      ui.innerHTML = `<button type="button" class="btn btn-ghost btn-sm" id="bk-pickrepos">🔎 Choisir les dépôts (${W.repos.length})</button>
        ${W.repos.length ? `<div class="bk-repolist">${W.repos.map((p) => `<span class="bk-repochip">${esc(baseNameJS(p))}</span>`).join("")}</div>` : `<div class="sub">Aucun dépôt choisi pour l'instant.</div>`}`;
      el("bk-pickrepos").onclick = () => backupRepoSelect(W.repos, (sel) => { W.repos = sel; drawScope(); });
    }
  };
  const stepBody = (k) => {
    if (k === "name") return `<label>Nom du plan</label><input id="bk-n" class="inp" placeholder="ex. KERNEL vers Nextcloud" autocomplete="off" value="${esc(W.name)}" />
      <div class="bk-step-h" style="margin-top:14px">Type de sauvegarde</div>
      ${seg("type", W.type, [{ v: "generic", l: "Générique (dynamique)" }, { v: "targeted", l: "Ciblé (figé)" }])}
      <div class="sub" id="bk-type-help"></div><div class="inline-error hidden" id="bk-e"></div>`;
    if (k === "what") return `<div id="bk-scope-ui"></div><div class="inline-error hidden" id="bk-e"></div>`;
    if (k === "layer") return `<div class="sub" style="margin-bottom:6px">Que contient la sauvegarde ?</div>${seg("layer", W.layer, [{ v: "local", l: "Local" }, { v: "atelier", l: "Atelier" }, { v: "both", l: "Les deux" }])}`;
    if (k === "form") return `${seg("form", W.form, [{ v: "bundle", l: "Bundle git (léger)" }, { v: "mirror", l: "Miroir (fichiers lisibles)" }])}<div class="sub" id="bk-form-help"></div>`;
    if (k === "dest") return `<label>Cloud de destination</label>
      <select id="bk-c" class="select">${clouds.map((c) => `<option value="${esc(c.name)}"${W.cloud === c.name ? " selected" : ""}>${esc(c.name)} — ${esc(c.kind)}</option>`).join("")}</select>
      <div class="bk-dest-row" style="margin-top:8px">→ <code id="bk-dest">${esc(W.dest || "/")}</code> <button class="btn btn-ghost btn-sm" type="button" id="bk-browse">📁 Choisir le dossier…</button></div>`;
    if (k === "routine") return `${seg("routine", W.routine, [{ v: "manual", l: "Manuel" }, { v: "on_open", l: "À l'ouverture" }, { v: "on_close", l: "À la fermeture" }])}<div class="sub">Les routines automatiques ne s'exécutent que pour les clouds sans PIN (autonomes).</div>`;
    if (k === "review") {
      const what = W.type === "targeted" ? `${W.repos.length} dépôt(s) choisi(s)` : scopeLabel(W.scope);
      return `<div class="bk-review">
        <div><b>Nom</b><span>${esc(W.name || "—")}</span></div>
        <div><b>Type</b><span>${W.type === "generic" ? "Générique (dynamique)" : "Ciblé (figé)"}</span></div>
        <div><b>Contenu</b><span>${esc(what)}</span></div>
        <div><b>Couche</b><span>${esc(W.layer)}</span></div>
        <div><b>Forme</b><span>${esc(bkFormLabel(W.form))}</span></div>
        <div><b>Destination</b><span>${esc(W.cloud)} <code>${esc(W.dest || "/")}</code></span></div>
        <div><b>Routine</b><span>${esc(bkRoutineLabel(W.routine))}</span></div>
      </div><div class="inline-error hidden" id="bk-e"></div>`;
    }
    return "";
  };
  const wireStep = (k) => {
    el("modal-body").querySelectorAll(".bk-seg").forEach((sg) => sg.onclick = (ev) => {
      const b = ev.target.closest("[data-v]"); if (!b) return;
      W[sg.dataset.seg] = b.dataset.v;
      sg.querySelectorAll("button").forEach((x) => x.classList.toggle("on", x === b));
      helps();
    });
    if (k === "name") { helps(); const n = el("bk-n"); if (n) n.oninput = () => { W.name = n.value; }; }
    if (k === "what") drawScope();
    if (k === "form") helps();
    if (k === "dest") {
      const c = el("bk-c"); if (c) c.onchange = () => { W.cloud = c.value; };
      const br = el("bk-browse"); if (br) br.onclick = () => cloudFolderPicker(el("bk-c").value, W.dest, (d) => { W.dest = d; const dc = el("bk-dest"); if (dc) dc.textContent = d || "/"; });
    }
  };
  const validate = (k) => {
    const e = el("bk-e");
    if (k === "name") { const n = el("bk-n"); W.name = (n ? n.value : W.name).trim(); if (!W.name) { if (e) { e.textContent = "Donnez un nom au plan."; e.classList.remove("hidden"); } return false; } }
    if (k === "what" && W.type === "targeted" && W.repos.length === 0) { if (e) { e.textContent = "Sélectionnez au moins un dépôt."; e.classList.remove("hidden"); } return false; }
    if (k === "dest") { const c = el("bk-c"); if (c) W.cloud = c.value; }
    return true;
  };
  const save = async () => {
    const e = el("bk-e");
    try { await App().SaveBackupPlan({ name: W.name, type: W.type, scope: W.scope, repos: W.repos, layer: W.layer, form: W.form, routine: W.routine, cloud: W.cloud, dest: W.dest }); closeModal(); renderBackup(); }
    catch (err) { if (e) { e.textContent = cleanErr(err); e.classList.remove("hidden"); } }
  };
  const draw = () => {
    const d = STEPS[si], k = d.k, last = si === STEPS.length - 1;
    const dots = `<span class="bk-dots">${STEPS.map((_, j) => `<span class="bk-dot${j <= si ? " on" : ""}"></span>`).join("")}</span>`;
    el("modal-body").innerHTML = `<div class="field bk-tunnel">
      <div class="bk-wiz-h">${esc(d.t)} <span class="muted">— étape ${si + 1}/${STEPS.length}</span></div>
      <div class="bk-wiz-body">${stepBody(k)}</div>
      <div class="bk-wiz-foot">${dots}<div class="bk-wiz-btns">
        ${si > 0 ? `<button type="button" class="btn btn-ghost btn-sm" id="bk-prev">‹ Précédent</button>` : ""}
        ${last ? `<button type="button" class="btn btn-primary btn-sm" id="bk-save">${W.editing ? "Enregistrer" : "Créer le plan"}</button>` : `<button type="button" class="btn btn-primary btn-sm" id="bk-next">Suivant ›</button>`}
      </div></div></div>`;
    wireStep(k);
    if (el("bk-prev")) el("bk-prev").onclick = () => { si--; draw(); };
    if (el("bk-next")) el("bk-next").onclick = () => { if (validate(k)) { si++; draw(); } };
    if (el("bk-save")) el("bk-save").onclick = save;
  };
  openModal(W.editing ? "Modifier le plan" : "Nouveau plan de sauvegarde", `<div id="bk-wiz"></div>`, null);
  if (el("modal-ok")) el("modal-ok").classList.add("hidden");
  if (el("modal-cancel")) el("modal-cancel").textContent = "Annuler";
  draw();
}

// baseNameJS : dernier segment d'un chemin (coupe sur \ et /).
function baseNameJS(p) { const s = String(p || "").replace(/[\\/]+$/, ""); const i = Math.max(s.lastIndexOf("/"), s.lastIndexOf("\\")); return i < 0 ? s : s.slice(i + 1); }

// backupRepoSelect : sélecteur de dépôts appairés (filtres KERNEL/spoke/autre + recherche). onPick(paths[]).
async function backupRepoSelect(current, onPick) {
  let rows = [];
  try { rows = (await App().ProjectsLight()) || []; } catch (e) { toast(cleanErr(e), "err"); return; }
  const items = rows.filter((p) => p.local && p.local.path).map((p) => ({
    path: p.local.path, name: p.name,
    role: p.kernel ? "kernel" : (p.spoke ? "spoke" : "other"),
    tag: p.tag || "",
  }));
  const sel = new Set(current || []);
  const filt = { q: "", role: "all" };
  const render = () => {
    const list = items.filter((it) => (filt.role === "all" || it.role === filt.role) && (!filt.q || (it.name + " " + it.tag).toLowerCase().includes(filt.q)));
    el("brs-list").innerHTML = list.length ? list.map((it) => `<label class="brs-row"><input type="checkbox" data-p="${esc(it.path)}" ${sel.has(it.path) ? "checked" : ""} />
      <span class="brs-ico">${bkPicto(it.role)}</span><b>${esc(it.name)}</b>${it.tag ? `<span class="brs-tag">${esc(it.tag)}</span>` : ""}</label>`).join("") : `<div class="muted" style="padding:10px">Aucun dépôt.</div>`;
    el("brs-list").querySelectorAll("input[data-p]").forEach((c) => c.onchange = () => { if (c.checked) sel.add(c.dataset.p); else sel.delete(c.dataset.p); el("brs-n").textContent = sel.size; });
  };
  openModal("Choisir les dépôts à sauvegarder", `<div class="field">
    <div class="org-filters" style="margin-top:0">
      <div class="org-search"><span class="org-search-ico">⌕</span><input id="brs-q" class="inp" type="search" placeholder="Rechercher…" autocomplete="off" /></div>
      <div class="org-chips">${[["all", "Tous"], ["kernel", "KERNEL"], ["spoke", "Spokes"], ["other", "Autres"]].map(([k, l]) => `<button type="button" class="org-chip ${k === "all" ? "on" : ""}" data-role="${k}">${k !== "all" && k !== "other" ? `<span class="btn-ico">${bkPicto(k)}</span>` : ""}${l}</button>`).join("")}</div>
    </div>
    <div id="brs-list" class="brs-listbox"></div>
    <div class="sub"><b id="brs-n">${sel.size}</b> dépôt(s) sélectionné(s).</div></div>`, () => { onPick(Array.from(sel)); closeModal(); });
  el("modal-ok").textContent = "Valider la sélection";
  el("brs-q").oninput = () => { filt.q = el("brs-q").value.trim().toLowerCase(); render(); };
  el("modal-body").querySelectorAll("[data-role]").forEach((b) => b.onclick = () => { filt.role = b.dataset.role; el("modal-body").querySelectorAll("[data-role]").forEach((x) => x.classList.toggle("on", x === b)); render(); });
  render();
}

// cloudFolderPicker : navigue l'arbre d'un cloud, permet de créer un sous-dossier, puis choisit. onPick(path).
async function cloudFolderPicker(cloud, start, onPick) {
  let cur = start && start !== "/" ? start : "/";
  const load = async () => {
    el("cfp-cur").textContent = cur;
    el("cfp-list").innerHTML = `<div class="skeleton"></div>`;
    let entries = [];
    try { entries = (await App().CloudList(cloud, cur)) || []; } catch (e) { el("cfp-list").innerHTML = `<div class="callout warn">${esc(cleanErr(e))}</div>`; return; }
    const dirs = entries.filter((e) => e.is_dir || e.type === "dir");
    el("cfp-list").innerHTML = (cur !== "/" ? `<button type="button" class="cfp-row cfp-up" data-up="1">⬆ ..</button>` : "") +
      (dirs.length ? dirs.map((d) => `<button type="button" class="cfp-row" data-into="${esc(d.name)}">📁 ${esc(d.name)}</button>`).join("") : `<div class="muted" style="padding:8px">Aucun sous-dossier.</div>`);
    el("cfp-list").querySelectorAll("[data-into]").forEach((b) => b.onclick = () => { cur = (cur === "/" ? "" : cur) + "/" + b.dataset.into; load(); });
    const up = el("cfp-list").querySelector("[data-up]"); if (up) up.onclick = () => { cur = cur.replace(/\/[^/]+$/, "") || "/"; load(); };
  };
  openModal("Choisir le dossier — " + esc(cloud), `<div class="field">
    <div class="cfp-bar">Dossier courant : <code id="cfp-cur">/</code></div>
    <div id="cfp-list" class="cfp-listbox"><div class="skeleton"></div></div>
    <div class="cfp-newrow"><input id="cfp-new" class="inp" placeholder="nom d'un nouveau sous-dossier (optionnel)" autocomplete="off" />
      <span class="sub">Il sera créé automatiquement à la sauvegarde.</span></div></div>`, () => {
    const n = (el("cfp-new").value || "").trim().replace(/[\\/]/g, "");
    let dest = cur;
    if (n) dest = (cur === "/" ? "" : cur) + "/" + n;
    onPick(dest || "/"); closeModal();
  });
  el("modal-ok").textContent = "Choisir ce dossier";
  load();
}

function runBackupFlow(name) {
  pinConfirm("Lancer la sauvegarde « " + name + " »", async (pin) => {
    const out = await App().RunBackupPlan(name, pin).catch(async (e) => {
      // Rétro-compat : un plan hérité (mono-dépôt) passe par RunBackup.
      if (/introuvable|type/i.test(cleanErr(e))) return App().RunBackup(name, pin);
      throw e;
    });
    toast("Sauvegardé : " + out, "ok");
    renderBackup();
  });
}
