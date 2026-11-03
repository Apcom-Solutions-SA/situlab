/* ---------------- Onboarding CANALISANT : mise en route (1.13) ----------------
 * DISTINCT de la visite guidée (coachmarks) et de l'onboarding initial (PIN + 1re connexion, 01-*).
 * Ici : une TIMELINE d'étapes obligatoires/facultatives, calculée sur l'ÉTAT RÉEL du poste (connexions,
 * appairements, KERNEL). Tant que les étapes OBLIGATOIRES ne sont pas faites, les menus de domaine sont
 * BRIDÉS et l'app atterrit sur cette page. Fail-OPEN : si on ne peut pas calculer l'état (verrouillé /
 * hors ligne), on ne bride JAMAIS — on ne piège pas l'utilisateur.
 */

// refreshSetup : calcule state.setup = {s1,s2,s3,s4,s5,done} puis applique le bridage.
async function refreshSetup() {
  try {
    const insts = (state.instances || []).length;
    const hasVitrine = (state.instances || []).some((i) => i.role === "production");
    let vitAnswer = null;
    try { vitAnswer = localStorage.getItem("situlab.setup.vitrine"); } catch (e) {}
    state._setupHasVitrine = hasVitrine;
    state._setupVitAnswer = vitAnswer;
    // Étape 1 = au moins une connexion (atelier). La VITRINE est une carte À PART (1.18.1), facultative
    // et révisable à tout moment : elle ne bride jamais la mise en route.
    const s1 = insts > 0;

    // Appairages en attente : on lit la DERNIÈRE valeur connue (cache) au lieu d'appeler RepoMatrix
    // (= réseau), qui bloquait la PREMIÈRE peinture. Le cache est tenu à jour en arrière-plan par
    // refreshPairingAlert (et par les vues Organiser/Projets) ; showApp re-calcule la mise en route une
    // fois ce rafraîchissement réseau revenu. Au tout premier lancement, 0 → on ne bride pas (fail-open).
    const pending = state.pendingPairings || 0;
    const s2 = pending === 0;

    let kernels = [];
    try { kernels = (await App().Kernels()) || []; } catch (e) {}
    const s3 = kernels.some((k) => k && k.run_file);

    let spokes = [], backups = [], clouds = [];
    try { spokes = (await App().Spokes()) || []; } catch (e) {}
    try { backups = (await App().Backups()) || []; } catch (e) {}
    try { clouds = (await App().FileClouds()) || []; } catch (e) {}

    // s4/s5/hasClouds : EXISTENCE des ressources de domaine → dévoilement progressif des menus (1.18.5).
    state.setup = { s1, s2, s3, s4: spokes.length > 0, s5: backups.length > 0, hasClouds: clouds.length > 0, done: s1 && s2 && s3 };
  } catch (e) {
    state.setup = { done: true }; // fail-open : ne jamais brider sur erreur de lecture
  }
  applySetupBridage();
}

// applySetupBridage : pendant la mise en route, masque les menus de domaine et pose un accès « Mise en
// route » en tête de barre. Une fois terminé, restaure les menus selon les modules.
function applySetupBridage() {
  const s = state.setup || {};
  const brid = s.done === false;
  if (typeof applyModules === "function") applyModules();
  if (brid) {
    // Dévoilement PROGRESSIF (1.18.5) : on ne masque QUE les domaines encore VIDES (le prérequis présent
    // les révèle). BACKUP ⟸ CORPUS : révélé dès qu'un plan OU un cloud existe (1.19).
    const hideIfEmpty = (view, has) => { const b = document.querySelector(`.nav-item[data-view="${view}"]`); if (b && !has) b.classList.add("hidden"); };
    hideIfEmpty("spokes", s.s4);
    hideIfEmpty("backup", s.s5 || s.hasClouds);
    hideIfEmpty("corpus", s.hasClouds);
  }
  renderSetupNav();
}

