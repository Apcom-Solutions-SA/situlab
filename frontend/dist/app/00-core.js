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
// pinModalOpen : vrai tant qu'une fenêtre PIN DÉCLENCHÉE PAR LE DÉMON (situlab:pin-request) est à
// l'écran. Cette fenêtre déverrouille DÉJÀ la session (le démon fait Unlock/gatePIN avec le PIN saisi) :
// pendant qu'elle est ouverte, le poll d'arrière-plan ne doit PAS empiler par-dessus son propre écran de
// déverrouillage « verrouillé », sinon l'utilisateur ressaisit son PIN pour rien (double demande).
let pinModalOpen = false;
let lockGrace = false;
// handleLock : une lecture a échoué sur « session verrouillée ». AVANT de montrer l'écran de
// déverrouillage STANDARD, on laisse une courte fenêtre à une éventuelle demande de PIN d'AGENT
// (situlab:pin-request) de prendre la main : si c'est l'IA qui provoque ce verrou, c'est SA fenêtre
// contextuelle qui doit s'afficher (avec ce qui est demandé), et elle doit ÉPARGNER l'écran standard.
// L'écran standard ne reste donc que pour un verrouillage SANS demande d'agent (inactivité, ouverture
// humaine). Le démon ré-émet sa demande jusqu'à réponse (voir prompter.go), ce qui garantit qu'une
// demande d'agent en cours finit par lever pinModalOpen pendant cette fenêtre de grâce.
async function handleLock() {
  if (relocking || lockGrace) return;
  if (pinModalOpen) return; // la fenêtre agent gère déjà le (dé)verrouillage
  lockGrace = true;
  for (let i = 0; i < 14; i++) {
    if (pinModalOpen) { lockGrace = false; return; } // une demande d'agent a pris la main
    await sleep(60);
  }
  lockGrace = false;
  if (pinModalOpen || relocking) return;
  relocking = true;
  stopLive();
  lockedScreen(); // page accueillante « session verrouillée » (gèle l'app, bouton de déverrouillage)
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
  repoState: "",   // filtre « État » du catalogue de dépôts (1.9.11)
  repoProject: "", // filtre « Projet » du catalogue : "" | "in" | "out" (1.9.11)
  repoTab: "",     // onglet d'emplacement du catalogue : "" (tous) | "locale" | "atelier" | "vitrine" (1.9.11)
  projectsIndex: null, // index « instance/owner/repo -> nom de projet » (atelier+vitrines, zéro réseau)
  users: [],       // cache admin
  advanced: false, // mode avancé : révèle les étapes séparées (envoyer/récupérer/enregistrer), la
                   //               matrice des versions de travail, les outils git locaux (mise de
                   //               côté) et les réglages de dépôt (v1.3)
  checkedAt: 0,    // horodatage du dernier tableau de bord reçu (« vérifié il y a X min »)
  readOnly: false, // mode lecture seule (v1.4) : masque les actions de mutation + bannière
  directory: null, // cache de l'annuaire consolidé (v1.4)
  dirSearch: "",   // filtre de recherche de l'annuaire
  repoView: "grid", // présentation du catalogue de dépôts : "grid" | "list" (v1.5.0)
};
try { if (localStorage.getItem("situlab.repoView") === "list") state.repoView = "list"; } catch (e) {}
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
  applyZoom(readZoom());
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
      // Charge utile {reason, detail, action, intent}. Compat : une ancienne chaîne seule reste gérée.
      // intent (Point 4) = phrase LIBRE de l'agent, montrée distinctement des actions constatées.
      const p = (payload && typeof payload === "object") ? payload : { reason: payload };
      pinRequestModal(p.reason || "Autoriser l'opération", p.detail || null, p.action || "", p.intent || "");
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
    // Démarrage à froid : si c'est une demande d'AGENT qui a réveillé l'app, le démon ré-émet
    // "situlab:pin-request" ; on lui laisse une courte fenêtre pour afficher SA fenêtre contextuelle
    // (avec ce qui est demandé) plutôt que l'écran de déverrouillage standard. Sinon (ouverture
    // humaine, sans demande en cours), on bascule sur l'écran standard.
    for (let i = 0; i < 14 && !pinModalOpen; i++) await sleep(60);
    if (pinModalOpen) { hideBoot(); return; } // la fenêtre agent prend la main ; elle rejouera l'app après saisie
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

// lockedScreen : page accueillante affichée quand la session se VERROUILLE pendant que l'app est
// ouverte (délai d'inactivité). Elle GÈLE l'usage (le markup de l'app est masqué, remplacé par cette
// carte) pour qu'aucune action ne parte sur des données périmées, explique calmement l'état, et offre
// un bouton explicite qui lance la saisie du PIN. Distincte de l'écran de déverrouillage d'ouverture :
// ici on NE présume pas que l'utilisateur veut déjà taper son PIN — on l'informe d'abord, il déclenche.
function lockedScreen() {
  hideBoot();
  stopLive(); // plus aucun appel d'arrière-plan tant que c'est verrouillé
  el("app").classList.add("hidden");
  const ob = el("onboarding");
  ob.classList.remove("hidden");
  ob.innerHTML = `<div class="connect-card">
    <img src="logo.svg" class="connect-logo" alt="" />
    <h1>SituLab</h1>
    <div class="lock-badge" aria-hidden="true">🔒</div>
    <p class="tagline">Session verrouillée</p>
    <p class="sub">Par sécurité, vos accès ont été protégés après une période d'inactivité. Vos jetons restent chiffrés sur ce poste. Déverrouillez avec votre PIN pour reprendre là où vous en étiez.</p>
    <button class="btn btn-primary" id="lk-unlock">🔓 Déverrouiller</button></div>`;
  el("lk-unlock").onclick = () => unlockScreen(() => location.reload());
}

function showFatal(msg) {
  hideBoot();
  el("onboarding").classList.add("hidden");
  el("app").classList.remove("hidden");
  el("view").innerHTML = surfaceError("SituLab n'a pas pu démarrer", msg, null);
}

