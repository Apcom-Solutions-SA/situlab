/* ---------------- ORGANISER MES DÉPÔTS : la matrice alignée (1.11) ----------------
 * Le service que SituLab rend vraiment : relier et assembler les dépôts — clone LOCAL ↔ dépôt
 * ATELIER ↔ dépôt VITRINE — au lieu d'une vue de gestionnaire. Présentation plug-and-play pour un
 * public non-dev : une MATRICE alignée (une ligne par dépôt, trois colonnes qui se font face),
 * pictos dossier colorés, nom + organisation en petit, zéro URL. Case vide = bouton « créer ».
 * Appariement proposé = bouton discret « Créer le projet ». Rien n'est imposé.
 */

// Picto dossier (monoline), colorisé par la classe de colonne (local/atelier/vitrine).
function orgFolder(kind) {
  return `<span class="org-folder org-${kind}"><svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z"/></svg></span>`;
}

// Connexions par rôle (pour les actions « créer l'atelier / la vitrine »).
function connsByRole(role) {
  return (state.instances || []).filter((i) => (role === "production" ? i.role === "production" : i.role !== "production"));
}

async function renderOrganize() {
  const view = el("view");
  view.innerHTML = `<div class="view-head view-head-row">
      <div><button class="btn btn-ghost btn-sm" id="org-back">‹ Dépôts git</button>
      <div class="view-title view-title-stacked">Organiser mes dépôts</div>
      <div class="view-sub">Chaque ligne est un dépôt. SituLab aligne votre copie <b>locale</b>, son <b>atelier</b> et sa <b>vitrine</b> ; complétez une case vide d'un clic.</div></div>
      <div class="view-head-actions" id="org-actions">
        <button class="btn btn-ghost btn-sm hidden" id="org-ai-match" type="button" title="Demander à votre IA de repérer les clones déjà présents sur ce poste et de les relier">${ico("robot", 14)} Appairer via l'IA</button>
        <button class="btn btn-primary btn-sm" id="org-create" type="button">${ico("plus", 14)} Créer un dépôt</button>
        <button class="btn btn-ghost btn-sm" id="org-refresh" type="button" title="Actualiser">↻</button>
      </div></div>
    <div id="org-body"><div class="skeleton"></div><div class="skeleton" style="margin-top:10px"></div></div>`;
  el("org-back").onclick = () => navTo("projects");
  el("org-refresh").onclick = () => renderOrganize();
  el("org-create").onclick = () => createRepoTunnel();
  // Prompt d'APPARIAGE des clones locaux déjà présents (ex-bouton de la modale de clone, remonté au
  // 1er niveau en 1.18.1). Reste masqué tant qu'il n'y a rien à apparier (révélé par loadMatrix).
  el("org-ai-match").onclick = () => copyLinkLocalPrompt();
  loadMatrix();
}