// renderSetupNav : entrée PERMANENTE « Actions nécessaires » en bas de la barre (près des pastilles
// d'état), à 3 états (#27, 1.19) : REQUIS (orange clignotant + compte), FACULTATIF ouvert (sobre),
// À JOUR (discret). On la voit quand il y a à faire ; sobre et smart sinon.
function renderSetupNav() {
  const nav = document.querySelector(".sidebar");
  if (!nav) return;
  const s = state.setup || {};
  const todoReq = [s.s1, s.s2, s.s3].filter((x) => x === false).length;
  const optionalOpen = (state.pendingPairings || 0) > 0;
  let mode, label, badge = "";
  if (todoReq > 0) {
    mode = "req"; label = "Actions nécessaires";
    badge = `<span class="nav-setup-badge" aria-label="${todoReq} action${todoReq > 1 ? "s" : ""} requise${todoReq > 1 ? "s" : ""}">${todoReq}</span>`;
  } else if (optionalOpen) {
    mode = "opt"; label = "Actions facultatives";
  } else {
    mode = "done"; label = "Actions à jour";
  }
  let it = document.getElementById("nav-setup");
  if (!it) {
    it = document.createElement("button");
    it.id = "nav-setup"; it.setAttribute("data-view", "setup");
    it.onclick = () => navTo("setup");
    // Juste AU-DESSUS des pastilles d'état (Git/Cloud) : on l'insère en tête du pied de barre.
    const foot = nav.querySelector(".sidebar-foot");
    if (foot) foot.insertBefore(it, foot.firstChild); else nav.appendChild(it);
  }
  it.className = `nav-item nav-setup nav-setup-${mode}`;
  it.innerHTML = `<span class="nav-ico" aria-hidden="true">${ico(mode === "done" ? "check" : "warning", 15)}</span>${label}${badge}`;
}

// setupActive : reste-t-il une action nécessaire ? (pilote la présence de l'entrée de menu + le masquage
// des domaines VIDES). Vrai tant que les requis ne sont pas tous faits.
function setupActive() { return !!(state.setup && state.setup.done === false); }

// setupMustLand : faut-il ATTERRIR de force sur « Actions nécessaires » ? Seulement pour le nouvel arrivant
// dont les bases manquent (atelier ou KERNEL). S'il ne reste qu'un appairage (bases faites), on NE force
// PAS : l'entrée de menu signale l'action, le user en piste reste libre de son écran (1.18.5).
function setupMustLand() { const s = state.setup || {}; return s.done === false && (!s.s1 || !s.s3); }

// su-card : une étape = une CARD autonome (1.18.1). Un ordre est suggéré (numéro), mais aucune
// chronologie n'est forcée : toutes les cards sont actionnables à tout moment. st ∈ done|todo|opt.
function suCard(o) {
  const st = o.st;
  const badge = st === "done" ? `<span class="su-badge ok">fait</span>`
    : o.optional ? `<span class="su-badge opt">facultatif</span>` : `<span class="su-badge req">requis</span>`;
  // Numéro seulement pour les cards REQUISES (elles portent l'ordre) ; ✓ quand c'est fait ; rien pour
  // les facultatives (elles se distinguent par le cadre sobre + le badge).
  const num = st === "done" ? `<span class="su-num">✓</span>` : o.optional ? "" : `<span class="su-num">${o.n}</span>`;
  return `<div class="su-card ${st}${o.optional ? " su-card-opt" : ""}">
    <div class="su-card-top">
      ${num}
      <span class="su-card-ico">${o.ico}</span>
      <b class="su-card-t">${esc(o.title)}</b>
      ${badge}
    </div>
    <div class="su-card-d">${o.desc}</div>
    ${o.body ? `<div class="su-card-act">${o.body}</div>` : ""}
  </div>`;
}

