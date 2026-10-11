// Frontal GUI de SituLab — tableau de bord vivant « distant ⇄ local ».
// N'appelle QUE le noyau Go (window.go.main.App.*). Aucune logique métier ici :
// la parité GUI/CLI est structurelle (même core.Service des deux côtés).

const App = () => window.go.main.App;
const el = (id) => document.getElementById(id);
const esc = (s) => (s == null ? "" : String(s).replace(/[&<>"'\/]/g, (c) =>
  ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;", "/": "&#47;" }[c])));
const cleanErr = (e) => String((e && e.message) || e).replace(/^Error:\s*/, "");

const CODELEN = 6; // longueur du PIN et des codes 2FA (chiffres)
// codeInput transforme un conteneur en CODELEN cases d'un chiffre (PIN / 2FA) : avance
// automatique, Backspace recule, collage d'un code complet, et appelle onComplete(code) dès
// que toutes les cases sont remplies (plus de bouton à cliquer). mask=true masque (PIN).
function codeInput(mount, mask, onComplete, autofocus) {
  mount.innerHTML = "";
  mount.className = "code-input";
  mount.setAttribute("role", "group");
  mount.setAttribute("aria-label", mask ? "Code PIN à 6 chiffres" : "Code à 6 chiffres");
  const boxes = [];
  for (let i = 0; i < CODELEN; i++) {
    const b = document.createElement("input");
    b.type = mask ? "password" : "text";
    b.inputMode = "numeric"; b.maxLength = 1; b.autocomplete = "off"; b.className = "code-box";
    b.setAttribute("aria-label", "Chiffre " + (i + 1));
    boxes.push(b); mount.appendChild(b);
  }
  const read = () => boxes.map((b) => b.value).join("");
  const maybe = () => { if (onComplete && boxes.every((b) => b.value)) onComplete(read()); };
  boxes.forEach((b, i) => {
    b.oninput = () => { b.value = b.value.replace(/\D/g, "").slice(0, 1); if (b.value && i < CODELEN - 1) boxes[i + 1].focus(); maybe(); };
    b.onkeydown = (e) => { if (e.key === "Backspace" && !b.value && i > 0) boxes[i - 1].focus(); };
    b.onpaste = (e) => { e.preventDefault(); const t = ((e.clipboardData || window.clipboardData).getData("text") || "").replace(/\D/g, "").slice(0, CODELEN); for (let j = 0; j < t.length; j++) boxes[j].value = t[j]; boxes[Math.min(t.length, CODELEN - 1)].focus(); maybe(); };
  });
  if (autofocus) {
    boxes[0].setAttribute("autofocus", ""); // indice natif pour la WebView
    // Met le curseur dans la 1re case. À l'ouverture de l'app la fenêtre n'a parfois pas
    // encore le focus clavier (focus() échoue alors) : on insiste par salves pendant ~3 s,
    // et dès que la fenêtre reçoit le focus, jusqu'à ce qu'une case soit réellement active.
    try { App().FocusWindow(); } catch (e) {} // donne le focus clavier à la fenêtre native
    const f = () => {
      if (!document.body.contains(boxes[0])) return true; // écran remplacé → stop
      try { window.focus(); } catch (e) {}
      boxes[0].focus({ preventScroll: true });
      return document.activeElement === boxes[0];
    };
    requestAnimationFrame(f);
    let n = 0;
    const iv = setInterval(() => { if (f() || ++n > 30) clearInterval(iv); }, 100);
    const onWin = () => { if (f()) window.removeEventListener("focus", onWin); };
    window.addEventListener("focus", onWin);
    setTimeout(() => window.removeEventListener("focus", onWin), 8000);
  }
  return { read, clear: () => { boxes.forEach((b) => (b.value = "")); if (boxes[0]) boxes[0].focus(); } };
}
// La session peut se re-verrouiller en cours d'usage (timeout d'inactivité) : on détecte
// l'erreur « session verrouillée » et on repropose le déverrouillage au lieu de boucler.
const isLocked = (e) => /verrouill/i.test(cleanErr(e));
// isAuthError : l'erreur vient d'un jeton refusé par le serveur (révoqué/expiré/droits), pas d'un
// simple réseau coupé. Déclenche la proposition de RECONNEXION (recréer l'accès).
const isAuthError = (e) => /accès refusé|token invalide|non autoris|unauthoriz|forbidden|identifiants|401|403/i.test(cleanErr(e));
// pingInstance : état réel d'une connexion — "ok", "auth" (jeton refusé), "net" (injoignable).
// Un verrouillage n'est pas une déconnexion : on ne stigmatise pas la connexion dans ce cas.
async function pingInstance(name) {
  try { await App().Whoami(name); return "ok"; }
  catch (e) { if (isLocked(e)) return "ok"; return isAuthError(e) ? "auth" : "net"; }
}
let relocking = false;
function handleLock() {
  if (relocking) return;
  relocking = true;
  stopLive();
  unlockScreen(() => location.reload());
}

// Modèle d'état : une seule source de vérité UI (le code d'état vient du noyau).
// Chaque état = libellé clair + icône + classe couleur + infobulle (terme git) + conseil.
const STATES = {
  uptodate:   { label: "À jour",                  ico: "✓", cls: "s-uptodate",  tip: "Local et serveur synchronisés" },
  ahead:      { label: "À envoyer",               ico: "↑", cls: "s-ahead",     tip: "Des points de sauvegarde locaux ne sont pas encore en ligne (git push)", hint: "Envoyez vos modifications en ligne." },
  behind:     { label: "Nouveautés en ligne",     ico: "↓", cls: "s-behind",    tip: "Le serveur a avancé (git pull)", hint: "Récupérez les nouveautés du serveur." },
  diverged:   { label: "À réunir",                ico: "⇅", cls: "s-diverged",  tip: "Local et serveur ont tous deux avancé", hint: "Les deux côtés ont changé — à réunir." },
  dirty:      { label: "Modifications en cours",  ico: "●", cls: "s-dirty",     tip: "Fichiers modifiés non encore enregistrés (git commit)", hint: "Des changements ne sont pas encore enregistrés." },
  detached:   { label: "Tête détachée",           ico: "⚑", cls: "s-detached",  tip: "HEAD détachée (git — avancé)" },
  unreadable: { label: "Illisible",               ico: "⚠", cls: "s-unreadable", tip: "Le dossier local n'a pas pu être lu" },
  nolocal:    { label: "Pas sur ce poste",        ico: "○", cls: "s-nolocal",   tip: "Dépôt en ligne non copié localement" },
};

const state = {
  instance: "",
  view: "projects", // projects (accueil/tableau de bord) | repos | help | settings | detail
  search: "",
  org: "",
  detail: null,
  repos: null,     // dernier dashboard (cache mémoire)
  repoSig: "",     // signature anti-scintillement
  online: true,
  expanded: {},    // cartes dépliées (clé dépôt -> true) : persiste entre rafraîchissements
  branchCache: {}, // contenu HTML des branches déjà chargées (clé dépôt) : évite le flicker au refresh
  users: [],       // cache admin
  advanced: false, // mode avancé : révèle les étapes séparées (envoyer/récupérer/enregistrer), la
                   //               matrice des versions de travail, les outils git locaux (mise de
                   //               côté) et les réglages de dépôt (v1.3)
  checkedAt: 0,    // horodatage du dernier tableau de bord reçu (« vérifié il y a X min »)
  readOnly: false, // mode lecture seule (v1.4) : masque les actions de mutation + bannière
  directory: null, // cache de l'annuaire consolidé (v1.4)
  dirSearch: "",   // filtre de recherche de l'annuaire
  repoView: "grid", // présentation du tableau de bord : "grid" | "list" (v1.5.0)
  hideNolocal: false, // masquer les dépôts pas copiés sur ce poste (v1.5.0)
};
try { if (localStorage.getItem("situlab.repoView") === "list") state.repoView = "list"; } catch (e) {}
try { state.hideNolocal = localStorage.getItem("situlab.hideNolocal") === "1"; } catch (e) {}
// Préférence « mode avancé » mémorisée par poste (confort d'affichage, sans incidence sécurité).
try { state.advanced = localStorage.getItem("situlab.advanced") === "1"; } catch (e) {}
let liveTimer = null;
let liveBusy = false;
let liveTick = 0;

document.addEventListener("DOMContentLoaded", boot);

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Splash d'ouverture : affiché jusqu'à ce que l'app ou l'onboarding soit prêt,
// avec un minimum pour que le motif terminal soit perçu (pas un flash).
const bootStart = Date.now();
function hideBoot() {
  const b = el("boot"); if (!b) return;
  const wait = Math.max(0, 850 - (Date.now() - bootStart));
  setTimeout(() => b.classList.add("hidden"), wait);
}

// Le runtime Wails peuple window.go.main.App juste après le chargement : on attend
// qu'il soit prêt (jusqu'à ~5 s) plutôt que de supposer qu'il l'est au DOMContentLoaded.
async function boot() {
  applyTheme(readTheme());
  for (let i = 0; i < 100; i++) {
    if (window.go && window.go.main && window.go.main.App) return init();
    await sleep(50);
  }
  showFatal("Le pont applicatif n'a pas démarré. Fermez puis rouvrez SituLab ; si le problème persiste, vérifiez que WebView2 est installé.");
}

let pinEventBound = false;
async function init() {
  try { el("version").textContent = "SituLab " + (await App().AppVersion()); } catch (e) {}
  // Le démon (cette fenêtre) demande le PIN sur le bureau quand une commande CLI/agent
  // doit déverrouiller ou confirmer une écriture. Le PIN ne quitte jamais l'app. On ne lie
  // l'écoute qu'une fois (init peut être rejoué après un déverrouillage).
  if (!pinEventBound) {
    try { window.runtime.EventsOn("situlab:pin-request", (payload) => {
      // v1.7 : la charge utile est {reason, detail}. Compat : une ancienne chaîne seule reste gérée.
      const p = (payload && typeof payload === "object") ? payload : { reason: payload };
      pinRequestModal(p.reason || "Autoriser l'opération", p.detail || null, p.action || "");
    }); pinEventBound = true; } catch (e) {}
  }
  let instances = [];
  try { instances = await App().GetInstances(); }
  catch (e) { return showFatal(cleanErr(e)); }
  // La session est verrouillée au démarrage (la clé ne vit que dans le process) : si un PIN
  // existe, il faut déverrouiller avant toute lecture, sinon le dashboard échoue sans issue.
  const pinSet = await App().PinSet().catch(() => false);
  if (!pinSet) {
    // Pas de PIN : fraîche install → onboarding (PIN + 1re instance). Après un reset de PIN
    // (instances conservées mais déconnectées) → recréer un PIN, puis reconnecter.
    if (instances && instances.length > 0) return createPinScreen();
    return showOnboarding();
  }
  if (!(await App().Unlocked().catch(() => false))) {
    return unlockScreen(() => init()); // après déverrouillage, on rejoue init
  }
  if (!instances || instances.length === 0) showOnboarding();
  else showApp(instances);
}

// createPinScreen : recréer un PIN après « PIN oublié » (les instances existent encore,
// déconnectées). Pas de saisie d'instance ici — on reconnecte ensuite depuis l'app.
function createPinScreen() {
  hideBoot();
  el("app").classList.add("hidden");
  const ob = el("onboarding");
  ob.classList.remove("hidden");
  ob.innerHTML = `<div class="connect-card">
    <img src="logo.svg" class="connect-logo" alt="" />
    <h1>SituLab</h1>
    <p class="tagline">Choisissez un nouveau PIN (6 chiffres).</p>
    <p class="sub">Vos instances seront à reconnecter juste après (identifiant + mot de passe).</p>
    <label>Votre PIN</label><div id="np1"></div>
    <label>Confirmer</label><div id="np2"></div>
    <div id="np-err" class="inline-error hidden" role="alert"></div></div>`;
  const c1 = codeInput(el("np1"), true, null, true);
  const c2 = codeInput(el("np2"), true, async () => {
    const err = el("np-err"); err.classList.add("hidden");
    const a = c1.read(), b = c2.read();
    if (a.length < CODELEN) return;
    if (a !== b) { err.textContent = "Les PIN ne correspondent pas."; err.classList.remove("hidden"); c2.clear(); return; }
    try { await App().SetPin("", a); location.reload(); }
    catch (e) { err.textContent = cleanErr(e); err.classList.remove("hidden"); }
  });
}

// unlockScreen demande le PIN pour ouvrir la session (retour dans l'app, ou onboarding avec
// un PIN déjà défini mais aucune instance). Réutilise la carte de connexion.
function unlockScreen(onDone) {
  hideBoot();
  el("app").classList.add("hidden");
  const ob = el("onboarding");
  ob.classList.remove("hidden");
  ob.innerHTML = `<div class="connect-card">
    <img src="logo.svg" class="connect-logo" alt="" />
    <h1>SituLab</h1>
    <p class="tagline">Entrez votre PIN pour déverrouiller vos accès.</p>
    <div id="u-pin"></div>
    <div id="unlock-error" class="inline-error hidden" role="alert"></div></div>`;
  let busy = false;
  const ci = codeInput(el("u-pin"), true, async (code) => {
    if (busy) return; busy = true;
    const err = el("unlock-error"); err.classList.add("hidden");
    try { await App().Unlock(code); }
    catch (e) { err.textContent = cleanErr(e); err.classList.remove("hidden"); busy = false; ci.clear(); return; }
    // Rechargement : DOM propre (cet écran a remplacé le markup d'onboarding) puis init() rejoue.
    location.reload();
  }, true);
  void onDone; // conservé pour compat d'appel ; le rechargement rejoue le bon écran
}

function showFatal(msg) {
  hideBoot();
  el("onboarding").classList.add("hidden");
  el("app").classList.remove("hidden");
  el("view").innerHTML = surfaceError("SituLab n'a pas pu démarrer", msg, null);
}

/* ---------------- Onboarding ---------------- */
// setupStepper reflète l'avancement : étape PIN puis étape Connexion.
function setupStepper(pinDone) {
  const s = el("stepper"); if (!s) return;
  s.hidden = false;
  el("step-1").className = "step " + (pinDone ? "done" : "active");
  el("step-2").className = "step " + (pinDone ? "active" : "");
}
// obShow affiche un seul panneau du tunnel et cache les autres.
function obShow(which) {
  const map = { pin: "panel-pin", provider: "panel-provider", connect: "connect-form", "2fa": "panel-2fa", github: "github-form" };
  Object.values(map).forEach((id) => { const n = el(id); if (n) n.hidden = true; });
  const node = el(map[which]); if (node) node.hidden = false;
}

async function showOnboarding() {
  hideBoot();
  el("onboarding").classList.remove("hidden");
  el("app").classList.add("hidden");

  // Onboarding « PIN-only », en tunnel séquentiel : 1) un PIN (s'il n'existe pas),
  // 2) la connexion Forgejo, 2b) le code 2FA seulement si le compte en a un. Les
  // identifiants déjà saisis ne sont jamais redemandés.
  let pinDone = await App().PinSet().catch(() => false);

  const welcome = el("welcome-card"), card = el("connect-card");
  let onOtpComplete = null;
  const ciPin = codeInput(el("f-pin"), true, null, false);
  const ciPin2 = codeInput(el("f-pin2"), true, () => el("pin-next").click());
  const ciOtp = codeInput(el("f-otp"), false, () => { if (onOtpComplete) onOtpComplete(); });

  // Étape 1 — PIN. « Continuer » valide le PIN, le définit, puis passe à la connexion.
  el("pin-next").onclick = async () => {
    const err = el("pin-err"); err.classList.add("hidden");
    const p1 = ciPin.read(), p2 = ciPin2.read();
    if (p1.length < CODELEN) { err.textContent = "Le PIN doit faire 6 chiffres."; err.classList.remove("hidden"); return; }
    if (p1 !== p2) { err.textContent = "Les PIN ne correspondent pas."; err.classList.remove("hidden"); ciPin2.clear(); return; }
    const btn = el("pin-next"); btn.disabled = true; btn.textContent = "…";
    try { await App().SetPin("", p1); pinDone = true; }
    catch (e) { err.textContent = cleanErr(e); err.classList.remove("hidden"); btn.disabled = false; btn.textContent = "Continuer"; return; }
    gotoProvider();
  };

  // Étape 2 — choix du fournisseur, puis la connexion correspondante.
  function gotoProvider() { setupStepper(true); obShow("provider"); }
  function gotoConnect() {
    setupStepper(true);
    obShow("connect");
    setTimeout(() => { const fn = el("f-name"); if (fn) fn.focus(); }, 40);
  }
  function gotoGitHub() {
    setupStepper(true);
    obShow("github");
    setTimeout(() => { const fn = el("g-name"); if (fn) fn.focus(); }, 40);
  }
  document.querySelectorAll("#panel-provider [data-provider]").forEach((b) => {
    b.onclick = () => (b.dataset.provider === "github" ? gotoGitHub() : gotoConnect());
  });

  // Connexion GitHub : jeton personnel collé (SituLab ne peut pas le créer via l'API).
  el("github-form").onsubmit = async (ev) => {
    ev.preventDefault();
    const btn = el("github-btn"), err = el("github-error");
    err.classList.add("hidden");
    const name = el("g-name").value.trim(), token = el("g-token").value.trim();
    if (!name || !token) { err.textContent = "Un nom et un jeton sont nécessaires."; err.classList.remove("hidden"); return; }
    btn.disabled = true; btn.textContent = "Connexion…";
    try { await App().AddInstanceToken(name, "github", "", token); location.reload(); return; }
    catch (e) { err.textContent = cleanErr(e); err.classList.remove("hidden"); }
    finally { btn.disabled = false; btn.textContent = "Se connecter"; }
  };
  el("github-back").onclick = () => gotoProvider();
  { const cb = el("connect-back"); if (cb) cb.onclick = () => gotoProvider(); }

  async function doLogin(err, otp) {
    const name = el("f-name").value.trim(), url = el("f-url").value.trim();
    const user = el("f-user").value.trim(), pass = el("f-pass").value;
    if (!name || !url || !user || !pass) { err.textContent = "Nom, adresse, identifiant et mot de passe sont nécessaires."; err.classList.remove("hidden"); return false; }
    // Jeton standard par défaut (moindre privilège) : le droit admin est accordé à la demande,
    // via « Activer l'administration » dans l'onglet Administration (v1.4.2).
    await App().AddInstanceLogin(name, url, user, pass, otp, false, !!(el("f-remember") && el("f-remember").checked));
    return true;
  }

  el("connect-form").onsubmit = async (ev) => {
    ev.preventDefault();
    const btn = el("connect-btn"), err = el("connect-error");
    err.classList.add("hidden");
    btn.disabled = true; btn.textContent = "Connexion…";
    try {
      if (await doLogin(err, "")) { location.reload(); return; }
    } catch (e) {
      // Échec : on bascule sur l'étape 2FA (le compte en a peut-être une), sans
      // redemander les identifiants déjà saisis. Un retour reste possible.
      obShow("2fa");
      ciOtp.clear();
      el("otp-err").classList.add("hidden");
      setTimeout(() => { const b = el("f-otp").querySelector("input"); if (b) b.focus(); }, 40);
    } finally {
      btn.disabled = false; btn.textContent = "Se connecter";
    }
  };

  // Étape 2b — 2FA. On rejoue la connexion avec le code, identifiants inchangés.
  const submitOtp = async () => {
    const err = el("otp-err"); err.classList.add("hidden");
    const code = ciOtp.read();
    if (code.length < CODELEN) { err.textContent = "Entrez le code à 6 chiffres."; err.classList.remove("hidden"); return; }
    const btn = el("otp-btn"); btn.disabled = true; btn.textContent = "…";
    try {
      if (await doLogin(err, code)) { location.reload(); return; }
    } catch (e) {
      err.textContent = cleanErr(e); err.classList.remove("hidden"); ciOtp.clear();
    } finally { btn.disabled = false; btn.textContent = "Valider"; }
  };
  el("otp-btn").onclick = submitOtp;
  el("otp-back").onclick = () => { obShow("connect"); setTimeout(() => { const fn = el("f-name"); if (fn) fn.focus(); }, 40); };
  onOtpComplete = submitOtp; // validation auto dès 6 chiffres

  // Choix de l'écran de départ : accueil (fraîche install) ou directement la connexion.
  if (!pinDone) {
    welcome.hidden = false; card.hidden = true;
    el("welcome-start").onclick = () => {
      welcome.hidden = true; card.hidden = false;
      setupStepper(false); obShow("pin");
      setTimeout(() => { const b = el("f-pin").querySelector("input"); if (b) b.focus(); }, 40);
    };
  } else {
    welcome.hidden = true; card.hidden = false;
    gotoProvider();
  }

  addRevealToggle(el("connect-form"));
  addRevealToggle(el("github-form"));
}

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
const OFFLINE_OK_VIEWS = ["projects", "settings", "help", "git"];
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
  setNav(state.view === "detail" ? "repos" : (UM_VIEWS.includes(state.view) ? "admin" : state.view));
  // Serveur courant déconnecté : on FIGE les écrans qui en dépendent et on explique comment revenir.
  // Projets et Réglages restent vivants (voir OFFLINE_OK_VIEWS).
  if (state.online === false && !OFFLINE_OK_VIEWS.includes(state.view)) return renderReconnectGate();
  if (state.view === "repos") renderRepos(false);
  else if (state.view === "projects") renderProjects();
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

/* ---------------- Gestionnaire de tokens ---------------- */
const SCOPES = [
  ["read:repository", "Lire les dépôts"], ["write:repository", "Modifier les dépôts"],
  ["read:issue", "Lire les issues"], ["write:issue", "Modifier les issues"],
  ["read:user", "Lire le profil"], ["read:organization", "Lire les organisations"],
];
const SCOPE_LABEL = Object.fromEntries(SCOPES.map(([s, l]) => [s, l]));
// scopeText rend une portée en langage clair ; un slug inconnu est traduit grossièrement.
const scopeText = (s) => SCOPE_LABEL[s] || String(s).replace(/^read:/, "lire : ").replace(/^write:/, "modifier : ").replace(/^admin:/, "admin : ");
// Un token nommé « SituLab@… » est celui que SituLab a créé pour lui-même à l'onboarding.
const isSitulabToken = (name) => /^SituLab@/.test(String(name || ""));
// Instance courante + indicateur 2FA (pour ne demander que le code 2FA le cas échéant).
const curInst = () => (state.instances || []).find((i) => i.name === state.instance) || {};
const curIsGitHub = () => curInst().provider === "github";
const needs2FA = () => !!curInst().two_fa;
// Badge/icône Forgejo (git branch) — signale un login EXTERNE, style volontairement distinct.
const FJ_ICON = `<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path d="M7 4.5a2 2 0 1 0 0 4 2 2 0 0 0 0-4Zm0 11a2 2 0 1 0 0 4 2 2 0 0 0 0-4Zm10-11a2 2 0 1 0 0 4 2 2 0 0 0 0-4ZM7 8.5v7m0-4h5a4 4 0 0 0 4-4" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg>`;
const GH_ICON = `<svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg>`;
// forgejoBlock enveloppe DES CHAMPS d'identifiants Forgejo dans un bloc entièrement thématisé
// « Forgejo » (teal, encadré) : on voit d'emblée que c'est un login EXTERNE, pas le PIN SituLab.
const forgejoBlock = (fieldsHtml, url, variant) => {
  if (variant === "github") {
    return `<div class="forgejo-block github-block">
      <div class="fj-head"><span class="fj-badge gh">${GH_ICON} GitHub</span> Connexion par jeton</div>
      <div class="fj-sub">Jeton d'accès personnel GitHub — chiffré par votre PIN${url ? ` · <a class="lnk" data-ext="${esc(url)}">${esc(url)}</a>` : ""}.</div>
      ${fieldsHtml}</div>`;
  }
  return `<div class="forgejo-block">
  <div class="fj-head"><span class="fj-badge">${FJ_ICON} Forgejo</span> Connexion à votre serveur git</div>
  <div class="fj-sub">Vos identifiants de compte <b>Forgejo</b> — pas votre PIN SituLab. Une question&nbsp;? Votre administrateur Forgejo${url ? ` · <a class="lnk" data-ext="${esc(url)}">${esc(url)}</a>` : ""}.</div>
  ${fieldsHtml}</div>`;
};

// tokenHelp : aide CONTEXTUELLE au type de jeton selon le système git. GitHub = jeton personnel à
// créer soi-même (scopes précis) ; Forgejo/Gitea = SituLab le crée depuis le login. Repliable.
function tokenHelp(provider) {
  if (provider === "github") {
    return `<details class="tok-help"><summary>🔑 Quel jeton créer sur GitHub ?</summary>
      <ol class="tok-steps">
        <li>Ouvrez <a class="lnk" data-ext="https://github.com/settings/personal-access-tokens">GitHub → Settings → Developer settings → Personal access tokens</a>.</li>
        <li>Choisissez <b>Fine-grained token</b> (recommandé) ou <b>Token (classic)</b>.</li>
        <li><b>Fine-grained</b> : « Repository access » = les dépôts voulus (ou « All ») ; « Permissions » → <b>Contents : Read and write</b> (ajoutez <b>Issues</b> et <b>Pull requests</b> si vous les gérez).<br><b>Classic</b> : cochez simplement le scope <b>repo</b>.</li>
        <li><b>Generate</b>, puis <b>copiez</b> le jeton (<code>github_pat_…</code> ou <code>ghp_…</code>) et collez-le ci-dessus.</li>
      </ol>
      <div class="tok-note">GitHub interdit à une application de créer le jeton à votre place — d'où cette étape manuelle. Il est chiffré par votre PIN.</div>
    </details>`;
  }
  return `<details class="tok-help"><summary>🔑 Et le jeton sur Forgejo / Gitea ?</summary>
    <div class="tok-note">Rien à créer à la main : avec votre <b>identifiant</b>, votre <b>mot de passe</b> (et votre <b>code 2FA</b> si activé), SituLab génère pour vous un jeton d'accès, chiffré par votre PIN.</div>
    <ul class="tok-steps">
      <li>Portées demandées : <b>lecture/écriture</b> des dépôts, des issues, du profil et des organisations.</li>
      <li>Votre compte doit avoir les droits sur les dépôts visés (et « administrateur » si vous gérez les comptes).</li>
      <li>Vous le retrouverez côté serveur dans <i>Paramètres → Applications → Jetons d'accès</i>.</li>
    </ul>
  </details>`;
}

// serverPageURL : URL de gestion côté serveur selon le fournisseur et le type de page.
// GitHub n'expose pas les jetons/les comptes par API → on y renvoie l'utilisateur par un bouton.
function serverPageURL(inst, kind) {
  const u = (inst && inst.url ? inst.url : "").replace(/\/+$/, "");
  const gh = inst && (inst.provider === "github" || /github\.com/i.test(u));
  if (gh) {
    const base = "https://github.com";
    if (kind === "tokens") return base + "/settings/tokens";
    if (kind === "users") return base + "/settings/organizations";
    return base + "/settings";
  }
  if (!u) return "";
  if (kind === "tokens") return u + "/user/settings/applications";
  if (kind === "users") return u + "/-/admin/users";
  return u + "/user/settings";
}

// serverLinkBtn : bouton « gérer sur le serveur ↗ » ouvrant la page du fournisseur dans le navigateur.
// Via data-openurl + écouteur délégué (la CSP script-src 'self' interdit les onclick inline).
function serverLinkBtn(inst, kind, label) {
  const url = serverPageURL(inst, kind);
  if (!url) return "";
  return `<button class="btn btn-ghost btn-sm" data-openurl="${esc(url)}">${esc(label || "Gérer sur le serveur")} ↗</button>`;
}
// safeOpenURL n'ouvre dans le navigateur systeme qu'une URL http(s) EXPLICITE (defense en profondeur
// contre file://, javascript:, data:… qu'un contenu Markdown/README non fiable tenterait d'injecter).
function safeOpenURL(u) {
  if (typeof u === "string" && /^https?:\/\//i.test(u.trim())) {
    try { window.runtime.BrowserOpenURL(u.trim()); } catch (_) {}
  }
}
document.addEventListener("click", (e) => {
  const b = e.target.closest && e.target.closest("[data-openurl]");
  if (b) safeOpenURL(b.dataset.openurl);
});

async function renderTokens() {
  if (curIsGitHub()) {
    el("view").innerHTML = `<div class="view-head"><div class="view-title">Jetons</div>
        <div class="view-sub">GitHub ne permet pas à une application de lister ou créer vos jetons — ça se gère sur github.com.</div></div>
      <div class="panel" style="max-width:560px"><div class="panel-h">Jetons d'accès personnels (GitHub)</div>
        <p class="sub">Vos <b>PAT</b> (Personal Access Tokens) se créent, se listent et se révoquent directement sur GitHub → <i>Settings → Developer settings → Personal access tokens</i>.</p>
        <div class="set-row-c" style="justify-content:flex-start">${serverLinkBtn(curInst(), "tokens", "Ouvrir les jetons GitHub")}</div>
      </div>`;
    return;
  }
  const title = `<div class="view-head view-head-row"><div><div class="view-title">Jetons Forgejo</div>
    <div class="view-sub">Vos jetons sur « ${esc(state.instance)} ». Un jeton hérite de vos droits.</div></div>
    ${serverLinkBtn(curInst(), "tokens", "Gérer sur le serveur")}</div>`;
  const twoFA = needs2FA();
  // Compte SANS 2FA : le mot de passe mémorisé (chiffré par le PIN) donne l'accès direct —
  // le PIN suffit, pas de re-saisie. On tente une fois ; échec → panneau.
  if (!twoFA && !state.tokStored && !state._tokTried) {
    state._tokTried = true;
    try { await App().TokenList(state.instance, state.login, "", ""); state.tokStored = true; }
    catch (e) { /* pas de mot de passe mémorisé → panneau */ }
  }
  if (!state.tokStored) {
    // Compte 2FA : on ne demande QUE le code 2FA (le mot de passe vient du trousseau).
    el("view").innerHTML = title + `
      <div class="panel" style="max-width:460px">
        <div class="panel-h">${twoFA ? "Code à deux facteurs" : "Déverrouiller"}</div>
        <p class="sub">${twoFA ? "Votre compte utilise la double authentification : saisissez le code 2FA du moment (il n'est pas mémorisable). Votre mot de passe, lui, est déjà connu de SituLab (chiffré par votre PIN)." : "Saisissez le mot de passe du compte pour gérer les jetons."}</p>
        ${forgejoBlock(`<div class="field">
          ${twoFA ? "" : `<label>Mot de passe de « ${esc(state.login || "")} »<input id="tok-pwd" type="password" /></label>`}
          <label>Code 2FA ${twoFA ? "" : `<span class="muted">(si votre compte en a un)</span>`}</label><div id="tok-otp" class="git-code"></div>
          <div class="inline-error hidden" id="tok-err"></div>
        </div>`, curInst().url)}
        ${twoFA ? "" : `<button class="btn btn-primary" id="tok-unlock">Déverrouiller</button>`}
      </div>`;
    const go = async () => {
      const pwd = twoFA ? "" : el("tok-pwd").value, otp = ciOtp.read(); const er = el("tok-err");
      try {
        await App().TokenList(state.instance, state.login, pwd, otp);
        state.tokOtp = otp; state.tokStored = true;
        // Mémorise le mot de passe (upgrade des instances créées avant) → plus de re-saisie.
        try { await App().RememberPassword(state.instance, pwd, !!otp); const ci2 = curInst(); if (ci2) ci2.two_fa = !!otp; } catch (e) {}
        renderTokens();
      } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); ciOtp.clear(); }
    };
    // Compte 2FA : validation auto dès les 6 chiffres. Sinon : bouton (après le mot de passe).
    const ciOtp = codeInput(el("tok-otp"), false, twoFA ? () => go() : null, twoFA);
    if (el("tok-unlock")) el("tok-unlock").onclick = go;
    if (el("tok-pwd")) { el("tok-pwd").onkeydown = (e) => { if (e.key === "Enter") go(); }; setTimeout(() => el("tok-pwd").focus(), 30); }
    return;
  }
  el("view").innerHTML = title + `
    <div class="toolbar"><button class="btn btn-primary btn-sm" id="tok-ia">🤖 Jeton pour mon IA</button>
      <button class="btn btn-ghost btn-sm" id="tok-new">＋ Nouveau token</button>
      <button class="btn btn-ghost btn-sm" id="tok-lock">Verrouiller</button></div>
    <div id="tok-list"><div class="skeleton"></div></div>`;
  el("tok-new").onclick = () => tokenCreateModal();
  el("tok-ia").onclick = () => tokenCreateModal({ ia: true, name: iaTokenName(), scopes: IA_SCOPES });
  el("tok-lock").onclick = () => { state.tokPwd = null; state.tokOtp = null; state.tokStored = false; state._tokTried = false; renderTokens(); };
  try {
    const toks = await App().TokenList(state.instance, state.login, state.tokPwd || "", state.tokOtp || "");
    const active = await App().ActiveTokenName(state.instance).catch(() => ""); // le SEUL à protéger
    el("tok-list").onclick = async (ev) => {
      const del = ev.target.closest("[data-del]"); if (!del) return;
      tokenDeleteConfirm(+del.dataset.del, del.dataset.name);
    };
    // On sépare le JETON DE SESSION de SituLab (celui qu'il utilise pour cette connexion) du
    // reste : il n'est pas administrable ici, on le met au second plan, discret.
    const list = toks || [];
    const sess = list.find((t) => t.name === active);
    const mine = list.filter((t) => t.name !== active);
    const pillsOf = (t) => (t.scopes || []).length ? t.scopes.map((s) => `<span class="pill">${esc(scopeText(s))}</span>`).join(" ") : `<span class="muted">portée par défaut</span>`;
    const rows = mine.length ? mine.map((t) =>
      `<div class="srow"><div><b>${esc(t.name)}</b> ${pillsOf(t)}</div><button class="lnk danger" data-del="${t.id}" data-name="${esc(t.name)}">supprimer${isSitulabToken(t.name) ? " (résidu)" : ""}</button></div>`
    ).join("") : `<p class="muted">Aucun jeton pour relier d'autres outils. Créez-en un avec « Nouveau token ».</p>`;
    const sessBlock = sess
      ? `<div class="sess-token"><span class="sess-dot"></span><div><div class="sess-name">Jeton de session SituLab</div><div class="sess-sub">Créé et géré par SituLab pour cette connexion. Non modifiable ici — révocable uniquement depuis Forgejo, par un administrateur.</div></div></div>`
      : "";
    el("tok-list").innerHTML = rows + sessBlock;
  } catch (e) { state.tokPwd = null; state.tokOtp = null; state.tokStored = false; state._tokTried = false; el("tok-list").innerHTML = surfaceError("Session expirée", cleanErr(e), () => renderTokens()); }
}

// IA_SCOPES : droits conseillés pour un assistant IA de développement (lecture + écriture
// des dépôts et des issues ; aucun droit d'administration).
const IA_SCOPES = ["read:repository", "write:repository", "read:issue", "write:issue", "read:user"];

function tokenCreateModal(preset) {
  const ia = preset && preset.ia;
  const presetScopes = (preset && preset.scopes) || null;
  const isChecked = (s) => presetScopes ? presetScopes.includes(s) : (s === "read:repository" || s === "write:repository");
  const checks = SCOPES.map(([s, l]) => `<label style="flex-direction:row;align-items:center" title="${s}"><input type="checkbox" class="sc" value="${s}" style="width:auto;margin-right:8px"${isChecked(s) ? " checked" : ""} />${esc(l)}</label>`).join("");
  const repoNames = (state.repos || []).filter((r) => r.has_remote).map((r) => r.full_name);
  const repoChecks = repoNames.map((n) => `<label style="flex-direction:row;align-items:center"><input type="checkbox" class="rp" value="${esc(n)}" style="width:auto;margin-right:8px" />${esc(n)}</label>`).join("");
  openModal(ia ? "Créer un jeton pour mon IA" : "Nouveau token", `<div class="field">
    ${ia ? `<div class="callout">Ce jeton donne à votre assistant IA l'accès à vos dépôts et issues (lecture + écriture), <b>sans</b> droits d'administration. Les envois resteront confirmés par <b>votre PIN</b>.</div>` : ""}
    <label>Nom<input id="tk-name" placeholder="ex. situlab-poste" value="${ia ? esc((preset && preset.name) || "") : ""}" /></label>
    <div class="blk-h" style="margin-top:4px">Portées</div>${checks}
    <div class="blk-h" style="margin-top:4px">Accès aux dépôts</div>
    <label style="flex-direction:row;align-items:center"><input type="radio" name="tkmode" id="tk-all" value="all" checked style="width:auto;margin-right:8px" />Tous mes dépôts (selon mes droits)</label>
    <label style="flex-direction:row;align-items:center"><input type="radio" name="tkmode" value="some" style="width:auto;margin-right:8px" />Dépôts spécifiques</label>
    <p class="sub" style="margin-top:4px">Un jeton peut agir sur tous vos dépôts, ou seulement sur ceux que vous cochez — préférez la seconde option s'il ne sert qu'à un projet.</p>
    <div id="tk-repos" class="repo-picker hidden">${repoChecks || '<span class="muted">Vos dépôts ne sont pas encore chargés — ouvrez d\'abord l\'onglet Dépôts.</span>'}</div>
    ${needs2FA() ? forgejoBlock(`<label>Code 2FA du moment</label><div id="tk-otp" class="git-code"></div>`, curInst().url) : ""}
    <div class="inline-error hidden" id="tk-err"></div></div>`, async () => {
    const er = el("tk-err");
    const scopes = [...document.querySelectorAll(".sc:checked")].map((c) => c.value);
    const repos = el("tk-all").checked ? [] : [...document.querySelectorAll(".rp:checked")].map((c) => c.value);
    const tkname = el("tk-name").value.trim();
    if (!tkname) { er.textContent = "Donnez un nom au jeton."; er.classList.remove("hidden"); return; }
    if (!el("tk-all").checked && repos.length === 0) { er.textContent = "Cochez au moins un dépôt, ou choisissez « Tous mes dépôts »."; er.classList.remove("hidden"); return; }
    const otp = ciOtp ? ciOtp.read() : (state.tokOtp || "");
    // v1.5.0 : plus de PIN ici — l'authentification est le mot de passe Forgejo (+ 2FA).
    try {
      const val = await App().TokenCreate(state.instance, state.login, state.tokPwd || "", otp, tkname, scopes, repos, "");
      closeModal();
      renderTokens();
      openModal("Jeton créé", `<div class="field"><div class="callout warn">⚠ Copiez ce jeton maintenant : pour votre sécurité, il ne sera plus jamais réaffiché. Perdu, il faudra le supprimer et en créer un autre.</div>
        <pre class="filebox" id="tk-val" style="user-select:text;white-space:pre-wrap">${esc(val)}</pre>
        <button class="btn btn-ghost btn-sm" type="button" id="tk-copy">Copier le jeton</button>
        ${ia ? `<button class="btn btn-primary btn-sm" type="button" id="tk-prompt" style="margin-top:6px">Configurer mon agent IA</button>` : ""}</div>`, () => closeModal());
      const cp = el("tk-copy"); if (cp) cp.onclick = async () => { try { await navigator.clipboard.writeText(val); toast("Jeton copié", "ok"); } catch (e) { toast("Copie impossible — sélectionnez le texte puis Ctrl+C", "err"); } };
      const pb = el("tk-prompt"); if (pb) pb.onclick = () => aiAgentModal();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); if (ciOtp) ciOtp.clear(); }
  });
  el("modal-ok").textContent = "Continuer";
  document.querySelectorAll('input[name="tkmode"]').forEach((r) => r.onchange = () => el("tk-repos").classList.toggle("hidden", el("tk-all").checked));
  const ciOtp = needs2FA() ? codeInput(el("tk-otp"), false, null) : null;
}

