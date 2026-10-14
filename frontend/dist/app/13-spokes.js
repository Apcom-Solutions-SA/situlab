/* ---------------- SPOKES : projections de mission (sous-menu KERNEL, 1.11) ----------------
 * Un spoke est un dépôt local qui PROJETTE une mission dérivée d'un KERNEL. Ce qui le distingue d'un
 * dépôt ordinaire : son KERNEL parent et sa mission. L'humain désigne les spokes (geste de fenêtre) ;
 * il voit leur état en clair et peut récupérer/enregistrer leur travail. Réutilise klTone (11-kernel).
 */

// renderSpokes : la page SPOKES liste vos dépôts désignés comme spokes, en CARTES CLASSEUR (même
// langage visuel que KERNEL / Dépôts git). Un clic ouvre le DÉTAIL du dépôt. Désigner = choisir un
// dépôt appairé (plug-and-play, comme le KERNEL).
async function renderSpokes() {
  const view = el("view");
  view.innerHTML = `<div class="view-head view-head-row">
      <div><div class="view-title">SPOKES</div><div class="view-sub">Vos dépôts de mission (projections). Un clic ouvre le détail.</div></div>
      <button class="btn btn-primary btn-sm" id="sp-new">＋ Désigner un spoke</button></div>
    <div id="sp-home"><div class="skeleton"></div><div class="skeleton" style="margin-top:10px"></div></div>`;
  el("sp-new").onclick = pickSpokeRepo;
  let rows = [], spCfg = [], kernels = [];
  try {
    rows = (await App().ProjectsLight()) || [];
    spCfg = (await App().Spokes()) || [];       // config des spokes : KERNEL parent + mission
    kernels = (await App().Kernels()) || [];     // KERNEL disponibles comme parents
  } catch (e) { if (isLocked(e)) return handleLock(); const b = el("sp-home"); if (b) b.innerHTML = surfaceError("Lecture impossible", cleanErr(e), null); return; }
  const kdedup = dedupeKernels(kernels);
  state.spokeCfg = spCfg; state.kernelsForSpoke = kdedup;
  const spokeByName = {}; spCfg.forEach((s) => { spokeByName[(s.name || "").toLowerCase()] = s; });
  const spokes = rows.filter((p) => p.spoke && !p.kernel);
  const home = el("sp-home"); if (!home) return;
  if (!spokes.length) {
    home.innerHTML =
      blankslate("🛰", "Aucun spoke désigné", "Un spoke est un dépôt de mission, rattaché à un KERNEL. Désignez-en un parmi vos dépôts appairés — SituLab le marquera dans Dépôts git.") +
      `<div class="cc-center"><button class="btn btn-primary" id="sp-new2">＋ Désigner un spoke</button></div>`;
    if (el("sp-new2")) el("sp-new2").onclick = pickSpokeRepo;
    return;
  }
  home.innerHTML = `<div class="classeur-grid">${spokes.map((p) => {
    const cfg = spokeByName[(p.name || "").toLowerCase()] || {};
    const parent = cfg.kernel || "";
    const run = cfg.run_file || "";
    const link = parent
      ? `<span class="sp-parent">↳ <span class="sp-parent-pill">💡 ${esc(parent)}</span></span><button type="button" class="lnk" data-parent="${esc(p.name)}">modifier</button>`
      : `<button type="button" class="lnk sp-parent-warn" data-parent="${esc(p.name)}">↳ Rattacher à un KERNEL</button>`;
    const runLink = run
      ? `<span class="run-pill" title="Fichier run pour l'IA">📄 ${esc(run)}</span><button type="button" class="lnk" data-run="${esc(p.name)}">modifier</button>`
      : `<button type="button" class="lnk run-set" data-run="${esc(p.name)}">📄 Définir le fichier run</button>`;
    return `<div class="desig-wrap">${classeurCardHTML(p)}
      <div class="desig-meta">${link}</div>
      <div class="desig-meta">${runLink}<button type="button" class="lnk danger" data-remove="${esc(p.name)}">Retirer</button></div></div>`;
  }).join("")}</div>`;
  home.onclick = (ev) => {
    const rm = ev.target.closest("[data-remove]");
    if (rm) { ev.stopPropagation(); return removeSpokeFlow(rm.dataset.remove); }
    const par = ev.target.closest("[data-parent]");
    if (par) { ev.stopPropagation(); const cfg = spokeByName[par.dataset.parent.toLowerCase()] || {}; return spokeParentModal(par.dataset.parent, cfg.kernel || "", kdedup, (k) => setSpokeParent(par.dataset.parent, k)); }
    const rf = ev.target.closest("[data-run]");
    if (rf) {
      ev.stopPropagation();
      const cfg = spokeByName[rf.dataset.run.toLowerCase()] || {};
      const path = cfg.local_path || "";
      if (!path) { toast("Chemin local du spoke inconnu", "err"); return; }
      return runFileModal(path, cfg.run_file || "", async (f) => {
        try { await App().SetSpokeRunFile(path, f); closeModal(); toast(f ? "Fichier run défini" : "Fichier run retiré", "ok"); renderSpokes(); }
        catch (e) { if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); }
      }, "Fichier run — spoke");
    }
    const c = ev.target.closest("[data-open-project]"); if (c) openProject(c.dataset.openProject);
  };
}

