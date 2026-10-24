/* ---------------- Live loop (anti-réentrance, éco-ressources v1.3) ----------------
   La boucle se suspend quand la fenêtre est masquée/réduite et ralentit après une
   inactivité prolongée : l'application reste souvent ouverte en arrière-plan, inutile
   d'y relancer `git status` et d'interroger le serveur toutes les 3,5 s pour rien. */
const LIVE_MS = 3500;         // fenêtre active, utilisateur présent
const LIVE_SLOW_MS = 20000;   // ralenti après inactivité prolongée
const IDLE_AFTER_MS = 120000; // 2 min sans interaction → ralenti
// Vues qui dépendent du serveur courant (listes/état distants) : la sonde Whoami périodique ne tourne
// que sur elles ; ailleurs elle est inutile (perf 1.17, Point 2).
const LIVE_WHOAMI_VIEWS = ["projects", "detail", "project", "organize", "corpus", "admin", "directory", "stats", "tokens"];
let lastInteraction = Date.now();
let ecoInstalled = false;
let liveGen = 0; // génération de la boucle : un tick en vol dont la génération a changé ne se replanifie pas
function markInteraction() { lastInteraction = Date.now(); }

async function liveTickOnce() {
  if (liveBusy) return;
  liveBusy = true; liveTick++;
  try {
    // Détection PROACTIVE du verrouillage auto (inactivité) : le noyau peut verrouiller la session
    // tout seul (minuterie). Si la vue courante n'appelle aucune lecture nécessitant la session, on ne
    // le « verrait » pas et l'app resterait faussement utilisable sur des données périmées. On constate
    // donc l'état à chaque tick (appel local, très bon marché) et on gèle l'app (page friendly) dès que
    // c'est verrouillé. Garde-fous : ne rien faire si un écran de (dé)verrouillage est déjà en place
    // (app masquée) ou si une demande d'agent / une grâce est en cours. Erreur de pont → on suppose
    // déverrouillé (true) pour ne jamais geler l'app à tort sur un hoquet transitoire.
    if (!relocking && !pinModalOpen && !lockGrace && !el("app").classList.contains("hidden")) {
      if ((await App().Unlocked().catch(() => true)) === false) return handleLock();
    }
    // Sonde « en ligne » périodique (besoin perf 1.17) : seulement sur les vues qui AFFICHENT des
    // données du serveur courant. Sur une vue statique (Réglages, Aide, KERNEL, Agent IA…),
    // marteler Whoami toutes les ~30 s ne sert à rien ; l'état en ligne sera re-sondé dès qu'on
    // reviendra sur une vue connectée. Le contrôle de verrouillage ci-dessus, lui, reste à chaque tick.
    if (liveTick % 3 === 0 && LIVE_WHOAMI_VIEWS.includes(state.view)) await refreshWhoami();
    if (state.view === "projects") await renderProjects(true);
    else if (state.view === "detail") {
      await refreshDetailLocal();
      if (liveTick % 3 === 0) await refreshDetailRemote();
    }
    updateCheckedAt();
  } catch (e) { /* une erreur ponctuelle ne doit pas tuer la boucle */ }
  finally { liveBusy = false; }
}
// updateCheckedAt rafraîchit l'indicateur « vérifié il y a X min » sans repeindre la liste.
function updateCheckedAt() {
  const n = el("checked-at");
  if (!n || !state.checkedAt) return;
  const secs = Math.max(0, Math.round((Date.now() - state.checkedAt) / 1000));
  n.textContent = secs < 10 ? "· vérifié à l'instant"
    : secs < 60 ? "· vérifié il y a " + secs + " s"
    : "· vérifié il y a " + Math.floor(secs / 60) + " min";
}
// setAdvanced bascule le mode avancé, le mémorise et repeint immédiatement la vue courante.
function setAdvanced(on) {
  state.advanced = !!on;
  try { localStorage.setItem("situlab.advanced", on ? "1" : "0"); } catch (e) {}
  if (state.view === "detail" && state.detail) { state.detail._paintSig = null; paintDetail(); }
  else if (state.view === "repos") { state.repoSig = ""; paintRepos(); }
}
function scheduleLive() {
  if (liveTimer) { clearTimeout(liveTimer); liveTimer = null; }
  if (document.hidden) return; // fenêtre masquée : boucle suspendue (reprise par resumeLive)
  const gen = liveGen;
  const idle = Date.now() - lastInteraction > IDLE_AFTER_MS;
  liveTimer = setTimeout(async () => {
    await liveTickOnce();
    if (gen === liveGen) scheduleLive(); // ne se replanifie pas si stopLive() est passé entre-temps
  }, idle ? LIVE_SLOW_MS : LIVE_MS);
}
// resumeLive relance la boucle si la fenêtre est visible, l'app affichée et aucun timer en cours.
// Filet de sécurité : plusieurs signaux de reprise (revisibilité, focus, pageshow) pour éviter
// qu'un évènement « visible » manqué ne fige définitivement le « temps réel ».
function resumeLive() {
  if (document.hidden || liveTimer) return;
  if (el("app").classList.contains("hidden")) return; // verrouillé/onboarding : pas de boucle
  markInteraction();
  liveTickOnce().then(scheduleLive);
}
// installEco branche (une seule fois) la reprise et le suivi d'activité.
function installEco() {
  if (ecoInstalled) return;
  ecoInstalled = true;
  document.addEventListener("visibilitychange", () => { if (document.hidden) stopLive(); else resumeLive(); });
  window.addEventListener("focus", resumeLive);
  window.addEventListener("pageshow", resumeLive);
  ["click", "keydown", "pointerdown", "input"].forEach((ev) =>
    document.addEventListener(ev, markInteraction, { passive: true, capture: true }));
}
function startLive() { installEco(); markInteraction(); stopLive(); scheduleLive(); }
function stopLive() { liveGen++; if (liveTimer) clearTimeout(liveTimer); liveTimer = null; }