function tokenDeleteConfirm(id, name) {
  let ciOtp;
  openModal("Supprimer le token", `<div class="field">
    <p class="sub">Supprimer le token « ${esc(name)} » ? Les outils qui l'utilisent cesseront de fonctionner.</p>
    ${needs2FA() ? forgejoBlock(`<label>Code 2FA du moment</label><div id="td-otp" class="git-code"></div>`, curInst().url) : ""}
    <div class="inline-error hidden" id="td-err"></div></div>`, async () => {
    const otp = ciOtp ? ciOtp.read() : (state.tokOtp || "");
    const er = el("td-err");
    try {
      await App().TokenDelete(state.instance, state.login, state.tokPwd || "", otp, id, "");
      closeModal(); toast("Token supprimé", "ok"); renderTokens();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); if (ciOtp) ciOtp.clear(); }
  });
  ciOtp = needs2FA() ? codeInput(el("td-otp"), false, null, true) : null;
  el("modal-ok").textContent = "Supprimer"; el("modal-ok").classList.add("danger-btn");
}

// iaTokenName : nom par défaut, identifiable et révocable, du jeton dédié à l'IA.
function iaTokenName() { return "IA-SituLab"; }

// firstPromptText : message de PREMIÈRE PRISE DE CONTACT, volontairement léger et ouvert — pas de
// liste de dépôts ni de commandes imposées. On dit à l'IA où est l'outil, de lire la fiche
// AGENTS.md (mode d'emploi + collaboration), de regarder l'état et de nous dire ce qu'elle en
// pense. exe = chemin complet de l'exécutable (résout le « pas dans le PATH »). kind = "mcp" | "cli".
function firstPromptText(kind, exe) {
  exe = exe || "situlab";
  const launch = kind === "mcp"
    ? "- Tu me pilotes SituLab via le serveur MCP « situlab » (ses outils apparaissent chez toi)."
    : `- SituLab est installé ici : "${exe}". Lance-le en ligne de commande par ce chemin complet (il n'est pas forcément dans le PATH), par ex. "${exe}" --help.`;
  return [
    "Salut ! On va travailler ensemble avec SituLab, mon utilitaire git. Le principe : toi tu prépares, moi je garde la main — je valide par mon PIN.",
    "",
    launch,
    "- Près de mes dépôts, tu trouveras un fichier AGENTS.md : c'est à la fois le mode d'emploi pour toi ET notre fiche de collaboration. Lis-le en premier.",
    "- Ensuite, regarde l'état de mes dépôts et dis-moi simplement ce que tu comprends et ce que tu proposes.",
    "- Règle d'or : ne me demande jamais mon PIN et ne cherche pas à le contourner. Rien ne part en ligne sans ma validation.",
    "",
    "Pour commencer : dis-moi ce que tu as compris de SituLab, et propose-moi ce qu'on pourrait noter dans AGENTS.md sur notre façon de travailler (tu peux l'enrichir au fil de l'eau).",
  ].join("\n");
}

// aiAgentModal : « faisons connaissance avec votre agent IA » (v1.4.2). L'utilisateur choisit son
// type d'agent (terminal/CLI ou système/MCP) ; SituLab produit le prompt de première connexion
// sur mesure, la config MCP à coller, et propose d'installer le guide AGENTS.md.
function aiAgentModal() {
  infoModal("Faisons connaissance avec votre agent IA", `
    <p class="sub">SituLab est fait pour que votre assistant <b>prépare</b> le travail et que <b>vous</b> validiez par votre PIN. Lequel utilisez-vous&nbsp;?</p>
    <div class="ai-choice">
      <button type="button" class="ai-pick" data-k="cli"><span class="ai-pick-ic">🖥️</span><b>Un agent en terminal</b><span class="sub">Claude Code, agent shell… il tape des commandes <code>situlab</code>.</span></button>
      <button type="button" class="ai-pick" data-k="mcp"><span class="ai-pick-ic">🧩</span><b>Un agent système (MCP)</b><span class="sub">Claude Desktop, Cursor… il se connecte à <code>situlab mcp</code>.</span></button>
    </div>
    <div id="ai-out"></div>`);
  document.querySelectorAll(".ai-pick").forEach((b) => b.onclick = () => aiAgentShow(b.dataset.k));
}

async function aiAgentShow(kind) {
  document.querySelectorAll(".ai-pick").forEach((b) => b.classList.toggle("active", b.dataset.k === kind));
  const out = el("ai-out");
  if (!out) return;
  let exe = "situlab";
  try { exe = (await App().ExePath()) || "situlab"; } catch (e) {}
  let mcpBlock = "", cfg = "";
  if (kind === "mcp") {
    cfg = JSON.stringify({ mcpServers: { situlab: { command: exe, args: ["mcp"] } } }, null, 2);
    mcpBlock = `<div class="ai-step"><div class="ai-step-h">1. Ajoutez SituLab à votre agent MCP</div>
      <p class="sub">Collez ceci dans la configuration MCP de votre agent (Claude Desktop, Cursor…) :</p>
      <pre class="filebox" style="user-select:text;white-space:pre-wrap">${esc(cfg)}</pre>
      <button type="button" class="btn btn-ghost btn-sm" id="ai-cfg">Copier la configuration</button></div>`;
  }
  const a = kind === "mcp" ? "2" : "1", b = kind === "mcp" ? "3" : "2";
  out.innerHTML = mcpBlock + `
    <div class="ai-step"><div class="ai-step-h">${a}. Dites bonjour à votre agent</div>
      <p class="sub">Copiez ce message et collez-le dans votre assistant : il lui dit où est SituLab, l'invite à lire la fiche de collaboration et à vous donner son premier avis. Léger, ouvert — pas de commandes imposées.</p>
      <button type="button" class="btn btn-primary btn-sm" id="ai-prompt">Copier le message de présentation 🤖</button></div>
    <div class="ai-step"><div class="ai-step-h">${b}. Déposez la fiche de collaboration</div>
      <p class="sub"><code>AGENTS.md</code> est à la fois le mode d'emploi de votre agent ET votre fiche de collaboration : déposez-la dans votre projet (les agents la lisent automatiquement), votre IA la lira au démarrage et pourra l'enrichir avec vous. Modifiable aussi dans Réglages.</p>
      <button type="button" class="btn btn-ghost btn-sm" id="ai-doc">Déposer AGENTS.md dans un dossier…</button></div>`;
  if (el("ai-cfg")) el("ai-cfg").onclick = () => copyPrompt(cfg, { close: false, title: "Configuration MCP" });
  el("ai-prompt").onclick = () => copyPrompt(firstPromptText(kind, exe), { close: false, title: "Message de présentation" });
  el("ai-doc").onclick = installDirectives;
}

// installDirectives : choisit un dossier et y écrit le guide AGENTS.md pour l'agent de l'utilisateur.
async function installDirectives() {
  try {
    const dir = await App().PickFolder();
    if (!dir) return;
    const p = await App().InstallAgentsDoc(dir);
    toast("Directives enregistrées : " + shortPath(p), "ok");
  } catch (e) { toast(cleanErr(e), "err"); }
}

/* ---------------- Invite IA réutilisable (pour chaque action CRUD) ---------------- */
// copyPrompt copie un prompt, signale à l'utilisateur d'aller le coller dans son IA, et
// (par défaut) ferme le dialogue en cours. opts.close=false pour ne pas fermer (ex. Aide,
// ou modale « Jeton créé » qui doit rester visible). Repli visuel si le presse-papier est refusé.
async function copyPrompt(text, opts) {
  opts = opts || {};
  const close = opts.close !== false;
  try {
    await navigator.clipboard.writeText(text);
    toast("Prompt copié 🤖 — collez-le dans votre IA", "ok");
    if (close) closeModal();
  } catch (e) {
    infoModal(opts.title || "Prompt pour votre IA", `<p class="sub">Copie automatique refusée — sélectionnez puis copiez (Ctrl+C), et collez dans votre IA :</p><pre class="filebox" style="user-select:text;white-space:pre-wrap">${esc(text)}</pre>`);
  }
}

// aiInviteHTML : bloc « laissez votre IA le faire » (même habillage partout). À placer dans
// une modale CRUD ; appeler ensuite wireAiInvite(prompt) après openModal pour brancher le clic.
function aiInviteHTML(sub) {
  return `<div class="ai-invite">
    <div class="ai-invite-txt"><span class="ai-bot">🤖</span><div><b>Pas à l'aise ? Laissez votre IA le faire.</b><div class="sub">${esc(sub || "Votre agent est là pour ça. Copiez le prompt et collez-le dans votre IA — SituLab vous demandera votre PIN au moment d'agir.")}</div></div></div>
    <button type="button" class="btn btn-ghost btn-sm" id="ai-copy">Copier le prompt pour mon IA</button>
  </div>`;
}
// wireAiInvite branche le bouton de l'invite sur le prompt donné (texte, ou fonction qui le produit).
function wireAiInvite(prompt) {
  const b = el("ai-copy"); if (!b) return;
  b.onclick = () => copyPrompt(typeof prompt === "function" ? prompt() : prompt);
}

// Prompts par action — concis, avec la commande CLI exacte et le rappel « le PIN reste à moi ».
function aiHeader(verb) {
  const inst = curInst();
  return `${verb} via SituLab (instance « ${inst.name || state.instance} »). Utilise la CLI \`situlab\` et ajoute \`--instance "${inst.name || state.instance}"\`. Les écritures déclencheront une demande de MON PIN dans une fenêtre — ne me demande pas le PIN, ne le contourne pas.`;
}
function promptRepoCreate(owner) {
  const orgFlag = owner ? ` --org "${owner}"` : "";
  return [aiHeader("Crée un dépôt distant"), "",
    `Commande : situlab repo create "<nom-du-depot>" --description "<description>" --private${orgFlag}`,
    owner ? `- Ce serveur crée les dépôts dans l'organisation « ${owner} » (garde --org).` : "- Si le serveur refuse l'espace personnel, ajoute --org \"<organisation>\".",
    "- Enlève --private si le dépôt doit être public.",
    "- Demande-moi le nom et la description si tu ne les as pas.",
  ].join("\n");
}
function promptIssueCreate(owner, repo) {
  const slug = owner && repo ? `${owner}/${repo}` : "<owner>/<repo>";
  return [aiHeader("Ouvre une issue"), "",
    `Commande : situlab issue create ${slug} --title "<titre>" --body "<description>"`,
    "- Propose-moi un titre et une description clairs avant de lancer.",
  ].join("\n");
}
function promptRepoDelete(owner, repo) {
  const slug = owner && repo ? `${owner}/${repo}` : "<owner>/<repo>";
  return [aiHeader("Supprime un dépôt distant"), "",
    `Commande : situlab repo delete ${slug}`,
    "- Action irréversible : confirme bien le dépôt avec moi avant de lancer.",
  ].join("\n");
}
function promptUserCreate() {
  return [aiHeader("Crée un compte utilisateur (administration)"), "",
    'Commande : situlab admin user-create "<identifiant>" --email "<email>" --password "<mot-de-passe-provisoire>"',
    "- Ajoute --admin uniquement si la personne doit être administratrice.",
    "- Le compte devra changer son mot de passe à la première connexion.",
  ].join("\n");
}

/* ---------------- Administration ---------------- */
// userMgmtHeader : en-tête commun « Gestion des utilisateurs » + barre d'onglets (Admin / Annuaire
// / Statistiques). active = vue en cours. wireUserMgmtTabs() branche la navigation entre onglets.
function userMgmtHeader(active) {
  const tabs = [["admin", "Administration"], ["directory", "Annuaire"], ["stats", "Statistiques"]];
  return `<div class="view-head"><div class="view-title">Gestion des utilisateurs</div></div>
    <div class="um-tabs">${tabs.map(([v, l]) => `<button class="um-tab${v === active ? " active" : ""}" data-umv="${v}">${esc(l)}</button>`).join("")}</div>`;
}
function wireUserMgmtTabs() { document.querySelectorAll(".um-tab").forEach((t) => t.onclick = () => navTo(t.dataset.umv)); }

async function renderAdmin() {
  // Réservé aux administrateurs : sinon, message doux plutôt qu'une erreur sèche.
  if (!state.isAdmin) {
    el("view").innerHTML = `<div class="view-head"><div class="view-title">Administration</div></div>` +
      blankslate("⌘", "Réservé aux administrateurs", "Cet espace sert à gérer les comptes de l'équipe. Votre compte n'a pas les droits pour l'ouvrir — demandez à un administrateur si vous pensez que c'est une erreur.");
    return;
  }
  // Compte admin, mais l'accès SituLab n'a pas (encore) le droit de gestion : on propose de
  // l'activer à la demande (v1.4.2 — plus de case « admin » à la connexion).
  const hasScope = await App().AdminScopeFor(state.instance).catch(() => true);
  if (!hasScope) {
    el("view").innerHTML = userMgmtHeader("admin") +
      `<div class="callout"><b>Activer l'administration ?</b><div class="sub">Votre compte est administrateur, mais votre accès SituLab actuel n'inclut pas encore le droit de gérer les comptes. Activez-le pour continuer — SituLab recrée votre accès avec ce droit (identifiant + mot de passe + votre PIN).</div>
      <button class="btn btn-primary btn-sm" id="activate-admin" style="margin-top:10px">Activer l'administration</button></div>`;
    wireUserMgmtTabs();
    el("activate-admin").onclick = () => reconnectModal(state.instance, { admin: true });
    return;
  }
  el("view").innerHTML = userMgmtHeader("admin") + `
    <div class="view-sub" style="margin:-6px 0 10px">Gérer les comptes qui peuvent utiliser « ${esc(state.instance)} ».</div>
    <div class="toolbar">
      <button class="btn btn-primary btn-sm" id="new-user">＋ Nouvel utilisateur</button>
      ${serverLinkBtn(curInst(), "users", "Gérer sur le serveur")}
    </div>
    <div id="users"><div class="skeleton"></div></div>`;
  wireUserMgmtTabs();
  el("new-user").onclick = createUserModal;
  try {
    const users = await App().AdminUsers(state.instance);
    state.users = users || [];
    el("users").innerHTML = state.users.length ? state.users.map((u) => adminRow(u)).join("")
      : blankslate("☺", "Aucun compte pour l'instant", "Ajoutez la première personne avec « ＋ Nouvel utilisateur », en haut de l'écran.");
    wireAdminRows();
  } catch (e) {
    if (isLocked(e)) return handleLock();
    el("users").innerHTML = surfaceError("Impossible de charger les comptes", cleanErr(e), () => renderAdmin());
  }
}

// statut lisible d'un compte : suspendu prime, puis inactif, puis actif.
function userStatus(u) {
  if (u.prohibit_login) return { k: "suspendu", cls: "warn" };
  if (!u.active) return { k: "inactif", cls: "muted" };
  return { k: "actif", cls: "ver" };
}

/* ---------------- Annuaire consolidé multi-instances (v1.4) ---------------- */
async function renderDirectory() {
  if (!state.isAdmin) {
    el("view").innerHTML = `<div class="view-head"><div class="view-title">Annuaire</div></div>` +
      blankslate("☰", "Réservé aux administrateurs", "L'annuaire réunit les comptes de toutes vos instances Forgejo. Il faut un compte administrateur pour l'ouvrir.");
    return;
  }
  el("view").innerHTML = userMgmtHeader("directory") + `
    <div class="view-sub" style="margin:-6px 0 10px">Tous les comptes de vos instances Forgejo, réunis — recherche par nom ou e-mail sur toutes les instances à la fois.</div>
    <div class="toolbar"><input id="dir-search" type="text" placeholder="Rechercher une personne…" aria-label="Rechercher" value="${esc(state.dirSearch || "")}" /></div>
    <div id="dir-body"><div class="skeleton"></div></div>`;
  wireUserMgmtTabs();
  el("dir-search").oninput = (e) => { state.dirSearch = e.target.value; paintDirectory(); };
  try {
    state.directory = await App().ConsolidatedDirectory();
    paintDirectory();
  } catch (e) {
    if (isLocked(e)) return handleLock();
    el("dir-body").innerHTML = surfaceError("Impossible de charger l'annuaire", cleanErr(e), () => renderDirectory());
  }
}

const DIR_BADGE = { actif: "ver", suspendu: "warn", inactif: "muted", absent: "" };
function paintDirectory() {
  const d = state.directory;
  const body = el("dir-body");
  if (!d || !body) return;
  const okInst = (d.instances || []).filter((i) => i.ok);
  const ko = (d.instances || []).filter((i) => !i.ok);
  const q = (state.dirSearch || "").trim().toLowerCase();
  // e.search regroupe login + tous les e-mails/noms distincts (toutes instances) : la recherche
  // transverse trouve donc la personne même si son e-mail/nom diffère d'une instance à l'autre.
  const entries = (d.entries || []).filter((e) => !q || (e.search || e.login.toLowerCase()).includes(q));
  const koNote = ko.length
    ? `<div class="callout warn">Non interrogées : ${ko.map((i) => `${esc(i.name)} (${esc(i.error || "indisponible")})`).join(" · ")}</div>` : "";
  if (!okInst.length) {
    body.innerHTML = koNote + blankslate("☰", "Aucune instance interrogeable", "Aucune de vos instances Forgejo n'a pu être interrogée en administration. Reconnectez un compte administrateur.");
    return;
  }
  if (!entries.length) {
    body.innerHTML = koNote + blankslate("☺", "Aucun résultat", q ? "Aucun compte ne correspond à cette recherche." : "Aucun compte trouvé sur vos instances.");
    return;
  }
  const head = `<div class="dir-row dir-head"><div class="dir-who">Personne</div>${okInst.map((i) => `<div class="dir-cell">${esc(i.name)}</div>`).join("")}</div>`;
  const rows = entries.map((e) => {
    const cells = okInst.map((i) => {
      const st = e.presence[i.name] || "absent";
      return `<div class="dir-cell"><span class="pill ${DIR_BADGE[st] || ""}">${esc(st)}</span></div>`;
    }).join("");
    return `<div class="dir-row">
      <div class="dir-who"><b>${esc(e.login)}</b>${e.admin ? ' <span class="pill ver">admin</span>' : ""}<div class="muted">${esc(e.full_name || "")}${e.email ? " · " + esc(e.email) : ""}</div></div>
      ${cells}</div>`;
  }).join("");
  body.innerHTML = koNote + `<div class="dir-table">${head}${rows}</div>`;
}

function adminRow(u) {
  const st = userStatus(u);
  const self = u.login === state.login;
  const actions = self
    ? `<span class="muted" style="font-size:12px">vous</span>`
    : `<button class="btn btn-ghost btn-sm" data-act="detail" data-u="${esc(u.login)}">Droits</button>`
      + `<button class="btn btn-ghost btn-sm" data-act="susp" data-u="${esc(u.login)}" data-on="${u.prohibit_login ? "" : "1"}">${u.prohibit_login ? "Réactiver" : "Suspendre"}</button>`
      + `<button class="btn btn-ghost btn-sm btn-danger" data-act="del" data-u="${esc(u.login)}">Supprimer</button>`;
  return `<div class="srow">
    <div><b>${esc(u.login)}</b> <span class="pill ${u.is_admin ? "ver" : ""}">${u.is_admin ? "Administrateur" : "Standard"}</span>
      <span class="pill ${st.cls}">${st.k}</span> <span class="muted">${esc(u.email || "")}</span></div>
    <div class="srow-acts">${actions}</div></div>`;
}

function wireAdminRows() {
  document.querySelectorAll("#users [data-act]").forEach((b) => {
    const login = b.dataset.u;
    if (b.dataset.act === "detail") b.onclick = () => userDetail(login);
    else if (b.dataset.act === "susp") b.onclick = () => suspendUser(login, b.dataset.on === "1");
    else if (b.dataset.act === "del") b.onclick = () => deleteUser(login);
  });
}

function suspendUser(login, suspend) {
  const reason = suspend ? `Suspendre le compte « ${login} »` : `Réactiver le compte « ${login} »`;
  const intro = suspend
    ? `<div class="callout">La personne ne pourra plus se connecter. <b>Rien n'est effacé</b> : ses dépôts et son historique restent. Vous pourrez la réactiver à tout moment.</div>`
    : `<div class="callout">La personne pourra de nouveau se connecter et utiliser son compte.</div>`;
  openModal(suspend ? "Suspendre l'accès" : "Réactiver l'accès", `<div class="field">${intro}</div>`, async () => {
    pinConfirm(reason, async (pin) => {
      await App().AdminSuspendUser(state.instance, login, suspend, pin);
      closeModal();
      renderAdmin();
    });
  });
  el("modal-ok").textContent = "Continuer";
}

function deleteUser(login) {
  openModal("Supprimer définitivement", `<div class="field">
    <div class="callout warn"><b>Action irréversible.</b> Le compte « ${esc(login)} » sera supprimé de Forgejo. Pour simplement bloquer l'accès sans rien perdre, préférez <b>Suspendre</b>.</div>
    <label style="flex-direction:row;align-items:center"><input id="del-purge" type="checkbox" style="width:auto;margin-right:8px" />Supprimer aussi tous ses dépôts</label>
    <small>Sans la case, la suppression échoue si la personne possède encore des dépôts (sécurité Forgejo).</small>
    <label>Pour confirmer, tapez l'identifiant <b>${esc(login)}</b><input id="del-confirm" placeholder="${esc(login)}" /></label>
    <div class="inline-error hidden" id="del-error"></div></div>`, async () => {
    const er = el("del-error");
    if (el("del-confirm").value.trim() !== login) { er.textContent = "L'identifiant saisi ne correspond pas."; er.classList.remove("hidden"); return; }
    const purge = el("del-purge").checked;
    pinConfirm(`Supprimer définitivement « ${login} »`, async (pin) => {
      await App().AdminDeleteUser(state.instance, login, purge, pin);
      closeModal();
      renderAdmin();
    });
  });
  el("modal-ok").textContent = "Continuer";
}