async function loadMatrix() {
  const box = el("org-body");
  if (!box) return;
  let m;
  try {
    m = await App().RepoMatrix();
  } catch (e) {
    if (isLocked(e)) return handleLock();
    box.innerHTML = blankslate("⚠", "Matrice indisponible", cleanErr(e));
    return;
  }
  state.matrix = m;
  const rows = m.rows || [];
  const unreach = m.unreachable || [];
  const pending = m.pending || 0;
  if (typeof setPendingPairings === "function") setPendingPairings(pending); // alimente la pastille header
  // Révèle le bouton « correspondances locales » SEULEMENT s'il reste des appariements à faire (1.18.1).
  if (el("org-ai-match")) el("org-ai-match").classList.toggle("hidden", !pending);

  const bannerHTML = pending
    ? `<div class="org-banner"><span class="org-banner-n">${pending}</span> dépôt${pending > 1 ? "s" : ""} à traiter : <b>appairez</b> ceux qui sont alignés, <b>complétez</b> les autres (créer le clone, l'atelier ou la vitrine) ou <b>archivez</b>-les.</div>`
    : `<div class="org-banner ok">Tout est rangé : chaque dépôt est appairé dans un projet.</div>`;
  const unreachHTML = unreach.length
    ? `<div class="callout warn" style="margin:10px 0">Connexion(s) non lues (hors ligne ou à reconnecter) : <b>${unreach.map(esc).join(", ")}</b>.</div>`
    : "";

  // En-tête de colonnes + lignes alignées.
  const head = `<div class="mx-head">
    <div class="mx-h mx-h-local">${ico("desktop", 15)} Sur mon poste</div>
    <div class="mx-h mx-h-atl">🛠️ Atelier</div>
    <div class="mx-h mx-h-vit">${ico("globe", 15)} Vitrine</div>
    <div class="mx-h mx-h-act"></div></div>`;

  // Marque de forge (GitHub / Forgejo) pour les cases distantes : on identifie d'un coup d'œil OÙ vit le
  // dépôt (atelier/vitrine), avec le logo + l'instance en infobulle. Local = pas de forge (c'est le poste).
  const forgeMark = (kind, c) => (kind !== "local" && c && c.instance && typeof providerMark === "function")
    ? `<span class="mx-forge" title="${esc(c.instance)}">${providerMark(c.instance)}</span>` : "";
  const cell = (kind, c, row, idx) => {
    if (c && c.present) {
      const sub = kind === "local" ? "sur ce poste" : (c.org || c.instance || "");
      return `<button type="button" class="mx-cell on" data-open="${kind}:${idx}" title="Ouvrir le détail">
        ${orgFolder(kind)}<span class="mx-cell-txt"><span class="mx-name">${esc(c.name || "")}</span><span class="mx-org">${forgeMark(kind, c)}${esc(sub)}</span></span></button>`;
    }
    // Case vide → action « créer », selon la colonne.
    const label = kind === "local" ? "Créer le clone" : kind === "atelier" ? "Créer l'atelier" : "Créer la vitrine";
    return `<button type="button" class="mx-cell empty" data-create="${kind}:${idx}"><span class="mx-plus">＋</span><span class="mx-create-t">${label}</span></button>`;
  };

  const vitCell = (row, idx) => {
    const vs = row.vitrines || [];
    if (!vs.length) {
      // pas de vitrine : proposer la création seulement si un atelier existe (sinon rien à publier).
      if (!row.atelier || !row.atelier.present) return `<div class="mx-cell void"></div>`;
      return `<button type="button" class="mx-cell empty" data-create="vitrine:${idx}"><span class="mx-plus">＋</span><span class="mx-create-t">Créer la vitrine</span></button>`;
    }
    // Vitrine CONFIRMÉE (dans un projet) → clic = ouvrir le détail. Vitrine SUGGÉRÉE (match par nom ou
    // VitrineHint, pas encore dans un projet) → trait pointillé + badge « confirmer », et clic = la
    // CONFIRMER dans le projet (ConfirmVitrine, crée le projet au besoin) — jamais une impasse (#16, 1.19).
    return vs.map((v, vi) => {
      const sugg = v.suggested;
      const dataAttr = sugg ? `data-confirmvit="${idx}:${vi}"` : `data-open="vitrine:${idx}:${vi}"`;
      const title = sugg ? "Vitrine suggérée — cliquez pour la confirmer dans le projet" : "Ouvrir le détail";
      const style = sugg ? ` style="border:1px dashed var(--accent,#6b8afd)"` : "";
      const badge = sugg ? ` <span style="font-size:10px;padding:1px 6px;border-radius:6px;background:var(--accent,#6b8afd);color:#fff;white-space:nowrap">confirmer</span>` : "";
      return `<button type="button" class="mx-cell on${sugg ? " mx-suggested" : ""}" ${dataAttr}${style} title="${title}">
      ${orgFolder("vitrine")}<span class="mx-cell-txt"><span class="mx-name">${esc(v.name || "")}${badge}</span><span class="mx-org">${forgeMark("vitrine", v)}${esc(v.org || v.instance || "")}</span></span></button>`;
    }).join("");
  };

  const rowHTML = rows.map((row, i) => {
    // Action de ligne : appairer (si en attente) ou badge projet ; + archiver (ranger) si pas un projet.
    let act = "";
    // Compté « à traiter » = row.pending (inclut les solitaires). Mais le bouton « Appairer » ne
    // s'affiche que si c'est VRAIMENT alignable (atelier + contrepartie) : row.alignable. Un dépôt seul
    // reste compté, sans bouton — il se complète via « Créer le clone / l'atelier / la vitrine ».
    if (row.project) act = `<span class="mx-proj" title="Déjà appairé dans « ${esc(row.project)} »">✓ ${esc(row.project)}</span>`;
    else if (row.alignable) act = `<button type="button" class="mx-make" data-make="${i}" title="Appairer ces dépôts (local ↔ atelier ↔ vitrine)">⇄ Appairer</button>`;
    const arch = row.project ? "" : `<button type="button" class="mx-arch" data-arch="${i}" title="Ranger ou supprimer cette suggestion"><span class="mx-arch-ico">🗄</span> Archiver</button>`;
    // Case de sélection pour les actions EN MASSE (seulement les lignes rangeable, pas les projets).
    const selbox = row.project ? "" : `<input type="checkbox" class="mx-sel" data-sel="${i}" aria-label="Sélectionner pour action en masse" />`;
    // Facettes de filtrage (1.12) : état d'appairage + présence locale + texte cherchable.
    const fstate = row.project ? "paired" : (row.pending ? "pending" : "none");
    const hay = [row.name, row.local && row.local.name, row.atelier && row.atelier.name, row.atelier && row.atelier.org,
      ...(row.vitrines || []).map((v) => v.name), ...(row.vitrines || []).map((v) => v.org)].filter(Boolean).join(" ").toLowerCase();
    return `<div class="mx-row${row.pending ? " pending" : ""}" data-fstate="${fstate}" data-flocal="${row.local && row.local.present ? "1" : "0"}" data-hay="${esc(hay)}">
      <div class="mx-col">${cell("local", row.local, row, i)}</div>
      <div class="mx-col">${cell("atelier", row.atelier, row, i)}</div>
      <div class="mx-col">${vitCell(row, i)}</div>
      <div class="mx-col mx-col-act">${selbox}${act}${arch}</div>
    </div>`;
  }).join("");

  // Barre de filtres (1.12) : recherche + facettes combinables. Pensée usage : l'utilisateur cherche
  // « ce qui reste à faire » (à appairer), « ce qui est sur mon poste », ou un dépôt par son nom.
  const n = { all: rows.length, pending: rows.filter((r) => r.pending).length, paired: rows.filter((r) => r.project).length, local: rows.filter((r) => r.local && r.local.present).length };
  const filterHTML = rows.length ? `<div class="org-filters">
    <div class="org-search"><span class="org-search-ico">⌕</span><input id="org-q" class="inp" type="search" placeholder="Rechercher un dépôt…" autocomplete="off" spellcheck="false" /></div>
    <div class="org-chips" id="org-chips">
      <button type="button" class="org-chip on" data-f="all">Tous <span class="org-chip-n">${n.all}</span></button>
      <button type="button" class="org-chip" data-f="pending" title="Dépôts qu'il reste à appairer">⚠ À appairer <span class="org-chip-n">${n.pending}</span></button>
      <button type="button" class="org-chip" data-f="local" title="Dépôts présents sur ce poste">${ico("desktop", 14)} Sur le poste <span class="org-chip-n">${n.local}</span></button>
      <button type="button" class="org-chip" data-f="paired" title="Dépôts déjà appairés">✓ Appairés <span class="org-chip-n">${n.paired}</span></button>
    </div>
  </div>` : "";

  const body = rows.length
    ? `<div class="mx-grid">${head}${rowHTML}</div>`
    : blankslate(ico("folder", 30), "Aucun dépôt", "Reliez une connexion git et clonez un dépôt pour commencer.");

  // Section ARCHIVE : suggestions rangées, repliable, avec restauration.
  const archived = m.archived || [];
  let archHTML = "";
  if (archived.length) {
    const arow = (row, j) => `<div class="mx-arch-row">
      <span class="mx-arch-n">${esc(row.name)}</span>
      <span class="mx-arch-d muted">${[row.local && row.local.present ? "local" : "", row.atelier && row.atelier.present ? "atelier" : "", (row.vitrines || []).length ? "vitrine" : ""].filter(Boolean).join(" · ")}</span>
      <button type="button" class="btn btn-ghost btn-sm" data-unarch="${j}">⇄ Repasser en appairage</button></div>`;
    archHTML = `<details class="org-arch"><summary>🗄 Archive <span class="muted">(${archived.length})</span></summary>
      <div class="mx-arch-list">${archived.map(arow).join("")}</div></details>`;
  }

  const bulkHTML = rows.length ? `<div class="org-bulk hidden" id="org-bulk">
    <span class="org-bulk-txt"><b id="org-bulk-n">0</b> sélectionné(s)</span>
    <button type="button" class="btn btn-ghost btn-sm" id="org-selall">Tout sélectionner (visibles)</button>
    <button type="button" class="btn btn-ghost btn-sm" id="org-deselect">Désélectionner</button>
    <span class="org-bulk-sep"></span>
    <button type="button" class="btn btn-primary btn-sm" id="org-bulk-pair">⇄ Appairer la sélection</button>
    <button type="button" class="btn btn-ghost btn-sm" id="org-bulk-arch">🗄 Archiver la sélection</button>
    <button type="button" class="btn btn-ghost btn-sm btn-danger" id="org-bulk-del">🗑 Supprimer la sélection</button>
  </div>` : "";

  box.innerHTML = bannerHTML + unreachHTML + filterHTML + bulkHTML + body + `<div class="org-noresult hidden" id="org-noresult">Aucun dépôt ne correspond à ce filtre.</div>` + archHTML;

  // --- Sélection multiple + actions EN MASSE (archiver / supprimer) ---
  const selectedRows = () => Array.from(box.querySelectorAll(".mx-sel:checked")).map((c) => rows[+c.dataset.sel]).filter(Boolean);
  const updateBulk = () => {
    const n = box.querySelectorAll(".mx-sel:checked").length;
    const bar = el("org-bulk"); if (bar) bar.classList.toggle("hidden", n === 0);
    const nb = el("org-bulk-n"); if (nb) nb.textContent = n;
  };
  box.querySelectorAll(".mx-sel").forEach((c) => c.onchange = updateBulk);
  if (el("org-selall")) el("org-selall").onclick = () => {
    box.querySelectorAll(".mx-row:not(.hidden) .mx-sel").forEach((c) => c.checked = true); updateBulk();
  };
  if (el("org-deselect")) el("org-deselect").onclick = () => {
    box.querySelectorAll(".mx-sel").forEach((c) => c.checked = false); updateBulk();
  };
  if (el("org-bulk-pair")) el("org-bulk-pair").onclick = async () => {
    // Appairer en masse (#14, 1.19) : n'agit que sur les lignes ALIGNABLES cochées (atelier + une
    // contrepartie locale/vitrine). Geste local, sans PIN (AddProject). Les non-alignables sont ignorées.
    const sel = selectedRows().filter((r) => r.alignable && r.atelier && r.atelier.present);
    if (!sel.length) { toast("Aucune ligne alignable dans la sélection", "err"); return; }
    el("org-bulk-pair").disabled = true;
    let ok = 0; const fails = [];
    for (const row of sel) {
      const atelier = { instance: row.atelier.instance, owner: row.atelier.org, repo: row.atelier.name };
      const productions = (row.vitrines || []).map((v) => ({ instance: v.instance, owner: v.org, repo: v.name }));
      try { await App().AddProject(row.name, "", atelier, productions); ok++; }
      catch (e) { if (isLocked(e)) return handleLock(); fails.push(row.name + " : " + cleanErr(e)); }
    }
    toast(ok + " dépôt(s) appairé(s)" + (fails.length ? ", " + fails.length + " en échec" : ""), fails.length ? "err" : "ok");
    renderOrganize();
  };
  if (el("org-bulk-arch")) el("org-bulk-arch").onclick = async () => {
    const sel = selectedRows(); if (!sel.length) return;
    el("org-bulk-arch").disabled = true;
    let ok = 0;
    for (const row of sel) { try { await App().ArchivePairing(row.key); ok++; } catch (e) { if (isLocked(e)) return handleLock(); } }
    toast(ok + " suggestion(s) archivée(s)", "ok"); renderOrganize();
  };
  if (el("org-bulk-del")) el("org-bulk-del").onclick = () => {
    const sel = selectedRows(); if (!sel.length) return;
    openModal(`Supprimer ${sel.length} dépôt(s) ?`, `<div class="field">
      <div class="callout danger"><b>Action irréversible.</b> Les dépôts <b>distants</b> (atelier/vitrine) de la sélection seront supprimés du serveur. Les <b>dossiers locaux sont conservés</b> (retirés du suivi seulement).</div>
      <ul class="arch-del-list">${sel.slice(0, 20).map((r) => `<li>${esc(r.name)}</li>`).join("")}${sel.length > 20 ? `<li class="muted">… et ${sel.length - 20} autre(s)</li>` : ""}</ul></div>`, () => {
      pinConfirm(`Supprimer définitivement ${sel.length} dépôt(s) distants`, async (pin) => {
        let ok = 0; const fails = [];
        for (const row of sel) {
          const hasAtl = !!(row.atelier && row.atelier.present);
          const atelier = hasAtl ? { instance: row.atelier.instance, owner: row.atelier.org, repo: row.atelier.name } : { instance: "", owner: "", repo: "" };
          const productions = (row.vitrines || []).map((v) => ({ instance: v.instance, owner: v.org, repo: v.name }));
          const localPath = (row.local && row.local.present) ? row.local.local_path : "";
          try { await App().PairingDelete(localPath, false, atelier, productions, pin); ok++; }
          catch (e) {
            // PIN faux ou session verrouillée : inutile de poursuivre (tout le lot échouerait pareil). On
            // relance l'erreur pour que pinConfirm l'affiche et garde la modale ouverte (ressaisie).
            if (isLocked(e) || /pin/i.test(cleanErr(e))) throw e;
            fails.push(row.name + " : " + cleanErr(e)); // échec propre à ce dépôt : on continue le lot
          }
        }
        // pinConfirm referme la modale PIN de façon centrale après ce callback. On ne tait JAMAIS les
        // échecs : un lot partiel est signalé, avec le détail de ce qui reste (modale différée pour ne pas
        // être refermée par le closeModal central de pinConfirm).
        renderOrganize();
        if (fails.length) {
          toast(`${ok} supprimé(s), ${fails.length} échec(s)`, "err");
          setTimeout(() => infoModal("Suppressions partielles", `<div class="field"><div class="callout warn"><b>${ok}</b> dépôt(s) supprimé(s), <b>${fails.length} échec(s)</b>, à reprendre :</div><ul class="arch-del-list">${fails.slice(0, 30).map((f) => `<li>${esc(f)}</li>`).join("")}${fails.length > 30 ? `<li class="muted">… et ${fails.length - 30} autre(s)</li>` : ""}</ul></div>`), 0);
        } else {
          toast(ok + " dépôt(s) supprimé(s)", "ok");
        }
      });
    });
    el("modal-ok").textContent = "Supprimer définitivement"; el("modal-ok").classList.add("danger-btn"); el("modal-cancel").textContent = "Annuler";
  };

  // --- Filtrage (recherche + facette) en direct, sans réseau ni re-render ---
  const applyFilter = () => {
    const q = ((el("org-q") && el("org-q").value) || "").trim().toLowerCase();
    const facet = (state.orgFacet || "all");
    let shown = 0;
    box.querySelectorAll(".mx-row").forEach((r) => {
      const okFacet = facet === "all"
        || (facet === "pending" && r.dataset.fstate === "pending")
        || (facet === "paired" && r.dataset.fstate === "paired")
        || (facet === "local" && r.dataset.flocal === "1");
      const okText = !q || (r.dataset.hay || "").includes(q);
      const on = okFacet && okText;
      r.classList.toggle("hidden", !on);
      if (on) shown++;
    });
    const nr = el("org-noresult"); if (nr) nr.classList.toggle("hidden", shown > 0 || !rows.length);
  };
  if (el("org-q")) el("org-q").oninput = applyFilter;
  box.querySelectorAll(".org-chip").forEach((c) => c.onclick = () => {
    state.orgFacet = c.dataset.f;
    box.querySelectorAll(".org-chip").forEach((x) => x.classList.toggle("on", x === c));
    applyFilter();
  });
  // Restaure la facette courante (persistée pendant la session).
  if (state.orgFacet && state.orgFacet !== "all") {
    const cur = box.querySelector(`.org-chip[data-f="${state.orgFacet}"]`);
    if (cur) { box.querySelectorAll(".org-chip").forEach((x) => x.classList.toggle("on", x === cur)); applyFilter(); }
  }

  // --- Ouverture du détail (le detail distant a besoin de la bonne connexion) ---
  const openCell = (spec) => {
    const parts = spec.split(":");
    const kind = parts[0], i = +parts[1];
    const row = rows[i]; if (!row) return;
    if (kind === "local") {
      openDetail({ has_remote: false, name: row.local.name, local_path: row.local.local_path });
    } else if (kind === "atelier") {
      state.instance = row.atelier.instance;
      openDetail({ has_remote: true, full_name: row.atelier.full_name, local_path: row.atelier.local_path || "" });
    } else { // vitrine:i:vi
      const v = (row.vitrines || [])[+parts[2]]; if (!v) return;
      state.instance = v.instance;
      openDetail({ has_remote: true, full_name: v.full_name, local_path: v.local_path || "" });
    }
  };
  box.querySelectorAll("[data-open]").forEach((b) => b.onclick = () => openCell(b.dataset.open));

  // --- Actions « créer » dans les cases vides ---
  box.querySelectorAll("[data-create]").forEach((b) => b.onclick = () => {
    const [kind, i] = b.dataset.create.split(":");
    const row = rows[+i]; if (!row) return;
    if (kind === "local") return createLocalCloneFlow(row);
    if (kind === "atelier") return createRemoteFlow(row, "atelier");
    if (kind === "vitrine") return createRemoteFlow(row, "production");
  });

  // --- Créer le projet depuis un appariement en attente ---
  box.querySelectorAll("[data-make]").forEach((b) => b.onclick = async () => {
    const row = rows[+b.dataset.make]; if (!row || !row.atelier || !row.atelier.present) return;
    b.disabled = true;
    const atelier = { instance: row.atelier.instance, owner: row.atelier.org, repo: row.atelier.name };
    const productions = (row.vitrines || []).map((v) => ({ instance: v.instance, owner: v.org, repo: v.name }));
    try {
      await App().AddProject(row.name, "", atelier, productions);
      toast("Dépôts « " + row.name + " » appairés", "ok");
      renderOrganize();
    } catch (e) {
      b.disabled = false;
      if (isLocked(e)) return handleLock();
      toast(cleanErr(e), "err");
    }
  });

  // --- Confirmer une vitrine SUGGÉRÉE dans le projet (#16) : l'ajoute au projet (ou le crée) ---
  box.querySelectorAll("[data-confirmvit]").forEach((b) => b.onclick = async (ev) => {
    ev.stopPropagation();
    const parts = (b.dataset.confirmvit || "").split(":"); const ri = +parts[0], vi = +parts[1];
    const row = rows[ri]; if (!row || !row.atelier || !row.atelier.present) return;
    const v = (row.vitrines || [])[vi]; if (!v) return;
    b.disabled = true;
    const atelier = { instance: row.atelier.instance, owner: row.atelier.org, repo: row.atelier.name };
    const vitrine = { instance: v.instance, owner: v.org, repo: v.name };
    try { await App().ConfirmVitrine(atelier, vitrine); toast("Vitrine « " + (v.name || "") + " » confirmée", "ok"); renderOrganize(); }
    catch (e) { b.disabled = false; if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); }
  });

  // --- Archiver (ranger / supprimer) une suggestion ---
  box.querySelectorAll("[data-arch]").forEach((b) => b.onclick = () => {
    const row = rows[+b.dataset.arch]; if (row) archivePairingModal(row);
  });
  // --- Restaurer depuis l'archive ---
  box.querySelectorAll("[data-unarch]").forEach((b) => b.onclick = async () => {
    const row = (m.archived || [])[+b.dataset.unarch]; if (!row) return;
    try { await App().UnarchivePairing(row.key); toast("Suggestion repassée en appairage", "ok"); renderOrganize(); }
    catch (e) { if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); }
  });
}

