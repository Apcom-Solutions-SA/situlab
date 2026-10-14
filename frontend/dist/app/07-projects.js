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
      <div class="pboard-ttl">${ico}<b class="pboard-name">${esc(b.name)}</b> <span class="muted">${esc(atl.owner)}/${esc(atl.repo)}</span></div>
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
  // Tags RÉELLEMENT publiés sur l'atelier (API) : sert à signaler honnêtement un tag LOCAL non poussé
  // (git describe local montrerait sinon une version que l'atelier n'a pas). `known` reste faux si on
  // n'a pas pu lire les tags (hors-ligne) → on ne marque alors JAMAIS « non poussé » par prudence.
  let atlTags = new Set(), atlTagsKnown = false;
  try {
    if (b.atelier) {
      for (const t of (await App().RepoTags(b.atelier.instance, b.atelier.owner, b.atelier.repo)) || []) if (t.name) atlTags.add(t.name);
      atlTagsKnown = true;
    }
  } catch (_) {}
  if (el("pg-" + idx)) box.innerHTML = projectGraphHTML(b, branches, atlTags, atlTagsKnown);
}

// projectGraphHTML : une VOIE par branche, 3 zones Local · Atelier · Production. Nœud par zone où la
// branche existe, segment coloré selon l'état ; nuance ambre si modifs non enregistrées.
function projectGraphHTML(b, branches, atlTags, atlTagsKnown) {
  if (!branches.length) return `<div class="muted" style="padding:8px 2px">Aucune branche.</div>`;
  atlTags = atlTags || new Set();
  const pub = (v) => !!v && atlTags.has(v); // ce tag existe-t-il VRAIMENT sur l'atelier ?
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
    // Nœud LOCAL — on signale un tag LOCAL non encore publié sur l'atelier (marqueur ⬆ + ambre).
    const locVer = br.local_version || "";
    const locUnpushed = atlTagsKnown && br.has_local && !!locVer && !pub(locVer);
    const locLabel = locVer ? (locVer + (locUnpushed ? " ⬆" : "")) : "";
    const locTitle = (dirty
      ? "travail en cours (" + b.local.changed + " fichier" + (b.local.changed > 1 ? "s" : "") + ")"
      : "sur ce poste")
      + (locVer ? " · " + locVer + (locUnpushed ? " — version LOCALE, pas encore publiée sur l'atelier" : "") : "");
    const localNode = br.has_local
      ? node(`pg-local${dirty ? " pg-dirty" : ""}${hereBranch ? " pg-here" : ""}${locUnpushed ? " pg-unpushed" : ""}`, locLabel, locTitle)
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
    // Nœud ATELIER : vert quand à jour, ambre quand il lui manque des commits locaux (br.ahead).
    // Version AFFICHÉE = la version réellement PUBLIÉE sur l'atelier (et non le `git describe` local qui
    // pourrait montrer un tag local non poussé). Si la pointe distante n'est pas exactement une version
    // publiée, on retombe (branche de prod) sur la dernière version publiée du dépôt, sinon rien.
    let atlVer = br.remote_version || "", atlExtra = "";
    if (atlTagsKnown && br.has_remote) {
      if (pub(br.remote_version)) atlVer = br.remote_version;
      else if (br.name === prodBranch && b.atelier && b.atelier.version) { atlVer = b.atelier.version; atlExtra = " (dernière version publiée)"; }
      else { atlVer = ""; atlExtra = " (aucune version publiée à cette pointe)"; }
    }
    const atelierNode = br.has_remote
      ? node(`pg-atelier${br.has_local && br.ahead ? " pg-late" : ""}`, atlVer,
          "sur l'atelier" + (atlVer ? " · " + atlVer : "") + atlExtra)
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
    toast(res && res.summary ? res.summary : "Synchronisé", "ok"); refreshProjectsView();
  } catch (e) {
    if (isLocked(e)) return handleLock();
    if (isPinNeeded(e)) {
      pinConfirm("Synchroniser l'atelier « " + b.atelier.owner + "/" + b.atelier.repo + " »", async (pin) => {
        const res = await run(pin); closeModal(); toast(res && res.summary ? res.summary : "Synchronisé", "ok"); refreshProjectsView();
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
      closeModal(); toast("Atelier copié sur votre poste", "ok"); refreshProjectsView();
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
        try { await App().RemoveProject(dp.dataset.delproj); closeModal(); toast("Projet supprimé", "ok"); state.view = "projects"; renderProjects(); } catch (e) { toast(cleanErr(e), "err"); }
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
      closeModal(); toast(res && res.summary ? res.summary : "Promu en production", "ok"); refreshProjectsView();
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

// ===================================================================================================
// Panneau git d'un projet (1.9.8) — branches + copies de travail (worktrees), MANIPULABLES en interface.
// Gestes LOCAUX (basculer, fusionner, supprimer, worktrees) : aucun PIN (le cœur garde la lecture seule).
// Supprimer une branche jamais fusionnée déclenche une alerte VISUELLE (le travail serait perdu).
// ===================================================================================================

// ---- Panneau git À ONGLETS (1.9.10) : un onglet par git — Local · Atelier · Vitrine ----
// Mêmes codes couleur que le graphe du dessus (--ok local, --accent atelier, --vitrine vitrine). Les
// 3 onglets NE sont PAS symétriques, c'est VOULU et rendu lisible : Local = établi (aucun PIN) ; Atelier
// = source partagée (supprimer une branche distante → PIN, branche par défaut protégée) ; Vitrine = prod
// publique (LECTURE + Promouvoir seulement — aucun geste destructif direct sur la prod, Règle 24).

// gpVitrineTargets : productions joignables (sans serveur en erreur), avec leur index d'ORIGINE (pour
// Promouvoir, qui indexe b.productions). gpActiveTab : onglet courant, retombe sur « local » si la
// vitrine demandée n'existe pas.
function gpVitrineTargets(b) {
  return (b.productions || []).map((p, i) => ({ ...p, origIdx: i })).filter((p) => !p.error);
}
function gpActiveTab(b) {
  const t = state.gpTab || "local";
  if (t === "vitrine" && !gpVitrineTargets(b).length) return "local";
  return t;
}

// gitPanelHTML : coquille à onglets. Le panneau existe même SANS clone local (les onglets distants lisent
// par API) ; c'est l'onglet Local qui invite à cloner si besoin. Rempli ensuite par fillGitTab.
function gitPanelHTML(b) {
  const active = gpActiveTab(b);
  const hasVitrine = gpVitrineTargets(b).length > 0;
  const tab = (id, ico, label) => `<button type="button" class="gp-tab gp-tab-${id}${active === id ? " on" : ""}" data-gptab="${id}">${ico}<span>${label}</span></button>`;
  const tabs = [tab("local", ZICON.desktop, "Local"), tab("atelier", ZICON.forgejo, "Atelier")];
  if (hasVitrine) tabs.push(tab("vitrine", ZICON.github, "Vitrine"));
  return `<div class="gitpanel" data-gp>
    <div class="gp-tabs" role="tablist">${tabs.join("")}</div>
    <div class="gp-tabcontent" id="gp-tabcontent"><div class="skeleton"></div></div>
  </div>`;
}

// fillGitTab : remplit le contenu de l'onglet courant (local / atelier / vitrine).
async function fillGitTab(b) {
  const tab = gpActiveTab(b);
  if (tab === "atelier") return fillRemoteTab(b, "atelier");
  if (tab === "vitrine") return fillRemoteTab(b, "vitrine");
  return fillLocalTab(b);
}

// fillLocalTab : l'établi — branches locales (empreinte Local·Atelier·Vitrine + gestes sans PIN) et
// copies de travail (worktrees). Sans clone local → invite à cloner.
async function fillLocalTab(b) {
  const box = el("gp-tabcontent"); if (!box) return;
  const path = b.local && b.local.path;
  if (!path) {
    box.innerHTML = `<div class="gitpanel-empty">
      <div class="gpe-ico">🖥</div>
      <div><b>Pas encore de clone sur ce poste ${helpDot("git-local")}</b>
      <p class="muted">Clonez l'atelier ici (« Cloner ici » ci-dessus) pour gérer branches et copies de travail.</p></div></div>`;
    return;
  }
  box.innerHTML = `
    <section class="gp-sec">
      <div class="gp-sec-h"><span class="gp-sec-t">⎇ Branches ${helpDot("git-local")}</span>
        <button class="btn btn-ghost btn-sm" data-gpnewbranch>＋ Nouvelle branche</button></div>
      <div class="gp-list" id="gp-branches"><div class="skeleton"></div></div>
    </section>
    <section class="gp-sec">
      <div class="gp-sec-h"><span class="gp-sec-t">🗂 Copies de travail ${helpDot("git-worktree")}</span>
        <button class="btn btn-ghost btn-sm" data-gpnewwt>＋ Nouveau worktree</button></div>
      <div class="gp-list" id="gp-worktrees"><div class="skeleton"></div></div>
    </section>`;
  const bbox = el("gp-branches");
  if (bbox) {
    try {
      const m = await App().BranchMatrix(path);
      const branches = ((m && m.branches) || []).filter((x) => x.has_local);
      // Présence VITRINE par branche : best-effort réseau. On distingue HONNÊTEMENT « absente » (serveur
      // lu, branche non trouvée) de « inconnu » (serveur non joignable) — jamais de faux « absente ».
      const prods = (b.productions || []).filter((p) => !p.error);
      let vitNames = null, vitOk = true;
      if (prods.length) {
        vitNames = new Set();
        await Promise.all(prods.map(async (p) => {
          try { ((await App().RepoBranches(p.instance, p.owner, p.repo)) || []).forEach((x) => vitNames.add(x.name)); }
          catch (_) { vitOk = false; }
        }));
      }
      const vitState = (name) => !prods.length ? "none" : (vitNames.has(name) ? "present" : (vitOk ? "absent" : "unknown"));
      if (!el("gp-branches")) return;
      bbox.innerHTML = branches.length
        ? branches.map((br) => branchRowHTML(br, vitState(br.name))).join("")
        : `<div class="muted" style="padding:6px 2px">Aucune branche locale.</div>`;
    } catch (e) { if (el("gp-branches")) bbox.innerHTML = `<div class="muted" style="padding:6px 2px">Branches indisponibles : ${esc(cleanErr(e))}</div>`; }
  }
  const wbox = el("gp-worktrees");
  if (wbox) {
    try {
      const wts = (await App().GitWorktrees(path)) || [];
      wbox.innerHTML = wts.map(worktreeRowHTML).join("") || `<div class="muted" style="padding:6px 2px">Un seul dossier de travail.</div>`;
    } catch (e) { if (el("gp-worktrees")) wbox.innerHTML = `<div class="muted" style="padding:6px 2px">Copies indisponibles : ${esc(cleanErr(e))}</div>`; }
  }
}

// fillRemoteTab : onglet ATELIER (source partagée, suppression distante sous PIN) ou VITRINE (prod :
// LECTURE + Promouvoir). Les branches distantes sont lues par API (RepoBranches). Pour l'atelier, la
// branche par défaut (récupérée via reposOf, en cache) est protégée : pas de bouton « supprimer ».
async function fillRemoteTab(b, which) {
  const box = el("gp-tabcontent"); if (!box) return;
  const atelier = which === "atelier";
  const targets = atelier ? (b.atelier ? [{ ...b.atelier, origIdx: -1 }] : []) : gpVitrineTargets(b);
  const note = atelier
    ? `<div class="gp-scope-note gp-note-atelier"><span class="gp-sn-ico">${ZICON.forgejo}</span> <b>Atelier</b> — source de vérité partagée ${helpDot("git-atelier")}</div>`
    : `<div class="gp-scope-note gp-note-vitrine"><span class="gp-sn-ico">${ZICON.github}</span> <b>Vitrine</b> — production publique, lecture seule ${helpDot("git-vitrine")}</div>`;
  if (!targets.length) {
    box.innerHTML = note + `<div class="muted" style="padding:8px 2px">${atelier ? "Aucun atelier défini pour ce projet." : "Aucune vitrine reliée."}</div>`;
    return;
  }
  box.innerHTML = note + `<div id="gp-remote-groups"><div class="skeleton"></div></div>`;
  const groups = await Promise.all(targets.map(async (t) => {
    let branches = [], err = "", def = "", tags = {};
    try {
      branches = (await App().RepoBranches(t.instance, t.owner, t.repo)) || [];
      if (atelier) { try { const rp = (await reposOf(t.instance)).find((r) => r.full_name === t.owner + "/" + t.repo); def = (rp && rp.default_branch) || ""; } catch (_) {} }
      // Correspondance exacte pointe↔version : on bâtit SHA → nom de tag (premier rencontré). Honnête :
      // on ne pourra afficher la version que si la pointe de branche EST exactement ce tag (best-effort).
      try { for (const x of (await App().RepoTags(t.instance, t.owner, t.repo)) || []) { if (x.sha && !tags[x.sha]) tags[x.sha] = x.name; } } catch (_) {}
    } catch (e) { err = cleanErr(e); }
    return { t, branches, err, def, tags };
  }));
  const gb = el("gp-remote-groups"); if (!gb) return;
  gb.innerHTML = groups.map((g) => remoteGroupHTML(g, which, b)).join("");
}

// remoteGroupHTML : un dépôt distant (atelier ou une vitrine) = en-tête + liste de branches. Vitrine :
// bouton « Promouvoir → » (reprend le tunnel existant). Atelier : suppression distante par branche.
function remoteGroupHTML(g, which, b) {
  const atelier = which === "atelier";
  const ico = atelier ? ZICON.forgejo : ZICON.github;
  const promo = !atelier ? `<button class="btn btn-primary btn-sm" data-gppromote="${esc(b.name)}" data-pidx="${g.t.origIdx}" data-plabel="${esc(g.t.owner + "/" + g.t.repo)}">Promouvoir →</button>` : "";
  const head = `<div class="gp-remote-h">${ico}<b>${esc(g.t.owner)}/${esc(g.t.repo)}</b> <span class="muted">${esc(g.t.instance)}</span>${promo}</div>`;
  if (g.err) return `<div class="gp-remote-group gp-remote-${which}">${head}<div class="muted" style="padding:6px 2px">Branches indisponibles : ${esc(g.err)}</div></div>`;
  const rows = g.branches.length ? g.branches.map((br) => remoteBranchRowHTML(br, g, which)).join("") : `<div class="muted" style="padding:6px 2px">Aucune branche.</div>`;
  return `<div class="gp-remote-group gp-remote-${which}">${head}<div class="gp-list">${rows}</div></div>`;
}

// remoteBranchRowHTML : une branche distante. La branche par défaut porte un badge et N'EST PAS
// supprimable. La suppression (atelier uniquement) est sous PIN.
function remoteBranchRowHTML(br, g, which) {
  const isDefault = g.def ? br.name === g.def : (br.name === "main" || br.name === "master");
  const tag = isDefault ? `<span class="gp-badge" title="branche par défaut — protégée">par défaut</span>` : "";
  const del = (which === "atelier" && !isDefault)
    ? `<button class="lnk danger" data-gprdel="${esc(br.name)}" data-inst="${esc(g.t.instance)}" data-owner="${esc(g.t.owner)}" data-repo="${esc(g.t.repo)}" data-scope="atelier">supprimer</button>`
    : "";
  const date = (br.commit_date || "").slice(0, 10);
  const ver = gpVerChip((g.tags && br.commit_sha) ? g.tags[br.commit_sha] : "", 0, true);
  return `<div class="gp-row">
    <div class="gp-row-main"><span class="gp-bname">⎇ ${esc(br.name)}</span> ${ver}${tag}</div>
    <div class="gp-row-bits">${date ? `<small class="muted">${esc(date)}</small>` : ""}</div>
    <div class="gp-row-acts">${del}</div>
  </div>`;
}

// deleteRemoteBranchFlow : supprimer une branche DISTANTE (atelier). PIN requis (pinConfirm gère le mode
// lecture seule + saisie). La branche par défaut est déjà écartée côté UI, et re-refusée côté cœur.
function deleteRemoteBranchFlow(d, reload) {
  pinConfirm(`Supprimer la branche distante « ${d.branch} » sur l'atelier ${d.owner}/${d.repo} — elle disparaîtra pour toute l'équipe. (Les clones locaux et la vitrine ne sont pas touchés.)`, async (pin) => {
    await App().DeleteRemoteBranch(d.inst, d.owner, d.repo, d.branch, pin);
    closeModal(); toast("Branche distante « " + d.branch + " » supprimée", "ok"); reload();
  });
}

// scopeTrioHTML : trio de pastilles « où vit la branche » — Local · Atelier · Vitrine. Réutilise le
// langage visuel du graphe (icônes poste/forgejo/github). pleine = présente, éteinte = absente, « ? » =
// inconnu (serveur non joignable → jamais de faux « absente »). vitrine "none" → pastille vitrine masquée.
function scopeTrioHTML(local, atelier, vitrine) {
  // gp-pip-<kind> porte la couleur d'EMPLACEMENT (local=cyan · atelier=rose · vitrine=violet), distincte
  // de la palette d'état, pour qu'on identifie l'emplacement d'un coup d'œil sans confusion.
  const kindCls = { desktop: "gp-pip-local", forgejo: "gp-pip-atelier", github: "gp-pip-vitrine" };
  const pip = (kind, on, label, unknown) => `<span class="gp-pip ${kindCls[kind] || ""} ${on ? "on" : (unknown ? "unknown" : "off")}" title="${esc(label)}">${ZICON[kind]}${unknown ? `<span class="gp-pip-q">?</span>` : ""}</span>`;
  const out = [
    pip("desktop", !!local, local ? "présente sur ce poste (local)" : "absente de ce poste"),
    pip("forgejo", !!atelier, atelier ? "présente sur l'atelier" : "absente de l'atelier"),
  ];
  if (vitrine && vitrine !== "none") {
    out.push(pip("github", vitrine === "present",
      vitrine === "present" ? "présente sur la vitrine" : (vitrine === "unknown" ? "vitrine : état inconnu (serveur non joignable)" : "absente de la vitrine"),
      vitrine === "unknown"));
  }
  return `<span class="gp-scope" aria-hidden="true">${out.join("")}</span>`;
}

// gpVerChip : puce « 🏷 version » d'une branche. ahead>0 → « 🏷 v1.9.10 +N » (N commits APRÈS la version).
// exactOnly=true (onglets distants) : on n'affiche que si la pointe est PILE sur un tag (match SHA), car
// l'API distante ne permet pas de calculer honnêtement l'écart ; vide sinon.
function gpVerChip(name, ahead, exactOnly) {
  if (!name) return "";
  const n = ahead | 0;
  if (exactOnly && n > 0) return "";
  const txt = n > 0 ? `${esc(name)} +${n}` : esc(name);
  const title = n > 0 ? `${n} commit(s) après la version ${name}` : `pile sur la version ${name}`;
  return `<span class="gp-ver ${n > 0 ? "gp-ver-ahead" : "gp-ver-exact"}" title="${esc(title)}">🏷 ${txt}</span>`;
}

// branchRowHTML : une ligne de branche locale, avec son EMPREINTE (trio Local·Atelier·Vitrine), son état
// et ses actions. Le trio répond à « où vit cette branche » AVANT toute suppression. Une branche NON
// fusionnée (hors courante) porte un badge d'avertissement. `vitrine` : present|absent|unknown|none.
function branchRowHTML(br, vitrine) {
  vitrine = vitrine || "none";
  const cur = br.current ? `<span class="gp-cur" title="branche courante de ce dossier">● ici</span>` : "";
  const ver = gpVerChip(br.local_version, br.local_ver_ahead, false);
  const bits = [];
  if (br.ahead) bits.push(`<span class="gp-ahead" title="${br.ahead} commit(s) d'avance sur l'atelier (non publiés)">↑${br.ahead}</span>`);
  if (br.behind) bits.push(`<span class="gp-behind" title="${br.behind} commit(s) de retard sur l'atelier">↓${br.behind}</span>`);
  const scope = scopeTrioHTML(true, !!br.has_remote, vitrine);
  let badge = "";
  if (!br.current) {
    badge = br.merged
      ? `<span class="gp-badge gp-merged" title="fusionnée dans l'intégration — sûre à supprimer">fusionnée</span>`
      : `<span class="gp-badge gp-unmerged" title="jamais fusionnée : la supprimer perdrait son travail">non fusionnée</span>`;
  }
  const acts = br.current ? "" : `
    <button class="lnk" data-gpswitch="${esc(br.name)}">basculer</button>
    <button class="lnk" data-gpmerge="${esc(br.name)}" title="fusionner « ${esc(br.name)} » dans la branche courante">fusionner ici</button>
    <button class="lnk danger" data-gpdel="${esc(br.name)}" data-merged="${br.merged ? "1" : "0"}" data-atelier="${br.has_remote ? "1" : "0"}" data-vitrine="${esc(vitrine)}" data-ahead="${br.ahead || 0}">supprimer</button>`;
  return `<div class="gp-row">
    <div class="gp-row-main">${scope}<span class="gp-bname">⎇ ${esc(br.name)}</span> ${ver}${cur}${badge}</div>
    <div class="gp-row-bits">${bits.join(" ")}${br.author ? ` <small class="muted">· ${esc(br.author)}</small>` : ""}</div>
    <div class="gp-row-acts">${acts}</div>
  </div>`;
}

// worktreeRowHTML : une ligne de worktree. Le worktree PRINCIPAL ne se retire pas (c'est le dépôt).
function worktreeRowHTML(w) {
  const main = w.main ? `<span class="gp-badge">principal</span>` : "";
  const lock = w.locked ? `<span class="gp-badge" title="worktree verrouillé">🔒</span>` : "";
  const br = w.branch ? `⎇ ${esc(w.branch)}` : (w.detached ? "<span class='muted'>(détaché)</span>" : (w.bare ? "<span class='muted'>(nu)</span>" : ""));
  const rm = w.main ? "" : `<button class="lnk danger" data-gpwtrm="${esc(w.path)}" title="retirer ce worktree (son dossier sera supprimé)">retirer</button>`;
  return `<div class="gp-row">
    <div class="gp-row-main"><span class="gp-wfolder" title="${esc(w.path)}">📁 ${esc(folderOf(w.path))}</span> ${main}${lock}</div>
    <div class="gp-row-bits">${br}</div>
    <div class="gp-row-acts">${rm}</div>
  </div>`;
}

// bindGitPanel : délégation des actions du panneau À ONGLETS. refresh() re-remplit l'onglet courant
// après un geste (et, câblé depuis le détail d'un projet, rafraîchit aussi le graphe au-dessus). Ne
// dépend PAS d'un clone local : les onglets distants et le changement d'onglet marchent sans clone ;
// les gestes locaux ne sont câblés que si leurs boutons existent (onglet Local avec clone).
function bindGitPanel(b, refresh) {
  const root = el("proj-gitpanel"); if (!root) return;
  const reload = refresh || (() => fillGitTab(b));
  const path = b.local && b.local.path;
  root.onclick = async (ev) => {
    // Changement d'onglet (bascule la classe active + remplit).
    const tb = ev.target.closest("[data-gptab]");
    if (tb) {
      state.gpTab = tb.dataset.gptab;
      root.querySelectorAll(".gp-tab").forEach((x) => x.classList.toggle("on", x.dataset.gptab === state.gpTab));
      return fillGitTab(b);
    }
    // Onglets DISTANTS : suppression (atelier, PIN) et Promouvoir (vitrine).
    const rdl = ev.target.closest("[data-gprdel]");
    if (rdl) return deleteRemoteBranchFlow({ branch: rdl.dataset.gprdel, inst: rdl.dataset.inst, owner: rdl.dataset.owner, repo: rdl.dataset.repo }, reload);
    const pr = ev.target.closest("[data-gppromote]");
    if (pr) return promoteWizard(b, parseInt(pr.dataset.pidx, 10), pr.dataset.plabel);
    // Onglet LOCAL (ces boutons n'existent que s'il y a un clone).
    if (!path) return;
    const sw = ev.target.closest("[data-gpswitch]");
    if (sw) {
      try { await App().GitSwitchBranch(path, sw.dataset.gpswitch); toast("Sur la branche « " + sw.dataset.gpswitch + " »", "ok"); reload(); }
      catch (e) { if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); }
      return;
    }
    const mg = ev.target.closest("[data-gpmerge]"); if (mg) return mergeBranchFlow(path, mg.dataset.gpmerge, reload);
    const dl = ev.target.closest("[data-gpdel]");
    if (dl) return deleteBranchFlow(path, dl.dataset.gpdel, {
      merged: dl.dataset.merged === "1",
      atelier: dl.dataset.atelier === "1",
      vitrine: dl.dataset.vitrine,
      ahead: parseInt(dl.dataset.ahead || "0", 10),
    }, reload);
    const nb = ev.target.closest("[data-gpnewbranch]"); if (nb) return newBranchModal(path, reload);
    const nw = ev.target.closest("[data-gpnewwt]"); if (nw) return addWorktreeModal(b, reload);
    const wr = ev.target.closest("[data-gpwtrm]"); if (wr) return removeWorktreeFlow(path, wr.dataset.gpwtrm, reload);
  };
}

// newBranchModal : créer une branche locale (et basculer dessus).
function newBranchModal(path, reload) {
  openModal("Nouvelle branche", `<div class="field">
    <label>Nom de la branche</label><input id="nb-name" class="inp" placeholder="ex. feat/ma-fonction" autocomplete="off" />
    <div class="inline-error hidden" id="nb-err"></div></div>`, async () => {
    const name = (el("nb-name").value || "").trim(); const er = el("nb-err");
    if (!name) { er.textContent = "Donnez un nom."; er.classList.remove("hidden"); return; }
    try { await App().GitCreateBranch(path, name); closeModal(); toast("Branche « " + name + " » créée", "ok"); reload(); }
    catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  el("modal-ok").textContent = "Créer";
  const i = el("nb-name"); if (i) { i.focus(); i.onkeydown = (e) => { if (e.key === "Enter") modalOk(); }; }
}

// mergeBranchFlow : fusionner une branche dans la courante. Conflit → message guidé + « annuler la fusion ».
function mergeBranchFlow(path, branch, reload) {
  openModal("Fusionner une branche", `<div class="field">
    <div class="callout">Fusionner <b>⎇ ${esc(branch)}</b> dans la branche <b>courante</b> de ce dossier.</div>
    <p class="sub">La branche courante reçoit les commits de « ${esc(branch)} ». En cas de conflit, SituLab vous guidera.</p></div>`, async () => {
    try { await App().GitMerge(path, branch); closeModal(); toast("Fusionné", "ok"); reload(); }
    catch (e) {
      closeModal();
      const msg = cleanErr(e);
      if (/conflit|conflict/i.test(msg)) {
        openModal("Conflit de fusion", `<div class="field"><div class="callout warn">La fusion a rencontré un <b>conflit</b>. Résolvez les fichiers en conflit puis enregistrez, ou annulez la fusion pour revenir en arrière.</div></div>`, async () => {
          try { await App().GitMergeAbort(path); closeModal(); toast("Fusion annulée", "ok"); reload(); }
          catch (e2) { toast(cleanErr(e2), "err"); }
        });
        el("modal-ok").textContent = "Annuler la fusion"; el("modal-ok").classList.add("danger-btn");
      } else { toast(msg, "err"); }
    }
  });
  el("modal-ok").textContent = "Fusionner";
}

// deleteBranchFlow : supprimer une branche LOCALE, avec un message CONSCIENT DE LA PORTÉE (où la branche
// survit). scope = {merged, atelier(bool), vitrine(present|absent|unknown|none), ahead(int)}. Quatre cas
// honnêtes : (A) vit aussi sur atelier/vitrine sans commit local en attente → serein, rien perdu ;
// (B) idem mais des commits locaux non publiés → prudent, perte réelle ; (C) locale seule mais fusionnée
// → doux ; (D) locale seule jamais fusionnée → alerte forte. Geste 100 % local (pas de PIN).
function deleteBranchFlow(path, branch, scope, reload) {
  scope = scope || {};
  const onAtelier = !!scope.atelier, onVitrine = scope.vitrine === "present";
  const elsewhere = onAtelier || onVitrine, aheadLost = (scope.ahead || 0) > 0;
  const where = [onAtelier ? "l'atelier" : null, onVitrine ? "la vitrine" : null].filter(Boolean);
  const whereTxt = where.length === 2 ? "l'atelier et la vitrine" : (where[0] || "");

  const doDelete = async (force) => {
    try { await App().GitDeleteBranch(path, branch, force); closeModal(); toast("Branche « " + branch + " » supprimée", "ok"); reload(); }
    catch (e) {
      const msg = cleanErr(e);
      // Repli : si git refuse (branche non fusionnée détectée côté git) et qu'elle ne vit pas ailleurs,
      // bascule sur l'alerte forte. Sinon on force (le travail existe sur atelier/vitrine).
      if (!force && /fusionn|not fully merged|merged/i.test(msg)) {
        if (elsewhere && !aheadLost) return doDelete(true);
        closeModal(); return unmergedAlert();
      }
      toast(msg, "err");
    }
  };
  const unmergedAlert = () => {
    openModal("🗑 Supprimer une branche NON fusionnée ?", `<div class="field">
      <div class="danger-hero"><div class="dh-ico">🗑️</div><div class="dh-burst">⚠️</div></div>
      <div class="callout danger"><b>« ${esc(branch)} » n'existe que sur ce poste et n'a jamais été fusionnée.</b><br>Son travail n'existe nulle part ailleurs : le supprimer, c'est le <b>perdre définitivement</b>.</div>
      <p class="sub">Si vous en êtes sûr·e, confirmez. Sinon, fusionnez-la d'abord (« fusionner ici »).</p></div>`, () => doDelete(true));
    el("modal-ok").textContent = "Supprimer quand même"; el("modal-ok").classList.add("danger-btn");
  };

  // (A) Vit aussi sur atelier/vitrine, rien en attente localement → serein.
  if (elsewhere && !aheadLost) {
    openModal("Supprimer la copie locale ?", `<div class="field">
      <div class="callout ok"><b>⎇ ${esc(branch)}</b> existe aussi sur <b>${whereTxt}</b>.<br>Vous retirez seulement la copie de <b>ce poste</b> — re-clonable à tout moment. <b>Aucun travail n'est perdu.</b></div>
      <div class="gp-del-scope">Où vit la branche : ${scopeTrioHTML(true, onAtelier, scope.vitrine)}</div></div>`, () => doDelete(true));
    el("modal-ok").textContent = "Retirer la copie locale";
    return;
  }
  // (B) Vit ailleurs MAIS des commits locaux ne sont pas encore publiés → perte réelle.
  if (elsewhere && aheadLost) {
    openModal("⚠ Commits locaux non publiés", `<div class="field">
      <div class="callout warn"><b>« ${esc(branch)} » a ${scope.ahead} commit(s) d'avance</b> qui ne sont <b>pas encore</b> sur ${whereTxt}.<br>Supprimer la copie locale maintenant <b>perdrait ces ${scope.ahead} commit(s)</b> — la version de ${whereTxt} est plus ancienne.</div>
      <p class="sub">Pour les garder, publiez d'abord (« Synchroniser »). Sinon, confirmez la perte.</p></div>`, () => doDelete(true));
    el("modal-ok").textContent = "Supprimer malgré la perte"; el("modal-ok").classList.add("danger-btn");
    return;
  }
  // (C) Locale seule, mais fusionnée dans l'intégration → aucun travail perdu.
  if (scope.merged) {
    openModal("Supprimer la branche ?", `<div class="field"><div class="callout">La branche <b>⎇ ${esc(branch)}</b> n'existe que sur ce poste, mais elle est <b>fusionnée dans l'intégration</b> : la supprimer ne perd aucun travail.</div></div>`, () => doDelete(false));
    el("modal-ok").textContent = "Supprimer"; el("modal-ok").classList.add("danger-btn");
    return;
  }
  // (D) Locale seule, jamais fusionnée → alerte forte.
  unmergedAlert();
}

// addWorktreeModal : créer un worktree (autre dossier, autre branche, même historique). Option « nouvelle
// branche » pour démarrer une ligne de travail neuve (ex. une session IA parallèle).
function addWorktreeModal(b, reload) {
  const path = b.local.path;
  openModal("Nouveau worktree", `<div class="field">
    <p class="sub">Un worktree ouvre une <b>autre branche</b> dans un <b>autre dossier</b>, en partageant l'historique de ce dépôt. Idéal pour travailler en parallèle sans re-cloner.</p>
    <label>Dossier du worktree (sera créé)</label>
    <input id="wt-dir" class="inp" placeholder="C:\\work\\${esc((b.atelier && b.atelier.repo) || "projet")}-parallele" autocomplete="off" />
    <label style="margin-top:8px">Branche</label>
    <input id="wt-branch" class="inp" placeholder="ex. feat/essai" autocomplete="off" />
    <label class="set-row toggle" style="margin-top:8px"><div class="set-row-l">Créer une nouvelle branche <small>sinon, extraire une branche existante</small></div>
      <span class="switch"><input id="wt-new" type="checkbox" checked /><span class="track"></span></span></label>
    <div class="inline-error hidden" id="wt-err"></div></div>`, async () => {
    const dir = (el("wt-dir").value || "").trim(), branch = (el("wt-branch").value || "").trim(), neuf = el("wt-new").checked, er = el("wt-err");
    if (!dir || !branch) { er.textContent = "Indiquez un dossier et une branche."; er.classList.remove("hidden"); return; }
    try { await App().GitWorktreeAdd(path, dir, branch, neuf); closeModal(); toast("Worktree créé", "ok"); reload(); }
    catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  el("modal-ok").textContent = "Créer le worktree";
}

// removeWorktreeFlow : retirer un worktree (son dossier est supprimé). Si modifications non enregistrées,
// git refuse → on propose le retrait forcé.
function removeWorktreeFlow(path, wtPath, reload) {
  const doRemove = async (force) => {
    try { await App().GitWorktreeRemove(path, wtPath, force); closeModal(); toast("Worktree retiré", "ok"); reload(); }
    catch (e) {
      const msg = cleanErr(e);
      if (!force && /modif|dirty|contains modified|use --force/i.test(msg)) {
        closeModal();
        openModal("Worktree avec des modifications", `<div class="field"><div class="callout warn">Ce worktree a des <b>modifications non enregistrées</b>. Le retirer de force les <b>perdra</b>.</div></div>`, () => doRemove(true));
        el("modal-ok").textContent = "Retirer de force"; el("modal-ok").classList.add("danger-btn");
        return;
      }
      toast(msg, "err");
    }
  };
  openModal("Retirer ce worktree ?", `<div class="field"><div class="callout warn">Le dossier <b>${esc(folderOf(wtPath))}</b> sera <b>supprimé</b> de ce poste. L'historique du dépôt n'est pas touché.</div></div>`, () => doRemove(false));
  el("modal-ok").textContent = "Retirer"; el("modal-ok").classList.add("danger-btn");
}

// refreshProjectsView : re-rendu de la bonne vue selon qu'on est sur l'accueil « classeurs » ou dans
// le détail d'un projet. Utilisé par les actions partagées (sync, promote, clone, modifier).
function refreshProjectsView() {
  if (state.view === "project" && state.projectName) renderProject(state.projectName);
  else renderProjects();
}

// classeurVerdict : un mot d'état HONNÊTE, déduit du SEUL clone local (aucun réseau). « à jour » n'est
// pas affirmé (il exigerait le serveur) : on parle du clone (propre / en cours / à publier / en retard).
function classeurVerdict(p) {
  const loc = p.local;
  if (!loc) return { tone: "muted", label: "non cloné ici" };
  if (!loc.clean) return { tone: "wip", label: "travail en cours" };
  if (loc.ahead && loc.behind) return { tone: "wip", label: "à réunir" };
  if (loc.ahead) return { tone: "accent", label: "à publier ↑" + loc.ahead };
  if (loc.behind) return { tone: "warn", label: "en retard ↓" + loc.behind };
  return { tone: "ok", label: "propre" };
}

// chealthHTML : glyphe de santé COMPACT et STATIQUE (jamais de pulse : il dit « dernier état connu »).
// Triptyque Local · Atelier · Vitrine, lu 100% sans réseau : présence (plein / pointillé), état du clone
// local (couleur), nombre de vitrines (compteur violet = identité production). aria porté par le classeur.
function chealthHTML(p) {
  const loc = p.local;
  const localState = !loc ? "absent" : (!loc.clean ? "dirty" : (loc.ahead ? "ahead" : (loc.behind ? "behind" : "clean")));
  const vit = (p.vitrines || 0) > 0 ? "1" : "0";
  return `<span class="chealth" aria-hidden="true">
    <span class="chealth-node" data-role="local" data-state="${localState}"></span>
    <span class="chealth-seg" data-on="${loc ? "1" : "0"}"></span>
    <span class="chealth-node" data-role="atelier" data-state="on"></span>
    <span class="chealth-seg" data-on="${vit}"></span>
    <span class="chealth-node" data-role="vitrine" data-state="${vit === "1" ? "on" : "absent"}"></span>
    ${(p.vitrines || 0) > 0 ? `<span class="chealth-count">×${p.vitrines}</span>` : ""}
  </span>`;
}

// classeurCardHTML : un projet = un classeur à onglet (même langage visuel que la carte détail .pboard).
// Onglet = icône + nom ; corps = atelier + glyphe de santé + verdict. Le classeur entier ouvre le détail.
function classeurCardHTML(p) {
  const atl = p.atelier || {};
  const v = classeurVerdict(p);
  const aria = `${p.name} — atelier ${atl.owner || ""}/${atl.repo || ""}${p.local ? ", cloné sur ce poste" : ", pas de clone local"}, ${p.vitrines || 0} vitrine(s), ${v.label}`;
  return `<button type="button" class="classeur" data-open-project="${esc(p.name)}" aria-label="${esc(aria)}">
    <span class="classeur-tab">${iconHTML(p.icon, "classeur-ico")}<span class="classeur-name">${esc(p.name)}</span></span>
    <span class="classeur-body">
      <span class="classeur-repo"><span class="classeur-k">atelier</span> <span class="org">${esc(atl.owner || "")}/</span>${esc(atl.repo || "")}</span>
      <span class="classeur-foot">${chealthHTML(p)}<span class="classeur-state" data-tone="${v.tone}">${esc(v.label)}</span></span>
    </span>
  </button>`;
}

// openProject : ouvre le détail d'un projet (vue « project »).
function openProject(name) {
  state.projectName = name;
  state.view = "project";
  render();
}

// renderProjects(live) : ACCUEIL « classeurs » — LÉGER et INSTANTANÉ (App().ProjectsLight(), zéro réseau).
// Le graphe lourd (branches, tags) part au clic, dans le détail. live=true recharge en silence sans
// flicker (signature). C'est la réponse à « la page Projets est lente » : plus d'appel réseau par carte.
async function renderProjects(live) {
  if (!live) {
    el("view").innerHTML = `<div class="view-head view-head-row">
      <div><div class="view-title">Projets</div><div class="view-sub">Vos classeurs — un clic ouvre le détail et vérifie l'état en ligne.</div></div>
      <button class="btn btn-primary btn-sm" id="proj-add">＋ Nouveau projet</button></div>
    <div id="proj-home"><div class="skeleton"></div><div class="skeleton" style="margin-top:10px"></div></div>`;
    el("proj-add").onclick = addProjectModal;
    state.projSig = "";
  }
  let rows = [];
  try { rows = (await App().ProjectsLight()) || []; }
  catch (e) { if (isLocked(e)) return handleLock(); if (!live) { const b = el("proj-home"); if (b) b.innerHTML = surfaceError("Lecture impossible", cleanErr(e), null); } return; }
  const sig = JSON.stringify(rows);
  if (live && sig === state.projSig) return; // rien de neuf : on ne touche pas au DOM
  state.projSig = sig;
  const home = el("proj-home"); if (!home) return;
  if (!rows.length) {
    home.innerHTML = `<div class="proj-empty">
      <div class="pe-ico">⇄</div>
      <div class="pe-title">Reliez votre premier projet</div>
      <p class="pe-txt">Un projet relie un dépôt <b>atelier</b> à sa <b>vitrine</b>, et montre en une vue l'état entre <b>poste</b>, <b>atelier</b> et <b>prod</b>.</p>
      <button class="btn btn-primary" id="proj-add2">＋ Créer mon premier projet</button>
    </div>`;
    if (el("proj-add2")) el("proj-add2").onclick = addProjectModal;
    return;
  }
  home.innerHTML = `<div class="classeur-grid">${rows.map(classeurCardHTML).join("")}</div>`;
  home.onclick = (ev) => { const c = ev.target.closest("[data-open-project]"); if (c) openProject(c.dataset.openProject); };
}

// renderProject(name) : DÉTAIL d'un projet — le tableau 3 colonnes (réseau pour ce projet seul) + le
// panneau git (branches, fusions, worktrees : local, sans PIN). Bouton retour vers l'accueil « classeurs ».
async function renderProject(name) {
  if (!name) { state.view = "projects"; return renderProjects(); }
  state.projSig = "";
  el("view").innerHTML = `<div class="view-head view-head-row">
      <div><button class="btn btn-ghost btn-sm" id="proj-back">‹ Projets</button>
      <div class="view-title" style="margin-top:6px">${esc(name)}</div></div></div>
    <div id="proj-board"><div class="skeleton"></div></div>
    <div class="gp-head">🔧 Panneau git</div>
    <div id="proj-gitpanel"><div class="skeleton"></div></div>`;
  el("proj-back").onclick = () => { state.view = "projects"; render(); };
  let b;
  try { b = await App().ProjectBoardOne(name); }
  catch (e) { if (isLocked(e)) return handleLock(); const bd = el("proj-board"); if (bd) bd.innerHTML = surfaceError("Projet indisponible", cleanErr(e), null); return; }
  if (!b) return;
  const board = el("proj-board"); if (board) {
    board.innerHTML = projectGraphCard(b, 0);
    bindProjectBoard([b]);
    fillProjectGraph(b, 0);
  }
  const gp = el("proj-gitpanel");
  if (gp) {
    gp.innerHTML = gitPanelHTML(b);
    // Panneau à onglets (1.9.10) : fonctionne même SANS clone (les onglets distants lisent par API). On
    // re-remplit l'onglet COURANT après un geste + le graphe au-dessus (local : BranchMatrix sans réseau).
    bindGitPanel(b, () => { fillGitTab(b); fillProjectGraph(b, 0); });
    fillGitTab(b);
  }
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
      const newName = (E.name || "").trim();
      await App().UpdateProject(oldName, newName, E.icon, E.atelier, E.productions);
      // Si on renomme depuis le détail, réaligner la vue sur le NOUVEAU nom (sinon renderProject(ancien)
      // → « projet introuvable » après une opération pourtant réussie).
      if (state.view === "project" && state.projectName === oldName) state.projectName = newName;
      closeModal(); toast("Projet modifié", "ok"); refreshProjectsView();
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