/* ---------------- Vue Dépôts ---------------- */
async function renderRepos(live) {
  if (!live) {
    el("view").innerHTML = `
      <div class="view-head">
        <div class="view-title">Dépôts</div>
        <div class="view-sub">
          <label class="sr-only" for="repo-instance-sel">Connexion git active</label>
          <select id="repo-instance-sel" class="select" title="Connexion git active">${(state.instances || []).map((i) => `<option value="${esc(i.name)}"${i.name === state.instance ? " selected" : ""}>${esc(i.name)}</option>`).join("")}</select>
          <span id="role-badge" class="role-badge" title="Rôle de la connexion"></span>
          <span id="checked-at" class="checked"></span>
        </div>
      </div>
      <div class="toolbar">
        <input id="repo-search" type="text" placeholder="Rechercher un dépôt…" aria-label="Rechercher" value="${esc(state.search)}" />
        <select id="repo-org" class="select" aria-label="Filtrer par organisation"></select>
        <select id="repo-state" class="select" aria-label="Filtrer par état">
          <option value="">Tous les états</option>
          <option value="dirty">Modifications en cours</option>
          <option value="ahead">À envoyer</option>
          <option value="behind">Nouveautés en ligne</option>
          <option value="diverged">À réunir</option>
          <option value="attention">À vérifier</option>
          <option value="uptodate">À jour</option>
          <option value="nolocal">Pas sur ce poste</option>
        </select>
        <select id="repo-project" class="select" aria-label="Filtrer par projet">
          <option value="">Projet : tous</option>
          <option value="in">Dans un projet</option>
          <option value="out">Hors projet</option>
        </select>
        <select id="repo-branch" class="select" aria-label="Filtrer par branche locale" title="Branche du clone sur ce poste">
          <option value="">Branche : toutes</option>
          <option value="main">Sur la branche par défaut</option>
          <option value="work">Sur une branche de travail (work/*)</option>
          <option value="detached">HEAD détaché</option>
        </select>
        <div class="seg sm" id="repo-view" role="group" aria-label="Présentation">
          <button data-v="grid" title="Grille"${state.repoView === "grid" ? ' class="on"' : ""}>▦ Grille</button>
          <button data-v="list" title="Liste"${state.repoView === "list" ? ' class="on"' : ""}>☰ Liste</button>
        </div>
        <button class="btn btn-primary btn-sm" id="new-repo">＋ Créer un dépôt</button>
      </div>
      <div id="repo-tabs" class="gp-tabs repo-tabs" role="group" aria-label="Emplacement"></div>
      <div id="repo-list">${state.repos ? "" : skeletons(4)}</div>`;
    const risel = el("repo-instance-sel");
    if (risel) risel.onchange = (e) => switchInstance(e.target.value);
    updateRoleBadge();
    el("repo-search").oninput = (e) => { state.search = e.target.value; paintRepos(); };
    el("repo-org").onchange = (e) => { state.org = e.target.value; paintRepos(); };
    el("repo-state").value = state.repoState || "";
    el("repo-state").onchange = (e) => { state.repoState = e.target.value; paintRepos(); };
    el("repo-project").value = state.repoProject || "";
    el("repo-project").onchange = (e) => { state.repoProject = e.target.value; paintRepos(); };
    el("repo-branch").value = state.repoBranch || "";
    el("repo-branch").onchange = (e) => { state.repoBranch = e.target.value; paintRepos(); };
    el("new-repo").onclick = createRepoModal;
    el("repo-tabs").onclick = (ev) => {
      const b = ev.target.closest("[data-tab]"); if (!b || b.disabled) return;
      state.repoTab = b.dataset.tab; paintRepos();
    };
    document.querySelectorAll("#repo-view button").forEach((b) => b.onclick = () => {
      state.repoView = b.dataset.v;
      try { localStorage.setItem("situlab.repoView", state.repoView); } catch (e) {}
      document.querySelectorAll("#repo-view button").forEach((x) => x.classList.toggle("on", x === b));
      paintRepos();
    });
    await loadProjectsIndex(); // index atelier+vitrines (zéro réseau) : allume le bouton « Fiche projet »
    if (state.repos) paintRepos();
  }
  const liveEl = el("live"); if (liveEl) liveEl.classList.add("syncing"); // indicateur « synchro en cours »
  try {
    const data = await App().Dashboard(state.instance);
    state.repos = (data && data.repos) || [];
    state.truncated = !!(data && data.truncated);
    state.checkedAt = Date.now();
    updateCheckedAt();
    const sig = JSON.stringify(state.repos) + state.truncated;
    if (sig !== state.repoSig) { state.repoSig = sig; paintRepos(); }
  } catch (e) {
    if (isLocked(e)) return handleLock();
    if (isDisconnected(state.instance)) {
      const rl = el("repo-list");
      if (rl) { rl.innerHTML = `<div class="surface-error"><span class="se-ico" aria-hidden="true">⚠</span><div class="se-msg"><b>Instance « ${esc(state.instance)} » déconnectée</b><div class="sub">Reconnectez-la pour retrouver vos dépôts.</div></div><button class="btn btn-primary btn-sm" id="go-recon">Reconnecter</button></div>`; const b = el("go-recon"); if (b) b.onclick = () => navTo("settings"); }
      return;
    }
    if (!live && !state.repos) el("repo-list").innerHTML = surfaceError("Impossible de charger les dépôts", cleanErr(e), () => renderRepos(false));
  } finally {
    const l = el("live"); if (l) l.classList.remove("syncing");
  }
}

