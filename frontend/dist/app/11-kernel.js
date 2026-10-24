/* ---------------- KERNEL : la mémoire, lisible (1.11) ----------------
 * Le KERNEL est le dépôt qui EST votre mémoire — sorti du bruit des dépôts logiciels et présenté
 * pour ce qu'il est : le centre. L'humain désigne quel dépôt fait office de mémoire (plusieurs
 * possibles, un actif), voit son état en langage humain, récupère/enregistre ses notes sans jargon
 * git, et visualise ce que le KERNEL englobe. La désignation est un geste de fenêtre (gouvernance).
 */

function klIcon() {
  return `<svg viewBox="0 0 24 24" width="26" height="26" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 3 7.5v9L12 21l9-4.5v-9L12 3Z"/><path d="M12 12 3 7.5M12 12l9-4.5M12 12v9"/></svg>`;
}

// klTone : couleur de la pastille d'état (vert à jour, bleu notes à récupérer, orange à enregistrer).
function klTone(st) {
  if (!st || !st.is_repo) return "red";
  if (st.dirty_files > 0 || st.ahead_notes > 0) return "orange";
  if (st.behind_notes > 0) return "blue";
  return "green";
}

// renderKernel : la page KERNEL liste vos dépôts désignés comme mémoire, en CARTES CLASSEUR (même
// langage visuel que Dépôts git). Un clic ouvre le DÉTAIL du dépôt. Désigner = choisir un dépôt appairé.
async function renderKernel() {
  const view = el("view");
  view.innerHTML = `<div class="view-head view-head-row">
      <div><div class="view-title">KERNEL</div><div class="view-sub">Le(s) dépôt(s) qui portent votre mémoire. Un clic ouvre le détail.</div></div>
      <button class="btn btn-primary btn-sm" id="kl-add">＋ Désigner un KERNEL</button></div>
    <div id="kl-home"><div class="skeleton"></div><div class="skeleton" style="margin-top:10px"></div></div>`;
  el("kl-add").onclick = pickKernelRepo;
  let rows = [];
  try { rows = (await App().ProjectsLight()) || []; }
  catch (e) { if (isLocked(e)) return handleLock(); const b = el("kl-home"); if (b) b.innerHTML = surfaceError("Lecture impossible", cleanErr(e), null); return; }
  const kernels = rows.filter((p) => p.kernel);
  const home = el("kl-home"); if (!home) return;
  if (!kernels.length) {
    home.innerHTML =
      blankslate("🧠", "Aucun KERNEL désigné", "Le KERNEL est le dépôt qui porte votre mémoire. Désignez-en un parmi vos dépôts appairés — SituLab le marquera dans Dépôts git.") +
      `<div class="kl-center"><button class="btn btn-primary" id="kl-add2">＋ Désigner un KERNEL</button></div>`;
    if (el("kl-add2")) el("kl-add2").onclick = pickKernelRepo;
    return;
  }
  const runOf = (p) => p.kernel_run || "";
  home.innerHTML = `<div class="classeur-grid">${kernels.map((p, i) => {
    const run = runOf(p);
    const runLink = run
      ? `<span class="run-pill" title="Fichier d'ouverture pour l'IA">📄 ${esc(run)}</span><button type="button" class="lnk" data-run="${i}">modifier</button>`
      : `<button type="button" class="lnk run-set" data-run="${i}">📄 Définir le fichier d'ouverture</button>`;
    return `<div class="desig-wrap">${classeurCardHTML(p)}
      <div class="desig-meta">${runLink}<button type="button" class="icon-del" data-remove="${esc(p.name)}" title="Retirer ce KERNEL" aria-label="Retirer ce KERNEL « ${esc(p.name)} »">${ICO_TRASH}</button></div>
    </div>`;
  }).join("")}</div>`;
  home.onclick = (ev) => {
    const rm = ev.target.closest("[data-remove]");
    if (rm) { ev.stopPropagation(); return removeKernelFlow(rm.dataset.remove); }
    const rf = ev.target.closest("[data-run]");
    if (rf) {
      ev.stopPropagation();
      const p = kernels[+rf.dataset.run]; if (!p || !p.local) return;
      const path = p.local.path;
      return runFileModal(path, runOf(p), async (f) => {
        try { await App().SetKernelRunFile(path, f); closeModal(); toast(f ? "Fichier d'ouverture défini" : "Fichier d'ouverture retiré", "ok"); renderKernel(); }
        catch (e) { if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); }
      }, "Fichier d'ouverture — KERNEL");
    }
    const c = ev.target.closest("[data-open-project]"); if (c) openProject(c.dataset.openProject);
  };
}

// removeKernelFlow : retire la DÉSIGNATION KERNEL (le dépôt n'est pas touché).
function removeKernelFlow(name, onDone) {
  confirmDelete({
    title: `Retirer le KERNEL « ${esc(name)} » ?`,
    lead: `« ${esc(name)} » ne sera plus <b>marqué comme KERNEL</b> dans SituLab.`,
    items: ["La désignation KERNEL (marquage + fichier d'ouverture associé)"],
    safe: ["Le dépôt n'est pas supprimé ni dé-cloné — il reste dans vos dépôts appairés", "Le contenu sur le disque et le serveur n'est pas touché"],
    confirmLabel: "Retirer ce KERNEL",
    onConfirm: async () => {
      try { await App().RemoveKernel(name); } catch (e) { if (isLocked(e)) { handleLock(); return; } throw e; }
      toast("KERNEL retiré", "ok");
      (onDone || renderKernel)();
    },
  });
}

async function loadKernelStatus() {
  const box = el("kl-status");
  if (!box) return;
  let st;
  try {
    st = await App().KernelStatus(state.kernelName);
  } catch (e) {
    if (isLocked(e)) return handleLock();
    box.innerHTML = `<div class="kl-card red"><div class="kl-dot"></div><div class="kl-sum">${esc(cleanErr(e))}</div></div>`;
    return;
  }
  state.kernelStatus = st;
  const tone = klTone(st);
  const when = st.last_note ? "Dernière note : " + esc(fmtWhen(st.last_note)) : "";
  box.innerHTML = `<div class="kl-card ${tone}">
    <div class="kl-dot"></div>
    <div class="kl-info">
      <div class="kl-sum">${esc(st.summary || "")}</div>
      <div class="kl-meta">${esc(st.branch || "")}${when ? " · " + when : ""}</div>
    </div>
  </div>`;
}

async function loadKernelContents() {
  const box = el("kl-contents");
  if (!box) return;
  let es;
  try {
    es = (await App().KernelContents(state.kernelName)) || [];
  } catch (e) {
    box.innerHTML = `<div class="muted" style="padding:8px">${esc(cleanErr(e))}</div>`;
    return;
  }
  if (!es.length) {
    box.innerHTML = `<div class="muted" style="padding:8px">Le KERNEL est vide.</div>`;
    return;
  }
  const ico = (kind) => (kind === "dépôt" ? "📦" : kind === "dossier" ? "📁" : "📄");
  box.innerHTML = es.map((e) => `<div class="kl-item" title="${esc(e.kind)}"><span class="kl-it-ico">${ico(e.kind)}</span><span class="kl-it-n">${esc(e.name)}</span><span class="kl-it-k">${esc(e.kind)}</span></div>`).join("");
}

function kernelPullFlow() {
  const btn = el("kl-pull");
  if (btn) btn.disabled = true;
  App()
    .KernelPull(state.kernelName)
    .then(() => {
      toast("Notes récupérées", "ok");
      loadKernelStatus();
      loadKernelContents();
    })
    .catch((e) => {
      if (isLocked(e)) return handleLock();
      toast(cleanErr(e), "err");
    })
    .finally(() => { if (btn) btn.disabled = false; });
}

function kernelSaveFlow() {
  if (state.readOnly) {
    toast("Mode lecture seule actif — désactivez-le dans Réglages", "err");
    return;
  }
  // Message pré-rempli d'un horodatage lisible, que l'utilisateur peut écraser.
  const stamp = new Date().toLocaleString("fr-CH", { timeZone: "Europe/Zurich", dateStyle: "short", timeStyle: "short" });
  openModal(
    "Enregistrer mes notes",
    `<div class="field"><label>Libellé de l'enregistrement</label>
     <input id="kl-msg" class="inp" value="Notes du ${esc(stamp)}" autocomplete="off" />
     <p class="sub">Un court intitulé pour vous y retrouver plus tard. L'envoi en ligne demandera votre PIN.</p></div>`,
    () => {
      const msg = el("kl-msg").value.trim();
      closeModal();
      pinConfirm("Enregistrer et envoyer vos notes du KERNEL", async (pin) => {
        const res = await App().KernelSave(state.kernelName, msg, pin);
        toast(res && res.summary ? res.summary : "Notes enregistrées", "ok");
        loadKernelStatus();
        loadKernelContents();
      });
    }
  );
  el("modal-ok").textContent = "Continuer";
}

