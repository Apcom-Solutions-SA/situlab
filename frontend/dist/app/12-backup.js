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
    // Dernière exécution par plan (pour l'état ✓/⚠/✕ sur chaque carte). Best-effort : le journal est
    // du plus récent au plus ancien, donc la 1re occurrence d'un plan est sa dernière exécution.
    let lastByPlan = {};
    try { ((await App().BackupRuns()) || []).forEach((r) => { if (!lastByPlan[r.plan]) lastByPlan[r.plan] = r; }); } catch (_) {}
    box.innerHTML = plans.map((p) => bkCardHTML(p, lastByPlan[p.name])).join("");
    box.querySelectorAll("[data-journal]").forEach((b) => b.onclick = () => navTo("bkjournal"));
    box.querySelectorAll("[data-run]").forEach((b) => b.onclick = () => runBackupFlow(b.dataset.run));
    box.querySelectorAll("[data-edit]").forEach((b) => b.onclick = () => { const p = plans.find((x) => x.name === b.dataset.edit); if (p) backupTunnel(p); });
    box.querySelectorAll("[data-rm]").forEach((b) => b.onclick = () => {
      const nm = b.dataset.rm;
      confirmDelete({
        title: `Supprimer le plan « ${esc(nm)} » ?`,
        lead: `Le plan de sauvegarde <b>${esc(nm)}</b> sera retiré de SituLab. Il ne s'exécutera plus.`,
        items: ["Le plan et son réglage (quoi, où, quand, rétention)"],
        safe: ["Les sauvegardes déjà envoyées sur le cloud (bundles et archives) restent en place", "Vos dépôts locaux ne sont pas touchés"],
        confirmLabel: "Supprimer le plan",
        onConfirm: async () => { await App().RemoveBackupPlan(nm); renderBackup(); },
      });
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

function bkCardHTML(b, lastRun) {
  const legacy = !b.type;
  // État de la DERNIÈRE exécution, visible d'un coup d'œil (observabilité) : ✓ réussie, ⚠ partielle,
  // ✕ échouée. Cliquable → Journal pour le détail (où, quoi, quelle erreur).
  let statut = "";
  if (lastRun) {
    const ko = !lastRun.ok, partial = lastRun.ok && lastRun.failed > 0;
    const ico = ko ? "✕" : partial ? "⚠" : "✓";
    const cls = ko ? "ko" : partial ? "warn" : "ok";
    statut = `<button class="bk-status ${cls}" data-journal="1" title="Voir le Journal des sauvegardes">${ico} dernière : ${esc(fmtWhen(b.last_run || lastRun.when))}</button>`;
  } else {
    statut = `<span class="bk-meta-run">${b.last_run ? "dernière : " + esc(fmtWhen(b.last_run)) : "jamais exécutée"}</span>`;
  }
  return `<div class="bk-card">
    <div class="bk-main"><b>${esc(b.name)}</b>
      <span class="bk-flow">${bkWhat(b)} → <b>${esc(b.cloud)}</b><code>${esc(b.dest || "/")}</code></span>
      <span class="bk-meta">${legacy ? "plan simple" : `${esc(bkFormLabel(b.form))} · ${esc(b.layer || "local")} · ${esc(bkRoutineLabel(b.routine))} · ${b.overwrite ? (b.archives ? "écrase +" + b.archives + " arch." : "écrase") : "cumulatif"}`} · ${statut}</span></div>
    <div class="bk-act"><button class="btn btn-primary btn-sm" data-run="${esc(b.name)}">Sauvegarder maintenant</button>
      ${legacy ? "" : `<button class="btn btn-ghost btn-sm" data-edit="${esc(b.name)}">Modifier</button>`}
      <button class="icon-del" data-rm="${esc(b.name)}" title="Retirer le plan « ${esc(b.name)} »" aria-label="Retirer le plan de sauvegarde « ${esc(b.name)} »">${ICO_TRASH}</button></div></div>`;
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
    // Rétention (chaînes dans le tunnel, converties à l'enregistrement). Nouveau plan : écrase + 2
    // archives (cloud borné). Plan existant : on garde son réglage (absent = cumulatif, comportement
    // historique préservé).
    overwrite: plan ? (plan.overwrite ? "yes" : "no") : "yes",
    // Édition : on reprend le nombre d'archives du plan ; absent = 0 (un ancien enregistrement omettait le
    // zéro). Création : défaut 2. (Avant, un plan édité à « 0 archive » repassait à tort à 2.)
    archives: plan ? (typeof plan.archives === "number" ? String(plan.archives) : "0") : "2",
    editing: !!plan,
    _orig: (plan && plan.name) || "", // nom AVANT édition → détecte un renommage à l'enregistrement
  };
  // NB : plus d'étape « couche » (local/atelier/both). Elle n'avait pas d'effet réel — un bundle « --all »
  // d'un clone capture déjà tout, et le miroir n'en faisait qu'un sous-dossier. Un contrôle qui ne fait
  // pas ce qu'il annonce n'a pas sa place dans un flux clé-en-main. W.layer reste « local » (champ conservé).
  const STEPS = [
    { k: "name", t: "Nom & type" },
    { k: "what", t: "Que sauvegarder" },
    { k: "form", t: "Forme" },
    { k: "retention", t: "Conservation" },
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
    // Rétention : affiche l'avertissement « cumulatif » OU le choix du nombre d'archives.
    const rc = el("bk-ret-cumul"), ra = el("bk-ret-arch");
    if (rc && ra) { const ow = W.overwrite !== "no"; rc.classList.toggle("hidden", ow); ra.classList.toggle("hidden", !ow); }
  };
  const drawScope = () => {
    const ui = el("bk-scope-ui"); if (!ui) return;
    if (W.type === "generic") {
      ui.innerHTML = `<div class="sub" style="margin-bottom:6px">Portée (dynamique) :</div><div class="bk-scope-grid">${BK_SCOPES.map((s) => `<button type="button" class="bk-scope ${W.scope === s.k ? "on" : ""}" data-scope="${s.k}"><span class="bk-scope-ico">${bkPicto(s.k)}</span>${esc(s.label)}</button>`).join("")}</div>`;
      ui.querySelectorAll("[data-scope]").forEach((b) => b.onclick = () => { W.scope = b.dataset.scope; drawScope(); });
    } else {
      ui.innerHTML = `<button type="button" class="btn btn-ghost btn-sm" id="bk-pickrepos">🔎 Choisir les dépôts (${W.repos.length})</button>
        ${W.repos.length ? `<div class="bk-repolist">${W.repos.map((p) => `<span class="bk-repochip">${esc(baseNameJS(p))}</span>`).join("")}</div>` : `<div class="sub">Aucun dépôt choisi pour l'instant.</div>`}`;
      el("bk-pickrepos").onclick = () => backupRepoSelect(W.repos, (sel) => { W.repos = sel; reopenTunnel(); }, reopenTunnel);
    }
  };
  const stepBody = (k) => {
    if (k === "name") return `<label>Nom du plan</label><input id="bk-n" class="inp" placeholder="ex. KERNEL vers Nextcloud" autocomplete="off" value="${esc(W.name)}" />
      <div class="bk-step-h" style="margin-top:14px">Type de sauvegarde</div>
      ${seg("type", W.type, [{ v: "generic", l: "Générique (dynamique)" }, { v: "targeted", l: "Ciblé (figé)" }])}
      <div class="sub" id="bk-type-help"></div><div class="inline-error hidden" id="bk-e"></div>`;
    if (k === "what") return `<div id="bk-scope-ui"></div><div class="inline-error hidden" id="bk-e"></div>`;
    if (k === "form") return `${seg("form", W.form, [{ v: "bundle", l: "Bundle git (léger)" }, { v: "mirror", l: "Miroir (fichiers lisibles)" }])}<div class="sub" id="bk-form-help"></div>`;
    if (k === "retention") return `<div class="sub" style="margin-bottom:6px">À chaque sauvegarde…</div>
      ${seg("overwrite", W.overwrite, [{ v: "yes", l: "Écraser le précédent" }, { v: "no", l: "Tout conserver" }])}
      <div id="bk-ret-cumul" class="callout warn" style="margin-top:10px">⚠ En <b>conservant tout</b>, le dossier cloud <b>s'alourdit</b> à chaque sauvegarde (un bundle de plus à chaque fois). À surveiller.</div>
      <div id="bk-ret-arch">
        <div class="sub" style="margin:12px 0 6px">Archives conservées (versions précédentes rangées dans un sous-dossier <code>archive/</code>) :</div>
        ${seg("archives", W.archives, [{ v: "0", l: "Aucune" }, { v: "1", l: "1" }, { v: "2", l: "2" }, { v: "3", l: "3" }])}
        <div class="sub">Seul le dernier bundle reste en vue ; au-delà, les plus anciennes archives sont supprimées automatiquement à chaque sauvegarde.</div>
      </div>`;
    if (k === "dest") return `<label>Cloud de destination</label>
      <select id="bk-c" class="select">${clouds.map((c) => `<option value="${esc(c.name)}"${W.cloud === c.name ? " selected" : ""}>${esc(c.name)} — ${esc(c.kind)}</option>`).join("")}</select>
      <div class="bk-dest-row" style="margin-top:8px">→ <code id="bk-dest">${esc(W.dest || "/")}</code> <button class="btn btn-ghost btn-sm" type="button" id="bk-browse">📁 Choisir le dossier…</button></div>`;
    if (k === "routine") return `${seg("routine", W.routine, [{ v: "manual", l: "Manuel" }, { v: "on_open", l: "À l'ouverture" }, { v: "on_close", l: "À la fermeture" }])}<div class="sub">Les sauvegardes se font <b>sans PIN</b> : les routines « ouverture » et « fermeture » s'exécutent toutes seules (avec un suivi visuel à l'écran).</div>`;
    if (k === "review") {
      const what = W.type === "targeted" ? `${W.repos.length} dépôt(s) choisi(s)` : scopeLabel(W.scope);
      return `<div class="bk-review">
        <div><b>Nom</b><span>${esc(W.name || "—")}</span></div>
        <div><b>Type</b><span>${W.type === "generic" ? "Générique (dynamique)" : "Ciblé (figé)"}</span></div>
        <div><b>Contenu</b><span>${esc(what)}</span></div>
        <div><b>Forme</b><span>${esc(bkFormLabel(W.form))}</span></div>
        <div><b>Conservation</b><span>${W.overwrite === "no" ? "tout conserver (cumulatif)" : (W.archives === "0" ? "écrase (aucune archive)" : "écrase + " + esc(W.archives) + " archive(s)")}</span></div>
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
    if (k === "retention") helps();
    if (k === "dest") {
      const c = el("bk-c"); if (c) c.onchange = () => { W.cloud = c.value; };
      const br = el("bk-browse"); if (br) br.onclick = () => cloudFolderPicker(el("bk-c").value, W.dest, (d) => { W.dest = d; reopenTunnel(); }, reopenTunnel);
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
    try { await App().SaveBackupPlan(W._orig || "", { name: W.name, type: W.type, scope: W.scope, repos: W.repos, layer: W.layer, form: W.form, routine: W.routine, cloud: W.cloud, dest: W.dest, overwrite: W.overwrite !== "no", archives: parseInt(W.archives, 10) || 0 }); closeModal(); renderBackup(); }
    catch (err) { const m = cleanErr(err); if (e) { e.textContent = m; e.classList.remove("hidden"); } else { toast(m, "err"); } }
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
  // La modale étant globale et partagée, les sous-sélecteurs (dépôts, dossier cloud) la réécrivent ;
  // on reconstruit le tunnel à l'étape courante après chaque pick/annulation (sinon le tunnel est
  // détruit et « Créer le plan » jamais atteint — bug de création des plans).
  const reopenTunnel = () => {
    openModal(W.editing ? "Modifier le plan" : "Nouveau plan de sauvegarde", `<div id="bk-wiz"></div>`, null);
    if (el("modal-ok")) el("modal-ok").classList.add("hidden");
    if (el("modal-cancel")) el("modal-cancel").textContent = "Annuler";
    draw();
  };
  reopenTunnel();
}

// baseNameJS : dernier segment d'un chemin (coupe sur \ et /).
function baseNameJS(p) { const s = String(p || "").replace(/[\\/]+$/, ""); const i = Math.max(s.lastIndexOf("/"), s.lastIndexOf("\\")); return i < 0 ? s : s.slice(i + 1); }

// backupRepoSelect : sélecteur de dépôts appairés (filtres KERNEL/spoke/autre + recherche). onPick(paths[]).
async function backupRepoSelect(current, onPick, onCancel) {
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
    <div class="sub"><b id="brs-n">${sel.size}</b> dépôt(s) sélectionné(s).</div></div>`, () => { onPick(Array.from(sel)); });
  el("modal-ok").textContent = "Valider la sélection";
  if (onCancel && el("modal-cancel")) el("modal-cancel").onclick = onCancel;
  el("brs-q").oninput = () => { filt.q = el("brs-q").value.trim().toLowerCase(); render(); };
  el("modal-body").querySelectorAll("[data-role]").forEach((b) => b.onclick = () => { filt.role = b.dataset.role; el("modal-body").querySelectorAll("[data-role]").forEach((x) => x.classList.toggle("on", x === b)); render(); });
  render();
}

// cloudFolderPicker : navigue l'arbre d'un cloud, permet de créer un sous-dossier, puis choisit. onPick(path).
async function cloudFolderPicker(cloud, start, onPick, onCancel) {
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
    onPick(dest || "/");
  });
  el("modal-ok").textContent = "Choisir ce dossier";
  if (onCancel && el("modal-cancel")) el("modal-cancel").onclick = onCancel;
  load();
}