function paintRepos() {
  const list = el("repo-list"); if (!list) return;
  const repos = state.repos || [];

  const orgs = [...new Set(repos.filter((r) => r.has_remote && r.owner).map((r) => r.owner))].sort();
  const orgSel = el("repo-org");
  if (orgSel && orgSel.dataset.n != String(orgs.length)) { // ne reconstruit pas à chaque tick (évite de fermer le menu ouvert)
    orgSel.dataset.n = String(orgs.length);
    orgSel.innerHTML = `<option value="">Toutes les organisations</option>` +
      orgs.map((o) => `<option value="${esc(o)}"${o === state.org ? " selected" : ""}>${esc(o)}</option>`).join("");
  }

  const q = state.search.trim().toLowerCase();
  // 1) Pré-filtre sur le dépôt BRUT : organisation + recherche texte.
  const pre = repos.filter((r) => {
    if (state.org && r.owner !== state.org) return false;
    if (q && !(r.full_name + " " + r.name + " " + (r.local_path || "")).toLowerCase().includes(q)) return false;
    return true;
  });

  // 2) Regroupement par DÉPÔT DISTANT : un dépôt = UNE carte (ses clones locaux listés dedans).
  // Les dépôts purement locaux (hors instance) restent individuels.
  const byRepo = new Map(), order = [];
  pre.forEach((r) => {
    const key = r.has_remote ? "R:" + r.full_name : "L:" + r.name + "@" + (r.local_path || "");
    let g = byRepo.get(key);
    if (!g) { g = []; byRepo.set(key, g); order.push(key); }
    g.push(r);
  });
  const aggregated = order.map((key) => {
    const views = byRepo.get(key);
    const locals = views.filter((v) => v.local_path);
    let rep = views[0];
    for (const v of views) if (stateRank(v.state) < stateRank(rep.state)) rep = v; // représentant = le plus actionnable
    rep = Object.assign({}, rep); // copie : ne pas altérer l'objet d'origine
    rep._clones = locals;
    if (locals.length > 1) rep.state = locals.reduce((a, v) => stateRank(v.state) < stateRank(a) ? v.state : a, locals[0].state);
    return rep;
  });

  // 3) Filtres État + Projet appliqués au dépôt représentatif.
  const base = aggregated.filter((r) => matchRepoState(r, state.repoState) && matchRepoProject(r, state.repoProject) && matchRepoBranch(r, state.repoBranch));

  // 4) Onglets d'EMPLACEMENT : Ce poste / Atelier / Vitrine. On n'affiche que les onglets qui ont
  //    du contenu (le rôle de la connexion courante décide atelier↔vitrine) + un onglet « Tous ».
  renderRepoTabs(base);
  const shown = base.filter((r) => matchRepoTab(r, state.repoTab));

  if (shown.length === 0) {
    list.innerHTML = repos.length === 0
      ? blankslate("◍", "Aucun dépôt", "Votre token ne donne accès à aucun dépôt sur cette instance.")
      : blankslate("⌕", "Aucun résultat", "Aucun dépôt ne correspond à vos filtres. Élargissez la recherche ou réinitialisez les filtres.");
    return;
  }

  const gridClass = state.repoView === "list" ? "repo-grid list" : "repo-grid";
  const content = document.querySelector(".content");
  const scroll = content ? content.scrollTop : 0;
  let html = state.truncated ? `<div class="offline-banner">Beaucoup de dépôts : la liste affichée est partielle.</div>` : "";
  html += `<div class="${gridClass}">` + shown.map((r, i) => repoCard(r, i)).join("") + `</div>`;
  list.innerHTML = html;
  // Délégation sur le conteneur STABLE #repo-list (survit aux repaints « en direct »), lecture via
  // state._shown (jamais de closure périmée) : « Voir détail », « Fiche projet » et l'ouverture d'un clone.
  state._shown = shown;
  list.onclick = (ev) => {
    const p = ev.target.closest("[data-proj]");
    if (p) { openProject(p.dataset.proj); return; }
    const g = ev.target.closest("[data-get]");
    if (g) { const r = state._shown[+g.dataset.get]; if (r) getRepoHereModal(r); return; }
    const o = ev.target.closest("[data-open]");
    if (o) { const r = state._shown[+o.dataset.i]; if (r) openDetail(r); return; }
  };
  if (content) content.scrollTop = scroll;
}

// matchRepoState : filtre « État » du catalogue. "attention" couvre detached + unreadable.
function matchRepoState(r, f) {
  if (!f) return true;
  const s = r.state || "nolocal";
  if (f === "attention") return s === "detached" || s === "unreadable";
  return s === f;
}

