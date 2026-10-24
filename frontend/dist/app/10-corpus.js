/* ---------------- CORPUS : clouds de fichiers souverains (1.10) ----------------
 * SituLab est la PORTE UNIQUE entre l'IA du poste et les clouds de fichiers. Dans la FENÊTRE,
 * l'humain a un accès complet : il explore, importe, télécharge, et surtout CHOISIT les dossiers
 * que l'IA pourra voir (allowlist). L'agent (CLI/MCP) ne voit que ces dossiers (borne imposée par
 * le démon). Cibles souveraines : Nextcloud (auto-hébergé) et kDrive (Infomaniak).
 */

// cloudLogo : logo inline (CSP-safe) selon le type de cloud.
function cloudLogo(kind) {
  if (kind === "kdrive") {
    return `<svg viewBox="0 0 24 24" width="26" height="26" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 19h11a4 4 0 0 0 .5-7.97A6 6 0 0 0 6 10a4.5 4.5 0 0 0 0 9z"/><path d="M9.5 14.5 12 12l2.5 2.5"/></svg>`;
  }
  return `<svg viewBox="0 0 24 24" width="26" height="26" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 19h11a4 4 0 0 0 .5-7.97A6 6 0 0 0 6 10a4.5 4.5 0 0 0 0 9z"/><circle cx="12" cy="13.5" r="1.4"/></svg>`;
}

// cloudNeedsPIN : miroir fidèle de defaultCloudRequiresPIN côté Go (write autonome, delete sous PIN).
function cloudNeedsPIN(fc, action) {
  const def = action === "delete";
  const p = fc && fc.pin_policy;
  if (!p) return def;
  const v = action === "delete" ? p.delete : p.write;
  return v === undefined || v === null ? def : !!v;
}

function ccJoin(dir, name) {
  dir = (dir || "/").replace(/\/+$/, "");
  return (dir === "" ? "" : dir) + "/" + name;
}
function ccParent(p) {
  p = (p || "/").replace(/\/+$/, "");
  const i = p.lastIndexOf("/");
  return i <= 0 ? "/" : p.slice(0, i);
}

// Glisser-déposer : relayé par Go (event "corpus:drop" avec les chemins absolus). Lié une seule fois.
function bindCorpusDrop() {
  if (state._corpusDropBound) return;
  if (!(window.runtime && window.runtime.EventsOn)) return;
  try { window.runtime.EventsOn("corpus:drop", onCorpusDrop); state._corpusDropBound = true; } catch (_) {}
}
function onCorpusDrop(paths) {
  if (!paths || !paths.length) return;
  if (state.view !== "corpus" || state.corpusView !== "cloud") { toast("Ouvrez un cloud (CORPUS) pour y déposer des fichiers", "err"); return; }
  if (!state.corpusCloud) { toast("Ouvrez un cloud avant de déposer des fichiers", "err"); return; }
  if (state.readOnly) { toast("Mode lecture seule actif — désactivez-le dans Réglages", "err"); return; }
  const fc = (state.corpusClouds || []).find((c) => c.name === state.corpusCloud);
  if (!fc) return;
  const dir = state.corpusPath || "/";
  pinConfirmOrSession(fc, "write", `Déposer ${paths.length} fichier(s) dans « ${dir} »`, async (pin) => {
    let n = 0;
    for (const p of paths) { await App().CloudUpload(fc.name, p, dir, pin); n++; }
    toast(n + " fichier(s) déposé(s)", "ok");
  }, () => loadCorpusDir());
}

// renderCorpus : dispatcher — board des clouds (accueil) ou page d'un cloud (explorateur + réglages).
async function renderCorpus() {
  bindCorpusDrop();
  let clouds;
  try {
    clouds = (await App().FileClouds()) || [];
  } catch (e) {
    if (isLocked(e)) return handleLock();
    el("view").innerHTML = blankslate("⚠", "Clouds indisponibles", cleanErr(e));
    return;
  }
  state.corpusClouds = clouds;
  if (state.corpusView === "cloud" && clouds.find((c) => c.name === state.corpusCloud)) {
    return renderCorpusCloud(clouds.find((c) => c.name === state.corpusCloud), clouds);
  }
  state.corpusView = "board";
  renderCorpusBoard(clouds);
}

// cloudAccessSummary : résumé d'accès IA d'une connexion — "on" (tout), "partial" (restreint), "off".
function cloudAccessSummary(fc) {
  const A = (fc.allowed_dirs || []).map(ccClean);
  if (!A.length) return "off";
  if (A.includes("/") && !(fc.denied_dirs || []).length) return "on";
  return "partial";
}

