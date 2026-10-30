/* ---------------- Shell ---------------- */
// applyModules : applique les groupes de modules d'interface (v1.13) — masque/affiche les menus de
// domaine et pilote le « git avancé ». Un groupe absent/true = activé (tout montré par défaut).
function applyModules() {
  const m = state.modules || {};
  const on = (k) => m[k] !== false;
  state.advanced = on("git_advanced"); // « Git avancé » = l'ancien mode avancé
  // Le module « agents » pilote l'entrée « Agent IA » du menu principal (v1.17, besoin #1).
  [["corpus", "corpus"], ["spokes", "spokes"], ["backup", "backup"], ["agents", "ia"]].forEach(([mod, view]) => {
    const btn = document.querySelector(`.nav-item[data-view="${view}"]`);
    if (btn) btn.classList.toggle("hidden", !on(mod));
  });
  // Si la vue courante vient d'être masquée, on retombe sur KERNEL (toujours visible).
  if ((state.view === "corpus" && !on("corpus")) || (state.view === "spokes" && !on("spokes")) || (state.view === "backup" && !on("backup")) || (state.view === "ia" && !on("agents"))) {
    if (typeof navTo === "function") navTo("kernel");
  }
}


async function showApp(instances) {
  hideBoot();
  el("onboarding").classList.add("hidden");
  el("app").classList.remove("hidden");

  state.instances = instances || [];
  state.instance = (instances.find((i) => i.default) || instances[0]).name;

  document.querySelectorAll(".nav-item[data-view]").forEach((b) => { b.onclick = () => navTo(b.dataset.view); });

  state.readOnly = await App().ReadOnly().catch(() => false);
  updateReadOnlyBanner();
  state.modules = await App().Modules().catch(() => ({}));
  applyModules();

  // Revenir à la dernière vue de 1er niveau (ex. après une reconnexion admin qui recharge la page).
  try { const gv = sessionStorage.getItem("situlab.view"); if (gv && TOP_VIEWS.includes(gv)) state.view = gv; } catch (e) {}
  // Restauration des onglets de projets (P3) : on repropose la session d'avant fermeture. Le projet
  // actif est rouvert s'il figure encore dans les onglets mémorisés.
  try {
    state.tabs = loadTabs();
    const at = localStorage.getItem("situlab.activeTab");
    if (state.tabs.length && at && state.tabs.includes(at)) { state.projectName = at; state.view = "project"; }
  } catch (e) {}

  // Mise en route canalisante (1.13) : décide la vue d'atterrissage ; gardée AVANT le rendu (lectures
  // locales : matrice, kernels, spokes, backups — pas d'allers-retours réseau).
  await refreshSetup();
  if (setupMustLand()) state.view = "setup"; // 1.18.5 : on n'atterrit de force que pour le nouvel arrivant (bases manquantes)

  // Peinture IMMÉDIATE avec le strict nécessaire local (modules → nav, lecture seule, vue). On n'enchaîne
  // PLUS les appels réseau (Whoami, santé cloud, Doctor) AVANT le premier rendu — ils bloquaient la
  // première peinture (perf 1.17, Point 2). Ils enrichissent ensuite en arrière-plan ; refreshWhoami
  // re-rend tout seul si l'état « en ligne » bascule, et les pastilles se peignent une fois les données là.
  render();
  startLive();
  startAutoFetch(); // récupération de fond des nouveautés distantes (noyau : FetchAllLocal)
  if (typeof installBackupProgress === "function") installBackupProgress(); // suivi visuel permanent des sauvegardes

  Promise.allSettled([
    updateGitBanner(),
    refreshDisconnected(),
    refreshWhoami(),
    (typeof refreshPairingAlert === "function" ? refreshPairingAlert() : Promise.resolve()),
  ]).then(async () => {
    updateHealth(); // pastilles de santé git + cloud, une fois les données fraîches
    // Mise en route recalculée avec le compte d'appairages RÉEL (refreshPairingAlert vient de l'actualiser) :
    // la première peinture s'est faite sur le cache pour ne pas attendre le réseau. On réconcilie la vue
    // d'atterrissage si l'état a basculé (entrée dans la mise en route, ou sortie une fois résolue).
    const wasSetupView = state.view === "setup";
    if (typeof refreshSetup === "function") await refreshSetup();
    if (setupMustLand() && !wasSetupView) { state.view = "setup"; render(); }
    else if (!setupActive() && wasSetupView) { state.view = "organize"; render(); }
    if (!setupActive()) maybeAutoTour(); // visite auto : seulement une fois la mise en route terminée
  });
}

