// renderPromptLib : la bibliothèque de prompts (accordéons éditables). Les prompts par défaut portent un
// badge ; les personnalisations de l'utilisateur sont conservées à travers les mises à jour (stockées en config).
async function renderPromptLib() {
  const box = el("prompt-lib"); if (!box) return;
  box.innerHTML = `<div class="skeleton"></div>`;
  let rows = [];
  try { rows = (await App().Prompts()) || []; } catch (_) { box.innerHTML = ""; return; }
  box.innerHTML = rows.map((p) => `
    <details class="plib-item">
      <summary><span class="plib-title">${esc(p.title)}</span> ${p.builtin ? `<span class="pill sm">${p.overridden ? "modifié" : "par défaut"}</span>` : `<span class="pill sm">perso</span>`}</summary>
      <pre class="plib-body">${esc(p.body)}</pre>
      <div class="plib-actions">
        <button class="btn btn-ghost btn-sm" data-pcopy="${esc(p.id)}">Copier</button>
        <button class="btn btn-ghost btn-sm" data-pedit="${esc(p.id)}">Modifier</button>
        ${p.builtin && p.overridden ? `<button class="btn btn-ghost btn-sm" data-preset="${esc(p.id)}">Réinitialiser</button>` : ""}
        <button class="lnk danger" data-pdel="${esc(p.id)}">${p.builtin ? "Masquer" : "Supprimer"}</button>
      </div>
    </details>`).join("") || `<div class="muted">Aucun prompt.</div>`;
  const byId = {}; rows.forEach((r) => byId[r.id] = r);
  box.querySelectorAll("[data-pcopy]").forEach((b) => b.onclick = () => copyPrompt(byId[b.dataset.pcopy].body, { title: byId[b.dataset.pcopy].title }));
  box.querySelectorAll("[data-pedit]").forEach((b) => b.onclick = () => promptEditModal(byId[b.dataset.pedit]));
  box.querySelectorAll("[data-preset]").forEach((b) => b.onclick = async () => { try { await App().ResetPrompt(b.dataset.preset); renderPromptLib(); } catch (e) { toast(cleanErr(e), "err"); } });
  box.querySelectorAll("[data-pdel]").forEach((b) => b.onclick = () => {
    const p = byId[b.dataset.pdel];
    openModal(p.builtin ? "Masquer ce prompt ?" : "Supprimer ce prompt ?", `<div class="field"><div class="callout">« ${esc(p.title)} »${p.builtin ? " sera masqué (réversible via « réinitialiser » plus tard)." : " sera supprimé."}</div></div>`, async () => {
      try { await App().DeletePrompt(p.id); closeModal(); renderPromptLib(); } catch (e) { toast(cleanErr(e), "err"); }
    });
  });
}

// promptEditModal : créer (p=null) ou modifier un prompt. Un prompt par défaut modifié devient une
// surcharge (conservée) sans toucher l'original livré.
function promptEditModal(p) {
  const isNew = !p;
  openModal(isNew ? "Nouveau prompt" : "Modifier le prompt", `<div class="field">
    <label>Titre<input id="pe-title" class="inp" value="${esc(p ? p.title : "")}" /></label>
    <label>Prompt<textarea id="pe-body" rows="6" style="width:100%;resize:vertical">${esc(p ? p.body : "")}</textarea></label>
    <div class="inline-error hidden" id="pe-err"></div></div>`, async () => {
    const er = el("pe-err");
    try { await App().SavePrompt(p ? p.id : "", el("pe-title").value, el("pe-body").value); closeModal(); renderPromptLib(); }
    catch (e) { if (er) { er.textContent = cleanErr(e); er.classList.remove("hidden"); } }
  });
  el("modal-ok").textContent = "Enregistrer";
}