async function renderSetup() {
  await refreshSetup();
  const s = state.setup || {};
  const kico = (typeof NAV_ICO !== "undefined" ? NAV_ICO.kernel : "");
  const pending = state.pendingPairings || 0;
  const hasVit = state._setupHasVitrine, vitA = state._setupVitAnswer;

  // Carte VITRINE — séparée de l'atelier, facultative, TOUJOURS révisable.
  let vitSt, vitDesc, vitBody;
  if (hasVit) {
    vitSt = "done"; vitDesc = "Une connexion <b>vitrine</b> (production) est en place. Vos versions validées pourront y être publiées.";
    vitBody = `<button class="btn btn-ghost btn-sm" data-go="addconn">Gérer mes connexions</button>`;
  } else if (vitA === "yes") {
    vitSt = "todo"; vitDesc = "Vous aurez une <b>vitrine</b> : connectez un git de <b>production</b> (séparé de l'atelier).";
    vitBody = `<button class="btn btn-primary btn-sm" data-go="addconn">${ico("plus", 14)} Connecter une vitrine</button>
      <button class="btn btn-ghost btn-sm" data-vit="no">Changer d'avis : finalement non</button>`;
  } else if (vitA === "no") {
    vitSt = "opt"; vitDesc = "Pas de vitrine pour l'instant, c'est très bien. <span class=\"muted\">Vous pourrez revenir sur ce choix quand vous voulez.</span>";
    vitBody = `<button class="btn btn-ghost btn-sm" data-vit="yes">Finalement oui, j'en veux une</button>`;
  } else {
    vitSt = "todo"; vitDesc = "Aurez-vous une <b>vitrine</b> ? Un dépôt de <b>production</b>, séparé de l'atelier, où l'on publie les versions validées.";
    vitBody = `<button class="btn btn-primary btn-sm" data-vit="yes">Oui, j'en aurai une</button>
      <button class="btn btn-ghost btn-sm" data-vit="no">Non, pas pour l'instant</button>`;
  }

  // Carte APPAIRER — compteur « il vous reste… », ou invite à CRÉER quand tout est appairé.
  const pairDesc = pending > 0
    ? `Appairez les dépôts alignés, complétez ou archivez les autres.`
    : "Tout est appairé. Créez un nouveau dépôt si besoin.";
  // Pas de blabla : on réutilise la MÊME pastille « ⚠ XX à appairer » que le header (clic → Organiser).
  const pairBody = pending > 0
    ? `<button class="hpill orange" id="su-pair" title="Appairer vos dépôts (local ↔ atelier ↔ vitrine)"><span class="hp-dot"></span>⚠ ${pending} à appairer</button>`
    : `<button class="btn btn-primary btn-sm" id="su-create">${ico("plus", 14)} Créer un dépôt</button>
       <button class="btn btn-ghost btn-sm" data-go="organize">${ico("puzzle", 14)} Organiser mes dépôts</button>`;

  // Hero adapté au moment (#27, 1.19) : REQUIS (orange), FACULTATIF ouvert (accent), TOUT À JOUR (vert).
  const todoReq = [s.s1, s.s2, s.s3].filter((x) => x === false).length;
  let hMode, hTitle, hSub, hIco;
  if (todoReq > 0) {
    hMode = "req"; hIco = ico("warning", 20); hTitle = "Actions nécessaires";
    hSub = "Des actions de votre part sont requises pour le bon fonctionnement de SituLab. Traitez-les dans l'ordre que vous voulez.";
  } else if (pending > 0) {
    hMode = "opt"; hIco = ico("puzzle", 20); hTitle = "Actions facultatives ouvertes";
    hSub = "Les bases sont en place. Il reste des options à compléter si vous le souhaitez, rien d'obligatoire.";
  } else {
    hMode = "done"; hIco = ico("check", 20); hTitle = "Tout est à jour";
    hSub = "Rien à faire : votre SituLab est prêt. Revenez ici quand vous voulez.";
  }
  el("view").innerHTML = `
    <div class="su-head su-head-${hMode}">
      <span class="su-head-ico">${hIco}</span>
      <div class="su-head-txt"><div class="view-title">${hTitle}</div>
        <div class="view-sub">${hSub}</div></div>
    </div>
    <div class="su-section-h su-section-req"><span class="su-section-dot req"></span>Requis — en priorité</div>
    <div class="su-cards">
      ${suCard({ n: 1, st: s.s1 ? "done" : "todo", ico: "🛠️", title: "Connecter un atelier",
        desc: "Ajoutez au moins une connexion git : votre <b>atelier</b> de travail (Forgejo ou GitHub).",
        body: s.s1 ? `<button class="btn btn-ghost btn-sm" data-go="addconn">Gérer mes connexions</button>` : `<button class="btn btn-primary btn-sm" data-go="addconn">${ico("plus", 14)} Ajouter une connexion</button>` })}
      ${suCard({ n: 2, st: s.s2 ? "done" : "todo", ico: ico("puzzle", 26), title: "Appairer vos dépôts", desc: pairDesc, body: pairBody })}
      ${suCard({ n: 3, st: s.s3 ? "done" : "todo", ico: kico || ico("repo", 26), title: "Désigner un KERNEL",
        desc: "Choisissez le dépôt qui <b>EST</b> votre mémoire, puis son <b>fichier d'ouverture</b> (ce que votre IA lit en premier).",
        body: s.s3 ? `<button class="btn btn-ghost btn-sm" data-go="kernel">Gérer le KERNEL</button>` : `<button class="btn btn-primary btn-sm" data-go="kernel"><span class="btn-ico">${kico}</span> Aller à KERNEL</button>` })}
    </div>
    <div class="su-section-h su-section-opt"><span class="su-section-dot opt"></span>Facultatif — à tout moment</div>
    <div class="su-cards">
      ${suCard({ st: vitSt, optional: true, ico: ico("globe", 26), title: "Vitrine (production)", desc: vitDesc, body: vitBody })}
      ${suCard({ st: s.s4 ? "done" : "opt", optional: true, ico: "🛰", title: "Désigner des spokes",
        desc: "Des projections de mission rattachées à un KERNEL, pour organiser vos chantiers.",
        body: `<button class="btn btn-ghost btn-sm" data-go="spokes">Aller à SPOKES</button>` })}
      ${suCard({ st: s.s5 ? "done" : "opt", optional: true, ico: ico("shield", 26), title: "Plan de sauvegarde",
        desc: "Sauvegardez vos dépôts vers un cloud souverain, en clé-en-main.",
        body: `<button class="btn btn-ghost btn-sm" data-go="backup">Aller à BACKUP</button>` })}
    </div>
    <div class="su-foot">
      ${s.done ? `<button class="btn btn-primary" id="su-enter">✓ Entrer dans SituLab</button>` : ""}
    </div>`;

  el("view").querySelectorAll("[data-go]").forEach((b) => b.onclick = () => {
    const g = b.dataset.go;
    if (g === "addconn") { state.settabTarget = "conn"; navTo("settings"); }
    else navTo(g); // organize | kernel | spokes | backup
  });
  el("view").querySelectorAll("[data-vit]").forEach((b) => b.onclick = () => {
    try { localStorage.setItem("situlab.setup.vitrine", b.dataset.vit); } catch (e) {}
    renderSetup();
  });
  if (el("su-create")) el("su-create").onclick = () => createRepoTunnel();
  if (el("su-pair")) el("su-pair").onclick = () => navTo("organize");
  if (el("su-enter")) el("su-enter").onclick = () => navTo("kernel");
}