// updateGitBanner affiche un bandeau si git est absent du poste (SituLab pilote git, il ne le
// remplace pas) avec un lien d'installation. Sans git, l'essentiel (clone, synchro, versions) est
// indisponible : mieux vaut le dire clairement qu'échouer opération par opération.
async function updateGitBanner() {
  let absent = false;
  try { const d = await App().Doctor(); absent = d && d.git_available === false; } catch (e) {}
  let b = el("git-banner");
  if (absent) {
    if (!b) {
      b = document.createElement("div");
      b.id = "git-banner";
      b.className = "ro-banner git-banner";
      b.innerHTML = `⚠ git n'est pas installé sur ce poste : SituLab pilote git, il ne peut pas fonctionner sans. ` +
        `<a class="lnk" data-ext="https://git-scm.com/downloads">Installer git</a>, puis rouvrez SituLab.`;
      el("app").prepend(b);
    }
  } else if (b) {
    b.remove();
  }
}

// Auto-fetch : récupère périodiquement les refs distantes (noyau FetchAllLocal, best-effort, sans
// PIN) pour que l'état « nouveautés en ligne » se mette à jour tout seul. Suspendu quand la fenêtre
// est masquée. Toute la logique est dans le noyau ; ici, juste un minuteur.
const AUTOFETCH_MS = 7 * 60 * 1000; // 7 min (fenêtre 5–10 min demandée)
let autoFetchTimer = null;
let autoFetchBusy = false; // garde de réentrance : setInterval n'attend pas la fin de l'appel async
async function autoFetchOnce() {
  if (document.hidden || autoFetchBusy) return;
  autoFetchBusy = true;
  try { await App().FetchAllLocal(); } catch (e) { /* best-effort : jamais bloquant */ }
  finally { autoFetchBusy = false; }
}
function startAutoFetch() {
  if (autoFetchTimer) clearInterval(autoFetchTimer);
  autoFetchTimer = setInterval(autoFetchOnce, AUTOFETCH_MS);
}

// roleOf renvoie le rôle effectif d'une connexion par son nom (défaut : atelier).
function roleOf(name) {
  const inst = (state.instances || []).find((i) => i.name === name);
  return inst && inst.role === "production" ? "production" : "atelier";
}
// refreshInstances recharge la liste des connexions (rôle inclus) et met à jour le header
// INSTANTANÉMENT après un changement de réglage — plus besoin de redémarrer l'application.
async function refreshInstances() {
  try {
    const list = await App().GetInstances();
    if (!list) return;
    state.instances = list;
    // Le sélecteur d'instance vit désormais sur la page Dépôts (1.11). On le met à jour s'il est affiché.
    const sel = el("repo-instance-sel");
    if (sel) sel.innerHTML = list.map((i) => `<option value="${esc(i.name)}"${i.name === state.instance ? " selected" : ""}>${esc(i.name)}</option>`).join("");
    updateRoleBadge();
    updateHealth();
  } catch (e) {}
}

// switchInstance change la connexion git active (depuis le sélecteur de la page Dépôts) et route vers
// Dépôts en connaissant l'état en ligne (pas de flash d'erreur).
async function switchInstance(name) {
  state.instance = name;
  state.detail = null; state.repos = null; state.repoSig = ""; state.online = undefined;
  updateRoleBadge();
  await refreshWhoami();
  updateHealth();
  navTo("organize");
}