function createUserModal() {
  openModal("Nouvel utilisateur", `<div class="field">
    <label>Identifiant<input id="uu" placeholder="ex. j.martin" /><small>Nom de connexion de la personne, sans espace.</small></label>
    <label>E-mail<input id="ue" type="email" placeholder="prenom.nom@exemple.fr" /></label>
    <label>Mot de passe initial<input id="up" type="password" /><small>À communiquer à la personne ; elle devra le changer à sa première connexion.</small></label>
    <label style="flex-direction:row;align-items:center"><input id="ua" type="checkbox" style="width:auto;margin-right:8px" />Donner les droits d'administrateur</label>
    <small>Sans la case, le compte est « Standard » : il utilise SituLab sans gérer les autres.</small>
    <div class="inline-error hidden" id="u-error"></div>
    ${aiInviteHTML()}</div>`, async () => {
    const er = el("u-error");
    const login = el("uu").value.trim(), email = el("ue").value.trim(), pwd = el("up").value, admin = el("ua").checked;
    if (!login || !email || !pwd) { er.textContent = "Identifiant, e-mail et mot de passe initial sont nécessaires."; er.classList.remove("hidden"); return; }
    pinConfirm(`Créer le compte « ${login} »`, async (pin) => {
      await App().AdminCreateUser(state.instance, login, email, pwd, admin, pin);
      closeModal();
      openModal("Compte créé", `<div class="field"><div class="callout">Transmettez à la personne son identifiant « ${esc(login)} » et le mot de passe provisoire que vous venez de saisir. <b>Elle devra le changer à sa première connexion.</b></div></div>`, () => { closeModal(); renderAdmin(); });
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(promptUserCreate);
}


// infoModal : modale de simple lecture, un seul bouton « Fermer ».
function infoModal(title, html) {
  openModal(title, html, () => closeModal());
  el("modal-ok").textContent = "Fermer";
  el("modal-cancel").classList.add("hidden");
}

// VISIBILITY : qui peut voir le profil de la personne.
const VISIBILITY = { public: "profil public", limited: "profil visible des membres connectés", private: "profil privé" };

// droitsResume : phrases claires de ce que la personne peut faire et voir.
function droitsResume(u) {
  const out = [];
  if (u.is_admin) out.push("Peut tout gérer sur ce serveur : comptes, dépôts et réglages.");
  else out.push("Compte standard : utilise SituLab pour ses dépôts, sans gérer les autres.");
  if (u.restricted) out.push("Accès restreint : ne voit que les dépôts qui lui sont explicitement partagés.");
  if (u.prohibit_login) out.push("Connexion suspendue : ne peut pas se connecter en ce moment.");
  const m = u.max_repo_creation;
  if (m === 0) out.push("Ne peut pas créer de nouveaux dépôts.");
  else if (m > 0) out.push(`Peut créer jusqu'à ${m} dépôt${m > 1 ? "s" : ""}.`);
  else out.push("Création de dépôts selon le réglage du serveur.");
  out.push(VISIBILITY[u.visibility] || "profil public");
  return out;
}

// userDetail : fiche « Droits » — ce que la personne peut faire et voir, + ses dépôts.
function userDetail(login) {
  const u = (state.users || []).find((x) => x.login === login) || { login };
  const st = userStatus(u);
  infoModal(`Droits de « ${login} »`, `<div class="field">
    <div class="drow"><span class="pill ${u.is_admin ? "ver" : ""}">${u.is_admin ? "Administrateur" : "Standard"}</span>
      <span class="pill ${st.cls}">${st.k}</span> <span class="muted">${esc(u.email || "")}</span></div>
    <ul class="droits">${droitsResume(u).map((s) => `<li>${esc(s)}</li>`).join("")}</ul>
    <div class="meta-grid">
      <div><span class="muted">Créé le</span><br>${esc(fmtDate(u.created)) || "—"}</div>
      <div><span class="muted">Dernière connexion</span><br>${esc(fmtDate(u.last_login)) || "—"}</div>
    </div>
    <h4 class="sub-h">Dépôts accessibles</h4>
    <div class="muted" style="font-size:11.5px;margin-bottom:6px">Ce que la personne possède ou peut consulter, y compris via ses organisations et équipes.</div>
    <div id="ud-repos"><div class="skeleton"></div></div></div>`);
  App().AdminUserRepos(state.instance, u.id).then((repos) => {
    const box = el("ud-repos"); if (!box) return;
    box.innerHTML = (repos || []).length
      ? (repos || []).map((r) => `<div class="repo-mini"><b>${esc(r.full_name)}</b> ${r.private ? '<span class="lock">🔒</span>' : ""}<span class="pill sm">${r.private ? "privé" : "public"}</span></div>`).join("")
      : `<div class="muted">Aucun dépôt accessible.</div>`;
  }).catch((e) => { const box = el("ud-repos"); if (box) box.innerHTML = `<div class="muted">${esc(cleanErr(e))}</div>`; });
}

/* ---------------- Statistiques & activités ---------------- */
// Libellés lisibles des types d'activité Forgejo.
const ACT_LABEL = {
  create_repo: "a créé un dépôt", rename_repo: "a renommé un dépôt", transfer_repo: "a transféré un dépôt",
  commit_repo: "a poussé des commits", create_branch: "a créé une branche", delete_branch: "a supprimé une branche",
  push_tag: "a publié un tag", delete_tag: "a supprimé un tag",
  create_issue: "a ouvert une issue", close_issue: "a fermé une issue", reopen_issue: "a rouvert une issue",
  comment_issue: "a commenté une issue",
  create_pull_request: "a ouvert une demande de fusion", close_pull_request: "a fermé une demande de fusion",
  reopen_pull_request: "a rouvert une demande de fusion", merge_pull_request: "a fusionné une demande de fusion",
  comment_pull: "a commenté une demande de fusion", approve_pull_request: "a approuvé une demande de fusion",
  reject_pull_request: "a demandé des changements", pull_review_dismissed: "a annulé une revue",
  star_repo: "a mis un dépôt en favori", mirror_sync_push: "a synchronisé un miroir",
};
const actLabel = (t) => ACT_LABEL[t] || (t ? t.replace(/_/g, " ") : "a eu une activité");

/* ---------------- Statistiques : analyse d'équipe (v1.5.0) ---------------- */
const statsCtx = { charts: [], report: null, metric: "commits", excluded: {}, progressOff: null };

function statsDestroyCharts() { statsCtx.charts.forEach((c) => { try { c.destroy(); } catch (e) {} }); statsCtx.charts = []; }

// statsPresetRange renvoie {since, until} (ISO) pour un préréglage de période.
function statsPresetRange(preset) {
  const now = new Date(), y = now.getFullYear(), m = now.getMonth(), d = now.getDate();
  const startOfWeek = (ref) => { const x = new Date(ref); const dow = (x.getDay() + 6) % 7; x.setDate(x.getDate() - dow); x.setHours(0, 0, 0, 0); return x; };
  let s, e;
  if (preset === "week") { s = startOfWeek(now); e = now; }
  else if (preset === "lastweek") { e = startOfWeek(now); s = new Date(e); s.setDate(s.getDate() - 7); }
  else if (preset === "month") { s = new Date(y, m, 1); e = now; }
  else if (preset === "lastmonth") { s = new Date(y, m - 1, 1); e = new Date(y, m, 1); }
  else return null;
  return { since: s.toISOString(), until: e.toISOString() };
}

async function renderStats() {
  if (!state.isAdmin) {
    el("view").innerHTML = `<div class="view-head"><div class="view-title">Statistiques</div></div>` +
      blankslate("▤", "Réservé aux administrateurs", "L'analyse d'équipe couvre l'activité de tous les comptes.");
    return;
  }
  statsDestroyCharts();
  el("view").innerHTML = userMgmtHeader("stats") + `
    <div class="view-sub" style="margin:-6px 0 10px">Qui a produit quoi, où et quand sur « ${esc(state.instance)} ». L'analyse interroge le serveur en arrière-plan ; le rapport est enregistré (« état au… ») et rejouable.</div>
    <div class="stats-config">
      <div class="sc-row"><span class="sc-lab">Période</span>
        <div class="seg" id="stats-presets" role="group">
          <button data-p="week">Semaine en cours</button><button data-p="lastweek">Semaine passée</button>
          <button data-p="month" class="on">Mois en cours</button><button data-p="lastmonth">Mois dernier</button>
          <button data-p="custom">Personnalisé</button>
        </div>
      </div>
      <div class="sc-row hidden" id="stats-custom"><span class="sc-lab">Du</span><input type="date" id="stats-from" /><span class="sc-lab">au</span><input type="date" id="stats-to" /></div>
      <div class="sc-row"><button class="btn btn-primary" id="stats-run">Lancer l'analyse</button>
        <span class="muted" id="stats-range-lbl"></span></div>
    </div>
    <div id="stats-progress" class="hidden"></div>
    <div id="stats-out"></div>
    <div class="blk" style="margin-top:18px"><div class="blk-h">Analyses enregistrées</div><div id="stats-history"><div class="muted">—</div></div></div>`;

  let preset = "month";
  const setPreset = (p) => {
    preset = p;
    document.querySelectorAll("#stats-presets button").forEach((b) => b.classList.toggle("on", b.dataset.p === p));
    el("stats-custom").classList.toggle("hidden", p !== "custom");
    const r = p === "custom" ? null : statsPresetRange(p);
    el("stats-range-lbl").textContent = r ? `du ${fmtDay(r.since)} au ${fmtDay(r.until)}` : "choisissez deux dates";
  };
  document.querySelectorAll("#stats-presets button").forEach((b) => b.onclick = () => setPreset(b.dataset.p));
  setPreset("month");
  wireUserMgmtTabs();
  el("stats-run").onclick = () => statsLaunch(preset);
  renderStatsHistory();
}

async function renderStatsHistory() {
  const box = el("stats-history"); if (!box) return;
  try {
    const list = await App().ListTeamReports();
    box.innerHTML = (list && list.length)
      ? list.map((r) => `<div class="srow"><div><b>État au ${esc(fmtDate(r.created_at))}</b> <span class="muted">· ${esc(fmtDay(r.since))} → ${esc(fmtDay(r.until))} · ${r.commits} commits · ${r.authors} personnes</span></div>
          <button class="btn btn-ghost btn-sm" data-rep="${esc(r.id)}">Ouvrir</button></div>`).join("")
      : `<div class="muted">Aucune analyse pour l'instant — lancez-en une ci-dessus.</div>`;
    box.querySelectorAll("[data-rep]").forEach((b) => b.onclick = async () => {
      try { const rep = await App().LoadTeamReport(b.dataset.rep); statsRenderReport(rep); window.scrollTo(0, 0); }
      catch (e) { toast(cleanErr(e), "err"); }
    });
  } catch (e) { box.innerHTML = `<div class="muted">${esc(cleanErr(e))}</div>`; }
}

async function statsLaunch(preset) {
  const range = preset === "custom"
    ? { since: el("stats-from").value ? new Date(el("stats-from").value + "T00:00:00").toISOString() : "", until: el("stats-to").value ? new Date(el("stats-to").value + "T23:59:59").toISOString() : "" }
    : statsPresetRange(preset);
  if (!range || !range.since || !range.until) { toast("Choisissez une période", "err"); return; }
  const prog = el("stats-progress"); el("stats-out").innerHTML = "";
  prog.classList.remove("hidden");
  prog.innerHTML = `<div class="stats-run-wait"><div class="spin"></div><div><b>Analyse en cours…</b><div class="muted" id="stats-prog-lbl">préparation…</div><div class="progbar"><span id="stats-prog-bar"></span></div></div></div>
    <div class="callout" style="margin-top:10px">⏳ Cette génération peut prendre <b>plusieurs minutes</b>. Vous pouvez <b>continuer à travailler</b> dans SituLab (changez de page sans souci) — le rapport apparaîtra dans « Analyses enregistrées » dès qu'il est prêt. <b>Ne fermez pas l'application</b> avant la fin (l'analyse s'arrêterait).</div>`;
  // Progression en arrière-plan (évènement du noyau).
  try { if (statsCtx.progressOff) statsCtx.progressOff(); } catch (e) {}
  try {
    statsCtx.progressOff = window.runtime.EventsOn("report:progress", (p) => {
      if (!p) return;
      const pct = p.total ? Math.round((p.done / p.total) * 100) : 0;
      const bar = el("stats-prog-bar"), lbl = el("stats-prog-lbl");
      if (bar) bar.style.width = pct + "%";
      if (lbl) lbl.textContent = p.repo ? `dépôt ${p.done}/${p.total} — ${p.repo}` : `${p.done}/${p.total} dépôts`;
    });
  } catch (e) {}
  const runBtn = el("stats-run"); if (runBtn) runBtn.disabled = true;
  try {
    const rep = await App().AnalyzeTeam(state.instance, [], range.since, range.until);
    // L'utilisateur a pu changer de page pendant l'analyse : le rapport est enregistré côté noyau
    // (il est dans l'historique). On ne met à jour l'affichage QUE si on est encore sur Statistiques.
    if (state.view !== "stats" || !el("stats-out")) return;
    prog.classList.add("hidden");
    statsCtx.excluded = {};
    statsRenderReport(rep);
    renderStatsHistory();
  } catch (e) {
    if (state.view !== "stats" || !el("stats-out")) return; // page quittée : l'erreur n'a plus de cible
    prog.classList.add("hidden");
    if (isLocked(e)) return handleLock();
    el("stats-out").innerHTML = surfaceError("Analyse impossible", cleanErr(e), null);
  } finally {
    const rb = el("stats-run"); if (rb) rb.disabled = false;
    try { if (statsCtx.progressOff) { statsCtx.progressOff(); statsCtx.progressOff = null; } } catch (e) {}
  }
}

function statsRenderReport(rep) {
  statsCtx.report = rep;
  statsCtx.excluded = {}; // repartir sans exclusions (y compris à l'ouverture depuis l'historique)
  statsDestroyCharts();
  const out = el("stats-out"); if (!out) return;
  const t = rep.totals || {};
  const kpis = `<div class="kpis">
    <div class="kpi"><div class="kpi-n">${t.commits || 0}</div><div class="kpi-l">commits</div></div>
    <div class="kpi"><div class="kpi-n" style="color:var(--ok)">+${(t.additions || 0).toLocaleString("fr-CH")}</div><div class="kpi-l">lignes ajoutées</div></div>
    <div class="kpi"><div class="kpi-n" style="color:var(--danger)">−${(t.deletions || 0).toLocaleString("fr-CH")}</div><div class="kpi-l">lignes supprimées</div></div>
    <div class="kpi"><div class="kpi-n">${t.authors || 0}</div><div class="kpi-l">personnes</div></div>
    <div class="kpi"><div class="kpi-n">${t.repos || 0}</div><div class="kpi-l">dépôts actifs</div></div>
  </div>`;
  out.innerHTML = `
    <div class="report-head"><div><b>État au ${esc(fmtDate(rep.created_at))}</b> <span class="muted">· ${esc(fmtDay(rep.since))} → ${esc(fmtDay(rep.until))} · ${rep.repos_scanned} dépôts analysés</span></div>
      <button class="btn btn-ghost btn-sm" id="stats-print">Exporter / imprimer</button></div>
    ${kpis}
    <div class="chart-grid">
      <div class="chart-card"><div class="chart-h">Par personne <span class="seg sm" id="stats-metric"><button data-m="commits" class="on">commits</button><button data-m="additions">+ lignes</button><button data-m="deletions">− lignes</button></span></div><div class="chart-wrap"><canvas id="ch-authors"></canvas></div></div>
      <div class="chart-card"><div class="chart-h">Activité par jour</div><div class="chart-wrap"><canvas id="ch-date"></canvas></div></div>
      <div class="chart-card"><div class="chart-h">Horaires d'activité (heure locale des commits)</div><div class="chart-wrap"><canvas id="ch-hour"></canvas></div></div>
      <div class="chart-card"><div class="chart-h">Dépôts les plus actifs</div><div class="chart-wrap"><canvas id="ch-repos"></canvas></div></div>
    </div>
    <div class="blk"><div class="blk-h">Détail par personne <span class="muted">(cliquez pour inclure/exclure des graphiques)</span></div><div id="stats-authors-tbl"></div></div>`;
  el("stats-print").onclick = () => window.print();
  el("stats-metric").querySelectorAll("button").forEach((b) => b.onclick = () => {
    statsCtx.metric = b.dataset.m;
    el("stats-metric").querySelectorAll("button").forEach((x) => x.classList.toggle("on", x === b));
    statsDrawAuthors(rep);
  });
  statsDrawAuthors(rep);
  statsDrawDate(rep);
  statsDrawHour(rep);
  statsDrawRepos(rep);
  statsDrawAuthorTable(rep);
}

// Palette + fabrique de graphiques Chart.js (thème-aware via les variables CSS).
function cssVar(n) { return getComputedStyle(document.documentElement).getPropertyValue(n).trim() || "#4b7bec"; }
function mkChart(id, cfg) {
  const cv = el(id); if (!cv || typeof Chart === "undefined") return;
  // Chart.js v4 refuse de réutiliser un canvas occupé : on détruit l'instance précédente d'abord
  // (statsDrawAuthors est ré-appelée à chaque filtre métrique/exclusion).
  const prev = Chart.getChart(cv);
  if (prev) { try { prev.destroy(); } catch (e) {} statsCtx.charts = statsCtx.charts.filter((c) => c !== prev); }
  cfg.options = Object.assign({ responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } } }, cfg.options || {});
  statsCtx.charts.push(new Chart(cv.getContext("2d"), cfg));
}
function statsActiveAuthors(rep) { return (rep.authors || []).filter((a) => !statsCtx.excluded[a.login || a.name]); }

function statsDrawAuthors(rep) {
  const m = statsCtx.metric, top = statsActiveAuthors(rep).slice(0, 12);
  mkChart("ch-authors", {
    type: "bar",
    data: { labels: top.map((a) => a.login || a.name || "?"), datasets: [{ data: top.map((a) => a[m] || 0), backgroundColor: cssVar("--accent"), borderRadius: 4 }] },
    options: { indexAxis: "y", scales: { x: { beginAtZero: true } } },
  });
}
function statsDrawDate(rep) {
  const dates = Object.keys(rep.by_date || {}).sort();
  mkChart("ch-date", {
    type: "line",
    data: { labels: dates, datasets: [{ data: dates.map((d) => rep.by_date[d]), borderColor: cssVar("--accent"), backgroundColor: "transparent", tension: .3, pointRadius: 2, fill: false }] },
    options: { scales: { y: { beginAtZero: true } } },
  });
}
function statsDrawHour(rep) {
  mkChart("ch-hour", {
    type: "bar",
    data: { labels: Array.from({ length: 24 }, (_, h) => h + "h"), datasets: [{ data: (rep.by_hour || []), backgroundColor: cssVar("--info"), borderRadius: 3 }] },
    options: { scales: { y: { beginAtZero: true } } },
  });
}
function statsDrawRepos(rep) {
  const top = (rep.repos || []).slice(0, 10);
  mkChart("ch-repos", {
    type: "bar",
    data: { labels: top.map((r) => r.full_name), datasets: [{ data: top.map((r) => r.commits), backgroundColor: cssVar("--ok"), borderRadius: 4 }] },
    options: { indexAxis: "y", scales: { x: { beginAtZero: true } } },
  });
}
function statsDrawAuthorTable(rep) {
  const box = el("stats-authors-tbl"); if (!box) return;
  box.innerHTML = (rep.authors || []).map((a) => {
    const k = a.login || a.name, off = statsCtx.excluded[k];
    return `<div class="srow stats-arow${off ? " off" : ""}" data-a="${esc(k)}">
      <div><b>${esc(a.login || a.name || "?")}</b>${a.login && a.name && a.login !== a.name ? ` <span class="muted">${esc(a.name)}</span>` : ""}</div>
      <div class="muted">${a.commits} commits · <span style="color:var(--ok)">+${a.additions}</span> / <span style="color:var(--danger)">−${a.deletions}</span> · ${a.repos} dépôts</div></div>`;
  }).join("") || `<div class="muted">Aucune activité sur la période.</div>`;
  box.querySelectorAll(".stats-arow").forEach((row) => row.onclick = () => {
    const k = row.dataset.a; statsCtx.excluded[k] = !statsCtx.excluded[k];
    row.classList.toggle("off", !!statsCtx.excluded[k]);
    statsDrawAuthors(rep);
  });
}

async function refreshWhoami() {
  const w = el("whoami"), live = el("live");
  const was = state.online;
  try {
    const u = await App().Whoami(state.instance);
    state.online = true; state.offlineReason = null;
    state.isAdmin = !!u.is_admin;
    state.login = u.login;
    const gh = curIsGitHub();
    w.innerHTML = `<span class="dot"></span>${esc(u.login)}${gh ? ' · <span class="prov-tag">GitHub</span>' : (u.is_admin ? " · admin" : "")}`;
    live.classList.remove("off");
    // Admin, Statistiques et Tokens sont propres à Forgejo (API absente sur GitHub).
    const na = el("nav-admin"); if (na) na.classList.toggle("hidden", gh || !u.is_admin); // « Gestion des utilisateurs »
    const nt = document.querySelector('.nav-item[data-view="tokens"]'); if (nt) nt.classList.toggle("hidden", gh);
  } catch (e) {
    if (isLocked(e)) { handleLock(); return; }
    state.online = false;
    state.offlineReason = isAuthError(e) ? "auth" : "net";
    w.innerHTML = `<span class="dot off"></span>${state.offlineReason === "auth" ? "déconnecté" : "hors ligne"}`;
    live.classList.add("off");
  }
  // Bascule d'état (en ligne ⇄ hors ligne) : si l'écran courant est bridé, on le (dé)verrouille
  // aussitôt — la boucle reste cohérente sans attendre une action de l'utilisateur.
  if (was !== state.online && !OFFLINE_OK_VIEWS.includes(state.view)) render();
}

/* ---------------- Live loop (anti-réentrance, éco-ressources v1.3) ----------------
   La boucle se suspend quand la fenêtre est masquée/réduite et ralentit après une
   inactivité prolongée : l'application reste souvent ouverte en arrière-plan, inutile
   d'y relancer `git status` et d'interroger le serveur toutes les 3,5 s pour rien. */
const LIVE_MS = 3500;         // fenêtre active, utilisateur présent
const LIVE_SLOW_MS = 20000;   // ralenti après inactivité prolongée
const IDLE_AFTER_MS = 120000; // 2 min sans interaction → ralenti
let lastInteraction = Date.now();
let ecoInstalled = false;
let liveGen = 0; // génération de la boucle : un tick en vol dont la génération a changé ne se replanifie pas
function markInteraction() { lastInteraction = Date.now(); }

async function liveTickOnce() {
  if (liveBusy) return;
  liveBusy = true; liveTick++;
  try {
    if (liveTick % 3 === 0) await refreshWhoami();
    if (state.view === "repos") await renderRepos(true);
    else if (state.view === "projects") await renderProjects(true);
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
        <div class="view-sub">« ${esc(state.instance)} » <span id="checked-at" class="checked"></span></div>
      </div>
      <div class="toolbar">
        <input id="repo-search" type="text" placeholder="Rechercher un dépôt…" aria-label="Rechercher" value="${esc(state.search)}" />
        <select id="repo-org" class="select" aria-label="Filtrer par organisation"></select>
        <div class="seg sm" id="repo-view" role="group" aria-label="Présentation">
          <button data-v="grid" title="Grille"${state.repoView === "grid" ? ' class="on"' : ""}>▦ Grille</button>
          <button data-v="list" title="Liste"${state.repoView === "list" ? ' class="on"' : ""}>☰ Liste</button>
        </div>
        <button type="button" id="hide-nolocal" class="chip${state.hideNolocal ? " on" : ""}" aria-pressed="${state.hideNolocal ? "true" : "false"}" title="N'afficher que les dépôts copiés sur ce poste">Sur ce poste</button>
        <button class="btn btn-ghost btn-sm" id="link-local">📁 Lier un dépôt local</button>
        <button class="btn btn-primary btn-sm" id="new-repo">＋ Créer un dépôt</button>
      </div>
      <div id="repo-list">${state.repos ? "" : skeletons(4)}</div>`;
    el("repo-search").oninput = (e) => { state.search = e.target.value; paintRepos(); };
    el("repo-org").onchange = (e) => { state.org = e.target.value; paintRepos(); };
    el("new-repo").onclick = createRepoModal;
    el("link-local").onclick = linkLocalRepo;
    document.querySelectorAll("#repo-view button").forEach((b) => b.onclick = () => {
      state.repoView = b.dataset.v;
      try { localStorage.setItem("situlab.repoView", state.repoView); } catch (e) {}
      document.querySelectorAll("#repo-view button").forEach((x) => x.classList.toggle("on", x === b));
      paintRepos();
    });
    el("hide-nolocal").onclick = (e) => {
      state.hideNolocal = !state.hideNolocal;
      e.currentTarget.classList.toggle("on", state.hideNolocal);
      e.currentTarget.setAttribute("aria-pressed", state.hideNolocal ? "true" : "false");
      try { localStorage.setItem("situlab.hideNolocal", state.hideNolocal ? "1" : "0"); } catch (err) {}
      paintRepos();
    };
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
  const shown = repos.filter((r) => {
    if (state.org && r.owner !== state.org) return false;
    if (state.hideNolocal && (r.state || "nolocal") === "nolocal") return false;
    if (q && !(r.full_name + " " + r.name + " " + (r.local_path || "")).toLowerCase().includes(q)) return false;
    return true;
  });

  if (shown.length === 0) {
    list.innerHTML = repos.length === 0
      ? blankslate("◍", "Aucun dépôt", "Votre token ne donne accès à aucun dépôt sur cette instance.")
      : blankslate("⌕", "Aucun résultat", state.hideNolocal && repos.length ? "Aucun dépôt n'est copié sur ce poste. Désactivez le filtre « Sur ce poste » pour voir les autres." : "Aucun dépôt ne correspond à votre recherche.");
    return;
  }

  // Regroupement par DÉPÔT DISTANT : un dépôt = UNE carte, ses clones locaux listés dedans (plus de
  // carte par clone). Les dépôts purement locaux (hors instance) restent individuels.
  const byRepo = new Map(), order = [];
  shown.forEach((r) => {
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

  // Regroupement logique : l'actionnable d'abord, « à jour » ensuite, « pas sur ce poste » en fin.
  const GROUPS = [
    ["dirty", "Modifications en cours", ["dirty"]],
    ["ahead", "En avance — à envoyer", ["ahead"]],
    ["behind", "En retard — nouveautés en ligne", ["behind"]],
    ["diverged", "À réunir (changements croisés)", ["diverged"]],
    ["attention", "À vérifier", ["detached", "unreadable"]],
    ["uptodate", "À jour", ["uptodate"]],
    ["nolocal", "Pas sur ce poste", ["nolocal"]],
  ];
  const gridClass = state.repoView === "list" ? "repo-grid list" : "repo-grid";
  const content = document.querySelector(".content");
  const scroll = content ? content.scrollTop : 0;
  let html = state.truncated ? `<div class="offline-banner">Beaucoup de dépôts : la liste affichée est partielle.</div>` : "";
  GROUPS.forEach(([k, title, states]) => {
    const items = aggregated.map((r, i) => [r, i]).filter(([r]) => states.includes(r.state || "nolocal"));
    if (!items.length) return;
    html += `<div class="repo-group"><div class="repo-group-h">${esc(title)} <span class="rg-count">${items.length}</span></div>` +
      `<div class="${gridClass}">` + items.map(([r, i]) => repoCard(r, i)).join("") + `</div></div>`;
  });
  list.innerHTML = html;
  // Délégation sur le conteneur STABLE #repo-list (survit aux repaints « en direct »), lecture via
  // state._shown (jamais de closure périmée) : « Ouvrir le détail » et le dépliage restent fiables.
  state._shown = aggregated;
  const openClone = (val) => { const [gi, ci] = String(val).split(":").map(Number); const g = state._shown[gi]; const c = g && g._clones && g._clones[ci]; if (c) openDetail(c); };
  list.onclick = (ev) => {
    const oc = ev.target.closest("[data-openc]");
    if (oc) { openClone(oc.dataset.openc); return; }
    const o = ev.target.closest("[data-open]");
    if (o) { const r = state._shown[+o.dataset.i]; if (r) openDetail(r); return; }
    const h = ev.target.closest(".repo-head[data-head]");
    if (h) { const i = +h.dataset.i, r = state._shown[i]; if (r) toggleRepoCard(r, i); }
  };
  list.onkeydown = (ev) => {
    if (ev.key !== "Enter" && ev.key !== " ") return;
    const oc = ev.target.closest("[data-openc]");
    if (oc) { ev.preventDefault(); openClone(oc.dataset.openc); return; }
    const h = ev.target.closest(".repo-head[data-head]");
    if (h) { ev.preventDefault(); const i = +h.dataset.i, r = state._shown[i]; if (r) toggleRepoCard(r, i); }
  };
  // Cartes mono-clone restées dépliées : rafraîchissement SILENCIEUX (le contenu caché reste affiché,
  // pas de skeleton) — la carte ne « se referme » plus au tick « en direct ».
  aggregated.forEach((r, i) => { if (!(r._clones && r._clones.length > 1) && state.expanded[repoKey(r)]) loadBranches(r, i, true); });
  if (content) content.scrollTop = scroll;
}

function createRepoModal() {
  // Propriétaires possibles : moi (espace perso) + les organisations vues dans mes dépôts.
  const orgs = [...new Set((state.repos || []).filter((r) => r.has_remote && r.owner && r.owner !== state.login).map((r) => r.owner))].sort();
  const ownerOpts = `<option value="">Moi (${esc(state.login || "espace personnel")})</option>` +
    orgs.map((o) => `<option value="${esc(o)}">${esc(o)} (organisation)</option>`).join("");
  openModal("Créer un dépôt", `<div class="field">
    <label>Propriétaire<select id="nowner" class="select">${ownerOpts}</select>
      <small>Beaucoup de serveurs interdisent les dépôts personnels : choisissez une organisation si « Moi » est refusé.</small></label>
    <label>Nom<input id="nr" placeholder="mon-projet" /></label>
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
      closeModal(); toast("Dépôt créé", "ok"); state.repoSig = ""; renderRepos(false);
    });
  });
  // Si des organisations existent, on en présélectionne une (l'espace perso est souvent bloqué).
  if (orgs.length) el("nowner").value = orgs[0];
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => promptRepoCreate(el("nowner") ? el("nowner").value : ""));
}

