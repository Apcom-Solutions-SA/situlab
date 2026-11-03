// Mapping CORPUS↔dépôt (P19, v1.18.2) — panneau sous les colonnes git de la fiche projet. Relie un
// projet à des dossiers/fichiers de clouds souverains (mémoire situative : git + fichiers cloud).
// Réutilise les primitives CORPUS de 10-corpus.js (aiGlyph, cloudVisState, ccClean, aiAccessTreeModal,
// cloudPINModal) au RUNTIME — aucune référence top-level à un autre fichier (invariant de chargement).
// « Relier » ne donne AUCUN droit : l'accès de l'IA reste réglé par « Accès IA ». Geste FENÊTRE.

async function renderProjectCorpusPanel(projectName) {
  const box = el("proj-corpus");
  if (!box) return;
  let projects = [], clouds = [];
  try {
    projects = (await App().Projects()) || [];
    clouds = (await App().FileClouds()) || [];
  } catch (e) {
    if (isLocked(e)) return handleLock();
    box.innerHTML = surfaceError("CORPUS indisponible", cleanErr(e), null);
    return;
  }
  const proj = projects.find((p) => (p.name || "").toLowerCase() === (projectName || "").toLowerCase());
  const links = ((proj && proj.corpus) || []).map((l) => ({ cloud: l.cloud, paths: (l.paths || []).slice() }));
  const cloudByName = {};
  clouds.forEach((c) => (cloudByName[(c.name || "").toLowerCase()] = c));

  const save = async (next) => {
    try {
      await App().SetProjectCorpus(projectName, next.filter((l) => l.paths.length));
      renderProjectCorpusPanel(projectName);
    } catch (e) {
      if (isLocked(e)) return handleLock();
      toast(cleanErr(e), "err");
    }
  };

  if (!clouds.length) {
    box.innerHTML = `<div class="pcx-empty">Aucun cloud CORPUS connecté. Connectez un cloud dans <b>CORPUS</b> pour relier des fichiers à ce projet.</div>`;
    return;
  }
  if (!links.length) {
    // Empty-state RICHE (1.18.4, retour #11) : reprend le schéma « dossier → IA » de l'état plein
    // (en version fantôme) plutôt qu'un simple paragraphe. Invite soignée, à la hauteur du reste.
    box.innerHTML = `<div class="pcx-empty-rich">
      <div class="pcx-empty-viz" aria-hidden="true">
        <span class="pcx-node pcx-src">${ico("folder", 16)}<span class="pcx-nname">dossier cloud</span></span>
        <span class="pcx-wire ghost"><i></i></span>
        <span class="pcx-node pcx-ainode">${ico("robot", 16)}<span class="pcx-nname">IA</span></span>
      </div>
      <div class="pcx-empty-ttl">Reliez les fichiers de ce projet</div>
      <p class="pcx-empty-txt">Les dossiers cloud que vous reliez entrent dans la <b>mémoire situative</b> du projet : votre agent IA peut les mobiliser, dans les limites d'accès que vous fixez.</p>
      <button class="btn btn-primary" id="pcx-add">${ico("link", 14)} Relier un cloud / dossier</button>
      <p class="pcx-empty-note">${ico("info", 12)} Relier n'est pas autoriser : l'accès de l'IA se règle via « Accès IA ».</p>
    </div>`;
    el("pcx-add").onclick = () => linkPickerModal(projectName, links, clouds, save);
    return;
  }
  // Vue schématique « mémoire situative » (R12, 1.18.3) : chaque dossier relié nourrit le contexte de
  // l'IA. On montre le FLUX dossier cloud → IA, avec un connecteur animé et l'état d'accès de l'IA.
  const cards = links.map((l) => {
    const fc = cloudByName[(l.cloud || "").toLowerCase()];
    const A = new Set(((fc && fc.allowed_dirs) || []).map(ccClean));
    const D = new Set(((fc && fc.denied_dirs) || []).map(ccClean));
    const head = fc
      ? `<div class="pcx-head"><span class="pcx-cloud">${ico("cloud", 15)} ${esc(l.cloud)}</span><span class="pcx-acts"><button class="lnk" data-ai="${esc(l.cloud)}">Accès IA</button><button class="lnk" data-rights="${esc(l.cloud)}">Droits</button></span></div>`
      : `<div class="pcx-head"><span class="pcx-cloud off">${ico("cloud", 15)} ${esc(l.cloud)} — déconnecté</span></div>`;
    const rows = l.paths.map((p) => {
      const vis = fc ? cloudVisState(p, true, A, D) : "none";
      const visLbl = vis === "on" ? "IA : accès complet" : vis === "partial" ? "IA : accès partiel" : "IA : aucun accès";
      const name = p.split("/").filter(Boolean).pop() || p;
      return `<div class="pcx-flow pcx-vis-${vis}">
        <span class="pcx-node pcx-src" title="${esc(p)}">${ico("folder", 15)}<span class="pcx-nname">${esc(name)}</span></span>
        <span class="pcx-wire" aria-hidden="true"><i></i></span>
        <span class="pcx-node pcx-ainode" title="${esc(visLbl)}">${ico("robot", 15)}<span class="pcx-nname">IA</span></span>
        <button class="btn-icon danger pcx-unlink" data-unlink="${esc(l.cloud)}||${esc(p)}" title="Délier ce dossier" aria-label="Délier">${ICO_TRASH}</button>
      </div>`;
    }).join("");
    return `<div class="pcx-card pcx-magic">${head}${rows}</div>`;
  }).join("");
  box.innerHTML = `${cards}<div class="pcx-addrow"><button class="btn btn-ghost btn-sm" id="pcx-add">${ico("link", 14)} Relier un cloud / dossier</button></div>`;
  el("pcx-add").onclick = () => linkPickerModal(projectName, links, clouds, save);
  box.querySelectorAll("[data-ai]").forEach((b) => b.onclick = () => { const fc = cloudByName[b.dataset.ai.toLowerCase()]; if (fc) aiAccessTreeModal(fc, () => renderProjectCorpusPanel(projectName)); });
  box.querySelectorAll("[data-rights]").forEach((b) => b.onclick = () => { const fc = cloudByName[b.dataset.rights.toLowerCase()]; if (fc) cloudPINModal(fc, () => renderProjectCorpusPanel(projectName)); });
  box.querySelectorAll("[data-unlink]").forEach((b) => b.onclick = () => {
    const [cloud, p] = b.dataset.unlink.split("||");
    const next = links.map((l) => l.cloud === cloud ? { cloud, paths: l.paths.filter((x) => x !== p) } : l);
    save(next);
  });
}