/* ---- Sauvegarde CORPUS rattachée au KERNEL ---- */
async function renderKernelBackup() {
  const box = el("kl-backup");
  if (!box) return;
  const k = (state.kernels || []).find((x) => x.name === state.kernelName) || {};
  let plans = [];
  try {
    plans = (await App().Backups()) || [];
  } catch (e) { /* non bloquant */ }
  const linked = k.backup ? plans.find((b) => b.name === k.backup) : null;
  if (linked) {
    box.innerHTML = `<div class="kl-bk-row">
      <div class="kl-bk-main"><b>${esc(linked.name)}</b><span class="sub">→ ${esc(linked.cloud)}:${esc(linked.dest || "/")}</span>
      <span class="kl-bk-when">${linked.last_run ? "dernière : " + esc(fmtWhen(linked.last_run)) : "jamais exécutée"}</span></div>
      <button class="btn btn-primary btn-sm" id="kl-bk-run">Sauvegarder maintenant</button></div>`;
    el("kl-bk-run").onclick = async () => {
      // Sauvegarde = routine additive, sans PIN (elle se fait). Rétro-compat RunBackup pour un plan ciblé.
      if (state.readOnly) { toast("Mode lecture seule actif — désactivez-le pour sauvegarder", "err"); return; }
      toast("Sauvegarde en cours…", "info");
      try {
        const out = await App().RunBackupPlan(linked.name).catch(async (e) => {
          if (/introuvable|type/i.test(cleanErr(e))) return App().RunBackup(linked.name);
          throw e;
        });
        toast("✓ " + (out || "Sauvegarde terminée"), /échec/i.test(String(out || "")) ? "info" : "ok");
      } catch (e) { toast("Échec de la sauvegarde : " + cleanErr(e), "err"); }
      renderKernelBackup();
    };
    return;
  }
  box.innerHTML = `<div class="muted" style="padding:6px 0">Aucune sauvegarde rattachée. Créez un plan dans le menu <b>BACKUP</b> (dépôt = ce KERNEL), puis rattachez-le ici via « Gérer ».</div>`;
}