// deleteRepoModal : suppression définitive d'un dépôt distant, avec garde-fou (retaper le
// nom), invite IA, et confirmation par PIN.
function deleteRepoModal(owner, repo) {
  const slug = `${owner}/${repo}`;
  openModal("Supprimer le dépôt", `<div class="field">
    <div class="callout warn"><b>Action irréversible.</b> Le dépôt « ${esc(slug)} » et tout son contenu seront supprimés du serveur.</div>
    <label>Pour confirmer, tapez <b>${esc(repo)}</b><input id="dr-confirm" placeholder="${esc(repo)}" /></label>
    <div class="inline-error hidden" id="dr-error"></div>
    ${aiInviteHTML("Vous pouvez aussi demander la suppression à votre IA. Copiez le prompt et collez-le dans votre IA — SituLab vous demandera votre PIN au moment d'agir.")}</div>`, async () => {
    const er = el("dr-error");
    if (el("dr-confirm").value.trim() !== repo) { er.textContent = "Le nom saisi ne correspond pas."; er.classList.remove("hidden"); return; }
    pinConfirm(`Supprimer définitivement « ${slug} »`, async (pin) => {
      await App().DeleteRepo(state.instance, owner, repo, pin);
      closeModal(); toast("Dépôt supprimé", "ok"); state.repoSig = ""; navTo("repos");
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => promptRepoDelete(owner, repo));
}

// linkLocalRepo : assistant « lier un dépôt local ». Deux voies : manuelle (sélecteur de
// dossier natif) ou déléguée à l'IA (prompt prêt à coller). L'humain choisit.
function linkLocalRepo() {
  infoModal("Lier un dépôt local", `<div class="field">
    <p class="sub">Rattachez un dossier git de ce poste à son dépôt en ligne.</p>
    <div class="link-opt">
      <div class="link-opt-h">À la main</div>
      <p class="sub">Choisissez le dossier (celui qui contient le sous-dossier <b>.git</b>).</p>
      <button class="btn btn-primary btn-sm" type="button" id="ll-pick">📁 Choisir un dossier…</button>
    </div>
    <div class="link-opt">
      <div class="link-opt-h">🤖 Ou laissez votre IA le faire</div>
      <p class="sub">Votre agent est là pour ça : il trouve vos dépôts et les relie tout seul. Copiez ce prompt et collez-le dans votre IA.</p>
      <button class="btn btn-ghost btn-sm" type="button" id="ll-prompt">Copier le prompt pour mon IA</button>
    </div></div>`);
  el("ll-pick").onclick = () => { closeModal(); linkLocalPick(); };
  el("ll-prompt").onclick = () => copyLinkLocalPrompt();
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
    state.repoSig = ""; renderRepos(false);
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

// repoCardGrouped : carte d'un dépôt distant ayant PLUSIEURS clones locaux. En-tête = identité
// distante ; dessous, une ligne par clone (dossier, type worktree/clone, branche, écart, état).
// Chaque ligne ouvre le détail de SON clone. Pas de dépliage de branches ici (via le détail).
function repoCardGrouped(r, i) {
  const vis = `<span class="pill">${r.private ? "privé" : "public"}</span>`;
  const n = r._clones.length;
  const rows = r._clones.map((c, ci) => {
    const kind = c.local_kind === "worktree" ? `<span class="pill sm" title="rattaché au dépôt principal (git worktree)">worktree</span>`
      : c.local_kind === "clone" ? `<span class="pill sm" title="clone autonome">clone</span>` : "";
    const bits = [];
    if (c.local_error) { bits.push(`<span class="muted">${esc(c.local_error)}</span>`); }
    else {
      bits.push(`<span class="pill" title="version de travail (branche git)">⎇ ${esc(c.local_branch || "?")}</span>`);
      if (c.ahead) bits.push(`<span class="s-ahead" title="à envoyer">↑${c.ahead}</span>`);
      if (c.behind) bits.push(`<span class="s-behind" title="à récupérer">↓${c.behind}</span>`);
      if (c.changed_count) bits.push(`<span class="hint">${c.changed_count} modif.</span>`);
      if (c.last_date) bits.push(`<span class="repo-when">maj ${esc(fmtWhen(c.last_date))}</span>`);
    }
    return `<div class="clone-row" data-openc="${i}:${ci}" role="button" tabindex="0" title="Ouvrir : ${esc(c.local_path)}">
      <div class="clone-main"><span class="pill pill-folder">📁 ${esc(folderOf(c.local_path))}</span>${kind ? " " + kind : ""}</div>
      <div class="clone-bits">${bits.join(" ")} ${stateBadge(c.state)}</div>
    </div>`;
  }).join("");
  return `<div class="repo-card grouped ${(STATES[r.state] || STATES.nolocal).cls}" data-key="${esc("R:" + r.full_name)}" data-i="${i}">
    <div class="repo-head repo-head-static">
      <div class="repo-title">${repoTitleHTML(r.full_name)} ${vis} <span class="pill sm clone-count" title="Ce même dépôt est copié ${n} fois sur ce poste (dossiers ou branches différents)">📁 ${n} copies</span></div>
      ${stateBadge(r.state)}
    </div>
    <div class="repo-sub muted">Plusieurs copies locales du même dépôt — un dossier par ligne (branche / état propres).</div>
    <div class="clone-list">${rows}</div>
  </div>`;
}

// repoCard : carte épurée. En-tête = nom + état d'un coup d'œil. Clic sur l'en-tête =
// déplie l'état par branche (chargé à la demande). « Ouvrir » = vue détaillée complète.
function repoCard(r, i) {
  if (r._clones && r._clones.length > 1) return repoCardGrouped(r, i);
  const title = r.has_remote ? r.full_name : r.name;
  const key = repoKey(r);
  const open = !!state.expanded[key];
  const vis = r.has_remote
    ? `<span class="pill">${r.private ? "privé" : "public"}</span>`
    : `<span class="pill">local</span>`;

  // Résumé instantané (sans calcul git) : branche + version (tag seul, jamais de n° de commit) +
  // écart + date de dernière mise à jour.
  const sum = [];
  if (r.local_path) {
    sum.push(`<span class="pill pill-folder" title="dossier local : ${esc(r.local_path)}">📁 ${esc(folderOf(r.local_path))}</span>`);
    sum.push(`<span class="pill" title="version de travail (branche git)">⎇ ${esc(r.local_branch || "?")}</span>`);
    const tag = cleanTag(r.describe);
    if (tag) sum.push(`<span class="pill ver" title="version publiée">${esc(tag)}</span>`);
    if (r.ahead) sum.push(`<span class="s-ahead" title="à envoyer">↑${r.ahead}</span>`);
    if (r.behind) sum.push(`<span class="s-behind" title="à récupérer">↓${r.behind}</span>`);
    if (r.changed_count) sum.push(`<span class="hint">${r.changed_count} modif.</span>`);
    if (r.last_date) sum.push(`<span class="repo-when">maj ${esc(fmtWhen(r.last_date))}</span>`);
  } else if (r.local_error) {
    sum.push(`<span class="muted">${esc(r.local_error)}</span>`);
  } else {
    sum.push(`<span class="muted">pas copié sur ce poste</span>`);
  }

  return `<div class="repo-card ${(STATES[r.state] || STATES.nolocal).cls}" data-key="${esc(key)}" data-i="${i}">
    <div class="repo-head" data-head data-i="${i}" role="button" tabindex="0" aria-expanded="${open}" title="Cliquez pour voir les branches">
      <div class="repo-title"><span class="chev">${open ? "▾" : "▸"}</span> ${r.has_remote ? repoTitleHTML(title) : `<span class="nm">${esc(title)}</span>`} ${vis}</div>
      ${stateBadge(r.state)}
    </div>
    <div class="repo-summary">${sum.join(" ")}</div>
    <div class="repo-branches ${open ? "" : "hidden"}" id="rbody-${i}">${open ? (state.branchCache[key] || `<div class="skeleton"></div>`) : ""}</div>
    <div class="repo-foot"><span class="repo-hint">${open ? "branches ci-dessus" : "▸ branches"}</span><button class="btn btn-ghost btn-sm" data-open data-i="${i}">Ouvrir le détail ›</button></div>
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

// toggleRepoCard : ouvre/ferme l'état par branche d'une carte, en le chargeant la 1re fois.
function toggleRepoCard(r, i) {
  const key = repoKey(r), body = el("rbody-" + i);
  const card = body && body.closest(".repo-card");
  const chev = card && card.querySelector(".chev");
  const head = card && card.querySelector("[data-head]");
  if (!body) return;
  const nowOpen = body.classList.contains("hidden");
  body.classList.toggle("hidden", !nowOpen);
  if (chev) chev.textContent = nowOpen ? "▾" : "▸";
  if (head) head.setAttribute("aria-expanded", String(nowOpen));
  if (nowOpen) { state.expanded[key] = true; loadBranches(r, i); }
  else { delete state.expanded[key]; body.dataset.loaded = ""; }
}

// loadBranches charge (ou rafraîchit) le graphe de branches d'une carte. silent=true : rafraîchit
// SANS repasser par un skeleton (refresh de fond du « en direct ») — on garde le contenu affiché à
// l'écran et on le remplace seulement une fois les données prêtes. Le résultat est mis en cache
// (state.branchCache) pour que le prochain repaint réaffiche la carte dépliée sans flicker.
async function loadBranches(r, i, silent) {
  const body = el("rbody-" + i); if (!body) return;
  const key = repoKey(r);
  if (body.dataset.loading === "1") return; // évite deux chargements concurrents
  body.dataset.loading = "1";
  if (!silent && !state.branchCache[key]) body.innerHTML = `<div class="skeleton"></div>`;
  const put = (htmlStr) => { state.branchCache[key] = htmlStr; const b = el("rbody-" + i); if (b) { b.innerHTML = htmlStr; b.dataset.loaded = "1"; b.dataset.loading = ""; } };
  try {
    if (r.local_path) {
      put(branchMatrixHTML(await App().BranchMatrix(r.local_path)));
    } else if (r.has_remote) {
      const { owner, repo } = splitFull(r.full_name);
      put(remoteOnlyHTML(await App().RepoBranches(state.instance, owner, repo)));
    } else {
      put(`<div class="muted" style="padding:6px 0">Aucune information de branche.</div>`);
    }
  } catch (e) {
    body.dataset.loading = "";
    if (isLocked(e)) return handleLock();
    if (!state.branchCache[key]) body.innerHTML = `<div class="muted" style="padding:6px 0">${esc(cleanErr(e))}</div>`;
  }
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

// remoteOnlyHTML : dépôt pas copié ici — graphe des branches distantes (nœud « En ligne » seul).
function remoteOnlyHTML(branches) {
  if (!(branches || []).length) return `<div class="muted" style="padding:6px 0">Aucune branche distante.</div>`;
  const lanes = branches.map((b) => branchLane({
    name: b.name, hasLocal: false, hasRemote: true, remoteDate: b.commit_date,
  })).join("");
  return `<div class="branch-graph">${lanes}</div><div class="bg-foot">Ce dépôt n'est pas copié sur ce poste. Ouvrez le détail pour le récupérer.</div>`;
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

/* ---------------- Détail ---------------- */
function openDetail(r) {
  if (!r || (!r.has_remote && !r.local_path)) return;
  const { owner, repo } = r.has_remote ? splitFull(r.full_name) : { owner: "", repo: r.name };
  state.detail = {
    full_name: r.full_name || r.name, owner, repo, has_remote: r.has_remote,
    html_url: r.html_url || "", local_path: r.local_path || "", ref: r.default_branch || "", tab: "apercu",
    path: "", refs: [], tags: [], _remote: null, _local: null,
  };
  state.view = "detail";
  render();
}

async function renderDetail() {
  const d = state.detail; if (!d) return;
  el("view").innerHTML = `
    <div class="detail-head">
      <button class="btn-back" id="back">‹ Dépôts</button>
      <div class="detail-title">${repoTitleHTML(d.full_name || d.repo)}</div>
      <div class="detail-actions">
        ${d.has_remote ? `<label class="sr-only" for="ref-sel">Branche ou version</label><select id="ref-sel" class="select ref-sel"></select>` : ""}
        ${d.has_remote ? `<button class="btn btn-ghost btn-sm" id="open-web">Ouvrir en ligne ↗</button>` : ""}
        ${d.local_path && state.advanced && !state.readOnly ? `<button class="btn btn-ghost btn-sm" id="git-local">⎇ Git local</button>` : ""}
        ${d.local_path ? `<button class="btn btn-ghost btn-sm" id="unfollow">Ne plus suivre</button>` : ""}
        ${d.has_remote && state.advanced && !state.readOnly ? `<button class="btn btn-ghost btn-sm" id="repo-settings">⚙ Réglages</button>` : ""}
        ${d.has_remote ? `<button class="btn btn-ghost btn-sm btn-danger" id="del-repo">Supprimer</button>` : ""}
      </div>
    </div>
    <div class="tabs">
      <button class="tab active" data-tab="apercu">Aperçu</button>
      ${d.has_remote ? `<button class="tab" data-tab="fichiers">Fichiers</button><button class="tab" data-tab="histo">Historique</button><button class="tab" data-tab="activite">Activité</button>` : ""}
    </div>
    <div id="detail-body"><div class="skeleton"></div></div>`;
  el("back").onclick = () => navTo("repos");
  if (el("open-web")) el("open-web").onclick = () => safeOpenURL(d.html_url || ((state.repos || []).find((x) => x.full_name === d.full_name) || {}).html_url);
  if (el("unfollow")) el("unfollow").onclick = () => {
    // Protection (1.9.2) : « ne plus suivre » demande désormais une confirmation explicite. Le dossier
    // local et son contenu ne sont PAS touchés — seul le suivi dans SituLab est retiré.
    openModal("Ne plus suivre ce dépôt ?", `<div class="field"><div class="callout">SituLab cessera d'afficher le clone local <b>${esc(d.local_path)}</b>. <b>Aucun fichier n'est supprimé</b> : le dossier reste sur votre poste et peut être resuivi plus tard.</div></div>`, async () => {
      try { await App().RemoveLocalRepo(d.local_path); closeModal(); toast("Dépôt retiré du suivi", "ok"); navTo("repos"); } catch (e) { toast(cleanErr(e), "err"); }
    });
    el("modal-ok").textContent = "Ne plus suivre";
  };
  if (el("git-local")) el("git-local").onclick = () => gitLocalModal(d);
  if (el("repo-settings")) el("repo-settings").onclick = () => repoSettingsModal(d);
  if (el("del-repo")) el("del-repo").onclick = () => deleteRepoModal(d.owner, d.repo);
  document.querySelectorAll(".tab").forEach((t) => {
    t.onclick = () => {
      document.querySelectorAll(".tab").forEach((x) => x.classList.remove("active"));
      t.classList.add("active"); d.tab = t.dataset.tab; d.path = ""; paintDetail();
    };
  });

  if (d.has_remote) {
    try {
      const [branches, tags] = await Promise.all([
        App().RepoBranches(state.instance, d.owner, d.repo),
        App().RepoTags(state.instance, d.owner, d.repo),
      ]);
      if (state.detail !== d) return; // l'utilisateur a quitté pendant l'attente
      d.refs = (branches || []).map((b) => b.name);
      d.tags = (tags || []).map((t) => t.name);
      const sel = el("ref-sel");
      if (sel) {
        const grp = (g, arr) => arr.length ? `<optgroup label="${g}">` + arr.map((x) => `<option value="${esc(x)}"${x === d.ref ? " selected" : ""}>${esc(x)}</option>`).join("") + `</optgroup>` : "";
        sel.innerHTML = grp("Branches", d.refs) + grp("Versions", d.tags);
        sel.onchange = (e) => { d.ref = e.target.value; d.path = ""; paintDetail(); refreshDetailRemote(); };
      }
    } catch (e) {}
  }
  await refreshDetailRemote();
  await refreshDetailLocal();
  if (state.detail === d) paintDetail();
}

async function refreshDetailRemote() {
  const d = state.detail; if (!d || !d.has_remote) return;
  try {
    const commits = await App().RepoCommits(state.instance, d.owner, d.repo, d.ref, 20);
    if (state.detail !== d) return;
    d._remote = { commits: commits || [] };
    if ((d.tab === "apercu" || d.tab === "histo") && !d._diffOpen) paintDetail(true);
  } catch (e) {}
}
async function refreshDetailLocal() {
  const d = state.detail; if (!d || !d.local_path) return;
  try {
    const loc = await App().LocalDetail(d.local_path);
    if (state.detail !== d) return;
    d._local = loc;
    if (d.tab === "apercu" && !d._diffOpen) paintDetail(true);
  } catch (e) {}
}

function paintDetail(liveOnly) {
  const d = state.detail, body = el("detail-body"); if (!d || !body) return;
  if (d.tab === "apercu") return paintApercu(body, liveOnly);
  if (d.tab === "fichiers") return paintFiles(body, liveOnly);
  if (d.tab === "histo") return paintHisto(body, liveOnly);
  if (d.tab === "activite") return paintActivite(body, liveOnly);
}

async function paintActivite(body, liveOnly) {
  if (liveOnly) return;
  const d = state.detail;
  body.innerHTML = `<div class="toolbar">
      <button class="btn btn-primary btn-sm" id="new-issue" title="Une issue = une note de suivi : un bug à corriger, une idée, une question. Elle reste ouverte jusqu'à ce qu'on la ferme.">＋ Nouvelle issue</button>
      <button class="btn btn-ghost btn-sm" id="new-pr" title="Une demande de fusion propose de réunir le travail d'une branche dans une autre, pour relecture avant d'intégrer.">＋ Demande de fusion</button>
      <button class="btn btn-ghost btn-sm" id="new-rel" title="Une version (release) marque un point stable, étiqueté (ex. v1.0), à publier.">＋ Version</button>
      <span class="act-help muted">Issues = notes de suivi · Fusions = réunir une branche après relecture · Versions = points stables publiés.</span>
    </div><div id="act"><div class="skeleton"></div></div>`;
  el("new-issue").onclick = () => newIssueModal(d);
  el("new-pr").onclick = () => newPullModal(d);
  el("new-rel").onclick = () => newReleaseModal(d);
  try {
    const [issues, pulls, releases] = await Promise.all([
      App().RepoIssues(state.instance, d.owner, d.repo, "open").catch(() => []),
      App().RepoPulls(state.instance, d.owner, d.repo, "open").catch(() => []),
      App().RepoReleases(state.instance, d.owner, d.repo).catch(() => []),
    ]);
    if (state.detail !== d) return;
    const list = (arr, fn) => arr.length ? arr.map(fn).join("") : `<p class="muted">Rien pour le moment.</p>`;
    const acts = (n, kind) => `<span class="srow-acts">
      <button class="btn btn-ghost btn-sm" data-act="comment" data-n="${n}">Commenter</button>
      ${kind === "issue" ? `<button class="btn btn-ghost btn-sm" data-act="close" data-n="${n}">Fermer</button>`
        : `<button class="btn btn-ghost btn-sm" data-act="review" data-n="${n}">Revue</button><button class="btn btn-ghost btn-sm" data-act="merge" data-n="${n}">Fusionner</button>`}</span>`;
    el("act").innerHTML = `
      <div class="blk"><div class="blk-h">Issues ouvertes (${issues.length})</div>
        ${list(issues, (i) => `<div class="srow"><div><b>#${i.number}</b> ${esc(i.title)} <span class="muted">${esc(i.user && i.user.login || "")}</span></div>${acts(i.number, "issue")}</div>`)}</div>
      <div class="blk"><div class="blk-h">Demandes de fusion ouvertes (${pulls.length})</div>
        ${list(pulls, (p) => `<div class="srow"><div><b>#${p.number}</b> ${esc(p.title)} <span class="muted">${esc(p.user && p.user.login || "")}</span></div>${acts(p.number, "pr")}</div>`)}</div>
      <div class="blk"><div class="blk-h">Versions publiées (${releases.length})</div>
        <div class="tags">${releases.length ? releases.map((r) => `<span class="tag-chip">${esc(r.tag_name)}</span>`).join("") : `<span class="muted">aucune</span>`}</div></div>`;
    el("act").onclick = (ev) => {
      const b = ev.target.closest("[data-act]"); if (!b) return;
      const n = +b.dataset.n;
      if (b.dataset.act === "comment") commentIssueModal(d, n);
      else if (b.dataset.act === "close") closeIssueConfirm(d, n);
      else if (b.dataset.act === "merge") mergePullModal(d, n);
      else if (b.dataset.act === "review") pullReviewModal(d, n);
    };
  } catch (e) { el("act").innerHTML = surfaceError("Activité indisponible", cleanErr(e), null); }
}

// commentIssueModal : commenter une issue/PR (+ invite IA + PIN).
function commentIssueModal(d, n) {
  openModal(`Commenter #${n}`, `<div class="field">
    <label>Commentaire<textarea id="ci-body" rows="3"></textarea></label>
    <div class="inline-error hidden" id="ci-err"></div>
    ${aiInviteHTML()}</div>`, async () => {
    const er = el("ci-err"); const b = el("ci-body").value.trim();
    if (!b) { er.textContent = "Écrivez un commentaire."; er.classList.remove("hidden"); return; }
    pinConfirm(`Commenter #${n}`, async (pin) => {
      await App().CommentIssue(state.instance, d.owner, d.repo, n, b, pin);
      closeModal(); toast("Commentaire ajouté", "ok"); paintActivite(el("detail-body"));
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => `${aiHeader("Commente une issue/PR")}\n\nCommande : situlab issue comment ${d.owner}/${d.repo} ${n} --body "<texte>"`);
}

// closeIssueConfirm : fermer une issue (PIN).
function closeIssueConfirm(d, n) {
  pinConfirm(`Fermer l'issue #${n}`, async (pin) => {
    await App().SetIssueState(state.instance, d.owner, d.repo, n, "closed", pin);
    toast("Issue fermée", "ok"); paintActivite(el("detail-body"));
  });
}

// mergePullModal : fusionner une demande de fusion (méthode + invite IA + PIN).
function mergePullModal(d, n) {
  openModal(`Fusionner #${n}`, `<div class="field">
    <label>Méthode<select id="mp-method" class="select"><option value="merge">Fusion (merge)</option><option value="squash">Squash</option><option value="rebase">Rebase</option></select></label>
    <div class="inline-error hidden" id="mp-err"></div>
    ${aiInviteHTML()}</div>`, async () => {
    pinConfirm(`Fusionner #${n}`, async (pin) => {
      await App().MergePull(state.instance, d.owner, d.repo, n, el("mp-method").value, pin);
      closeModal(); toast("Demande de fusion fusionnée", "ok"); paintActivite(el("detail-body"));
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => `${aiHeader("Fusionne une demande de fusion")}\n\nCommande : situlab pull-request merge ${d.owner}/${d.repo} ${n} --method merge`);
}

// newPullModal : ouvrir une demande de fusion (head → base) + invite IA + PIN.
function newPullModal(d) {
  openModal("Nouvelle demande de fusion", `<div class="field">
    <label>Titre<input id="pr-title" /></label>
    <label>Branche source (head)<input id="pr-head" placeholder="ex. dev" /></label>
    <label>Branche cible (base)<input id="pr-base" placeholder="${esc(d.ref || "main")}" /></label>
    <label>Description<textarea id="pr-body" rows="2"></textarea></label>
    <div class="inline-error hidden" id="pr-err"></div>
    ${aiInviteHTML()}</div>`, async () => {
    const er = el("pr-err");
    const t = el("pr-title").value.trim(), h = el("pr-head").value.trim(), ba = el("pr-base").value.trim() || (d.ref || "main"), bo = el("pr-body").value;
    if (!t || !h) { er.textContent = "Titre et branche source sont nécessaires."; er.classList.remove("hidden"); return; }
    pinConfirm(`Ouvrir la demande de fusion « ${t} »`, async (pin) => {
      await App().CreatePull(state.instance, d.owner, d.repo, t, h, ba, bo, pin);
      closeModal(); toast("Demande de fusion ouverte", "ok"); paintActivite(el("detail-body"));
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => promptPullCreate(d.owner, d.repo));
}
function promptPullCreate(owner, repo) {
  return [aiHeader("Ouvre une demande de fusion"), "",
    `Commande : situlab pull-request create ${owner}/${repo} --title "<titre>" --head "<branche source>" --base "<branche cible>" --body "<description>"`,
    "- Demande-moi les branches source/cible si tu ne les as pas.",
  ].join("\n");
}

// pullReviewModal : détail d'une PR + diff + boutons de revue (approuver / changements /
// commentaire) + fermer. Chaque action est confirmée par PIN.
async function pullReviewModal(d, n) {
  infoModal(`Demande de fusion #${n}`, `<div class="field"><div id="pr-detail"><div class="skeleton"></div></div></div>`);
  try {
    const [pi, diff] = await Promise.all([
      App().PullDetail(state.instance, d.owner, d.repo, n),
      App().PullDiff(state.instance, d.owner, d.repo, n).catch(() => ""),
    ]);
    const box = el("pr-detail"); if (!box) return;
    const review = (event, label, cls) => `<button class="btn ${cls} btn-sm" data-ev="${event}">${label}</button>`;
    box.innerHTML = `
      <div class="drow"><b>${esc(pi.title)}</b> <span class="pill ${pi.state === "open" ? "ver" : "muted"}">${esc(pi.state)}</span></div>
      <p class="sub">${esc(pi.head && pi.head.ref || "")} → ${esc(pi.base && pi.base.ref || "")} · par ${esc(pi.user && pi.user.login || "")}${pi.merged ? " · déjà fusionnée" : (pi.mergeable ? " · fusionnable" : " · conflits")}</p>
      <label>Commentaire de revue (optionnel)<textarea id="pr-rbody" rows="2"></textarea></label>
      <div class="srow-acts" style="margin:8px 0">
        ${review("APPROVED", "Approuver", "btn-primary")}
        ${review("REQUEST_CHANGES", "Demander des changements", "btn-ghost")}
        ${review("COMMENT", "Commentaire", "btn-ghost")}
        <button class="btn btn-ghost btn-sm btn-danger" id="pr-close">Fermer la PR</button>
      </div>
      <h4 class="sub-h">Diff</h4>
      <pre class="license-text">${esc(diff || "(diff indisponible ou vide)")}</pre>`;
    box.querySelectorAll("[data-ev]").forEach((b) => {
      b.onclick = () => pinConfirm(`Revue (${b.dataset.ev}) sur #${n}`, async (pin) => {
        await App().ReviewPull(state.instance, d.owner, d.repo, n, b.dataset.ev, el("pr-rbody").value, pin);
        closeModal(); toast("Revue déposée", "ok"); paintActivite(el("detail-body"));
      });
    });
    const cb = el("pr-close"); if (cb) cb.onclick = () => pinConfirm(`Fermer la PR #${n}`, async (pin) => {
      await App().SetPullState(state.instance, d.owner, d.repo, n, "closed", pin);
      closeModal(); toast("PR fermée", "ok"); paintActivite(el("detail-body"));
    });
  } catch (e) { const box = el("pr-detail"); if (box) box.innerHTML = `<div class="muted">${esc(cleanErr(e))}</div>`; }
}

// newReleaseModal : publier une version (tag) + invite IA + PIN.
function newReleaseModal(d) {
  openModal("Publier une version", `<div class="field">
    <label>Tag<input id="rl-tag" placeholder="ex. v1.2.0" /></label>
    <label>Titre<input id="rl-name" /></label>
    <label>Notes<textarea id="rl-body" rows="2"></textarea></label>
    <label>Branche/commit cible <span class="muted">(défaut : branche par défaut)</span><input id="rl-target" placeholder="${esc(d.ref || "")}" /></label>
    <label style="flex-direction:row;align-items:center"><input id="rl-pre" type="checkbox" style="width:auto;margin-right:8px" />Pré-version</label>
    <div class="inline-error hidden" id="rl-err"></div>
    ${aiInviteHTML()}</div>`, async () => {
    const er = el("rl-err"); const tag = el("rl-tag").value.trim();
    if (!tag) { er.textContent = "Un tag est nécessaire (ex. v1.2.0)."; er.classList.remove("hidden"); return; }
    pinConfirm(`Publier la version « ${tag} »`, async (pin) => {
      await App().CreateRelease(state.instance, d.owner, d.repo, tag, el("rl-name").value, el("rl-body").value, el("rl-target").value.trim(), false, el("rl-pre").checked, pin);
      closeModal(); toast("Version publiée", "ok"); paintActivite(el("detail-body"));
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => `${aiHeader("Publie une version")}\n\nCommande : situlab release create ${d.owner}/${d.repo} "<tag>" --name "<titre>" --body "<notes>"`);
}

// gitLocalModal : opérations git LOCALES sur le clone (branches, annuler, stash). Pas de PIN
// (aucun accès distant) ; « Annuler les modifications » est destructif → confirmation explicite.
function gitLocalModal(d) {
  const path = d.local_path;
  infoModal("Git local — " + esc(shortPath(path)), `<div class="field">
    <label>Créer une branche et basculer dessus<div style="display:flex;gap:8px"><input id="gl-new" placeholder="ex. feature-x" /><button class="btn btn-ghost btn-sm" type="button" id="gl-new-btn">Créer</button></div></label>
    <label>Changer de branche<div style="display:flex;gap:8px"><input id="gl-sw" placeholder="ex. main" /><button class="btn btn-ghost btn-sm" type="button" id="gl-sw-btn">Basculer</button></div></label>
    <div class="srow-acts" style="margin-top:4px">
      <button class="btn btn-ghost btn-sm" type="button" id="gl-stash">Mettre de côté</button>
      <button class="btn btn-ghost btn-sm" type="button" id="gl-pop">Réappliquer</button>
      <button class="btn btn-ghost btn-sm btn-danger" type="button" id="gl-discard">Annuler mes modifs</button>
    </div>
    <div id="gl-out" class="muted" style="font-size:12px;white-space:pre-wrap;margin-top:4px"></div></div>`);
  const out = () => el("gl-out");
  const run = async (fn, okMsg) => { try { await fn(); out().textContent = okMsg; state.repoSig = ""; } catch (e) { if (isLocked(e)) return handleLock(); out().textContent = cleanErr(e); } };
  el("gl-new-btn").onclick = () => { const n = el("gl-new").value.trim(); if (n) run(() => App().GitCreateBranch(path, n), "Branche « " + n + " » créée."); };
  el("gl-sw-btn").onclick = () => { const n = el("gl-sw").value.trim(); if (n) run(() => App().GitSwitchBranch(path, n), "Sur la branche « " + n + " »."); };
  el("gl-stash").onclick = () => run(() => App().GitStashSave(path, ""), "Modifications mises de côté.");
  el("gl-pop").onclick = () => run(() => App().GitStashPop(path), "Remise de côté réappliquée.");
  el("gl-discard").onclick = () => {
    openModal("Annuler mes modifications ?", `<div class="field"><div class="callout warn"><b>Destructif.</b> Les modifications non enregistrées de ce dépôt seront perdues (l'historique et les commits ne sont pas touchés).</div></div>`, async () => {
      try { await App().GitDiscard(path, ""); closeModal(); toast("Modifications annulées", "ok"); state.repoSig = ""; } catch (e) { closeModal(); toast(cleanErr(e), "err"); }
    });
    el("modal-ok").textContent = "Annuler mes modifs"; el("modal-ok").classList.add("danger-btn");
  };
}

// repoSettingsModal : réglages d'un dépôt (renommer, visibilité, archiver, branche défaut).
function repoSettingsModal(d) {
  openModal(`Réglages — ${esc(d.full_name || d.repo)}`, `<div class="field">
    <label>Renommer le dépôt<div style="display:flex;gap:8px"><input id="rs-name" placeholder="${esc(d.repo)}" /><button class="btn btn-ghost btn-sm" type="button" id="rs-rename-btn">Renommer</button></div></label>
    <label>Branche par défaut<div style="display:flex;gap:8px"><input id="rs-branch" placeholder="${esc(d.ref || "main")}" /><button class="btn btn-ghost btn-sm" type="button" id="rs-branch-btn">Appliquer</button></div></label>
    <div class="srow-acts" style="margin-top:6px">
      <button class="btn btn-ghost btn-sm" type="button" id="rs-private">Rendre privé</button>
      <button class="btn btn-ghost btn-sm" type="button" id="rs-public">Rendre public</button>
      <button class="btn btn-ghost btn-sm" type="button" id="rs-archive">Archiver</button>
      <button class="btn btn-ghost btn-sm" type="button" id="rs-unarchive">Désarchiver</button>
    </div>
    <small>Chaque action est confirmée par votre PIN.</small></div>`, () => closeModal());
  el("modal-ok").textContent = "Fermer"; el("modal-cancel").classList.add("hidden");
  const after = (msg) => { toast(msg, "ok"); closeModal(); state.repoSig = ""; };
  el("rs-rename-btn").onclick = () => { const v = el("rs-name").value.trim(); if (!v) return; pinConfirm(`Renommer en « ${v} »`, async (pin) => { await App().EditRepoSettings(state.instance, d.owner, d.repo, { name: v }, pin); after("Dépôt renommé"); navTo("repos"); }); };
  el("rs-branch-btn").onclick = () => { const v = el("rs-branch").value.trim(); if (!v) return; pinConfirm(`Branche par défaut → ${v}`, async (pin) => { await App().EditRepoSettings(state.instance, d.owner, d.repo, { default_branch: v }, pin); after("Branche par défaut mise à jour"); }); };
  el("rs-private").onclick = () => pinConfirm("Rendre le dépôt privé", async (pin) => { await App().EditRepoSettings(state.instance, d.owner, d.repo, { private: true }, pin); after("Dépôt privé"); });
  el("rs-public").onclick = () => pinConfirm("Rendre le dépôt public", async (pin) => { await App().EditRepoSettings(state.instance, d.owner, d.repo, { private: false }, pin); after("Dépôt public"); });
  el("rs-archive").onclick = () => pinConfirm("Archiver le dépôt", async (pin) => { await App().EditRepoSettings(state.instance, d.owner, d.repo, { archived: true }, pin); after("Dépôt archivé"); });
  el("rs-unarchive").onclick = () => pinConfirm("Désarchiver le dépôt", async (pin) => { await App().EditRepoSettings(state.instance, d.owner, d.repo, { archived: false }, pin); after("Dépôt désarchivé"); });
}

function newIssueModal(d) {
  openModal("Nouvelle issue", `<div class="field">
    <label>Titre<input id="it" /></label>
    <label>Description<input id="ib" /></label>
    <div class="inline-error hidden" id="i-error"></div>
    ${aiInviteHTML()}</div>`, async () => {
    const er = el("i-error");
    const it = el("it").value.trim(), ib = el("ib").value;
    if (!it) { er.textContent = "Donnez un titre."; er.classList.remove("hidden"); return; }
    pinConfirm(`Créer l'issue « ${it} »`, async (pin) => {
      await App().CreateIssue(state.instance, d.owner, d.repo, it, ib, pin);
      closeModal(); toast("Issue créée", "ok"); paintActivite(el("detail-body"));
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => promptIssueCreate(d.owner, d.repo));
}

function paintApercu(body, liveOnly) {
  const d = state.detail;
  const rem = d._remote || {};
  const last = (rem.commits && rem.commits[0]) || null;
  const loc = d._local;

  // Anti-scintillement : ne réécrit que si le contenu a changé (préserve la sélection).
  const sig = "apercu|" + JSON.stringify([d.ref, d.tags, last, loc]);
  if (liveOnly && sig === d._paintSig) return;
  d._paintSig = sig;

  // « À envoyer » : versions enregistrées non envoyées — message seul, sans n° de commit (v1.5.0).
  const unpushed = loc && loc.is_repo ? (loc.unpushed || []).map((c) => `<li>${esc(c.message)}</li>`).join("") : "";
  const changed = loc && loc.is_repo ? (loc.changed || []).slice(0, 16).map((f) => `<li>${esc(f)}</li>`).join("") : "";
  const lists = (unpushed || changed) ? `<div class="panels">
    ${unpushed ? `<div class="panel"><div class="panel-h">À envoyer en ligne (${loc.unpushed.length})</div><ul class="cmts">${unpushed}</ul></div>` : ""}
    ${changed ? `<div class="panel"><div class="panel-h">Modifications en cours</div><ul class="files">${changed}</ul></div>` : ""}
  </div>` : "";

  body.innerHTML = syncHero(d, loc) + versionScale(d, loc) +
    (d.local_path ? `<div class="detail-branches"><div class="vscale-h">Branches (en ligne ⟷ ce poste)</div><div id="detail-bg"><div class="skeleton"></div></div></div>` : "") +
    lists;

  // Graphe de branches visuel (Lot 2) : chargé depuis le clone local (sans réseau).
  if (d.local_path) {
    App().BranchMatrix(d.local_path)
      .then((m) => { const t = el("detail-bg"); if (t) t.innerHTML = branchMatrixHTML(m); })
      .catch(() => { const t = el("detail-bg"); if (t) t.innerHTML = `<div class="muted">Branches indisponibles.</div>`; });
  }

  // Actions (délégation sur la zone des boutons).
  const bar = body.querySelector(".sync-actions");
  if (bar) bar.onclick = (ev) => { const b = ev.target.closest("[data-act]"); if (b) repoAct(b.dataset.act); };
}

// Bandeau de synchro : EN LIGNE ⟷ CE POSTE, avec l'état au centre et les actions.
function syncHero(d, loc) {
  const hasLocal = loc && loc.is_repo;
  const ahead = hasLocal ? (loc.ahead || 0) : 0;
  const behind = hasLocal ? (loc.behind || 0) : 0;
  const dirty = hasLocal ? !loc.clean : false;
  const tag = (d.tags && d.tags[0]) || "";

  let center = "", acts = [];
  // tip = terme git conservé en infobulle pour qui le connaît (lexique v1.3).
  const btn = (a, l, k, tip) => `<button class="btn btn-${k} btn-sm" data-act="${a}"${tip ? ` title="${esc(tip)}"` : ""}>${esc(l)}</button>`;
  if (!hasLocal) {
    center = `<div class="sync-msg s-nolocal">○ Pas encore copié sur votre poste</div>`;
    if (d.has_remote) acts.push(btn("clone", "Copier sur mon poste", "primary", "git clone"));
  } else {
    const bits = [];
    if (behind) bits.push(`<span class="s-behind">↓ ${behind} à récupérer</span>`);
    if (ahead) bits.push(`<span class="s-ahead">↑ ${ahead} à envoyer</span>`);
    if (dirty) bits.push(`<span class="s-dirty">● modifications non enregistrées</span>`);
    center = bits.length ? `<div class="sync-msg">${bits.join(" · ")}</div>` : `<div class="sync-msg s-uptodate">✓ Tout est synchronisé</div>`;
    const needsSend = ahead || dirty;      // quelque chose à envoyer → Synchroniser (PIN)
    const recvOnly = behind && !needsSend; // seulement des nouveautés à recevoir → Récupérer (sans PIN)
    // En lecture seule (v1.4), aucune action modifiant l'arbre : seulement consulter.
    if (!state.readOnly) {
      // Cas courant : un seul geste. Recevoir seul ne demande pas le PIN (aucun envoi).
      if (recvOnly) acts.push(btn("pull", "⟳ Récupérer", "primary", "git pull --ff-only"));
      else if (needsSend) acts.push(btn("sync", "⟳ Synchroniser", "primary", "Enregistre, récupère et envoie en une fois (git commit + pull + push)"));
    }
    if (dirty) acts.push(btn("diff", "Voir les changements", "ghost", "git diff"));
    // « Vérifier le serveur » (fetch) est une pure lecture : toujours proposé en lecture seule
    // (où les actions de mutation sont masquées), sinon seulement quand il n'y a rien d'autre à faire.
    if (state.readOnly || (!behind && !needsSend)) acts.push(btn("fetch", "Vérifier le serveur", "ghost", "git fetch"));
    // Mode avancé : étapes séparées pour qui connaît git (jamais en lecture seule).
    if (state.advanced && !state.readOnly) {
      if (behind && needsSend) acts.push(btn("pull", "Récupérer", "ghost", "git pull --ff-only"));
      if (dirty) acts.push(btn("commit", "Enregistrer une version", "ghost", "git commit"));
      if (ahead) acts.push(btn("push", "Envoyer en ligne", "ghost", "git push"));
    }
  }
  const arrows = `<div class="sync-arrows">${behind ? `<span class="s-behind">↓</span>` : ""}${ahead ? `<span class="s-ahead">↑</span>` : ""}${!ahead && !behind && hasLocal ? `<span class="s-uptodate">=</span>` : ""}</div>`;
  return `<div class="sync">
    <div class="sync-side"><div class="sync-h">En ligne</div><div class="sync-big">⎇ ${esc(d.ref || "?")}</div><div class="sync-sub">${tag ? "version " + esc(tag) : "—"}</div></div>
    ${arrows}
    <div class="sync-mid">${center}<div class="sync-actions">${acts.join("")}</div></div>
    ${arrows}
    <div class="sync-side right"><div class="sync-h">Ce poste</div><div class="sync-big">${hasLocal ? "⎇ " + esc(loc.branch || "?") : "—"}</div><div class="sync-sub">${hasLocal ? esc(loc.describe || "") : "non copié"}</div></div>
  </div>`;
}

// Frise des versions « à l'échelle » : les tags en jalons, la position locale repérée.
function versionScale(d, loc) {
  const tags = (d.tags || []).slice(0, 12).reverse(); // du plus ancien (gauche) au plus récent (droite)
  if (tags.length === 0) return `<div class="vscale"><div class="vcap muted">Aucune version (tag) publiée sur ce dépôt.</div></div>`;
  let baseTag = null, offset = 0;
  if (loc && loc.describe) {
    const m = loc.describe.match(/^(.*)-(\d+)-g[0-9a-f]+$/);
    if (m && tags.includes(m[1])) { baseTag = m[1]; offset = +m[2]; }
    else if (tags.includes(loc.describe)) { baseTag = loc.describe; }
  }
  const baseIdx = baseTag ? tags.indexOf(baseTag) : -1;
  const ticks = tags.map((t, i) => {
    const here = i === baseIdx, latest = i === tags.length - 1;
    return `<div class="vtick${here ? " here" : ""}${latest ? " latest" : ""}">
      ${here ? `<div class="vyou">vous${offset ? " +" + offset : ""}</div>` : ""}
      <div class="vdot"></div><div class="vlab">${esc(t)}</div></div>`;
  }).join("");
  const cap = baseTag
    ? `Votre poste est basé sur <b>${esc(baseTag)}</b>${offset ? ` + ${offset} commit(s)` : ""}. Dernière version en ligne : <b>${esc(tags[tags.length - 1])}</b>.`
    : `Dernière version en ligne : <b>${esc(tags[tags.length - 1])}</b>.`;
  return `<div class="vscale"><div class="vscale-h">Versions</div><div class="vtrack"><div class="vline"></div>${ticks}</div><div class="vcap">${cap}</div></div>`;
}

/* ---------------- Actions dépôt (vague 1) ---------------- */
async function repoAct(action) {
  const d = state.detail; if (!d) return;
  try {
    if (action === "fetch") { toast("Vérification du serveur…"); await App().Fetch(state.instance, d.local_path); toast("Serveur vérifié", "ok"); }
    else if (action === "pull") { toast("Récupération…"); await App().Pull(state.instance, d.local_path); toast("Nouveautés récupérées", "ok"); }
    else if (action === "sync") return syncWithPin(d);
    else if (action === "push") return pushWithPin(d);
    else if (action === "commit") return commitModal(d);
    else if (action === "diff") return diffView(d);
    else if (action === "clone") return cloneModal(d);
    await refreshDetailLocal(); await refreshDetailRemote(); paintDetail();
  } catch (e) { toast(cleanErr(e), "err"); }
}

function pushWithPin(d) {
  let ci;
  openModal("Envoyer en ligne", `<div class="field">
    <p class="sub">Les points de sauvegarde locaux vont être envoyés sur le serveur. Confirmez avec votre PIN.</p>
    <label>PIN</label><div id="pp"></div>
    <div class="inline-error hidden" id="pp-error"></div></div>`, async () => {
    const er = el("pp-error");
    try {
      await App().Push(state.instance, d.local_path, ci.read());
      closeModal(); toast("Envoyé en ligne", "ok");
      await refreshDetailLocal(); await refreshDetailRemote(); paintDetail();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); ci.clear(); }
  });
  ci = codeInput(el("pp"), true, () => modalOk(), true); // 1 seul champ → validation auto
}

// syncWithPin : le geste unique « Synchroniser » (récupérer → enregistrer → envoyer), confirmé
// par le PIN. Les cas délicats (changements croisés, ancienne version) remontent une phrase
// claire de ce qu'il faut faire — jamais une sortie git brute.
function syncWithPin(d) {
  let ci;
  const loc = d._local || {};
  const nChanged = (loc.changed || []).length;
  // Consentement éclairé : Synchroniser enregistre et publie TOUTES les modifications en cours
  // (fichiers nouveaux compris). On annonce le nombre concerné pour éviter une publication surprise.
  const notice = nChanged
    ? `<div class="callout">${nChanged} fichier${nChanged > 1 ? "s" : ""} en cours seront enregistrés puis envoyés en ligne.</div>`
    : "";
  openModal("Synchroniser", `<div class="field">
    <p class="sub">SituLab va <b>récupérer</b> les nouveautés du serveur, <b>enregistrer</b> vos modifications en cours, puis <b>envoyer</b> le tout. Un seul geste. Confirmez avec votre PIN.</p>
    ${notice}
    <label>PIN</label><div id="sp"></div>
    <div class="inline-error hidden" id="sp-error"></div></div>`, async () => {
    const er = el("sp-error");
    try {
      const res = await App().Synchronize(state.instance, d.local_path, "", ci.read());
      closeModal(); toast(res && res.summary ? res.summary : "Synchronisé", "ok");
      await refreshDetailLocal(); await refreshDetailRemote(); paintDetail();
    } catch (e) {
      er.textContent = cleanErr(e); er.classList.remove("hidden"); ci.clear();
      // Une version a pu être enregistrée avant l'erreur (ex. envoi refusé) : rafraîchir l'état réel.
      await refreshDetailLocal(); paintDetail();
    }
  });
  ci = codeInput(el("sp"), true, () => modalOk(), true);
}

function commitModal(d) {
  openModal("Enregistrer une version", `<div class="field">
    <p class="sub">Toutes vos modifications seront enregistrées localement (rien n'est envoyé en ligne).</p>
    <label>Description<input id="cm" placeholder="ex. mise à jour de l'accueil" /></label>
    <div class="inline-error hidden" id="cm-error"></div></div>`, async () => {
    const er = el("cm-error");
    try { await App().Commit(d.local_path, el("cm").value); closeModal(); toast("Version enregistrée", "ok"); await refreshDetailLocal(); paintDetail(); }
    catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
}

async function diffView(d) {
  const body = el("detail-body");
  d._diffOpen = true; // empêche le rafraîchissement live de réécrire l'Aperçu par-dessus le diff
  body.innerHTML = `<div class="file-head"><button class="btn-back" id="dback">‹ Aperçu</button><span class="file-name">Modifications en cours</span></div><div class="skeleton"></div>`;
  el("dback").onclick = () => { d._diffOpen = false; d._paintSig = null; paintDetail(); };
  try {
    const diff = await App().Diff(d.local_path);
    body.querySelector(".skeleton").outerHTML = diff.trim()
      ? `<pre class="filebox diff">${colorDiff(diff)}</pre>`
      : blankslate("✓", "Rien à afficher", "Aucune modification non enregistrée.");
  } catch (e) { body.innerHTML = surfaceError("Diff impossible", cleanErr(e), null); }
}

// Colorise un diff unifié (déjà échappé) : + vert, - rouge, @@ bleu.
function colorDiff(diff) {
  return esc(diff).split("\n").map((l) => {
    if (l.startsWith("+") && !l.startsWith("+++")) return `<span class="d-add">${l}</span>`;
    if (l.startsWith("-") && !l.startsWith("---")) return `<span class="d-del">${l}</span>`;
    if (l.startsWith("@@")) return `<span class="d-hunk">${l}</span>`;
    return l;
  }).join("\n");
}

function cloneModal(d) {
  openModal("Copier sur mon poste", `<div class="field">
    <p class="sub">Choisissez un dossier de destination (il sera créé).</p>
    <label>Dossier<input id="cl" placeholder="C:\\work\\${esc(d.repo || "")}" /></label>
    <div class="inline-error hidden" id="cl-error"></div></div>`, async () => {
    const er = el("cl-error");
    try {
      toast("Copie en cours…");
      await App().Clone(state.instance, d.owner, d.repo, el("cl").value);
      closeModal(); toast("Dépôt copié sur votre poste", "ok");
      state.detail.local_path = el("cl").value;
      await refreshDetailLocal(); await refreshDetailRemote(); paintDetail();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
}

async function paintFiles(body, liveOnly) {
  if (liveOnly) return;
  const d = state.detail;
  body.innerHTML = `<div class="crumbs" id="crumbs"></div><div id="files"><div class="skeleton"></div></div>`;
  paintCrumbs();
  try {
    const entries = await App().RepoContents(state.instance, d.owner, d.repo, d.path, d.ref);
    if (state.detail !== d) return;
    const sorted = (entries || []).sort((a, b) => (a.type === b.type) ? a.name.localeCompare(b.name) : (a.type === "dir" ? -1 : 1));
    const box = el("files");
    box.innerHTML = sorted.length ? sorted.map((e, i) => `
      <div class="frow" data-i="${i}" role="button" tabindex="0">
        <span class="fico" aria-hidden="true">${e.type === "dir" ? "📁" : "📄"}</span>
        <span class="fname">${esc(e.name)}</span>
        <span class="fsize">${e.type === "file" ? fmtSize(e.size) : ""}</span>
      </div>`).join("") : blankslate("∅", "Dossier vide", "");
    box.onclick = (ev) => {
      const row = ev.target.closest(".frow"); if (!row) return;
      const e = sorted[+row.dataset.i];
      if (e.type === "dir") { d.path = e.path; paintFiles(body); } else openFile(e);
    };
  } catch (e) { el("files").innerHTML = surfaceError("Lecture impossible", cleanErr(e), null); }
}

function paintCrumbs() {
  const d = state.detail, c = el("crumbs"); if (!c) return;
  const parts = d.path ? d.path.split("/") : [];
  let acc = "";
  const segs = [`<a data-p="">${esc(d.repo)}</a>`];
  parts.forEach((p) => { acc = acc ? acc + "/" + p : p; segs.push(`<a data-p="${esc(acc)}">${esc(p)}</a>`); });
  c.innerHTML = segs.join(`<span class="sep">/</span>`);
  c.onclick = (ev) => { const a = ev.target.closest("a"); if (a) { d.path = a.dataset.p; paintFiles(el("detail-body")); } };
}

// mdToHTML : rendu Markdown MINIMAL et SÛR (échappement d'abord), zéro dépendance. Couvre titres, code
// (inline + bloc), gras/italique, listes à puces, liens http(s) et paragraphes — suffisant pour un README.
function mdToHTML(src) {
  const esc2 = (s) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");
  const inline = (s) => esc2(s)
    .replace(/`([^`]+)`/g, (_, c) => "<code>" + c + "</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>")
    .replace(/\*([^*]+)\*/g, "<i>$1</i>")
    .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, (_, t, u) => `<a href="${u}" data-ext>${t}</a>`);
  const lines = String(src).replace(/\r\n/g, "\n").split("\n");
  const out = []; let inCode = false, inList = false; const buf = [];
  const flushP = () => { if (buf.length) { out.push("<p>" + inline(buf.join(" ")) + "</p>"); buf.length = 0; } };
  const closeList = () => { if (inList) { out.push("</ul>"); inList = false; } };
  for (const ln of lines) {
    if (/^```/.test(ln)) { if (inCode) { out.push("</code></pre>"); inCode = false; } else { flushP(); closeList(); out.push('<pre class="md-code"><code>'); inCode = true; } continue; }
    if (inCode) { out.push(esc2(ln) + "\n"); continue; }
    const h = ln.match(/^(#{1,6})\s+(.*)$/);
    if (h) { flushP(); closeList(); const lvl = Math.min(6, h[1].length) + 2; out.push(`<h${lvl}>` + inline(h[2]) + `</h${lvl}>`); continue; }
    const li = ln.match(/^\s*[-*]\s+(.*)$/);
    if (li) { flushP(); if (!inList) { out.push("<ul>"); inList = true; } out.push("<li>" + inline(li[1]) + "</li>"); continue; }
    if (ln.trim() === "") { flushP(); closeList(); continue; }
    buf.push(ln);
  }
  if (inCode) out.push("</code></pre>");
  flushP(); closeList();
  return out.join("\n");
}

// downloadText : propose le téléchargement d'un contenu texte via un lien blob (best-effort WebView2).
function downloadText(name, text) {
  try {
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: "application/octet-stream" }));
    a.download = name || "fichier.txt"; document.body.appendChild(a); a.click();
    setTimeout(() => { try { URL.revokeObjectURL(a.href); } catch (_) {} a.remove(); }, 1000);
  } catch (_) { toast("Téléchargement impossible sur ce poste", "err"); }
}

// openFile : consultation d'un fichier en plusieurs MODES (1.9.2). Onglet « Interprété » (par défaut pour
// .md et .html, mise en forme), onglet « Texte brut » (affichage d'origine), plus « Ouvrir dans le
// navigateur » et « Télécharger ». Le HTML est rendu dans une iframe SANDBOXÉE (aucun script exécuté).
async function openFile(e) {
  const d = state.detail, body = el("detail-body");
  body.innerHTML = `<div class="crumbs" id="crumbs"></div><div class="skeleton"></div>`;
  paintCrumbs();
  let content;
  try { content = await App().RepoFile(state.instance, d.owner, d.repo, e.path, d.ref); }
  catch (err) { body.innerHTML = surfaceError("Fichier illisible", cleanErr(err), null); return; }
  if (state.detail !== d) return;
  const ext = (e.name.split(".").pop() || "").toLowerCase();
  const isMd = ext === "md" || ext === "markdown";
  const isHtml = ext === "html" || ext === "htm";
  const canInterpret = isMd || isHtml;
  const openBtn = e.html_url ? `<button class="btn btn-ghost btn-sm" id="f-open">Ouvrir dans le navigateur ↗</button>` : "";
  const hasLocal = !!d.local_path; // l'historique par fichier se lit sur le clone local
  const histoTab = hasLocal ? `<button class="ftab" data-ft="histo">Historique</button>` : "";
  const tabs = `<div class="file-tabs">${canInterpret ? `<button class="ftab active" data-ft="rendu">Interprété</button>` : ""}<button class="ftab${canInterpret ? "" : " active"}" data-ft="brut">Texte brut</button>${histoTab}</div>`;
  body.innerHTML = `<div class="file-head"><button class="btn-back" id="fback">‹ ${esc(parentOf(e.path) || d.repo)}</button><span class="file-name">${esc(e.name)}</span><span class="file-actions">${openBtn}<button class="btn btn-ghost btn-sm" id="f-dl">Télécharger</button></span></div>${tabs}<div id="fview"></div>`;
  const fv = el("fview");
  const showRaw = () => { fv.innerHTML = `<pre class="filebox">${esc(content)}</pre>`; };
  const showRendered = () => {
    if (isMd) { fv.innerHTML = `<div class="md-render">${mdToHTML(content)}</div>`; }
    else if (isHtml) { fv.innerHTML = `<iframe class="html-render" sandbox title="aperçu"></iframe>`; const f = fv.querySelector("iframe"); if (f) f.srcdoc = content; }
    fv.querySelectorAll("a[data-ext]").forEach((a) => a.onclick = (ev) => { ev.preventDefault(); safeOpenURL(a.getAttribute("href")); });
  };
  // Historique du FICHIER : qui a changé quoi/quand, diff coloré du commit choisi (clone local).
  const showHisto = async () => {
    fv.innerHTML = `<div class="skeleton"></div>`;
    let rows = [];
    try { rows = (await App().FileHistory(d.local_path, e.path, 50)) || []; } catch (err) { fv.innerHTML = `<div class="callout warn">${esc(cleanErr(err))}</div>`; return; }
    if (!rows.length) { fv.innerHTML = blankslate("∅", "Aucun historique pour ce fichier", ""); return; }
    fv.innerHTML = `<div class="fh-list">${rows.map((c) => `<button type="button" class="fh-row" data-sha="${esc(c.sha)}"><code>${esc(c.short)}</code><span class="fh-msg">${esc(c.message)}</span><span class="fh-meta">${esc(c.author)} · ${esc(fmtDate(c.date))}</span></button>`).join("")}</div><div class="fh-diff" id="fh-diff"></div>`;
    const showDiff = async (sha) => {
      const dd = el("fh-diff"); if (!dd) return;
      dd.innerHTML = `<div class="skeleton"></div>`;
      try { const diff = await App().FileDiffAt(d.local_path, sha, e.path); dd.innerHTML = `<pre class="filebox diff">${colorDiff(diff || "(aucune modification)")}</pre>`; }
      catch (err) { dd.innerHTML = `<div class="callout warn">${esc(cleanErr(err))}</div>`; }
    };
    fv.querySelectorAll(".fh-row").forEach((r) => r.onclick = () => {
      fv.querySelectorAll(".fh-row").forEach((x) => x.classList.remove("on")); r.classList.add("on");
      showDiff(r.dataset.sha);
    });
    const first = fv.querySelector(".fh-row"); if (first) first.click();
  };
  const views = { rendu: showRendered, brut: showRaw, histo: showHisto };
  (canInterpret ? showRendered : showRaw)();
  el("fback").onclick = () => { d.path = parentOf(e.path); paintFiles(body); };
  if (el("f-open")) el("f-open").onclick = () => safeOpenURL(e.html_url);
  el("f-dl").onclick = () => downloadText(e.name, content);
  body.querySelectorAll(".ftab").forEach((t) => t.onclick = () => {
    body.querySelectorAll(".ftab").forEach((x) => x.classList.remove("active")); t.classList.add("active");
    (views[t.dataset.ft] || showRaw)();
  });
}

// Sceau « certified » SituLab (garantie d'origine) : bouclier + coche, stroke --ok.
const SEAL_SVG = `<svg class="seal" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2.6l7 2.3v5.3c0 4.6-3 7.9-7 9.2-4-1.3-7-4.6-7-9.2V4.9z"/><path d="M8.6 12l2.4 2.4 4.5-4.9"/></svg>`;

// stripAttestTrailer retire le trailer « SituLab-Approved: » du message affiché : il est représenté
// par le sceau, jamais montré en base64 brut.
function stripAttestTrailer(msg) {
  return String(msg || "").split("\n").filter((l) => !/^SituLab-Approved:\s/.test(l)).join("\n").replace(/\n+$/, "");
}

function paintHisto(body, liveOnly) {
  const d = state.detail, rem = d._remote || { commits: [] };
  const tags = (d.tags || []).map((t) => `<span class="tag-chip">${esc(t)}</span>`).join("") || `<span class="muted">aucune version</span>`;
  const commits = (rem.commits || []).map((c) => {
    const full = stripAttestTrailer(c.message), nl = full.indexOf("\n");
    const subject = nl < 0 ? full : full.slice(0, nl);
    const bodyTxt = nl < 0 ? "" : full.slice(nl + 1).trim();
    const sha = esc(c.sha || "");
    return `<div class="cmt" data-sha="${sha}">
      <div class="cmt-head"><span class="cmt-subject">${esc(subject)}</span><span class="att-badge" data-att="${sha}"></span></div>
      <div class="cmt-meta"><code>${esc(c.short)}</code><span class="cmt-who">${esc(c.author)}</span><span class="cmt-when">${esc(fmtDate(c.date))}</span></div>
      ${bodyTxt ? `<button class="cmt-more" data-more="${sha}">Détails ▾</button><div class="cmt-body hidden" id="cb-${sha}">${esc(bodyTxt)}</div>` : ""}
    </div>`;
  }).join("") || blankslate("∅", "Aucun commit", "");
  const sig = "histo|" + JSON.stringify([d.ref, d.tags || [], rem.commits || [], d.local_path || ""]);
  if (liveOnly && sig === d._paintSig) return;
  d._paintSig = sig;
  body.innerHTML = `<div id="att-banner"></div>
    <div class="blk"><div class="blk-h">Versions</div><div class="tags">${tags}</div></div>
    <div class="blk"><div class="blk-h">Historique — ${esc(d.ref)}</div><div class="cmt-timeline">${commits}</div></div>`;
  // Déplier/replier le corps d'un commit (délégation sur la timeline).
  const tl = body.querySelector(".cmt-timeline");
  if (tl) tl.onclick = (ev) => {
    const btn = ev.target.closest(".cmt-more"); if (!btn) return;
    const cb = el("cb-" + btn.dataset.more); if (!cb) return;
    const hidden = cb.classList.toggle("hidden");
    btn.textContent = hidden ? "Détails ▾" : "Masquer ▴";
  };
  // Enrichissement « SituLab approved » (sceau d'attestation locale) — async, best-effort, hors rendu.
  enrichAttestation(d);
}

// enrichAttestation appose le SCEAU « certified » sur les commits vérifiés (par SHA) et affiche un
// bandeau tant que la racine embarquée est la racine de TEST. Lecture locale (clone requis).
async function enrichAttestation(d) {
  try {
    if (await App().AttestUsingTestRoot()) {
      const b = el("att-banner");
      if (b) b.innerHTML = `<div class="callout warn" style="margin-bottom:10px">Signature en mode <b>TEST</b> — la racine de gouvernance n'est pas encore installée : les sceaux ne sont pas vérifiables réellement.</div>`;
    }
  } catch (_) {}
  if (!d.local_path) return;
  let rows = [];
  try { rows = (await App().AttestHistory(d.local_path, 100)) || []; } catch (_) { return; }
  if (state.detail !== d) return;
  const map = {};
  rows.forEach((r) => { if (r && r.sha) map[r.sha] = r.attestation; });
  document.querySelectorAll(".cmt[data-sha]").forEach((row) => {
    const v = map[row.dataset.sha], badge = row.querySelector(".att-badge");
    if (v && v.verified && badge) {
      badge.className = "att-badge ok";
      badge.innerHTML = SEAL_SVG;
      badge.title = "Signé via SituLab — garantie d'origine" + (v.subject ? " · " + v.subject : "");
      row.classList.add("signed");
    }
  });
}

/* ---------------- Vue Projets = Tableau de bord 3 colonnes (v1.8) ---------------- */
// isPinNeeded : l'erreur indique que la politique de la connexion exige le PIN pour cette action.
function isPinNeeded(e) { return /PIN requis|PIN demand/i.test(cleanErr(e)); }

// Icônes des paliers. LOCAL = ce poste (écran) ; ATELIER/VITRINE = logo du serveur (GitHub / Forgejo).
const ZICON = {
  desktop: `<svg class="pg2-ico" viewBox="0 0 24 24" aria-hidden="true"><rect x="2.5" y="4" width="19" height="12.5" rx="1.5"/><path d="M8.5 20.5h7M12 16.5v4"/></svg>`,
  github: `<svg class="pg2-ico" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2.2a9.8 9.8 0 0 0-3.1 19.1c.5.1.68-.21.68-.47v-1.6c-2.77.6-3.35-1.34-3.35-1.34-.45-1.15-1.1-1.46-1.1-1.46-.9-.62.07-.6.07-.6 1 .07 1.53 1.03 1.53 1.03.9 1.53 2.34 1.09 2.91.83.09-.65.35-1.1.63-1.34-2.21-.25-4.53-1.1-4.53-4.92 0-1.09.39-1.98 1.03-2.68-.1-.26-.45-1.27.1-2.64 0 0 .84-.27 2.75 1.02a9.5 9.5 0 0 1 5 0c1.91-1.29 2.75-1.02 2.75-1.02.55 1.37.2 2.38.1 2.64.64.7 1.03 1.59 1.03 2.68 0 3.83-2.33 4.67-4.55 4.91.36.31.68.92.68 1.86v2.75c0 .27.18.58.69.47A9.8 9.8 0 0 0 12 2.2Z"/></svg>`,
  forgejo: `<svg class="pg2-ico" viewBox="0 0 24 24" aria-hidden="true"><circle cx="7" cy="5.5" r="2.2"/><circle cx="7" cy="18.5" r="2.2"/><circle cx="17" cy="9.5" r="2.2"/><path d="M7 7.7v8.6M7 12h5a3 3 0 0 0 3-3V11.7"/></svg>`,
};
// providerMark : logo du fournisseur d'une connexion (GitHub explicite ; sinon Forgejo/Gitea).
function providerMark(instance) {
  const i = (state.instances || []).find((x) => x.name === instance) || {};
  const prov = (i.provider || "").toLowerCase();
  const url = (i.url || "").toLowerCase();
  if (prov === "github" || url.includes("github.")) return ZICON.github;
  return ZICON.forgejo; // Forgejo et Gitea partagent l'API ; même pictogramme « forge »
}
// Palette d'icônes proposées pour un projet (emoji, rendu partout, zéro dépendance). Une icône MAISON
// (image recadrée en carré) est aussi possible : elle est stockée en data URL dans le même champ `icon`.
const PROJECT_ICONS = ["🗂️", "🚀", "🌐", "🛠️", "📦", "💡", "🧩", "🎨", "📊", "🔮", "🏷️", "🧪", "📝", "⚙️", "🔒", "🌱",
  "🖥️", "📱", "☁️", "🔧", "📚", "🗃️", "🧠", "⭐", "🔥", "🏔️", "🌲", "🏭", "🛰️", "🧰", "🔗", "📡"];

// isCustomIcon : une icône maison est une image encodée en data URL (sinon c'est un emoji).
function isCustomIcon(ic) { return typeof ic === "string" && ic.indexOf("data:image/") === 0; }

// iconHTML rend une icône de projet partout de la même façon : <img> pour une icône maison (data URL),
// sinon l'emoji en texte. cls donne la classe CSS de taille/forme.
function iconHTML(ic, cls) {
  if (!ic) return "";
  if (isCustomIcon(ic)) return `<img class="${cls} custom-ico" src="${ic}" alt="" />`;
  return `<span class="${cls}">${esc(ic)}</span>`;
}

// openIconCropper : éditeur de CADRAGE CARRÉ pour une icône maison. Overlay autonome (n'interfère pas
// avec la modale en cours). L'utilisateur charge une image, la déplace et la zoome dans un cadre carré ;
// « Utiliser » exporte une vignette 96×96 PNG en data URL passée à onPick. Zéro dépendance (canvas natif).
function openIconCropper(onPick) {
  const inp = document.createElement("input");
  inp.type = "file"; inp.accept = "image/*";
  inp.onchange = () => {
    const f = inp.files && inp.files[0]; if (!f) return;
    const r = new FileReader();
    r.onload = () => { const img = new Image(); img.onload = () => cropUI(img, onPick); img.src = r.result; };
    r.readAsDataURL(f);
  };
  inp.click();
}

function cropUI(img, onPick) {
  const BOX = 240; // viewport carré d'édition
  const ov = document.createElement("div");
  ov.className = "cropper-ov";
  ov.innerHTML = `<div class="cropper-card">
    <div class="cropper-ttl">Cadrer l'icône <span class="muted">(glissez pour positionner, curseur pour zoomer)</span></div>
    <div class="cropper-stage" style="width:${BOX}px;height:${BOX}px"><canvas width="${BOX}" height="${BOX}"></canvas><div class="cropper-ring"></div></div>
    <input type="range" class="cropper-zoom" min="1" max="3" step="0.01" value="1" />
    <div class="cropper-actions"><button type="button" class="btn btn-ghost btn-sm cr-cancel">Annuler</button><button type="button" class="btn btn-primary btn-sm cr-ok">Utiliser</button></div>
  </div>`;
  document.body.appendChild(ov);
  const cv = ov.querySelector("canvas"), ctx = cv.getContext("2d"), zoom = ov.querySelector(".cropper-zoom");
  const base = Math.max(BOX / img.width, BOX / img.height); // couvre le carré
  let z = 1, ox = 0, oy = 0; // zoom relatif + décalage
  const clamp = () => {
    const w = img.width * base * z, h = img.height * base * z;
    const mx = Math.max(0, (w - BOX) / 2), my = Math.max(0, (h - BOX) / 2);
    ox = Math.min(mx, Math.max(-mx, ox)); oy = Math.min(my, Math.max(-my, oy));
  };
  const draw = () => {
    clamp();
    const w = img.width * base * z, h = img.height * base * z;
    ctx.clearRect(0, 0, BOX, BOX);
    ctx.drawImage(img, (BOX - w) / 2 + ox, (BOX - h) / 2 + oy, w, h);
  };
  draw();
  zoom.oninput = () => { z = parseFloat(zoom.value); draw(); };
  let drag = null;
  cv.onpointerdown = (e) => { drag = { x: e.clientX, y: e.clientY }; cv.setPointerCapture(e.pointerId); };
  cv.onpointermove = (e) => { if (!drag) return; ox += e.clientX - drag.x; oy += e.clientY - drag.y; drag = { x: e.clientX, y: e.clientY }; draw(); };
  cv.onpointerup = () => { drag = null; };
  const close = () => ov.remove();
  ov.querySelector(".cr-cancel").onclick = close;
  ov.onclick = (e) => { if (e.target === ov) close(); };
  ov.querySelector(".cr-ok").onclick = () => {
    const out = document.createElement("canvas"); out.width = out.height = 96;
    const octx = out.getContext("2d"); const k = 96 / BOX;
    const w = img.width * base * z * k, h = img.height * base * z * k;
    octx.drawImage(img, (96 - w) / 2 + ox * k, (96 - h) / 2 + oy * k, w, h);
    close(); onPick(out.toDataURL("image/png"));
  };
}

// iconGridHTML : la grille d'icônes (∅ + emojis + icône maison courante + bouton « 📷 maison »).
function iconGridHTML(cur) {
  const none = `<button type="button" class="ep-ico${!cur ? " on" : ""}" data-ic="">∅</button>`;
  const tiles = PROJECT_ICONS.map((ic) => `<button type="button" class="ep-ico${cur === ic ? " on" : ""}" data-ic="${esc(ic)}">${ic}</button>`).join("");
  const custom = isCustomIcon(cur) ? `<button type="button" class="ep-ico on" data-ic="__cur__"><img src="${cur}" alt="" /></button>` : "";
  const up = `<button type="button" class="ep-ico ep-ico-up" data-ic="__up__" title="Icône maison (image)">📷</button>`;
  return none + tiles + custom + up;
}

// bindIconGrid remplit un conteneur de grille d'icônes et câble la sélection SANS re-render destructif :
// un emoji bascule juste la classe active (le nom saisi est préservé) ; « 📷 » ouvre le cadreur carré et
// ne re-rend QUE la grille. getCur/setCur lisent/écrivent l'icône dans l'état du formulaire (W ou E).
function bindIconGrid(gridEl, getCur, setCur) {
  if (!gridEl) return;
  gridEl.innerHTML = iconGridHTML(getCur());
  gridEl.querySelectorAll(".ep-ico").forEach((btn) => btn.onclick = () => {
    const v = btn.dataset.ic;
    if (v === "__up__") { openIconCropper((url) => { setCur(url); bindIconGrid(gridEl, getCur, setCur); }); return; }
    if (v === "__cur__") return; // déjà l'icône maison courante
    setCur(v);
    gridEl.querySelectorAll(".ep-ico").forEach((x) => x.classList.toggle("on", x.dataset.ic === v));
  });
}

// projectGraphCard : coquille d'une carte projet = en-tête + actions + le graphe (rempli async).
function projectGraphCard(b, idx) {
  const atl = b.atelier || {}, prods = b.productions || [];
  const syncBtn = b.local && (!b.local.clean || b.local.ahead || b.local.behind) ? `<button class="btn btn-ghost btn-sm" data-sync="${esc(b.name)}" title="Récupérer, enregistrer vos modifications et les envoyer à l'atelier — en un geste, sous PIN. (Proposé seulement quand il y a quelque chose à synchroniser ; la lecture de l'état, elle, est automatique.)">Synchroniser</button>` : "";
  const cloneBtn = !b.local ? `<button class="btn btn-ghost btn-sm" data-clone="${esc(b.name)}" title="Copier le dépôt atelier dans un dossier de ce poste">Cloner ici</button>` : "";
  const prodBtns = prods.map((p, i) => {
    if (p.error) return ""; // serveur vitrine déconnecté → masque + Reconnecter dans la boîte VITRINE
    const prod = `${esc(p.owner)}/${esc(p.repo)}`;
    return p.balanced
      ? `<button class="lnk" data-promote="${esc(b.name)}" data-idx="${i}" data-label="${prod}">repromouvoir</button>`
      : `<button class="btn btn-primary btn-sm" data-promote="${esc(b.name)}" data-idx="${i}" data-label="${prod}">Promouvoir →</button>`;
  }).join("");
  const ico = iconHTML(b.icon, "pboard-ico");
  return `<div class="pboard">
    <div class="pboard-h">
      <div class="pboard-ttl">${ico}<b class="pboard-name">${esc(b.name)}</b> <span class="muted">${esc(atl.owner)}/${esc(atl.repo)}</span>${atl.version ? ` <span class="pill ver">${esc(atl.version)}</span>` : ""}</div>
      <div class="pbc-act">${syncBtn}${cloneBtn}${prodBtns}<button class="lnk" data-editproj="${esc(b.name)}" title="Renommer, changer l'icône, l'atelier ou les vitrines du projet">modifier</button><button class="lnk" data-detatl="${esc(b.name)}" title="Ouvrir le détail du dépôt atelier (branches, fichiers, activité)">détail ›</button><button class="lnk danger" data-delproj="${esc(b.name)}" title="Retirer ce lien de projet (aucun dépôt n'est supprimé)">supprimer</button></div>
    </div>
    <div class="pgraph" id="pg-${idx}"><div class="skeleton"></div></div>
  </div>`;
}

// fillProjectGraph : récupère les branches (clone local si présent, sinon atelier distant) et dessine
// le graphe à voies.
async function fillProjectGraph(b, idx) {
  const box = el("pg-" + idx); if (!box) return;
  let branches = [];
  try {
    if (b.local) {
      const m = await App().BranchMatrix(b.local.path);
      branches = (m && m.branches) || [];
    } else {
      const br = await App().RepoBranches(b.atelier.instance, b.atelier.owner, b.atelier.repo);
      branches = (br || []).map((x) => ({ name: x.name, has_local: false, has_remote: true, author: x.author, ahead: 0, behind: 0 }));
    }
  } catch (e) { if (!el("pg-" + idx)) return; box.innerHTML = `<div class="muted" style="padding:8px 2px">Branches indisponibles : ${esc(cleanErr(e))}</div>`; return; }
  if (el("pg-" + idx)) box.innerHTML = projectGraphHTML(b, branches);
}

// projectGraphHTML : une VOIE par branche, 3 zones Local · Atelier · Production. Nœud par zone où la
// branche existe, segment coloré selon l'état ; nuance ambre si modifs non enregistrées.
function projectGraphHTML(b, branches) {
  if (!branches.length) return `<div class="muted" style="padding:8px 2px">Aucune branche.</div>`;
  const prods = b.productions || [];
  const names = branches.map((x) => x.name);
  const prodBranch = names.includes("main") ? "main" : names.includes("master") ? "master" : ((branches.find((x) => x.current) || branches[0]).name);
  const sorted = branches.slice().sort((a, c) => (a.name === prodBranch ? -1 : c.name === prodBranch ? 1 : a.name.localeCompare(c.name)));

  // tag d'une branche connue (pour afficher « depuis quoi » un travail local démarre)
  const tagOf = (nm) => { const x = branches.find((y) => y.name === nm); return x ? (x.remote_version || x.local_version || "") : ""; };
  // bulle : vert = présent/à jour, ambre = attention (travail en cours / en retard) ; libellé = tag si connu
  const node = (cls, label, title) => `<span class="pg-node ${cls}${label ? " pg-tagged" : ""}" title="${esc(title)}">${label ? esc(label) : ""}</span>`;

  const lanes = sorted.map((br) => {
    const hereBranch = b.local && br.name === b.local.branch;
    const dirty = hereBranch && !b.local.clean;
    // Nœud LOCAL
    const localNode = br.has_local
      ? node(`pg-local${dirty ? " pg-dirty" : ""}${hereBranch ? " pg-here" : ""}`, br.local_version,
          dirty ? "travail en cours (" + b.local.changed + " fichier" + (b.local.changed > 1 ? "s" : "") + ")" : "sur ce poste" + (br.local_version ? " · " + br.local_version : ""))
      : node("pg-empty", "", "pas sur ce poste");
    // Segment LOCAL→ATELIER
    let la = "pg-s-empty", laTxt = "";
    if (br.has_local && br.has_remote) {
      if (dirty) { la = "pg-s-dirty"; laTxt = "travail en cours"; }
      else if (br.ahead && br.behind) { la = "pg-s-div"; laTxt = "à réunir"; }
      else if (br.ahead) { la = "pg-s-ahead"; laTxt = "↑" + br.ahead; }
      else if (br.behind) { la = "pg-s-behind"; laTxt = "↓" + br.behind; }
      else { la = "pg-s-ok"; laTxt = "✓"; }
    } else if (br.has_local) { la = "pg-s-local"; laTxt = "local seul"; }
    // Nœud ATELIER : vert quand à jour, ambre quand il lui manque des commits locaux (br.ahead)
    const atelierNode = br.has_remote
      ? node(`pg-atelier${br.has_local && br.ahead ? " pg-late" : ""}`, br.remote_version,
          "sur l'atelier" + (br.remote_version ? " · " + br.remote_version : ""))
      : node("pg-empty", "", "pas sur l'atelier");
    // Segment ATELIER→PROD + nœud PROD (voie principale uniquement)
    let ap = "pg-s-empty", apTxt = "", prodNode = node("pg-empty", "", "");
    if (br.name === prodBranch && prods.length && !prods[0].error) {
      const p = prods[0];
      ap = p.balanced ? "pg-s-ok" : "pg-s-behind";
      apTxt = p.balanced ? "✓" : "↓" + p.behind;
      prodNode = node(`pg-prod${p.balanced ? "" : " pg-late"}`, p.version || "—", esc(p.owner) + "/" + esc(p.repo) + " · " + esc(p.version || "—"));
    }
    // « depuis quoi » : branche de suivi d'un travail local non encore publié (lien discret pointillé)
    let from = "";
    if (br.has_local && !br.has_remote && br.upstream) {
      const base = br.upstream.replace(/^origin\//, ""); const bt = tagOf(base);
      from = ` <small class="pg-from" title="suit ${esc(br.upstream)}">⇠ ${esc(base)}${bt ? " " + esc(bt) : ""}</small>`;
    }
    // Branche PRINCIPALE (main/master ou la branche de prod) vs SECONDAIRE (feat/work…). Une secondaire
    // non encore réunie vers l'atelier porte un marqueur « en cours » (orange = travail en cours).
    const isPrincipal = br.name === prodBranch || br.name === "main" || br.name === "master";
    const wip = !isPrincipal && br.has_local && (!br.has_remote || br.ahead);
    const wipChip = wip ? ` <span class="chip-wip" title="travail en cours — pas encore réuni vers ${esc(prodBranch)}">en cours</span>` : "";
    return {
      label: `<span class="pg-brn ${isPrincipal ? "pg-brn-main" : "pg-brn-sec"}">⎇ ${esc(br.name)}</span>${hereBranch ? ` <span class="pg-youare">ici</span>` : ""}${wipChip}${from}${br.author ? `<small> · ${esc(br.author)}</small>` : ""}`,
      title: esc(br.name),
      localOn: !!br.has_local, localNode,
      atlOn: !!br.has_remote, atelierNode,
      prodOn: br.name === prodBranch && prods.length && !prods[0].error, prodNode,
      la, laTxt, ap, apTxt,
    };
  });

  // Chaque palier est une BOÎTE ; les traits de branche la traversent (bulle = présence/état),
  // et franchissent l'espace entre boîtes avec un segment coloré selon l'écart.
  // Présence de branche : deux cas DISTINCTS (1.9.2). Si la branche EXISTE sur ce git → trait plein + sa
  // bulle (vide si pas de version). Si elle N'EXISTE PAS ici → aucun trait et un picto « absent » clair,
  // au lieu d'un trait grisé + bulle vide qui laissait croire qu'elle existait mais était vide.
  const cell = (on, inner) => on
    ? `<div class="pg2-cell pg2-on">${inner}</div>`
    : `<div class="pg2-cell pg2-absent"><span class="pg-absent" title="cette branche n'existe pas ici">–</span></div>`;
  const gap = (cls, txt) => `<div class="pg2-cell pg-seg ${cls}"><em>${txt}</em></div>`;
  // Masque grisant posé sur une boîte dont le serveur est déconnecté : message amical + Reconnecter.
  const offMask = (inst, err) => inst ? `<div class="pg2-off-mask"><div class="pg2-off-ico">🔌</div><div class="pg2-off-t">Serveur « ${esc(inst)} » déconnecté</div><button class="btn btn-primary btn-sm" data-reconinst="${esc(inst)}">Reconnecter</button></div>` : "";
  const zone = (title, cls, cells, icon, mask) => `<div class="pg2-col pg2-zone ${cls}"><div class="pg2-head">${icon || ""}<span>${title}</span></div>${cells.join("")}${mask || ""}</div>`;
  const gapCol = (cells) => `<div class="pg2-col pg2-gapcol"><div class="pg2-head"></div>${cells.join("")}</div>`;

  const atl = b.atelier || {}, prod0 = (b.productions || [])[0] || {};
  const atlMask = atl.error ? offMask(atl.instance, atl.error) : "";
  const prodErr = (b.productions || []).find((p) => p.error);
  const hasVitrine = !!(b.productions || []).length;

  const labelCol = `<div class="pg2-col pg2-labels"><div class="pg2-head"></div>` +
    lanes.map((r) => `<div class="pg2-name" title="${r.title}">${r.label}</div>`).join("") + `</div>`;
  const localCol = zone("LOCAL", "pg2-local", lanes.map((r) => cell(r.localOn, r.localNode)), ZICON.desktop);
  const g1 = gapCol(lanes.map((r) => gap(r.la, r.laTxt)));
  const atlCol = zone("ATELIER", "pg2-atl", lanes.map((r) => cell(r.atlOn, r.atelierNode)), providerMark(atl.instance), atlMask);

  // Colonne Vitrine CONDITIONNELLE (1.9.2) : affichée seulement s'il y a une vitrine (ou un serveur
  // vitrine déconnecté à signaler). Sans vitrine → pas de colonne « Aucune vitrine » vide, mais un
  // simple bouton « connecter une vitrine » qui ouvre le tunnel à l'étape vitrine.
  let tail;
  if (hasVitrine || prodErr) {
    const prodMask = prodErr ? offMask(prodErr.instance, prodErr.error) : "";
    const g2 = gapCol(lanes.map((r) => gap(r.ap, r.apTxt)));
    const prodCol = zone("VITRINE", "pg2-prod", lanes.map((r) => cell(r.prodOn, r.prodNode)), providerMark(prod0.instance), prodMask);
    tail = g2 + prodCol;
  } else {
    tail = `<div class="pg2-col pg2-connect"><div class="pg2-head"></div><div class="pg2-connect-box"><button type="button" class="btn btn-ghost btn-sm" data-addvitrine="${esc(b.name)}">＋ Connecter une vitrine</button></div></div>`;
  }

  return `<div class="pg2">${labelCol}${localCol}${g1}${atlCol}${tail}</div>`;
}

// projectSync : Synchroniser le clone local avec l'atelier. L'atelier n'exige pas le PIN par défaut
// (politique « atelier autonome ») → on tente sans PIN ; si la politique l'exige, on le demande.
async function projectSync(b) {
  if (!b.local) return;
  const run = (pin) => App().Synchronize(b.atelier.instance, b.local.path, "", pin);
  try {
    toast("Synchronisation…");
    const res = await run("");
    toast(res && res.summary ? res.summary : "Synchronisé", "ok"); renderProjects();
  } catch (e) {
    if (isLocked(e)) return handleLock();
    if (isPinNeeded(e)) {
      pinConfirm("Synchroniser l'atelier « " + b.atelier.owner + "/" + b.atelier.repo + " »", async (pin) => {
        const res = await run(pin); closeModal(); toast(res && res.summary ? res.summary : "Synchronisé", "ok"); renderProjects();
      });
    } else toast(cleanErr(e), "err");
  }
}

// projectCloneModal : cloner le dépôt atelier d'un projet sur ce poste.
function projectCloneModal(b) {
  openModal("Cloner l'atelier ici", `<div class="field">
    <p class="sub">Copier le dépôt atelier <b>${esc(b.atelier.owner)}/${esc(b.atelier.repo)}</b> dans un dossier (il sera créé).</p>
    <label class="form-label">Dossier de destination</label>
    <input id="pcl" class="inp" placeholder="C:\\work\\${esc(b.atelier.repo || "")}" />
    <div class="inline-error hidden" id="pcl-err"></div></div>`, async () => {
    const er = el("pcl-err");
    try {
      toast("Copie en cours…");
      await App().Clone(b.atelier.instance, b.atelier.owner, b.atelier.repo, el("pcl").value.trim());
      closeModal(); toast("Atelier copié sur votre poste", "ok"); renderProjects();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  el("modal-ok").textContent = "Cloner";
}

// openProjectDetail : ouvre le détail d'un dépôt d'un projet (bascule sur sa connexion au besoin).
function openProjectDetail(instance, owner, repo, localPath) {
  if (instance && instance !== state.instance) {
    state.instance = instance;
    const sel = el("instance-select"); if (sel) sel.value = instance;
    state.repos = null; state.repoSig = ""; updateRoleBadge();
  }
  state.detail = { full_name: owner + "/" + repo, owner, repo, has_remote: true, html_url: "", local_path: localPath || "", ref: "", tab: "apercu", path: "", refs: [], tags: [], _remote: null, _local: null };
  state.view = "detail"; render();
}

// bindProjectBoard : actions du tableau (délégation stable, résolution par nom de projet).
function bindProjectBoard(boards) {
  const byName = {}; boards.forEach((b) => byName[b.name] = b);
  const root = el("proj-board"); if (!root) return;
  root.onclick = (ev) => {
    const pr = ev.target.closest("[data-promote]");
    if (pr) return promoteWizard(byName[pr.dataset.promote], parseInt(pr.dataset.idx, 10), pr.dataset.label);
    const rc = ev.target.closest("[data-reconinst]"); if (rc) return reconnectModal(rc.dataset.reconinst);
    const ep = ev.target.closest("[data-editproj]"); if (ep) return editProjectModal(byName[ep.dataset.editproj]);
    const av = ev.target.closest("[data-addvitrine]"); if (av) return editProjectModal(byName[av.dataset.addvitrine], { addVitrine: true });
    const sy = ev.target.closest("[data-sync]"); if (sy) return projectSync(byName[sy.dataset.sync]);
    const cl = ev.target.closest("[data-clone]"); if (cl) return projectCloneModal(byName[cl.dataset.clone]);
    const dt = ev.target.closest("[data-detatl]"); if (dt) { const b = byName[dt.dataset.detatl]; return openProjectDetail(b.atelier.instance, b.atelier.owner, b.atelier.repo, b.local ? b.local.path : ""); }
    const dp = ev.target.closest("[data-delproj]");
    if (dp) {
      openModal("Supprimer ce projet ?", `<div class="field"><div class="callout warn">Le lien « ${esc(dp.dataset.delproj)} » est retiré. Aucun dépôt n'est touché.</div></div>`, async () => {
        try { await App().RemoveProject(dp.dataset.delproj); closeModal(); toast("Projet supprimé", "ok"); renderProjects(); } catch (e) { toast(cleanErr(e), "err"); }
      });
      el("modal-ok").textContent = "Supprimer"; el("modal-ok").classList.add("danger-btn");
    }
  };
}

// promoteWizard : l'utilisateur DÉCIDE ce qu'il promeut — quelle version de quelle branche d'atelier,
// vers quelle branche de production — puis confirme au PIN. Aucune valeur implicite déclenchée seule.
async function promoteWizard(b, idx, label) {
  if (!b) return;
  if (!b.local || !b.local.path) {
    openModal("Clone requis", `<div class="field"><div class="callout warn">Pour promouvoir, l'atelier de ce projet doit d'abord être <b>cloné sur ce poste</b> (bouton « Cloner ici »). On promeut depuis le clone local, jamais à l'aveugle.</div></div>`, () => closeModal());
    el("modal-ok").textContent = "Compris"; return;
  }
  const path = b.local.path, prod = (b.productions || [])[idx] || {};
  let ci;
  const onOk = async () => {
    const er = el("promo-err"); if (!er || !ci) return;
    try {
      const pwMode = el("pw-mode"), pwNotes = el("pw-notes");
      const res = await App().ProjectPromote(b.name, idx, el("pw-src").value, el("pw-ver").value, el("pw-dst").value, (pwMode ? pwMode.value : "tag"), (pwNotes ? pwNotes.value : ""), ci.read());
      closeModal(); toast(res && res.summary ? res.summary : "Promu en production", "ok"); renderProjects();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); ci.clear(); }
  };
  openModal("Promouvoir en production", `<div class="field"><div class="skeleton"></div></div>`, onOk);
  el("modal-ok").disabled = true; // tant que les listes ne sont pas chargées
  // Charge les branches d'atelier et les branches de la production cible.
  let atlBranches = [], prodBranches = [];
  try {
    const m = await App().BranchMatrix(path);
    atlBranches = ((m && m.branches) || []).filter((x) => x.has_remote).map((x) => x.name);
    prodBranches = ((await App().RepoBranches(prod.instance, prod.owner, prod.repo)) || []).map((x) => x.name);
  } catch (e) { el("modal-body").innerHTML = `<h3>Promouvoir en production</h3><div class="field"><div class="inline-error">${esc(cleanErr(e))}</div></div>`; return; }
  if (!atlBranches.length) atlBranches = ["main"];
  const atlDef = atlBranches.includes("main") ? "main" : atlBranches[0];
  const prodDef = prodBranches.includes("main") ? "main" : (prodBranches.includes("master") ? "master" : (prodBranches[0] || "main"));
  const opt = (v, sel) => `<option value="${esc(v)}"${v === sel ? " selected" : ""}>${esc(v)}</option>`;
  el("modal-body").innerHTML = `<h3 id="modal-title">Promouvoir en production</h3><div class="field">
    <p class="sub">Publiez une version <b>choisie</b> de l'atelier vers la production <b>${esc(label)}</b>, en une version propre.</p>
    <div class="callout warn">La production est une vitrine : rien n'est promu sans que vous ayez décidé quelle version et quelle branche.</div>
    <label>Branche de l'atelier (source)</label>
    <select class="select" id="pw-src">${atlBranches.map((x) => opt(x, atlDef)).join("")}</select>
    <label>Version à publier</label>
    <select class="select" id="pw-ver"><option>…</option></select>
    <label>Branche de la production (cible)</label>
    <select class="select" id="pw-dst">${prodBranches.map((x) => opt(x, prodDef)).join("")}</select>
    <label>Méthode</label>
    <select class="select" id="pw-mode">
      <option value="tag" selected>Version (code + tag)</option>
      <option value="code">Code seul (sans tag)</option>
      <option value="release">Version + Release</option>
    </select>
    <div id="pw-mode-help" class="set-note"></div>
    <div id="pw-notes-wrap" class="hidden"><label>Notes de version</label><textarea class="inp" id="pw-notes" rows="3" placeholder="Nouveautés de cette version…"></textarea></div>
    <label>PIN</label><div id="promo-pin"></div>
    <div class="inline-error hidden" id="promo-err"></div></div>`;
  // Explication de la méthode (promouvoir ≠ publier une Release) + champ notes réservé au mode release.
  const MODE_HELP = {
    code: "Met à jour la branche de la vitrine sans tag — aucune version ni Release créée.",
    tag: "Pousse le code + un tag de version. Sur GitHub, le tag déclenche la CI qui bâtit les installeurs et publie la Release (vérifiez Actions — la Release n'est pas automatique côté app).",
    release: "Pousse le code + un tag ET crée la Release (avec notes). Les installeurs restent bâtis par la CI.",
  };
  const onMode = () => {
    const m = el("pw-mode").value;
    el("pw-mode-help").textContent = MODE_HELP[m] || "";
    el("pw-notes-wrap").classList.toggle("hidden", m !== "release");
  };
  el("pw-mode").onchange = onMode; onMode();
  // Recharge les versions (tags) joignables à chaque changement de branche source.
  const loadVersions = async () => {
    const sel = el("pw-ver"); const src = el("pw-src").value;
    sel.innerHTML = `<option>…</option>`;
    let tags = [];
    try { tags = (await App().BranchTags(path, src)) || []; } catch (_) { tags = []; }
    const tip = `<option value="">pointe de « ${esc(src)} » (sans tag)</option>`;
    sel.innerHTML = tags.map((t, i) => `<option value="${esc(t)}"${i === 0 ? " selected" : ""}>${esc(t)}</option>`).join("") + tip;
  };
  el("pw-src").onchange = loadVersions;
  await loadVersions();
  ci = codeInput(el("promo-pin"), true, () => modalOk && modalOk(), true);
  el("modal-ok").disabled = false;
}

// renderProjects(live) : live=false repeint tout (en-tête + squelette) ; live=true (appelé par la
// boucle « en direct ») recharge silencieusement les boards et ne repeint #proj-board QUE si l'état
// a changé (signature) — pas de skeleton, pas de flicker. Corrige la page qui ne se mettait pas à
// jour seule (il fallait sortir/revenir).
async function renderProjects(live) {
  if (!live) {
    el("view").innerHTML = `<div class="view-head view-head-row">
      <div><div class="view-title">Projets</div></div>
      <button class="btn btn-primary btn-sm" id="proj-add">＋ Nouveau projet</button></div>
    <div id="proj-board"><div class="skeleton"></div><div class="skeleton" style="margin-top:10px"></div></div>`;
    el("proj-add").onclick = addProjectModal;
    state.projSig = "";
  }
  const liveEl = el("live"); if (live && liveEl) liveEl.classList.add("syncing");
  let boards = [];
  try { boards = (await App().ProjectBoards()) || []; }
  catch (e) { if (isLocked(e)) return handleLock(); if (!live) { const b = el("proj-board"); if (b) b.innerHTML = surfaceError("Lecture impossible", cleanErr(e), null); } return; }
  finally { const l = el("live"); if (live && l) l.classList.remove("syncing"); }
  const sig = JSON.stringify(boards);
  if (live && sig === state.projSig) return; // rien de neuf : on ne touche pas au DOM
  state.projSig = sig;
  const board = el("proj-board"); if (!board) return;
  if (!boards.length) {
    board.innerHTML = `<div class="proj-empty">
      <div class="pe-ico">⇄</div>
      <div class="pe-title">Reliez votre premier projet</div>
      <p class="pe-txt">Un projet relie un dépôt <b>atelier</b> à sa <b>vitrine</b>, et montre en une vue l'état entre <b>poste</b>, <b>atelier</b> et <b>prod</b>.</p>
      <button class="btn btn-primary" id="proj-add2">＋ Créer mon premier projet</button>
    </div>`;
    if (el("proj-add2")) el("proj-add2").onclick = addProjectModal;
    return;
  }
  board.innerHTML = boards.map((b, i) => projectGraphCard(b, i)).join("");
  bindProjectBoard(boards);
  boards.forEach((b, i) => fillProjectGraph(b, i)); // dessine chaque graphe (branches) en parallèle
}

// Cache des dépôts accessibles par connexion (évite de recharger à chaque changement).
const _repoCache = {};
async function reposOf(instance) {
  if (_repoCache[instance]) return _repoCache[instance];
  const repos = (await App().Repos(instance)) || [];
  _repoCache[instance] = repos;
  return repos;
}
// splitFull découpe « owner/repo » (sur le 1er « / »).
function splitFull(fn) {
  const i = fn.indexOf("/");
  return i < 0 ? { owner: "", repo: fn } : { owner: fn.slice(0, i), repo: fn.slice(i + 1) };
}
// addProjectModal : tunnel PAS À PAS. Nom → Atelier (compte → organisation si plusieurs → dépôt)
// → Production (uniquement les connexions marquées « production » ; sinon message « impossible »)
// → dépôt → « ajouter une autre production ? » → créer. Navigation Retour/Annuler.
function addProjectModal() {
  const insts = state.instances || [];
  const ateliers = insts.filter((i) => (i.role || "atelier") !== "production");
  const prods = insts.filter((i) => i.role === "production");
  const W = { name: "", icon: "", atelier: null, productions: [] };
  const hist = []; // pile des écrans pour « Retour »
  const m = el("modal"), okBtn = el("modal-ok"), cancelBtn = el("modal-cancel");
  m.classList.remove("hidden");
  m.onclick = (ev) => { if (ev.target === m) closeModal(); };

  const head = (n, total, t) => `<h3 id="modal-title">Nouveau projet</h3><div class="wiz-head"><span class="wiz-step">Étape ${n}/${total}</span> · ${esc(t)}</div>`;
  function screen(html) { el("modal-body").innerHTML = html; }
  function buttons(okText, okFn) {
    okBtn.classList.toggle("hidden", !okText);
    okBtn.classList.remove("danger-btn"); okBtn.disabled = false;
    okBtn.textContent = okText || ""; okBtn.onclick = okFn || null;
    cancelBtn.textContent = hist.length ? "Retour" : "Annuler";
    cancelBtn.onclick = hist.length ? back : closeModal;
  }
  function go(fn) { hist.push(fn); fn(); }
  function back() { hist.pop(); const prev = hist[hist.length - 1]; prev ? prev() : stepName(); }

  // Liste de choix cliquable (auto-avance). items : {label, sub, value}.
  function pickItems(items) {
    return items.map((it, ix) => `<button type="button" class="pick-item" data-ix="${ix}"><span class="pick-main">${esc(it.label)}</span>${it.sub ? `<span class="pick-sub">${esc(it.sub)}</span>` : ""}</button>`).join("");
  }
  function pickList(items) { return `<div class="pick-list">${pickItems(items)}</div>`; }
  function bindPick(items, onPick) { el("modal-body").querySelectorAll(".pick-item").forEach((b) => b.onclick = () => onPick(items[+b.dataset.ix])); }
  // Habillage coloré par type : atelier (bleu) vs production (ambre), bien distincts.
  const kindCls = (kind) => kind === "production" ? "wiz-prod" : "wiz-atelier";
  const kindBanner = (kind) => kind === "production"
    ? `<div class="wiz-banner"><span class="role-badge role-production">VITRINE</span> <span>la <b>vitrine</b> (production) — on n'y publie que des versions validées, jamais depuis le local</span></div>`
    : `<div class="wiz-banner"><span class="role-badge role-atelier">ATELIER</span> <span>votre <b>terrain de dev</b> — la source de vérité du projet</span></div>`;
  // Liste de dépôts AVEC recherche (les listes peuvent être longues). Filtre en direct sur le nom.
  function searchablePick(headHtml, items, onPick, kind) {
    screen(`<div class="wiz-scope ${kindCls(kind)}">${kindBanner(kind)}${headHtml}<input id="wiz-search" class="inp" placeholder="Rechercher un dépôt…" autocomplete="off" /><div id="wiz-list" class="pick-list" style="margin-top:8px"></div></div>`);
    const draw = (f) => {
      const q = (f || "").trim().toLowerCase();
      const shown = q ? items.filter((it) => (it.label + " " + (it.sub || "")).toLowerCase().includes(q)) : items;
      const box = el("wiz-list");
      box.innerHTML = shown.length ? pickItems(shown) : `<div class="muted" style="padding:8px 2px">Aucun dépôt ne correspond à « ${esc(f)} ».</div>`;
      box.querySelectorAll(".pick-item").forEach((b) => b.onclick = () => onPick(shown[+b.dataset.ix]));
    };
    draw("");
    const s = el("wiz-search"); if (s) { s.oninput = () => draw(s.value); setTimeout(() => s.focus(), 30); }
  }

  // Après le choix d'une connexion : organisation (si plusieurs) puis dépôt. cb(ref).
  async function chooseRepo(kind, instance, cb) {
    const label = kind === "atelier" ? "Atelier" : "Vitrine";
    screen(`${head(kind === "atelier" ? 2 : 4, 4, label + " — chargement des dépôts…")}<div class="skeleton"></div>`); buttons("", null);
    let repos;
    try { repos = (await reposOf(instance)) || []; } catch (e) {
      screen(`${head(kind === "atelier" ? 2 : 4, 4, label)}<div class="callout warn">Lecture impossible : ${esc(cleanErr(e))}</div>`); buttons("", null); return;
    }
    if (!repos.length) { screen(`${head(kind === "atelier" ? 2 : 4, 4, label)}<div class="callout warn">Aucun dépôt accessible sur « ${esc(instance)} ».</div>`); buttons("", null); return; }
    const ownerOf = (r) => splitFull(r.full_name).owner; // forgejo.Repo n'a pas de champ owner séparé
    const owners = [...new Set(repos.map(ownerOf).filter(Boolean))].sort();
    const repoStep = (owner) => go(() => {
      const list = repos.filter((r) => ownerOf(r) === owner);
      const items = list.map((r) => ({ label: r.full_name, sub: r.private ? "privé" : "public", value: r.full_name }));
      searchablePick(head(kind === "atelier" ? 2 : 4, 4, label + " — choisissez le dépôt (" + list.length + ")"),
        items, (it) => { const s = splitFull(it.value); cb({ instance, owner: s.owner, repo: s.repo }); }, kind);
      buttons("", null);
    });
    if (owners.length <= 1) { repoStep(owners[0] || ""); return; }
    go(() => {
      const items = owners.map((o) => ({ label: o, sub: repos.filter((r) => ownerOf(r) === o).length + " dépôt(s)", value: o }));
      screen(`<div class="wiz-scope ${kindCls(kind)}">${kindBanner(kind)}${head(kind === "atelier" ? 2 : 4, 4, label + " — choisissez l'organisation")}${pickList(items)}</div>`);
      bindPick(items, (it) => repoStep(it.value));
      buttons("", null);
    });
  }

  // Étape 1 — nom.
  function stepName() {
    screen(`${head(1, 4, "Nom & icône du projet")}<input id="wiz-name" class="inp" placeholder="ex. mon-site" value="${esc(W.name)}" />
      <label style="margin-top:10px">Icône <span class="muted">(facultatif)</span></label><div class="ep-icogrid" id="wiz-icogrid"></div>`);
    buttons("Suivant", () => { const v = el("wiz-name").value.trim(); if (!v) { toast("Donnez un nom de projet.", "err"); return; } W.name = v; go(stepAtelierConn); });
    const inp = el("wiz-name"); if (inp) { inp.focus(); inp.oninput = () => W.name = inp.value; inp.onkeydown = (e) => { if (e.key === "Enter") okBtn.click(); }; }
    // Grille d'icônes (emoji + icône maison) ; la sélection ne re-rend pas l'écran → le nom saisi est
    // préservé (correction du bug 1.9.1 « changer d'icône efface le nom »).
    bindIconGrid(el("wiz-icogrid"), () => W.icon, (v) => W.icon = v);
  }
  // Étape 2 — atelier : compte git.
  function stepAtelierConn() {
    if (!ateliers.length) { screen(`${head(2, 4, "Atelier")}<div class="callout warn">Aucune connexion « atelier ». Marquez une connexion comme atelier dans Réglages → Connexions → « rôle & PIN ».</div>`); buttons("", null); return; }
    const items = ateliers.map((i) => ({ label: i.name, sub: i.url, value: i.name }));
    screen(`<div class="wiz-scope wiz-atelier">${kindBanner("atelier")}${head(2, 4, "Quel est l'atelier ? (compte git)")}${pickList(items)}</div>`);
    bindPick(items, (it) => chooseRepo("atelier", it.value, (ref) => { W.atelier = ref; go(stepProdIntro); }));
    buttons("", null);
  }
  // Étape 3 — production : intro (ou impossible), puis chooseRepo.
  function stepProdConn() {
    const items = prods.map((i) => ({ label: i.name, sub: i.url, value: i.name }));
    screen(`<div class="wiz-scope wiz-prod">${kindBanner("production")}${head(3, 4, "Quelle est la vitrine ? (production)")}${pickList(items)}</div>`);
    bindPick(items, (it) => chooseRepo("production", it.value, (ref) => { addProd(ref); go(stepAddMore); }));
    buttons("", null);
  }
  // Étape 3 — portée : atelier seul, ou atelier + vitrine. Choix explicite (un projet n'exige qu'un
  // atelier ; la vitrine se rajoute ici ou plus tard via « modifier »).
  function stepProdIntro() {
    const noProd = !prods.length;
    screen(`${head(3, 4, "Portée du projet")}
      <p class="sub">Un projet se crée au minimum avec un <b>atelier</b>. Vous pouvez y relier une <b>vitrine</b> (production) maintenant, ou plus tard via « modifier ».</p>
      <div class="pick-list" style="margin-top:8px">
        <button type="button" class="pick-item" id="scope-solo"><span class="pick-main">🏠 Local + Atelier seul</span><span class="pick-sub">le poste et l'atelier interne — pas de vitrine publique pour l'instant</span></button>
        <button type="button" class="pick-item" id="scope-vitrine"${noProd ? " disabled" : ""}><span class="pick-main">🌐 Local + Atelier + Vitrine</span><span class="pick-sub">${noProd ? "aucune connexion « production » — marquez-en une dans Réglages" : "relier une vitrine (production) à promouvoir"}</span></button>
      </div>`);
    buttons("", null);
    el("scope-solo").onclick = () => { W.productions = []; go(stepAddMore); };
    const sv = el("scope-vitrine"); if (sv && !noProd) sv.onclick = () => go(stepProdConn);
  }
  function addProd(ref) {
    const key = ref.instance + "/" + ref.owner + "/" + ref.repo;
    if (!W.productions.some((p) => p.instance + "/" + p.owner + "/" + p.repo === key)) W.productions.push(ref);
  }
  // Étape 4 — récapitulatif + « ajouter une autre production ? ».
  function stepAddMore() {
    const prodList = W.productions.map((p) => `<li>${esc(p.owner)}/${esc(p.repo)} <span class="muted">(${esc(p.instance)})</span></li>`).join("");
    screen(`${head(4, 4, "Récapitulatif")}
      <div class="wiz-recap">
        <div>${W.icon ? iconHTML(W.icon, "pboard-ico") + " " : ""}<b>${esc(W.name)}</b></div>
        <div style="margin-top:6px"><span class="role-badge role-atelier">ATELIER</span> ${esc(W.atelier.owner)}/${esc(W.atelier.repo)} <span class="muted">(${esc(W.atelier.instance)})</span></div>
        <div class="wiz-recap-t">Vitrine(s) :</div>
        <ul class="wiz-recap-prods">${prodList || '<li class="muted">aucune — local + atelier</li>'}</ul>
      </div>
      ${prods.length ? `<button type="button" class="btn btn-ghost btn-sm" id="wiz-more">＋ Ajouter une vitrine</button>` : ""}`);
    buttons("Créer le projet", finish);
    const more = el("wiz-more"); if (more) more.onclick = () => go(stepProdConn);
  }
  async function finish() {
    try { await App().AddProject(W.name, W.icon, W.atelier, W.productions); closeModal(); toast("Projet créé", "ok"); renderProjects(); }
    catch (e) { toast(cleanErr(e), "err"); }
  }

  stepName(); // démarre le tunnel (écran racine, non empilé)
}

// editProjectModal : modifier un projet à tout moment — renommer, icône, changer l'atelier, ajouter/
// retirer des vitrines (passer de « local + atelier » à « local + atelier + vitrine » et inversement).
function editProjectModal(b, opts) {
  if (!b) return;
  opts = opts || {};
  const insts = state.instances || [];
  const ateliers = insts.filter((i) => (i.role || "atelier") !== "production");
  const prodsConn = insts.filter((i) => i.role === "production");
  const oldName = b.name;
  const E = {
    name: b.name, icon: b.icon || "",
    atelier: b.atelier ? { instance: b.atelier.instance, owner: b.atelier.owner, repo: b.atelier.repo } : null,
    productions: (b.productions || []).map((p) => ({ instance: p.instance, owner: p.owner, repo: p.repo })),
  };
  openModal("Modifier le projet", `<div class="field" id="ep-body"></div>`, async () => {
    const er = el("ep-err");
    try {
      await App().UpdateProject(oldName, (E.name || "").trim(), E.icon, E.atelier, E.productions);
      closeModal(); toast("Projet modifié", "ok"); renderProjects();
    } catch (e) { if (er) { er.textContent = cleanErr(e); er.classList.remove("hidden"); } else toast(cleanErr(e), "err"); }
  });
  el("modal-ok").textContent = "Enregistrer";

  async function repoPickInto(kind, conns, onPick) {
    const pick = el("ep-pick"); if (!pick) return;
    if (!conns.length) { pick.innerHTML = `<div class="callout warn">Aucune connexion « ${kind === "atelier" ? "atelier" : "production"} » disponible.</div>`; return; }
    pick.innerHTML = `<div class="ep-picker"><label>Connexion</label><select class="select" id="ep-conn">${conns.map((c) => `<option value="${esc(c.name)}">${esc(c.name)}</option>`).join("")}</select>
      <label style="margin-top:6px">Dépôt</label><input class="inp" id="ep-rsearch" placeholder="Rechercher un dépôt…" autocomplete="off" />
      <div id="ep-rlist" class="pick-list" style="margin-top:6px;max-height:190px;overflow:auto"></div></div>`;
    const loadRepos = async () => {
      const inst = el("ep-conn").value, list = el("ep-rlist"); list.innerHTML = `<div class="skeleton"></div>`;
      let repos = []; try { repos = (await reposOf(inst)) || []; } catch (e) { list.innerHTML = `<div class="callout warn">${esc(cleanErr(e))}</div>`; return; }
      const draw2 = (f) => {
        const q = (f || "").trim().toLowerCase();
        const shown = q ? repos.filter((r) => r.full_name.toLowerCase().includes(q)) : repos;
        list.innerHTML = shown.length ? shown.map((r) => `<button type="button" class="pick-item" data-fn="${esc(r.full_name)}"><span class="pick-main">${esc(r.full_name)}</span><span class="pick-sub">${r.private ? "privé" : "public"}</span></button>`).join("") : `<div class="muted" style="padding:6px">Aucun dépôt.</div>`;
        list.querySelectorAll(".pick-item").forEach((btn) => btn.onclick = () => { const s = splitFull(btn.dataset.fn); pick.innerHTML = ""; onPick({ instance: inst, owner: s.owner, repo: s.repo }); });
      };
      draw2(""); const s = el("ep-rsearch"); if (s) s.oninput = () => draw2(s.value);
    };
    el("ep-conn").onchange = loadRepos; await loadRepos();
  }

  function draw() {
    const prodRows = E.productions.length
      ? E.productions.map((p, i) => `<div class="ep-slot"><span class="role-badge role-production">VITRINE</span> ${esc(p.owner + "/" + p.repo)} <span class="muted">(${esc(p.instance)})</span> <button type="button" class="lnk danger" data-rmprod="${i}">retirer</button></div>`).join("")
      : `<div class="muted">Aucune vitrine — projet <b>local + atelier</b>.</div>`;
    el("ep-body").innerHTML = `
      <label>Nom du projet</label><input id="ep-name" class="inp" value="${esc(E.name)}" />
      <label style="margin-top:10px">Icône</label><div class="ep-icogrid" id="ep-icogrid"></div>
      <label style="margin-top:10px">Atelier (source)</label>
      <div class="ep-slot"><span class="role-badge role-atelier">ATELIER</span> ${E.atelier ? esc(E.atelier.owner + "/" + E.atelier.repo) + ` <span class="muted">(${esc(E.atelier.instance)})</span>` : '<span class="muted">non défini</span>'} <button type="button" class="lnk" id="ep-chg-atl">changer…</button></div>
      <label style="margin-top:10px">Vitrines (production)</label>
      <div id="ep-prods">${prodRows}</div>
      ${prodsConn.length ? `<button type="button" class="btn btn-ghost btn-sm" id="ep-add-prod" style="margin-top:6px">＋ Ajouter une vitrine</button>` : `<div class="set-note">Aucune connexion « production » : marquez-en une dans Réglages pour ajouter une vitrine.</div>`}
      <div id="ep-pick" style="margin-top:10px"></div>
      <div class="inline-error hidden" id="ep-err" style="margin-top:10px"></div>`;
    el("ep-name").oninput = () => E.name = el("ep-name").value;
    // Grille d'icônes (emoji + icône maison) sans re-render destructif (préserve le nom et le focus).
    bindIconGrid(el("ep-icogrid"), () => E.icon, (v) => E.icon = v);
    el("ep-body").querySelectorAll("[data-rmprod]").forEach((btn) => btn.onclick = () => { E.productions.splice(+btn.dataset.rmprod, 1); draw(); });
    const chg = el("ep-chg-atl"); if (chg) chg.onclick = () => repoPickInto("atelier", ateliers, (ref) => { E.atelier = ref; draw(); });
    const addp = el("ep-add-prod"); if (addp) addp.onclick = () => repoPickInto("production", prodsConn, (ref) => { const k = ref.instance + "/" + ref.owner + "/" + ref.repo; if (!E.productions.some((p) => p.instance + "/" + p.owner + "/" + p.repo === k)) E.productions.push(ref); draw(); });
  }
  draw();
  // Entrée depuis « connecter une vitrine » (projet local+atelier seul) : ouvrir directement le choix
  // de vitrine, sinon guider vers Réglages s'il n'existe aucune connexion « production ».
  if (opts.addVitrine) {
    if (prodsConn.length) repoPickInto("production", prodsConn, (ref) => { const k = ref.instance + "/" + ref.owner + "/" + ref.repo; if (!E.productions.some((p) => p.instance + "/" + p.owner + "/" + p.repo === k)) E.productions.push(ref); draw(); });
    else toast("Aucune connexion « production » : marquez-en une dans Réglages d'abord.", "err");
  }
}

// renderPromptLib : la bibliothèque de prompts (accordéons éditables). Les prompts par défaut portent un
// badge ; les personnalisations de l'utilisateur sont conservées à travers les mises à jour (stockées en config).
async function renderPromptLib() {
  const box = el("prompt-lib"); if (!box) return;
  box.innerHTML = `<div class="skeleton"></div>`;
  let rows = [];
  try { rows = (await App().Prompts()) || []; } catch (_) { box.innerHTML = ""; return; }
  box.innerHTML = rows.map((p) => `
    <details class="plib-item">
      <summary><span class="plib-title">${esc(p.title)}</span> ${p.builtin ? `<span class="pill sm">${p.overridden ? "modifié" : "par défaut"}</span>` : `<span class="pill sm">perso</span>`}</summary>
      <pre class="plib-body">${esc(p.body)}</pre>
      <div class="plib-actions">
        <button class="btn btn-ghost btn-sm" data-pcopy="${esc(p.id)}">Copier</button>
        <button class="btn btn-ghost btn-sm" data-pedit="${esc(p.id)}">Modifier</button>
        ${p.builtin && p.overridden ? `<button class="btn btn-ghost btn-sm" data-preset="${esc(p.id)}">Réinitialiser</button>` : ""}
        <button class="lnk danger" data-pdel="${esc(p.id)}">${p.builtin ? "Masquer" : "Supprimer"}</button>
      </div>
    </details>`).join("") || `<div class="muted">Aucun prompt.</div>`;
  const byId = {}; rows.forEach((r) => byId[r.id] = r);
  box.querySelectorAll("[data-pcopy]").forEach((b) => b.onclick = () => copyPrompt(byId[b.dataset.pcopy].body, { title: byId[b.dataset.pcopy].title }));
  box.querySelectorAll("[data-pedit]").forEach((b) => b.onclick = () => promptEditModal(byId[b.dataset.pedit]));
  box.querySelectorAll("[data-preset]").forEach((b) => b.onclick = async () => { try { await App().ResetPrompt(b.dataset.preset); renderPromptLib(); } catch (e) { toast(cleanErr(e), "err"); } });
  box.querySelectorAll("[data-pdel]").forEach((b) => b.onclick = () => {
    const p = byId[b.dataset.pdel];
    openModal(p.builtin ? "Masquer ce prompt ?" : "Supprimer ce prompt ?", `<div class="field"><div class="callout">« ${esc(p.title)} »${p.builtin ? " sera masqué (réversible via « réinitialiser » plus tard)." : " sera supprimé."}</div></div>`, async () => {
      try { await App().DeletePrompt(p.id); closeModal(); renderPromptLib(); } catch (e) { toast(cleanErr(e), "err"); }
    });
  });
}

// promptEditModal : créer (p=null) ou modifier un prompt. Un prompt par défaut modifié devient une
// surcharge (conservée) sans toucher l'original livré.
function promptEditModal(p) {
  const isNew = !p;
  openModal(isNew ? "Nouveau prompt" : "Modifier le prompt", `<div class="field">
    <label>Titre<input id="pe-title" class="inp" value="${esc(p ? p.title : "")}" /></label>
    <label>Prompt<textarea id="pe-body" rows="6" style="width:100%;resize:vertical">${esc(p ? p.body : "")}</textarea></label>
    <div class="inline-error hidden" id="pe-err"></div></div>`, async () => {
    const er = el("pe-err");
    try { await App().SavePrompt(p ? p.id : "", el("pe-title").value, el("pe-body").value); closeModal(); renderPromptLib(); }
    catch (e) { if (er) { er.textContent = cleanErr(e); er.classList.remove("hidden"); } }
  });
  el("modal-ok").textContent = "Enregistrer";
}

async function renderSettings() {
  const theme = readTheme();
  const navItems = [["secu", "🔒 Compte & sécurité"], ["conn", "🔗 Connexions"], ["app", "🎨 Apparence"], ["ia", "🤖 Assistant IA"], ["diag", "🩺 Diagnostic"], ["ver", "🏷️ Versions"], ["lic", "⚖️ Licence"]];
  let setTab = "secu";
  try { setTab = localStorage.getItem("situlab.settab") || "secu"; } catch (_) {}
  el("view").innerHTML = `
    <div class="view-head"><div class="view-title">Réglages</div></div>
    <div class="set-layout">
      <nav class="set-nav">${navItems.map(([k, l]) => `<button class="set-navbtn" data-settab="${k}">${l}</button>`).join("")}</nav>
      <div class="set-panels">

    <section class="set-sec set-panel" data-panel="secu"><div class="set-sec-h">🔒 Compte &amp; sécurité</div>
      <div class="set-sec-d">Votre PIN déverrouille vos accès (jetons chiffrés sur ce poste) et confirme chaque envoi vers le serveur. Personne ne peut lire vos accès sans lui.</div>
      <div class="set-row"><div class="set-row-l"><b id="pin-state"></b></div>
        <div class="set-row-c"><button class="btn btn-ghost btn-sm" id="set-pin"></button><button class="lnk" id="pin-forgot">PIN oublié ?</button></div></div>
      <div class="set-row"><div class="set-row-l">Session <span id="sess-state" class="muted"></span></div>
        <div class="set-row-c"><button class="btn btn-ghost btn-sm" id="sess-btn"></button></div></div>
      <div class="set-row"><div class="set-row-l">Verrouillage automatique</div>
        <div class="set-row-c"><select id="lock-delay" class="select">
          <option value="10">après 10 minutes</option><option value="15">après 15 minutes</option>
          <option value="30">après 30 minutes</option><option value="60">après 1 heure</option>
          <option value="-1">jamais</option></select></div></div>
      <label class="set-row toggle"><div class="set-row-l">Verrouiller après chaque écriture <small>reverrouille dès qu'une modification est confirmée</small></div>
        <span class="switch"><input id="lock-write" type="checkbox" /><span class="track"></span></span></label>
      <label class="set-row toggle"><div class="set-row-l">Mode lecture seule <small>consulter sans risque : bloque tout envoi, suppression et modification</small></div>
        <span class="switch"><input id="read-only" type="checkbox" /><span class="track"></span></span></label>
      <div class="set-note">Gestion des jetons d'accès : onglet <b>Tokens</b>.</div>
    </section>

    <section class="set-sec set-panel" data-panel="conn"><div class="set-sec-h">🔗 Connexions</div>
      <div class="set-sec-d">Vos serveurs git (Forgejo/Gitea, GitHub). ★ = connexion par défaut.</div>
      <div id="inst-list"></div>
      <button class="btn btn-ghost btn-sm" id="add-inst" style="margin-top:10px">＋ Ajouter une connexion</button>
    </section>

    <section class="set-sec set-panel" data-panel="app"><div class="set-sec-h">🎨 Apparence &amp; affichage</div>
      <div class="set-row"><div class="set-row-l">Thème</div>
        <div class="set-row-c"><div class="seg" role="group" aria-label="Thème">
          <button data-th="auto" class="${theme === "auto" ? "on" : ""}">Auto</button>
          <button data-th="light" class="${theme === "light" ? "on" : ""}">Clair</button>
          <button data-th="dark" class="${theme === "dark" ? "on" : ""}">Sombre</button>
        </div></div></div>
      <label class="set-row toggle"><div class="set-row-l">Mode avancé <small>révèle les étapes git séparées, la matrice des branches, les outils git locaux et les réglages de dépôt</small></div>
        <span class="switch"><input id="adv-mode" type="checkbox" /><span class="track"></span></span></label>
    </section>

    <section class="set-sec set-panel" data-panel="ia"><div class="set-sec-h">🤖 Assistant IA</div>
      <div class="set-sec-d">SituLab dépose un fichier <code>AGENTS.md</code> près de vos dépôts : mode d'emploi pour votre IA + votre <b>fiche de collaboration</b>. Votre IA la lit au démarrage et peut l'enrichir avec vous. <button class="lnk" id="agents-view">Consulter AGENTS.md</button></div>
      <label class="set-row-l" style="display:block">Notes de collaboration (reprises dans AGENTS.md)
        <textarea id="agent-notes" rows="4" placeholder="Ex. : réponds en français ; propose toujours un plan avant d'agir ; mes dépôts de prod sont sur l'organisation apcom…" style="width:100%;margin-top:6px;resize:vertical"></textarea></label>
      <div class="set-row-c" style="margin-top:10px;justify-content:flex-start">
        <button class="btn btn-primary btn-sm" id="agent-save">Enregistrer</button>
        <button class="btn btn-ghost btn-sm" id="agent-deposit">Déposer AGENTS.md dans un dossier…</button>
        <button class="btn btn-ghost btn-sm" id="agent-config">Configurer mon agent IA</button>
      </div>
      <div class="plib-h"><b>Bibliothèque de prompts</b> <span class="muted">— des messages prêts à coller dans votre IA. Les vôtres sont conservés lors des mises à jour.</span></div>
      <div id="prompt-lib"></div>
      <button class="btn btn-ghost btn-sm" id="prompt-add" style="margin-top:8px">＋ Ajouter un prompt</button>
    </section>

    <section class="set-sec set-panel" data-panel="diag"><div class="set-sec-h">🩺 Diagnostic</div>
      <div class="set-sec-d">L'état de SituLab sur ce poste en un coup d'œil : git, PIN, session, connexions, coffre, signature, et les accès git qui échappent à la confirmation PIN.</div>
      <div id="diag-creds"><div class="muted">Vérification…</div></div>
    </section>

    <section class="set-sec set-panel" data-panel="ver"><div class="set-sec-h">🏷️ Versions</div>
      <div class="set-row"><div class="set-row-l">Version installée <b id="app-ver">…</b></div>
        <div class="set-row-c"><button class="btn btn-ghost btn-sm" id="ver-check">Vérifier les mises à jour ↗</button></div></div>
      <div class="set-row"><div class="set-row-l">Téléchargements (votre OS)</div>
        <div class="set-row-c"><button class="btn btn-ghost btn-sm" id="ver-site">situememoring.com ↗</button><button class="btn btn-ghost btn-sm" id="ver-gh">GitHub ↗</button></div></div>
      <div class="set-note">Les installeurs et l'historique complet des versions sont publiés sur situememoring.com (lecture en direct des Releases GitHub).</div>
    </section>

    <section class="set-sec set-panel" data-panel="lic"><div class="set-sec-h">⚖️ Licence</div>
      <div class="set-sec-d">SituLab — logiciel libre sous licence <b>Apache-2.0</b>. © 2026 <b>APCOM Solutions SA</b>. La marque « SituLab » n'est pas couverte par la licence.</div>
      <div class="set-row-c" style="justify-content:flex-start"><button class="btn btn-ghost btn-sm" id="lic-show">Texte de la licence</button><button class="btn btn-ghost btn-sm" id="notice-show">NOTICE</button></div>
    </section>

    <section class="set-sec danger-zone set-panel" data-panel="secu"><div class="dz-h">⚠ Zone sensible — effacer mes données de ce poste</div>
      <p class="sub">Supprime définitivement vos connexions, votre trousseau chiffré, vos réglages et les jetons du coffre Windows sur cette machine. Pour repartir à zéro ou céder le poste. <b>Irréversible.</b></p>
      <button class="btn btn-ghost btn-sm btn-danger" id="purge-data">Effacer mes données…</button>
    </section>

      </div>
    </div>`;

  // Nav à onglets : affiche un panneau à la fois (le DOM garde tous les panneaux → le câblage par id
  // ci-dessous reste valable). Onglet mémorisé par poste (confort local).
  const showTab = (k) => {
    document.querySelectorAll(".set-panel").forEach((p) => p.classList.toggle("hidden", p.dataset.panel !== k));
    document.querySelectorAll(".set-navbtn").forEach((b) => b.classList.toggle("active", b.dataset.settab === k));
    try { localStorage.setItem("situlab.settab", k); } catch (_) {}
  };
  document.querySelectorAll(".set-navbtn").forEach((b) => b.onclick = () => showTab(b.dataset.settab));
  showTab(setTab);
  const agv = el("agents-view");
  if (agv) agv.onclick = async () => { try { infoModal("AGENTS.md", `<div class="md-render" style="max-height:60vh;overflow:auto">${mdToHTML(await App().Doc())}</div>`); } catch (_) {} };
  renderPromptLib();
  if (el("prompt-add")) el("prompt-add").onclick = () => promptEditModal(null);

  document.querySelectorAll("[data-th]").forEach((b) => b.onclick = () => { writeTheme(b.dataset.th); applyTheme(b.dataset.th); renderSettings(); });
  const adv = el("adv-mode");
  if (adv) {
    adv.checked = !!state.advanced;
    adv.onchange = () => { setAdvanced(adv.checked); toast(adv.checked ? "Mode avancé activé" : "Mode simplifié activé", "ok"); };
  }

  // Section Versions : version installée + liens (téléchargement/MAJ sur le site, lecture en direct des
  // Releases GitHub). La détection fine de MAJ se fait côté site (connect-src 'self' interdit le fetch
  // d'une API externe depuis la fenêtre) ; le bouton ouvre donc la page des versions dans le navigateur.
  const openURL = (u) => safeOpenURL(u);
  try { el("app-ver").textContent = await App().AppVersion(); } catch (_) { el("app-ver").textContent = "?"; }
  if (el("ver-check")) el("ver-check").onclick = () => openURL("https://situememoring.com/#telecharger");
  if (el("ver-site")) el("ver-site").onclick = () => openURL("https://situememoring.com/");
  if (el("ver-gh")) el("ver-gh").onclick = () => openURL("https://situememoring.com/#telecharger");
  // Section Licence : texte Apache-2.0 + NOTICE embarqués (go:embed), affichés en modale.
  if (el("lic-show")) el("lic-show").onclick = async () => { try { infoModal("Licence Apache-2.0", `<pre class="filebox" style="user-select:text;white-space:pre-wrap">${esc(await App().License())}</pre>`); } catch (_) {} };
  if (el("notice-show")) el("notice-show").onclick = async () => { try { infoModal("NOTICE", `<pre class="filebox" style="user-select:text;white-space:pre-wrap">${esc(await App().Notice())}</pre>`); } catch (_) {} };

  try {
    const ins = await App().GetInstances();
    state._settingsInstances = ins;
    await refreshDisconnected();
    // Ping réel de chaque connexion : détecte aussi un jeton présent mais REFUSÉ (révoqué en ligne),
    // que la liste « sans token » ne voit pas. "auth" = à reconnecter ; "net" = serveur injoignable.
    const health = {};
    await Promise.all(ins.map(async (i) => { health[i.name] = isDisconnected(i.name) ? "auth" : await pingInstance(i.name); }));
    el("inst-list").innerHTML = ins.map((i) => {
      const h = health[i.name] || "ok", dc = h !== "ok";
      const prod = i.role === "production";
      const badge = `<span class="role-badge ${prod ? "role-production" : "role-atelier"}">${prod ? "VITRINE" : "ATELIER"}</span>`;
      const pill = h === "auth" ? `<span class="pill disc-pill">déconnectée</span>` : (h === "net" ? `<span class="pill disc-pill">hors ligne</span>` : "");
      const reconBtn = dc ? `<button class="btn btn-primary btn-sm" data-recon="${esc(i.name)}">Reconnecter</button>` : "";
      return `<div class="srow${dc ? " srow-disc" : ""}"><div>${i.default ? "★ " : ""}<b>${esc(i.name)}</b> ${badge} <span class="muted">${esc(i.url)}</span> ${pill}</div>
      <div class="srow-act">${reconBtn}<button class="lnk" data-pol="${esc(i.name)}">rôle &amp; PIN</button>${i.default ? "" : `<button class="lnk" data-def="${esc(i.name)}">défaut</button>`}<button class="lnk danger" data-del="${esc(i.name)}">supprimer</button></div></div>`;
    }).join("");
    el("inst-list").onclick = async (ev) => {
      const def = ev.target.closest("[data-def]"), del = ev.target.closest("[data-del]"), recon = ev.target.closest("[data-recon]"), pol = ev.target.closest("[data-pol]");
      if (pol) return connPolicyModal(pol.dataset.pol);
      if (recon) return reconnectModal(recon.dataset.recon);
      if (def) { try { await App().SetDefaultInstance(def.dataset.def); toast("Instance par défaut mise à jour", "ok"); renderSettings(); } catch (e) { toast(cleanErr(e), "err"); } }
      if (del) {
        const nm = del.dataset.del;
        openModal("Supprimer cette connexion ?", `<div class="field"><div class="callout warn">Vous retirez « ${esc(nm)} » et l'accès chiffré que SituLab garde pour ce serveur. Vos dépôts et fichiers sur le poste ne sont pas touchés, mais pour revenir il faudra vous reconnecter (identifiant + mot de passe + code 2FA).</div></div>`, async () => {
          try { await App().RemoveInstance(nm); closeModal(); toast("Connexion supprimée", "ok"); location.reload(); } catch (e) { toast(cleanErr(e), "err"); }
        });
        el("modal-ok").textContent = "Supprimer"; el("modal-ok").classList.add("danger-btn");
      }
    };
  } catch (e) { el("inst-list").innerHTML = surfaceError("Lecture impossible", cleanErr(e), null); }
  el("add-inst").onclick = addInstanceModal;

  const hasPin = await App().PinSet().catch(() => false);
  el("pin-state").textContent = hasPin ? "Un PIN est défini." : "Aucun PIN défini pour le moment.";
  el("set-pin").textContent = hasPin ? "Changer le PIN" : "Créer un PIN";
  el("set-pin").onclick = setPinModal;
  el("pin-forgot").onclick = pinForgotModal;
  el("pin-forgot").classList.toggle("hidden", !hasPin);
  // État de session + bouton verrouiller/déverrouiller.
  const unlocked = hasPin && (await App().Unlocked().catch(() => false));
  el("sess-state").textContent = !hasPin ? "" : unlocked ? "● Session déverrouillée" : "○ Session verrouillée";
  const sb = el("sess-btn");
  sb.classList.toggle("hidden", !hasPin);
  sb.textContent = unlocked ? "Verrouiller maintenant" : "Déverrouiller";
  sb.onclick = async () => {
    if (unlocked) { try { await App().Lock(); toast("Session verrouillée", "ok"); renderSettings(); } catch (e) { toast(cleanErr(e), "err"); } }
    else unlockScreen(() => init());
  };
  // Délai d'auto-verrouillage réglable.
  try {
    const d = await App().LockDelay();
    const ls = el("lock-delay");
    if (ls) {
      ls.value = String(d === 0 ? 15 : d);
      ls.onchange = async () => { try { await App().SetLockDelay(parseInt(ls.value, 10)); toast("Délai de verrouillage mis à jour", "ok"); } catch (e) { toast(cleanErr(e), "err"); } };
    }
    const lw = el("lock-write");
    if (lw) {
      lw.checked = await App().LockAfterWrite().catch(() => false);
      lw.onchange = async () => { try { await App().SetLockAfterWrite(lw.checked); toast(lw.checked ? "Verrouillage après écriture activé" : "Verrouillage après écriture désactivé", "ok"); } catch (e) { toast(cleanErr(e), "err"); lw.checked = !lw.checked; } };
    }
    const ro = el("read-only");
    if (ro) {
      ro.checked = !!state.readOnly;
      ro.onchange = async () => {
        try {
          await App().SetReadOnly(ro.checked);
          state.readOnly = ro.checked;
          updateReadOnlyBanner();
          toast(ro.checked ? "Mode lecture seule activé" : "Mode lecture seule désactivé", "ok");
        } catch (e) { toast(cleanErr(e), "err"); ro.checked = !ro.checked; }
      };
    }
    const pd = el("purge-data");
    if (pd) pd.onclick = purgeDataModal;
    // Assistant IA : fiche de collaboration éditable + dépôt + configuration.
    const an = el("agent-notes");
    if (an) {
      App().AgentNotes().then((v) => { an.value = v || ""; }).catch(() => {});
      el("agent-save").onclick = async () => { try { await App().SetAgentNotes(an.value); toast("Fiche de collaboration enregistrée", "ok"); } catch (e) { toast(cleanErr(e), "err"); } };
      el("agent-deposit").onclick = installDirectives;
      el("agent-config").onclick = aiAgentModal;
    }
  } catch (e) {}
  renderDiagnostic();
}

// renderDiagnostic : diagnostic VIVANT (1.9.4) — une liste de contrôles lisibles (git, PIN, session,
// connexions, coffre OS, signature, autres accès git) avec statut, explication et action quand utile.
async function renderDiagnostic() {
  const dc = el("diag-creds"); if (!dc) return;
  dc.innerHTML = `<div class="skeleton"></div>`;
  let doc;
  try { doc = await App().Doctor(); } catch (e) { dc.innerHTML = `<div class="callout warn">${esc(cleanErr(e))}</div>`; return; }
  const row = (state, label, detail, action) => `<div class="diag-row diag-${state}"><span class="diag-ico">${state === "ok" ? "✅" : state === "warn" ? "⚠️" : "ℹ️"}</span><div class="diag-txt"><b>${esc(label)}</b>${detail ? `<div class="muted">${detail}</div>` : ""}</div>${action || ""}</div>`;
  const rows = [];
  rows.push(doc.git_available
    ? row("ok", "Git installé", esc(doc.git_version || ""))
    : row("warn", "Git absent", "Clone, synchro et versions locales sont indisponibles.", `<button class="btn btn-ghost btn-sm" data-openurl="https://git-scm.com/downloads">Installer git ↗</button>`));
  rows.push(doc.pin_set ? row("ok", "PIN configuré", "Vos jetons sont chiffrés par votre PIN.") : row("warn", "Aucun PIN", "Définissez-en un dans Compte & sécurité."));
  rows.push(doc.unlocked ? row("ok", "Session déverrouillée", "SituLab peut lire vos accès.") : row("info", "Session verrouillée", "Déverrouillez avec votre PIN pour agir."));
  rows.push(row("info", "Connexions", `${doc.instances} serveur(s) configuré(s).`));
  rows.push(doc.vault_residual ? row("warn", "Coffre Windows", `${doc.vault_residual} jeton(s) encore dans l'ancien coffre OS — ils seront migrés automatiquement au prochain déverrouillage.`) : row("ok", "Coffre Windows", "Aucun jeton résiduel."));
  let swActive = false; try { swActive = await App().AttestSoftwareActive(); } catch (_) {}
  rows.push(doc.sign_test_root
    ? row("warn", "Signature de garantie", "Racine de TEST embarquée — la vraie racine de gouvernance n'est pas installée (les ✅ ne sont pas vérifiables réellement).")
    : swActive
      ? row("ok", "Signature logicielle", "Active — chaque commit via SituLab porte la garantie « SituLab » (signature du logiciel, pas par utilisateur).")
      : row("info", "Signature logicielle", "Pas encore activée sur ce binaire — elle sera mintée par la gouvernance (identité unique « SituLab »)."));
  rows.push((doc.other_git_creds || []).length
    ? row("warn", "Autres accès git sur ce poste", `${doc.other_git_creds.map(esc).join(", ")}. Ces identifiants (clé SSH, credential helper) permettent un accès git <b>indépendant</b> de SituLab, hors confirmation PIN. Pour un cloisonnement strict, faites tourner vos agents sous un autre compte / bac à sable.`)
    : row("ok", "Autres accès git", "Aucun identifiant git concurrent détecté — la confirmation PIN protège bien."));
  dc.innerHTML = `<div class="diag-list">${rows.join("")}</div>`;
}

// pinForgotModal : explication HONNÊTE (le PIN est irrécupérable) + action « repartir de zéro ».
function pinForgotModal() {
  openModal("PIN oublié ?", `<div class="field">
    <p class="sub">Pas de souci. Vous allez simplement <b>choisir un nouveau PIN</b>. Vos instances seront alors <b>déconnectées</b> : il suffira de les <b>reconnecter</b> (identifiant + mot de passe + code 2FA) — SituLab vous guidera. Vos dépôts et fichiers sur ce poste ne bougent pas.</p>
    <div class="inline-error hidden" id="pf-err"></div></div>`, async () => {
    try { await App().ResetKeyring(); location.reload(); }
    catch (e) { const er = el("pf-err"); er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  el("modal-ok").textContent = "Choisir un nouveau PIN";
}

/* ---------------- Aide ---------------- */
/* ---------------- Comprendre git (page pedagogique, nav laterale) ---------------- */
// Petits visuels SVG inline (stroke = currentColor -> suit couleur/theme), CSP-safe (pas de handler inline).
const GV = {
  chain: `<svg class="gviz" viewBox="0 0 170 34" aria-hidden="true"><line x1="18" y1="17" x2="152" y2="17"/><circle cx="18" cy="17" r="6"/><circle cx="85" cy="17" r="6"/><circle class="gv-ok" cx="152" cy="17" r="6"/></svg>`,
  fork: `<svg class="gviz" viewBox="0 0 170 46" aria-hidden="true"><path d="M16 31h40c14 0 14-16 28-16h66"/><line x1="56" y1="31" x2="150" y2="31"/><circle cx="16" cy="31" r="5"/><circle class="gv-accent" cx="150" cy="15" r="5"/><circle cx="150" cy="31" r="5"/></svg>`,
  merge: `<svg class="gviz" viewBox="0 0 170 46" aria-hidden="true"><path d="M16 15h66c14 0 14 16 28 16h44"/><line x1="16" y1="31" x2="154" y2="31"/><circle class="gv-accent" cx="16" cy="15" r="5"/><circle cx="16" cy="31" r="5"/><circle class="gv-ok" cx="154" cy="31" r="5"/></svg>`,
  sync: `<svg class="gviz" viewBox="0 0 190 48" aria-hidden="true"><rect x="6" y="14" width="54" height="20" rx="4"/><rect class="gv-accent" x="130" y="14" width="54" height="20" rx="4"/><line x1="62" y1="20" x2="124" y2="20"/><polyline points="119,16 124,20 119,24"/><line x1="128" y1="28" x2="66" y2="28"/><polyline points="71,24 66,28 71,32"/></svg>`,
  pipeline: `<svg class="gviz" viewBox="0 0 220 40" aria-hidden="true"><rect class="gv-ok" x="4" y="10" width="56" height="20" rx="4"/><rect class="gv-accent" x="82" y="10" width="56" height="20" rx="4"/><rect class="gv-vitrine" x="160" y="10" width="56" height="20" rx="4"/><line x1="62" y1="20" x2="80" y2="20"/><polyline points="75,16 80,20 75,24"/><line x1="140" y1="20" x2="158" y2="20"/><polyline points="153,16 158,20 153,24"/></svg>`,
  undo: `<svg class="gviz" viewBox="0 0 120 40" aria-hidden="true"><path d="M44 12a16 16 0 1 1-14 8"/><polyline points="24 10 30 20 42 16"/></svg>`,
  stash: `<svg class="gviz" viewBox="0 0 150 44" aria-hidden="true"><line x1="14" y1="22" x2="92" y2="22"/><circle cx="14" cy="22" r="5"/><circle cx="54" cy="22" r="5"/><rect class="gv-accent" x="104" y="12" width="36" height="20" rx="4"/><line x1="92" y1="22" x2="102" y2="22" stroke-dasharray="3 3"/></svg>`,
  cloud: `<svg class="gviz" viewBox="0 0 190 46" aria-hidden="true"><path d="M26 28a9 9 0 0 1 2-17 12 12 0 0 1 23 2 8 8 0 0 1-2 15z"/><rect class="gv-ok" x="128" y="12" width="52" height="22" rx="4"/><line x1="58" y1="23" x2="124" y2="23"/><polyline points="119,19 124,23 119,27"/></svg>`,
  seal: `<svg class="gviz" viewBox="0 0 40 40" aria-hidden="true"><path class="gv-ok" d="M20 5l11 3.6v8.3c0 7.2-4.7 12.4-11 14.5-6.3-2.1-11-7.3-11-14.5V8.6z"/><path class="gv-ok" d="M14 20l4 4 7.5-8"/></svg>`,
};
// Contenu : groupe -> cartes { cmd?, t?, what (HTML de confiance, statique), in? (renvoi SituLab), viz? }.
const GIT_GUIDE = [
  { k: "bases", label: "🧭 Les bases", intro: "Enregistrer son travail, pas à pas. Ce sont les gestes de tous les jours.", items: [
    { cmd: "git clone", what: "Copier un dépôt en ligne sur votre poste pour y travailler.", in: "Dépôts → « Lier un dépôt local », ou « Cloner ici » sur une carte de projet.", viz: GV.cloud },
    { cmd: "git status", what: "Voir, d'un coup d'œil, ce qui a changé et où vous en êtes.", in: "L'état de chaque carte de dépôt (« à jour » / « modifications en cours »)." },
    { cmd: "git add", t: "indexer", what: "Choisir les modifications qui entreront dans le prochain enregistrement.", in: "Fait pour vous : « Enregistrer » prend tout l'arbre de travail." },
    { cmd: "git commit", what: "Figer un <b>point de sauvegarde</b> local, avec un message.", in: "Bouton « Enregistrer », ou l'étape 2 de « Synchroniser ».", viz: GV.chain },
    { cmd: "git log", what: "Lister les enregistrements : qui a fait quoi, quand.", in: "Onglet <b>Historique</b> d'un dépôt — une timeline quoi / qui / quand.", viz: GV.chain },
    { cmd: "git diff", what: "Voir, ligne par ligne, ce qui a changé.", in: "« Voir les modifications » et le diff coloré de l'historique par fichier." },
    { cmd: "git show", what: "Le détail complet d'un enregistrement précis.", in: "Clic sur un commit ; historique par fichier." },
  ]},
  { k: "branch", label: "⎇ Branches", intro: "Travailler en parallèle sans se marcher dessus, puis réunir.", items: [
    { cmd: "git branch", what: "Créer / lister des <b>lignes de travail parallèles</b>.", in: "La matrice des branches (mode avancé) ; les branches du poste sur chaque carte.", viz: GV.fork },
    { cmd: "git switch", t: "changer de branche", what: "Basculer la branche active de votre dossier de travail.", in: "Outils git locaux (mode avancé)." },
    { cmd: "git merge", what: "Réunir une branche dans une autre.", in: "Panneau git des projets (en interface).", viz: GV.merge },
    { cmd: "git rebase", what: "Rejouer vos commits au sommet d'une autre branche (historique plus droit).", in: "Outils git locaux (mode avancé)." },
    { cmd: "git cherry-pick", what: "Reprendre un commit précis, et lui seul, sur la branche courante.", in: "Outils git locaux (mode avancé)." },
  ]},
  { k: "undo", label: "↩ Annuler & réparer", intro: "Revenir en arrière proprement. Certains gestes sont destructifs : SituLab les confirme.", items: [
    { cmd: "git revert", what: "Annuler un commit en créant son <b>inverse</b> — sûr, l'historique est préservé.", in: "Outils git locaux.", viz: GV.undo },
    { cmd: "git reset", what: "Déplacer la tête en arrière (soft / mixed / hard). <b>⚠ hard = destructif</b> (perte des modifs).", in: "Outils git locaux — le mode hard est confirmé." },
    { cmd: "git commit --amend", what: "Corriger le dernier enregistrement (message ou contenu).", in: "Outils git locaux." },
    { cmd: "git stash", what: "Mettre de côté un travail en cours, puis le reprendre plus tard.", in: "Outils git locaux (ranger / reprendre).", viz: GV.stash },
    { t: "Annuler les modifications", what: "Jeter les changements non encore enregistrés. <b>⚠ destructif</b>.", in: "« Annuler les modifications » — confirmation demandée." },
  ]},
  { k: "sync", label: "⇅ Synchroniser", intro: "Échanger avec le serveur : récupérer les nouveautés, envoyer son travail.", items: [
    { cmd: "git fetch", what: "Regarder les nouveautés du serveur <b>sans</b> toucher à votre travail.", in: "Fait en fond ; « vérifié à l'instant » sur la vue Dépôts.", viz: GV.sync },
    { cmd: "git pull", what: "Récupérer et intégrer les nouveautés (avance rapide, jamais de fusion surprise).", in: "Inclus dans « Synchroniser »." },
    { cmd: "git push", what: "Envoyer vos enregistrements en ligne.", in: "« Synchroniser » / « Envoyer » — sous PIN selon la connexion.", viz: GV.sync },
    { cmd: "git push --force-with-lease", what: "Republier après réécriture <b>sans</b> écraser le travail d'un autre apparu entre-temps.", in: "Outils git locaux — sous PIN." },
    { cmd: "git tag", t: "versions", what: "Étiqueter une version (ex. <code>v1.2.3</code>).", in: "Bloc « Versions » de l'onglet Historique." },
  ]},
  { k: "promote", label: "🚀 Promouvoir", intro: "Le modèle SituLab : on travaille à l'atelier, on publie une version propre en production.", items: [
    { t: "Les trois paliers", what: "<b>Local</b> (ce poste) → <b>Atelier</b> (le dépôt de travail, où l'IA peut être autonome) → <b>Vitrine</b> (la production, protégée).", viz: GV.pipeline, in: "Les 3 colonnes d'une carte de projet." },
    { t: "Promouvoir / repromouvoir", what: "Publier vers la production la version validée de l'atelier — en <b>un seul commit propre</b>, sous PIN.", in: "Bouton « Promouvoir » d'une carte de projet (fenêtre PIN violette « Production »)." },
  ]},
  { k: "copies", label: "🗂 Copies de travail", intro: "Deux façons d'avoir plusieurs dossiers de travail — utile quand plusieurs IA travaillent en parallèle.", items: [
    { t: "Clone", what: "Un <b>dossier indépendant</b>, avec son propre <code>.git</code>. Isolation totale : idéale pour faire travailler plusieurs IA en parallèle sans qu'elles se gênent.", in: "« Cloner ici » ; la vue Dépôts montre « N copies » quand il y en a plusieurs.", viz: GV.cloud },
    { t: "Worktree", what: "Plusieurs dossiers <b>greffés sur un seul</b> <code>.git</code>. Plus léger en disque, mais une branche ne peut être active que dans un seul worktree à la fois.", in: "Affiché comme « worktree » dans la liste des copies." },
    { t: "⚠ Piège Windows", what: "Ne jamais partager un dossier entre worktrees par une <b>jonction</b> : retirer le worktree peut vider le dossier d'origine.", in: "SituLab vous guide au moment de créer une copie." },
  ]},
  { k: "sign", label: "🛡 Signature", intro: "La garantie d'origine « SituLab approved » — qui a fait le commit, via SituLab.", items: [
    { t: "Le sceau « certified »", what: "Chaque commit créé via SituLab porte une <b>attestation d'origine immuable</b> (dans le commit lui-même), vérifiée localement contre la racine APCOM. Un agent ne peut pas la forger.", in: "Le <b>sceau vert</b> dans l'onglet Historique (infobulle « Signé via SituLab »).", viz: GV.seal },
  ]},
];

function renderGit() {
  let tab = "bases";
  try { tab = localStorage.getItem("situlab.gittab") || "bases"; } catch (_) {}
  const card = (it) => `<div class="git-card">
    <div class="git-card-h">${it.cmd ? `<code class="git-cmd">${esc(it.cmd)}</code>` : ""}${(it.t || !it.cmd) ? `<span class="git-card-t">${esc(it.t || "")}</span>` : ""}</div>
    ${it.viz ? `<div class="git-viz">${it.viz}</div>` : ""}
    <div class="git-what">${it.what}</div>
    ${it.in ? `<div class="git-in"><b>Dans SituLab :</b> ${it.in}</div>` : ""}
  </div>`;
  const panels = GIT_GUIDE.map((g) => `<section class="set-sec set-panel" data-panel="${g.k}">
    <div class="set-sec-h">${esc(g.label)}</div>
    ${g.intro ? `<div class="git-intro">${esc(g.intro)}</div>` : ""}
    <div class="git-grid">${g.items.map(card).join("")}</div>
  </section>`).join("");
  el("view").innerHTML = `
    <div class="view-head"><div class="view-title">Comprendre git</div><div class="view-sub">Le vocabulaire git en clair — et où chaque geste se trouve dans SituLab.</div></div>
    <div class="set-layout">
      <nav class="set-nav">${GIT_GUIDE.map((g) => `<button class="set-navbtn" data-gittab="${g.k}">${esc(g.label)}</button>`).join("")}</nav>
      <div class="set-panels">${panels}</div>
    </div>`;
  const showTab = (k) => {
    document.querySelectorAll("#view .set-panel").forEach((p) => p.classList.toggle("hidden", p.dataset.panel !== k));
    document.querySelectorAll("#view .set-navbtn").forEach((b) => b.classList.toggle("active", b.dataset.gittab === k));
    try { localStorage.setItem("situlab.gittab", k); } catch (_) {}
  };
  document.querySelectorAll("#view .set-navbtn").forEach((b) => b.onclick = () => showTab(b.dataset.gittab));
  if (!GIT_GUIDE.some((g) => g.k === tab)) tab = "bases";
  showTab(tab);
}

async function renderHelp() {
  let tab = "guide";
  try { tab = localStorage.getItem("situlab.helptab") || "guide"; } catch (_) {}
  el("view").innerHTML = `
    <div class="view-head"><div class="view-title">Aide</div><div class="view-sub">Documentation embarquée — pour vous comme pour vos agents.</div></div>
    <div class="set-layout">
      <nav class="set-nav">
        <button class="set-navbtn" data-helptab="guide">📖 Guide</button>
        <button class="set-navbtn" data-helptab="about">ℹ À propos</button>
      </nav>
      <div class="set-panels">
        <section class="set-sec set-panel" data-panel="guide"><div class="doc" id="doc"><div class="skeleton"></div></div></section>
        <section class="set-sec set-panel" data-panel="about">
          <div class="set-sec-h">À propos de SituLab</div>
          <div class="about">
            <div class="about-id">SituLab <span id="about-ver"></span></div>
            <div class="about-cp">© 2026 APCOM Solutions SA — Logiciel libre sous licence Apache 2.0.</div>
            <div class="about-acts" style="margin-top:14px">
              <button class="btn btn-primary btn-sm" id="replay-tour">↺ Rejouer la visite guidée</button>
              <button class="btn btn-ghost btn-sm" id="copy-prompt">🤖 Configurer mon agent IA</button>
            </div>
            <div class="about-acts" style="margin-top:10px">
              <button class="btn btn-ghost btn-sm" id="show-license">Licence (Apache 2.0)</button>
              <button class="btn btn-ghost btn-sm" id="show-notice">Avis &amp; marques (NOTICE)</button>
            </div>
          </div>
        </section>
      </div>
    </div>`;
  const showTab = (k) => {
    document.querySelectorAll("#view .set-panel").forEach((p) => p.classList.toggle("hidden", p.dataset.panel !== k));
    document.querySelectorAll("#view .set-navbtn").forEach((b) => b.classList.toggle("active", b.dataset.helptab === k));
    try { localStorage.setItem("situlab.helptab", k); } catch (_) {}
  };
  document.querySelectorAll("#view .set-navbtn").forEach((b) => b.onclick = () => showTab(b.dataset.helptab));
  showTab(tab === "about" ? "about" : "guide");
  try { el("doc").innerHTML = mdToHtml(await App().Doc()); }
  catch (e) { el("doc").innerHTML = surfaceError("Documentation indisponible", cleanErr(e), null); }
  try { el("about-ver").textContent = "v" + (await App().AppVersion()); } catch (e) {}
  el("show-license").onclick = async () => {
    try { infoModal("Licence — Apache 2.0", `<pre class="license-text">${esc(await App().License())}</pre>`); }
    catch (e) { infoModal("Licence", `<div class="muted">${esc(cleanErr(e))}</div>`); }
  };
  el("show-notice").onclick = async () => {
    try { infoModal("Avis de droits — NOTICE", `<pre class="license-text">${esc(await App().Notice())}</pre>`); }
    catch (e) { infoModal("NOTICE", `<div class="muted">${esc(cleanErr(e))}</div>`); }
  };
  el("replay-tour").onclick = () => { navTo("repos"); setTimeout(() => startTour(true), 350); };
  el("copy-prompt").onclick = () => aiAgentModal();
}

/* ---------------- Visite guidée (coachmarks) ---------------- */
const TOUR_KEY = "situlab.tourDone.v1";
let tourState = null;

// maybeAutoTour lance la visite une seule fois, après la toute première connexion.
function maybeAutoTour() {
  let done = false;
  try { done = localStorage.getItem(TOUR_KEY) === "1"; } catch (e) {}
  if (done) return;
  setTimeout(() => startTour(false), 650); // laisser le tableau de bord se peindre
}

// buildTourSteps construit la séquence selon le contexte (cible absente → étape centrée).
function buildTourSteps() {
  const steps = [
    { ic: "👋", title: "Bienvenue", body: "Un tour rapide de SituLab, en quelques étapes. Vous pouvez passer à tout moment." },
    { ic: "▦", sel: "#repo-list", title: "Votre tableau de bord", body: "Une carte par dépôt. Cliquez l'en-tête d'une carte pour déplier l'état de chaque branche : version en ligne, version sur ce poste, et l'écart entre les deux." },
  ];
  if (el("instance-select") && (state.instances || []).length > 1) {
    steps.push({ ic: "🔀", sel: "#instance-select", title: "Vos connexions", body: "Basculez entre vos serveurs ici — Forgejo/Gitea ou GitHub." });
  }
  if (el("new-repo")) steps.push({ ic: "＋", sel: "#new-repo", title: "Créer un dépôt", body: "Créez un dépôt en ligne en un clic, directement depuis SituLab." });
  if (!curIsGitHub()) {
    const t = document.querySelector('.nav-item[data-view="tokens"]');
    if (t && !t.classList.contains("hidden")) steps.push({ ic: "⚿", sel: '.nav-item[data-view="tokens"]', title: "Un accès pour votre IA", body: "Dans Tokens, créez un jeton dédié à votre assistant IA — avec juste les droits utiles." });
  }
  steps.push({ ic: "?", sel: '.nav-item[data-view="help"]', title: "Aide & visite", body: "Toute la documentation est ici. Vous pourrez rejouer cette visite à tout moment depuis cette page." });
  // Dernière étape : faire connaissance avec l'agent IA de l'utilisateur (ouvre la modale dédiée).
  steps.push({ ic: "🤖", title: "Votre assistant IA", body: "Dernière chose : présentons SituLab à votre agent IA. Terminez pour choisir votre assistant et obtenir, sur mesure, son message de première connexion.", ai: true });
  return steps;
}

// startTour démarre/rejoue la visite. force=true ignore le drapeau « déjà vue ».
function startTour(force) {
  if (tourState) endTour(); // pas de double visite
  if (!force) {
    let done = false;
    try { done = localStorage.getItem(TOUR_KEY) === "1"; } catch (e) {}
    if (done) return;
  }
  const steps = buildTourSteps();
  if (!steps.length) return;
  const ov = document.createElement("div");
  ov.id = "tour";
  ov.innerHTML = `<div id="tour-hole"></div>
    <div id="tour-tip" role="dialog" aria-live="polite">
      <div id="tour-head"><span id="tour-icon" aria-hidden="true"></span><div id="tour-title"></div></div>
      <div id="tour-body"></div>
      <div id="tour-foot">
        <span id="tour-count" class="tour-dots" aria-hidden="true"></span>
        <span class="tour-btns">
          <button class="btn btn-ghost btn-sm" id="tour-skip">Passer</button>
          <button class="btn btn-primary btn-sm" id="tour-next">Suivant</button>
        </span>
      </div>
    </div>`;
  document.body.appendChild(ov);
  tourState = { steps, i: 0, ov, onKey: null, onResize: null };
  el("tour-skip").onclick = () => endTour(true);
  el("tour-next").onclick = () => tourNext();
  ov.addEventListener("click", (e) => { if (e.target === ov) {} }); // clic sur le voile : neutre (ne ferme pas par accident)
  tourState.onKey = (e) => { if (e.key === "Escape") endTour(true); else if (e.key === "Enter") tourNext(); };
  tourState.onResize = () => positionTour();
  document.addEventListener("keydown", tourState.onKey);
  window.addEventListener("resize", tourState.onResize);
  window.addEventListener("scroll", tourState.onResize, true);
  showTourStep(0);
}

function tourNext() {
  if (!tourState) return;
  if (tourState.i >= tourState.steps.length - 1) {
    const last = tourState.steps[tourState.steps.length - 1];
    endTour(true);
    if (last && last.ai) setTimeout(aiAgentModal, 150); // la dernière étape enchaîne sur l'onboarding IA
    return;
  }
  showTourStep(tourState.i + 1);
}

function showTourStep(i) {
  tourState.i = i;
  const s = tourState.steps[i], last = i === tourState.steps.length - 1, n = tourState.steps.length;
  el("tour-icon").textContent = s.ic || "•";
  el("tour-title").textContent = s.title;
  el("tour-body").textContent = s.body;
  // Pastilles de progression (● pour l'étape courante/passées, ○ pour les suivantes).
  el("tour-count").innerHTML = Array.from({ length: n }, (_, k) => `<span class="tour-dot${k <= i ? " on" : ""}"></span>`).join("");
  el("tour-next").textContent = last ? (s.ai ? "Terminer et configurer mon IA" : "Terminer") : "Suivant";
  positionTour();
}

// positionTour place le spotlight sur la cible et l'infobulle à proximité (ou centrée).
function positionTour() {
  if (!tourState) return;
  const s = tourState.steps[tourState.i];
  const hole = el("tour-hole"), tip = el("tour-tip");
  const target = s.sel ? document.querySelector(s.sel) : null;
  if (target) {
    const r = target.getBoundingClientRect();
    const pad = 6;
    hole.style.display = "block";
    hole.style.left = (r.left - pad) + "px";
    hole.style.top = (r.top - pad) + "px";
    hole.style.width = (r.width + pad * 2) + "px";
    hole.style.height = (r.height + pad * 2) + "px";
    // Infobulle : sous la cible si place, sinon au-dessus ; à droite si la cible est étroite (sidebar).
    tip.style.visibility = "hidden"; tip.style.left = "0px"; tip.style.top = "0px";
    const tw = tip.offsetWidth, th = tip.offsetHeight, gap = 12, vw = window.innerWidth, vh = window.innerHeight;
    let left, top;
    if (r.right + gap + tw < vw) { left = r.right + gap; top = r.top; }      // à droite
    else if (r.bottom + gap + th < vh) { left = r.left; top = r.bottom + gap; } // dessous
    else { left = r.left; top = r.top - th - gap; }                           // dessus
    left = Math.max(16, Math.min(left, vw - tw - 16));
    top = Math.max(16, Math.min(top, vh - th - 16));
    tip.style.left = left + "px"; tip.style.top = top + "px";
    tip.style.visibility = "visible";
  } else {
    hole.style.display = "none";
    tip.style.visibility = "hidden";
    tip.style.left = "50%"; tip.style.top = "50%"; tip.style.transform = "translate(-50%,-50%)";
    tip.style.visibility = "visible";
    return;
  }
  tip.style.transform = "none";
}

function endTour(markDone) {
  if (!tourState) return;
  document.removeEventListener("keydown", tourState.onKey);
  window.removeEventListener("resize", tourState.onResize);
  window.removeEventListener("scroll", tourState.onResize, true);
  tourState.ov.remove();
  tourState = null;
  if (markDone) { try { localStorage.setItem(TOUR_KEY, "1"); } catch (e) {} }
}

/* ---------------- Modales ---------------- */
let modalOk = null, modalCancel = null;
function openModal(title, bodyHtml, onOk) {
  const m = el("modal");
  el("modal-body").innerHTML = `<h3 id="modal-title">${esc(title)}</h3>${bodyHtml}`;
  m.classList.remove("hidden");
  // Réinitialise le bouton Valider (un appelant précédent a pu le renommer / le colorer).
  const ok = el("modal-ok");
  ok.textContent = "Valider"; ok.classList.remove("danger-btn", "hidden"); ok.disabled = false;
  el("modal-cancel").textContent = "Annuler"; el("modal-cancel").classList.remove("hidden"); // un appelant précédent a pu le renommer/masquer
  // Enveloppe onOk pour DÉSACTIVER le bouton pendant l'appel : empêche la double soumission
  // (double-clic -> deux écritures / deux tokens). Ré-activé si la modale reste ouverte.
  modalOk = onOk ? async () => {
    if (ok.disabled) return;
    ok.disabled = true;
    try { await onOk(); } finally { if (!m.classList.contains("hidden")) ok.disabled = false; }
  } : null;
  modalCancel = closeModal;
  ok.onclick = () => modalOk && modalOk();
  el("modal-cancel").onclick = () => (modalCancel || closeModal)();
  m.onclick = (ev) => { if (ev.target === m) (modalCancel || closeModal)(); };
  addRevealToggle(el("modal-body"));
  const first = m.querySelector("input"); if (first) first.focus();
}
function closeModal() { el("modal").classList.add("hidden"); modalOk = null; modalCancel = null; }

// pinConfirm : 2e étape d'une écriture — l'utilisateur a saisi ses données, on lui demande
// maintenant SON PIN SituLab pour confirmer l'action. onPin(pin) exécute l'action ; en cas
// d'erreur, la saisie du PIN reste affichée. Validation auto dès les 6 chiffres.
function pinConfirm(reason, onPin) {
  // Lecture seule (v1.4) : court-circuiter AVANT la saisie du PIN — le cœur refuserait de toute
  // façon. Couvre d'un seul point toutes les mutations confirmées par PIN (suppression, admin,
  // jetons, issues/PR, création…).
  if (state.readOnly) { toast("Mode lecture seule actif — désactivez-le dans Réglages", "err"); return; }
  let ci;
  openModal("Confirmer avec votre PIN", `<div class="field">
    <p class="sub">${esc(reason)}</p>
    <label>Votre PIN SituLab</label><div id="pc-pin"></div>
    <div class="inline-error hidden" id="pc-err"></div></div>`, async () => {
    const err = el("pc-err"); err.classList.add("hidden");
    try { await onPin(ci.read()); }
    catch (e) { err.textContent = cleanErr(e); err.classList.remove("hidden"); ci.clear(); }
  });
  el("modal-ok").textContent = "Confirmer";
  ci = codeInput(el("pc-pin"), true, () => modalOk(), true);
}

// providerLogo : logo inline (zéro dépendance, CSP-safe) pour les cartes de choix de serveur.
function providerLogo(kind) {
  if (kind === "github") {
    return `<svg class="conn-logo" viewBox="0 0 16 16" width="34" height="34" fill="currentColor" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 0 1 2-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z"/></svg>`;
  }
  return `<svg class="conn-logo" viewBox="0 0 24 24" width="34" height="34" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="6" cy="6" r="2.2"/><circle cx="6" cy="18" r="2.2"/><circle cx="17.5" cy="9" r="2.2"/><path d="M6 8.2v7.6M8.1 7.1 15.4 9M6 15.8c0-3 .5-4.8 3.5-4.8H15"/></svg>`;
}

// addInstanceModal : TUNNEL d'ajout de connexion (1.9.4) — nom → type (cartes/logos) → rôle → jeton
// (+ aide et URL) → confirmation. Réutilise AddInstanceToken/AddInstanceLogin + SetInstanceRole.
function addInstanceModal() {
  const W = { name: "", provider: "", role: "atelier" };
  const hist = [];
  const m = el("modal"), okBtn = el("modal-ok"), cancelBtn = el("modal-cancel");
  m.classList.remove("hidden");
  m.onclick = (ev) => { if (ev.target === m) closeModal(); };
  const head = (n, t) => `<h3 id="modal-title">Nouvelle connexion</h3><div class="wiz-head"><span class="wiz-step">Étape ${n}/4</span> · ${esc(t)}</div>`;
  const screen = (html) => { el("modal-body").innerHTML = html; };
  const buttons = (okText, okFn) => {
    okBtn.classList.toggle("hidden", !okText);
    okBtn.disabled = false; okBtn.textContent = okText || ""; okBtn.onclick = okFn || null;
    cancelBtn.classList.remove("hidden");
    cancelBtn.textContent = hist.length > 1 ? "Retour" : "Annuler";
    cancelBtn.onclick = hist.length > 1 ? back : closeModal;
  };
  const go = (fn) => { hist.push(fn); fn(); };
  const back = () => { hist.pop(); const p = hist[hist.length - 1]; if (p) p(); };

  function stepName() {
    screen(`${head(1, "Nom de la connexion")}<div class="field"><input id="c-name" class="inp" placeholder="ex. Travail, GitHub perso…" value="${esc(W.name)}" /><small class="muted">Une étiquette à votre choix.</small></div>`);
    buttons("Suivant", () => { const v = el("c-name").value.trim(); if (!v) { toast("Donnez un nom.", "err"); return; } W.name = v; go(stepType); });
    const i = el("c-name"); if (i) { i.focus(); i.onkeydown = (e) => { if (e.key === "Enter") okBtn.click(); }; }
  }
  function stepType() {
    screen(`${head(2, "Type de serveur")}<div class="conn-cards">
      <button type="button" class="conn-card" data-p="github">${providerLogo("github")}<span class="cc-name">GitHub</span><span class="cc-sub">github.com</span></button>
      <button type="button" class="conn-card" data-p="forgejo">${providerLogo("forgejo")}<span class="cc-name">Forgejo / Gitea</span><span class="cc-sub">serveur auto-hébergé</span></button>
    </div>`);
    buttons("", null);
    el("modal-body").querySelectorAll(".conn-card").forEach((b) => b.onclick = () => { W.provider = b.dataset.p; go(stepRole); });
  }
  function stepRole() {
    screen(`${head(3, "Rôle de cette connexion")}<div class="conn-cards">
      <button type="button" class="conn-card" data-r="atelier"><span class="cc-ico">🛠️</span><span class="cc-name">Atelier</span><span class="cc-sub">votre terrain de dev (autonome)</span></button>
      <button type="button" class="conn-card" data-r="production"><span class="cc-ico cc-vitrine">🌐</span><span class="cc-name">Vitrine</span><span class="cc-sub">production — tout sous PIN</span></button>
    </div>`);
    buttons("", null);
    el("modal-body").querySelectorAll(".conn-card").forEach((b) => b.onclick = () => { W.role = b.dataset.r; go(stepCreds); });
  }
  function stepCreds() {
    if (W.provider === "github") {
      screen(`${head(4, "Jeton d'accès GitHub")}<div class="field">
        <label>Jeton d'accès personnel (PAT)<input id="c-gtoken" type="password" placeholder="ghp_… ou github_pat_…" /></label>
        <small>Collez un jeton GitHub (accès <b>repo</b> / Contents). Chiffré par votre PIN.</small>
        ${tokenHelp("github")}
        <div class="inline-error hidden" id="c-err"></div></div>`);
      buttons("Créer la connexion", () => doCreate({ token: el("c-gtoken").value.trim() }));
      return;
    }
    screen(`${head(4, "Connexion au serveur")}<div class="field">
      <label>Adresse du serveur<input id="c-url" class="inp" placeholder="https://git.apcom.app" /></label>
      <label>Identifiant<input id="c-user" class="inp" /></label>
      <label>Mot de passe<input id="c-pass" type="password" /></label>
      <label>Code 2FA <span class="muted">(laisser vide si aucun)</span></label><div id="c-otp" class="git-code"></div>
      <label style="flex-direction:row;align-items:center"><input id="c-remember" type="checkbox" checked style="width:auto;margin-right:8px" />Mémoriser le mot de passe (chiffré par le PIN)</label>
      <label style="flex-direction:row;align-items:center"><input id="c-admin" type="checkbox" style="width:auto;margin-right:8px" />Compte administrateur</label>
      ${tokenHelp("forgejo")}
      <div class="inline-error hidden" id="c-err"></div></div>`);
    const ciOtp = codeInput(el("c-otp"), false, null);
    buttons("Créer la connexion", () => doCreate({
      url: el("c-url").value.trim(), user: el("c-user").value.trim(), pass: el("c-pass").value,
      otp: ciOtp.read(), admin: !!(el("c-admin") && el("c-admin").checked), remember: !!(el("c-remember") && el("c-remember").checked),
    }));
  }
  async function doCreate(dd) {
    const er = el("c-err"); if (er) er.classList.add("hidden");
    okBtn.disabled = true;
    try {
      if (W.provider === "github") await App().AddInstanceToken(W.name, "github", "", dd.token);
      else await App().AddInstanceLogin(W.name, dd.url, dd.user, dd.pass, dd.otp, dd.admin, dd.remember);
      try { await App().SetInstanceRole(W.name, W.role); } catch (_) {}
      stepDone();
    } catch (e) { okBtn.disabled = false; if (er) { er.textContent = cleanErr(e); er.classList.remove("hidden"); } }
  }
  function stepDone() {
    hist.length = 0;
    screen(`<div class="conn-done"><div class="cd-ico">✅</div><h3 id="modal-title">Connexion active</h3>
      <p class="sub">« <b>${esc(W.name)}</b> » — ${W.provider === "github" ? "GitHub" : "Forgejo / Gitea"} · <span class="role-badge ${W.role === "production" ? "role-production" : "role-atelier"}">${W.role === "production" ? "VITRINE" : "ATELIER"}</span> est prête.</p></div>`);
    okBtn.classList.remove("hidden"); okBtn.disabled = false; okBtn.textContent = "Terminer"; okBtn.onclick = () => location.reload();
    cancelBtn.classList.add("hidden");
  }
  go(stepName);
}

// reconnectModal : recrée le token d'une instance existante (passer en admin, mémoriser le
// mot de passe) — une seule saisie, puis Admin et Tokens marchent au PIN seul.
function reconnectModal(name, opts) {
  opts = opts || {};
  const inst = (state.instances || []).find((i) => i.name === name) || {};
  // GitHub : reconnexion = coller un nouveau jeton d'accès personnel.
  if (inst.provider === "github") {
    openModal("Reconnecter « " + name + " »", `<div class="field">
      <p class="sub">Collez un nouveau jeton d'accès personnel GitHub pour rétablir l'accès.</p>
      ${forgejoBlock(`<label>Jeton d'accès personnel (PAT)<input id="rc-gtoken" type="password" placeholder="ghp_… ou github_pat_…" /></label>
        <small>Collez un nouveau jeton GitHub (accès <b>repo</b> / Contents).</small>
        ${tokenHelp("github")}`, "https://github.com", "github")}
      <div class="inline-error hidden" id="rc-err"></div></div>`, async () => {
      const er = el("rc-err");
      try {
        await App().ReconnectToken(name, el("rc-gtoken").value.trim());
        closeModal(); toast("Compte reconnecté", "ok"); location.reload();
      } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
    });
    return;
  }
  let ciOtp;
  const title = opts.admin ? "Activer l'administration" : "Reconnecter « " + name + " »";
  const intro = opts.admin
    ? "SituLab va recréer votre accès à « " + esc(name) + " » en y ajoutant le droit de gestion des comptes. Confirmez vos identifiants Forgejo puis votre PIN."
    : "Recrée l'accès de SituLab pour ce serveur (activer l'Administration, mémoriser votre mot de passe, ou reconnecter après un changement de PIN). À faire une seule fois.";
  openModal(title, `<div class="field">
    <p class="sub">${intro}</p>
    ${forgejoBlock(`
      <label>Identifiant<input id="rc-user" value="${esc(state.login || "")}" /></label>
      <label>Mot de passe<input id="rc-pass" type="password" /></label>
      <label>Code 2FA <span class="muted">(laisser vide si aucun)</span></label><div id="rc-otp" class="git-code"></div>
      <label style="flex-direction:row;align-items:center"><input id="rc-admin" type="checkbox" ${opts.admin ? "checked" : ""} style="width:auto;margin-right:8px" />Compte administrateur (gérer les comptes)</label>
      <label style="flex-direction:row;align-items:center"><input id="rc-remember" type="checkbox" ${inst.remember ? "checked" : ""} style="width:auto;margin-right:8px" />Mémoriser mon mot de passe (chiffré par le PIN)</label>`, inst.url)}
    ${tokenHelp("forgejo")}
    <div class="inline-error hidden" id="rc-err"></div></div>`, async () => {
    const er = el("rc-err");
    try {
      await App().ReconnectInstance(name, el("rc-user").value.trim(), el("rc-pass").value, ciOtp.read(), !!el("rc-admin").checked, !!el("rc-remember").checked);
      closeModal(); toast("Compte reconnecté", "ok"); location.reload();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  ciOtp = codeInput(el("rc-otp"), false, null);
}

// purgeDataModal : effacement total des données locales (v1.4.1). Confirmation par saisie, puis
// vide aussi le localStorage de la fenêtre (thème, mode avancé) et repart sur l'onboarding.
function purgeDataModal() {
  openModal("Effacer mes données de ce poste", `<div class="field">
    <div class="callout warn"><b>Action irréversible.</b> Vos connexions, votre trousseau chiffré (jetons), vos réglages et les jetons du coffre Windows seront supprimés de cette machine. Vos dépôts et fichiers déjà clonés sur le poste ne sont pas touchés.</div>
    <p class="sub">Le cache d'affichage (navigateur embarqué) étant utilisé par la fenêtre, il ne sera entièrement vidé qu'au prochain lancement — ou via la désinstallation avancée. Les éléments sensibles (jetons) sont, eux, effacés immédiatement.</p>
    <label>Pour confirmer, tapez <b>EFFACER</b><input id="pg-confirm" placeholder="EFFACER" autocomplete="off" /></label>
    <div class="inline-error hidden" id="pg-error"></div></div>`, async () => {
    const er = el("pg-error");
    if (el("pg-confirm").value.trim().toUpperCase() !== "EFFACER") {
      er.textContent = "Tapez EFFACER pour confirmer."; er.classList.remove("hidden"); return;
    }
    try {
      await App().Purge();
      try { localStorage.clear(); } catch (e) {}
      closeModal(); toast("Données effacées — retour au départ", "ok");
      setTimeout(() => location.reload(), 600);
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  el("modal-ok").textContent = "Effacer définitivement"; el("modal-ok").classList.add("danger-btn");
}

async function setPinModal() {
  const hasPin = await App().PinSet().catch(() => false);
  let ci0, ci1, ci2;
  openModal(hasPin ? "Changer le PIN" : "Créer votre PIN", `<div class="field">
    <p class="sub">${hasPin ? "Saisissez votre PIN actuel, puis choisissez le nouveau (6 chiffres)." : "Choisissez un PIN de 6 chiffres. Il déverrouille vos accès et confirme vos envois. Retenez-le : il ne peut pas être récupéré."}</p>
    ${hasPin ? `<label>PIN actuel</label><div id="p0"></div>` : ""}
    <label>${hasPin ? "Nouveau PIN" : "Votre PIN"} (6 chiffres)</label><div id="p1"></div>
    <label>Confirmer</label><div id="p2"></div>
    <div class="inline-error hidden" id="p-error"></div></div>`, async () => {
    const er = el("p-error");
    const a = ci1.read(), b = ci2.read(), old = hasPin ? ci0.read() : "";
    if (a.length < CODELEN) { er.textContent = "Le PIN doit faire 6 chiffres."; er.classList.remove("hidden"); return; }
    if (a !== b) { er.textContent = "Les PIN ne correspondent pas."; er.classList.remove("hidden"); return; }
    try { await App().SetPin(old, a); closeModal(); toast("PIN enregistré", "ok"); renderSettings(); }
    catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  if (hasPin) ci0 = codeInput(el("p0"), true, null, true);
  ci1 = codeInput(el("p1"), true, null, !hasPin);
  ci2 = codeInput(el("p2"), true, () => modalOk()); // valide auto quand la confirmation est complète
}

// pinRequestModal : fenêtre de saisie du PIN déclenchée par le démon (demande d'un
// client CLI/agent). Valider renvoie le PIN au démon ; annuler/fond refuse l'opération.
// pushPreviewHTML : rend l'aperçu éclairé d'un envoi (v1.7) — branche, commits, fichiers, et
// surtout les NOUVEAUX fichiers non suivis mis en évidence. Transforme « confirmer à l'aveugle »
// en consentement réel. Vide si pas d'aperçu (actions non liées à un envoi).
function pushPreviewHTML(d) {
  if (!d || typeof d !== "object") return "";
  const list = (arr, n) => (arr || []).slice(0, n).map((x) => `<li>${esc(x)}</li>`).join("");
  const more = (arr, n) => ((arr || []).length > n ? `<li class="muted">… +${(arr || []).length - n}</li>` : "");
  const commits = (d.commits || []);
  const untr = (d.untracked || []);
  const chg = (d.changed || []);
  let h = `<div class="push-preview">`;
  h += `<div class="pp-head">À publier${d.branch ? " sur <b>" + esc(d.branch) + "</b>" : ""}${d.remote ? ` <span class="muted">→ ${esc(d.remote)}</span>` : ""}</div>`;
  if (commits.length) h += `<div class="pp-sec"><div class="pp-t">${commits.length} version(s) à envoyer</div><ul>${list(commits, 8)}${more(commits, 8)}</ul></div>`;
  if (chg.length) h += `<div class="pp-sec"><div class="pp-t">Fichiers modifiés (${chg.length})</div><ul>${list(chg, 8)}${more(chg, 8)}</ul></div>`;
  if (untr.length) h += `<div class="pp-sec pp-new"><div class="pp-t">⚠ Nouveaux fichiers non suivis (${untr.length}) — vérifiez qu'aucun n'est sensible</div><ul>${list(untr, 10)}${more(untr, 10)}</ul></div>`;
  if (!commits.length && !chg.length && !untr.length) h += `<div class="muted">Rien de nouveau à envoyer détecté.</div>`;
  h += `</div>`;
  return h;
}

// --- Rôle & politique PIN par connexion (v1.7) ---
// Les 4 classes d'action soumises à politique, et leur libellé non-dev.
const PIN_ACTIONS = [
  { key: "push", label: "Envoi de code (push / synchroniser)" },
  { key: "issues", label: "Issues, demandes de fusion, commentaires" },
  { key: "repo_admin", label: "Administration des dépôts (créer, supprimer, renommer, versions…)" },
  { key: "user_admin", label: "Administration des comptes et des jetons" },
];
// defaultPIN reflète fidèlement le défaut Go (config.defaultRequiresPIN).
function defaultPIN(role, act) {
  if (role === "production") return true;
  return act === "repo_admin" || act === "user_admin";
}
// effPIN : la valeur effective (politique explicite si définie, sinon défaut du rôle).
function effPIN(role, policy, act) {
  const v = policy ? policy[act] : undefined;
  return v === true || v === false ? v : defaultPIN(role, act);
}

function connPolicyModal(name) {
  const inst = (state._settingsInstances || []).find((i) => i.name === name) || { name, role: "atelier" };
  let role = inst.role === "production" ? "production" : "atelier";
  let policy = inst.pin_policy || null;
  let signMode = inst.sign_mode === "server" ? "server" : "local";
  let signURL = inst.sign_server_url || "";

  const rows = () => PIN_ACTIONS.map((a) =>
    `<label class="set-row toggle"><div class="set-row-l">${a.label}<small>activé = votre PIN est demandé ; désactivé = autorisé dès que SituLab est déverrouillé</small></div>
     <span class="switch"><input type="checkbox" data-act="${a.key}" ${effPIN(role, policy, a.key) ? "checked" : ""} /><span class="track"></span></span></label>`).join("");

  const body = () => `<div class="field">
    <div class="set-row"><div class="set-row-l">Rôle de la connexion</div>
      <div class="set-row-c"><div class="seg" role="group" aria-label="Rôle" id="role-seg">
        <button data-role="atelier" class="${role === "atelier" ? "on" : ""}">Atelier</button>
        <button data-role="production" class="${role === "production" ? "on" : ""}">Production</button>
      </div></div></div>
    <div class="set-sec-d" id="role-help">${role === "production"
      ? "Vitrine protégée : par défaut tout est confirmé par votre PIN. On n'y publie que des versions validées."
      : "Terrain de travail : votre IA peut y être autonome (envoi/issues sans PIN), l'administration reste sous PIN."}</div>
    <div class="seg" style="margin:8px 0" id="pol-presets">
      <button data-preset="atelier">Préréglage Atelier autonome</button>
      <button data-preset="production">Préréglage Production protégée</button>
    </div>
    <div id="pol-rows">${rows()}</div>
    <div class="set-row" style="margin-top:14px"><div class="set-row-l">Signature des commits<small>empreinte « SituLab approved »</small></div>
      <div class="set-row-c"><div class="seg" role="group" aria-label="Signature" id="sign-seg">
        <button data-sign="local" class="${signMode === "server" ? "" : "on"}">Logiciel</button>
        <button data-sign="server" class="${signMode === "server" ? "on" : ""}">Serveur</button>
      </div></div></div>
    <div id="sign-url-row" class="${signMode === "server" ? "" : "hidden"}"><label>URL du service de signature<input id="sign-url" class="inp" value="${esc(signURL)}" placeholder="https://situememoring.com" /></label></div>
    <div class="set-sec-d" id="sign-help">${signMode === "server"
      ? "Serveur : la clé de signature ne quitte jamais le serveur — un agent sur le poste ne peut rien forger (audit, rate-limit, révocation)."
      : "Logiciel : clé embarquée dans SituLab — marqueur d'authenticité « fait via SituLab » (extractible, limite assumée)."}</div>
  </div>`;

  openModal("🏷️ Rôle & PIN — " + esc(name), body(), async () => {
    const pol = {};
    let differs = false;
    document.querySelectorAll("#pol-rows input[data-act]").forEach((c) => {
      pol[c.dataset.act] = c.checked;
      if (c.checked !== defaultPIN(role, c.dataset.act)) differs = true;
    });
    try {
      await App().SetInstanceRole(name, role);
      // Si la politique correspond exactement aux défauts du rôle, on renvoie null (héritage) plutôt
      // qu'un figement explicite — ainsi un futur changement de rôle reste effectif.
      await App().SetInstancePINPolicy(name, differs ? pol : null);
      await App().SetInstanceSignMode(name, signMode, (el("sign-url") && signMode === "server") ? el("sign-url").value.trim() : "");
      await refreshInstances(); // met à jour immédiatement le badge du header et le sélecteur (pas de redémarrage)
      closeModal(); toast("Rôle, PIN & signature mis à jour", "ok"); renderSettings();
    } catch (e) { toast(cleanErr(e), "err"); }
  });
  el("modal-ok").textContent = "Enregistrer";

  // Interactions : changer le rôle re-sème les suggestions ; les préréglages remplissent les cases.
  const reseed = () => { el("pol-rows").innerHTML = rows(); };
  el("role-seg").onclick = (e) => {
    const b = e.target.closest("[data-role]"); if (!b) return;
    role = b.dataset.role; policy = null; // repart des défauts du nouveau rôle
    document.querySelectorAll("#role-seg button").forEach((x) => x.classList.toggle("on", x.dataset.role === role));
    el("role-help").textContent = role === "production"
      ? "Vitrine protégée : par défaut tout est confirmé par votre PIN. On n'y publie que des versions validées."
      : "Terrain de travail : votre IA peut y être autonome (envoi/issues sans PIN), l'administration reste sous PIN.";
    reseed();
  };
  el("pol-presets").onclick = (e) => {
    const b = e.target.closest("[data-preset]"); if (!b) return;
    policy = null; role = b.dataset.preset; // le préréglage fixe aussi le rôle cohérent
    document.querySelectorAll("#role-seg button").forEach((x) => x.classList.toggle("on", x.dataset.role === role));
    reseed();
  };
  el("sign-seg").onclick = (e) => {
    const b = e.target.closest("[data-sign]"); if (!b) return;
    signMode = b.dataset.sign;
    document.querySelectorAll("#sign-seg button").forEach((x) => x.classList.toggle("on", x.dataset.sign === signMode));
    el("sign-url-row").classList.toggle("hidden", signMode !== "server");
    el("sign-help").textContent = signMode === "server"
      ? "Serveur : la clé de signature ne quitte jamais le serveur — un agent sur le poste ne peut rien forger (audit, rate-limit, révocation)."
      : "Logiciel : clé embarquée dans SituLab — marqueur d'authenticité « fait via SituLab » (extractible, limite assumée).";
  };
}

// Visuel par type d'action de la fenetre PIN (Point 6) : couleur + icone + flux source->destination.
const PIN_ACTION_STYLE = {
  push:    { cls: "pa-push",    ico: "⬆",  label: "Envoi en ligne" },
  promote: { cls: "pa-promote", ico: "🚀", label: "Promotion vers la production" },
  delete:  { cls: "pa-delete",  ico: "🗑",  label: "Suppression" },
  create:  { cls: "pa-create",  ico: "✚",  label: "Création" },
  config:  { cls: "pa-config",  ico: "⚙",  label: "Réglage" },
  issue:   { cls: "pa-issue",   ico: "💬", label: "Issue / fusion" },
  admin:   { cls: "pa-admin",   ico: "🔑", label: "Administration" },
  unlock:  { cls: "pa-unlock",  ico: "🔓", label: "Déverrouillage" },
};
// pinFlowHTML : montre le SENS de l'action (ce poste -> en ligne, atelier -> production).
function pinFlowHTML(action, detail) {
  if (action === "push") {
    const dest = (detail && detail.remote) ? esc(detail.remote) : "en ligne";
    const br = (detail && detail.branch) ? ` <span class="muted">(${esc(detail.branch)})</span>` : "";
    return `<div class="pin-flow"><span class="pf-node pf-local">🖥 Ce poste${br}</span><span class="pf-arrow">→</span><span class="pf-node pf-atl">☁ ${dest}</span></div>`;
  }
  if (action === "promote") {
    return `<div class="pin-flow"><span class="pf-node pf-atl">Atelier</span><span class="pf-arrow">→</span><span class="pf-node pf-prod">Production</span></div>`;
  }
  return "";
}

function pinRequestModal(reason, detail, action) {
  let ci;
  const st = PIN_ACTION_STYLE[action] || null;
  const isDel = action === "delete";
  const chip = st ? `<div class="pin-action ${st.cls}"><span class="pin-ico">${st.ico}</span><span>${esc(st.label)}</span></div>` : "";
  const intro = isDel
    ? `<b>Suppression demandée.</b> Cette action est <b>destructive</b> et irréversible.`
    : `<b>Un agent / une commande demande votre feu vert.</b>`;
  openModal("🔐 Confirmation requise", `<div class="field">
    ${chip}
    <div class="callout ${isDel ? "danger" : "warn"}">${intro}<br>Elle ne se fera <b>que</b> si vous entrez votre PIN SituLab ci-dessous.</div>
    <p class="tagline">${esc(reason)}</p>
    ${pinFlowHTML(action, detail)}
    ${pushPreviewHTML(detail)}
    <label>Votre PIN SituLab</label><div id="pin-input"></div>
    <small>SituLab <b>constate</b> l'action ci-dessus ; votre PIN seul l'autorise — il ne quitte jamais SituLab. Si vous n'êtes pas à l'origine de cette demande, cliquez <b>Refuser</b>.</small></div>`, async () => {
    try { await App().SubmitPIN(ci ? ci.read() : ""); } catch (e) {}
    closeModal();
  });
  ci = codeInput(el("pin-input"), true, () => modalOk(), true); // validation auto dès 6 chiffres
  if (isDel && el("modal-ok")) el("modal-ok").classList.add("danger-btn"); // bouton de confirmation rouge pour une suppression
  // Refuser / fond / Échap : prévenir le démon tout de suite (sinon il attend 2 min).
  const refuse = () => { try { App().CancelPIN(); } catch (e) {} closeModal(); };
  modalCancel = refuse;
  el("modal-cancel").textContent = "Refuser";
  el("modal-cancel").onclick = refuse;
  el("modal").onclick = (ev) => { if (ev.target === el("modal")) refuse(); };
}

// Liens externes (vers l'instance Forgejo) : ouverts dans le navigateur du système.
document.addEventListener("click", (e) => {
  const a = e.target.closest && e.target.closest("[data-ext]");
  if (a) { e.preventDefault(); safeOpenURL(a.dataset.ext); }
});

// Robustesse focus (WebView2 ne focalise pas toujours l'input au lancement) : dès qu'un
// CHIFFRE est tapé alors qu'aucune case n'est active mais qu'un champ en cases est présent,
// on focalise la 1re case vide — l'utilisateur ouvre l'app et tape son PIN, sans clic.
document.addEventListener("keydown", (e) => {
  if (e.key && e.key.length === 1 && /\d/.test(e.key)) {
    const ae = document.activeElement;
    if (!ae || !ae.classList || !ae.classList.contains("code-box")) {
      const ci = document.querySelector(".code-input");
      if (ci) {
        const boxes = [...ci.querySelectorAll(".code-box")];
        const target = boxes.find((b) => !b.value) || boxes[0];
        if (target) {
          e.preventDefault();
          target.focus();
          target.value = e.key; // insère directement le chiffre (sinon la 1re frappe serait perdue)
          target.dispatchEvent(new Event("input", { bubbles: true })); // avance + vérifie la complétude
        }
      }
    }
  }
}, true);

/* ---------------- Clavier ---------------- */
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    const m = el("modal");
    if (m && !m.classList.contains("hidden")) { (modalCancel || closeModal)(); return; }
    if (state.view === "detail") navTo("repos");
  }
  if (e.key === "Enter" && !el("modal").classList.contains("hidden") && modalOk && e.target.tagName === "INPUT") { e.preventDefault(); modalOk(); }
  if (e.key === "Enter" || e.key === " ") {
    const c = e.target.closest && e.target.closest("[data-fn], .frow");
    if (c && (c.matches("[data-fn]") || c.matches(".frow"))) { e.preventDefault(); c.click(); }
  }
});

/* ---------------- Composants d'état ---------------- */
function blankslate(ico, title, text) {
  return `<div class="blankslate"><div class="bs-ico" aria-hidden="true">${ico}</div><div class="bs-title">${esc(title)}</div><div class="bs-text">${esc(text)}</div></div>`;
}
function surfaceError(title, msg, retry) {
  const id = "retry-" + Math.round(performance.now());
  setTimeout(() => { const b = el(id); if (b && retry) b.onclick = retry; }, 0);
  return `<div class="surface-error"><span class="se-ico" aria-hidden="true">⚠</span><div class="se-msg"><b>${esc(title)}</b><div class="sub">${esc(msg)}</div></div>${retry ? `<button class="btn btn-ghost btn-sm" id="${id}">Réessayer</button>` : ""}</div>`;
}
function skeletons(n) { return Array.from({ length: n }, () => `<div class="skeleton"></div>`).join(""); }

function toast(msg, kind) {
  const t = document.createElement("div");
  t.className = "toast " + (kind || "");
  t.textContent = msg;
  el("toasts").appendChild(t);
  setTimeout(() => { t.style.opacity = "0"; setTimeout(() => t.remove(), 250); }, 2800);
}

// Icône « œil » (afficher/masquer). Un seul glyphe ; l'état masqué ajoute la barre oblique.
const EYE_SVG = `<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" fill="none" stroke="currentColor" stroke-width="1.7"/><circle cx="12" cy="12" r="3" fill="none" stroke="currentColor" stroke-width="1.7"/><line class="eye-slash" x1="4" y1="20" x2="20" y2="4" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>`;

// addRevealToggle place une icône « œil » DANS chaque champ mot de passe (clic = afficher/masquer),
// sans texte. Saisir un mot de passe à l'aveugle est une cause fréquente d'échecs pour un non-initié.
// (Nom conservé pour les appelants ; comportement v1.4.2 : icône in-field, plus de case à cocher.)
function addRevealToggle(root) {
  if (!root) return;
  root.querySelectorAll('input[type="password"]:not(.code-box)').forEach((inp) => {
    if (inp.dataset.eye === "1") return; // déjà équipé
    inp.dataset.eye = "1";
    const wrap = document.createElement("span");
    wrap.className = "pw-wrap";
    inp.parentNode.insertBefore(wrap, inp);
    wrap.appendChild(inp);
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "pw-eye";
    btn.setAttribute("aria-label", "Afficher le mot de passe");
    btn.innerHTML = EYE_SVG;
    btn.onclick = () => {
      const show = inp.type === "password";
      inp.type = show ? "text" : "password";
      btn.classList.toggle("on", show);
      btn.setAttribute("aria-label", show ? "Masquer le mot de passe" : "Afficher le mot de passe");
    };
    wrap.appendChild(btn);
  });
}

/* ---------------- Thème ---------------- */
function readTheme() { try { return localStorage.getItem("situlab-theme") || "auto"; } catch (e) { return "auto"; } }
function writeTheme(v) { try { localStorage.setItem("situlab-theme", v); } catch (e) {} }
function applyTheme(v) { if (v === "auto") delete document.documentElement.dataset.theme; else document.documentElement.dataset.theme = v; }

/* ---------------- Markdown léger ---------------- */
function mdToHtml(md) {
  const lines = String(md).replace(/\r/g, "").split("\n");
  let html = "", inCode = false, inTable = false, listType = "";
  const closeList = () => { if (listType) { html += `</${listType}>`; listType = ""; } };
  const closeTable = () => { if (inTable) { html += "</tbody></table>"; inTable = false; } };
  const inline = (s) => esc(s).replace(/`([^`]+)`/g, "<code>$1</code>").replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>");
  for (let raw of lines) {
    if (raw.startsWith("```")) { closeList(); closeTable(); inCode = !inCode; html += inCode ? "<pre class='doc-code'>" : "</pre>"; continue; }
    if (inCode) { html += esc(raw) + "\n"; continue; }
    if (/^\s*\|.*\|\s*$/.test(raw)) {
      const cells = raw.trim().replace(/^\||\|$/g, "").split("|").map((c) => c.trim());
      if (cells.every((c) => /^-+$/.test(c))) continue;
      if (!inTable) { closeList(); html += "<table class='doc-table'><tbody>"; inTable = true; }
      html += "<tr>" + cells.map((c) => `<td>${inline(c)}</td>`).join("") + "</tr>"; continue;
    }
    closeTable();
    const h = raw.match(/^(#{1,4})\s+(.*)$/);
    if (h) { closeList(); html += `<h${h[1].length} class="doc-h">${inline(h[2])}</h${h[1].length}>`; continue; }
    if (/^\s*>\s?/.test(raw)) { closeList(); html += `<blockquote>${inline(raw.replace(/^\s*>\s?/, ""))}</blockquote>`; continue; }
    const li = raw.match(/^(\s*)([-*]|\d+\.)\s+(.*)$/);
    if (li) { const t = /\d/.test(li[2]) ? "ol" : "ul"; if (listType !== t) { closeList(); html += `<${t}>`; listType = t; } html += `<li>${inline(li[3])}</li>`; continue; }
    if (raw.trim() === "") { closeList(); continue; }
    closeList(); html += `<p>${inline(raw)}</p>`;
  }
  closeList(); closeTable(); if (inCode) html += "</pre>";
  return html;
}

/* ---------------- Utils ---------------- */
function shortPath(p) { if (!p) return ""; const parts = p.split(/[\\/]/).filter(Boolean); return parts.length <= 2 ? p : "…/" + parts.slice(-2).join("/"); }
function parentOf(p) { const i = p.lastIndexOf("/"); return i >= 0 ? p.slice(0, i) : ""; }
function fmtSize(n) { if (n == null) return ""; if (n < 1024) return n + " o"; if (n < 1048576) return (n / 1024).toFixed(0) + " Ko"; return (n / 1048576).toFixed(1) + " Mo"; }
// Repli SÛR : si la date distante n'est pas parsable, on ne réinjecte JAMAIS la
// chaîne brute (elle pourrait contenir du HTML) — on renvoie une chaîne vide.
// Format de date suisse jj.mm.aaaa (séparateur point), construit à la main pour un rendu stable.
const pad2 = (n) => String(n).padStart(2, "0");
// fmtDay : date seule « 09.10.2026 ».
function fmtDay(s) { if (!s) return ""; const d = new Date(s); return isNaN(d) ? "" : `${pad2(d.getDate())}.${pad2(d.getMonth() + 1)}.${d.getFullYear()}`; }
// fmtDate : date + heure « 09.10.2026 14:24 ».
function fmtDate(s) { if (!s) return ""; const d = new Date(s); return isNaN(d) ? "" : `${fmtDay(s)} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`; }
// fmtWhen : identique (jj.mm.aaaa hh:mm), pour les horodatages « maj » des cartes.
const fmtWhen = fmtDate;
