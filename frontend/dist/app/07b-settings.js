async function renderSettings() {
  const theme = readTheme();
  const zoom = readZoom();
  const mods = state.modules || {};
  const MODULE_GROUPS = [
    { k: "agents", label: "Accès IA (agents)", d: "Agent IA et configuration CLI/MCP." },
    { k: "git_advanced", label: "Git avancé", d: "Étapes git séparées, matrice des branches, outils git locaux, réglages de dépôt." },
    { k: "spokes", label: "SPOKES", d: "Le menu des projections de mission." },
    { k: "corpus", label: "CORPUS (clouds)", d: "Le menu des clouds de fichiers souverains." },
    { k: "backup", label: "BACKUP", d: "Le menu des sauvegardes git → cloud." },
  ];
  // « Agent IA » est sorti des Réglages (v1.17) → entrée du menu principal (voir renderAssistantIA).
  // « Aide » est entrée dans les Réglages (v1.17) → onglet ci-dessous, à la place de l'entrée de menu.
  const navItems = [["secu", "🔒 Compte & sécurité"], ["conn", "🔗 Connexions git"], ["cloud", "☁ Connexions cloud"], ["app", "🎨 Apparence"], ["modules", "🧩 Modules"],
    ["diag", "🩺 Diagnostic"], ["help", "❓ Aide"], ["about", "ℹ️ À propos"]];
  let setTab = "secu";
  try { setTab = localStorage.getItem("situlab.settab") || "secu"; } catch (_) {}
  // Garde-fou : un onglet mémorisé qui n'existe plus (ex. « osbox » retiré en 1.17) laisserait la page
  // vide → on retombe sur « secu ».
  if (!navItems.some(([k]) => k === setTab)) setTab = "secu";
  // Une pastille de santé (ou un lien) peut demander à ouvrir un onglet précis des Réglages.
  if (state.settabTarget) { setTab = state.settabTarget; state.settabTarget = null; }
  el("view").innerHTML = `
    <div class="view-head"><div class="view-title">Réglages</div></div>
    <div class="set-layout">
      <nav class="set-nav">${navItems.map(([k, l]) => `<button class="set-navbtn" data-settab="${k}">${l}</button>`).join("")}</nav>
      <div class="set-panels">

    <section class="set-sec set-panel" data-panel="secu"><div class="set-sec-h">🔒 Compte &amp; sécurité</div>
      <div class="set-sec-d">Votre PIN déverrouille vos accès (jetons chiffrés sur ce poste) et confirme chaque envoi vers le serveur. Personne ne peut lire vos accès sans lui.</div>
      <div class="callout warn hidden" id="pol-reval-row" style="margin-bottom:12px">
        <b>⚠ Politique de sécurité à re-valider.</b> La signature de ce poste n'est plus reconnue (mise à niveau, clé d'appareil renouvelée, ou configuration éditée hors SituLab). Tant qu'elle n'est pas re-validée, SituLab reste en <b>lecture seule</b>. Vérifiez vos réglages ci-dessous (rôles, périmètres, PIN), puis re-validez.
        <div style="margin-top:8px"><button class="btn btn-primary btn-sm" id="pol-reval">Re-valider la politique de sécurité</button></div>
      </div>
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
      <label class="set-row toggle"><div class="set-row-l">Mode lecture seule <small>consulter sans risque : bloque tout envoi, suppression et modification — git <b>et</b> cloud (CORPUS)</small></div>
        <span class="switch"><input id="read-only" type="checkbox" /><span class="track"></span></span></label>
    </section>

    <section class="set-sec set-panel" data-panel="conn"><div class="set-sec-h">🔗 Connexions git</div>
      <div class="set-sec-d">Vos serveurs git (Forgejo/Gitea, GitHub). ★ = connexion par défaut. Les <b>jetons</b> et les <b>utilisateurs</b> se gèrent par connexion, directement sur chaque ligne.</div>
      <div id="inst-list"></div>
      <div class="set-actions-row" style="margin-top:10px">
        <button class="btn btn-ghost btn-sm" id="add-inst">＋ Ajouter une connexion</button>
        <button class="btn btn-ghost btn-sm" id="go-git">⎇ Comprendre git</button>
      </div>
    </section>

    <section class="set-sec set-panel" data-panel="cloud"><div class="set-sec-h">☁ Connexions cloud</div>
      <div class="set-sec-d">Vos clouds de fichiers souverains (Nextcloud, kDrive) qui portent le <b>CORPUS</b>. Ici vous les ajoutez, réglez l'accès de l'IA et le retirez ; l'exploration des fichiers se fait dans l'onglet CORPUS.</div>
      <div id="sc-cloud-list"><div class="skeleton"></div></div>
      <button class="btn btn-ghost btn-sm" id="sc-add-cloud" style="margin-top:10px">＋ Connecter un cloud</button>
    </section>

    <section class="set-sec set-panel" data-panel="app"><div class="set-sec-h">🎨 Apparence &amp; affichage</div>
      <div class="set-row"><div class="set-row-l">Thème</div>
        <div class="set-row-c"><div class="seg" role="group" aria-label="Thème">
          <button data-th="auto" class="${theme === "auto" ? "on" : ""}">Auto</button>
          <button data-th="light" class="${theme === "light" ? "on" : ""}">Clair</button>
          <button data-th="dark" class="${theme === "dark" ? "on" : ""}">Sombre</button>
        </div></div></div>
      <div class="set-row"><div class="set-row-l">Densité <small>agrandit tout le logiciel d'un coup — texte, marges, boutons, icônes</small></div>
        <div class="set-row-c"><div class="seg" role="group" aria-label="Densité">
          <button data-zoom="compact" class="${zoom === "compact" ? "on" : ""}">Compact</button>
          <button data-zoom="normal" class="${zoom === "normal" ? "on" : ""}">Normal</button>
          <button data-zoom="grand" class="${zoom === "grand" ? "on" : ""}">Grand</button>
        </div></div></div>
      <div class="set-note">Le « mode avancé » est devenu le module <b>Git avancé</b> — réglez-le dans l'onglet <b>🧩 Modules</b>.</div>
    </section>

    <section class="set-sec set-panel" data-panel="modules"><div class="set-sec-h">🧩 Modules — ce que vous voyez</div>
      <div class="set-sec-d">Adaptez SituLab à votre usage. <b>Tout est activé par défaut</b> : réduisez pour une interface simple (utilisateur porté), gardez tout pour le plein contrôle (développeur).</div>
      <div class="seg" style="margin:4px 0 12px" role="group" aria-label="Préréglage" id="mod-presets">
        <button data-mpreset="essentiel">Essentiel</button>
        <button data-mpreset="complet">Complet</button>
      </div>
      <div id="mod-rows">${MODULE_GROUPS.map((g) => `<label class="set-row toggle"><div class="set-row-l">${g.label}<small>${g.d}</small></div>
        <span class="switch"><input type="checkbox" data-mod="${g.k}" ${mods[g.k] !== false ? "checked" : ""} /><span class="track"></span></span></label>`).join("")}</div>
    </section>

    <section class="set-sec set-panel" data-panel="help"><div class="set-sec-h">❓ Aide — le guide SituLab</div>
      <div class="set-sec-d">Toute la documentation, pour vous comme pour vos agents. Vous pouvez aussi rejouer la visite guidée.</div>
      <div class="set-actions-row" style="margin-top:0;border-top:none;padding-top:0">
        <button class="btn btn-ghost btn-sm" id="help-replay-tour">↺ Rejouer la visite guidée</button>
      </div>
      <div class="doc" id="set-doc" style="margin-top:12px"><div class="skeleton"></div></div>
    </section>

    <section class="set-sec set-panel" data-panel="diag"><div class="set-sec-h">🩺 Diagnostic</div>
      <div class="set-sec-d">L'état de SituLab sur ce poste en un coup d'œil : git, PIN, session, connexions, coffre, signature.</div>
      <div id="diag-creds"><div class="muted">Vérification…</div></div>
    </section>

    <section class="set-sec set-panel" data-panel="about"><div class="set-sec-h">ℹ️ À propos</div>
      <div class="set-sub-h">Version</div>
      <div class="set-row"><div class="set-row-l">Version installée <b id="app-ver">…</b></div>
        <div class="set-row-c"><button class="btn btn-ghost btn-sm" id="ver-check">Vérifier les mises à jour ↗</button></div></div>
      <div class="set-row"><div class="set-row-l">Téléchargements (votre OS)</div>
        <div class="set-row-c"><button class="btn btn-ghost btn-sm" id="ver-site">situememoring.com ↗</button><button class="btn btn-ghost btn-sm" id="ver-gh">GitHub ↗</button></div></div>
      <div class="set-sub-h" style="margin-top:16px">Licence</div>
      <div class="set-sec-d">SituLab — logiciel libre sous licence <b>Apache-2.0</b>. © 2026 <b>APCOM Solutions SA</b>. La marque « SituLab » n'est pas couverte par la licence.</div>
      <div class="set-row-c" style="justify-content:flex-start"><button class="btn btn-ghost btn-sm" id="lic-show">Texte de la licence</button><button class="btn btn-ghost btn-sm" id="notice-show">NOTICE</button></div>
    </section>

    <section class="set-sec danger-zone set-panel" data-panel="secu"><div class="dz-h">⚠ Zone sensible — effacer mes données de ce poste</div>
      <p class="sub">Supprime définitivement, sur cette machine, <b>tout</b> ce que SituLab y garde : connexions git <b>et</b> cloud, trousseau chiffré par le PIN, réglages, et votre classement (KERNEL, spokes, tags, appairages, fichiers d'ouverture). Pour repartir à zéro ou céder le poste. <b>Irréversible.</b> Vos dépôts et fichiers sur le disque ne sont pas touchés.</p>
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
    // Chargement paresseux de l'Aide (le guide AGENTS.md) : seulement quand l'onglet est ouvert.
    if (k === "help" && el("set-doc") && !el("set-doc").dataset.loaded) {
      el("set-doc").dataset.loaded = "1";
      App().Doc().then((d) => { const n = el("set-doc"); if (n) n.innerHTML = (typeof mdToHtml === "function" ? mdToHtml : mdToHTML)(d); })
        .catch((e) => { const n = el("set-doc"); if (n) n.innerHTML = surfaceError("Documentation indisponible", cleanErr(e), null); });
    }
  };
  document.querySelectorAll(".set-navbtn").forEach((b) => b.onclick = () => showTab(b.dataset.settab));
  showTab(setTab);

  // « Comprendre git » reste un accès primaire ; jetons et utilisateurs sont PAR connexion (ci-dessous).
  if (el("go-git")) el("go-git").onclick = () => navTo("git");
  // Chapitre « Connexions cloud » : liste + admin (réutilise les tunnels CORPUS avec retour Réglages).
  renderSettingsClouds();
  if (el("sc-add-cloud")) el("sc-add-cloud").onclick = () => addCloudModal(renderSettings);
  // Onglet Aide : rejouer la visite guidée (le guide se charge paresseusement via showTab).
  if (el("help-replay-tour")) el("help-replay-tour").onclick = () => { navTo("kernel"); setTimeout(() => startTour(true), 350); };

  document.querySelectorAll("[data-th]").forEach((b) => b.onclick = () => { writeTheme(b.dataset.th); applyTheme(b.dataset.th); renderSettings(); });
  document.querySelectorAll("[data-zoom]").forEach((b) => b.onclick = () => { writeZoom(b.dataset.zoom); applyZoom(b.dataset.zoom); renderSettings(); });
  // Onglet Modules : préréglages + bascules → persisté au cœur, appliqué immédiatement (menus + git avancé).
  const refreshMods = async () => { state.modules = await App().Modules().catch(() => ({})); if (typeof applyModules === "function") applyModules(); };
  document.querySelectorAll("#mod-rows input[data-mod]").forEach((c) => c.onchange = async () => {
    try { await App().SetModule(c.dataset.mod, c.checked); await refreshMods(); renderSettings(); }
    catch (e) { toast(cleanErr(e), "err"); }
  });
  document.querySelectorAll("#mod-presets [data-mpreset]").forEach((b) => b.onclick = async () => {
    const all = {}; MODULE_GROUPS.forEach((g) => all[g.k] = true);
    const target = b.dataset.mpreset === "essentiel" ? { ...all, agents: false, git_advanced: false, spokes: false } : all;
    try { await App().SetModules(target); await refreshMods(); toast("Préréglage appliqué", "ok"); renderSettings(); }
    catch (e) { toast(cleanErr(e), "err"); }
  });

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
      // Jetons : pour toute connexion. Utilisateurs : seulement Forgejo/Gitea (pas GitHub).
      const usrBtn = i.provider === "github" ? "" : `<button class="lnk" data-usr="${esc(i.name)}">utilisateurs</button>`;
      void pill;
      const statusPill = h === "auth" ? connStatus("ko", "Déconnectée") : h === "net" ? connStatus("warn", "Hors ligne") : connStatus("ok", "Connectée");
      return `<div class="conn-card${dc ? " conn-card-disc" : ""}">
        <div class="conn-left">${connLogo(i.provider)}
          <div class="conn-main"><div class="conn-id">${i.default ? "★ " : ""}<b>${esc(i.name)}</b> ${badge} ${statusPill}</div>
            <div class="conn-meta">${esc(i.url)}</div></div></div>
        <div class="conn-acts">${reconBtn}<button class="lnk" data-tok="${esc(i.name)}">jetons</button>${usrBtn}<button class="lnk" data-pol="${esc(i.name)}">rôle &amp; PIN</button>${i.default ? "" : `<button class="lnk" data-def="${esc(i.name)}">défaut</button>`}<button class="icon-del" data-del="${esc(i.name)}" title="Supprimer la connexion « ${esc(i.name)} »" aria-label="Supprimer la connexion « ${esc(i.name)} »">${ICO_TRASH}</button></div></div>`;
    }).join("");
    el("inst-list").onclick = async (ev) => {
      const def = ev.target.closest("[data-def]"), del = ev.target.closest("[data-del]"), recon = ev.target.closest("[data-recon]"), pol = ev.target.closest("[data-pol]");
      const tok = ev.target.closest("[data-tok]"), usr = ev.target.closest("[data-usr]");
      // Jetons / utilisateurs : on cible la connexion CLIQUÉE (plus de sélecteur global dans l'en-tête).
      if (tok) { state.instance = tok.dataset.tok; return navTo("tokens"); }
      if (usr) { state.instance = usr.dataset.usr; return navTo("admin"); }
      if (pol) return connPolicyModal(pol.dataset.pol);
      if (recon) return reconnectModal(recon.dataset.recon);
      if (def) { try { await App().SetDefaultInstance(def.dataset.def); toast("Instance par défaut mise à jour", "ok"); renderSettings(); } catch (e) { toast(cleanErr(e), "err"); } }
      if (del) {
        const nm = del.dataset.del;
        confirmDelete({
          title: `Supprimer la connexion « ${esc(nm)} » ?`,
          lead: `La connexion git <b>${esc(nm)}</b> et l'accès chiffré que SituLab garde pour ce serveur seront retirés de ce poste.`,
          items: ["La connexion et son jeton chiffré (trousseau + coffre OS)"],
          safe: ["Vos dépôts et fichiers déjà clonés sur le poste restent intacts", "Le serveur git distant n'est pas touché"],
          confirmLabel: "Supprimer la connexion",
          onConfirm: async () => { await App().RemoveInstance(nm); toast("Connexion supprimée", "ok"); location.reload(); },
        });
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
      // Fail-closed (besoin #5) : on reflète l'état AUTORITAIRE du cœur (pas le cache d'interface), lu à
      // frais à l'ouverture des Réglages ; en cas de doute (erreur de lecture) on affiche le côté SÛR —
      // lecture seule cochée — plutôt que de laisser croire, à tort, que les écritures sont ouvertes.
      let roState = !!state.readOnly;
      try { roState = await App().ReadOnly(); } catch (e) { roState = true; }
      state.readOnly = roState;
      ro.checked = roState;
      ro.onchange = async () => {
        try {
          await App().SetReadOnly(ro.checked);
          state.readOnly = ro.checked;
          updateReadOnlyBanner();
          toast(ro.checked ? "Mode lecture seule activé" : "Mode lecture seule désactivé", "ok");
        } catch (e) { toast(cleanErr(e), "err"); ro.checked = !ro.checked; }
      };
    }
    // Politique à re-valider (mise à niveau / clé renouvelée / édition hors SituLab) : seul geste qui
    // sort de la position fermée. Sans ce bouton, l'utilisateur était piégé (lecture seule non décochable,
    // message « re-validez dans Réglages » sans cible). Le bouton re-signe la politique courante.
    try {
      const needs = await App().SecurityPolicyNeedsRevalidation();
      const prr = el("pol-reval-row");
      if (prr) prr.classList.toggle("hidden", !needs);
      const pb = el("pol-reval");
      if (pb) pb.onclick = async () => {
        try {
          await App().RevalidateSecurityPolicy();
          toast("Politique de sécurité re-validée", "ok");
          try { state.readOnly = await App().ReadOnly(); } catch (e) {}
          updateReadOnlyBanner();
          renderSettings(); // re-render : bannière retirée, lecture seule reflète l'état réel
        } catch (e) { toast(cleanErr(e), "err"); }
      };
    } catch (e) {}
    const pd = el("purge-data");
    if (pd) pd.onclick = purgeDataModal;
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
  let nClouds = 0; try { nClouds = ((await App().FileClouds()) || []).length; } catch (_) {}
  rows.push(row("info", "Connexions",
    `${doc.instances} serveur(s) git · ${nClouds} cloud(s) CORPUS configuré(s).`,
    `<span class="diag-links"><button class="lnk" data-gototab="conn">git ›</button><button class="lnk" data-gototab="cloud">cloud ›</button></span>`));
  rows.push(doc.vault_residual ? row("warn", "Coffre Windows", `${doc.vault_residual} jeton(s) encore dans l'ancien coffre OS — ils seront migrés automatiquement au prochain déverrouillage.`) : row("ok", "Coffre Windows", "Aucun jeton résiduel."));
  rows.push(doc.sign_test_root
    ? row("warn", "Signature de garantie", "Racine de TEST embarquée — la vraie racine de gouvernance n'est pas installée (les sceaux ne sont pas vérifiables réellement).")
    : row("ok", "Signature de garantie", "Automatique : chaque commit et chaque promotion créés via SituLab sont signés par le service APCOM (clé jamais sur le poste). Le sceau se lit dans l'historique."));
  dc.innerHTML = `<div class="diag-list">${rows.join("")}</div>`;
  // Liens « Connexions » → onglet correspondant des Réglages (navigation contextuelle).
  dc.querySelectorAll("[data-gototab]").forEach((b) => b.onclick = () => { const t = document.querySelector(`.set-navbtn[data-settab="${b.dataset.gototab}"]`); if (t) t.click(); });
}