// renderCorpusBoard : accueil CORPUS — une carte par cloud (logo, défaut, joignabilité, accès IA).
async function renderCorpusBoard(clouds) {
  const view = el("view");
  view.innerHTML = `
    <div class="view-head"><div class="view-title">CORPUS</div>
      <button class="btn btn-primary btn-sm" id="cb-add">＋ Ajouter un cloud</button></div>
    <p class="cc-intro">Vos clouds de fichiers souverains. Cliquez une carte pour explorer, importer (aussi par glisser-déposer) et régler l'accès de l'IA.</p>
    <div class="cloud-board" id="cloud-board">${clouds.length ? `<div class="skeleton"></div>` : ""}</div>`;
  el("cb-add").onclick = () => addCloudModal();
  const board = el("cloud-board");
  if (!clouds.length) {
    board.innerHTML = blankslate("☁", "Aucun cloud connecté", "Branchez un Nextcloud ou un kDrive pour porter votre CORPUS — l'atelier de fichiers que vous ouvrez à l'IA, dossier par dossier.");
    return;
  }
  let health = {};
  try { (await cloudHealthCached() || []).forEach((h) => { health[h.name] = h; }); } catch (_) {}
  board.innerHTML = clouds
    .map((fc) => {
      const h = health[fc.name], reach = h ? (h.ok ? "ok" : "ko") : "unknown";
      const acc = cloudAccessSummary(fc);
      const nd = (fc.allowed_dirs || []).length;
      return `<button class="cloud-card" data-cloud="${esc(fc.name)}">
        <div class="clc-top"><span class="clc-logo">${cloudLogo(fc.kind)}</span>${fc.default ? `<span class="pill sm">défaut</span>` : ""}</div>
        <div class="clc-name">${esc(fc.name)}</div>
        <div class="clc-kind">${esc(fc.kind)}</div>
        <div class="clc-badges">
          <span class="clc-badge ${reach}">${reach === "ok" ? "🟢 joignable" : reach === "ko" ? "🔴 injoignable" : "⚪ non testé"}</span>
          <span class="clc-badge">${aiGlyph(acc)} ${acc === "on" ? "IA : tout" : acc === "partial" ? `IA : ${nd} dossier(s)` : "IA : rien"}</span>
        </div></button>`;
    })
    .join("");
  board.onclick = (ev) => {
    const c = ev.target.closest("[data-cloud]");
    if (c) { state.corpusCloud = c.dataset.cloud; state.corpusPath = "/"; state.corpusView = "cloud"; renderCorpus(); }
  };
}

// renderCorpusCloud : page d'un cloud — explorateur de fichiers + réglages (Accès IA, PIN).
function renderCorpusCloud(fc, clouds) {
  if (state.corpusPath === undefined) state.corpusPath = "/";
  el("view").innerHTML = `
    <div class="cc-head">
      <div class="cc-conn">
        <button class="btn btn-ghost btn-sm" id="cc-back">‹ Mes clouds</button>
        <span class="cc-logo">${cloudLogo(fc.kind)}</span><b class="cc-title">${esc(fc.name)}</b>
      </div>
      <div class="cc-actions">
        <button class="btn btn-ghost btn-sm" id="cc-ai">🤖 Accès IA</button>
        <button class="btn btn-ghost btn-sm" id="cc-pin">🔒 PIN</button>
        ${fc.default ? "" : `<button class="btn btn-ghost btn-sm" id="cc-def">Par défaut</button>`}
        <button class="icon-del" id="cc-del" title="Supprimer ce cloud" aria-label="Supprimer ce cloud">${ICO_TRASH}</button>
      </div>
    </div>
    <div class="cc-allow" id="cc-allow"></div>
    <div class="cc-toolbar">
      <div class="cc-crumbs" id="cc-crumbs"></div>
      <div class="cc-tools">
        <button class="btn btn-ghost btn-sm" id="cc-up" title="Dossier parent">⬆</button>
        <button class="btn btn-ghost btn-sm" id="cc-mkdir">＋ Dossier</button>
        <button class="btn btn-primary btn-sm" id="cc-import" title="Bouton ou glisser-déposer dans la fenêtre">⬆ Importer</button>
      </div>
    </div>
    <div class="cc-list" id="cc-list"><div class="skeleton"></div></div>`;
  el("cc-back").onclick = () => { state.corpusView = "board"; renderCorpus(); };
  el("cc-ai").onclick = () => aiAccessTreeModal(fc);
  el("cc-pin").onclick = () => cloudPINModal(fc);
  el("cc-del").onclick = () => removeCloudFlow(fc);
  if (el("cc-def")) el("cc-def").onclick = async () => {
    try { await App().SetDefaultCloud(fc.name); toast("Cloud par défaut : " + fc.name, "ok"); renderCorpus(); } catch (e) { toast(cleanErr(e), "err"); }
  };
  el("cc-up").onclick = () => { state.corpusPath = ccParent(state.corpusPath); loadCorpusDir(); };
  el("cc-mkdir").onclick = () => mkdirFlow(fc);
  el("cc-import").onclick = () => importFlow(fc);
  renderAllowBanner(fc);
  renderCrumbs();
  loadCorpusDir();
}