// dedupeKernels : évite les doublons de KERNEL pointant le même dépôt (clé = chemin local).
function dedupeKernels(ks) {
  const seen = new Set(); const out = [];
  (ks || []).forEach((k) => { const key = (k.local_path || k.name || "").toLowerCase(); if (!seen.has(key)) { seen.add(key); out.push(k); } });
  return out;
}

// spokeParentModal : choisir le KERNEL parent d'un spoke, en CARTES cliquables (pas de select). onPick(name).
function spokeParentModal(spokeName, current, kernels, onPick) {
  const list = (kernels || []).map((k) => `<button type="button" class="kparent${config_eq(k.name, current) ? " on" : ""}" data-k="${esc(k.name)}"><span class="kparent-ico">💡</span>${esc(k.name)}</button>`).join("");
  openModal(`KERNEL parent — ${esc(spokeName)}`, `<div class="field">
    <p class="sub">À quel <b>KERNEL</b> rattacher ce spoke ? (un spoke projette la mission d'un KERNEL)</p>
    <div class="kparent-list">
      <button type="button" class="kparent${current ? "" : " on"}" data-k=""><span class="kparent-ico">∅</span>Aucun</button>
      ${list}
    </div></div>`, null);
  el("modal-ok").classList.add("hidden");
  el("modal-body").querySelectorAll(".kparent").forEach((b) => b.onclick = () => { closeModal(); onPick(b.dataset.k); });
}

// config_eq : comparaison de noms insensible à la casse (comme côté noyau).
function config_eq(a, b) { return (a || "").toLowerCase() === (b || "").toLowerCase(); }

// setSpokeParent : (re)rattache un spoke à un KERNEL parent. Réutilise AddSpoke (met à jour l'existant),
// en conservant chemin, mission et connexion déjà enregistrés.
async function setSpokeParent(name, kernel) {
  const cfg = (state.spokeCfg || []).find((s) => config_eq(s.name, name)) || {};
  try {
    await App().AddSpoke(name, cfg.local_path || "", kernel, cfg.mission || "", cfg.instance || "");
    toast(kernel ? "Spoke rattaché à « " + kernel + " »" : "Rattachement retiré", "ok");
    renderSpokes();
  } catch (e) { if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); }
}

// removeSpokeFlow : retire la DÉSIGNATION spoke (le dépôt n'est pas touché).
function removeSpokeFlow(name) {
  openModal("Retirer ce spoke ?", `<div class="field"><div class="callout">« ${esc(name)} » ne sera plus marqué comme <b>spoke</b>. Le dépôt n'est <b>pas</b> touché — il reste dans vos dépôts appairés.</div></div>`, async () => {
    try { await App().RemoveSpoke(name); closeModal(); toast("Spoke retiré", "ok"); renderSpokes(); }
    catch (e) { if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); }
  });
  el("modal-ok").textContent = "Retirer";
}