// kpickCardHTML : une carte « classeur » (dépôt appairé) sélectionnable pour désigner le KERNEL —
// même langage visuel que la page Dépôts git (picto, nom, vignettes local·atelier·vitrine). Réutilise
// iconHTML / classeurVerdict / chealthHTML (07-projects.js, chargé avant).
function kpickCardHTML(p, i) {
  const atl = p.atelier || {};
  const v = classeurVerdict(p);
  const hasLocal = !!(p.local && p.local.path);
  return `<div class="classeur kpick${hasLocal ? "" : " kpick-off"}" data-i="${i}" role="button" tabindex="0" aria-label="${esc(p.name)}">
    <span class="classeur-tab">${iconHTML(p.icon, "classeur-ico")}<span class="classeur-name">${esc(p.name)}</span></span>
    <span class="classeur-body">
      <span class="classeur-repo"><span class="classeur-k">atelier</span> <span class="org">${esc(atl.owner || "")}/</span>${esc(atl.repo || "")}</span>
      <span class="classeur-foot">${chealthHTML(p)}<span class="classeur-state" data-tone="${hasLocal ? v.tone : "muted"}">${hasLocal ? esc(v.label) : "non cloné ici"}</span></span>
    </span>
  </div>`;
}

// afterKernelDesignated : juste après la désignation, on PROPOSE de choisir le fichier d'ouverture —
// sinon l'utilisateur repart sans comprendre pourquoi l'étape de mise en route reste incomplète.
function afterKernelDesignated(path, name) {
  renderKernel();
  runFileModal(path, "", async (f) => {
    try {
      await App().SetKernelRunFile(path, f);
      closeModal();
      toast(f ? "Fichier d'ouverture défini" : "Vous pourrez définir le fichier d'ouverture plus tard", f ? "ok" : "info");
      renderKernel();
      if (typeof refreshSetup === "function") refreshSetup();
    } catch (e) { if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); }
  }, "Fichier d'ouverture — " + (name || "KERNEL"));
}