// runBackupFlow : lance une sauvegarde manuelle. PAS de PIN (routine additive) — elle « se fait ».
// Garde-fou lecture seule (le cœur le refuserait aussi), état « en cours », puis résultat / erreur clairs.
async function runBackupFlow(name) {
  if (state.readOnly) { toast("Mode lecture seule actif — désactivez-le dans Réglages pour sauvegarder", "err"); return; }
  toast("Sauvegarde en cours…", "info");
  try {
    // Dispatch par TYPE du plan, pas en interceptant un message d'erreur : l'ancien repli se déclenchait
    // sur tout échec contenant « type »/« introuvable » et MASQUAIT la cause réelle. Plan v1.14 (type
    // défini) → RunBackupPlan ; plan hérité mono-dépôt → RunBackup (qui, côté cœur, redirige au besoin).
    let plan = null;
    try { plan = ((await App().Backups()) || []).find((b) => b && b.name === name) || null; } catch (e) {}
    const out = (plan && plan.type) ? await App().RunBackupPlan(name) : await App().RunBackup(name);
    // Succès complet ou partiel : le cœur renvoie « N dépôt(s) sauvegardé(s) [, M échec(s)] → cloud:chemin ».
    const partial = /échec/i.test(String(out || ""));
    toast("✓ " + (out || "Sauvegarde terminée"), partial ? "info" : "ok");
  } catch (e) {
    toast("Échec de la sauvegarde : " + cleanErr(e), "err");
  }
  renderBackup();
}