// pickSpokeRepo : désigner un spoke = CHOISIR l'un de ses DÉPÔTS APPAIRÉS. Plug-and-play : au clic, on
// désigne direct (nom, chemin local, connexion) depuis l'appairage — sans formulaire.
async function pickSpokeRepo() {
  let rows = [], kernels = [];
  try { rows = (await App().ProjectsLight()) || []; kernels = dedupeKernels((await App().Kernels()) || []); }
  catch (e) { if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); return; }
  const eligible = rows.filter((p) => !p.kernel); // un KERNEL ne redevient pas un spoke
  if (!eligible.length) {
    openModal("Désigner un spoke", `<div class="field"><p class="sub">Aucun dépôt appairé disponible. Appairez d'abord vos dépôts dans <b>Dépôts git</b>, puis revenez désigner un spoke.</p></div>`, () => { closeModal(); navTo("projects"); });
    el("modal-ok").textContent = "Aller à Dépôts git";
    return;
  }
  openModal("Désigner un spoke", `<div class="field">
    <p class="sub">Un spoke est l'un de vos <b>dépôts appairés</b>. Choisissez celui qui porte votre mission.</p>
    <div class="kpick-grid">${eligible.map(kpickCardHTML).join("")}</div></div>`, null);
  el("modal-ok").classList.add("hidden");
  el("modal-body").querySelectorAll(".kpick").forEach((c) => {
    const pick = async () => {
      const p = eligible[+c.dataset.i];
      if (!p) return;
      if (!p.local || !p.local.path) { toast("Ce dépôt n'est pas cloné sur ce poste — clonez-le d'abord", "err"); return; }
      const add = async (kernel) => {
        try {
          await App().AddSpoke(p.name, p.local.path, kernel || "", "", (p.atelier || {}).instance || "");
          closeModal();
          toast("Spoke « " + p.name + " » désigné" + (kernel ? " · KERNEL : " + kernel : ""), "ok");
          renderSpokes();
        } catch (e) {
          if (isLocked(e)) return handleLock();
          toast(cleanErr(e), "err");
        }
      };
      // Ergonomie : on POSE la question du KERNEL parent à la désignation (s'il existe des KERNEL).
      if (kernels.length) spokeParentModal(p.name, "", kernels, add);
      else add("");
    };
    c.onclick = pick;
    c.onkeydown = (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); pick(); } };
  });
}

async function loadSpokeStatus(name, i) {
  const box = el("spk-st-" + i);
  if (!box) return;
  try {
    const st = await App().SpokeStatus(name);
    const tone = klTone(st);
    box.innerHTML = `<span class="spk-dot ${tone}"></span>${esc(st.summary || "")} <span class="muted">· ${esc(st.branch || "")}</span>`;
  } catch (e) {
    box.innerHTML = `<span class="spk-dot red"></span>${esc(cleanErr(e))}`;
  }
}

function spokePull(name) {
  App().SpokePull(name)
    .then(() => { toast("Récupéré", "ok"); renderSpokes(); })
    .catch((e) => { if (isLocked(e)) return handleLock(); toast(cleanErr(e), "err"); });
}

function spokeSave(name) {
  if (state.readOnly) { toast("Mode lecture seule actif — désactivez-le dans Réglages", "err"); return; }
  const stamp = new Date().toLocaleString("fr-CH", { timeZone: "Europe/Zurich", dateStyle: "short", timeStyle: "short" });
  openModal("Enregistrer le spoke", `<div class="field"><label>Libellé de l'enregistrement</label>
    <input id="sp-msg" class="inp" value="Travail du ${esc(stamp)}" autocomplete="off" />
    <p class="sub">L'envoi en ligne demandera votre PIN.</p></div>`, () => {
    const msg = el("sp-msg").value.trim();
    closeModal();
    pinConfirm("Enregistrer et envoyer le travail du spoke « " + name + " »", async (pin) => {
      const res = await App().SpokeSave(name, msg, pin);
      toast(res && res.summary ? res.summary : "Enregistré", "ok");
      renderSpokes();
    });
  });
  el("modal-ok").textContent = "Continuer";
}

