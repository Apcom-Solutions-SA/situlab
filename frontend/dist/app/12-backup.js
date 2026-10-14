/* ---------------- BACKUP : sauvegardes git → cloud CORPUS (1.11) ----------------
 * Menu dédié de premier niveau. Un plan dit : QUEL dépôt git (ex. votre KERNEL) est sauvegardé,
 * dans QUEL cloud CORPUS, et dans QUEL dossier. La sauvegarde est un bundle git de tout l'historique
 * (un fichier restaurable par `git clone`). Exécution manuelle (planification auto = étape suivante).
 */

async function renderBackup() {
  const view = el("view");
  view.innerHTML = `
    <div class="view-head"><div class="view-title">BACKUP</div>
      <button class="btn btn-primary btn-sm" id="bk-new">＋ Nouveau plan</button></div>
    <p class="cc-intro">Sauvegardez un dépôt git (par exemple votre <b>KERNEL</b>) dans un dossier d'un cloud <b>CORPUS</b>. Chaque sauvegarde est un <b>bundle</b> de tout l'historique — restaurable par <code>git clone</code>.</p>
    <div id="bk-list"><div class="skeleton"></div></div>`;
  el("bk-new").onclick = backupModal;
  // On a besoin des clouds CORPUS pour la destination.
  try { state.corpusClouds = (await App().FileClouds()) || []; } catch (e) { if (isLocked(e)) return handleLock(); }
  let plans = [];
  try { plans = (await App().Backups()) || []; } catch (e) {
    if (isLocked(e)) return handleLock();
    el("bk-list").innerHTML = blankslate("⚠", "Sauvegardes indisponibles", cleanErr(e));
    return;
  }
  const box = el("bk-list");
  if (!plans.length) {
    box.innerHTML = blankslate("🛟", "Aucun plan de sauvegarde", "Créez un plan : un dépôt git local → un dossier d'un cloud CORPUS.") +
      `<div class="cc-center"><button class="btn btn-primary" id="bk-new2">＋ Nouveau plan</button></div>`;
    el("bk-new2").onclick = backupModal;
    return;
  }
  box.innerHTML = plans
    .map((b) => `<div class="bk-card">
      <div class="bk-main"><b>${esc(b.name)}</b>
        <span class="bk-flow"><code>${esc(b.repo_path)}</code> → <b>${esc(b.cloud)}</b><code>${esc(b.dest || "/")}</code></span>
        <span class="bk-when">${b.last_run ? "dernière sauvegarde : " + esc(fmtWhen(b.last_run)) : "jamais exécutée"}</span></div>
      <div class="bk-act"><button class="btn btn-primary btn-sm" data-run="${esc(b.name)}">Sauvegarder maintenant</button>
        <button class="btn btn-ghost btn-sm" data-rm="${esc(b.name)}">Retirer</button></div></div>`)
    .join("");
  box.querySelectorAll("[data-run]").forEach((b) => (b.onclick = () => runBackupFlow(b.dataset.run)));
  box.querySelectorAll("[data-rm]").forEach((b) => (b.onclick = async () => {
    try { await App().RemoveBackupPlan(b.dataset.rm); renderBackup(); } catch (e) { toast(cleanErr(e), "err"); }
  }));
}

// prefill (facultatif) : {name,repo,cloud,dest} — pré-remplit, p. ex. au retour du sélecteur de
// dépôts existants (openRepoPicker).
function backupModal(prefill) {
  prefill = (prefill && typeof prefill === "object" && !prefill.type) ? prefill : {};
  const clouds = state.corpusClouds || [];
  if (!clouds.length) {
    toast("Connectez d'abord un cloud dans CORPUS", "err");
    return;
  }
  openModal("Nouveau plan de sauvegarde", `<div class="field">
    <label>Nom du plan</label><input id="bk-n" class="inp" placeholder="ex. KERNEL vers Nextcloud" autocomplete="off" value="${esc(prefill.name || "")}" />
    <label>Dépôt git local à sauvegarder</label><div class="cc-pickrow"><input id="bk-r" class="inp" placeholder="C:\\tmp\\KERNEL" autocomplete="off" value="${esc(prefill.repo || "")}" /><button class="btn btn-ghost btn-sm" type="button" id="bk-scan" title="Choisir parmi les dépôts déjà sur cette machine">🔎</button><button class="btn btn-ghost btn-sm" type="button" id="bk-pick" title="Parcourir un dossier">📁</button></div>
    <p class="sub">🔎 propose les dépôts déjà présents sur votre machine · 📁 choisit un dossier à la main.</p>
    <label>Cloud CORPUS de destination</label><select id="bk-c" class="select">${clouds.map((c) => `<option value="${esc(c.name)}"${prefill.cloud === c.name ? " selected" : ""}>${esc(c.name)} — ${esc(c.kind)}</option>`).join("")}</select>
    <label>Dossier dans le cloud</label><input id="bk-d" class="inp" placeholder="/Backups/KERNEL" value="${esc(prefill.dest || "/")}" autocomplete="off" />
    <div class="inline-error hidden" id="bk-e"></div></div>`, async () => {
    const name = el("bk-n").value.trim(), repo = el("bk-r").value.trim(), cloud = el("bk-c").value, dest = el("bk-d").value.trim() || "/";
    const e = el("bk-e");
    if (!name || !repo || !cloud) { e.textContent = "Nom, dépôt et cloud sont requis."; e.classList.remove("hidden"); return; }
    try {
      await App().AddBackupPlan(name, repo, cloud, dest);
      closeModal();
      renderBackup();
    } catch (err) { e.textContent = cleanErr(err); e.classList.remove("hidden"); }
  });
  el("modal-ok").textContent = "Enregistrer";
  el("bk-pick").onclick = async () => {
    try { const p = await App().PickFolder(); if (p) el("bk-r").value = p; } catch (_) {}
  };
  el("bk-scan").onclick = () => {
    const cur = { name: el("bk-n").value.trim(), cloud: el("bk-c").value, dest: el("bk-d").value.trim() };
    openRepoPicker((r) => backupModal({ name: cur.name, repo: r.path, cloud: cur.cloud, dest: cur.dest }));
  };
}

function runBackupFlow(name) {
  // La politique PIN est celle de la connexion cloud de destination ; on tente, et si le cœur l'exige,
  // la fenêtre PIN prend le relais (geste in-process).
  pinConfirm("Lancer la sauvegarde « " + name + " » (bundle de l'historique → cloud)", async (pin) => {
    const out = await App().RunBackup(name, pin);
    toast("Sauvegardé : " + out, "ok");
    renderBackup();
  });
}
