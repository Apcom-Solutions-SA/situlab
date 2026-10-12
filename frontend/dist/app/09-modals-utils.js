/* ---------------- Modales ---------------- */
let modalOk = null, modalCancel = null;
function openModal(title, bodyHtml, onOk) {
  const m = el("modal");
  el("modal-body").innerHTML = `<h3 id="modal-title">${esc(title)}</h3>${bodyHtml}`;
  m.classList.remove("hidden");
  // Réinitialise le bouton Valider (un appelant précédent a pu le renommer / le colorer).
  const ok = el("modal-ok");
  ok.textContent = "Valider"; ok.classList.remove("danger-btn", "hidden"); ok.disabled = false;
  el("modal-cancel").textContent = "Annuler"; el("modal-cancel").classList.remove("hidden"); // un appelant précédent a pu le renommer/masquer
  // Enveloppe onOk pour DÉSACTIVER le bouton pendant l'appel : empêche la double soumission
  // (double-clic -> deux écritures / deux tokens). Ré-activé si la modale reste ouverte.
  modalOk = onOk ? async () => {
    if (ok.disabled) return;
    ok.disabled = true;
    try { await onOk(); } finally { if (!m.classList.contains("hidden")) ok.disabled = false; }
  } : null;
  modalCancel = closeModal;
  ok.onclick = () => modalOk && modalOk();
  el("modal-cancel").onclick = () => (modalCancel || closeModal)();
  m.onclick = (ev) => { if (ev.target === m) (modalCancel || closeModal)(); };
  addRevealToggle(el("modal-body"));
  const first = m.querySelector("input"); if (first) first.focus();
}
function closeModal() { el("modal").classList.add("hidden"); modalOk = null; modalCancel = null; }

// pinConfirm : 2e étape d'une écriture — l'utilisateur a saisi ses données, on lui demande
// maintenant SON PIN SituLab pour confirmer l'action. onPin(pin) exécute l'action ; en cas
// d'erreur, la saisie du PIN reste affichée. Validation auto dès les 6 chiffres.
function pinConfirm(reason, onPin) {
  // Lecture seule (v1.4) : court-circuiter AVANT la saisie du PIN — le cœur refuserait de toute
  // façon. Couvre d'un seul point toutes les mutations confirmées par PIN (suppression, admin,
  // jetons, issues/PR, création…).
  if (state.readOnly) { toast("Mode lecture seule actif — désactivez-le dans Réglages", "err"); return; }
  let ci;
  openModal("Confirmer avec votre PIN", `<div class="field">
    <p class="sub">${esc(reason)}</p>
    <label>Votre PIN SituLab</label><div id="pc-pin"></div>
    <div class="inline-error hidden" id="pc-err"></div></div>`, async () => {
    const err = el("pc-err"); err.classList.add("hidden");
    try { await onPin(ci.read()); }
    catch (e) { err.textContent = cleanErr(e); err.classList.remove("hidden"); ci.clear(); }
  });
  el("modal-ok").textContent = "Confirmer";
  ci = codeInput(el("pc-pin"), true, () => modalOk(), true);
}

// providerLogo : logo inline (zéro dépendance, CSP-safe) pour les cartes de choix de serveur.
function providerLogo(kind) {
  if (kind === "github") {
    return `<svg class="conn-logo" viewBox="0 0 16 16" width="34" height="34" fill="currentColor" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 0 1 2-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z"/></svg>`;
  }
  return `<svg class="conn-logo" viewBox="0 0 24 24" width="34" height="34" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="6" cy="6" r="2.2"/><circle cx="6" cy="18" r="2.2"/><circle cx="17.5" cy="9" r="2.2"/><path d="M6 8.2v7.6M8.1 7.1 15.4 9M6 15.8c0-3 .5-4.8 3.5-4.8H15"/></svg>`;
}