// updateRoleBadge reflète le rôle de la connexion active (atelier/production) là où le badge est
// affiché (page Dépôts en 1.11 ; absent ailleurs → no-op).
function updateRoleBadge() {
  const b = el("role-badge"); if (!b) return;
  const r = roleOf(state.instance);
  b.textContent = r === "production" ? "VITRINE" : "ATELIER";
  b.className = "role-badge " + (r === "production" ? "role-production" : "role-atelier");
}

// --- Pastilles de santé des connexions (1.11) --------------------------------------------------
// Deux pastilles (git + cloud), vert/orange/rouge, qui disent d'un coup d'œil si tout va bien et
// proposent d'aller se reconnecter au bon endroit. Remplace l'ancien indicateur « en direct ».

// healthTone : 'green' si rien de cassé, 'red' si tout est cassé, 'orange' si une partie l'est.
function healthTone(bad, total) {
  if (total === 0) return "none";
  if (bad === 0) return "green";
  if (bad >= total) return "red";
  return "orange";
}

function healthLabel(kind, tone, bad, total) {
  if (tone === "green") return (kind === "git" ? "Git" : "Cloud") + " ✓";
  if (tone === "red") return (kind === "git" ? "Git" : "Cloud") + " ✗";
  return (kind === "git" ? "Git" : "Cloud") + " " + (total - bad) + "/" + total;
}

// Cache court de la santé cloud (perf 1.17) : App().CloudHealth() teste chaque cloud par le réseau, et
// plusieurs vues la réclament en rafale (pastilles d'en-tête + CORPUS + Réglages, souvent coup sur coup).
// On mutualise le résultat quelques secondes et on dédoublonne les appels concurrents. Invalidé après une
// mutation de cloud (ajout/suppression) ; sinon le TTL borne la fraîcheur.
let _clHealth = { at: 0, data: null, inflight: null };
const CLOUD_HEALTH_TTL = 15000;
async function cloudHealthCached(force) {
  if (!force && _clHealth.data && (Date.now() - _clHealth.at) < CLOUD_HEALTH_TTL) return _clHealth.data;
  if (_clHealth.inflight) return _clHealth.inflight; // un seul ping en vol à la fois
  _clHealth.inflight = App().CloudHealth().then((d) => {
    _clHealth = { at: Date.now(), data: d || [], inflight: null };
    return _clHealth.data;
  }).catch((e) => { _clHealth.inflight = null; throw e; });
  return _clHealth.inflight;
}
function invalidateCloudHealth() { _clHealth = { at: 0, data: null, inflight: null }; }