// matchRepoBranch : filtre « Branche locale » — sur quelle branche se trouve le clone de CE poste.
// Zéro réseau (champs local_branch/detached/default_branch déjà portés par le dashboard). Un filtre
// actif exclut les dépôts sans clone local (il n'y a pas de branche locale à comparer). Pour un dépôt
// à plusieurs clones, on retient la correspondance si AU MOINS un clone correspond.
function matchRepoBranch(r, f) {
  if (!f) return true;
  const clones = (r._clones && r._clones.length) ? r._clones : (r.local_path ? [r] : []);
  if (!clones.length) return false;
  const def = r.default_branch || "";
  return clones.some((c) => {
    if (f === "detached") return !!c.detached;
    if (c.detached) return false;
    const b = c.local_branch || "";
    if (f === "work") return b.indexOf("work/") === 0;
    if (f === "main") return b !== "" && (b === def || b === "main" || b === "master");
    return false;
  });
}

// matchRepoProject : filtre « Projet » — "in" = appartient à un projet, "out" = n'y appartient pas.
function matchRepoProject(r, f) {
  if (!f) return true;
  const inProj = !!repoProjectName(r);
  return f === "in" ? inProj : !inProj;
}

// repoHasLocal : ce dépôt est-il présent sur CE poste (cloné / worktree) ?
function repoHasLocal(r) { return !!(r._clones && r._clones.length) || !!r.local_path; }

// matchRepoTab : onglets de PRÉSENCE (modèle honnête : une connexion = un seul rôle, inutile de
// prétendre deux onglets distants). "ici" = présent sur ce poste ; "online" = en ligne mais pas
// encore ici. Le monde (atelier/vitrine) est dit par la COULEUR de l'onglet, pas par un onglet mort.
function matchRepoTab(r, tab) {
  if (!tab) return true;
  if (tab === "ici") return repoHasLocal(r);
  if (tab === "online") return !!r.has_remote && !repoHasLocal(r);
  return true;
}

// renderRepoTabs : barre d'onglets de présence avec compteurs. L'onglet « En ligne » prend la couleur
// et l'icône du rôle de la connexion courante (atelier=rose/forgejo · vitrine=violet/github). Un onglet
// vidé par les filtres se désactive ; l'onglet actif alors devenu vide retombe sur « Tous ».
function renderRepoTabs(base) {
  const bar = el("repo-tabs"); if (!bar) return;
  const count = (tab) => base.filter((r) => matchRepoTab(r, tab)).length;
  const prod = roleOf(state.instance) === "production";
  const tabs = [
    { key: "", label: "Tous", cls: "", ico: "", n: base.length, tip: "Tous les dépôts" },
    { key: "ici", label: "Sur ce poste", cls: "gp-tab-local", ico: ZICON.desktop, n: count("ici"), tip: "Dépôts copiés sur ce poste" },
    { key: "online", label: "En ligne", cls: prod ? "gp-tab-vitrine" : "gp-tab-atelier", ico: prod ? ZICON.github : ZICON.forgejo, n: count("online"), tip: (prod ? "En vitrine" : "Dans l'atelier") + ", pas encore sur ce poste" },
  ];
  const active = tabs.find((t) => t.key === state.repoTab);
  if (!active || (state.repoTab && active.n === 0)) state.repoTab = "";
  bar.innerHTML = tabs.map((t) => {
    const dis = t.key && t.n === 0;
    const on = t.key === state.repoTab;
    return `<button type="button" class="gp-tab ${t.cls}${on ? " on" : ""}" data-tab="${t.key}"${dis ? " disabled" : ""} aria-pressed="${on}" title="${esc(t.tip)}">${t.ico}${esc(t.label)}<span class="repo-tab-n">${t.n}</span></button>`;
  }).join("");
}

// loadProjectsIndex : construit l'index « instance/owner/repo → nom de projet » (atelier ET vitrines),
// en lecture de config pure (zéro réseau). Sert à allumer le bouton « Fiche projet » d'un dépôt.
async function loadProjectsIndex() {
  try {
    const projs = (await App().Projects()) || [];
    const idx = new Map();
    const add = (ref, name) => { if (ref && ref.instance && ref.owner && ref.repo) idx.set(ref.instance + "/" + ref.owner + "/" + ref.repo, name); };
    for (const p of projs) { add(p.atelier, p.name); (p.productions || []).forEach((pr) => add(pr, p.name)); }
    state.projectsIndex = idx;
  } catch (e) { /* non bloquant : simplement pas de bouton « Fiche projet » */ }
}

// repoProjectName : nom du projet auquel ce dépôt appartient (via l'index), "" sinon. Un dépôt du
// catalogue est sur l'instance courante ; sa clé est « instance courante / owner-repo ».
function repoProjectName(r) {
  const idx = state.projectsIndex;
  if (!idx || !r.has_remote || !r.full_name) return "";
  return idx.get(state.instance + "/" + r.full_name) || "";
}