/* ---------------- Suivi VISUEL permanent des sauvegardes (1.17) ----------------
 * Un indicateur flottant présent sur TOUTES les vues pendant qu'une sauvegarde tourne (manuelle,
 * ouverture ou fermeture), et un écran plein à la fermeture pour que l'utilisateur sache qu'il ne doit
 * pas forcer la sortie (la fenêtre se ferme toute seule une fois la sauvegarde finie). Branché une fois
 * au démarrage (installBackupProgress), piloté par les événements Wails "backup:progress"/"backup:closing".
 */
let _bkIndTimer = null;
function installBackupProgress() {
  if (!(window.runtime && window.runtime.EventsOn) || state._bkProgBound) return;
  state._bkProgBound = true;
  try { window.runtime.EventsOn("backup:progress", (ev) => bkOnProgress(ev || {})); } catch (_) {}
  try { window.runtime.EventsOn("backup:closing", (c) => bkOnClosing(!!(c && c.active))); } catch (_) {}
  // Au montage : si une sauvegarde tourne déjà (rechargement pendant un run), on affiche l'état.
  try { App().BackupBusy().then((busy) => { if (busy) bkOnProgress({ active: true, phase: "prepare" }); }).catch(() => {}); } catch (_) {}
}

function bkTriggerLabel(t) { return t === "on_open" ? "à l'ouverture" : t === "on_close" ? "à la fermeture" : "manuelle"; }
function bkWhereLabel(ev) {
  return (ev.phase === "repo" && ev.total)
    ? `dépôt ${ev.index}/${ev.total}${ev.repo ? " — " + esc(ev.repo) : ""}`
    : "préparation…";
}