// addInstanceModal : TUNNEL d'ajout de connexion (1.9.4) — nom → type (cartes/logos) → rôle → jeton
// (+ aide et URL) → confirmation. Réutilise AddInstanceToken/AddInstanceLogin + SetInstanceRole.
function addInstanceModal() {
  const W = { name: "", provider: "", role: "atelier" };
  const hist = [];
  const m = el("modal"), okBtn = el("modal-ok"), cancelBtn = el("modal-cancel");
  m.classList.remove("hidden");
  m.onclick = (ev) => { if (ev.target === m) closeModal(); };
  const head = (n, t) => `<h3 id="modal-title">Nouvelle connexion</h3><div class="wiz-head"><span class="wiz-step">Étape ${n}/4</span> · ${esc(t)}</div>`;
  const screen = (html) => { el("modal-body").innerHTML = html; };
  const buttons = (okText, okFn) => {
    okBtn.classList.toggle("hidden", !okText);
    okBtn.disabled = false; okBtn.textContent = okText || ""; okBtn.onclick = okFn || null;
    cancelBtn.classList.remove("hidden");
    cancelBtn.textContent = hist.length > 1 ? "Retour" : "Annuler";
    cancelBtn.onclick = hist.length > 1 ? back : closeModal;
  };
  const go = (fn) => { hist.push(fn); fn(); };
  const back = () => { hist.pop(); const p = hist[hist.length - 1]; if (p) p(); };

  function stepName() {
    screen(`${head(1, "Nom de la connexion")}<div class="field"><input id="c-name" class="inp" placeholder="ex. Travail, GitHub perso…" value="${esc(W.name)}" /><small class="muted">Une étiquette à votre choix.</small></div>`);
    buttons("Suivant", () => { const v = el("c-name").value.trim(); if (!v) { toast("Donnez un nom.", "err"); return; } W.name = v; go(stepType); });
    const i = el("c-name"); if (i) { i.focus(); i.onkeydown = (e) => { if (e.key === "Enter") okBtn.click(); }; }
  }
  function stepType() {
    screen(`${head(2, "Type de serveur")}<div class="conn-cards">
      <button type="button" class="conn-card" data-p="github">${providerLogo("github")}<span class="cc-name">GitHub</span><span class="cc-sub">github.com</span></button>
      <button type="button" class="conn-card" data-p="forgejo">${providerLogo("forgejo")}<span class="cc-name">Forgejo / Gitea</span><span class="cc-sub">serveur auto-hébergé</span></button>
    </div>`);
    buttons("", null);
    el("modal-body").querySelectorAll(".conn-card").forEach((b) => b.onclick = () => { W.provider = b.dataset.p; go(stepRole); });
  }
  function stepRole() {
    screen(`${head(3, "Rôle de cette connexion")}<div class="conn-cards">
      <button type="button" class="conn-card" data-r="atelier"><span class="cc-ico">🛠️</span><span class="cc-name">Atelier</span><span class="cc-sub">votre terrain de dev (autonome)</span></button>
      <button type="button" class="conn-card" data-r="production"><span class="cc-ico cc-vitrine">🌐</span><span class="cc-name">Vitrine</span><span class="cc-sub">production — tout sous PIN</span></button>
    </div>`);
    buttons("", null);
    el("modal-body").querySelectorAll(".conn-card").forEach((b) => b.onclick = () => { W.role = b.dataset.r; go(stepCreds); });
  }
  function stepCreds() {
    if (W.provider === "github") {
      screen(`${head(4, "Jeton d'accès GitHub")}<div class="field">
        <label>Jeton d'accès personnel (PAT)<input id="c-gtoken" type="password" placeholder="ghp_… ou github_pat_…" /></label>
        <small>Collez un jeton GitHub (accès <b>repo</b> / Contents). Chiffré par votre PIN.</small>
        ${tokenHelp("github")}
        <div class="inline-error hidden" id="c-err"></div></div>`);
      buttons("Créer la connexion", () => doCreate({ token: el("c-gtoken").value.trim() }));
      return;
    }
    screen(`${head(4, "Connexion au serveur")}<div class="field">
      <label>Adresse du serveur<input id="c-url" class="inp" placeholder="https://git.apcom.app" /></label>
      <label>Identifiant<input id="c-user" class="inp" /></label>
      <label>Mot de passe<input id="c-pass" type="password" /></label>
      <label>Code 2FA <span class="muted">(laisser vide si aucun)</span></label><div id="c-otp" class="git-code"></div>
      <label style="flex-direction:row;align-items:center"><input id="c-remember" type="checkbox" checked style="width:auto;margin-right:8px" />Mémoriser le mot de passe (chiffré par le PIN)</label>
      <label style="flex-direction:row;align-items:center"><input id="c-admin" type="checkbox" style="width:auto;margin-right:8px" />Compte administrateur</label>
      ${tokenHelp("forgejo")}
      <div class="inline-error hidden" id="c-err"></div></div>`);
    const ciOtp = codeInput(el("c-otp"), false, null);
    buttons("Créer la connexion", () => doCreate({
      url: el("c-url").value.trim(), user: el("c-user").value.trim(), pass: el("c-pass").value,
      otp: ciOtp.read(), admin: !!(el("c-admin") && el("c-admin").checked), remember: !!(el("c-remember") && el("c-remember").checked),
    }));
  }
  async function doCreate(dd) {
    const er = el("c-err"); if (er) er.classList.add("hidden");
    okBtn.disabled = true;
    try {
      if (W.provider === "github") await App().AddInstanceToken(W.name, "github", "", dd.token);
      else await App().AddInstanceLogin(W.name, dd.url, dd.user, dd.pass, dd.otp, dd.admin, dd.remember);
      try { await App().SetInstanceRole(W.name, W.role); } catch (_) {}
      stepDone();
    } catch (e) { okBtn.disabled = false; if (er) { er.textContent = cleanErr(e); er.classList.remove("hidden"); } }
  }
  function stepDone() {
    hist.length = 0;
    screen(`<div class="conn-done"><div class="cd-ico">✅</div><h3 id="modal-title">Connexion active</h3>
      <p class="sub">« <b>${esc(W.name)}</b> » — ${W.provider === "github" ? "GitHub" : "Forgejo / Gitea"} · <span class="role-badge ${W.role === "production" ? "role-production" : "role-atelier"}">${W.role === "production" ? "VITRINE" : "ATELIER"}</span> est prête.</p></div>`);
    okBtn.classList.remove("hidden"); okBtn.disabled = false; okBtn.textContent = "Terminer"; okBtn.onclick = () => location.reload();
    cancelBtn.classList.add("hidden");
  }
  go(stepName);
}

