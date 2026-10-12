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

