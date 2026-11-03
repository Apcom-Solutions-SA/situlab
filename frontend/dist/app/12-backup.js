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
  view.innerHTML = pageHero("backup", ico("shield", 26), "BACKUP",
      "Vos plans de sauvegarde des dépôts vers un cloud. Sans PIN, à l'ouverture, à la fermeture ou à la demande.",
      `<button class="btn btn-ghost btn-sm" id="bk-journal-link">${ico("activity", 14)} Journal</button><button class="btn btn-primary btn-sm" id="bk-new">${ico("plus", 14)} Nouveau plan</button>`) +
    `<div id="bk-list"><div class="skeleton"></div></div>`;
  el("bk-new").onclick = () => backupTunnel(null);
  el("bk-journal-link").onclick = () => navTo("bkjournal");
  try { state.corpusClouds = (await App().FileClouds()) || []; } catch (e) { if (isLocked(e)) return handleLock(); }
  // Dépendance BACKUP ⟸ CORPUS (1.19) : un plan de sauvegarde vise un dossier d'un cloud. Sans aucun
  // cloud connecté, « Nouveau plan » mène à une impasse — on désactive l'action et on oriente d'abord
  // vers CORPUS, au lieu d'inviter à un clic qui échoue.
  const noCloud = !((state.corpusClouds || []).length);
  if (noCloud) { const nb = el("bk-new"); if (nb) { nb.disabled = true; nb.title = "Connectez d'abord un cloud (CORPUS)"; } }
  let plans = [];
  try { plans = (await App().Backups()) || []; } catch (e) {
    if (isLocked(e)) return handleLock();
    el("bk-list").innerHTML = blankslate("⚠", "Sauvegardes indisponibles", cleanErr(e)); return;
  }
  const box = el("bk-list");
  if (!plans.length) {
    if (noCloud) {
      box.innerHTML = blankslate(ico("cloud", 30), "Connectez d'abord un cloud", "Un plan de sauvegarde dépose vos dépôts dans un dossier d'un cloud CORPUS. Connectez un cloud pour créer votre premier plan.") +
        `<div class="cc-center"><button class="btn btn-primary" id="bk-gocorpus">${ico("cloud", 14)} Connecter un cloud</button></div>`;
      el("bk-gocorpus").onclick = () => navTo("corpus");
    } else {
      box.innerHTML = blankslate(ico("shield", 30), "Aucun plan de sauvegarde", "Créez un plan en quelques clics : quoi sauvegarder, sous quelle forme, vers quel cloud, et quand.") +
        `<div class="cc-center"><button class="btn btn-primary" id="bk-new2">${ico("plus", 14)} Nouveau plan</button></div>`;
      el("bk-new2").onclick = () => backupTunnel(null);
    }
  } else {
    // Dernière exécution par plan (pour l'état ✓/⚠/✕ sur chaque carte). Best-effort : le journal est
    // du plus récent au plus ancien, donc la 1re occurrence d'un plan est sa dernière exécution.
    let lastByPlan = {};
    try { ((await App().BackupRuns()) || []).forEach((r) => { if (!lastByPlan[r.plan]) lastByPlan[r.plan] = r; }); } catch (_) {}
    box.innerHTML = plans.map((p) => bkCardHTML(p, lastByPlan[p.name])).join("");
    box.querySelectorAll("[data-journal]").forEach((b) => b.onclick = () => navTo("bkjournal"));
    box.querySelectorAll("[data-run]").forEach((b) => b.onclick = () => runBackupFlow(b.dataset.run));
    box.querySelectorAll("[data-edit]").forEach((b) => b.onclick = () => { const p = plans.find((x) => x.name === b.dataset.edit); if (p) backupTunnel(p); });
    box.querySelectorAll("[data-bkauto]").forEach((b) => b.onclick = async () => {
      const p = plans.find((x) => x.name === b.dataset.bkauto); if (!p) return;
      try {
        await App().SaveBackupPlan(p.name, { name: p.name, type: p.type, scope: p.scope, repos: p.repos, layer: p.layer, source: p.source, form: p.form, routine: p.routine, cloud: p.cloud, dest: p.dest, overwrite: !!p.overwrite, archives: p.archives || 0, auto: !p.auto, every_min: p.every_min || 30 });
        toast(p.auto ? "Sauvegarde automatique désactivée" : "Sauvegarde automatique activée (toutes les 30 min)", "ok"); renderBackup();
      } catch (e) { toast(cleanErr(e), "err"); }
    });
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
      <div class="view-title view-title-stacked">🧾 Journal des sauvegardes</div></div></div>
    <div id="bkj-list"><div class="skeleton"></div></div>`;
  el("bkj-back").onclick = () => navTo("backup");
  let runs = [];
  try { runs = (await App().BackupRuns()) || []; } catch (e) { if (isLocked(e)) return handleLock(); el("bkj-list").innerHTML = blankslate("⚠", "Journal indisponible", cleanErr(e)); return; }
  if (!runs.length) { el("bkj-list").innerHTML = blankslate(ico("save", 30), "Aucune sauvegarde", "Lancez un plan depuis BACKUP, chaque exécution s'inscrira ici."); return; }
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
      <span class="bk-meta">${legacy ? "plan simple" : `${esc(bkFormLabel(b.form))} · ${esc(b.layer || "local")} · à chaque fermeture · ${b.overwrite ? (b.archives ? "écrase +" + b.archives + " arch." : "écrase") : "cumulatif"}`} · ${statut}</span></div>
    <div class="bk-act"><button class="icon-save" data-run="${esc(b.name)}" title="Sauvegarder « ${esc(b.name)} » maintenant" aria-label="Sauvegarder « ${esc(b.name)} » maintenant">${ico("save")}</button>
      ${legacy ? "" : `<button class="btn btn-ghost btn-sm${b.auto ? " on" : ""}" data-bkauto="${esc(b.name)}" title="Sauvegarde automatique en arrière-plan (toutes les ${b.every_min || 30} min, tant que SituLab est ouvert)">${ico("clock", 14)} ${b.auto ? "auto ✓ (" + (b.every_min || 30) + " min)" : "auto"}</button>`}
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
    source: (plan && plan.source) || "local", // SOURCE sauvegardée (#25) : local | atelier | vitrine
    form: (plan && plan.form) || "bundle",
    routine: "on_close", // 1.18.1 : la sauvegarde est le geste de fermeture — standard, plus un choix
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
    { k: "source", t: "Source" },
    { k: "form", t: "Forme" },
    { k: "retention", t: "Conservation" },
    { k: "dest", t: "Destination" },
    { k: "review", t: "Récapitulatif" },
  ];
  let si = 0;
  const seg = (name, cur, opts) => `<div class="seg bk-seg" data-seg="${name}">${opts.map((o) => `<button type="button" data-v="${o.v}" class="${cur === o.v ? "on" : ""}">${o.l}</button>`).join("")}</div>`;
  const scopeLabel = (k) => (BK_SCOPES.find((s) => s.k === k) || {}).label || k;
  const helps = () => {
    const th = el("bk-type-help"); if (th) th.textContent = W.type === "generic" ? "Portée dynamique : tout nouveau dépôt correspondant entrera automatiquement." : "Liste figée : seuls les dépôts inscrits sont sauvegardés.";
    const fh = el("bk-form-help"); if (fh) fh.textContent = W.form === "mirror" ? "Miroir : fichiers déposés lisibles dans le cloud (plus lourd)." : "Bundle : un .bundle par dépôt (léger, re-clonable, illisible sans git).";
    const sh = el("bk-source-help"); if (sh) sh.textContent = W.source === "atelier" ? "Atelier : SituLab rafraîchit l'atelier (origin) et sauvegarde SON état — pas vos branches locales. Format bundle." : "Local : l'état de votre poste (branches locales et l'atelier déjà récupéré).";
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
      ui.innerHTML = `<button type="button" class="btn btn-ghost btn-sm" id="bk-pickrepos">${ico("search", 14)} Choisir les dépôts (${W.repos.length})</button>
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
    if (k === "source") return `${seg("source", W.source, [{ v: "local", l: `${ico("folder", 14)} Local (ce poste)` }, { v: "atelier", l: `${ico("branch", 14)} Atelier` }])}
      <div class="bk-src-soon" style="margin-top:8px;font-size:12px;color:var(--text-faint)">${ico("globe", 13)} Vitrine (production) — bientôt (récupération de la production)</div>
      <div class="sub" id="bk-source-help"></div>`;
    if (k === "form") {
      // La source Atelier (état d'un dépôt distant) impose le bundle ; le miroir ne vaut que pour Local.
      if (W.source !== "local") return `${seg("form", "bundle", [{ v: "bundle", l: "Bundle git (léger)" }])}<div class="sub" id="bk-form-help"></div>`;
      return `${seg("form", W.form, [{ v: "bundle", l: "Bundle git (léger)" }, { v: "mirror", l: "Miroir (fichiers lisibles)" }])}<div class="sub" id="bk-form-help"></div>`;
    }
    if (k === "retention") return `<div class="sub" style="margin-bottom:6px">À chaque sauvegarde…</div>
      ${seg("overwrite", W.overwrite, [{ v: "yes", l: "Écraser le précédent" }, { v: "no", l: "Tout conserver" }])}
      <div id="bk-ret-cumul" class="callout warn" style="margin-top:10px">⚠ En <b>conservant tout</b>, le dossier cloud <b>s'alourdit</b> à chaque sauvegarde (un bundle de plus à chaque fois). À surveiller.</div>
      <div id="bk-ret-arch">
        <div class="sub" style="margin:12px 0 6px">Archives conservées (versions précédentes rangées dans un sous-dossier <code>archive/</code>) :</div>
        ${seg("archives", W.archives, [{ v: "0", l: "Aucune" }, { v: "1", l: "1" }, { v: "2", l: "2" }, { v: "3", l: "3" }])}
        <div class="sub">Seul le dernier bundle reste en vue ; au-delà, les plus anciennes archives sont supprimées automatiquement à chaque sauvegarde.</div>
      </div>`;
    if (k === "dest") return `<label>Cloud de destination</label>
      <select id="bk-c" class="select">${clouds.map((c) => `<option value="${esc(c.name)}"${W.cloud === c.name ? " selected" : ""}>${esc(c.name)} (${esc(c.kind)})</option>`).join("")}</select>
      <div class="bk-dest-row" style="margin-top:8px">→ <code id="bk-dest">${esc(W.dest || "/")}</code> <button class="btn btn-ghost btn-sm" type="button" id="bk-browse">${ico("folder", 14)} Choisir le dossier…</button></div>`;
    if (k === "review") {
      const what = W.type === "targeted" ? `${W.repos.length} dépôt(s) choisi(s)` : scopeLabel(W.scope);
      return `<div class="bk-review">
        <div><b>Nom</b><span>${esc(W.name || "—")}</span></div>
        <div><b>Type</b><span>${W.type === "generic" ? "Générique (dynamique)" : "Ciblé (figé)"}</span></div>
        <div><b>Contenu</b><span>${esc(what)}</span></div>
        <div><b>Source</b><span>${W.source === "atelier" ? "Atelier (état du serveur)" : "Local (ce poste)"}</span></div>
        <div><b>Forme</b><span>${esc(bkFormLabel(W.form))}</span></div>
        <div><b>Conservation</b><span>${W.overwrite === "no" ? "tout conserver (cumulatif)" : (W.archives === "0" ? "écrase (aucune archive)" : "écrase + " + esc(W.archives) + " archive(s)")}</span></div>
        <div><b>Destination</b><span>${esc(W.cloud)} <code>${esc(W.dest || "/")}</code></span></div>
        <div><b>Quand</b><span>à chaque fermeture de SituLab</span></div>
      </div><div class="inline-error hidden" id="bk-e"></div>`;
    }
    return "";
  };
  const wireStep = (k) => {
    el("modal-body").querySelectorAll(".bk-seg").forEach((sg) => sg.onclick = (ev) => {
      const b = ev.target.closest("[data-v]"); if (!b || b.disabled) return;
      W[sg.dataset.seg] = b.dataset.v;
      if (sg.dataset.seg === "source" && W.source !== "local") W.form = "bundle"; // atelier/vitrine → bundle (#25)
      sg.querySelectorAll("button").forEach((x) => x.classList.toggle("on", x === b));
      helps();
    });
    if (k === "name") { helps(); const n = el("bk-n"); if (n) n.oninput = () => { W.name = n.value; }; }
    if (k === "what") drawScope();
    if (k === "source") helps();
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
    try {
      const isNew = !W._orig, planName = W.name;
      await App().SaveBackupPlan(W._orig || "", { name: W.name, type: W.type, scope: W.scope, repos: W.repos, layer: W.layer, source: W.source, form: W.form, routine: W.routine, cloud: W.cloud, dest: W.dest, overwrite: W.overwrite !== "no", archives: parseInt(W.archives, 10) || 0 });
      closeModal(); renderBackup();
      // P16 : à la création d'un plan, première sauvegarde immédiate (progression visuelle via backup:progress).
      if (isNew) { toast("Plan créé : première sauvegarde en cours…", "ok"); App().RunBackupPlan(planName).then(() => renderBackup()).catch((e2) => toast(cleanErr(e2), "err")); }
    }
    catch (err) { const m = cleanErr(err); if (e) { e.textContent = m; e.classList.remove("hidden"); } else { toast(m, "err"); } }
  };
  const draw = () => {
    const d = STEPS[si], k = d.k, last = si === STEPS.length - 1;
    const dots = `<span class="bk-dots">${STEPS.map((_, j) => `<span class="bk-dot${j <= si ? " on" : ""}"></span>`).join("")}</span>`;
    el("modal-body").innerHTML = `<div class="field bk-tunnel">
      <div class="bk-wiz-h">${esc(d.t)} <span class="muted">(étape ${si + 1}/${STEPS.length})</span></div>
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
    el("cfp-list").innerHTML = (cur !== "/" ? `<button type="button" class="cfp-row cfp-up" data-up="1">${ico("chevronUp", 14)} ..</button>` : "") +
      (dirs.length ? dirs.map((d) => `<button type="button" class="cfp-row" data-into="${esc(d.name)}">${ico("folder", 15)} ${esc(d.name)}</button>`).join("") : `<div class="muted" style="padding:8px">Aucun sous-dossier.</div>`);
    el("cfp-list").querySelectorAll("[data-into]").forEach((b) => b.onclick = () => { cur = (cur === "/" ? "" : cur) + "/" + b.dataset.into; load(); });
    const up = el("cfp-list").querySelector("[data-up]"); if (up) up.onclick = () => { cur = cur.replace(/\/[^/]+$/, "") || "/"; load(); };
  };
  openModal("Choisir le dossier : " + esc(cloud), `<div class="field">
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
  if (state.readOnly) { toast("Mode lecture seule actif, désactivez-le dans Réglages pour sauvegarder", "err"); return; }
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

/* ---------------- Suivi VISUEL des sauvegardes ----------------
 * (1.17) Indicateur flottant pendant une sauvegarde MANUELLE lancée depuis la liste.
 * (1.18.1) À la fermeture : plus de sauvegarde d'office. Une modale DEMANDE (sauvegarder / fermer
 *   sans), puis une modale de SUIVI riche montre la liste des plans, la progression par plan, une
 *   estimation du temps restant et des quittances claires — la fenêtre se ferme seule à la fin.
 * Branché une fois au démarrage (installBackupProgress), piloté par les événements Wails
 * "backup:progress" (avancement) et "backup:close-prompt" (demande à la fermeture).
 */
let _bkIndTimer = null;
let _bkClose = null; // état de la modale de suivi de fermeture (null = pas en séquence de fermeture)

function installBackupProgress() {
  if (!(window.runtime && window.runtime.EventsOn) || state._bkProgBound) return;
  state._bkProgBound = true;
  try { window.runtime.EventsOn("backup:progress", (ev) => bkOnProgress(ev || {})); } catch (_) {}
  try { window.runtime.EventsOn("backup:close-prompt", (list) => bkCloseChoice(Array.isArray(list) ? list : [])); } catch (_) {}
  // 1.18.5 : suivi visuel des purges d'agent + modale INTELLIGENTE de fermeture (sauvegarde + purge).
  try { window.runtime.EventsOn("agentpurge:progress", (ev) => agPurgeOnProgress(ev || {})); } catch (_) {}
  try { window.runtime.EventsOn("close:prompt", (p) => closePromptModal(p || {})); } catch (_) {}
  // Fin des actions de fermeture (#4, 1.19) : plus d'auto-close — on montre le bilan et un bouton explicite.
  try { window.runtime.EventsOn("close:done", () => closeDoneScreen()); } catch (_) {}
  // Au montage : si une sauvegarde tourne déjà (rechargement pendant un run), on affiche l'état.
  try { App().BackupBusy().then((busy) => { if (busy) bkOnProgress({ active: true, phase: "prepare" }); }).catch(() => {}); } catch (_) {}
}

function bkTriggerLabel(t) { return t === "on_open" ? "à l'ouverture" : t === "on_close" ? "à la fermeture" : "manuelle"; }
function bkWhereLabel(ev) {
  return (ev.phase === "repo" && ev.total)
    ? `dépôt ${ev.index}/${ev.total}${ev.repo ? " · " + esc(ev.repo) : ""}`
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
  // Pendant la fermeture, c'est la modale de suivi qui montre tout : pas d'indicateur flottant en plus.
  if (_bkClose) { bkCloseRunUpdate(ev); return; }
  const n = bkIndEl();
  if (_bkIndTimer) { clearTimeout(_bkIndTimer); _bkIndTimer = null; }
  if (ev.active) {
    n.className = "bk-ind running";
    // Dépôt EN COURS nommé (#30) : on montre le travail concret même dans l'indicateur flottant.
    const sub = ev.repo
      ? `${ev.plan ? esc(ev.plan) + " · " : ""}« ${esc(ev.repo)} »${ev.total ? " (" + (ev.index || "?") + "/" + ev.total + ")" : ""}`
      : `${ev.plan ? esc(ev.plan) + " · " : ""}${bkWhereLabel(ev)}`;
    n.innerHTML = `<span class="bk-ind-spin" aria-hidden="true"></span>
      <div class="bk-ind-txt"><b>Sauvegarde ${esc(bkTriggerLabel(ev.trigger))} en cours</b>
        <span class="bk-ind-sub">${sub}</span></div>`;
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

// bkCloseChoice : à la fermeture, DEMANDER — ne jamais sauvegarder d'office. Modale enjouée : on
// résume ce qui serait sauvegardé, et deux voies nettes (sauvegarder puis fermer / fermer sans).
function bkCloseChoice(list) {
  if (!list.length) { try { App().CloseWithoutBackup(); } catch (_) {} return; } // garde-fou : rien à faire
  const items = list.reduce((a, p) => a + (p.items || 0), 0);
  const rows = list.map((p) => `<div class="bkc-sum">
    <span class="bkc-sum-ico">${bkPicto("repos")}</span>
    <span class="bkc-sum-n"><b>${esc(p.plan)}</b><span class="bkc-sum-d">${esc(bkFormLabel(p.form))} · ${esc(p.cloud)}<code>${esc(p.dest || "/")}</code> · ${p.items || 0} dépôt(s)</span></span></div>`).join("");
  openModal("Sauvegarder avant de fermer ?", `<div class="field bkc-choice">
    <div class="bkc-choice-lead">Vos dépôts peuvent être sauvegardés dans le cloud avant la fermeture.<br>
      <span class="muted">${list.length} plan(s) · ${items} dépôt(s) au total.</span></div>
    <div class="bkc-choice-list">${rows}</div>
    <div class="bkc-choice-btns">
      <button type="button" class="btn btn-primary" id="bkc-yes">${ico("save", 14)} Oui, sauvegarder puis fermer</button>
      <button type="button" class="btn btn-ghost" id="bkc-no">Fermer sans sauvegarder</button>
    </div></div>`, null);
  if (el("modal-ok")) el("modal-ok").classList.add("hidden");
  if (el("modal-cancel")) el("modal-cancel").classList.add("hidden");
  el("bkc-yes").onclick = () => { bkCloseRun(list); try { App().RunCloseBackups(); } catch (_) {} };
  el("bkc-no").onclick = () => { try { App().CloseWithoutBackup(); } catch (_) {} };
}

// bkCloseRun : bascule vers la modale de SUIVI (liste des plans + barres). L'avancement arrive ensuite
// par les événements "backup:progress" → bkCloseRunUpdate. _bkClose porte l'état agrégé.
function bkCloseRun(list) {
  const total = list.reduce((a, p) => a + (p.items || 0), 0);
  _bkClose = { total, start: 0, idx: {}, rows: {} };
  list.forEach((p, i) => { _bkClose.idx[p.plan] = i; _bkClose.rows[p.plan] = { total: p.items || 0, done: 0, status: "wait", failed: 0 }; });
  const rowsHTML = list.map((p, i) => `<div class="bkc-row wait" id="bkc-row-${i}">
    <span class="bkc-row-ico" id="bkc-rico-${i}">•</span>
    <div class="bkc-row-main"><b>${esc(p.plan)}</b>
      <span class="bkc-row-sub">${esc(p.cloud)}<code>${esc(p.dest || "/")}</code> · ${esc(bkFormLabel(p.form))} · ${p.items || 0} dépôt(s)</span>
      <div class="bkc-bar sm"><div class="bkc-bar-fill" id="bkc-rfill-${i}"></div></div>
      <div class="bkc-cur-wrap" id="bkc-rcur-${i}" hidden>
        <span class="bkc-cur-txt" id="bkc-rcurtxt-${i}"></span>
        <div class="bkc-bar xs"><div class="bkc-bar-fill" id="bkc-curfill-${i}"></div></div>
      </div></div>
    <span class="bkc-row-st" id="bkc-rst-${i}">en attente</span></div>`).join("");
  openModal("Sauvegarde avant fermeture", `<div class="field bkc">
    <div class="bkc-lead">Vos dépôts partent vers le cloud. <b>Ne fermez pas la fenêtre</b> : le bilan s'affichera à la fin.</div>
    <div class="bkc-overall"><div class="bkc-bar"><div class="bkc-bar-fill" id="bkc-ofill"></div></div>
      <div class="bkc-overall-meta"><span id="bkc-ocount">0 / ${total} dépôt(s)</span><span id="bkc-eta">estimation…</span></div></div>
    <div class="bkc-list">${rowsHTML}</div></div>`, null);
  if (el("modal-ok")) el("modal-ok").classList.add("hidden");
  if (el("modal-cancel")) el("modal-cancel").classList.add("hidden");
}

// bkCloseRunUpdate : applique un événement d'avancement à la modale de suivi (ligne du plan + barre
// globale + estimation du temps restant, calculée depuis le débit observé).
function bkCloseRunUpdate(ev) {
  if (!_bkClose) return;
  if (ev.active && !_bkClose.start) _bkClose.start = Date.now();
  const plan = ev.plan;
  if (!plan) return;
  if (!(plan in _bkClose.rows)) { _bkClose.rows[plan] = { total: ev.total || 0, done: 0, status: "wait", failed: 0 }; _bkClose.idx[plan] = Object.keys(_bkClose.idx).length; }
  const r = _bkClose.rows[plan], i = _bkClose.idx[plan];
  if (ev.phase === "prepare") { r.status = "run"; }
  else if (ev.phase === "repo") { r.status = "run"; if (ev.total) r.total = ev.total; r.done = ev.index || r.done; if (ev.repo) r.repo = ev.repo; r.subIdx = ev.sub_index || 0; r.subTot = ev.sub_total || 0; }
  else if (ev.phase === "done") { r.failed = ev.failed || 0; r.status = !ev.ok ? "ko" : (r.failed ? "warn" : "ok"); r.done = r.total; r.subIdx = r.subTot = 0; }
  const pct = (done, tot) => (tot ? Math.min(100, Math.round(100 * done / tot)) : (done ? 100 : 0));
  const ico = el("bkc-rico-" + i), st = el("bkc-rst-" + i), fill = el("bkc-rfill-" + i), row = el("bkc-row-" + i);
  if (ico) ico.textContent = r.status === "ok" ? "✓" : r.status === "warn" ? "⚠" : r.status === "ko" ? "✕" : r.status === "run" ? "⏳" : "•";
  // NIVEAU 2 — jauge du BLOC = avancement des DÉPÔTS du plan (dépôt X/N), pas l'intra-dépôt (#30).
  if (fill) fill.style.width = pct(r.done, r.total) + "%";
  if (st) st.textContent = r.status === "ok" ? "sauvegardé ✓" : r.status === "warn" ? (r.failed + " échec(s)") : r.status === "ko" ? "échec" : r.status === "run" ? `dépôt ${r.done}/${r.total || "?"}` : "en attente";
  if (row) row.className = "bkc-row " + r.status;
  // NIVEAU 3 — ligne du BAS : le DÉPÔT EN COURS et SA PROPRE progression (fichiers), avec mini-jauge
  // intra-dépôt. On voit le dépôt courant avancer, puis on passe au suivant (#30, 3 niveaux séparés).
  const cur = el("bkc-rcur-" + i), curtxt = el("bkc-rcurtxt-" + i), curfill = el("bkc-curfill-" + i);
  if (cur) {
    if (r.status === "run") {
      cur.hidden = false;
      if (curtxt) curtxt.innerHTML = r.repo
        ? `<span class="bkc-cur-spin" aria-hidden="true"></span> « <b>${esc(r.repo)}</b> » · dépôt ${r.done}/${r.total || "?"}${r.subTot ? ` · fichier ${r.subIdx}/${r.subTot} (${Math.round(100 * r.subIdx / r.subTot)}%)` : ""}`
        : `<span class="bkc-cur-spin" aria-hidden="true"></span> Préparation…`;
      if (curfill) { curfill.style.width = (r.subTot ? pct(r.subIdx, r.subTot) : 0) + "%"; if (curfill.parentElement) curfill.parentElement.style.visibility = r.subTot ? "visible" : "hidden"; }
    } else if (r.status === "wait") {
      cur.hidden = true;
    } else {
      cur.hidden = false;
      if (curtxt) curtxt.innerHTML = r.status === "ok" ? `✓ ${r.total} dépôt(s) sauvegardé(s)`
        : r.status === "warn" ? `⚠ ${r.total - r.failed}/${r.total} sauvegardé(s), ${r.failed} échec(s)`
        : "✕ échec";
      if (curfill && curfill.parentElement) curfill.parentElement.style.visibility = "hidden";
    }
  }
  // Agrégat : dépôts traités (plan terminé = tout son total ; plan en cours = son avancement courant).
  let done = 0; Object.keys(_bkClose.rows).forEach((k) => {
    const x = _bkClose.rows[k];
    const terminal = x.status === "ok" || x.status === "warn" || x.status === "ko";
    done += terminal ? x.total : (x.subTot ? Math.max(0, x.done - 1 + x.subIdx / x.subTot) : Math.max(0, x.done));
  });
  const total = _bkClose.total || done;
  const of = el("bkc-ofill"); if (of) of.style.width = pct(done, total) + "%";
  const oc = el("bkc-ocount"); if (oc) oc.textContent = `${Math.round(done)} / ${total} dépôt(s)`;
  const eta = el("bkc-eta");
  if (eta) {
    const allDone = Object.keys(_bkClose.rows).every((k) => { const s = _bkClose.rows[k].status; return s === "ok" || s === "warn" || s === "ko"; });
    if (allDone) { eta.textContent = "terminé"; }
    else if (_bkClose.start && done > 0) {
      const elapsed = (Date.now() - _bkClose.start) / 1000, rate = done / elapsed;
      const rem = rate > 0 ? Math.ceil((total - done) / rate) : 0;
      eta.textContent = rem > 0 ? `≈ ${rem}s restantes` : "presque fini…";
    } else { eta.textContent = "estimation…"; }
  }
}

/* ---------------- 1.18.5 : purge d'agent VISUALISÉE + fermeture intelligente ----------------
 * La purge n'est plus une bulle : un écran de SUIVI vivant où chaque entrée défile (⏳ → ✓/⚠), avec
 * barre de progression, compteur de Mo libérés et RAPPORT final. Alimenté par "agentpurge:progress".
 * À la fermeture, une seule modale propose sauvegarde ET purge ("close:prompt") ; le même écran montre
 * les étapes avant que la fenêtre se ferme.
 */
let _agRun = null;      // cible courante du suivi de purge ({ listId }) ; null = aucun écran ouvert
let _agClosing = false; // on purge dans la séquence de FERMETURE (après les éventuelles sauvegardes)

function agRunScaffold(title, sub) {
  return `<div class="agr">
    <div class="agr-head"><span class="agr-scan" aria-hidden="true"></span>
      <div class="agr-head-txt"><div class="agr-title">${esc(title)}</div>${sub ? `<div class="agr-sub">${sub}</div>` : ""}</div></div>
    <div class="agr-overall"><div class="agr-bar"><div class="agr-bar-fill" id="agr-ofill"></div></div>
      <div class="agr-meta"><span id="agr-count">préparation…</span><span id="agr-freed">0 o libérés</span></div></div>
    <div class="agr-list" id="agr-list"></div>
    <div class="agr-report" id="agr-report"></div></div>`;
}

// agPurgeOpen : ouvre l'écran de suivi (modale), masque OK/Annuler.
function agPurgeOpen(title, sub) {
  openModal(title, agRunScaffold(title, sub), null);
  if (el("modal-ok")) el("modal-ok").classList.add("hidden");
  if (el("modal-cancel")) el("modal-cancel").classList.add("hidden");
  _agRun = { listId: "agr-list" };
}

// agentPurgeRun : purge MANUELLE (depuis la liste des agents) avec suivi vivant. Ouvre l'écran, lance la
// purge et laisse les événements la peindre ; à la fin, un bouton Fermer.
async function agentPurgeRun(a) {
  _agClosing = false;
  agPurgeOpen(`Purge de « ${a.name} »`, `Dossier <code>${esc(a.dir)}</code>`);
  try { await App().PurgeAgent(a.key, "PURGER"); }
  catch (e) {
    const r = el("agr-report"); if (r) r.insertAdjacentHTML("afterbegin", `<div class="agr-rep-row"><span class="agr-rep ko">${esc(cleanErr(e))}</span></div>`);
    agPurgeDoneButton(); return;
  }
  agPurgeDoneButton(); // le rapport a été peint par l'événement "done"
  renderAgentPurge();
}

function agPurgeDoneButton() {
  const r = el("agr-report"); if (!r || el("agr-close")) return;
  r.insertAdjacentHTML("beforeend", `<button type="button" class="btn btn-primary agr-close-btn" id="agr-close">Fermer</button>`);
  el("agr-close").onclick = () => { _agRun = null; closeModal(); };
}

// agPurgeOnProgress : applique un événement de purge à l'écran (création de ligne au "start", mise à jour
// au "removed"/"skipped", rapport au "done"). Ids préfixés par la clé de l'agent → plusieurs agents à la
// fermeture sans collision.
function agPurgeOnProgress(ev) {
  // Cas fermeture : l'écran n'est pas encore ouvert quand la purge démarre (après les sauvegardes).
  if (!_agRun && _agClosing) agPurgeOpen(`Purge — ${esc(ev.name || "")}`, "Fermeture de SituLab. <b>Ne fermez pas la fenêtre</b> : le bilan s'affichera à la fin.");
  if (!_agRun) return;
  const list = el(_agRun.listId); if (!list) return;
  const rid = `agr-${ev.key}-${ev.index}`;
  if (ev.phase === "start") {
    if (ev.index === 1 && !el(`agr-h-${ev.key}`)) {
      list.insertAdjacentHTML("beforeend", `<div class="agr-ahead" id="agr-h-${ev.key}">${ico("robot", 13)} ${esc(ev.name)}</div>`);
    }
    if (!el(rid)) {
      list.insertAdjacentHTML("beforeend", `<div class="agr-row run" id="${rid}">
        <span class="agr-ico" id="${rid}-ic">⏳</span>
        <span class="agr-name" title="${esc(ev.entry)}">${esc(ev.entry)}${ev.is_dir ? "/" : ""}</span>
        <span class="agr-size">${fmtAgentSize(ev.size)}</span>
        <span class="agr-st" id="${rid}-st">en cours…</span></div>`);
      list.scrollTop = list.scrollHeight;
    }
  } else if (ev.phase === "removed" || ev.phase === "skipped") {
    const ok = ev.phase === "removed";
    const ic = el(`${rid}-ic`), st = el(`${rid}-st`), row = el(rid);
    if (ic) ic.textContent = ok ? "✓" : "⚠";
    if (st) st.textContent = ok ? `effacé · ${fmtAgentSize(ev.size)}` : "encore utilisé, ignoré";
    if (row) row.className = "agr-row " + (ok ? "ok" : "skip");
    const of = el("agr-ofill"); if (of && ev.total) of.style.width = Math.min(100, Math.round(100 * ev.index / ev.total)) + "%";
    const c = el("agr-count"); if (c) c.textContent = `${ev.index} / ${ev.total} · ${ev.removed} effacé(s)${ev.skipped ? ", " + ev.skipped + " ignoré(s)" : ""}`;
    const f = el("agr-freed"); if (f) f.textContent = `${fmtAgentSize(ev.freed)} libérés`;
  } else if (ev.phase === "done") {
    const of = el("agr-ofill"); if (of) of.style.width = "100%";
    const rep = el("agr-report"); if (!rep) return;
    const parts = [`<span class="agr-rep ok">✓ ${ev.removed} élément(s) effacé(s) · ${fmtAgentSize(ev.freed)} libérés</span>`];
    if (ev.skipped) parts.push(`<span class="agr-rep warn">⚠ ${ev.skipped} encore utilisé(s) par un process, ignoré(s). Fermez l'agent puis relancez.</span>`);
    rep.insertAdjacentHTML("afterbegin", `<div class="agr-rep-row">${parts.join("")}</div>`);
  }
}

// closePromptModal (close:prompt) : une seule modale INTELLIGENTE — sauvegarde ET/OU purge d'agents.
function closePromptModal(p) {
  const backups = (p && p.backups) || [];
  const agents = (p && p.agents) || [];
  if (!backups.length && !agents.length) { try { App().CloseWithoutBackup(); } catch (_) {} return; }
  const bkItems = backups.reduce((a, b) => a + (b.items || 0), 0);
  // Carte SAUVEGARDE — teinte d'univers backup (charte SituLab), registre rassurant (rien n'est détruit).
  const bkSec = backups.length ? `<label class="clp-card clp-backup">
    <span class="clp-badge">${ico("save", 18)}</span>
    <span class="clp-txt"><b>Sauvegarder mes dépôts</b><span class="clp-sub">${backups.length} plan(s) · ${bkItems} dépôt(s) → cloud, avant de fermer</span></span>
    <input type="checkbox" id="clp-bk" class="clp-sw" checked><span class="clp-track"></span></label>` : "";
  // Carte PURGE — teinte danger (destructif), registre d'alerte. Un interrupteur par agent.
  const agRows = agents.map((a) => `<label class="clp-agrow">
    <span class="clp-ag-ic">${ico("robot", 15)}</span>
    <span class="clp-ag-txt"><b>${esc(a.name)}</b> <span class="clp-sub">${fmtAgentSize(a.size_bytes)}</span></span>
    <input type="checkbox" class="clp-ag clp-sw" value="${esc(a.key)}" checked><span class="clp-track"></span></label>`).join("");
  const agTotal = agents.reduce((s, a) => s + (a.size_bytes || 0), 0);
  const agSec = agents.length ? `<div class="clp-purge-wrap clp-purge">
    <div class="clp-purge-h"><span class="clp-badge">${ico("broom", 16)}</span>
      <span class="clp-txt"><b>Purger des agents IA</b><span class="clp-sub">Jusqu'à <b class="clp-strong">${fmtAgentSize(agTotal)}</b> récupérables · login et minimum gardés</span></span></div>
    <div class="clp-aglist">${agRows}</div></div>` : "";
  openModal("Avant de fermer SituLab", `<div class="field clp">
    <div class="clp-lead">Souhaitez-vous préparer quelque chose avant la fermeture ?</div>
    ${bkSec}${agSec}
    <div class="clp-btns">
      <button type="button" class="btn btn-primary" id="clp-go">Continuer puis fermer</button>
      <button type="button" class="btn btn-ghost" id="clp-no">Fermer sans rien faire</button>
    </div></div>`, null);
  if (el("modal-ok")) el("modal-ok").classList.add("hidden");
  if (el("modal-cancel")) el("modal-cancel").classList.add("hidden");
  el("clp-no").onclick = () => { try { App().CloseWithoutBackup(); } catch (_) {} };
  el("clp-go").onclick = () => {
    const doBackup = !!(el("clp-bk") && el("clp-bk").checked);
    const keys = Array.from(document.querySelectorAll(".clp-ag:checked")).map((x) => x.value);
    if (!doBackup && !keys.length) { try { App().CloseWithoutBackup(); } catch (_) {} return; }
    _agClosing = keys.length > 0;
    // Sauvegardes d'abord (écran riche existant) ; sinon on ouvre direct l'écran de purge. La suite (les
    // purges) arrive par "agentpurge:progress" et bascule l'écran toute seule.
    if (doBackup && backups.length) { bkCloseRun(backups); }
    else if (keys.length) { agPurgeOpen("Purge avant fermeture", "Fermeture de SituLab. <b>Ne fermez pas la fenêtre</b> : le bilan s'affichera à la fin."); }
    try { App().RunCloseActions(doBackup, keys); } catch (_) {}
  };
}

// closeDoneScreen (close:done, #4, 1.19) : les actions de fermeture sont finies. On NE ferme PLUS tout
// seul — le bilan reste à l'écran (statut par plan sauvegardé, par agent et par dossier purgé, déjà
// peint par les écrans de suivi), on consolide le récap des sauvegardes si la purge a pris le relais,
// on neutralise les mentions « se fermera seule », et on pose un bouton explicite « OK — fermer ».
function closeDoneScreen() {
  if (_bkClose && _bkClose.rows && Object.keys(_bkClose.rows).length) {
    const rep = el("agr-report");
    if (rep && !el("bkc-recap")) {
      const rows = Object.keys(_bkClose.rows).map((k) => {
        const r = _bkClose.rows[k];
        const cls = r.status === "ok" ? "ok" : r.status === "ko" ? "ko" : "warn";
        const mk = r.status === "ok" ? "✓" : r.status === "ko" ? "✕" : "⚠";
        return `<div class="agr-rep-row"><span class="agr-rep ${cls}">${mk} Sauvegarde « ${esc(k)} »${r.failed ? " · " + r.failed + " échec(s)" : " · ok"}</span></div>`;
      }).join("");
      rep.insertAdjacentHTML("beforeend", `<div id="bkc-recap">${rows}</div>`);
    }
  }
  const lead = document.querySelector(".bkc-lead"); if (lead) lead.innerHTML = "<b>Terminé.</b> Vos sauvegardes et purges sont faites. Vous pouvez fermer quand vous voulez.";
  const asub = document.querySelector(".agr-sub"); if (asub) asub.innerHTML = "<b>Terminé.</b> Vous pouvez fermer quand vous voulez.";
  const eta = el("bkc-eta"); if (eta) eta.textContent = "terminé";
  const body = el("modal-body");
  if (body && !el("close-okbtn")) {
    const bar = document.createElement("div");
    bar.className = "clp-btns"; bar.style.marginTop = "16px";
    bar.innerHTML = `<button type="button" class="btn btn-primary" id="close-okbtn">${ico("check", 14)} OK — vous pouvez fermer maintenant</button>`;
    body.appendChild(bar);
    el("close-okbtn").onclick = () => { try { App().QuitNow(); } catch (_) {} };
    setTimeout(() => { const b = el("close-okbtn"); if (b) b.focus(); }, 40);
  }
}
