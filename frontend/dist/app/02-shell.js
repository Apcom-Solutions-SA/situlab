/* ---------------- Shell ---------------- */
async function showApp(instances) {
  hideBoot();
  el("onboarding").classList.add("hidden");
  el("app").classList.remove("hidden");

  state.instances = instances || [];
  const sel = el("instance-select");
  sel.innerHTML = instances.map((i) => `<option value="${esc(i.name)}"${i.default ? " selected" : ""}>${esc(i.name)}</option>`).join("");
  state.instance = (instances.find((i) => i.default) || instances[0]).name;
  updateRoleBadge();
  sel.onchange = async () => {
    state.instance = sel.value; state.detail = null; state.repos = null; state.repoSig = ""; state.online = undefined;
    updateRoleBadge();
    await refreshWhoami();     // on connaît l'état AVANT de router : pas de flash d'erreur Dépôts
    navTo("repos");            // render() figera l'écran si ce serveur est déconnecté
  };

  document.querySelectorAll(".nav-item[data-view]").forEach((b) => { b.onclick = () => navTo(b.dataset.view); });

  state.readOnly = await App().ReadOnly().catch(() => false);
  updateReadOnlyBanner();
  await updateGitBanner();

  // Revenir à la dernière vue de 1er niveau (ex. après une reconnexion admin qui recharge la page).
  try { const gv = sessionStorage.getItem("situlab.view"); if (gv && TOP_VIEWS.includes(gv)) state.view = gv; } catch (e) {}

  await refreshDisconnected();
  await refreshWhoami();
  render();
  startLive();
  startAutoFetch(); // récupération de fond des nouveautés distantes (noyau : FetchAllLocal)
  maybeAutoTour(); // visite guidée automatique, une seule fois après la 1re connexion
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
      b.innerHTML = `⚠ git n'est pas installé sur ce poste — SituLab pilote git, il ne peut pas fonctionner sans. ` +
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
    const sel = el("instance-select");
    if (sel) sel.innerHTML = list.map((i) => `<option value="${esc(i.name)}"${i.name === state.instance ? " selected" : ""}>${esc(i.name)}</option>`).join("");
    updateRoleBadge();
  } catch (e) {}
}

// updateRoleBadge reflète le rôle de la connexion active (atelier/production) dans la barre du haut.
function updateRoleBadge() {
  const b = el("role-badge"); if (!b) return;
  const r = roleOf(state.instance);
  b.textContent = r === "production" ? "VITRINE" : "ATELIER";
  b.className = "role-badge " + (r === "production" ? "role-production" : "role-atelier");
}

// updateReadOnlyBanner affiche/retire un bandeau permanent quand le mode lecture seule est actif.
function updateReadOnlyBanner() {
  let b = el("ro-banner");
  if (state.readOnly) {
    if (!b) {
      b = document.createElement("div");
      b.id = "ro-banner";
      b.className = "ro-banner";
      b.innerHTML = `🔒 Mode lecture seule — aucune modification ne sera envoyée. <button class="lnk" id="ro-off">Désactiver</button>`;
      el("app").prepend(b);
      el("ro-off").onclick = () => navTo("settings");
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
const TOP_VIEWS = ["repos", "projects", "settings", "tokens", "admin", "directory", "stats", "help", "git"];
function navTo(view) {
  state.view = view;
  if (view !== "detail") state.detail = null;
  // sessionStorage (pas localStorage) : la vue est mémorisée pour un RECHARGEMENT dans la session
  // (ex. reconnexion admin), mais PAS d'un lancement à l'autre — l'accueil reste « Projets ».
  if (TOP_VIEWS.includes(view)) { try { sessionStorage.setItem("situlab.view", view); } catch (e) {} }
  render();
}

function setNav(view) {
  document.querySelectorAll(".nav-item[data-view]").forEach((x) => x.classList.toggle("active", x.dataset.view === view));
}

// Les trois vues de « Gestion des utilisateurs » surlignent le même item de menu.
const UM_VIEWS = ["admin", "directory", "stats"];
// Écrans qui NE dépendent PAS de la connexion courante : restent ouverts même serveur déconnecté.
// Projets (tableau de bord transverse), Réglages (où l'on se reconnecte), Aide (statique).
const OFFLINE_OK_VIEWS = ["projects", "project", "settings", "help", "git"];
function render() {
  // En quittant les Statistiques : détruire les graphiques (sinon fuite mémoire / observers
  // orphelins) et couper l'abonnement de progression éventuel.
  if (state._prevView === "stats" && state.view !== "stats") {
    statsDestroyCharts();
    if (statsCtx.progressOff) { try { statsCtx.progressOff(); } catch (e) {} statsCtx.progressOff = null; }
  }
  state._prevView = state.view;
  // Colonne plus étroite pour les écrans « formulaire » (Réglages, Projets) ; large ailleurs.
  const vw = el("view"); if (vw) vw.classList.toggle("narrow", state.view === "settings");
  setNav(state.view === "detail" ? "repos" : state.view === "project" ? "projects" : (UM_VIEWS.includes(state.view) ? "admin" : state.view));
  // Serveur courant déconnecté : on FIGE les écrans qui en dépendent et on explique comment revenir.
  // Projets et Réglages restent vivants (voir OFFLINE_OK_VIEWS).
  if (state.online === false && !OFFLINE_OK_VIEWS.includes(state.view)) return renderReconnectGate();
  if (state.view === "repos") renderRepos(false);
  else if (state.view === "projects") renderProjects();
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
    <div class="gate-ico">${auth ? "🔌" : "📡"}</div>
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