/* ---- Désigner / modifier un spoke (FENÊTRE) ---- */
// prefill (facultatif) : {name,path,kernel,mission,instance} — utilisé au retour du sélecteur de
// dépôts existants (openRepoPicker), uniquement pour un NOUVEAU spoke.
function spokeModal(sp, prefill) {
  const isNew = !sp;
  prefill = prefill || {};
  const kernels = state.kernelsForSpoke || [];
  const insts = state.instances || [];
  const v = (k) => (sp ? (sp[k] || "") : (prefill[k] || ""));
  openModal(isNew ? "Nouveau spoke" : "Modifier « " + esc(sp.name) + " »", `<div class="field">
    <label>Nom</label><input id="sp-n" class="inp" value="${esc(sp ? sp.name : (prefill.name || ""))}" placeholder="ex. Mission Furie-Alpes" autocomplete="off" ${isNew ? "" : "readonly"} />
    <label>Dépôt local</label><div class="cc-pickrow"><input id="sp-p" class="inp" value="${esc(sp ? sp.local_path : (prefill.path || ""))}" placeholder="C:\\tmp\\mission" autocomplete="off" />${isNew ? `<button class="btn btn-ghost btn-sm" type="button" id="sp-scan" title="Choisir parmi les dépôts déjà sur cette machine">🔎</button>` : ""}<button class="btn btn-ghost btn-sm" type="button" id="sp-pick" title="Parcourir un dossier">📁</button></div>
    <label>KERNEL parent</label><select id="sp-k" class="select"><option value="">— aucun —</option>${kernels.map((k) => `<option value="${esc(k.name)}"${(sp ? sp.kernel : prefill.kernel) === k.name ? " selected" : ""}>${esc(k.name)}</option>`).join("")}</select>
    <label>Mission (intention)</label><textarea id="sp-m" class="inp" rows="2" placeholder="À quoi sert ce spoke ?">${esc(v("mission"))}</textarea>
    <label>Connexion git</label><select id="sp-i" class="select"><option value="">— par défaut —</option>${insts.map((x) => `<option value="${esc(x.name)}"${(sp ? sp.instance : prefill.instance) === x.name ? " selected" : ""}>${esc(x.name)}</option>`).join("")}</select>
    <div class="inline-error hidden" id="sp-e"></div>
    ${isNew ? "" : `<div class="kl-mg-danger"><button type="button" class="btn btn-ghost btn-sm" id="sp-rm">Retirer ce spoke</button><span class="sub">(le dépôt n'est pas touché)</span></div>`}
  </div>`, async () => {
    const name = el("sp-n").value.trim(), path = el("sp-p").value.trim(), kernel = el("sp-k").value, mission = el("sp-m").value.trim(), inst = el("sp-i").value;
    const er = el("sp-e");
    if (!name || !path) { er.textContent = "Nom et dépôt local sont requis."; er.classList.remove("hidden"); return; }
    try {
      await App().AddSpoke(name, path, kernel, mission, inst);
      closeModal();
      toast(isNew ? "Spoke désigné" : "Spoke mis à jour", "ok");
      renderSpokes();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  el("modal-ok").textContent = isNew ? "Désigner" : "Enregistrer";
  el("sp-pick").onclick = async () => { try { const p = await App().PickFolder(); if (p) el("sp-p").value = p; } catch (_) {} };
  if (el("sp-scan")) el("sp-scan").onclick = () => {
    const cur = { name: el("sp-n").value.trim(), kernel: el("sp-k").value, mission: el("sp-m").value.trim(), instance: el("sp-i").value };
    openRepoPicker((r) => spokeModal(null, { name: cur.name || r.name, path: r.path, kernel: cur.kernel, mission: cur.mission, instance: cur.instance }));
  };
  if (el("sp-rm")) el("sp-rm").onclick = async () => {
    try { await App().RemoveSpoke(sp.name); closeModal(); toast("Spoke retiré", "ok"); renderSpokes(); } catch (e) { const er = el("sp-e"); er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  };
}
