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
  // Worktree : UN seul .git (cercle) qui alimente PLUSIEURS dossiers de travail (branches differentes).
  worktree: `<svg class="gviz" viewBox="0 0 190 58" aria-hidden="true"><path d="M30 25h18c10 0 10-11 20-11h32"/><path d="M30 33h18c10 0 10 11 20 11h32"/><circle cx="22" cy="29" r="8"/><rect class="gv-accent" x="128" y="5" width="54" height="18" rx="4"/><rect class="gv-ok" x="128" y="35" width="54" height="18" rx="4"/></svg>`,
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
  { k: "copies", label: "🗂 Copies de travail", intro: "Deux façons d'avoir plusieurs dossiers de travail — utile quand plusieurs IA travaillent en parallèle sur le même poste.", items: [
    { t: "Clone", what: "Un <b>dossier indépendant</b>, avec son propre <code>.git</code> et son propre accès réseau. Isolation totale : idéal pour faire travailler plusieurs IA en parallèle sans qu'elles se gênent — chacune son clone.", in: "« Cloner ici » sur un projet ; la vue Dépôts montre « N copies » quand il y en a plusieurs.", viz: GV.cloud },
    { t: "Worktree", what: "Plusieurs dossiers <b>greffés sur un seul</b> <code>.git</code> : même historique partagé, une branche différente par dossier. Plus léger en disque qu'un clone, mais une même branche ne peut être active que dans <b>un seul</b> worktree à la fois.", in: "Panneau git d'un projet → <b>Copies de travail</b> : lister, créer et retirer vos worktrees.", viz: GV.worktree },
    { t: "Clone ou worktree ?", what: "<b>Clone</b> = tout séparé (réseau compris), la copie la plus simple à raisonner. <b>Worktree</b> = partage l'historique, idéal pour ouvrir vite une 2ᵉ branche sans re-télécharger — parfait pour une session IA parallèle.", in: "Le panneau git affiche le type (clone / worktree) de chaque copie." },
    { t: "⚠ Piège Windows", what: "Ne jamais partager un dossier entre worktrees par une <b>jonction</b> : retirer le worktree peut vider le dossier d'origine.", in: "SituLab vous guide au moment de créer une copie." },
  ]},
  { k: "sign", label: "🛡 Signature", intro: "La garantie d'origine « SituLab approved » — qui a fait le commit, via SituLab.", items: [
    { t: "Le sceau « certified »", what: "Chaque commit créé via SituLab porte une <b>attestation d'origine immuable</b> (dans le commit lui-même), vérifiée localement contre la racine APCOM. Un agent ne peut pas la forger.", in: "Le <b>sceau vert</b> dans l'onglet Historique (infobulle « Signé via SituLab »).", viz: GV.seal },
    { t: "Vert, ambre ou rouge", what: "<b>Vert</b> = origine <b>vérifiée</b> sur ce poste (seule vraie garantie). <b>Ambre</b> = sceau <b>déclaré mais pas encore vérifié</b> (vu en ligne sans copie locale) — à confirmer, pas une preuve. <b>Rouge</b> = sceau <b>invalide ou révoqué</b> : origine NON garantie.", in: "La couleur du sceau et le bandeau de résumé en haut de l'Historique." },
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
    <div class="view-head view-head-row">
      <div><button class="btn btn-ghost btn-sm" id="git-back">‹ Réglages</button>
      <div class="view-title" style="margin-top:6px">Comprendre git</div>
      <div class="view-sub">Le vocabulaire git en clair — et où chaque geste se trouve dans SituLab.</div></div>
    </div>
    <div class="set-layout">
      <nav class="set-nav">${GIT_GUIDE.map((g) => `<button class="set-navbtn" data-gittab="${g.k}">${esc(g.label)}</button>`).join("")}</nav>
      <div class="set-panels">${panels}</div>
    </div>`;
  // Retour contextuel : on revient dans Réglages, directement sur l'onglet « Connexions git » (1.12).
  if (el("git-back")) el("git-back").onclick = () => { state.settabTarget = "conn"; navTo("settings"); };
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
  // Aide = le Guide, point (1.12). « À propos » (version, licence, NOTICE) vit désormais dans Réglages
  // ▸ À propos — plus de doublon. La relance de la visite guidée reste ici, logiquement.
  el("view").innerHTML = `
    <div class="view-head view-head-row">
      <div><div class="view-title">Aide</div><div class="view-sub">Le guide SituLab — pour vous comme pour vos agents.</div></div>
      <div class="view-head-actions">
        <button class="btn btn-ghost btn-sm" id="replay-tour">↺ Rejouer la visite guidée</button>
        <button class="btn btn-ghost btn-sm" id="go-agentcfg">🤖 Configurer mon agent IA</button>
      </div></div>
    <div class="doc" id="doc"><div class="skeleton"></div></div>`;
  void tab;
  try { el("doc").innerHTML = mdToHtml(await App().Doc()); }
  catch (e) { el("doc").innerHTML = surfaceError("Documentation indisponible", cleanErr(e), null); }
  if (el("replay-tour")) el("replay-tour").onclick = () => { navTo("kernel"); setTimeout(() => startTour(true), 350); };
  if (el("go-agentcfg")) el("go-agentcfg").onclick = () => aiAgentModal();
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

// buildTourSteps : séquence ancrée sur le MENU latéral et le header — des cibles toujours présentes,
// quelle que soit la vue courante (réancrage 1.13 ; les anciennes cibles de la page « Dépôts » retirée
// n'existaient plus). Une cible masquée (module désactivé) est sautée ; cible absente → étape centrée.
function buildTourSteps() {
  const vis = (sel) => { const e = document.querySelector(sel); return e && !e.classList.contains("hidden"); };
  const steps = [
    { ic: "👋", title: "Bienvenue dans SituLab", body: "Un tour rapide en quelques étapes. Vous pouvez passer à tout moment (Échap). SituLab, c'est votre git et votre mémoire, sans navigateur." },
    { ic: (typeof NAV_ICO !== "undefined" ? NAV_ICO.kernel : "◆"), svg: true, sel: '.nav-item[data-view="kernel"]', title: "KERNEL — votre mémoire", body: "Le dépôt qui EST votre mémoire de travail. C'est le centre : on le garde à jour en deux gestes, et votre IA l'ouvre pour se contextualiser." },
  ];
  if (vis('.nav-item[data-view="corpus"]')) steps.push({ ic: "☁", sel: '.nav-item[data-view="corpus"]', title: "CORPUS — vos clouds souverains", body: "Vos fichiers sur Nextcloud ou kDrive, avec un accès IA borné par dossier : votre agent ne voit que ce que vous autorisez." });
  if (vis('.nav-item[data-view="spokes"]')) steps.push({ ic: "🛰", sel: '.nav-item[data-view="spokes"]', title: "SPOKES — vos missions", body: "Des projections de mission rattachées à un KERNEL, pour organiser vos chantiers." });
  if (vis('.nav-item[data-view="backup"]')) steps.push({ ic: "🛡", sel: '.nav-item[data-view="backup"]', title: "BACKUP — vos sauvegardes", body: "Des plans de sauvegarde de vos dépôts vers vos clouds, en clé-en-main." });
  steps.push({ ic: "⎇", sel: '.nav-item[data-view="projects"]', title: "Dépôts git", body: "Appairez vos dépôts (local ↔ atelier ↔ vitrine), classez-les, et ouvrez leur détail. Le point de départ pour organiser votre poste." });
  if (vis("#health")) steps.push({ ic: "🟢", sel: "#health", title: "L'état en un coup d'œil", body: "Les pastilles disent si vos connexions git et cloud sont joignables — et combien de dépôts restent à appairer." });
  if (vis('.nav-item[data-view="ia"]')) steps.push({ ic: "🤖", sel: '.nav-item[data-view="ia"]', title: "Agent IA", body: "La porte de votre agent vers SituLab : enregistrez la fiche de collaboration (AGENTS.md) et obtenez, sur mesure, son message de première connexion." });
  steps.push({ ic: "⚙", sel: '.nav-item[data-view="settings"]', title: "Réglages", body: "Vos connexions git & cloud, la sécurité (PIN, lecture seule), les Modules (adapter l'interface à votre usage) et l'Aide (le guide + cette visite)." });
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
  if (s.svg) el("tour-icon").innerHTML = s.ic || "•"; else el("tour-icon").textContent = s.ic || "•";
  el("tour-title").textContent = s.title;
  el("tour-body").textContent = s.body;
  // Pastilles de progression (● pour l'étape courante/passées, ○ pour les suivantes).
  el("tour-count").innerHTML = Array.from({ length: n }, (_, k) => `<span class="tour-dot${k <= i ? " on" : ""}"></span>`).join("");
  el("tour-next").textContent = last ? (s.ai ? "Terminer et configurer mon IA" : "Terminer") : "Suivant";
  positionTour();
}

// positionTour place le spotlight sur la cible et l'infobulle à proximité, TOUJOURS dans l'écran.
function positionTour() {
  if (!tourState) return;
  const s = tourState.steps[tourState.i];
  const hole = el("tour-hole"), tip = el("tour-tip");
  const target = s.sel ? document.querySelector(s.sel) : null;
  const vw = window.innerWidth, vh = window.innerHeight, gap = 14, m = 16;
  if (!target) {
    hole.style.display = "none";
    tip.style.left = "50%"; tip.style.top = "50%"; tip.style.transform = "translate(-50%,-50%)";
    tip.style.visibility = "visible";
    return;
  }
  const r = target.getBoundingClientRect();
  const pad = 4;
  hole.style.display = "block";
  hole.style.left = (r.left - pad) + "px";
  hole.style.top = (r.top - pad) + "px";
  hole.style.width = (r.width + pad * 2) + "px";
  hole.style.height = (r.height + pad * 2) + "px";
  // Mesure réelle de l'infobulle (contenu déjà posé), bornée à l'écran.
  tip.style.transform = "none";
  tip.style.visibility = "hidden";
  tip.style.left = "0px"; tip.style.top = "0px";
  const tw = Math.min(tip.offsetWidth, vw - 2 * m);
  const th = Math.min(tip.offsetHeight, vh - 2 * m);
  let left, top;
  // Priorité : à DROITE (items du menu, à gauche de l'écran), sinon dessous, dessus, à gauche, sinon centré.
  if (r.right + gap + tw <= vw - m) { left = r.right + gap; top = r.top; }
  else if (r.bottom + gap + th <= vh - m) { left = r.left; top = r.bottom + gap; }
  else if (r.top - gap - th >= m) { left = r.left; top = r.top - gap - th; }
  else if (r.left - gap - tw >= m) { left = r.left - gap - tw; top = r.top; }
  else { left = (vw - tw) / 2; top = (vh - th) / 2; }
  left = Math.max(m, Math.min(left, vw - tw - m));
  top = Math.max(m, Math.min(top, vh - th - m));
  tip.style.left = left + "px"; tip.style.top = top + "px";
  tip.style.visibility = "visible";
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