let healthBusy = false;
async function updateHealth() {
  const box = el("health");
  if (!box) return;
  if (healthBusy) return;
  healthBusy = true;
  try {
    // Git : d'après les connexions configurées et celles sans accès valide (state.disconnected).
    const insts = state.instances || [];
    const disc = state.disconnected || [];
    const gitTotal = insts.length;
    const gitBad = insts.filter((i) => disc.includes(i.name)).length;
    const gitTone = healthTone(gitBad, gitTotal);

    // Cloud : test best-effort de chaque connexion (peut être indisponible si session verrouillée).
    let cloudTone = "none", cloudBad = 0, cloudTotal = 0, cloudLocked = false;
    try {
      const h = await cloudHealthCached();
      cloudTotal = (h || []).length;
      cloudBad = (h || []).filter((c) => !c.ok).length;
      cloudTone = healthTone(cloudBad, cloudTotal);
    } catch (e) {
      cloudLocked = isLocked(e);
      const clouds = state.corpusClouds || [];
      cloudTotal = clouds.length;
      cloudTone = cloudTotal ? "idle" : "none";
    }

    const pills = [];
    if (gitTone !== "none") {
      pills.push(`<button class="hpill ${gitTone}" id="hp-git" title="${gitTone === "green" ? "Toutes les connexions git sont OK" : gitTone === "red" ? "Toutes les connexions git sont déconnectées" : "Certaines connexions git sont déconnectées"}"><span class="hp-dot"></span>${esc(healthLabel("git", gitTone, gitBad, gitTotal))}</button>`);
    }
    if (cloudTone === "idle") {
      pills.push(`<button class="hpill idle" id="hp-cloud" title="Déverrouillez pour vérifier vos clouds"><span class="hp-dot"></span>Cloud ⏳</button>`);
    } else if (cloudTone !== "none") {
      pills.push(`<button class="hpill ${cloudTone}" id="hp-cloud" title="${cloudTone === "green" ? "Tous les clouds sont joignables" : cloudTone === "red" ? "Aucun cloud n'est joignable" : "Certains clouds ne sont pas joignables"}"><span class="hp-dot"></span>${esc(healthLabel("cloud", cloudTone, cloudBad, cloudTotal))}</button>`);
    }
    // Pastille d'ALERTE « à appairer » : orange, visible dès qu'un dépôt attend d'être appairé.
    const np = state.pendingPairings || 0;
    if (np > 0) {
      pills.push(`<button class="hpill orange" id="hp-pair" title="Des dépôts attendent d'être appairés (local ↔ atelier ↔ vitrine)"><span class="hp-dot"></span>⚠ ${np} à appairer</button>`);
    }
    box.innerHTML = pills.join("");
    const hg = el("hp-git");
    if (hg) hg.onclick = () => healthReconnect("git", gitTone);
    const hc = el("hp-cloud");
    if (hc) hc.onclick = () => healthReconnect("cloud", cloudTone);
    const hpp = el("hp-pair");
    if (hpp) hpp.onclick = () => navTo("organize");
  } finally {
    healthBusy = false;
  }
}

// healthReconnect : un clic sur une pastille de santé emmène TOUJOURS vers la page de réglages de la
// connexion correspondante (git ▸ Connexions git, cloud ▸ Connexions cloud) — retour #10. Au vert,
// c'est un raccourci de gestion ; au rouge/orange, on l'annonce comme une reconnexion.
function healthReconnect(kind, tone) {
  if (kind === "git") {
    state.settabTarget = "conn";
    if (tone !== "green") toast("On vous emmène reconnecter vos serveurs git", "info");
    navTo("settings");
  } else {
    state.settabTarget = "cloud";
    if (tone !== "green") toast("On vous emmène gérer vos connexions cloud", "info");
    navTo("settings");
  }
}

// updateReadOnlyBanner affiche/retire un bandeau permanent quand le mode lecture seule est actif.
function updateReadOnlyBanner() {
  let b = el("ro-banner");
  if (state.readOnly) {
    if (!b) {
      b = document.createElement("div");
      b.id = "ro-banner";
      b.className = "ro-banner";
      b.innerHTML = `${ico("lock", 14)} Mode lecture seule : aucune modification ne sera envoyée. <button class="lnk" id="ro-off">Désactiver</button>`;
      el("app").prepend(b);
      el("ro-off").onclick = () => { state.settabTarget = "secu"; navTo("settings"); }; // → Compte & sécurité (toggle lecture seule + re-validation)
    }
  } else if (b) {
    b.remove();
  }
}

// refreshDisconnected met à jour la liste des instances déconnectées + l'alerte du header.
async function refreshDisconnected() {
  state.disconnected = await App().DisconnectedInstances().catch(() => []);
  const a = el("disc-alert"), c = el("disc-count");
  if (a) {
    const n = (state.disconnected || []).length;
    a.classList.toggle("hidden", n === 0);
    if (c) c.textContent = String(n);
    a.onclick = () => navTo("settings");
  }
}
const isDisconnected = (name) => (state.disconnected || []).includes(name);