// pickKernelRepo : désigner un KERNEL = CHOISIR l'un de ses DÉPÔTS APPAIRÉS (classeurs). On liste les
// projets ; le choix d'un classeur cloné sur ce poste pré-remplit nom + chemin local + connexion.
async function pickKernelRepo() {
  let rows = [];
  try { rows = (await App().ProjectsLight()) || []; }
  catch (e) { if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); return; }
  if (!rows.length) {
    openModal("Désigner un KERNEL", `<div class="field"><p class="sub">Aucun dépôt appairé pour l'instant. Appairez d'abord vos dépôts dans <b>Dépôts git</b>, puis revenez désigner votre KERNEL.</p></div>`, () => { closeModal(); navTo("projects"); });
    el("modal-ok").textContent = "Aller à Dépôts git";
    return;
  }
  openModal("Désigner un KERNEL", `<div class="field">
    <p class="sub">Votre KERNEL est l'un de vos <b>dépôts appairés</b>. Choisissez celui qui porte votre mémoire.</p>
    <div class="kpick-grid">${rows.map(kpickCardHTML).join("")}</div></div>`, null);
  el("modal-ok").classList.add("hidden");
  el("modal-body").querySelectorAll(".kpick").forEach((c) => {
    const pick = async () => {
      const p = rows[+c.dataset.i];
      if (!p) return;
      if (!p.local || !p.local.path) { toast("Ce dépôt n'est pas cloné sur ce poste — clonez-le d'abord", "err"); return; }
      // Tout est déjà connu depuis l'appairage : on désigne DIRECT (nom, chemin, connexion), sans formulaire.
      try {
        await App().AddKernel(p.name, p.local.path, (p.atelier || {}).instance || "", "");
        closeModal();
        toast("KERNEL « " + p.name + " » désigné", "ok");
        afterKernelDesignated(p.local.path, p.name);
      } catch (e) {
        if (isLocked(e)) return handleLock();
        toast(cleanErr(e), "err");
      }
    };
    c.onclick = pick;
    c.onkeydown = (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); pick(); } };
  });
}

/* ---- Désignation (FENÊTRE) : choisir le dépôt-mémoire ---- */
// prefill (facultatif) : {name,path,instance} — pré-remplit le formulaire, p. ex. au retour du
// sélecteur de dépôts existants (openRepoPicker).
function designateKernelModal(prefill) {
  prefill = prefill || {};
  const insts = state.instances || [];
  openModal(
    "Désigner un KERNEL",
    `<div class="field">
      <label>Nom (pour vous)</label><input id="kl-n" class="inp" placeholder="Ma mémoire APCOM" autocomplete="off" value="${esc(prefill.name || "")}" />
      <label>Dépôt local qui porte la mémoire</label>
      <div class="kl-pickrow"><input id="kl-p" class="inp" placeholder="C:\\tmp\\KERNEL" autocomplete="off" value="${esc(prefill.path || "")}" /><button class="btn btn-ghost btn-sm" type="button" id="kl-scan" title="Choisir parmi les dépôts déjà sur cette machine">🔎</button><button class="btn btn-ghost btn-sm" type="button" id="kl-pick" title="Parcourir un dossier">📁</button></div>
      <p class="sub">🔎 propose les dépôts déjà présents sur votre machine · 📁 choisit un dossier à la main.</p>
      <label>Connexion git (pour récupérer/enregistrer)</label>
      <select id="kl-i" class="select"><option value="">— par défaut —</option>${insts.map((i) => `<option value="${esc(i.name)}"${prefill.instance === i.name ? " selected" : ""}>${esc(i.name)}</option>`).join("")}</select>
      <div class="inline-error hidden" id="kl-e"></div></div>`,
    async () => {
      const name = el("kl-n").value.trim(), path = el("kl-p").value.trim(), inst = el("kl-i").value;
      const err = el("kl-e");
      if (!name || !path) {
        err.textContent = "Nom et dépôt local sont requis.";
        err.classList.remove("hidden");
        return;
      }
      try {
        await App().AddKernel(name, path, inst, "");
        closeModal();
        toast("KERNEL « " + name + " » désigné", "ok");
        state.kernelName = name;
        afterKernelDesignated(path, name);
      } catch (e) {
        err.textContent = cleanErr(e);
        err.classList.remove("hidden");
      }
    }
  );
  el("modal-ok").textContent = "Désigner";
  el("kl-pick").onclick = async () => {
    try { const p = await App().PickFolder(); if (p) el("kl-p").value = p; } catch (_) {}
  };
  el("kl-scan").onclick = () => {
    const cur = { name: el("kl-n").value.trim(), instance: el("kl-i").value };
    openRepoPicker((r) => designateKernelModal({ name: cur.name || r.name, path: r.path, instance: cur.instance }));
  };
}