// createRepoTunnel : « Créer un dépôt » — une modale à CARDS qui demande OÙ (1.18.1). Trois voies
// autonomes : sur l'atelier, sur la vitrine (avec la liste des connexions du bon rôle), ou récupérer
// un dépôt existant sur ce poste (catalogue). Disponible depuis la Mise en route ET « Organiser mes dépôts ».
function createRepoTunnel() {
  const ateliers = (state.instances || []).filter((i) => i.role !== "production");
  const vitrines = (state.instances || []).filter((i) => i.role === "production");
  const opts = (list) => list.map((c) => `<option value="${esc(c.name)}">${esc(c.name)}</option>`).join("");
  openModal("Créer un dépôt", `<div class="field gr-choices">
    <p class="sub">Où voulez-vous créer ce dépôt ?</p>
    <div class="gr-card gr-card-primary">
      <div class="gr-card-h"><span class="gr-card-ico">🛠️</span><b>Un dépôt atelier</b></div>
      <div class="gr-card-d">Votre dépôt de travail, sur un de vos serveurs atelier.</div>
      ${ateliers.length ? `<label class="gr-field">Connexion atelier<select id="crt-atl" class="select">${opts(ateliers)}</select></label>` : `<div class="gr-hint muted">Aucune connexion atelier, ajoutez-en une dans Réglages ▸ Connexions git.</div>`}
      <button type="button" class="btn btn-primary gr-cta" id="crt-atl-go"${ateliers.length ? "" : " disabled"}>Créer sur l'atelier</button>
    </div>
    <div class="gr-card">
      <div class="gr-card-h"><span class="gr-card-ico">${ico("globe", 16)}</span><b>Un dépôt vitrine</b></div>
      <div class="gr-card-d">Votre dépôt de production, séparé de l'atelier.</div>
      ${vitrines.length ? `<label class="gr-field">Connexion vitrine<select id="crt-vit" class="select">${opts(vitrines)}</select></label>` : `<div class="gr-hint muted">Aucune connexion vitrine, ajoutez-en une dans Réglages ▸ Connexions git.</div>`}
      <button type="button" class="btn btn-primary gr-cta" id="crt-vit-go"${vitrines.length ? "" : " disabled"}>Créer sur la vitrine</button>
    </div>
    <div class="gr-card">
      <div class="gr-card-h"><span class="gr-card-ico">${ico("desktop", 16)}</span><b>Récupérer un dépôt sur ce poste</b></div>
      <div class="gr-card-d">Un dépôt existe déjà en ligne ? Clonez-le sur ce poste depuis le catalogue.</div>
      <button type="button" class="btn btn-ghost gr-cta" id="crt-here">Aller au catalogue des dépôts</button>
    </div></div>`, null);
  if (el("modal-ok")) el("modal-ok").classList.add("hidden");
  if (el("modal-cancel")) el("modal-cancel").textContent = "Annuler";
  if (el("crt-atl-go")) el("crt-atl-go").onclick = () => { const inst = el("crt-atl") ? el("crt-atl").value : ""; closeModal(); createRepoModal({ instance: inst }); };
  if (el("crt-vit-go")) el("crt-vit-go").onclick = () => { const inst = el("crt-vit") ? el("crt-vit").value : ""; closeModal(); createRepoModal({ instance: inst }); };
  if (el("crt-here")) el("crt-here").onclick = () => { closeModal(); navTo("git"); };
}