// Vues de 1er niveau mémorisées (pour revenir au bon écran après un rechargement : reconnexion
// admin, déverrouillage…). On exclut « detail » (dépend d'un état non reconstituable au reload).
const TOP_VIEWS = ["projects", "organize", "corpus", "kernel", "spokes", "backup", "ia", "settings", "tokens", "admin", "directory", "stats", "help", "git"];
function navTo(view) {
  state.view = view;
  if (view !== "detail") state.detail = null;
  if (view === "corpus") state.corpusView = "board"; // entrer par la nav → accueil des clouds
  // sessionStorage (pas localStorage) : la vue est mémorisée pour un RECHARGEMENT dans la session
  // (ex. reconnexion admin), mais PAS d'un lancement à l'autre — l'accueil reste « Projets ».
  if (TOP_VIEWS.includes(view)) { try { sessionStorage.setItem("situlab.view", view); } catch (e) {} }
  render();
}

function setNav(view) {
  document.querySelectorAll(".nav-item[data-view]").forEach((x) => x.classList.toggle("active", x.dataset.view === view));
}

// --- Onglets de projets (P3, v1.18.2) : plusieurs projets ouverts, barre d'onglets, restauration ---
function loadTabs() { try { return JSON.parse(localStorage.getItem("situlab.tabs") || "[]"); } catch (_) { return []; } }
function saveTabs() { try { localStorage.setItem("situlab.tabs", JSON.stringify(state.tabs || [])); } catch (_) {} }
function saveActiveTab(name) { try { localStorage.setItem("situlab.activeTab", name || ""); } catch (_) {} }
function openTab(name) {
  if (!state.tabs) state.tabs = loadTabs();
  if (!state.tabs.includes(name)) { state.tabs.push(name); saveTabs(); }
  saveActiveTab(name);
}
function closeTab(name) {
  if (!state.tabs) state.tabs = loadTabs();
  const i = state.tabs.indexOf(name);
  if (i < 0) return;
  state.tabs.splice(i, 1); saveTabs();
  if (state.view === "project" && state.projectName === name) {
    const next = state.tabs[Math.max(0, i - 1)];
    if (next) { state.projectName = next; saveActiveTab(next); render(); }
    else { state.projectName = null; saveActiveTab(""); navTo("projects"); }
  } else { renderTabBar(); }
}
function renderTabBar() {
  const bar = el("tabbar"); if (!bar) return;
  if (!state.tabs) state.tabs = loadTabs();
  if (!state.tabs.length) { bar.innerHTML = ""; bar.classList.remove("has-tabs"); return; }
  bar.classList.add("has-tabs");
  bar.innerHTML = state.tabs.map((n) => {
    const active = state.view === "project" && state.projectName === n;
    return `<div class="tab-item${active ? " active" : ""}" data-tab="${esc(n)}"><span class="tab-label">${esc(n)}</span><button class="tab-close" data-close="${esc(n)}" title="Fermer" aria-label="Fermer l'onglet">×</button></div>`;
  }).join("");
  bar.querySelectorAll("[data-tab]").forEach((t) => t.onclick = (e) => { if (e.target.closest("[data-close]")) return; const n = t.dataset.tab; state.projectName = n; state.view = "project"; saveActiveTab(n); render(); });
  bar.querySelectorAll("[data-close]").forEach((b) => b.onclick = (e) => { e.stopPropagation(); closeTab(b.dataset.close); });
}