function bkIndEl() {
  let n = el("bk-ind");
  if (!n) {
    n = document.createElement("div");
    n.id = "bk-ind";
    n.className = "bk-ind hidden";
    n.title = "Voir le Journal des sauvegardes";
    n.onclick = () => navTo("bkjournal");
    document.body.appendChild(n);
  }
  return n;
}

function bkOnProgress(ev) {
  const n = bkIndEl();
  if (_bkIndTimer) { clearTimeout(_bkIndTimer); _bkIndTimer = null; }
  if (ev.active) {
    n.className = "bk-ind running";
    n.innerHTML = `<span class="bk-ind-spin" aria-hidden="true"></span>
      <div class="bk-ind-txt"><b>Sauvegarde ${esc(bkTriggerLabel(ev.trigger))} en cours</b>
        <span class="bk-ind-sub">${ev.plan ? esc(ev.plan) + " · " : ""}${bkWhereLabel(ev)}</span></div>`;
    bkClosingUpdate(ev); // reflète aussi l'avancement sur l'écran de fermeture s'il est affiché
  } else {
    const ko = !ev.ok, partial = ev.ok && ev.failed > 0;
    const ico = ko ? "✕" : partial ? "⚠" : "✓";
    n.className = "bk-ind done " + (ko ? "ko" : partial ? "warn" : "ok");
    n.innerHTML = `<span class="bk-ind-ico">${ico}</span>
      <div class="bk-ind-txt"><b>Sauvegarde ${ko ? "échouée" : partial ? "partielle" : "terminée"}</b>
        <span class="bk-ind-sub">${ev.plan ? esc(ev.plan) : ""}${ev.failed ? " · " + ev.failed + " échec(s)" : ""} · voir le Journal</span></div>`;
    _bkIndTimer = setTimeout(() => { n.className = "bk-ind hidden"; n.innerHTML = ""; }, 5000);
    if (state.view === "backup") renderBackup(); // rafraîchit l'état des cartes si on est sur BACKUP
  }
}