// createRepoModal : créer un dépôt distant. preset (facultatif) = { instance, name, owner } pour
// cibler une connexion précise (ex. depuis l'organiseur : « créer l'atelier/la vitrine »).
function createRepoModal(preset) {
  preset = (preset && typeof preset === "object" && !preset.type) ? preset : {};
  if (preset.instance) state.instance = preset.instance;
  // Propriétaires possibles : moi (espace perso) + les organisations vues dans mes dépôts.
  const orgs = [...new Set((state.repos || []).filter((r) => r.has_remote && r.owner && r.owner !== state.login).map((r) => r.owner))].sort();
  const ownerOpts = `<option value="">Moi (${esc(state.login || "espace personnel")})</option>` +
    orgs.map((o) => `<option value="${esc(o)}">${esc(o)} (organisation)</option>`).join("");
  openModal("Créer un dépôt", `<div class="field">
    <label>Propriétaire<select id="nowner" class="select">${ownerOpts}</select>
      <small>Beaucoup de serveurs interdisent les dépôts personnels : choisissez une organisation si « Moi » est refusé.</small></label>
    <label>Nom<input id="nr" placeholder="mon-projet" value="${esc(preset.name || "")}" /></label>
    <label>Description<input id="nd" /></label>
    <label style="flex-direction:row;align-items:center"><input id="npv" type="checkbox" style="width:auto;margin-right:8px" checked />Dépôt privé</label>
    <div class="inline-error hidden" id="nr-error"></div>
    ${aiInviteHTML()}</div>`, async () => {
    const er = el("nr-error");
    const owner = el("nowner").value, nr = el("nr").value.trim(), nd = el("nd").value, npv = el("npv").checked;
    if (!nr) { er.textContent = "Donnez un nom au dépôt."; er.classList.remove("hidden"); return; }
    const where = owner ? `dans « ${owner} »` : "dans votre espace personnel";
    pinConfirm(`Créer le dépôt « ${nr} » ${where}`, async (pin) => {
      await App().CreateRepo(state.instance, owner, nr, nd, npv, pin);
      closeModal(); toast("Dépôt créé", "ok"); afterRepoChange();
    });
  });
  // Présélection du propriétaire : celui demandé (preset) s'il est connu, sinon la 1re organisation
  // (l'espace perso est souvent bloqué).
  if (preset.owner && orgs.includes(preset.owner)) el("nowner").value = preset.owner;
  else if (orgs.length) el("nowner").value = orgs[0];
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => promptRepoCreate(el("nowner") ? el("nowner").value : "", el("nr") ? el("nr").value : ""));
  // Compléter avec les organisations RÉELLES de l'utilisateur (async, best-effort) — pas seulement celles
  // déjà vues dans ses dépôts : sinon impossible de créer un dépôt dans une organisation encore vide.
  (async () => {
    let real = [];
    try { real = (await App().Orgs(state.instance)) || []; } catch (e) { return; }
    const sel = el("nowner"); if (!sel) return;
    const names = real.map((o) => o && (o.username || o.login)).filter(Boolean);
    const merged = [...new Set([...orgs, ...names])].sort();
    if (!merged.length) return;
    const keep = sel.value;
    sel.innerHTML = `<option value="">Moi (${esc(state.login || "espace personnel")})</option>` +
      merged.map((o) => `<option value="${esc(o)}">${esc(o)} (organisation)</option>`).join("");
    if (keep && merged.includes(keep)) sel.value = keep;
    else if (preset.owner && merged.includes(preset.owner)) sel.value = preset.owner;
  })();
}