// reconnectModal : recrée le token d'une instance existante (passer en admin, mémoriser le
// mot de passe) — une seule saisie, puis Admin et Tokens marchent au PIN seul.
function reconnectModal(name, opts) {
  opts = opts || {};
  const inst = (state.instances || []).find((i) => i.name === name) || {};
  // GitHub : reconnexion = coller un nouveau jeton d'accès personnel.
  if (inst.provider === "github") {
    openModal("Reconnecter « " + name + " »", `<div class="field">
      <p class="sub">Collez un nouveau jeton d'accès personnel GitHub pour rétablir l'accès.</p>
      ${forgejoBlock(`<label>Jeton d'accès personnel (PAT)<input id="rc-gtoken" type="password" placeholder="ghp_… ou github_pat_…" /></label>
        <small>Collez un nouveau jeton GitHub (accès <b>repo</b> / Contents).</small>
        ${tokenHelp("github")}`, "https://github.com", "github")}
      <div class="inline-error hidden" id="rc-err"></div></div>`, async () => {
      const er = el("rc-err");
      try {
        await App().ReconnectToken(name, el("rc-gtoken").value.trim());
        closeModal(); toast("Compte reconnecté", "ok"); location.reload();
      } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
    });
    return;
  }
  let ciOtp;
  const title = opts.admin ? "Activer l'administration" : "Reconnecter « " + name + " »";
  const intro = opts.admin
    ? "SituLab va recréer votre accès à « " + esc(name) + " » en y ajoutant le droit de gestion des comptes. Confirmez vos identifiants Forgejo puis votre PIN."
    : "Recrée l'accès de SituLab pour ce serveur (activer l'Administration, mémoriser votre mot de passe, ou reconnecter après un changement de PIN). À faire une seule fois.";
  openModal(title, `<div class="field">
    <p class="sub">${intro}</p>
    ${forgejoBlock(`
      <label>Identifiant<input id="rc-user" value="${esc(state.login || "")}" /></label>
      <label>Mot de passe<input id="rc-pass" type="password" /></label>
      <label>Code 2FA <span class="muted">(laisser vide si aucun)</span></label><div id="rc-otp" class="git-code"></div>
      <label style="flex-direction:row;align-items:center"><input id="rc-admin" type="checkbox" ${opts.admin ? "checked" : ""} style="width:auto;margin-right:8px" />Compte administrateur (gérer les comptes)</label>
      <label style="flex-direction:row;align-items:center"><input id="rc-remember" type="checkbox" ${inst.remember ? "checked" : ""} style="width:auto;margin-right:8px" />Mémoriser mon mot de passe (chiffré par le PIN)</label>`, inst.url)}
    ${tokenHelp("forgejo")}
    <div class="inline-error hidden" id="rc-err"></div></div>`, async () => {
    const er = el("rc-err");
    try {
      await App().ReconnectInstance(name, el("rc-user").value.trim(), el("rc-pass").value, ciOtp.read(), !!el("rc-admin").checked, !!el("rc-remember").checked);
      closeModal(); toast("Compte reconnecté", "ok"); location.reload();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  ciOtp = codeInput(el("rc-otp"), false, null);
}

// purgeDataModal : effacement total des données locales (v1.4.1). Confirmation par saisie, puis
// vide aussi le localStorage de la fenêtre (thème, mode avancé) et repart sur l'onboarding.
function purgeDataModal() {
  openModal("Effacer mes données de ce poste", `<div class="field">
    <div class="callout warn"><b>Action irréversible.</b> Vos connexions, votre trousseau chiffré (jetons), vos réglages et les jetons du coffre Windows seront supprimés de cette machine. Vos dépôts et fichiers déjà clonés sur le poste ne sont pas touchés.</div>
    <p class="sub">Le cache d'affichage (navigateur embarqué) étant utilisé par la fenêtre, il ne sera entièrement vidé qu'au prochain lancement — ou via la désinstallation avancée. Les éléments sensibles (jetons) sont, eux, effacés immédiatement.</p>
    <label>Pour confirmer, tapez <b>EFFACER</b><input id="pg-confirm" placeholder="EFFACER" autocomplete="off" /></label>
    <div class="inline-error hidden" id="pg-error"></div></div>`, async () => {
    const er = el("pg-error");
    if (el("pg-confirm").value.trim().toUpperCase() !== "EFFACER") {
      er.textContent = "Tapez EFFACER pour confirmer."; er.classList.remove("hidden"); return;
    }
    try {
      await App().Purge();
      try { localStorage.clear(); } catch (e) {}
      closeModal(); toast("Données effacées — retour au départ", "ok");
      setTimeout(() => location.reload(), 600);
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  el("modal-ok").textContent = "Effacer définitivement"; el("modal-ok").classList.add("danger-btn");
}

async function setPinModal() {
  const hasPin = await App().PinSet().catch(() => false);
  let ci0, ci1, ci2;
  openModal(hasPin ? "Changer le PIN" : "Créer votre PIN", `<div class="field">
    <p class="sub">${hasPin ? "Saisissez votre PIN actuel, puis choisissez le nouveau (6 chiffres)." : "Choisissez un PIN de 6 chiffres. Il déverrouille vos accès et confirme vos envois. Retenez-le : il ne peut pas être récupéré."}</p>
    ${hasPin ? `<label>PIN actuel</label><div id="p0"></div>` : ""}
    <label>${hasPin ? "Nouveau PIN" : "Votre PIN"} (6 chiffres)</label><div id="p1"></div>
    <label>Confirmer</label><div id="p2"></div>
    <div class="inline-error hidden" id="p-error"></div></div>`, async () => {
    const er = el("p-error");
    const a = ci1.read(), b = ci2.read(), old = hasPin ? ci0.read() : "";
    if (a.length < CODELEN) { er.textContent = "Le PIN doit faire 6 chiffres."; er.classList.remove("hidden"); return; }
    if (a !== b) { er.textContent = "Les PIN ne correspondent pas."; er.classList.remove("hidden"); return; }
    try { await App().SetPin(old, a); closeModal(); toast("PIN enregistré", "ok"); renderSettings(); }
    catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); }
  });
  if (hasPin) ci0 = codeInput(el("p0"), true, null, true);
  ci1 = codeInput(el("p1"), true, null, !hasPin);
  ci2 = codeInput(el("p2"), true, () => modalOk()); // valide auto quand la confirmation est complète
}

// pinRequestModal : fenêtre de saisie du PIN déclenchée par le démon (demande d'un
// client CLI/agent). Valider renvoie le PIN au démon ; annuler/fond refuse l'opération.
// pushPreviewHTML : rend l'aperçu éclairé d'un envoi (v1.7) — branche, commits, fichiers, et
// surtout les NOUVEAUX fichiers non suivis mis en évidence. Transforme « confirmer à l'aveugle »
// en consentement réel. Vide si pas d'aperçu (actions non liées à un envoi).
function pushPreviewHTML(d) {
  if (!d || typeof d !== "object") return "";
  const list = (arr, n) => (arr || []).slice(0, n).map((x) => `<li>${esc(x)}</li>`).join("");
  const more = (arr, n) => ((arr || []).length > n ? `<li class="muted">… +${(arr || []).length - n}</li>` : "");
  const commits = (d.commits || []);
  const untr = (d.untracked || []);
  const chg = (d.changed || []);
  let h = `<div class="push-preview">`;
  h += `<div class="pp-head">À publier${d.branch ? " sur <b>" + esc(d.branch) + "</b>" : ""}${d.remote ? ` <span class="muted">→ ${esc(d.remote)}</span>` : ""}</div>`;
  if (commits.length) h += `<div class="pp-sec"><div class="pp-t">${commits.length} version(s) à envoyer</div><ul>${list(commits, 8)}${more(commits, 8)}</ul></div>`;
  if (chg.length) h += `<div class="pp-sec"><div class="pp-t">Fichiers modifiés (${chg.length})</div><ul>${list(chg, 8)}${more(chg, 8)}</ul></div>`;
  if (untr.length) h += `<div class="pp-sec pp-new"><div class="pp-t">⚠ Nouveaux fichiers non suivis (${untr.length}) — vérifiez qu'aucun n'est sensible</div><ul>${list(untr, 10)}${more(untr, 10)}</ul></div>`;
  if (!commits.length && !chg.length && !untr.length) h += `<div class="muted">Rien de nouveau à envoyer détecté.</div>`;
  h += `</div>`;
  return h;
}

// --- Rôle & politique PIN par connexion (v1.7) ---
// Les 4 classes d'action soumises à politique, et leur libellé non-dev.
const PIN_ACTIONS = [
  { key: "push", label: "Envoi de code (push / synchroniser)" },
  { key: "issues", label: "Issues, demandes de fusion, commentaires" },
  { key: "repo_admin", label: "Administration des dépôts (créer, supprimer, renommer, versions…)" },
  { key: "user_admin", label: "Administration des comptes et des jetons" },
];
// defaultPIN reflète fidèlement le défaut Go (config.defaultRequiresPIN).
function defaultPIN(role, act) {
  if (role === "production") return true;
  return act === "repo_admin" || act === "user_admin";
}
// effPIN : la valeur effective (politique explicite si définie, sinon défaut du rôle).
function effPIN(role, policy, act) {
  const v = policy ? policy[act] : undefined;
  return v === true || v === false ? v : defaultPIN(role, act);
}

function connPolicyModal(name) {
  const inst = (state._settingsInstances || []).find((i) => i.name === name) || { name, role: "atelier" };
  let role = inst.role === "production" ? "production" : "atelier";
  let policy = inst.pin_policy || null;
  let signMode = inst.sign_mode === "server" ? "server" : "local";
  let signURL = inst.sign_server_url || "";

  const rows = () => PIN_ACTIONS.map((a) =>
    `<label class="set-row toggle"><div class="set-row-l">${a.label}<small>activé = votre PIN est demandé ; désactivé = autorisé dès que SituLab est déverrouillé</small></div>
     <span class="switch"><input type="checkbox" data-act="${a.key}" ${effPIN(role, policy, a.key) ? "checked" : ""} /><span class="track"></span></span></label>`).join("");

  const body = () => `<div class="field">
    <div class="set-row"><div class="set-row-l">Rôle de la connexion</div>
      <div class="set-row-c"><div class="seg" role="group" aria-label="Rôle" id="role-seg">
        <button data-role="atelier" class="${role === "atelier" ? "on" : ""}">Atelier</button>
        <button data-role="production" class="${role === "production" ? "on" : ""}">Production</button>
      </div></div></div>
    <div class="set-sec-d" id="role-help">${role === "production"
      ? "Vitrine protégée : par défaut tout est confirmé par votre PIN. On n'y publie que des versions validées."
      : "Terrain de travail : votre IA peut y être autonome (envoi/issues sans PIN), l'administration reste sous PIN."}</div>
    <div class="seg" style="margin:8px 0" id="pol-presets">
      <button data-preset="atelier">Préréglage Atelier autonome</button>
      <button data-preset="production">Préréglage Production protégée</button>
    </div>
    <div id="pol-rows">${rows()}</div>
    <div class="set-row" style="margin-top:14px"><div class="set-row-l">Signature des commits<small>empreinte « SituLab approved »</small></div>
      <div class="set-row-c"><div class="seg" role="group" aria-label="Signature" id="sign-seg">
        <button data-sign="local" class="${signMode === "server" ? "" : "on"}">Logiciel</button>
        <button data-sign="server" class="${signMode === "server" ? "on" : ""}">Serveur</button>
      </div></div></div>
    <div id="sign-url-row" class="${signMode === "server" ? "" : "hidden"}"><label>URL du service de signature<input id="sign-url" class="inp" value="${esc(signURL)}" placeholder="https://situememoring.com" /></label></div>
    <div class="set-sec-d" id="sign-help">${signMode === "server"
      ? "Serveur : la clé de signature ne quitte jamais le serveur — un agent sur le poste ne peut rien forger (audit, rate-limit, révocation)."
      : "Logiciel : clé embarquée dans SituLab — marqueur d'authenticité « fait via SituLab » (extractible, limite assumée)."}</div>
  </div>`;

  openModal("🏷️ Rôle & PIN — " + esc(name), body(), async () => {
    const pol = {};
    let differs = false;
    document.querySelectorAll("#pol-rows input[data-act]").forEach((c) => {
      pol[c.dataset.act] = c.checked;
      if (c.checked !== defaultPIN(role, c.dataset.act)) differs = true;
    });
    try {
      await App().SetInstanceRole(name, role);
      // Si la politique correspond exactement aux défauts du rôle, on renvoie null (héritage) plutôt
      // qu'un figement explicite — ainsi un futur changement de rôle reste effectif.
      await App().SetInstancePINPolicy(name, differs ? pol : null);
      await App().SetInstanceSignMode(name, signMode, (el("sign-url") && signMode === "server") ? el("sign-url").value.trim() : "");
      await refreshInstances(); // met à jour immédiatement le badge du header et le sélecteur (pas de redémarrage)
      closeModal(); toast("Rôle, PIN & signature mis à jour", "ok"); renderSettings();
    } catch (e) { toast(cleanErr(e), "err"); }
  });
  el("modal-ok").textContent = "Enregistrer";

  // Interactions : changer le rôle re-sème les suggestions ; les préréglages remplissent les cases.
  const reseed = () => { el("pol-rows").innerHTML = rows(); };
  el("role-seg").onclick = (e) => {
    const b = e.target.closest("[data-role]"); if (!b) return;
    role = b.dataset.role; policy = null; // repart des défauts du nouveau rôle
    document.querySelectorAll("#role-seg button").forEach((x) => x.classList.toggle("on", x.dataset.role === role));
    el("role-help").textContent = role === "production"
      ? "Vitrine protégée : par défaut tout est confirmé par votre PIN. On n'y publie que des versions validées."
      : "Terrain de travail : votre IA peut y être autonome (envoi/issues sans PIN), l'administration reste sous PIN.";
    reseed();
  };
  el("pol-presets").onclick = (e) => {
    const b = e.target.closest("[data-preset]"); if (!b) return;
    policy = null; role = b.dataset.preset; // le préréglage fixe aussi le rôle cohérent
    document.querySelectorAll("#role-seg button").forEach((x) => x.classList.toggle("on", x.dataset.role === role));
    reseed();
  };
  el("sign-seg").onclick = (e) => {
    const b = e.target.closest("[data-sign]"); if (!b) return;
    signMode = b.dataset.sign;
    document.querySelectorAll("#sign-seg button").forEach((x) => x.classList.toggle("on", x.dataset.sign === signMode));
    el("sign-url-row").classList.toggle("hidden", signMode !== "server");
    el("sign-help").textContent = signMode === "server"
      ? "Serveur : la clé de signature ne quitte jamais le serveur — un agent sur le poste ne peut rien forger (audit, rate-limit, révocation)."
      : "Logiciel : clé embarquée dans SituLab — marqueur d'authenticité « fait via SituLab » (extractible, limite assumée).";
  };
}

// Visuel par type d'action de la fenetre PIN (Point 6) : couleur + icone + flux source->destination.
const PIN_ACTION_STYLE = {
  push:    { cls: "pa-push",    ico: "⬆",  label: "Envoi en ligne" },
  promote: { cls: "pa-promote", ico: "🚀", label: "Promotion vers la production" },
  delete:  { cls: "pa-delete",  ico: "🗑",  label: "Suppression" },
  create:  { cls: "pa-create",  ico: "✚",  label: "Création" },
  config:  { cls: "pa-config",  ico: "⚙",  label: "Réglage" },
  issue:   { cls: "pa-issue",   ico: "💬", label: "Issue / fusion" },
  admin:   { cls: "pa-admin",   ico: "🔑", label: "Administration" },
  unlock:  { cls: "pa-unlock",  ico: "🔓", label: "Déverrouillage" },
};
// pinFlowHTML : montre le SENS de l'action (ce poste -> en ligne, atelier -> production).
function pinFlowHTML(action, detail) {
  if (action === "push") {
    const dest = (detail && detail.remote) ? esc(detail.remote) : "en ligne";
    const br = (detail && detail.branch) ? ` <span class="muted">(${esc(detail.branch)})</span>` : "";
    return `<div class="pin-flow"><span class="pf-node pf-local">🖥 Ce poste${br}</span><span class="pf-arrow">→</span><span class="pf-node pf-atl">☁ ${dest}</span></div>`;
  }
  if (action === "promote") {
    return `<div class="pin-flow"><span class="pf-node pf-atl">Atelier</span><span class="pf-arrow">→</span><span class="pf-node pf-prod">Production</span></div>`;
  }
  return "";
}

function pinRequestModal(reason, detail, action, intent) {
  let ci;
  const st = PIN_ACTION_STYLE[action] || null;
  const isDel = action === "delete";
  const chip = st ? `<div class="pin-action ${st.cls}"><span class="pin-ico">${st.ico}</span><span>${esc(st.label)}</span></div>` : "";
  const intro = isDel
    ? `<b>Suppression demandée.</b> Cette action est <b>destructive</b> et irréversible.`
    : `<b>Un agent / une commande demande votre feu vert.</b>`;
  // Point 4 : l'intention LIBRE de l'agent (son « pourquoi »), à part et clairement attribuée — ce sont
  // les mots de l'agent, pas ceux de SituLab. Absente si l'agent n'a rien joint.
  const intentBlock = (intent && intent.trim())
    ? `<figure class="pin-intent"><figcaption class="pin-intent-h">💬 Ce que l'agent dit vouloir faire</figcaption><blockquote class="pin-intent-q">${esc(intent)}</blockquote></figure>`
    : "";
  openModal("🔐 Confirmation requise", `<div class="field">
    ${chip}
    <div class="callout ${isDel ? "danger" : "warn"}">${intro}<br>Elle ne se fera <b>que</b> si vous entrez votre PIN SituLab ci-dessous.</div>
    ${intentBlock}
    <div class="pin-constate"><div class="pin-constate-h">🔎 Ce que SituLab constate</div>
      <p class="tagline">${esc(reason)}</p>
      ${pinFlowHTML(action, detail)}
      ${pushPreviewHTML(detail)}</div>
    <label>Votre PIN SituLab</label><div id="pin-input"></div>
    <small>SituLab <b>constate</b> l'action réelle ci-dessus ; le message de l'agent est <b>déclaratif</b>. Votre PIN seul autorise — il ne quitte jamais SituLab. Si vous n'êtes pas à l'origine de cette demande, cliquez <b>Refuser</b>.</small></div>`, async () => {
    try { await App().SubmitPIN(ci ? ci.read() : ""); } catch (e) {}
    closeModal();
  });
  ci = codeInput(el("pin-input"), true, () => modalOk(), true); // validation auto dès 6 chiffres
  if (isDel && el("modal-ok")) el("modal-ok").classList.add("danger-btn"); // bouton de confirmation rouge pour une suppression
  // Refuser / fond / Échap : prévenir le démon tout de suite (sinon il attend 2 min).
  const refuse = () => { try { App().CancelPIN(); } catch (e) {} closeModal(); };
  modalCancel = refuse;
  el("modal-cancel").textContent = "Refuser";
  el("modal-cancel").onclick = refuse;
  el("modal").onclick = (ev) => { if (ev.target === el("modal")) refuse(); };
}

// Liens externes (vers l'instance Forgejo) : ouverts dans le navigateur du système.
document.addEventListener("click", (e) => {
  const a = e.target.closest && e.target.closest("[data-ext]");
  if (a) { e.preventDefault(); safeOpenURL(a.dataset.ext); }
});

// Robustesse focus (WebView2 ne focalise pas toujours l'input au lancement) : dès qu'un
// CHIFFRE est tapé alors qu'aucune case n'est active mais qu'un champ en cases est présent,
// on focalise la 1re case vide — l'utilisateur ouvre l'app et tape son PIN, sans clic.
document.addEventListener("keydown", (e) => {
  if (e.key && e.key.length === 1 && /\d/.test(e.key)) {
    const ae = document.activeElement;
    if (!ae || !ae.classList || !ae.classList.contains("code-box")) {
      const ci = document.querySelector(".code-input");
      if (ci) {
        const boxes = [...ci.querySelectorAll(".code-box")];
        const target = boxes.find((b) => !b.value) || boxes[0];
        if (target) {
          e.preventDefault();
          target.focus();
          target.value = e.key; // insère directement le chiffre (sinon la 1re frappe serait perdue)
          target.dispatchEvent(new Event("input", { bubbles: true })); // avance + vérifie la complétude
        }
      }
    }
  }
}, true);

/* ---------------- Clavier ---------------- */
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    const m = el("modal");
    if (m && !m.classList.contains("hidden")) { (modalCancel || closeModal)(); return; }
    if (state.view === "detail") navTo("repos");
    else if (state.view === "project") { state.view = "projects"; render(); }
  }
  if (e.key === "Enter" && !el("modal").classList.contains("hidden") && modalOk && e.target.tagName === "INPUT") { e.preventDefault(); modalOk(); }
  if (e.key === "Enter" || e.key === " ") {
    const c = e.target.closest && e.target.closest("[data-fn], .frow");
    if (c && (c.matches("[data-fn]") || c.matches(".frow"))) { e.preventDefault(); c.click(); }
  }
});

/* ---------------- Composants d'état ---------------- */
function blankslate(ico, title, text) {
  return `<div class="blankslate"><div class="bs-ico" aria-hidden="true">${ico}</div><div class="bs-title">${esc(title)}</div><div class="bs-text">${esc(text)}</div></div>`;
}
function surfaceError(title, msg, retry) {
  const id = "retry-" + Math.round(performance.now());
  setTimeout(() => { const b = el(id); if (b && retry) b.onclick = retry; }, 0);
  return `<div class="surface-error"><span class="se-ico" aria-hidden="true">⚠</span><div class="se-msg"><b>${esc(title)}</b><div class="sub">${esc(msg)}</div></div>${retry ? `<button class="btn btn-ghost btn-sm" id="${id}">Réessayer</button>` : ""}</div>`;
}
function skeletons(n) { return Array.from({ length: n }, () => `<div class="skeleton"></div>`).join(""); }

function toast(msg, kind) {
  const t = document.createElement("div");
  t.className = "toast " + (kind || "");
  t.textContent = msg;
  el("toasts").appendChild(t);
  setTimeout(() => { t.style.opacity = "0"; setTimeout(() => t.remove(), 250); }, 2800);
}

// Icône « œil » (afficher/masquer). Un seul glyphe ; l'état masqué ajoute la barre oblique.
const EYE_SVG = `<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" fill="none" stroke="currentColor" stroke-width="1.7"/><circle cx="12" cy="12" r="3" fill="none" stroke="currentColor" stroke-width="1.7"/><line class="eye-slash" x1="4" y1="20" x2="20" y2="4" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>`;

// addRevealToggle place une icône « œil » DANS chaque champ mot de passe (clic = afficher/masquer),
// sans texte. Saisir un mot de passe à l'aveugle est une cause fréquente d'échecs pour un non-initié.
// (Nom conservé pour les appelants ; comportement v1.4.2 : icône in-field, plus de case à cocher.)
function addRevealToggle(root) {
  if (!root) return;
  root.querySelectorAll('input[type="password"]:not(.code-box)').forEach((inp) => {
    if (inp.dataset.eye === "1") return; // déjà équipé
    inp.dataset.eye = "1";
    const wrap = document.createElement("span");
    wrap.className = "pw-wrap";
    inp.parentNode.insertBefore(wrap, inp);
    wrap.appendChild(inp);
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "pw-eye";
    btn.setAttribute("aria-label", "Afficher le mot de passe");
    btn.innerHTML = EYE_SVG;
    btn.onclick = () => {
      const show = inp.type === "password";
      inp.type = show ? "text" : "password";
      btn.classList.toggle("on", show);
      btn.setAttribute("aria-label", show ? "Masquer le mot de passe" : "Afficher le mot de passe");
    };
    wrap.appendChild(btn);
  });
}

/* ---------------- Thème ---------------- */
function readTheme() { try { return localStorage.getItem("situlab-theme") || "auto"; } catch (e) { return "auto"; } }
function writeTheme(v) { try { localStorage.setItem("situlab-theme", v); } catch (e) {} }
function applyTheme(v) { if (v === "auto") delete document.documentElement.dataset.theme; else document.documentElement.dataset.theme = v; }

/* ---------------- Markdown léger ---------------- */
function mdToHtml(md) {
  const lines = String(md).replace(/\r/g, "").split("\n");
  let html = "", inCode = false, inTable = false, listType = "";
  const closeList = () => { if (listType) { html += `</${listType}>`; listType = ""; } };
  const closeTable = () => { if (inTable) { html += "</tbody></table>"; inTable = false; } };
  const inline = (s) => esc(s).replace(/`([^`]+)`/g, "<code>$1</code>").replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>");
  for (let raw of lines) {
    if (raw.startsWith("```")) { closeList(); closeTable(); inCode = !inCode; html += inCode ? "<pre class='doc-code'>" : "</pre>"; continue; }
    if (inCode) { html += esc(raw) + "\n"; continue; }
    if (/^\s*\|.*\|\s*$/.test(raw)) {
      const cells = raw.trim().replace(/^\||\|$/g, "").split("|").map((c) => c.trim());
      if (cells.every((c) => /^-+$/.test(c))) continue;
      if (!inTable) { closeList(); html += "<table class='doc-table'><tbody>"; inTable = true; }
      html += "<tr>" + cells.map((c) => `<td>${inline(c)}</td>`).join("") + "</tr>"; continue;
    }
    closeTable();
    const h = raw.match(/^(#{1,4})\s+(.*)$/);
    if (h) { closeList(); html += `<h${h[1].length} class="doc-h">${inline(h[2])}</h${h[1].length}>`; continue; }
    if (/^\s*>\s?/.test(raw)) { closeList(); html += `<blockquote>${inline(raw.replace(/^\s*>\s?/, ""))}</blockquote>`; continue; }
    const li = raw.match(/^(\s*)([-*]|\d+\.)\s+(.*)$/);
    if (li) { const t = /\d/.test(li[2]) ? "ol" : "ul"; if (listType !== t) { closeList(); html += `<${t}>`; listType = t; } html += `<li>${inline(li[3])}</li>`; continue; }
    if (raw.trim() === "") { closeList(); continue; }
    closeList(); html += `<p>${inline(raw)}</p>`;
  }
  closeList(); closeTable(); if (inCode) html += "</pre>";
  return html;
}

/* ---------------- Utils ---------------- */
function shortPath(p) { if (!p) return ""; const parts = p.split(/[\\/]/).filter(Boolean); return parts.length <= 2 ? p : "…/" + parts.slice(-2).join("/"); }
function parentOf(p) { const i = p.lastIndexOf("/"); return i >= 0 ? p.slice(0, i) : ""; }
function fmtSize(n) { if (n == null) return ""; if (n < 1024) return n + " o"; if (n < 1048576) return (n / 1024).toFixed(0) + " Ko"; return (n / 1048576).toFixed(1) + " Mo"; }
// Repli SÛR : si la date distante n'est pas parsable, on ne réinjecte JAMAIS la
// chaîne brute (elle pourrait contenir du HTML) — on renvoie une chaîne vide.
// Format de date suisse jj.mm.aaaa (séparateur point), construit à la main pour un rendu stable.
const pad2 = (n) => String(n).padStart(2, "0");
// fmtDay : date seule « 09.10.2026 ».
function fmtDay(s) { if (!s) return ""; const d = new Date(s); return isNaN(d) ? "" : `${pad2(d.getDate())}.${pad2(d.getMonth() + 1)}.${d.getFullYear()}`; }
// fmtDate : date + heure « 09.10.2026 14:24 ».
function fmtDate(s) { if (!s) return ""; const d = new Date(s); return isNaN(d) ? "" : `${fmtDay(s)} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`; }
// fmtWhen : identique (jj.mm.aaaa hh:mm), pour les horodatages « maj » des cartes.
const fmtWhen = fmtDate;