// archivePairingModal : deux voies claires pour une suggestion non désirée —
//  • MASQUER (archiver) : réversible, ne touche à rien, range dans l'Archive ;
//  • SUPPRIMER les dépôts : destructif, liste ce qui existe (local/atelier/vitrine), PIN requis.
function archivePairingModal(row) {
  const hasLocal = !!(row.local && row.local.present);
  const hasAtl = !!(row.atelier && row.atelier.present);
  const vits = (row.vitrines || []);
  const existing = [
    hasLocal ? `<li><span class="org-folder org-local">${ico("folder", 14)}</span> <b>clone local</b> : ${esc(row.local.name)} <span class="muted">(retiré du suivi ; fichiers conservés)</span></li>` : "",
    hasAtl ? `<li><span class="org-folder org-atelier">${ico("folder", 14)}</span> <b>atelier</b> : ${esc(row.atelier.org ? row.atelier.org + "/" : "")}${esc(row.atelier.name)} <span class="muted">(supprimé définitivement du serveur)</span></li>` : "",
    ...vits.map((v) => `<li><span class="org-folder org-vitrine">${ico("folder", 14)}</span> <b>vitrine</b> : ${esc(v.org ? v.org + "/" : "")}${esc(v.name)} <span class="muted">(supprimée définitivement du serveur)</span></li>`),
  ].filter(Boolean).join("");

  openModal(`Ranger « ${esc(row.name)} »`, `<div class="field">
    <p class="sub">Cette suggestion ne vous est pas utile ? Choisissez :</p>
    <div class="arch-choices">
      <button type="button" class="arch-choice" id="arch-hide">
        <div class="arch-choice-h">🗄 Masquer la suggestion</div>
        <div class="arch-choice-s">La range dans l'<b>Archive</b> et la retire des « en attente ». <b>Rien n'est supprimé</b>, réversible à tout moment.</div>
      </button>
      <button type="button" class="arch-choice danger" id="arch-del">
        <div class="arch-choice-h">${ico("trash", 15)} Supprimer les dépôts</div>
        <div class="arch-choice-s">Supprime les dépôts existants de cet appariement. <b>Action définitive.</b></div>
      </button>
    </div>
    <div class="inline-error hidden" id="arch-e"></div></div>`, null);
  el("modal-ok").classList.add("hidden"); // le choix se fait sur les deux cartes

  el("arch-hide").onclick = async () => {
    try { await App().ArchivePairing(row.key); closeModal(); toast("Suggestion archivée", "ok"); renderOrganize(); }
    catch (e) { if (isLocked(e)) return handleLock(); const er = el("arch-e"); er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  };
  el("arch-del").onclick = () => {
    if (!hasLocal && !hasAtl && !vits.length) { closeModal(); return; }
    // 2e écran : récapitulatif + (si clone local) choix du sort des fichiers locaux, puis PIN.
    const localChoice = hasLocal ? `<div class="pd-local">
      <div class="pd-local-h">Et le dossier local <b>${esc(row.local.name)}</b> ?</div>
      <label class="pd-opt"><input type="radio" name="pd-wipe" value="0" checked /> <span>Garder les fichiers, <b>retirer du suivi</b> seulement (sûr, réversible)</span></label>
      <label class="pd-opt pd-opt-danger"><input type="radio" name="pd-wipe" value="1" /> <span>🗑 <b>Supprimer pour de vrai</b> le dossier du disque, <b>IRRÉVERSIBLE</b></span></label>
      <div id="pd-guard" class="hidden"><label>Double garde-fou : tapez <b>SUPPRIMER</b> pour confirmer<input id="pd-confirm" class="inp" placeholder="SUPPRIMER" autocomplete="off" /></label></div>
    </div>` : "";
    openModal(`Supprimer « ${esc(row.name)} »`, `<div class="field">
      <div class="callout danger"><b>Action irréversible.</b> Les éléments suivants seront supprimés :</div>
      <ul class="arch-del-list">${existing}</ul>
      <p class="sub">Les dépôts <b>distants</b> sont supprimés définitivement du serveur.</p>
      ${localChoice}
      <div class="inline-error hidden" id="pd-e"></div></div>`, () => {
      const wipe = hasLocal && document.querySelector('input[name="pd-wipe"]:checked') && document.querySelector('input[name="pd-wipe"]:checked').value === "1";
      const er = el("pd-e");
      if (wipe && (!el("pd-confirm") || el("pd-confirm").value.trim().toUpperCase() !== "SUPPRIMER")) {
        if (er) { er.textContent = "Tapez SUPPRIMER pour confirmer l'effacement du dossier local."; er.classList.remove("hidden"); }
        return;
      }
      const atelier = hasAtl ? { instance: row.atelier.instance, owner: row.atelier.org, repo: row.atelier.name } : { instance: "", owner: "", repo: "" };
      const productions = vits.map((v) => ({ instance: v.instance, owner: v.org, repo: v.name }));
      const localPath = hasLocal ? row.local.local_path : "";
      pinConfirm(`Supprimer définitivement les dépôts de « ${row.name} »${wipe ? " (dossier local inclus)" : ""}`, async (pin) => {
        await App().PairingDelete(localPath, !!wipe, atelier, productions, pin);
        toast(wipe ? "Dépôts et dossier local supprimés" : "Dépôts supprimés", "ok");
        renderOrganize();
      });
    });
    el("modal-ok").textContent = "Supprimer définitivement";
    el("modal-ok").classList.add("danger-btn");
    el("modal-cancel").textContent = "Retour";
    // Afficher le garde-fou de saisie seulement quand « supprimer pour de vrai » est choisi.
    document.querySelectorAll('input[name="pd-wipe"]').forEach((r) => r.onchange = () => {
      const on = document.querySelector('input[name="pd-wipe"]:checked').value === "1";
      el("pd-guard").classList.toggle("hidden", !on);
    });
  };
}

// createLocalCloneFlow : « Créer le clone » d'une case LOCAL vide — on clone l'atelier (sinon une
// vitrine) sur ce poste. Réutilise le flux éprouvé getRepoHereModal.
function createLocalCloneFlow(row) {
  const src = (row.atelier && row.atelier.present) ? row.atelier : ((row.vitrines || [])[0] || null);
  if (!src) { toast("Aucun dépôt distant à cloner pour cette ligne", "err"); return; }
  state.instance = src.instance;
  getRepoHereModal({ full_name: src.full_name });
}

// createRemoteFlow : « Créer l'atelier / la vitrine » d'une case distante vide — ouvre la création de
// dépôt distant, ciblée sur une connexion du bon rôle, nom pré-rempli. (Crée le dépôt distant ; y
// publier le code local reste une étape manuelle pour l'instant.)
function createRemoteFlow(row, role) {
  const conns = connsByRole(role);
  if (!conns.length) {
    toast(role === "production" ? "Ajoutez d'abord une connexion de type Vitrine (Réglages ▸ Connexions git)" : "Ajoutez d'abord une connexion de type Atelier", "err");
    return;
  }
  // Vitrine (1.18.1) : plus un simple formulaire — un CHOIX (créer / sélectionner un existant / IA).
  if (role === "production") return createVitrineChooser(row, conns);
  const inst = (conns.find((c) => c.default) || conns[0]).name;
  createRepoModal({ instance: inst, name: row.name });
}

// createVitrineChooser : relier une VITRINE à l'atelier de la ligne. Trois cards autonomes (1.18.1) —
//  ① créer un nouveau dépôt vitrine ; ② sélectionner une vitrine EXISTANTE non encore appairée et
//  l'affilier ; ③ déléguer à l'IA. Aucune action de fond (« Annuler » seul).
function createVitrineChooser(row, conns) {
  const inst = (conns.find((c) => c.default) || conns[0]).name;
  const atelierRef = { instance: row.atelier.instance, owner: row.atelier.org, repo: row.atelier.name };
  openModal("Vitrine de « " + row.name + " »", `<div class="field gr-choices">
    <p class="sub">Comment relier une <b>vitrine</b> (production) à cet atelier ?</p>
    <div class="gr-card gr-card-primary">
      <div class="gr-card-h"><span class="gr-card-ico">${ico("globe", 16)}</span><b>Créer le dépôt vitrine</b></div>
      <div class="gr-card-d">Un nouveau dépôt de production, pré-nommé « ${esc(row.name)} ».</div>
      <button type="button" class="btn btn-primary gr-cta" id="crv-new">Créer le dépôt</button>
    </div>
    <div class="gr-card">
      <div class="gr-card-h"><span class="gr-card-ico">${ico("search", 16)}</span><b>Sélectionner un dépôt existant</b></div>
      <div class="gr-card-d">Choisir une vitrine déjà en ligne, <b>pas encore appairée</b>, et l'affilier à cet atelier.</div>
      <button type="button" class="btn btn-ghost gr-cta" id="crv-pick">Choisir une vitrine existante…</button>
    </div>
    <div class="gr-card gr-card-ai">
      <div class="gr-card-h"><span class="gr-card-ico">${ico("robot", 16)}</span><b>Déléguer à mon IA</b></div>
      <div class="gr-card-d">Copie un prompt prêt à coller pour créer la vitrine via votre IA.</div>
      <button type="button" class="btn btn-ghost gr-cta" id="crv-ai">Copier le prompt pour mon IA</button>
    </div></div>`, null);
  if (el("modal-ok")) el("modal-ok").classList.add("hidden");
  if (el("modal-cancel")) el("modal-cancel").textContent = "Annuler";
  el("crv-new").onclick = () => { closeModal(); createRepoModal({ instance: inst, name: row.name }); };
  el("crv-ai").onclick = () => { state.instance = inst; copyPrompt(promptRepoCreate("", row.name)); };
  el("crv-pick").onclick = () => vitrineExistingPicker(row, inst, atelierRef);
}

// vitrineExistingPicker : liste les dépôts de la connexion vitrine qui ne sont PAS déjà appairés (pas
// présents en colonne vitrine de la matrice), pour en affilier un à l'atelier de la ligne.
async function vitrineExistingPicker(row, inst, atelierRef) {
  // On n'exclut QUE les vitrines déjà affiliées à un PROJET (r.project). Une vitrine orpheline (sa
  // propre ligne, sans projet) est LIBRE → elle doit rester sélectionnable (bug glim-time/glim_v2).
  const used = new Set();
  const m = state.matrix || {};
  (m.rows || []).concat(m.archived || []).forEach((r) => { if (r.project) (r.vitrines || []).forEach((v) => { if (v.full_name) used.add(v.full_name.toLowerCase()); }); });
  openModal("Choisir une vitrine existante", `<div class="field"><div id="vep-list"><div class="skeleton"></div></div></div>`, null);
  if (el("modal-ok")) el("modal-ok").classList.add("hidden");
  if (el("modal-cancel")) el("modal-cancel").textContent = "Retour";
  let repos = [];
  try { repos = (await App().Repos(inst)) || []; } catch (e) { if (isLocked(e)) return handleLock(); el("vep-list").innerHTML = `<div class="callout warn">${esc(cleanErr(e))}</div>`; return; }
  const cands = repos.filter((r) => r.full_name && !r.archived && !used.has(r.full_name.toLowerCase()));
  if (!cands.length) { el("vep-list").innerHTML = blankslate(ico("folder", 30), "Aucune vitrine disponible", "Tous les dépôts de cette connexion sont déjà appairés, créez plutôt un nouveau dépôt."); return; }
  el("vep-list").innerHTML = `<div class="sub" style="margin-bottom:8px">Vitrines non appairées sur <b>${esc(inst)}</b>, cliquez pour affilier à <b>${esc(row.name)}</b> :</div>
    <div class="vep-rows">${cands.map((r, i) => `<button type="button" class="vep-row" data-i="${i}">${ico("folder", 14)} <b>${esc(r.name)}</b> <span class="muted">${esc(r.full_name)}</span></button>`).join("")}</div>`;
  el("vep-list").querySelectorAll("[data-i]").forEach((b) => b.onclick = async () => {
    const r = cands[+b.dataset.i]; if (!r) return;
    const owner = r.full_name.includes("/") ? r.full_name.split("/")[0] : "";
    el("vep-list").querySelectorAll("[data-i]").forEach((x) => x.disabled = true);
    try {
      // ASSEMBLE la paire (suggestion), ne CONFIRME pas : la vitrine se rattache à la ligne et le
      // bouton « Appairer » apparaît — c'est l'utilisateur qui appaire.
      await App().AddVitrineHint(atelierRef, { instance: inst, owner, repo: r.name });
      closeModal(); toast("Vitrine « " + r.name + " » assemblée, cliquez « Appairer » pour confirmer", "ok");
      if (typeof renderOrganize === "function") renderOrganize();
    } catch (e) { el("vep-list").querySelectorAll("[data-i]").forEach((x) => x.disabled = false); if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); }
  });
}
