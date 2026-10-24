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
    let vitAnswered = false;
    try { vitAnswered = localStorage.getItem("situlab.setup.vitrine") != null; } catch (e) {}
    // Réponse IMPLICITE : si une connexion vitrine (production) existe déjà, la question est tranchée.
    const hasVitrine = (state.instances || []).some((i) => i.role === "production");
    const s1 = insts > 0 && (vitAnswered || hasVitrine);
    state._setupHasVitrine = hasVitrine;

    // Appairages en attente : on lit la DERNIÈRE valeur connue (cache) au lieu d'appeler RepoMatrix
    // (= réseau), qui bloquait la PREMIÈRE peinture. Le cache est tenu à jour en arrière-plan par
    // refreshPairingAlert (et par les vues Organiser/Projets) ; showApp re-calcule la mise en route une
    // fois ce rafraîchissement réseau revenu. Au tout premier lancement, 0 → on ne bride pas (fail-open).
    const pending = state.pendingPairings || 0;
    const s2 = pending === 0;

    let kernels = [];
    try { kernels = (await App().Kernels()) || []; } catch (e) {}
    const s3 = kernels.some((k) => k && k.run_file);

    let spokes = [], backups = [];
    try { spokes = (await App().Spokes()) || []; } catch (e) {}
    try { backups = (await App().Backups()) || []; } catch (e) {}

    state.setup = { s1, s2, s3, s4: spokes.length > 0, s5: backups.length > 0, done: s1 && s2 && s3 };
  } catch (e) {
    state.setup = { done: true }; // fail-open : ne jamais brider sur erreur de lecture
  }
  applySetupBridage();
}

// applySetupBridage : pendant la mise en route, masque les menus de domaine et pose un accès « Mise en
// route » en tête de barre. Une fois terminé, restaure les menus selon les modules.
function applySetupBridage() {
  const brid = !!(state.setup && state.setup.done === false);
  if (brid) {
    ["corpus", "spokes", "backup"].forEach((v) => { const b = document.querySelector(`.nav-item[data-view="${v}"]`); if (b) b.classList.add("hidden"); });
    if (!document.getElementById("nav-setup")) {
      const nav = document.querySelector(".sidebar");
      if (nav) {
        const it = document.createElement("button");
        it.id = "nav-setup"; it.className = "nav-item nav-setup active"; it.setAttribute("data-view", "setup");
        it.innerHTML = `<span class="nav-ico" aria-hidden="true">🚀</span>Mise en route`;
        it.onclick = () => navTo("setup");
        nav.insertBefore(it, nav.firstChild);
      }
    }
  } else {
    const it = document.getElementById("nav-setup"); if (it) it.remove();
    if (typeof applyModules === "function") applyModules(); // restaure les menus selon les modules
  }
}

// setupActive : la mise en route bride-t-elle encore ? (helper lisible)
function setupActive() { return !!(state.setup && state.setup.done === false); }

async function renderSetup() {
  await refreshSetup();
  const s = state.setup || {};
  let vit = null;
  try { vit = localStorage.getItem("situlab.setup.vitrine"); } catch (e) {}

  const step = (n, done, optional, title, desc, actions) => `
    <div class="su-step ${done ? "done" : (optional ? "opt" : "todo")}">
      <div class="su-rail"><div class="su-dot">${done ? "✓" : n}</div></div>
      <div class="su-body">
        <div class="su-h">${esc(title)}${optional ? ' <span class="su-tag opt">facultatif</span>' : ' <span class="su-tag req">obligatoire</span>'}${done ? ' <span class="su-tag ok">fait</span>' : ""}</div>
        <div class="su-d">${desc}</div>
        ${done ? "" : `<div class="su-act">${actions}</div>`}
      </div>
    </div>`;

  const vitQ = (vit != null || state._setupHasVitrine) ? "" : `<div class="su-vit callout">
    <div><b>Aurez-vous une vitrine ?</b> Un dépôt de <b>production</b>, séparé de l'atelier, où l'on publie les versions validées.</div>
    <div class="su-vit-btns"><button class="btn btn-primary btn-sm" data-vit="yes">Oui, j'en aurai une</button>
    <button class="btn btn-ghost btn-sm" data-vit="no">Non, pas pour l'instant</button></div></div>`;

  el("view").innerHTML = `
    <div class="view-head"><div class="view-title">🚀 Mise en route</div>
      <div class="view-sub">Quelques étapes pour démarrer. Les <b>obligatoires</b> (1 à 3) débloquent tous les menus de SituLab.</div></div>
    <div class="su-timeline">
      ${step(1, s.s1, false, "Connecter un atelier", "Ajoutez au moins une connexion git (votre <b>atelier</b> de travail), et dites si vous aurez une vitrine.",
        `<button class="btn btn-primary btn-sm" data-go="addconn">＋ Ajouter une connexion</button>${vitQ}`)}
      ${step(2, s.s2, false, "Appairer vos dépôts", "Reliez vos copies <b>locales</b> à leur atelier (et vitrine). Appairez les suggestions trouvées, ou rangez celles qui ne vous servent pas.",
        `<button class="btn btn-primary btn-sm" data-go="organize">🧩 Organiser mes dépôts</button>`)}
      ${step(3, s.s3, false, "Désigner un KERNEL", "Choisissez le dépôt qui <b>EST</b> votre mémoire, puis son <b>fichier d'ouverture</b> (ce que votre IA lit en premier) — SituLab vous le proposera juste après.",
        `<button class="btn btn-primary btn-sm" data-go="kernel"><span class="btn-ico">${(typeof NAV_ICO !== "undefined" ? NAV_ICO.kernel : "")}</span> Aller à KERNEL</button>`)}
      ${step(4, s.s4, true, "Désigner des spokes", "Des projections de mission rattachées à un KERNEL — pour organiser vos chantiers.",
        `<button class="btn btn-ghost btn-sm" data-go="spokes">🛰 Aller à SPOKES</button>`)}
      ${step(5, s.s5, true, "Créer un plan de sauvegarde", "Sauvegardez vos dépôts vers un cloud souverain, en clé-en-main.",
        `<button class="btn btn-ghost btn-sm" data-go="backup">🛡 Aller à BACKUP</button>`)}
    </div>
    <div class="su-foot">
      ${s.done
        ? `<button class="btn btn-primary" id="su-enter">✓ C'est parti — entrer dans SituLab</button>`
        : `<div class="callout">Terminez les étapes <b>obligatoires</b> (1 à 3) pour débloquer tous les menus. Les facultatives peuvent se faire plus tard.</div>`}
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
  if (el("su-enter")) el("su-enter").onclick = () => navTo("kernel");
}