// linkPickerModal : relier un cloud + des dossiers/fichiers. Navigation via App().CloudList (réemploi
// de la primitive CORPUS, pas de duplication). Les chemins cochés sont fusionnés dans le mapping.
function linkPickerModal(projectName, links, clouds, save) {
  let cloud = (links[0] && links[0].cloud) || (clouds[0] && clouds[0].name) || "";
  const picked = {}; // cloud → Set de chemins ajoutés dans cette session
  links.forEach((l) => (picked[l.cloud] = new Set(l.paths)));
  // Navigation + multi-sélection via l'EXPLORATEUR unique (1.18.3) — source cloud. Les chemins cochés
  // (ccClean) sont fusionnés dans le mapping. CORPUS : aucun cache disque (souveraineté).
  const cloudTabs = () => clouds.map((c) => `<button type="button" class="seg-tab${c.name === cloud ? " on" : ""}" data-cloud="${esc(c.name)}">${ico("cloud", 14)} ${esc(c.name)}</button>`).join("");
  openModal("Relier des fichiers CORPUS", `<div class="lp">
    <div class="seg lp-clouds" id="lp-clouds">${cloudTabs()}</div>
    <div id="lp-xp"></div>
    <div class="lp-foot"><span class="lp-count" id="lp-count"></span><span class="sub">Cochez les éléments à relier. Relier ≠ autoriser l'IA (réglé via « Accès IA »).</span></div>
  </div>`, async () => {
    const next = clouds.map((c) => ({ cloud: c.name, paths: [...(picked[c.name] || new Set())] })).filter((l) => l.paths.length);
    closeModal();
    await save(next);
  });
  el("modal-ok").textContent = "Enregistrer";
  const totalPicked = () => Object.values(picked).reduce((n, s) => n + (s ? s.size : 0), 0);
  const refreshCount = () => { const c = el("lp-count"); if (!c) return; const n = totalPicked(); c.textContent = n ? `${n} élément${n > 1 ? "s" : ""} relié${n > 1 ? "s" : ""}` : "Aucun élément relié"; c.classList.toggle("none", !n); };
  const mapEnts = (arr) => (arr || []).map((e) => ({ name: e.name, path: ccClean(e.path), type: e.is_dir ? "dir" : "file", size: e.size }))
    .sort((a, b) => (a.type === b.type ? a.name.localeCompare(b.name) : a.type === "dir" ? -1 : 1));
  const mount = () => mountExplorer(el("lp-xp"), {
    mode: "multi",
    load: (p) => App().CloudList(cloud, "/" + p).then(mapEnts),
    selectedSet: () => (picked[cloud] || (picked[cloud] = new Set())),
    onToggle: (e, on) => { const s = picked[cloud] || (picked[cloud] = new Set()); if (on) s.add(e.path); else s.delete(e.path); refreshCount(); },
    emptyText: "Dossier vide",
  });
  el("lp-clouds").querySelectorAll("[data-cloud]").forEach((b) => b.onclick = () => {
    cloud = b.dataset.cloud;
    el("lp-clouds").querySelectorAll("[data-cloud]").forEach((x) => x.classList.toggle("on", x === b));
    mount(); refreshCount();
  });
  mount();
  refreshCount();
}