function bkClosingEl() {
  let n = el("bk-closing");
  if (!n) {
    n = document.createElement("div");
    n.id = "bk-closing";
    n.className = "bk-closing hidden";
    document.body.appendChild(n);
  }
  return n;
}
function bkOnClosing(active) {
  const n = bkClosingEl();
  if (!active) { n.className = "bk-closing hidden"; n.innerHTML = ""; return; }
  n.className = "bk-closing";
  n.innerHTML = `<div class="bk-closing-card">
    <div class="bk-closing-spin" aria-hidden="true"></div>
    <div class="bk-closing-h">💾 Sauvegarde avant fermeture</div>
    <div class="bk-closing-d" id="bk-closing-d">Vos sauvegardes « à la fermeture » s'exécutent.<br>
      Merci de patienter — <b>ne forcez pas la fermeture</b> : la fenêtre se fermera toute seule.</div></div>`;
}
function bkClosingUpdate(ev) {
  const box = el("bk-closing");
  if (!box || box.classList.contains("hidden")) return;
  const d = el("bk-closing-d");
  if (d) d.innerHTML = `<b>${ev.plan ? esc(ev.plan) : ""}</b> — ${bkWhereLabel(ev)}<br>
    <span class="muted">Ne forcez pas la fermeture ; la fenêtre se fermera automatiquement.</span>`;
}