function renderCrumbs() {
  const box = el("cc-crumbs");
  if (!box) return;
  const parts = state.corpusPath.split("/").filter(Boolean);
  let acc = "";
  let html = `<button class="cc-crumb" data-p="/">racine</button>`;
  for (const seg of parts) {
    acc += "/" + seg;
    html += ` <span class="cc-sep">/</span> <button class="cc-crumb" data-p="${esc(acc)}">${esc(seg)}</button>`;
  }
  box.innerHTML = html;
  box.querySelectorAll(".cc-crumb").forEach((b) => (b.onclick = () => {
    state.corpusPath = b.dataset.p;
    loadCorpusDir();
  }));
}

function renderAllowBanner(fc) {
  const box = el("cc-allow");
  if (!box) return;
  const allow = fc.allowed_dirs || [], deny = fc.denied_dirs || [];
  if (!allow.length) {
    box.innerHTML = `<span class="cc-allow-none">👁 L'IA ne voit encore rien. Cliquez « Accès IA » pour ouvrir l'arbre et cocher ce que l'IA peut voir (ou tout le CORPUS d'un coup).</span>`;
    return;
  }
  const all = allow.some((d) => ccClean(d) === "/");
  let html = `<span class="cc-allow-lbl">👁 L'IA peut voir :</span>`;
  html += all ? `<span class="cc-chip">tout le CORPUS</span>` : allow.map((d) => `<span class="cc-chip">${esc(d)}</span>`).join("");
  if (deny.length) html += `<span class="cc-allow-lbl" style="margin-left:8px">sauf</span>` + deny.map((d) => `<span class="cc-chip deny">${esc(d)}</span>`).join("");
  box.innerHTML = html;
}

async function loadCorpusDir() {
  const box = el("cc-list");
  if (!box) return;
  renderCrumbs();
  box.innerHTML = `<div class="skeleton"></div>`;
  let entries;
  try {
    entries = (await App().CloudList(state.corpusCloud, state.corpusPath)) || [];
  } catch (e) {
    if (isLocked(e)) return handleLock();
    box.innerHTML = `<div class="muted" style="padding:10px">${esc(cleanErr(e))}</div>`;
    return;
  }
  entries.sort((a, b) => (a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : a.is_dir ? -1 : 1));
  const fc = (state.corpusClouds || []).find((c) => c.name === state.corpusCloud) || {};
  const A = new Set((fc.allowed_dirs || []).map(ccClean));
  const D = new Set((fc.denied_dirs || []).map(ccClean));
  if (!entries.length) {
    box.innerHTML = `<div class="muted" style="padding:10px">Dossier vide.</div>`;
    return;
  }
  box.innerHTML = entries
    .map((e, i) => {
      const dot = aiGlyph(cloudVisState(e.path, e.is_dir, A, D)); // picto IA (lecture seule ici)
      return `<div class="cc-row">
        <button class="cc-name${e.is_dir ? " dir" : ""}" ${e.is_dir ? `data-open="${i}"` : `data-dl="${i}"`}>
          ${dot}<span class="cc-ei">${e.is_dir ? "📁" : "📄"}</span>${esc(e.name)}${e.is_dir ? "" : `<span class="cc-size">${fmtBytes(e.size)}</span>`}</button>
        <div class="cc-rowact">${e.is_dir ? "" : `<button class="btn btn-ghost btn-sm" data-dl="${i}" title="Télécharger">📥</button>`}<button class="icon-del cc-del1" data-del="${i}" title="Supprimer" aria-label="Supprimer">${ICO_TRASH}</button></div>
      </div>`;
    })
    .join("");
  box.onclick = (ev) => {
    const o = ev.target.closest("[data-open]");
    if (o) {
      state.corpusPath = entries[+o.dataset.open].path;
      loadCorpusDir();
      return;
    }
    const d = ev.target.closest("[data-dl]");
    if (d) return downloadFlow(entries[+d.dataset.dl]);
    const rm = ev.target.closest("[data-del]");
    if (rm) return deleteFlow(fc, entries[+rm.dataset.del]);
  };
}

// aiGlyph : picto « IA » (robot, style monoline) coloré selon l'accès — vert = complet, orange =
// partiel, gris = aucun. SVG inline (CSP-safe, hors-ligne) ; la couleur suit `currentColor`.
function aiGlyph(stt, title) {
  const t = title || (stt === "on" ? "IA : accès complet (ce dossier et tout son contenu)" : stt === "partial" ? "IA : accès partiel (restreint à certains éléments)" : "IA : aucun accès");
  return `<span class="aiglyph ${stt}" title="${esc(t)}"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 4.6V3"/><circle cx="12" cy="2.3" r="1.05" fill="currentColor" stroke="none"/><rect x="4.4" y="7.4" width="15.2" height="11.2" rx="3.2"/><path d="M2.6 11.6v3.2M21.4 11.6v3.2"/><circle cx="9.2" cy="12.4" r="1.35" fill="currentColor" stroke="none"/><circle cx="14.8" cy="12.4" r="1.35" fill="currentColor" stroke="none"/><path d="M9.3 15.8h5.4"/></svg></span>`;
}