// renderSettingsClouds : liste et administre les connexions cloud dans les Réglages (1.11). La vue
// d'exploration des fichiers reste dans CORPUS ; ici on ajoute, règle l'accès IA/PIN et retire.
async function renderSettingsClouds() {
  const box = el("sc-cloud-list"); if (!box) return;
  let clouds = [];
  try { clouds = (await App().FileClouds()) || []; } catch (e) { box.innerHTML = `<div class="callout warn">${esc(cleanErr(e))}</div>`; return; }
  state.corpusClouds = clouds;
  if (!clouds.length) { box.innerHTML = `<div class="muted">Aucun cloud connecté. Branchez un Nextcloud ou un kDrive pour porter votre CORPUS.</div>`; return; }
  // Santé best-effort (peut être indisponible si session verrouillée).
  let health = {};
  try { (await cloudHealthCached() || []).forEach((h) => { health[h.name] = h; }); } catch (_) {}
  box.innerHTML = clouds.map((fc) => {
    const h = health[fc.name];
    const statusPill = h ? (h.ok ? connStatus("ok", "Joignable") : connStatus("ko", "Injoignable")) : connStatus("idle", "À vérifier");
    const nd = (fc.allowed_dirs || []).length;
    return `<div class="conn-card">
      <div class="conn-left">${connLogo(fc.kind)}
        <div class="conn-main"><div class="conn-id"><b>${esc(fc.name)}</b> <span class="conn-tag">${esc(fc.kind)}</span> ${statusPill}</div>
          <div class="conn-meta">${esc(fc.url)} · ${nd} dossier(s) ouvert(s) à l'IA</div></div></div>
      <div class="conn-acts"><button class="lnk" data-ai="${esc(fc.name)}">accès IA</button><button class="lnk" data-pin="${esc(fc.name)}">PIN</button><button class="icon-del" data-del="${esc(fc.name)}" title="Supprimer le cloud « ${esc(fc.name)} »" aria-label="Supprimer le cloud « ${esc(fc.name)} »">${ICO_TRASH}</button></div></div>`;
  }).join("");
  box.onclick = (ev) => {
    const ai = ev.target.closest("[data-ai]"), pin = ev.target.closest("[data-pin]"), del = ev.target.closest("[data-del]");
    const fc = clouds.find((c) => c.name === (ai ? ai.dataset.ai : pin ? pin.dataset.pin : del ? del.dataset.del : ""));
    if (!fc) return;
    if (ai) return aiAccessTreeModal(fc, renderSettings);
    if (pin) return cloudPINModal(fc, renderSettings);
    if (del) return removeCloudFlow(fc, renderSettings);
  };
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

