/* ---------------- Détail ---------------- */
function openDetail(r) {
  if (!r || (!r.has_remote && !r.local_path)) return;
  const { owner, repo } = r.has_remote ? splitFull(r.full_name) : { owner: "", repo: r.name };
  state.detail = {
    full_name: r.full_name || r.name, owner, repo, has_remote: r.has_remote,
    html_url: r.html_url || "", local_path: r.local_path || "", ref: r.default_branch || "", tab: "apercu",
    path: "", refs: [], tags: [], _remote: null, _local: null,
    // D'où l'on vient, pour que « Retour » y ramène (fiche projet, organiseur…).
    from: r.from || "organize", fromProject: r.fromProject || "",
  };
  state.view = "detail";
  render();
}

async function renderDetail() {
  const d = state.detail; if (!d) return;
  el("view").innerHTML = `
    <div class="detail-head">
      <button class="btn-back" id="back">‹ Retour</button>
      <div class="detail-title">${repoTitleHTML(d.full_name || d.repo)}</div>
      <div class="detail-actions">
        ${d.has_remote ? `<label class="sr-only" for="ref-sel">Branche ou version</label><select id="ref-sel" class="select ref-sel" title="Branche ou version affichée : change le contenu des onglets Fichiers / Historique"></select>` : ""}
        ${d.has_remote ? `<button class="btn btn-ghost btn-sm" id="open-web" title="Ouvrir ce dépôt dans le navigateur (page du serveur git en ligne)">Ouvrir en ligne ↗</button>` : ""}
        ${d.local_path && state.advanced && !state.readOnly ? `<button class="btn btn-ghost btn-sm" id="git-local" title="Gérer le dépôt sur CE poste : branches, fusions locales, copies de travail (worktrees)">⎇ Git local</button>` : ""}
        ${d.local_path ? `<button class="btn btn-ghost btn-sm" id="unfollow" title="Retirer ce dépôt du suivi SituLab — aucun fichier n'est supprimé sur le poste">Ne plus suivre</button>` : ""}
        ${d.has_remote && state.advanced && !state.readOnly ? `<button class="btn btn-ghost btn-sm" id="repo-settings" title="Réglages du dépôt EN LIGNE : nom, branche par défaut, visibilité, archivage">${ico("gear", 14)} Réglages</button>` : ""}
        ${d.has_remote ? `<button class="btn btn-ghost btn-sm btn-danger" id="del-repo" title="Supprimer le dépôt EN LIGNE (destructif — confirmation + PIN requis)">Supprimer</button>` : ""}
      </div>
    </div>
    <div class="tabs">
      <button class="tab${(d.tab || "apercu") === "apercu" ? " active" : ""}" data-tab="apercu">Aperçu</button>
      ${d.has_remote ? `<button class="tab${d.tab === "fichiers" ? " active" : ""}" data-tab="fichiers">Fichiers</button><button class="tab${d.tab === "histo" ? " active" : ""}" data-tab="histo">Historique</button><button class="tab${d.tab === "activite" ? " active" : ""}" data-tab="activite">Activité</button>` : ""}
    </div>
    <div id="detail-body"><div class="skeleton"></div></div>`;
  el("back").onclick = () => {
    // Retour contextuel (1.12) : vers la fiche projet si on en vient, sinon l'organiseur.
    if (d.from === "project" && d.fromProject && typeof openProject === "function") return openProject(d.fromProject);
    navTo(d.from || "organize");
  };
  if (el("open-web")) el("open-web").onclick = () => safeOpenURL(repoWebURL(d));
  if (el("unfollow")) el("unfollow").onclick = () => {
    // « Ne plus suivre » : retire le clone local du suivi SituLab. AUCUN fichier n'est supprimé.
    confirmDelete({
      title: "Ne plus suivre ce dépôt ?",
      lead: `SituLab cessera d'afficher le clone local <b>${esc(d.local_path)}</b>.`,
      items: ["Le suivi de ce clone dans SituLab (l'appairage)"],
      safe: ["Aucun fichier n'est supprimé : le dossier reste sur votre poste", "Vous pourrez le resuivre plus tard"],
      confirmLabel: "Ne plus suivre",
      onConfirm: async () => { await App().RemoveLocalRepo(d.local_path); toast("Dépôt retiré du suivi", "ok"); navTo("organize"); },
    });
  };
  if (el("git-local")) el("git-local").onclick = () => gitLocalModal(d);
  if (el("repo-settings")) el("repo-settings").onclick = () => repoSettingsModal(d);
  if (el("del-repo")) el("del-repo").onclick = () => deleteRepoModal(d.owner, d.repo);
  document.querySelectorAll(".tab").forEach((t) => {
    t.onclick = () => {
      document.querySelectorAll(".tab").forEach((x) => x.classList.remove("active"));
      // Réinitialiser l'état « diff ouvert » en changeant d'onglet : sinon il reste vrai (il n'est remis
      // à faux que par le bouton « retour » du diff) et bloque le rafraîchissement live du nouvel onglet.
      t.classList.add("active"); d.tab = t.dataset.tab; d.path = ""; d._diffOpen = false; paintDetail();
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
      // P9 : union avec les branches LOCALES (dont la branche courante non encore poussée, ex.
      // work/1.18.2) — sinon elles manquent au sélecteur alors que main/dev y sont.
      let localOnly = [];
      if (d.local_path) {
        try {
          const bm = await App().BranchMatrix(d.local_path);
          if (state.detail !== d) return;
          const names = (bm || []).map((r) => r.name || r.branch || r.Name || "").filter(Boolean);
          const remoteSet = new Set(d.refs);
          localOnly = [...new Set(names)].filter((n) => !remoteSet.has(n));
        } catch (_) {}
      }
      d._localOnly = new Set(localOnly); // réfs locales-seules : on n'ira pas les lire en ligne (404)
      const sel = el("ref-sel");
      if (sel) {
        const grp = (g, arr) => arr.length ? `<optgroup label="${g}">` + arr.map((x) => `<option value="${esc(x)}"${x === d.ref ? " selected" : ""}>${esc(x)}</option>`).join("") + `</optgroup>` : "";
        sel.innerHTML = grp("Branches", d.refs) + grp("Branches locales (pas encore en ligne)", localOnly) + grp("Versions", d.tags);
        sel.onchange = (e) => { d.ref = e.target.value; d.path = ""; paintDetail(); refreshDetailRemote(); };
      }
    } catch (e) {}
  }
  await refreshDetailRemote();
  await refreshDetailLocal();
  if (state.detail === d) paintDetail();
}

// commitsKey : clé de cache des commits distants (par connexion + dépôt + ref).
function commitsKey(d) { return "commits:" + state.instance + ":" + d.owner + "/" + d.repo + ":" + d.ref; }
async function refreshDetailRemote(fresh) {
  const d = state.detail; if (!d || !d.has_remote) return;
  // P9 : une branche LOCALE-seule n'existe pas en ligne → on n'interroge pas le serveur (évite un 404)
  // et on affiche un état « pas encore en ligne » plutôt qu'une erreur.
  if (d._localOnly && d._localOnly.has(d.ref)) { d._remote = { commits: [], localOnly: true }; if ((d.tab === "apercu" || d.tab === "histo") && !d._diffOpen) paintDetail(true); return; }
  // Réactivité (1.18.3) : commits via swr — on peint d'abord les derniers connus (mémoire→disque) puis
  // on rafraîchit en fond. fresh=true (après un geste git) invalide d'abord pour afficher l'état réel.
  if (fresh) swrDrop(commitsKey(d));
  try {
    await swr(commitsKey(d), () => App().RepoCommits(state.instance, d.owner, d.repo, d.ref, 20),
      (commits) => {
        if (state.detail !== d) return;
        d._remote = { commits: commits || [] };
        if ((d.tab === "apercu" || d.tab === "histo") && !d._diffOpen) paintDetail(true);
      },
      { alive: () => state.detail === d });
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
  body.innerHTML = scopeNoteHTML(d, "activite") + `<p class="detail-lead">Le <b>suivi en ligne</b> de ce dépôt : signalez des tâches à faire (<b>issues</b>), proposez de réunir des branches après relecture (<b>demandes de fusion</b>), et publiez des <b>versions</b> stables étiquetées.</p><div class="toolbar">
      <button class="btn btn-primary btn-sm" id="new-issue" title="Une issue = une note de suivi : un bug à corriger, une idée, une question. Elle reste ouverte jusqu'à ce qu'on la ferme.">${ico("plus", 14)} Nouvelle issue</button>
      <button class="btn btn-ghost btn-sm" id="new-pr" title="Une demande de fusion propose de réunir le travail d'une branche dans une autre, pour relecture avant d'intégrer.">${ico("plus", 14)} Demande de fusion</button>
      <button class="btn btn-ghost btn-sm" id="new-rel" title="Une version (release) marque un point stable, étiqueté (ex. v1.0), à publier.">${ico("plus", 14)} Version</button>
      <span class="act-help muted">Issues = notes de suivi · Fusions = réunir une branche après relecture · Versions = points stables publiés.</span>
    </div><div id="act"><div class="skeleton"></div></div>`;
  el("new-issue").onclick = () => newIssueModal(d);
  el("new-pr").onclick = () => newPullModal(d);
  el("new-rel").onclick = () => newReleaseModal(d);
  // Réactivité (1.18.3) : issues/fusions/versions via swr — on peint le dernier état connu puis on
  // rafraîchit en fond. refreshActivite() (après un geste) invalide d'abord pour afficher le frais.
  const paint = (data) => {
    if (state.detail !== d) return;
    const issues = data.issues || [], pulls = data.pulls || [], releases = data.releases || [];
    const act = el("act"); if (!act) return;
    const list = (arr, fn) => arr.length ? arr.map(fn).join("") : `<p class="muted">Rien pour le moment.</p>`;
    const acts = (n, kind) => `<span class="srow-acts">
      <button class="btn btn-ghost btn-sm" data-act="comment" data-n="${n}">Commenter</button>
      ${kind === "issue" ? `<button class="btn btn-ghost btn-sm" data-act="close" data-n="${n}">Fermer</button>`
        : `<button class="btn btn-ghost btn-sm" data-act="review" data-n="${n}">Revue</button><button class="btn btn-ghost btn-sm" data-act="merge" data-n="${n}">Fusionner</button>`}</span>`;
    act.innerHTML = `
      <div class="blk"><div class="blk-h">Issues ouvertes (${issues.length})</div>
        ${list(issues, (i) => `<div class="srow"><div><b>#${i.number}</b> ${esc(i.title)} <span class="muted">${esc(i.user && i.user.login || "")}</span></div>${acts(i.number, "issue")}</div>`)}</div>
      <div class="blk"><div class="blk-h">Demandes de fusion ouvertes (${pulls.length})</div>
        ${list(pulls, (p) => `<div class="srow"><div><b>#${p.number}</b> ${esc(p.title)} <span class="muted">${esc(p.user && p.user.login || "")}</span></div>${acts(p.number, "pr")}</div>`)}</div>
      <div class="blk"><div class="blk-h">Versions publiées (${releases.length})</div>
        <div class="tags">${releases.length ? releases.map((r) => `<span class="tag-chip">${esc(r.tag_name)}</span>`).join("") : `<span class="muted">aucune</span>`}</div></div>`;
    act.classList.add("sl-settle");
    act.onclick = (ev) => {
      const b = ev.target.closest("[data-act]"); if (!b) return;
      const n = +b.dataset.n;
      if (b.dataset.act === "comment") commentIssueModal(d, n);
      else if (b.dataset.act === "close") closeIssueConfirm(d, n);
      else if (b.dataset.act === "merge") mergePullModal(d, n);
      else if (b.dataset.act === "review") pullReviewModal(d, n);
    };
  };
  try {
    await swr(actKey(d),
      () => Promise.all([
        App().RepoIssues(state.instance, d.owner, d.repo, "open").catch(() => []),
        App().RepoPulls(state.instance, d.owner, d.repo, "open").catch(() => []),
        App().RepoReleases(state.instance, d.owner, d.repo).catch(() => []),
      ]).then(([issues, pulls, releases]) => ({ issues, pulls, releases })),
      paint, { alive: () => state.detail === d && d.tab === "activite" });
  } catch (e) { const act = el("act"); if (act) act.innerHTML = surfaceError("Activité indisponible", cleanErr(e), null); }
}

// actKey : clé de cache de l'onglet Activité (par connexion + dépôt). refreshActivite : après un
// geste (commenter/fermer/fusionner/publier…), on invalide puis on repeint pour afficher le frais.
function actKey(d) { return "act:" + state.instance + ":" + d.owner + "/" + d.repo; }
function refreshActivite(d) { swrDrop(actKey(d)); paintActivite(el("detail-body")); }

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
      closeModal(); toast("Commentaire ajouté", "ok"); refreshActivite(d);
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => `${aiHeader("Commente une issue/PR")}\n\nCommande : situlab issue comment ${d.owner}/${d.repo} ${n} --body "<texte>"`);
}

// closeIssueConfirm : fermer une issue (PIN).
function closeIssueConfirm(d, n) {
  pinConfirm(`Fermer l'issue #${n}`, async (pin) => {
    await App().SetIssueState(state.instance, d.owner, d.repo, n, "closed", pin);
    toast("Issue fermée", "ok"); refreshActivite(d);
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
      closeModal(); toast("Demande de fusion fusionnée", "ok"); refreshActivite(d);
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
      closeModal(); toast("Demande de fusion ouverte", "ok"); refreshActivite(d);
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
        closeModal(); toast("Revue déposée", "ok"); refreshActivite(d);
      });
    });
    const cb = el("pr-close"); if (cb) cb.onclick = () => pinConfirm(`Fermer la PR #${n}`, async (pin) => {
      await App().SetPullState(state.instance, d.owner, d.repo, n, "closed", pin);
      closeModal(); toast("PR fermée", "ok"); refreshActivite(d);
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
      closeModal(); toast("Version publiée", "ok"); refreshActivite(d);
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(() => `${aiHeader("Publie une version")}\n\nCommande : situlab release create ${d.owner}/${d.repo} "<tag>" --name "<titre>" --body "<notes>"`);
}

// gitLocalModal : opérations git LOCALES sur le clone (branches, annuler, stash). Pas de PIN
// (aucun accès distant) ; « Annuler les modifications » est destructif → confirmation explicite.
function gitLocalModal(d) {
  const path = d.local_path;
  infoModal("Git local : " + esc(shortPath(path)), `<div class="field">
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
  openModal(`Réglages : ${esc(d.full_name || d.repo)}`, `<div class="field">
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
  el("rs-rename-btn").onclick = () => { const v = el("rs-name").value.trim(); if (!v) return; pinConfirm(`Renommer en « ${v} »`, async (pin) => { await App().EditRepoSettings(state.instance, d.owner, d.repo, { name: v }, pin); after("Dépôt renommé"); navTo("organize"); }); };
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
      closeModal(); toast("Issue créée", "ok"); refreshActivite(d);
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

  body.innerHTML = scopeNoteHTML(d, "apercu") + syncHero(d, loc) + versionScale(d, loc) +
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
    await refreshDetailLocal(); await refreshDetailRemote(true); paintDetail();
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
      await refreshDetailLocal(); await refreshDetailRemote(true); paintDetail();
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
      await refreshDetailLocal(); await refreshDetailRemote(true); paintDetail();
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
      await refreshDetailLocal(); await refreshDetailRemote(true); paintDetail();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
}

// sortEntriesTree : dossiers d'abord, puis tri alphabétique — ordre commun à l'explorateur.
function sortEntriesTree(arr) {
  return (arr || []).slice().sort((a, b) => (a.type === b.type) ? a.name.localeCompare(b.name) : (a.type === "dir" ? -1 : 1));
}

// paintFiles (1.18.3) : l'onglet Fichiers passe par l'explorateur UNIQUE. Avec un clone local, l'arbre
// se peint INSTANTANÉMENT depuis le disque (hors-ligne), la date de dernière publication se remplit en
// différé (shimmer), puis la liste EN LIGNE réconcilie (doctrine « Fichiers = le dépôt en ligne »). Le
// cache persistant évite la boîte vide au lancement. d.path est tenu à jour pour l'ouverture de fichier.
async function paintFiles(body, liveOnly) {
  if (liveOnly) return;
  const d = state.detail;
  body.innerHTML = scopeNoteHTML(d, "fichiers") + `<div id="xp-files"></div>`;
  const hasLocal = !!d.local_path;
  mountExplorer(el("xp-files"), {
    mode: "browse",
    startPath: d.path || "",
    onNav: (p) => { d.path = p; },
    instant: hasLocal ? ((p) => App().LocalTreeAt(d.local_path, p).then(sortEntriesTree).catch(() => [])) : undefined,
    load: (p) => App().RepoContents(state.instance, d.owner, d.repo, p, d.ref).then(sortEntriesTree),
    dates: hasLocal ? ((paths) => App().LocalTreeDates(d.local_path, paths)) : undefined,
    totals: hasLocal ? ((p) => App().LocalTreeTotals(d.local_path, p)) : undefined,
    cacheKey: (p) => "files:" + state.instance + ":" + d.owner + "/" + d.repo + ":" + d.ref + ":" + p,
    onOpenFile: (e) => openFile(e),
    emptyText: "Dossier vide",
  });
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
// (inline + bloc), gras/italique, listes à puces ET numérotées, citations, TABLEAUX, liens http(s) et
// paragraphes — suffisant pour un README ou la fiche AGENTS.md (qui utilise des tableaux). NB : une
// seconde fonction mdToHtml (09-modals-utils) rend le même sous-ensemble pour les espaces « .doc »
// (Aide, Réglages) ; les deux doivent rester capables des tableaux pour un rendu cohérent de AGENTS.md.
function mdToHTML(src) {
  const esc2 = (s) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");
  const inline = (s) => esc2(s)
    .replace(/`([^`]+)`/g, (_, c) => "<code>" + c + "</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>")
    .replace(/\*([^*]+)\*/g, "<i>$1</i>")
    .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, (_, t, u) => `<a href="${u}" data-ext>${t}</a>`);
  const lines = String(src).replace(/\r\n/g, "\n").split("\n");
  const out = []; let inCode = false, inList = false, inOList = false, inTable = false; const buf = [];
  const flushP = () => { if (buf.length) { out.push("<p>" + inline(buf.join(" ")) + "</p>"); buf.length = 0; } };
  const closeList = () => { if (inList) { out.push("</ul>"); inList = false; } if (inOList) { out.push("</ol>"); inOList = false; } };
  const closeTable = () => { if (inTable) { out.push("</tbody></table>"); inTable = false; } };
  for (const ln of lines) {
    if (/^```/.test(ln)) { if (inCode) { out.push("</code></pre>"); inCode = false; } else { flushP(); closeList(); closeTable(); out.push('<pre class="md-code"><code>'); inCode = true; } continue; }
    if (inCode) { out.push(esc2(ln) + "\n"); continue; }
    // Tableau « | a | b | » : la ligne de séparation (---|:--:) est ignorée ; la 1re ligne fait l'en-tête.
    if (/^\s*\|.*\|\s*$/.test(ln)) {
      const cells = ln.trim().replace(/^\||\|$/g, "").split("|").map((c) => c.trim());
      if (cells.every((c) => /^:?-+:?$/.test(c))) continue;
      if (!inTable) { flushP(); closeList(); out.push('<table class="md-table"><tbody>'); inTable = true; }
      out.push("<tr>" + cells.map((c) => "<td>" + inline(c) + "</td>").join("") + "</tr>"); continue;
    }
    closeTable();
    const h = ln.match(/^(#{1,6})\s+(.*)$/);
    if (h) { flushP(); closeList(); const lvl = Math.min(6, h[1].length) + 2; out.push(`<h${lvl}>` + inline(h[2]) + `</h${lvl}>`); continue; }
    const bq = ln.match(/^\s*>\s?(.*)$/);
    if (bq) { flushP(); closeList(); out.push("<blockquote>" + inline(bq[1]) + "</blockquote>"); continue; }
    const oli = ln.match(/^\s*\d+\.\s+(.*)$/);
    if (oli) { flushP(); if (inList) { out.push("</ul>"); inList = false; } if (!inOList) { out.push("<ol>"); inOList = true; } out.push("<li>" + inline(oli[1]) + "</li>"); continue; }
    const li = ln.match(/^\s*[-*]\s+(.*)$/);
    if (li) { flushP(); if (inOList) { out.push("</ol>"); inOList = false; } if (!inList) { out.push("<ul>"); inList = true; } out.push("<li>" + inline(li[1]) + "</li>"); continue; }
    if (ln.trim() === "") { flushP(); closeList(); continue; }
    buf.push(ln);
  }
  if (inCode) out.push("</code></pre>");
  flushP(); closeList(); closeTable();
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
// Sceau INVALIDE / révoqué : même bouclier barré d'une croix (stroke --danger via .att-badge.bad) — un
// trailer PRÉSENT mais qui NE VÉRIFIE PAS cryptographiquement (forgé, révoqué, hors fenêtre) ne doit
// JAMAIS reprendre le visuel de la garantie verte : il porte son propre glyphe d'alerte.
const SEAL_BAD_SVG = `<svg class="seal" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 2.6l7 2.3v5.3c0 4.6-3 7.9-7 9.2-4-1.3-7-4.6-7-9.2V4.9z"/><path d="M9.2 9.2l5.6 5.6M14.8 9.2l-5.6 5.6"/></svg>`;

// stripAttestTrailer retire le trailer « SituLab-Approved: » du message affiché : il est représenté
// par le sceau, jamais montré en base64 brut.
function stripAttestTrailer(msg) {
  return String(msg || "").split("\n").filter((l) => !/^SituLab-Approved:\s/.test(l)).join("\n").replace(/\n+$/, "");
}

// scopeNoteHTML : dit CLAIREMENT ce que l'onglet montre (1.12). Fichiers/Historique/Activité = le dépôt
// EN LIGNE sélectionné (atelier ou vitrine selon d'où on a ouvert) ; Aperçu = comparaison en ligne ⟷ poste.
function scopeNoteHTML(d, kind) {
  if (kind === "apercu") return `<div class="detail-scope"><span class="ds-ico">⇄</span> en ligne ⟷ ce poste</div>`;
  const ref = d.ref ? ` · <b>${esc(d.ref)}</b>` : "";
  return `<div class="detail-scope"><span class="ds-ico">${ico("globe", 15)}</span> en ligne : ${esc(d.full_name || d.repo)}${ref}</div>`;
}

// hasAttestTrailer : le commit porte-t-il le sceau SituLab ? Le trailer « SituLab-Approved: » fait
// partie du message et voyage avec le commit — on le lit donc DIRECTEMENT dans le message renvoyé par
// le serveur, sans avoir besoin d'un clone local.
function hasAttestTrailer(msg) {
  return /^SituLab-Approved:\s/m.test(String(msg || ""));
}

function paintHisto(body, liveOnly) {
  const d = state.detail, rem = d._remote || { commits: [] };
  const tags = (d.tags || []).map((t) => `<span class="tag-chip">${esc(t)}</span>`).join("") || `<span class="muted">aucune version</span>`;
  let claimedN = 0;
  const commits = (rem.commits || []).map((c) => {
    const full = stripAttestTrailer(c.message), nl = full.indexOf("\n");
    const subject = nl < 0 ? full : full.slice(0, nl);
    const bodyTxt = nl < 0 ? "" : full.slice(nl + 1).trim();
    const sha = esc(c.sha || "");
    // DÉCLARATION de sceau d'après le drapeau serveur (présence du trailer sur le message complet, en ligne,
    // sans clone). C'est une PRÉTENTION, pas une preuve : un trailer peut être ajouté à la main, révoqué ou
    // périmé. On n'affiche donc PAS la garantie verte ici — on marque l'état « déclaré » (ambre, à vérifier).
    // Seule la vérification cryptographique LOCALE (enrichAttestation) peut promouvoir en garantie verte,
    // ou au contraire rétrograder en « invalide ». Fail-closed par défaut : rien n'est « garanti » sans preuve.
    const claimed = !!c.signed || hasAttestTrailer(c.message);
    if (claimed) claimedN++;
    const badge = claimed
      ? `<span class="att-badge claimed verifying" data-att="${sha}" title="Sceau SituLab déclaré, vérification locale en cours…">${SEAL_SVG}</span>`
      : `<span class="att-badge none" data-att="${sha}" title="Pas de sceau SituLab sur ce commit"></span>`;
    return `<div class="cmt${claimed ? " claimed" : ""}" data-sha="${sha}">
      <div class="cmt-head"><span class="cmt-subject">${esc(subject)}</span>${badge}</div>
      <div class="cmt-meta"><code>${esc(c.short)}</code><span class="cmt-who">${esc(c.author)}</span><span class="cmt-when">${esc(fmtDate(c.date))}</span></div>
      ${bodyTxt ? `<button class="cmt-more" data-more="${sha}">Détails ▾</button><div class="cmt-body hidden" id="cb-${sha}">${esc(bodyTxt)}</div>` : ""}
    </div>`;
  }).join("") || blankslate("∅", "Aucun commit", "");
  const shownN = (rem.commits || []).length;
  const sig = "histo|" + JSON.stringify([d.ref, d.tags || [], rem.commits || [], d.local_path || ""]);
  if (liveOnly && sig === d._paintSig) return;
  d._paintSig = sig;
  const summaryHTML = shownN === 0 ? ""
    : claimedN > 0
      ? `<div class="att-note claimed verifying"><span class="seal-sm">${SEAL_SVG}</span> <b>${claimedN}</b>/${shownN} commit(s) déclarent le sceau SituLab, <span class="att-live"><span class="att-spin"></span><b>vérification locale…</b></span></div>`
      : `<div class="att-note"><span class="seal-ph">${ico("shield", 14)}</span> Aucun de ces commits ne porte le sceau SituLab (le sceau marque un commit créé ou promu via SituLab).</div>`;
  body.innerHTML = scopeNoteHTML(d, "histo") + `<div id="att-banner"></div><div id="att-summary">${summaryHTML}</div>
    <div class="blk"><div class="blk-h">Versions</div><div class="tags">${tags}</div></div>
    <div class="blk"><div class="blk-h">Historique : ${esc(d.ref)}</div><div class="cmt-timeline">${commits}</div></div>`;
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
  // Bannière mode TEST (la racine de gouvernance n'est pas installée → les sceaux ne sont pas
  // vérifiables réellement). Le sceau lui-même est déjà posé par paintHisto d'après la présence du
  // trailer dans le message en ligne (pas besoin de clone local).
  try {
    if (await App().AttestUsingTestRoot()) {
      const b = el("att-banner");
      if (b) b.innerHTML = `<div class="callout warn" style="margin-bottom:10px">Signature en mode <b>TEST</b> : la racine de gouvernance n'est pas encore installée, les sceaux ne sont pas vérifiables réellement.</div>`;
    }
  } catch (_) {}
  if (state.detail !== d) return;
  // Vérification cryptographique LOCALE = SEULE autorité pour la garantie d'origine. Le clone porte le
  // commit COMPLET (arbre, parents, committer, trailer) : le cœur reconstruit le pré-image et vérifie
  // contre la racine épinglée + la liste de révocation. Trois issues par commit déclaré :
  //   • vérifié            → garantie VERTE (autoritatif) ;
  //   • présent mais KO    → sceau INVALIDE/révoqué/forgé → état ROUGE (jamais vert) ;
  //   • absent du clone / pas de clone → reste « déclaré, non vérifié » (ambre), jamais promu en vert.
  // On ne « garantit » donc JAMAIS sur la seule foi du drapeau serveur : sans preuve locale, pas de vert.
  let canVerify = !!d.local_path, loadErr = false;
  const map = {};
  if (canVerify) {
    let rows = null;
    try { rows = await App().AttestHistory(d.local_path, 200); } catch (_) { loadErr = true; }
    if (state.detail !== d) return;
    if (Array.isArray(rows)) rows.forEach((r) => { if (r && r.sha) map[r.sha] = r.attestation; });
    else canVerify = false;
  }
  let verified = 0, declared = 0, suspect = 0, shown = 0;
  document.querySelectorAll(".cmt[data-sha]").forEach((row) => {
    shown++;
    const badge = row.querySelector(".att-badge");
    if (!badge) return;
    const claimed = badge.classList.contains("claimed") || badge.classList.contains("ok") || badge.classList.contains("bad");
    const v = map[row.dataset.sha];
    if (v && v.verified) {
      badge.className = "att-badge ok"; badge.innerHTML = SEAL_SVG;
      badge.title = "Signé via SituLab, garantie d'origine vérifiée localement" + (v.subject ? " · " + v.subject : "");
      row.classList.add("signed"); row.classList.remove("claimed", "bad");
      verified++;
      return;
    }
    if (!claimed) return; // commit sans sceau déclaré : rien à poser
    if (v && !v.verified) {
      // Le commit EST dans le clone mais son trailer NE VÉRIFIE PAS (forgé, révoqué, cert hors fenêtre) :
      // rétrogradation EXPLICITE en état d'alerte — on ne laisse jamais une prétention invalide passer.
      badge.className = "att-badge bad"; badge.innerHTML = SEAL_BAD_SVG;
      badge.title = "Sceau SituLab INVALIDE ou révoqué, origine NON garantie" + (v.reason ? " · " + v.reason : "");
      row.classList.add("bad"); row.classList.remove("signed", "claimed");
      suspect++;
    } else {
      // Commit absent du clone (ou pas de clone / lecture échouée) : simple déclaration, non vérifiée.
      badge.className = "att-badge claimed"; badge.innerHTML = SEAL_SVG;
      badge.title = canVerify
        ? "Sceau SituLab déclaré : commit absent de la copie locale, non vérifié sur ce poste"
        : "Sceau SituLab déclaré : copie locale requise pour vérifier l'origine";
      row.classList.add("claimed"); row.classList.remove("signed", "bad");
      declared++;
    }
  });
  const sum = el("att-summary");
  if (sum && shown > 0) sum.innerHTML = summarizeAttest(verified, declared, suspect, shown, canVerify);
}

// summarizeAttest : résumé HONNÊTE de l'état d'attestation après vérification locale. Priorité au signal
// de sécurité (un seul sceau invalide/révoqué prime), puis au vérifié, puis au déclaré non vérifié.
function summarizeAttest(verified, declared, suspect, shown, canVerify) {
  if (suspect > 0) {
    return `<div class="att-note bad"><span class="seal-sm">${SEAL_BAD_SVG}</span> <b>${suspect}</b> commit(s) portent un sceau <b>invalide ou révoqué</b>, origine NON garantie.${verified > 0 ? ` <b>${verified}</b> vérifié(s).` : ""}</div>`;
  }
  if (verified > 0 && declared === 0) {
    return `<div class="att-note ok"><span class="seal-sm">${SEAL_SVG}</span> <b>${verified}</b>/${shown} commit(s) signé(s) via SituLab, garantie d'origine vérifiée localement.</div>`;
  }
  if (verified > 0) {
    return `<div class="att-note ok"><span class="seal-sm">${SEAL_SVG}</span> <b>${verified}</b> vérifié(s) · <b>${declared}</b> déclaré(s) non vérifié(s), sur ${shown} commit(s).</div>`;
  }
  if (declared > 0) {
    const why = canVerify ? "absents de la copie locale" : "copie locale requise pour vérifier";
    return `<div class="att-note claimed"><span class="seal-sm">${SEAL_SVG}</span> <b>${declared}</b>/${shown} commit(s) déclarent le sceau SituLab, <b>non vérifié</b> (${why}).</div>`;
  }
  return `<div class="att-note"><span class="seal-ph">${ico("shield", 14)}</span> Aucun de ces commits ne porte le sceau SituLab (le sceau marque un commit créé ou promu via SituLab).</div>`;
}