// deleteRepoModal : suppression définitive d'un dépôt distant, avec garde-fou (retaper le
// nom), invite IA, et confirmation par PIN.
function deleteRepoModal(owner, repo) {
  const slug = `${owner}/${repo}`;
  openModal("Supprimer le dépôt", `<div class="field">
    <div class="callout warn"><b>Action irréversible.</b> Le dépôt « ${esc(slug)} » et tout son contenu seront supprimés du serveur.</div>
    <label>Pour confirmer, tapez <b>${esc(repo)}</b><input id="dr-confirm" placeholder="${esc(repo)}" /></label>
    <div class="inline-error hidden" id="dr-error"></div>
    ${aiInviteHTML("Vous pouvez aussi déléguer la suppression à votre IA. Copiez le prompt et collez-le dans votre IA — SituLab gère seul la validation si elle est nécessaire.")}</div>`, async () => {
    const er = el("dr-error");
    if (el("dr-confirm").value.trim() !== repo) { er.textContent = "Le nom saisi ne correspond pas."; er.classList.remove("hidden"); return; }
    pinConfirm(`Supprimer définitivement « ${slug} »`, async (pin) => {
      await App().DeleteRepo(state.instance, owner, repo, pin);
      closeModal(); toast("Dépôt supprimé", "ok"); state.repoSig = ""; navTo("organize");
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => promptRepoDelete(owner, repo));
}

// afterRepoChange : après un clone / une liaison, rafraîchit la surface vivante. La page « Dépôts »
// ayant été retirée (1.11), ces gestes sont désormais déclenchés depuis l'organiseur (Projets ▸
// Organiser mes dépôts) — on le repeint s'il est à l'écran.
function afterRepoChange() {
  state.repoSig = "";
  if (state.view === "organize" && typeof renderOrganize === "function") renderOrganize();
}

// getRepoHereModal : « Récupérer sur ce poste » — action CONTEXTUELLE posée sur la carte d'un dépôt
// présent en ligne mais pas encore copié ici (1.9.11). Trois voies : cloner ici (par défaut), lier
// un dossier déjà présent, ou déléguer à l'IA. Après l'action, le catalogue se resynchronise.
function getRepoHereModal(r) {
  const { owner, repo } = splitFull(r.full_name);
  // Plug-and-play (1.12) : nom pré-rempli depuis le dépôt (éditable), emplacement choisi au sélecteur
  // natif. Aucune saisie de chemin à la main ; le chemin final est composé et affiché en aperçu.
  let base = "";                 // dossier parent choisi (sélecteur natif)
  const sep = () => (base.includes("\\") || /^[A-Za-z]:/.test(base) ? "\\" : "/");
  const destOf = () => { const n = (el("gr-name") && el("gr-name").value.trim()) || repo; return base ? base + sep() + n : ""; };
  const refreshPreview = () => { const p = el("gr-dest"); if (p) p.textContent = destOf() || "— choisissez d'abord un emplacement —"; };
  openModal("Récupérer « " + repo + " » sur ce poste", `<div class="field">
    <p class="sub">Copier le dépôt <b>${esc(r.full_name)}</b> sur ce poste. SituLab le relie et le synchronise aussitôt — rien à taper.</p>
    <label>Nom du dossier<input id="gr-name" class="inp" value="${esc(repo)}" autocomplete="off" spellcheck="false" /></label>
    <div class="gr-loc">
      <button class="btn btn-ghost btn-sm" type="button" id="gr-pick">📁 Choisir l'emplacement…</button>
      <span class="gr-dest-wrap">→ <code id="gr-dest" class="gr-dest">— choisissez d'abord un emplacement —</code></span>
    </div>
    <div class="inline-error hidden" id="gr-err"></div>
    <div class="link-opt">
      <div class="link-opt-h">Déjà présent ailleurs sur ce poste ?</div>
      <button class="btn btn-ghost btn-sm" type="button" id="gr-link">📁 Lier un dossier existant…</button>
    </div>
    <div class="link-opt">
      <div class="link-opt-h">Ou laissez votre IA le faire</div>
      <button class="btn btn-ghost btn-sm" type="button" id="gr-prompt">Copier le prompt pour mon IA</button>
    </div></div>`, async () => {
    const dir = destOf(), er = el("gr-err");
    if (!base) { er.textContent = "Choisissez l'emplacement avec le bouton ci-dessus."; er.classList.remove("hidden"); return; }
    if (!(el("gr-name").value.trim())) { er.textContent = "Indiquez un nom de dossier."; er.classList.remove("hidden"); return; }
    try {
      await App().Clone(state.instance, owner, repo, dir);
      closeModal(); toast("Dépôt récupéré sur ce poste", "ok"); afterRepoChange();
    } catch (e) { if (isLocked(e)) return handleLock(); er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  el("modal-ok").textContent = "Cloner ici";
  el("gr-name").oninput = refreshPreview;
  el("gr-pick").onclick = async () => {
    try { const p = await App().PickFolder(); if (p) { base = p; refreshPreview(); } }
    catch (e) { toast(cleanErr(e), "err"); }
  };
  el("gr-link").onclick = () => { closeModal(); linkLocalPick(); };
  el("gr-prompt").onclick = () => copyLinkLocalPrompt();
}

// linkLocalPick : sélecteur de dossier natif puis enregistrement (SituLab vérifie que c'est
// bien un dépôt git et le rattache à son équivalent en ligne par l'URL d'origine).
async function linkLocalPick() {
  let path;
  try { path = await App().PickFolder(); }
  catch (e) { toast(cleanErr(e), "err"); return; }
  if (!path) return; // annulé
  try {
    await App().AddLocalRepo(path);
    toast("Dépôt local lié", "ok");
    afterRepoChange();
  } catch (e) {
    if (isLocked(e)) return handleLock();
    openModal("Impossible de lier ce dossier", `<div class="field"><div class="callout warn">${esc(cleanErr(e))}</div><p class="sub">Choisissez le dossier racine d'un dépôt git (celui qui contient le sous-dossier <b>.git</b>).</p></div>`, () => closeModal());
    el("modal-cancel").classList.add("hidden"); el("modal-ok").textContent = "Compris";
  }
}

// linkLocalPromptText : prompt prêt à coller pour déléguer la liaison des dépôts locaux à l'IA.
function linkLocalPromptText() {
  return [
    "Relie mes dépôts git locaux à SituLab.",
    "",
    "- Parcours ce poste et repère les dossiers qui sont des dépôts git (ceux qui contiennent un sous-dossier .git).",
    "- Pour chacun, exécute : situlab local add \"<chemin absolu du dossier>\"",
    "- La commande est idempotente : un dossier déjà suivi n'est pas dupliqué.",
    "- À la fin, lance `situlab dashboard --output json` et résume-moi ce qui est relié.",
    "",
    "Si tu ne sais pas où chercher, demande-moi où se trouvent mes clones.",
  ].join("\n");
}

async function copyLinkLocalPrompt() {
  copyPrompt(linkLocalPromptText());
}

function stateBadge(code) {
  const s = STATES[code] || STATES.nolocal;
  return `<span class="state ${s.cls}" title="${esc(s.tip)}"><span class="ico" aria-hidden="true">${s.ico}</span>${esc(s.label)}</span>`;
}

// Clé UNIQUE par carte : inclut le chemin local, car un même dépôt distant peut avoir PLUSIEURS
// clones locaux (donc plusieurs cartes). Sans ça, déplier/mettre en cache l'un affectait l'autre.
function repoKey(r) { return (r.has_remote ? r.full_name : r.name) + "@" + (r.local_path || ""); }

// repoTitleHTML : met le NOM du dépôt en avant et l'organisation en préfixe discret. Pour un
// non-initié, un dépôt est d'abord un dossier ; « LiG/KERNEL » devient « LiG/ » (gris) + « KERNEL » (fort).
function repoTitleHTML(fullName) {
  const i = (fullName || "").indexOf("/");
  if (i < 0) return `<span class="nm">${esc(fullName || "")}</span>`;
  return `<span class="nm-org">${esc(fullName.slice(0, i))}/</span><span class="nm">${esc(fullName.slice(i + 1))}</span>`;
}
// folderOf : dernier segment du chemin local (pour distinguer deux clones du même dépôt).
function folderOf(p) { return p ? p.split(/[\\/]/).filter(Boolean).pop() : ""; }
// stateRank : ordre d'actionnabilité (plus petit = plus urgent). Sert à choisir l'état
// représentatif d'un dépôt qui a plusieurs clones.
const STATE_ORDER = ["dirty", "diverged", "ahead", "behind", "detached", "unreadable", "uptodate", "nolocal"];
function stateRank(s) { const i = STATE_ORDER.indexOf(s || "nolocal"); return i < 0 ? 99 : i; }

// projBtnHTML : bouton « Fiche projet » si le dépôt appartient à un projet, "" sinon.
function projBtnHTML(r) {
  const name = repoProjectName(r);
  return name ? `<button class="btn btn-ghost btn-sm repo-proj-btn" data-proj="${esc(name)}" title="Ouvrir la fiche du projet « ${esc(name)} »">🗂 Fiche projet</button>` : "";
}

// repoPresenceHTML : pastilles d'EMPLACEMENT du dépôt — 🖥 ce poste + le distant, coloré selon le
// RÔLE de la connexion courante (atelier=rose · vitrine=violet). Pip éteint = absent. Dit d'un coup
// d'œil « sur ce poste et dans l'atelier », « que dans l'atelier », « que dans la vitrine », etc.
function repoPresenceHTML(r) {
  const kind = roleOf(state.instance) === "production" ? "vitrine" : "atelier";
  const remoteIco = kind === "vitrine" ? ZICON.github : ZICON.forgejo;
  const remoteLbl = kind === "vitrine" ? "la vitrine" : "l'atelier";
  const pip = (cls, ico, on, label) => `<span class="gp-pip ${cls} ${on ? "on" : "off"}" title="${esc(label)}">${ico}</span>`;
  return `<span class="gp-scope" aria-hidden="true">${
    pip("gp-pip-local", ZICON.desktop, !!r.local_path, r.local_path ? "présent sur ce poste" : "pas sur ce poste")
  }${
    pip("gp-pip-" + kind, remoteIco, !!r.has_remote, r.has_remote ? ("présent sur " + remoteLbl) : ("absent de " + remoteLbl))
  }</span>`;
}

// États LOCAUX réellement actionnables : on n'affiche une pastille d'état que pour ceux-là. « À jour »
// et « pas sur ce poste » ne sont plus des badges — les pastilles d'emplacement les disent déjà.
const ACTION_STATES = ["dirty", "ahead", "behind", "diverged", "detached", "unreadable"];
function actionStateHTML(r) { return ACTION_STATES.includes(r.state) ? stateBadge(r.state) : ""; }

// repoCard : carte ÉPURÉE du catalogue (1.9.11). Identité du dépôt + état d'un coup d'œil + dossier
// s'il est copié ici. Deux portes seulement : « Fiche projet » (si applicable) et « Voir détail ».
// Tout le reste (branches, envois, versions) vit dans le détail ou dans Projets.
function repoCard(r, i) {
  const title = r.has_remote ? r.full_name : r.name;
  const vis = r.has_remote
    ? `<span class="pill">${r.private ? "privé" : "public"}</span>`
    : `<span class="pill">local</span>`;
  const st = actionStateHTML(r);
  // Un dépôt = UNE carte. S'il est copié plusieurs fois sur ce poste, on l'indique d'un compteur
  // discret (les copies et worktrees se gèrent dans le détail) — jamais une liste sur le catalogue.
  const nClones = (r._clones && r._clones.length) || 0;
  const viaAtl = r.via_atelier ? `<span class="pill sm via-atelier" title="Vous l'avez déjà sur ce poste via le clone d'atelier du projet — rien à récupérer">⇄ via l'atelier</span>` : "";
  const copies = nClones > 1
    ? `<span class="pill pill-folder" title="Ce dépôt est copié ${nClones} fois sur ce poste">📁 ${nClones} copies</span>${viaAtl}`
    : r.local_path
      ? `<span class="pill pill-folder" title="dossier local : ${esc(r.local_path)}">📁 ${esc(folderOf(r.local_path))}</span>${viaAtl}`
      : "";
  const ident = copies
    ? `<div class="repo-ident">${copies}${st}</div>`
    : r.local_error
      ? `<div class="repo-ident muted">${esc(r.local_error)}${st}</div>`
      : (st ? `<div class="repo-ident">${st}</div>` : "");
  // Dépôt en ligne pas encore sur ce poste → action contextuelle « Récupérer ici » (clone + synchro).
  const getBtn = (r.has_remote && !r.local_path)
    ? `<button class="btn btn-ghost btn-sm repo-get-btn" data-get="${i}" title="Cloner / lier ce dépôt sur ce poste">📁 Récupérer ici</button>` : "";
  return `<div class="repo-card ${(STATES[r.state] || STATES.nolocal).cls}" data-key="${esc(repoKey(r))}" data-i="${i}">
    <div class="repo-head repo-head-static">
      <div class="repo-title">${r.has_remote ? repoTitleHTML(title) : `<span class="nm">${esc(title)}</span>`} ${vis}</div>
      ${repoPresenceHTML(r)}
    </div>
    ${ident}
    <div class="repo-foot">${projBtnHTML(r)}${getBtn}<button class="btn btn-ghost btn-sm" data-open data-i="${i}">Voir détail ›</button></div>
  </div>`;
}

// cleanTag : extrait le TAG d'un `git describe` (« v1.2.0-5-gabc123 » → « v1.2.0 ») et renvoie ""
// pour un SHA nu — on ne montre jamais de numéro de commit sur une carte (v1.5.0).
function cleanTag(desc) {
  if (!desc) return "";
  const m = desc.match(/^(.*)-\d+-g[0-9a-f]+$/i);
  if (m) return m[1];
  if (/^[0-9a-f]{7,40}$/i.test(desc)) return "";
  return desc;
}

// branchMatrixHTML : graphe visuel « à voies ». Une ligne par branche ; deux nœuds
// « En ligne » et « Ce poste » reliés par un segment dont la direction/couleur dit l'écart
// (↑ à envoyer, ↓ à récupérer, = à jour). Tags de version + dates, jamais de n° de commit.
function branchMatrixHTML(m) {
  if (!m || !m.is_repo || !(m.branches || []).length) return `<div class="muted" style="padding:6px 0">Aucune branche.</div>`;
  // D'abord les branches PRÉSENTES SUR CE POSTE ; les autres (en ligne seulement) sont rangées dans
  // un accordéon discret, avec qui les a ouvertes (dernier auteur).
  const locals = m.branches.filter((b) => b.has_local);
  const remoteOnly = m.branches.filter((b) => b.has_remote && !b.has_local);
  let html = "";
  if (locals.length) {
    html += `<div class="branch-graph">` + locals.map((b) => branchLane({
      name: b.name, current: b.current, hasLocal: b.has_local, hasRemote: b.has_remote,
      ahead: b.ahead, behind: b.behind,
      remoteVer: b.remote_version, remoteDate: b.remote_date, localVer: b.local_version, localDate: b.local_date,
    })).join("") + `</div>`;
  } else {
    html += `<div class="muted" style="padding:6px 0">Aucune branche sur ce poste.</div>`;
  }
  if (remoteOnly.length) {
    const n = remoteOnly.length;
    html += `<details class="branch-more"><summary><span class="bm-caret">▸</span> ${n} autre${n > 1 ? "s" : ""} branche${n > 1 ? "s" : ""} en ligne — pas sur ce poste</summary>
      <div class="branch-more-list">` + remoteOnly.map((b) =>
      `<div class="bm-row"><span class="bm-name">⎇ ${esc(b.name)}</span>${b.author ? `<span class="bm-by">ouverte par ${esc(b.author)}</span>` : ""}${b.remote_date ? `<span class="bm-when">${esc(fmtWhen(b.remote_date))}</span>` : ""}</div>`).join("") + `</div></details>`;
  }
  const foot = m.fetched_at ? `<div class="bg-foot">État « en ligne » connu au ${esc(fmtWhen(m.fetched_at))} — récupérez les nouveautés pour l'actualiser.</div>` : "";
  return html + foot;
}


// branchLane : une voie du graphe. b = {name, current, hasLocal, hasRemote, ahead, behind,
// remoteVer, remoteDate, localVer, localDate}. Rendu HTML/CSS (responsive, zéro dépendance).
function branchLane(b) {
  const tag = cleanTag(b.localVer) || cleanTag(b.remoteVer);
  const tagPill = tag ? `<span class="pill sm ver" title="version publiée">${esc(tag)}</span>` : "";
  let cur = b.current ? `<span class="pill sm">ici</span>` : "";
  // Relation entre les deux côtés.
  let linkCls = "even", linkTxt = "";
  if (b.hasRemote && !b.hasLocal) { linkCls = "remote-only"; linkTxt = "pas sur ce poste"; }
  else if (b.hasLocal && !b.hasRemote) { linkCls = "local-only"; linkTxt = "pas en ligne"; }
  else if (b.ahead && b.behind) { linkCls = "diverged"; linkTxt = `↑${b.ahead} ↓${b.behind} à réunir`; }
  else if (b.ahead) { linkCls = "ahead"; linkTxt = `↑ ${b.ahead} à envoyer`; }
  else if (b.behind) { linkCls = "behind"; linkTxt = `↓ ${b.behind} à récupérer`; }
  else { linkCls = "even"; linkTxt = "à jour"; }
  const node = (on, k) => {
    const present = on ? b.hasRemote : b.hasLocal;
    const date = on ? b.remoteDate : b.localDate;
    return `<div class="bg-node ${present ? "" : "absent"}">
      <span class="bg-dot"></span><span class="bg-k">${k}</span>
      <span class="bg-when">${present ? (date ? "maj " + esc(fmtWhen(date)) : "") : "—"}</span></div>`;
  };
  return `<div class="bg-lane ${linkCls}">
    <div class="bg-head"><span class="bg-ico">⎇</span> <b>${esc(b.name)}</b> ${cur} ${tagPill}</div>
    <div class="bg-track">
      ${node(true, "En ligne")}
      <div class="bg-link"><span class="bg-line"></span><span class="bg-badge">${linkTxt}</span></div>
      ${node(false, "Ce poste")}
    </div>
  </div>`;
}