async function renderSettings() {
  const theme = readTheme();
  const navItems = [["secu", "🔒 Compte & sécurité"], ["conn", "🔗 Connexions"], ["app", "🎨 Apparence"], ["ia", "🤖 Assistant IA"], ["diag", "🩺 Diagnostic"], ["ver", "🏷️ Versions"], ["lic", "⚖️ Licence"]];
  let setTab = "secu";
  try { setTab = localStorage.getItem("situlab.settab") || "secu"; } catch (_) {}
  el("view").innerHTML = `
    <div class="view-head"><div class="view-title">Réglages</div></div>
    <div class="set-layout">
      <nav class="set-nav">${navItems.map(([k, l]) => `<button class="set-navbtn" data-settab="${k}">${l}</button>`).join("")}</nav>
      <div class="set-panels">

    <section class="set-sec set-panel" data-panel="secu"><div class="set-sec-h">🔒 Compte &amp; sécurité</div>
      <div class="set-sec-d">Votre PIN déverrouille vos accès (jetons chiffrés sur ce poste) et confirme chaque envoi vers le serveur. Personne ne peut lire vos accès sans lui.</div>
      <div class="set-row"><div class="set-row-l"><b id="pin-state"></b></div>
        <div class="set-row-c"><button class="btn btn-ghost btn-sm" id="set-pin"></button><button class="lnk" id="pin-forgot">PIN oublié ?</button></div></div>
      <div class="set-row"><div class="set-row-l">Session <span id="sess-state" class="muted"></span></div>
        <div class="set-row-c"><button class="btn btn-ghost btn-sm" id="sess-btn"></button></div></div>
      <div class="set-row"><div class="set-row-l">Verrouillage automatique</div>
        <div class="set-row-c"><select id="lock-delay" class="select">
          <option value="10">après 10 minutes</option><option value="15">après 15 minutes</option>
          <option value="30">après 30 minutes</option><option value="60">après 1 heure</option>
          <option value="-1">jamais</option></select></div></div>
      <label class="set-row toggle"><div class="set-row-l">Verrouiller après chaque écriture <small>reverrouille dès qu'une modification est confirmée</small></div>
        <span class="switch"><input id="lock-write" type="checkbox" /><span class="track"></span></span></label>
      <label class="set-row toggle"><div class="set-row-l">Mode lecture seule <small>consulter sans risque : bloque tout envoi, suppression et modification</small></div>
        <span class="switch"><input id="read-only" type="checkbox" /><span class="track"></span></span></label>
      <div class="set-note">Gestion des jetons d'accès : onglet <b>Tokens</b>.</div>
    </section>

    <section class="set-sec set-panel" data-panel="conn"><div class="set-sec-h">🔗 Connexions</div>
      <div class="set-sec-d">Vos serveurs git (Forgejo/Gitea, GitHub). ★ = connexion par défaut.</div>
      <div id="inst-list"></div>
      <button class="btn btn-ghost btn-sm" id="add-inst" style="margin-top:10px">＋ Ajouter une connexion</button>
    </section>

    <section class="set-sec set-panel" data-panel="app"><div class="set-sec-h">🎨 Apparence &amp; affichage</div>
      <div class="set-row"><div class="set-row-l">Thème</div>
        <div class="set-row-c"><div class="seg" role="group" aria-label="Thème">
          <button data-th="auto" class="${theme === "auto" ? "on" : ""}">Auto</button>
          <button data-th="light" class="${theme === "light" ? "on" : ""}">Clair</button>
          <button data-th="dark" class="${theme === "dark" ? "on" : ""}">Sombre</button>
        </div></div></div>
      <label class="set-row toggle"><div class="set-row-l">Mode avancé <small>révèle les étapes git séparées, la matrice des branches, les outils git locaux et les réglages de dépôt</small></div>
        <span class="switch"><input id="adv-mode" type="checkbox" /><span class="track"></span></span></label>
    </section>

    <section class="set-sec set-panel" data-panel="ia"><div class="set-sec-h">🤖 Assistant IA</div>
      <div class="set-sec-d">SituLab dépose un fichier <code>AGENTS.md</code> près de vos dépôts : mode d'emploi pour votre IA + votre <b>fiche de collaboration</b>. Votre IA la lit au démarrage et peut l'enrichir avec vous. <button class="lnk" id="agents-view">Consulter AGENTS.md</button></div>
      <label class="set-row-l" style="display:block">Notes de collaboration (reprises dans AGENTS.md)
        <textarea id="agent-notes" rows="4" placeholder="Ex. : réponds en français ; propose toujours un plan avant d'agir ; mes dépôts de prod sont sur l'organisation apcom…" style="width:100%;margin-top:6px;resize:vertical"></textarea></label>
      <div class="set-row-c" style="margin-top:10px;justify-content:flex-start">
        <button class="btn btn-primary btn-sm" id="agent-save">Enregistrer</button>
        <button class="btn btn-ghost btn-sm" id="agent-deposit">Déposer AGENTS.md dans un dossier…</button>
        <button class="btn btn-ghost btn-sm" id="agent-config">Configurer mon agent IA</button>
      </div>
      <div class="plib-h"><b>Bibliothèque de prompts</b> <span class="muted">— des messages prêts à coller dans votre IA. Les vôtres sont conservés lors des mises à jour.</span></div>
      <div id="prompt-lib"></div>
      <button class="btn btn-ghost btn-sm" id="prompt-add" style="margin-top:8px">＋ Ajouter un prompt</button>
    </section>

    <section class="set-sec set-panel" data-panel="diag"><div class="set-sec-h">🩺 Diagnostic</div>
      <div class="set-sec-d">L'état de SituLab sur ce poste en un coup d'œil : git, PIN, session, connexions, coffre, signature, et les accès git qui échappent à la confirmation PIN.</div>
      <div id="diag-creds"><div class="muted">Vérification…</div></div>
    </section>

    <section class="set-sec set-panel" data-panel="ver"><div class="set-sec-h">🏷️ Versions</div>
      <div class="set-row"><div class="set-row-l">Version installée <b id="app-ver">…</b></div>
        <div class="set-row-c"><button class="btn btn-ghost btn-sm" id="ver-check">Vérifier les mises à jour ↗</button></div></div>
      <div class="set-row"><div class="set-row-l">Téléchargements (votre OS)</div>
        <div class="set-row-c"><button class="btn btn-ghost btn-sm" id="ver-site">situememoring.com ↗</button><button class="btn btn-ghost btn-sm" id="ver-gh">GitHub ↗</button></div></div>
      <div class="set-note">Les installeurs et l'historique complet des versions sont publiés sur situememoring.com (lecture en direct des Releases GitHub).</div>
    </section>

    <section class="set-sec set-panel" data-panel="lic"><div class="set-sec-h">⚖️ Licence</div>
      <div class="set-sec-d">SituLab — logiciel libre sous licence <b>Apache-2.0</b>. © 2026 <b>APCOM Solutions SA</b>. La marque « SituLab » n'est pas couverte par la licence.</div>
      <div class="set-row-c" style="justify-content:flex-start"><button class="btn btn-ghost btn-sm" id="lic-show">Texte de la licence</button><button class="btn btn-ghost btn-sm" id="notice-show">NOTICE</button></div>
    </section>

    <section class="set-sec danger-zone set-panel" data-panel="secu"><div class="dz-h">⚠ Zone sensible — effacer mes données de ce poste</div>
      <p class="sub">Supprime définitivement vos connexions, votre trousseau chiffré, vos réglages et les jetons du coffre Windows sur cette machine. Pour repartir à zéro ou céder le poste. <b>Irréversible.</b></p>
      <button class="btn btn-ghost btn-sm btn-danger" id="purge-data">Effacer mes données…</button>
    </section>

      </div>
    </div>`;

  // Nav à onglets : affiche un panneau à la fois (le DOM garde tous les panneaux → le câblage par id
  // ci-dessous reste valable). Onglet mémorisé par poste (confort local).
  const showTab = (k) => {
    document.querySelectorAll(".set-panel").forEach((p) => p.classList.toggle("hidden", p.dataset.panel !== k));
    document.querySelectorAll(".set-navbtn").forEach((b) => b.classList.toggle("active", b.dataset.settab === k));
    try { localStorage.setItem("situlab.settab", k); } catch (_) {}
  };
  document.querySelectorAll(".set-navbtn").forEach((b) => b.onclick = () => showTab(b.dataset.settab));
  showTab(setTab);
  const agv = el("agents-view");
  if (agv) agv.onclick = async () => { try { infoModal("AGENTS.md", `<div class="md-render" style="max-height:60vh;overflow:auto">${mdToHTML(await App().Doc())}</div>`); } catch (_) {} };
  renderPromptLib();
  if (el("prompt-add")) el("prompt-add").onclick = () => promptEditModal(null);

  document.querySelectorAll("[data-th]").forEach((b) => b.onclick = () => { writeTheme(b.dataset.th); applyTheme(b.dataset.th); renderSettings(); });
  const adv = el("adv-mode");
  if (adv) {
    adv.checked = !!state.advanced;
    adv.onchange = () => { setAdvanced(adv.checked); toast(adv.checked ? "Mode avancé activé" : "Mode simplifié activé", "ok"); };
  }

  // Section Versions : version installée + liens (téléchargement/MAJ sur le site, lecture en direct des
  // Releases GitHub). La détection fine de MAJ se fait côté site (connect-src 'self' interdit le fetch
  // d'une API externe depuis la fenêtre) ; le bouton ouvre donc la page des versions dans le navigateur.
  const openURL = (u) => safeOpenURL(u);
  try { el("app-ver").textContent = await App().AppVersion(); } catch (_) { el("app-ver").textContent = "?"; }
  if (el("ver-check")) el("ver-check").onclick = () => openURL("https://situememoring.com/#telecharger");
  if (el("ver-site")) el("ver-site").onclick = () => openURL("https://situememoring.com/");
  if (el("ver-gh")) el("ver-gh").onclick = () => openURL("https://situememoring.com/#telecharger");
  // Section Licence : texte Apache-2.0 + NOTICE embarqués (go:embed), affichés en modale.
  if (el("lic-show")) el("lic-show").onclick = async () => { try { infoModal("Licence Apache-2.0", `<pre class="filebox" style="user-select:text;white-space:pre-wrap">${esc(await App().License())}</pre>`); } catch (_) {} };
  if (el("notice-show")) el("notice-show").onclick = async () => { try { infoModal("NOTICE", `<pre class="filebox" style="user-select:text;white-space:pre-wrap">${esc(await App().Notice())}</pre>`); } catch (_) {} };

  try {
    const ins = await App().GetInstances();
    state._settingsInstances = ins;
    await refreshDisconnected();
    // Ping réel de chaque connexion : détecte aussi un jeton présent mais REFUSÉ (révoqué en ligne),
    // que la liste « sans token » ne voit pas. "auth" = à reconnecter ; "net" = serveur injoignable.
    const health = {};
    await Promise.all(ins.map(async (i) => { health[i.name] = isDisconnected(i.name) ? "auth" : await pingInstance(i.name); }));
    el("inst-list").innerHTML = ins.map((i) => {
      const h = health[i.name] || "ok", dc = h !== "ok";
      const prod = i.role === "production";
      const badge = `<span class="role-badge ${prod ? "role-production" : "role-atelier"}">${prod ? "VITRINE" : "ATELIER"}</span>`;
      const pill = h === "auth" ? `<span class="pill disc-pill">déconnectée</span>` : (h === "net" ? `<span class="pill disc-pill">hors ligne</span>` : "");
      const reconBtn = dc ? `<button class="btn btn-primary btn-sm" data-recon="${esc(i.name)}">Reconnecter</button>` : "";
      return `<div class="srow${dc ? " srow-disc" : ""}"><div>${i.default ? "★ " : ""}<b>${esc(i.name)}</b> ${badge} <span class="muted">${esc(i.url)}</span> ${pill}</div>
      <div class="srow-act">${reconBtn}<button class="lnk" data-pol="${esc(i.name)}">rôle &amp; PIN</button>${i.default ? "" : `<button class="lnk" data-def="${esc(i.name)}">défaut</button>`}<button class="lnk danger" data-del="${esc(i.name)}">supprimer</button></div></div>`;
    }).join("");
    el("inst-list").onclick = async (ev) => {
      const def = ev.target.closest("[data-def]"), del = ev.target.closest("[data-del]"), recon = ev.target.closest("[data-recon]"), pol = ev.target.closest("[data-pol]");
      if (pol) return connPolicyModal(pol.dataset.pol);
      if (recon) return reconnectModal(recon.dataset.recon);
      if (def) { try { await App().SetDefaultInstance(def.dataset.def); toast("Instance par défaut mise à jour", "ok"); renderSettings(); } catch (e) { toast(cleanErr(e), "err"); } }
      if (del) {
        const nm = del.dataset.del;
        openModal("Supprimer cette connexion ?", `<div class="field"><div class="callout warn">Vous retirez « ${esc(nm)} » et l'accès chiffré que SituLab garde pour ce serveur. Vos dépôts et fichiers sur le poste ne sont pas touchés, mais pour revenir il faudra vous reconnecter (identifiant + mot de passe + code 2FA).</div></div>`, async () => {
          try { await App().RemoveInstance(nm); closeModal(); toast("Connexion supprimée", "ok"); location.reload(); } catch (e) { toast(cleanErr(e), "err"); }
        });
        el("modal-ok").textContent = "Supprimer"; el("modal-ok").classList.add("danger-btn");
      }
    };
  } catch (e) { el("inst-list").innerHTML = surfaceError("Lecture impossible", cleanErr(e), null); }
  el("add-inst").onclick = addInstanceModal;

  const hasPin = await App().PinSet().catch(() => false);
  el("pin-state").textContent = hasPin ? "Un PIN est défini." : "Aucun PIN défini pour le moment.";
  el("set-pin").textContent = hasPin ? "Changer le PIN" : "Créer un PIN";
  el("set-pin").onclick = setPinModal;
  el("pin-forgot").onclick = pinForgotModal;
  el("pin-forgot").classList.toggle("hidden", !hasPin);
  // État de session + bouton verrouiller/déverrouiller.
  const unlocked = hasPin && (await App().Unlocked().catch(() => false));
  el("sess-state").textContent = !hasPin ? "" : unlocked ? "● Session déverrouillée" : "○ Session verrouillée";
  const sb = el("sess-btn");
  sb.classList.toggle("hidden", !hasPin);
  sb.textContent = unlocked ? "Verrouiller maintenant" : "Déverrouiller";
  sb.onclick = async () => {
    if (unlocked) { try { await App().Lock(); toast("Session verrouillée", "ok"); renderSettings(); } catch (e) { toast(cleanErr(e), "err"); } }
    else unlockScreen(() => init());
  };
  // Délai d'auto-verrouillage réglable.
  try {
    const d = await App().LockDelay();
    const ls = el("lock-delay");
    if (ls) {
      ls.value = String(d === 0 ? 15 : d);
      ls.onchange = async () => { try { await App().SetLockDelay(parseInt(ls.value, 10)); toast("Délai de verrouillage mis à jour", "ok"); } catch (e) { toast(cleanErr(e), "err"); } };
    }
    const lw = el("lock-write");
    if (lw) {
      lw.checked = await App().LockAfterWrite().catch(() => false);
      lw.onchange = async () => { try { await App().SetLockAfterWrite(lw.checked); toast(lw.checked ? "Verrouillage après écriture activé" : "Verrouillage après écriture désactivé", "ok"); } catch (e) { toast(cleanErr(e), "err"); lw.checked = !lw.checked; } };
    }
    const ro = el("read-only");
    if (ro) {
      ro.checked = !!state.readOnly;
      ro.onchange = async () => {
        try {
          await App().SetReadOnly(ro.checked);
          state.readOnly = ro.checked;
          updateReadOnlyBanner();
          toast(ro.checked ? "Mode lecture seule activé" : "Mode lecture seule désactivé", "ok");
        } catch (e) { toast(cleanErr(e), "err"); ro.checked = !ro.checked; }
      };
    }
    const pd = el("purge-data");
    if (pd) pd.onclick = purgeDataModal;
    // Assistant IA : fiche de collaboration éditable + dépôt + configuration.
    const an = el("agent-notes");
    if (an) {
      App().AgentNotes().then((v) => { an.value = v || ""; }).catch(() => {});
      el("agent-save").onclick = async () => { try { await App().SetAgentNotes(an.value); toast("Fiche de collaboration enregistrée", "ok"); } catch (e) { toast(cleanErr(e), "err"); } };
      el("agent-deposit").onclick = installDirectives;
      el("agent-config").onclick = aiAgentModal;
    }
  } catch (e) {}
  renderDiagnostic();
}