// cloudVisState : état d'accès IA d'un chemin — "on" (complet), "partial" (restreint par des
// exceptions sous un parent autorisé, ou autorisé seulement plus bas) ou "off" (aucun). Partagé par
// l'explorateur ET l'arbre, pour une couleur cohérente partout.
function cloudVisState(p, isDir, A, D) {
  const eff = effAllowed(p, A, D);
  if (isDir) {
    if (eff && aitHasUnder(D, p)) return "partial";
    if (!eff && aitHasUnder(A, p)) return "partial";
  }
  return eff ? "on" : "off";
}

// ccClean normalise un chemin cloud (miroir de config.CloudCleanPath côté Go).
function ccClean(p) {
  p = (p || "/").replace(/\\/g, "/").trim();
  if (!p.startsWith("/")) p = "/" + p;
  p = p.replace(/\/+/g, "/");
  if (p !== "/") p = p.replace(/\/+$/, "");
  return p;
}

// effAllowed : visibilité IA d'un chemin selon autorisations + exclusions (miroir fidèle de
// config.PathAllowed côté Go : préfixe le plus spécifique gagne, exclusion prioritaire à égalité).
function effAllowed(p, A, D) {
  p = ccClean(p);
  let bestLen = -1, bestAllow = false, matched = false;
  const consider = (d, allow) => {
    d = ccClean(d);
    const covers = d === "/" || p === d || p.startsWith(d + "/");
    if (!covers) return;
    const l = d === "/" ? 0 : d.length;
    if (l > bestLen || (l === bestLen && !allow)) { bestLen = l; bestAllow = allow; matched = true; }
  };
  A.forEach((d) => consider(d, true));
  D.forEach((d) => consider(d, false));
  return matched && bestAllow;
}

function fmtBytes(n) {
  n = +n || 0;
  if (n < 1024) return n + " o";
  if (n < 1024 * 1024) return (n / 1024).toFixed(0) + " Ko";
  return (n / 1024 / 1024).toFixed(1) + " Mo";
}

async function downloadFlow(e) {
  try {
    const dest = await App().CloudSaveAs(state.corpusCloud, e.path);
    if (dest) toast("Téléchargé : " + dest, "ok");
  } catch (err) {
    if (isLocked(err)) return handleLock();
    toast(cleanErr(err), "err");
  }
}

function mkdirFlow(fc) {
  openModal("Nouveau dossier", `<div class="field"><label>Nom du dossier</label><input id="cc-nd" class="inp" placeholder="Documents" autocomplete="off" /><div class="inline-error hidden" id="cc-nde"></div></div>`, null);
  el("modal-ok").textContent = "Créer";
  el("modal").querySelector("[data-ok], #modal-ok").onclick = () => {
    const name = el("cc-nd").value.trim();
    if (!name) {
      const e = el("cc-nde");
      e.textContent = "Nom requis.";
      e.classList.remove("hidden");
      return;
    }
    const target = ccJoin(state.corpusPath, name);
    closeModal();
    runCloudWrite(fc, "write", "Créer le dossier « " + target + " »", (pin) => App().CloudMkdir(fc.name, target, pin), () => loadCorpusDir());
  };
}

function importFlow(fc) {
  runCloudWrite(
    fc,
    "write",
    "Importer des fichiers dans « " + state.corpusPath + " »",
    async (pin) => {
      const sent = await App().CloudPickAndUpload(fc.name, state.corpusPath, pin);
      if (sent && sent.length) toast(sent.length + " fichier(s) importé(s)", "ok");
    },
    () => loadCorpusDir()
  );
}

function deleteFlow(fc, e) {
  // Suppression = geste destructif → confirmation + PIN (par défaut).
  pinConfirmOrSession(fc, "delete", `Supprimer « ${e.path} » du cloud ${esc(fc.name)}. Ce geste est définitif.`, (pin) => App().CloudDelete(fc.name, e.path, pin), () => loadCorpusDir());
}

// runCloudWrite : exécute une écriture cloud en respectant la politique PIN de la connexion.
function runCloudWrite(fc, action, reason, fn, done) {
  pinConfirmOrSession(fc, action, reason, fn, done);
}

function pinConfirmOrSession(fc, action, reason, fn, done) {
  if (state.readOnly) {
    toast("Mode lecture seule actif — désactivez-le dans Réglages", "err");
    return;
  }
  if (cloudNeedsPIN(fc, action)) {
    pinConfirm(reason, async (pin) => {
      await fn(pin);
      if (done) done();
    });
    return;
  }
  // Politique autonome : pas de PIN, la session ouverte suffit.
  fn("")
    .then(() => done && done())
    .catch((e) => {
      if (isLocked(e)) return handleLock();
      toast(cleanErr(e), "err");
    });
}