// Les trois vues de « Gestion des utilisateurs » surlignent le même item de menu.
const UM_VIEWS = ["admin", "directory", "stats"];
// Écrans qui NE dépendent PAS de la connexion courante : restent ouverts même serveur déconnecté.
// Projets (tableau de bord transverse), Réglages (où l'on se reconnecte), Aide (statique).
const OFFLINE_OK_VIEWS = ["projects", "project", "corpus", "kernel", "spokes", "backup", "ia", "settings", "help", "git", "setup"];
function render() {
  // La page « Dépôts » a été retirée (1.11) : son service (relier/assembler) vit désormais dans
  // Projets ▸ « Organiser mes dépôts ». On redirige toute vue « repos » résiduelle (vue mémorisée,
  // ancien lien) vers l'organiseur, qui en est l'héritier.
  if (state.view === "repos") state.view = "organize";
  // En quittant les Statistiques : détruire les graphiques (sinon fuite mémoire / observers
  // orphelins) et couper l'abonnement de progression éventuel.
  if (state._prevView === "stats" && state.view !== "stats") {
    statsDestroyCharts();
    if (statsCtx.progressOff) { try { statsCtx.progressOff(); } catch (e) {} statsCtx.progressOff = null; }
  }
  state._prevView = state.view;
  // Colonne plus étroite pour les écrans « formulaire » (Réglages, Projets) ; large ailleurs.
  const vw = el("view"); if (vw) vw.classList.toggle("narrow", state.view === "settings");
  setNav((state.view === "detail" || state.view === "project" || state.view === "organize") ? "projects" : (UM_VIEWS.includes(state.view) ? "admin" : state.view));
  renderTabBar();
  // Serveur courant déconnecté : on FIGE les écrans qui en dépendent et on explique comment revenir.
  // Projets et Réglages restent vivants (voir OFFLINE_OK_VIEWS).
  if (state.online === false && !OFFLINE_OK_VIEWS.includes(state.view)) return renderReconnectGate();
  if (state.view === "setup") renderSetup();
  else if (state.view === "projects") renderProjects();
  else if (state.view === "organize") renderOrganize();
  else if (state.view === "corpus") renderCorpus();
  else if (state.view === "kernel") renderKernel();
  else if (state.view === "spokes") renderSpokes();
  else if (state.view === "backup") renderBackup();
  else if (state.view === "bkjournal") renderBkJournal();
  else if (state.view === "ia") renderAssistantIA();
  else if (state.view === "project") renderProject(state.projectName);
  else if (state.view === "help") renderHelp();
  else if (state.view === "git") renderGit();
  else if (state.view === "settings") renderSettings();
  else if (state.view === "admin") renderAdmin();
  else if (state.view === "directory") renderDirectory();
  else if (state.view === "stats") renderStats();
  else if (state.view === "tokens") renderTokens();
  else if (state.view === "detail") renderDetail();
}

// renderReconnectGate : écran de blocage clair quand le serveur courant est déconnecté. L'utilisateur
// comprend pourquoi et comment revenir ; Projets et Réglages restent accessibles via le menu.
function renderReconnectGate() {
  const name = state.instance, auth = state.offlineReason === "auth";
  el("view").innerHTML = `<div class="gate">
    <div class="gate-ico">${auth ? ico("warning", 22) : ico("cloud", 22)}</div>
    <div class="gate-ttl">Connexion « ${esc(name)} » ${auth ? "déconnectée" : "hors ligne"}</div>
    <div class="gate-sub">${auth
      ? "Le jeton d'accès de ce serveur n'est plus valide (révoqué ou expiré en ligne). Cet écran reste figé tant que l'accès n'est pas rétabli."
      : "Ce serveur est injoignable pour le moment. Vérifiez votre connexion puis réessayez."}</div>
    <div class="gate-act">
      ${auth ? `<button class="btn btn-primary" id="gate-recon">Reconnecter « ${esc(name)} »</button>` : ""}
      <button class="btn btn-ghost" id="gate-retry">Réessayer</button>
    </div>
    <div class="gate-note">Pendant ce temps, <b>Projets</b> et <b>Réglages</b> restent accessibles dans le menu.</div>
  </div>`;
  const r = el("gate-recon"); if (r) r.onclick = () => reconnectModal(name);
  el("gate-retry").onclick = async () => { await refreshWhoami(); render(); };
}