// renderDiagnostic : diagnostic VIVANT (1.9.4) — une liste de contrôles lisibles (git, PIN, session,
// connexions, coffre OS, signature, autres accès git) avec statut, explication et action quand utile.
async function renderDiagnostic() {
  const dc = el("diag-creds"); if (!dc) return;
  dc.innerHTML = `<div class="skeleton"></div>`;
  let doc;
  try { doc = await App().Doctor(); } catch (e) { dc.innerHTML = `<div class="callout warn">${esc(cleanErr(e))}</div>`; return; }
  const row = (state, label, detail, action) => `<div class="diag-row diag-${state}"><span class="diag-ico">${state === "ok" ? "✅" : state === "warn" ? "⚠️" : "ℹ️"}</span><div class="diag-txt"><b>${esc(label)}</b>${detail ? `<div class="muted">${detail}</div>` : ""}</div>${action || ""}</div>`;
  const rows = [];
  rows.push(doc.git_available
    ? row("ok", "Git installé", esc(doc.git_version || ""))
    : row("warn", "Git absent", "Clone, synchro et versions locales sont indisponibles.", `<button class="btn btn-ghost btn-sm" data-openurl="https://git-scm.com/downloads">Installer git ↗</button>`));
  rows.push(doc.pin_set ? row("ok", "PIN configuré", "Vos jetons sont chiffrés par votre PIN.") : row("warn", "Aucun PIN", "Définissez-en un dans Compte & sécurité."));
  rows.push(doc.unlocked ? row("ok", "Session déverrouillée", "SituLab peut lire vos accès.") : row("info", "Session verrouillée", "Déverrouillez avec votre PIN pour agir."));
  rows.push(row("info", "Connexions", `${doc.instances} serveur(s) configuré(s).`));
  rows.push(doc.vault_residual ? row("warn", "Coffre Windows", `${doc.vault_residual} jeton(s) encore dans l'ancien coffre OS — ils seront migrés automatiquement au prochain déverrouillage.`) : row("ok", "Coffre Windows", "Aucun jeton résiduel."));
  let swActive = false; try { swActive = await App().AttestSoftwareActive(); } catch (_) {}
  rows.push(doc.sign_test_root
    ? row("warn", "Signature de garantie", "Racine de TEST embarquée — la vraie racine de gouvernance n'est pas installée (les ✅ ne sont pas vérifiables réellement).")
    : swActive
      ? row("ok", "Signature logicielle", "Active — chaque commit via SituLab porte la garantie « SituLab » (signature du logiciel, pas par utilisateur).")
      : row("info", "Signature logicielle", "Pas encore activée sur ce binaire — elle sera mintée par la gouvernance (identité unique « SituLab »)."));
  rows.push((doc.other_git_creds || []).length
    ? row("warn", "Autres accès git sur ce poste", `${doc.other_git_creds.map(esc).join(", ")}. Ces identifiants (clé SSH, credential helper) permettent un accès git <b>indépendant</b> de SituLab, hors confirmation PIN. Pour un cloisonnement strict, faites tourner vos agents sous un autre compte / bac à sable.`)
    : row("ok", "Autres accès git", "Aucun identifiant git concurrent détecté — la confirmation PIN protège bien."));
  dc.innerHTML = `<div class="diag-list">${rows.join("")}</div>`;
}

// pinForgotModal : explication HONNÊTE (le PIN est irrécupérable) + action « repartir de zéro ».
function pinForgotModal() {
  openModal("PIN oublié ?", `<div class="field">
    <p class="sub">Pas de souci. Vous allez simplement <b>choisir un nouveau PIN</b>. Vos instances seront alors <b>déconnectées</b> : il suffira de les <b>reconnecter</b> (identifiant + mot de passe + code 2FA) — SituLab vous guidera. Vos dépôts et fichiers sur ce poste ne bougent pas.</p>
    <div class="inline-error hidden" id="pf-err"></div></div>`, async () => {
    try { await App().ResetKeyring(); location.reload(); }
    catch (e) { const er = el("pf-err"); er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  el("modal-ok").textContent = "Choisir un nouveau PIN";
}