function removeCloudFlow(fc, onDone) {
  confirmDelete({
    title: `Déconnecter le cloud « ${esc(fc.name)} » ?`,
    lead: `La connexion cloud <b>${esc(fc.name)}</b> sera retirée de SituLab.`,
    items: ["La connexion et son mot de passe chiffré", "Les périmètres d'accès IA définis pour ce cloud"],
    safe: ["Vos fichiers sur le cloud ne sont pas touchés", "Les sauvegardes déjà déposées restent en place"],
    confirmLabel: "Déconnecter le cloud",
    onConfirm: async () => {
      await App().RemoveFileCloud(fc.name);
      if (typeof invalidateCloudHealth === "function") invalidateCloudHealth();
      state.corpusCloud = "";
      toast("Cloud déconnecté", "ok");
      (onDone || renderCorpus)();
    },
  });
}

// --- Aides URL pour les liens DYNAMIQUES du tunnel ---
function ccUrlOrigin(raw) {
  try { const u = new URL(/:\/\//.test(raw) ? raw : "https://" + raw); return u.protocol + "//" + u.host; } catch (_) { return ""; }
}
// ccKdriveAccount : extrait l'ID de COMPTE Infomaniak de l'URL de l'app web (distinct de l'ID du
// drive, qui suit « /drive/ »). Ex. ksuite.infomaniak.com/53843/kdrive/app/drive/3869957/ → 53843.
function ccKdriveAccount(raw) {
  try {
    const u = new URL(/:\/\//.test(raw) ? raw : "https://" + raw);
    const segs = u.pathname.split("/").filter(Boolean);
    const di = segs.findIndex((s) => s.toLowerCase() === "drive");
    const drive = di >= 0 && /^\d+$/.test(segs[di + 1] || "") ? segs[di + 1] : "";
    for (const s of segs) { if (/^\d+$/.test(s) && s !== drive) return s; }
    return "";
  } catch (_) { return ""; }
}
// ccAppPwLink : lien DYNAMIQUE vers la page de création d'un mot de passe d'application, construit
// depuis l'URL saisie (compte Infomaniak pour kDrive ; serveur pour Nextcloud).
function ccAppPwLink(kind, rawURL) {
  if (kind === "kdrive") {
    const acc = ccKdriveAccount(rawURL);
    return acc
      ? `https://manager.infomaniak.com/v3/${acc}/ng/profile/user/connection-history/application-password`
      : `https://manager.infomaniak.com/v3/ng/profile/user/connection-history/application-password`;
  }
  const o = ccUrlOrigin(rawURL);
  return o ? o + "/settings/user/security" : "https://docs.nextcloud.com/server/latest/user_manual/fr/session_management.html";
}

/* ---- Tunnel d'ajout d'un cloud : Type → Nom → Adresse (vérifiée) → Jeton (liens dynamiques) → Test.
 * Calqué sur l'onboarding des connexions git. onDone (optionnel) : rendu de retour (ex. Réglages). */
function addCloudModal(onDone) {
  const W = { kind: "", name: "", url: "", endpoint: "" };
  const existing = (state.corpusClouds || []).map((c) => (c.name || "").toLowerCase());
  const STEPS = ["Type", "Nom", "Adresse", "Jeton"];
  const head = (n) => `<div class="cc-steps">${STEPS.map((s, i) => `<span class="cc-stp${i === n ? " on" : i < n ? " done" : ""}">${i + 1}. ${s}</span>`).join("")}</div>`;
  const step = (n, html) => {
    openModal("Connecter un cloud", head(n) + html, null);
    const ok = el("modal-ok");
    if (ok) { ok.disabled = false; ok.classList.remove("hidden"); } // état propre à chaque étape
  };
  const back = (fn) => { const b = el("cc-back"); if (b) b.onclick = fn; };

  function stepType() {
    step(0, `<div class="field"><label>Type de cloud souverain</label>
      <div class="conn-cards">
        <button type="button" class="conn-card" data-k="nextcloud"><span class="conn-logo">${cloudLogo("nextcloud")}</span><b>Nextcloud</b><span class="sub">auto-hébergé, souverain autonome</span></button>
        <button type="button" class="conn-card" data-k="kdrive"><span class="conn-logo">${cloudLogo("kdrive")}</span><b>kDrive</b><span class="sub">Infomaniak, souverain (droit suisse)</span></button>
      </div>
      <p class="sub">Seuls les clouds souverains sont proposés. OneDrive, Google Drive et Dropbox sont écartés (données captives).</p></div>`);
    el("modal-ok").classList.add("hidden");
    el("modal").querySelectorAll(".conn-card").forEach((c) => (c.onclick = () => { W.kind = c.dataset.k; el("modal-ok").classList.remove("hidden"); stepName(); }));
  }

  function stepName() {
    step(1, `<div class="field">
      <p class="sub">Vous pouvez connecter <b>plusieurs</b> clouds (plusieurs Nextcloud et/ou kDrive).</p>
      <label>Nom de la connexion</label>
      <input id="cc-n" class="inp" value="${esc(W.name)}" placeholder="ex. kDrive apcom, Nextcloud perso" autocomplete="off" />
      <p class="sub">Un nom parlant, <b>différent</b> des connexions existantes.</p>
      <div class="inline-error hidden" id="cc-ne"></div>
      <button type="button" class="btn btn-ghost btn-sm" id="cc-back" style="margin-top:10px">‹ Type</button></div>`);
    el("modal-ok").textContent = "Suivant";
    back(stepType);
    el("modal").querySelector("#modal-ok").onclick = () => {
      W.name = el("cc-n").value.trim();
      const er = el("cc-ne");
      if (!W.name) { er.textContent = "Donnez un nom à cette connexion."; er.classList.remove("hidden"); return; }
      if (existing.includes(W.name.toLowerCase())) { er.textContent = "Ce nom est déjà pris — choisissez-en un autre."; er.classList.remove("hidden"); return; }
      stepURL();
    };
  }

  function stepURL() {
    const kd = W.kind === "kdrive";
    const urlLabel = kd ? "Adresse de votre kDrive" : "Adresse de votre serveur Nextcloud";
    const urlPh = kd ? "https://ksuite.infomaniak.com/…/drive/3869957/" : "https://cloud.exemple.ch";
    const urlHint = kd
      ? `Collez l'adresse depuis votre navigateur (ex. <code>…/drive/3869957/</code>) ou juste l'ID — SituLab en déduit l'endpoint WebDAV. <a class="lnk" data-ext="https://www.infomaniak.com/fr/support/faq/2409/se-connecter-a-kdrive-par-webdav">Où trouver mon adresse kDrive ↗</a>`
      : `Collez l'adresse de votre Nextcloud — SituLab complète le chemin technique (<code>/remote.php/dav/…</code>). <a class="lnk" data-ext="https://docs.nextcloud.com/server/latest/user_manual/fr/files/access_webdav.html">Où trouver mon adresse Nextcloud ↗</a>`;
    step(2, `<div class="field cc-connform">
      <label>${urlLabel}</label><input id="cc-u" class="inp" value="${esc(W.url)}" placeholder="${urlPh}" autocomplete="off" />
      <p class="sub">${urlHint}</p>
      <div class="inline-error hidden" id="cc-ue"></div>
      <button type="button" class="btn btn-ghost btn-sm" id="cc-back" style="margin-top:10px">‹ Nom</button></div>`);
    el("modal-ok").textContent = "Vérifier l'adresse";
    back(stepName);
    el("modal").querySelector("#modal-ok").onclick = async () => {
      const url = el("cc-u").value.trim(), er = el("cc-ue");
      if (!url) { er.textContent = "Indiquez l'adresse de votre cloud."; er.classList.remove("hidden"); return; }
      el("modal-ok").disabled = true; el("modal-ok").textContent = "Vérification…";
      try {
        W.endpoint = await App().CloudResolveURL(W.kind, url, "");
        W.url = url;
        stepToken();
      } catch (e) {
        er.innerHTML = esc(cleanErr(e));
        er.classList.remove("hidden");
        el("modal-ok").disabled = false; el("modal-ok").textContent = "Vérifier l'adresse";
      }
    };
  }

  function stepToken() {
    const kd = W.kind === "kdrive";
    const pwLink = ccAppPwLink(W.kind, W.url);
    const appPwHint = kd
      ? `⚠️ N'utilisez <b>pas</b> votre mot de passe habituel : créez un <b>mot de passe d'application</b>. <a class="lnk" data-ext="${esc(pwLink)}">Ouvrir ma page de mots de passe d'application ↗</a> → <i>Générer</i>, nommez-le « SituLab », <b>copiez-le</b> (affiché une seule fois). L'identifiant reste votre <b>e-mail Infomaniak</b>.`
      : `Recommandé : un <b>mot de passe d'application</b>. <a class="lnk" data-ext="${esc(pwLink)}">Ouvrir la sécurité de mon Nextcloud ↗</a> → bas de page « Appareils &amp; sessions » → nommez l'app → <i>Créer un nouveau mot de passe d'application</i>, puis copiez-le.`;
    step(3, `<div class="field cc-connform">
      <p class="sub">✓ Adresse vérifiée : <code>${esc(W.endpoint)}</code></p>
      <label>Identifiant</label><input id="cc-us" class="inp" placeholder="${kd ? "votre e-mail Infomaniak" : "votre identifiant Nextcloud"}" autocomplete="off" />
      <label>Jeton / mot de passe d'application</label><input id="cc-pw" class="inp" type="password" autocomplete="off" />
      <p class="sub">${appPwHint}</p>
      <div class="inline-error hidden" id="cc-e"></div>
      <button type="button" class="btn btn-ghost btn-sm" id="cc-back" style="margin-top:10px">‹ Adresse</button></div>`);
    el("modal-ok").textContent = "Connecter";
    back(stepURL);
    el("modal").querySelector("#modal-ok").onclick = async () => {
      const user = el("cc-us").value.trim(), pw = el("cc-pw").value, e = el("cc-e");
      if (!user || !pw) { e.textContent = "Identifiant et jeton sont requis."; e.classList.remove("hidden"); return; }
      el("modal-ok").disabled = true; el("modal-ok").textContent = "Connexion…";
      try {
        await App().AddFileCloud(W.name, W.kind, W.url, user, pw);
        if (typeof invalidateCloudHealth === "function") invalidateCloudHealth();
        closeModal();
        toast("Cloud « " + W.name + " » connecté", "ok");
        state.corpusCloud = W.name;
        state.corpusPath = "/";
        (typeof onDone === "function" ? onDone : renderCorpus)();
      } catch (err) {
        const msg = cleanErr(err);
        const auth = /refus|identifiant|mot de passe|401|403/i.test(msg);
        e.innerHTML = esc(msg) + (auth ? `<div style="margin-top:8px">💡 ${appPwHint}</div>` : "");
        e.classList.remove("hidden");
        el("modal-ok").disabled = false; el("modal-ok").textContent = "Connecter";
      }
    };
  }

  stepType();
}

/* ---- Politique PIN d'une connexion (FENÊTRE uniquement) ----
 * onDone (optionnel) : rendu de retour après enregistrement (ex. renderSettings). */
function cloudPINModal(fc, onDone) {
  const wv = fc.pin_policy && fc.pin_policy.write === true ? "pin" : "auto";
  const dv = !fc.pin_policy || fc.pin_policy.delete === undefined || fc.pin_policy.delete === null ? "pin" : fc.pin_policy.delete ? "pin" : "auto";
  openModal("PIN — " + fc.name, `<div class="field">
    <label>Envoi / modification / création</label>
    <div class="seg sm" id="cp-w"><button data-v="auto"${wv === "auto" ? ' class="on"' : ""}>Sans PIN</button><button data-v="pin"${wv === "pin" ? ' class="on"' : ""}>PIN requis</button></div>
    <label>Suppression</label>
    <div class="seg sm" id="cp-d"><button data-v="auto"${dv === "auto" ? ' class="on"' : ""}>Sans PIN</button><button data-v="pin"${dv === "pin" ? ' class="on"' : ""}>PIN requis</button></div>
    <p class="sub">Qui peut écrire/supprimer sans votre PIN ? Par défaut : écriture autonome, suppression sous PIN. (L'accès de l'IA par dossier se règle via « Accès IA ».)</p>
  </div>`, async () => {
    const wsel = el("cp-w").querySelector(".on")?.dataset.v || "auto";
    const dsel = el("cp-d").querySelector(".on")?.dataset.v || "pin";
    try {
      await App().SetCloudPINPolicy(fc.name, { write: wsel === "pin", delete: dsel === "pin" });
      closeModal();
      toast("Politique PIN enregistrée", "ok");
      (typeof onDone === "function" ? onDone : renderCorpus)();
    } catch (e) {
      toast(cleanErr(e), "err");
    }
  });
  el("modal-ok").textContent = "Enregistrer";
  ["cp-w", "cp-d"].forEach((id) => {
    const seg = el(id);
    seg.querySelectorAll("button").forEach((b) => (b.onclick = () => seg.querySelectorAll("button").forEach((x) => x.classList.toggle("on", x === b))));
  });
}

/* ---- Accès de l'IA par ARBRE + pastilles (FENÊTRE uniquement, v1.11) ----
 * L'humain coche ce que l'IA peut voir. Cocher un dossier ouvre tout son contenu ; on peut ensuite
 * décocher finement un enfant (exclusion). Un bouton ouvre/ferme TOUT le CORPUS d'un coup.
 * Modèle : allow + deny, préfixe le plus spécifique gagne (miroir exact du Go). */
let AIT = null;
function aiAccessTreeModal(fc, onDone) {
  AIT = {
    name: fc.name,
    A: new Set((fc.allowed_dirs || []).map(ccClean)),
    D: new Set((fc.denied_dirs || []).map(ccClean)),
    open: new Set(),
    children: {}, // path -> entries
    onDone: onDone,
  };
  openModal("Accès de l'IA — " + esc(fc.name), `
    <div class="ait">
      <button type="button" class="ait-all" id="ait-all"></button>
      <p class="ait-help">🟢 visible par l'IA · ⚪ masqué. Cochez un dossier pour ouvrir tout son contenu, puis décochez finement ce que vous voulez retirer.</p>
      <div class="ait-tree" id="ait-tree"><div class="skeleton"></div></div>
    </div>`, async () => {
    try {
      await App().SetCloudAccess(AIT.name, [...AIT.A], [...AIT.D]);
      closeModal();
      toast("Accès de l'IA enregistré", "ok");
      (typeof onDone === "function" ? onDone : renderCorpus)();
    } catch (e) {
      toast(cleanErr(e), "err");
    }
  });
  el("modal-ok").textContent = "Enregistrer";
  aitLoad("/").then(aitRender);
}

async function aitLoad(dir) {
  dir = ccClean(dir);
  if (AIT.children[dir]) return;
  try {
    const es = (await App().CloudList(AIT.name, dir)) || [];
    es.sort((a, b) => (a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : a.is_dir ? -1 : 1));
    AIT.children[dir] = es;
  } catch (e) {
    AIT.children[dir] = [];
    toast(cleanErr(e), "err");
  }
}

// aitState : état d'un chemin pour la pastille/case — "on" (visible), "off" (masqué), "partial"
// (dossier globalement visible/masqué mais contredit plus bas).
function aitState(p, isDir) {
  return cloudVisState(p, isDir, AIT.A, AIT.D);
}
function aitHasUnder(set, p) {
  for (const d of set) {
    if (d === p) continue;
    if (p === "/" ? d !== "/" : d.startsWith(p + "/")) return true;
  }
  return false;
}
function aitPurgeUnder(set, p) {
  for (const d of [...set]) {
    if (p === "/" || d === p || d.startsWith(p + "/")) set.delete(d);
  }
}
// aitToggle : bascule la visibilité effective d'un chemin (dossier → tout son sous-arbre).
function aitToggle(p, isDir) {
  p = ccClean(p);
  const want = aitState(p, isDir) !== "on"; // off/partial → tout visible ; on → masquer
  const parentEff = p === "/" ? false : effAllowed(ccParent(p), AIT.A, AIT.D);
  aitPurgeUnder(AIT.A, p);
  aitPurgeUnder(AIT.D, p);
  if (p === "/") {
    if (want) AIT.A.add("/");
    return;
  }
  if (want) { if (!parentEff) AIT.A.add(p); }
  else { if (parentEff) AIT.D.add(p); }
}

function aitRender() {
  const allSt = aitState("/", true);
  const allBtn = el("ait-all");
  if (allBtn) {
    allBtn.className = "ait-all " + (allSt === "on" ? "on" : allSt === "partial" ? "partial" : "");
    allBtn.innerHTML = `<b>${aiGlyph(allSt)} Tout le CORPUS</b><span class="sub">${allSt === "on" ? "entièrement visible par l'IA" : allSt === "partial" ? "visible, avec des exceptions" : "cliquer pour tout ouvrir à l'IA"}</span>`;
    allBtn.onclick = () => { aitToggle("/", true); aitRender(); };
  }
  const box = el("ait-tree");
  if (box) box.innerHTML = aitRows("/", 0) || `<div class="muted" style="padding:8px">Dossier vide.</div>`;
}

function aitRows(dir, depth) {
  const es = AIT.children[dir];
  if (!es) return `<div class="ait-loading" style="padding-left:${12 + depth * 18}px">…</div>`;
  let html = "";
  for (let i = 0; i < es.length; i++) {
    const e = es[i], st = aitState(e.path, e.is_dir), open = AIT.open.has(ccClean(e.path));
    html += `<div class="ait-row" style="padding-left:${12 + depth * 18}px">
      <button type="button" class="ait-dotbtn" data-chk="${esc(e.path)}" data-dir="${e.is_dir ? 1 : 0}" title="${st === "on" ? "Accès complet — cliquer pour masquer" : st === "partial" ? "Accès partiel — cliquer pour tout ouvrir" : "Aucun accès — cliquer pour ouvrir à l'IA"}">${aiGlyph(st)}</button>
      ${e.is_dir ? `<button type="button" class="ait-exp" data-exp="${esc(e.path)}">${open ? "▾" : "▸"}</button>` : `<span class="ait-expsp"></span>`}
      <span class="ait-ic">${e.is_dir ? "📁" : "📄"}</span><span class="ait-nm">${esc(e.name)}</span>
    </div>`;
    if (e.is_dir && open) html += aitRows(ccClean(e.path), depth + 1);
  }
  return html;
}

// Délégation des clics de l'arbre (une seule fois).
document.addEventListener("click", async (ev) => {
  if (!AIT) return;
  const chk = ev.target.closest && ev.target.closest("[data-chk]");
  if (chk && document.getElementById("ait-tree")) {
    aitToggle(chk.dataset.chk, chk.dataset.dir === "1");
    aitRender();
    return;
  }
  const exp = ev.target.closest && ev.target.closest("[data-exp]");
  if (exp && document.getElementById("ait-tree")) {
    const p = ccClean(exp.dataset.exp);
    if (AIT.open.has(p)) { AIT.open.delete(p); aitRender(); }
    else { AIT.open.add(p); await aitLoad(p); aitRender(); }
  }
});