/* ---- Gérer un KERNEL : actif, sauvegarde rattachée, retrait (FENÊTRE) ---- */
function manageKernelModal(k) {
  if (!k) return;
  const kernels = state.kernels || [];
  let plans = [];
  App().Backups().then((p) => { plans = p || []; fill(); }).catch(() => fill());
  openModal("Gérer « " + esc(k.name) + " »", `<div class="field" id="kl-mg"></div>`, null);
  function fill() {
    const body = el("kl-mg");
    if (!body) return;
    body.innerHTML = `
      ${k.active ? `<p class="sub">✅ C'est votre KERNEL actif.</p>` : `<button class="btn btn-ghost btn-sm" id="kl-setact" style="margin-bottom:10px">Définir comme actif</button>`}
      <label>Plan de sauvegarde rattaché</label>
      <select id="kl-bk-sel" class="select"><option value="">— aucun —</option>${plans.map((p) => `<option value="${esc(p.name)}"${p.name === k.backup ? " selected" : ""}>${esc(p.name)} (${esc(p.cloud)})</option>`).join("")}</select>
      <p class="sub">${plans.length ? "Rattachez un plan existant pour sauvegarder ce KERNEL d'un bouton." : "Aucun plan pour l'instant — créez-en un dans le menu BACKUP."}</p>
      <div class="kl-mg-danger"><button class="btn btn-ghost btn-sm" id="kl-remove">Retirer ce KERNEL de la liste</button><span class="sub">(le dépôt sur le disque n'est pas touché)</span></div>
      <div class="inline-error hidden" id="kl-mge"></div>`;
    const sa = el("kl-setact");
    if (sa) sa.onclick = () => doKernel(() => App().SetActiveKernel(k.name), "KERNEL actif : " + k.name);
    el("kl-remove").onclick = () => removeKernelFlow(k.name);
  }
  // Enregistrer le rattachement au clic sur Valider.
  el("modal-ok").textContent = "Enregistrer";
  el("modal").querySelector("#modal-ok").onclick = async () => {
    const sel = el("kl-bk-sel");
    const backup = sel ? sel.value : "";
    try {
      // Rattachement = ré-enregistrement du KERNEL (AddKernel met à jour l'existant).
      await App().AddKernel(k.name, k.local_path, k.instance || "", backup);
      closeModal();
      toast("Réglages enregistrés", "ok");
      renderKernel();
    } catch (e) {
      const err = el("kl-mge");
      if (err) { err.textContent = cleanErr(e); err.classList.remove("hidden"); }
    }
  };
  async function doKernel(fn, msg, close) {
    try {
      await fn();
      closeModal();
      toast(msg, "ok");
      if (close) state.kernelName = "";
      renderKernel();
    } catch (e) {
      const err = el("kl-mge");
      if (err) { err.textContent = cleanErr(e); err.classList.remove("hidden"); }
    }
  }
}
