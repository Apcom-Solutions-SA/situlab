/* ---------------- OS DE TRAVAIL SÉPARÉ (v1.15) — OPT-IN ----------------
 * Réglages ▸ OS de travail : faire travailler une IA dans une VM invitée jetable (WSL2), sans clés, sans
 * accès direct git/cloud. SituLab (hôte) reste la seule porte. DÉSACTIVÉ par défaut : rien n'est installé
 * tant que l'utilisateur n'a pas lancé l'installation (après un écran d'avertissement franc).
 */

// SOFTWARE_CARDS : catalogue déclaratif (ids = recettes côté cœur). Extensible sans retoucher l'app.
const OSBOX_CARDS = [
  { id: "claude-code", label: "Claude Code", d: "L'agent IA en CLI (Node.js est installé automatiquement avec). Il n'accède à vos git/clouds que via SituLab (PIN).", size: "~260 Mo" },
  { id: "situlab-bridge", label: "SituLab Bridge", d: "Relie la box à SituLab. L'agent tape « situlab-bridge connect » puis « situlab-bridge repo … » ; les actions git/cloud sont validées par PIN sur ce poste. Aucune clé dans la box.", size: "intégré" },
];

let osboxPollTimer = null;

async function renderOSBoxPanel() {
  const box = el("osbox-panel"); if (!box) return;
  let st;
  try { st = await App().OSBoxStatus(); } catch (e) { box.innerHTML = surfaceError("OS de travail indisponible", cleanErr(e), null); return; }

  // Désactivé (défaut) : explication + compatibilité + toggle.
  if (!st.enabled) {
    stopOsboxPoll();
    box.innerHTML = `
      <div class="set-sec-d">Faites travailler une IA (Claude Code…) dans un <b>système Linux séparé et jetable</b> — sans jamais l'installer sur votre machine ni lui confier vos clés. <b>SituLab reste sa seule porte</b> vers vos dépôts et vos clouds (avec PIN). L'IA ne voit aucun de vos fichiers hôte.</div>
      <div class="callout ${st.available ? "" : "warn"}">${st.available ? "✓ Votre poste est compatible (WSL2)." : "⚠ " + esc(st.available_hint || "Poste non compatible pour l'instant.")}</div>
      <label class="set-row toggle"><div class="set-row-l">Activer un OS de travail séparé <small>désactivé par défaut — aucune VM n'est créée tant que vous ne lancez pas l'installation</small></div>
        <span class="switch"><input id="osb-enable" type="checkbox" ${st.available ? "" : "disabled"} /><span class="track"></span></span></label>`;
    if (el("osb-enable")) el("osb-enable").onchange = async (e) => {
      try { await App().OSBoxSetEnabled(e.target.checked); renderOSBoxPanel(); } catch (err) { toast(cleanErr(err), "err"); }
    };
    return;
  }

  // Activé : machine à états.
  const stateChip = { none: ["Non installé", "muted"], preparing: ["Préparation…", "accent"], ready: ["Prêt", "ok"], stopped: ["Arrêté", "muted"], error: ["Erreur", "danger"] }[st.state] || ["—", "muted"];
  let actions = "", content = "";

  if (st.state === "preparing") {
    content = `
      <div class="osb-prep"><div class="spinner"></div><div><b>Installation en cours — ${esc(st.phase || "préparation…")}</b>
        <div class="sub">Ne redémarrez pas, ne mettez pas l'ordinateur en veille. De quelques minutes à ~1 h selon votre connexion.</div></div></div>
      ${osboxBarHTML(st)}
      ${osboxStepsHTML(st)}`;
    startOsboxPoll();
  } else if (st.state === "error") {
    stopOsboxPoll();
    content = `
      <div class="callout danger"><b>L'installation a échoué.</b> ${esc(st.last_err || "Erreur inconnue.")}</div>
      ${osboxStepsHTML(st)}
      <div class="sub" style="margin-top:6px">Vous pouvez relancer : SituLab reprend là où c'est possible (l'image déjà téléchargée est réutilisée).</div>`;
    actions = `<button class="btn btn-primary btn-sm" id="osb-retry">↻ Réessayer l'installation</button>`;
  } else if (!st.installed) {
    stopOsboxPoll();
    content = `<div class="set-sec-d">L'option est activée, mais la box n'est pas encore installée.</div>`;
    actions = `<button class="btn btn-primary btn-sm" id="osb-install">⬇ Installer l'OS de travail…</button>`;
  } else {
    stopOsboxPoll();
    // Installée : état + logiciels + actions.
    const cards = OSBOX_CARDS.map((c) => {
      const done = (st.software || []).includes(c.id);
      const canInstall = st.running && !st.busy;
      return `<div class="osb-card">
        <div class="osb-card-h">${esc(c.label)} ${done ? '<span class="osb-done">installé ✓</span>' : `<span class="muted">${esc(c.size)}</span>`}</div>
        <div class="osb-card-d">${esc(c.d)}</div>
        <div class="osb-card-act">${done ? `<button class="btn btn-ghost btn-sm" data-osterm="1">Ouvrir dans l'OS</button>` : `<button class="btn btn-primary btn-sm" data-osinstall="${esc(c.id)}" ${canInstall ? "" : "disabled"}>Installer</button>`}</div>
      </div>`;
    }).join("");
    // Bandeau de progression quand une opération (install logicielle) est en cours, ou résumé de la
    // dernière opération (succès/échec) sinon — pour que l'utilisateur sache toujours où ça en est.
    const lastFail = !st.busy && (st.steps || []).some((x) => x.status === "fail");
    const progBlock = st.busy
      ? `<div class="osb-prep"><div class="spinner"></div><div><b>${esc(st.phase || "Opération en cours…")}</b>
          <div class="sub">Cela peut prendre quelques minutes (téléchargement de paquets). Ne fermez pas SituLab.</div></div></div>
         ${osboxBarHTML(st)}${osboxStepsHTML(st)}`
      : lastFail
        ? `<div class="callout danger"><b>La dernière opération a échoué.</b> Vous pouvez réessayer via le bouton « Installer ».</div>${osboxStepsHTML(st)}`
        : "";
    const hasBridge = (st.software || []).includes("situlab-bridge");
    const bridgeBlock = hasBridge ? `
      <div class="osb-bridge">
        <div class="osb-bridge-h">🔗 SituLab Bridge <span class="osb-done">installé ✓</span></div>
        <div class="sub">Dans le terminal, l'agent tape <code>situlab-bridge connect</code> pour se relier, puis p.ex. <code>situlab-bridge repo list</code>. Si la liaison échoue, autorisez le pont dans le pare-feu Windows (une seule fois) :</div>
        <div class="osb-bridge-act">
          <button class="btn btn-ghost btn-sm" id="osb-fw" ${st.busy ? "disabled" : ""}>🛡 Autoriser dans le pare-feu (admin)</button>
          <button class="btn btn-ghost btn-sm" id="osb-bridge-test" ${st.busy || !st.running ? "disabled" : ""}>↻ Tester la liaison</button>
        </div>
      </div>` : "";
    content = `
      <div class="osb-state-row">État : <span class="pill ${stateChip[1]}-pill">${stateChip[0]}</span>
        ${st.running && !st.busy ? `<button class="btn btn-ghost btn-sm" data-osterm="1">⌨ Ouvrir la SafeRoom</button>` : ""}</div>
      ${progBlock}
      <div class="osb-sw-h">Logiciels</div>
      <div class="osb-cards">${cards}</div>
      ${bridgeBlock}
      <div class="sub" style="margin-top:6px">L'agent n'accède à vos git/clouds que via SituLab (validation PIN sur ce poste).</div>`;
    if (st.busy) startOsboxPoll(); else stopOsboxPoll();
    actions = st.busy
      ? ""
      : st.running
        ? `<button class="btn btn-ghost btn-sm" id="osb-stop">■ Arrêter</button><button class="btn btn-ghost btn-sm btn-danger" id="osb-reset">↻ Réinitialiser</button>`
        : `<button class="btn btn-primary btn-sm" id="osb-start">▶ Démarrer</button><button class="btn btn-ghost btn-sm btn-danger" id="osb-reset">↻ Réinitialiser</button>`;
  }

  box.innerHTML = `
    <div class="set-sec-d">OS de travail séparé <b>activé</b>. <button class="lnk" id="osb-disable">désactiver</button></div>
    ${content}
    <div class="osb-actions">${actions}</div>`;

  if (el("osb-disable")) el("osb-disable").onclick = async () => { try { await App().OSBoxSetEnabled(false); renderOSBoxPanel(); } catch (e) { toast(cleanErr(e), "err"); } };
  if (el("osb-install")) el("osb-install").onclick = osboxConfirmInstall;
  if (el("osb-retry")) el("osb-retry").onclick = async () => {
    try { await App().OSBoxPrepare(); renderOSBoxPanel(); startOsboxPoll(); } catch (e) { toast(cleanErr(e), "err"); }
  };
  if (el("osb-start")) el("osb-start").onclick = async () => { try { await App().OSBoxStart(); renderOSBoxPanel(); } catch (e) { toast(cleanErr(e), "err"); } };
  if (el("osb-stop")) el("osb-stop").onclick = async () => { try { await App().OSBoxStop(); renderOSBoxPanel(); } catch (e) { toast(cleanErr(e), "err"); } };
  if (el("osb-reset")) el("osb-reset").onclick = osboxConfirmReset;
  box.querySelectorAll("[data-osterm]").forEach((b) => b.onclick = () => navTo("osterm"));
  if (el("osb-fw")) el("osb-fw").onclick = async () => {
    try { await App().OSBoxBridgeAuthorizeFirewall(); toast("Autorisation pare-feu demandée (validez l'invite Windows), puis « Tester la liaison ».", "ok"); } catch (e) { toast(cleanErr(e), "err"); }
  };
  if (el("osb-bridge-test")) el("osb-bridge-test").onclick = async () => {
    try { await App().OSBoxBridgeInstall(); toast("Test de liaison en cours…", "ok"); startOsboxPoll(); setTimeout(renderOSBoxPanel, 300); } catch (e) { toast(cleanErr(e), "err"); }
  };
  if (typeof refreshOsboxNav === "function") refreshOsboxNav(); // maj entrée « Terminal » de la sidebar
  box.querySelectorAll("[data-osinstall]").forEach((b) => b.onclick = async () => {
    const id = b.dataset.osinstall;
    try {
      if (id === "situlab-bridge") await App().OSBoxBridgeInstall();
      else await App().OSBoxInstallSoftware(id);
      toast("Installation lancée dans la box…", "ok"); startOsboxPoll(); setTimeout(renderOSBoxPanel, 300);
    } catch (e) { toast(cleanErr(e), "err"); }
  });
}

// osboxConfirmInstall : écran d'avertissement FRANC (§8) avant de lancer l'installation.
function osboxConfirmInstall() {
  openModal("Activer l'OS de travail", `<div class="field">
    <p class="sub">Activer l'OS de travail va <b>télécharger et installer un système Linux (Ubuntu)</b> sur votre poste (<b>~1 à 2 Go</b>). Comptez de <b>quelques minutes à ~1 h</b> selon votre connexion.</p>
    <div class="callout"><b>Avant de lancer :</b><ul style="margin:6px 0 0 18px">
      <li>une connexion internet <b>stable</b> ;</li>
      <li><b>ne pas redémarrer ni mettre en veille</b> pendant l'installation ;</li>
      <li>un moment <b>calme</b> — l'installation tourne en arrière-plan mais ne doit pas être interrompue ;</li>
      <li>si vous êtes accompagné, la disponibilité d'un <b>technicien</b> pour configurer l'OS ensuite.</li>
    </ul></div></div>`, async () => {
    try { await App().OSBoxPrepare(); closeModal(); renderOSBoxPanel(); startOsboxPoll(); } catch (e) { toast(cleanErr(e), "err"); }
  });
  el("modal-ok").textContent = "Lancer l'installation";
  el("modal-cancel").textContent = "Plus tard";
}

function osboxConfirmReset() {
  openModal("Réinitialiser l'OS de travail ?", `<div class="field"><div class="callout warn"><b>La box et tout ce qu'elle contient seront supprimés.</b> Vous repartez d'une box propre à la prochaine installation. Vos dépôts et fichiers hôte ne sont pas touchés.</div></div>`, async () => {
    try { await App().OSBoxReset(); closeModal(); renderOSBoxPanel(); } catch (e) { toast(cleanErr(e), "err"); }
  });
  el("modal-ok").textContent = "Réinitialiser"; el("modal-ok").classList.add("danger-btn");
}

// osboxBarHTML : barre de progression réelle pendant le téléchargement (pct connu), sinon barre
// indéterminée (installation/isolation sans pourcentage).
function osboxBarHTML(st) {
  const p = typeof st.pct === "number" ? st.pct : -1;
  if (p >= 0) return `<div class="osb-bar"><div class="osb-bar-fill" style="width:${Math.min(100, p)}%"></div></div><div class="osb-bar-pct">${Math.min(100, p)} %</div>`;
  return `<div class="osb-bar indet"><div class="osb-bar-fill"></div></div>`;
}

// osboxStepsHTML : journal des étapes — ce qui fonctionne (✓), ce qui échoue (✗), ce qui tourne (…).
function osboxStepsHTML(st) {
  if (!st.steps || !st.steps.length) return "";
  const ico = { ok: '<span class="osb-step-ic ok">✓</span>', fail: '<span class="osb-step-ic fail">✗</span>', doing: '<span class="osb-step-ic doing"></span>' };
  const rows = st.steps.map((s) => `<li class="osb-step ${esc(s.status)}">${ico[s.status] || ico.doing}
    <span class="osb-step-lb">${esc(s.label)}</span>
    ${s.status === "fail" && s.detail ? `<div class="osb-step-dt">${esc(s.detail)}</div>` : ""}</li>`).join("");
  return `<ul class="osb-steps">${rows}</ul>`;
}

function startOsboxPoll() { if (osboxPollTimer) return; osboxPollTimer = setInterval(() => { if (state.view === "settings" || state.view === "osterm") renderOSBoxPanel(); else stopOsboxPoll(); }, 1500); }
function stopOsboxPoll() { if (osboxPollTimer) { clearInterval(osboxPollTimer); osboxPollTimer = null; } }

// --- Terminal multi-onglets de l'OS de travail (VRAI TTY via xterm.js + PTY) -------------------
// Chaque onglet = un VRAI pseudo-terminal (ConPTY) attaché à `wsl.exe -d SituLab-Box` côté Go. On y
// lance des programmes interactifs plein écran — dont `claude` en mode agent, pour travailler dans la
// box comme dans un vrai terminal. Les sessions PERSISTENT : quitter la vue ne tue pas le PTY (il vit
// côté hôte), on ré-attache l'instance xterm au retour. La box reste SANS clé (git/cloud via SituLab).

function osTermState() {
  if (!state.osTerm) state.osTerm = { sessions: [], active: null, seq: 0 };
  if (!state.osTerm.sessions.length) osTermNew();
  if (!state.osTerm.sessions.find((s) => s.id === state.osTerm.active)) state.osTerm.active = state.osTerm.sessions[0].id;
  return state.osTerm;
}

function osTermTheme() {
  return { background: "#0a0d12", foreground: "#e6ebf2", cursor: "#4fe0b5", cursorAccent: "#0a0d12",
    selectionBackground: "rgba(79,224,181,.25)", black: "#0a0d12", brightBlack: "#5b6472",
    green: "#4fe0b5", brightGreen: "#6ff0c8", cyan: "#49c5d6", blue: "#6aa6ff", red: "#ff6b6b", yellow: "#ffcf6a" };
}

function osTermNew(name) {
  const t = state.osTerm || (state.osTerm = { sessions: [], active: null, seq: 0 });
  t.seq++;
  const term = new window.Terminal({
    cursorBlink: true, fontFamily: "var(--font-mono), monospace", fontSize: 13, scrollback: 4000,
    theme: osTermTheme(), allowProposedApi: true,
  });
  const fit = new window.FitAddon.FitAddon(); term.loadAddon(fit);
  const wrap = document.createElement("div"); wrap.className = "ost-xterm";
  const s = { id: "t" + t.seq, name: name || ("session " + t.seq), term, fit, wrap, off: null, started: false, exited: false };
  t.sessions.push(s); t.active = s.id; return s;
}
function osTermActive() { const t = osTermState(); return t.sessions.find((s) => s.id === t.active); }

function b64ToBytes(b64) { const bin = atob(b64); const u = new Uint8Array(bin.length); for (let i = 0; i < bin.length; i++) u[i] = bin.charCodeAt(i); return u; }

function osTermShellHTML() {
  return `<div class="ost-shell ost-fill">
      <div class="ost-tabs" id="ost-tabs"></div>
      <div class="ost-termbox" id="ost-termbox"></div>
    </div>`;
}

async function renderOSTerm() {
  osTermState();
  if (!state._osTermResizeBound) {
    state._osTermResizeBound = true;
    window.addEventListener("resize", () => { if (state.view === "osterm" || state.osTermFull) osTermFitActive(); });
    window.addEventListener("keydown", (e) => { if (e.key === "Escape" && state.osTermFull) osTermExitFull(); });
  }
  if (state.osTermFull) { osTermEnterFull(); return; }
  const view = el("view");
  view.innerHTML = osTermShellHTML();
  osTermPaintTabs();
  osTermMountActive();
}

// Plein écran : le terminal recouvre header + menu SituLab (seul le cadre natif de la fenêtre reste).
function osTermEnterFull() {
  state.osTermFull = true;
  const view = el("view");
  if (view) view.innerHTML = `<div class="ost-fullhint muted">SafeRoom en plein écran · <button class="lnk" id="ost-exithint">réduire</button> (ou Échap)</div>`;
  if (el("ost-exithint")) el("ost-exithint").onclick = osTermExitFull;
  let ov = el("ost-full");
  if (!ov) { ov = document.createElement("div"); ov.id = "ost-full"; ov.className = "ost-full"; document.body.appendChild(ov); }
  ov.innerHTML = osTermShellHTML() + `<button class="ost-full-restore" id="ost-restore" title="Réduire le terminal (Échap)">⤡</button>`;
  if (el("ost-restore")) el("ost-restore").onclick = osTermExitFull;
  osTermPaintTabs();
  osTermMountActive();
}
function osTermExitFull() {
  state.osTermFull = false;
  const ov = el("ost-full"); if (ov) ov.remove();
  renderOSTerm();
}
function osTermToggleFull() { if (state.osTermFull) osTermExitFull(); else osTermEnterFull(); }

function osTermPaintTabs() {
  const t = osTermState(); const bar = el("ost-tabs"); if (!bar) return;
  bar.innerHTML = t.sessions.map((s) => `
    <div class="ost-tab ${s.id === t.active ? "active" : ""}" data-tab="${s.id}" title="double-clic pour renommer">
      <span class="ost-tab-dot ${s.exited ? "dead" : ""}"></span><span class="ost-tab-nm" data-nm="${s.id}">${esc(s.name)}</span>
      ${t.sessions.length > 1 ? `<button class="ost-tab-x" data-close="${s.id}" title="Fermer">×</button>` : ""}
    </div>`).join("")
    + `<button class="ost-tab-add" id="ost-add" title="Nouvel onglet">+</button>`
    + `<button class="ost-tab-full" id="ost-full-btn" title="${state.osTermFull ? "Réduire (Échap)" : "Plein écran (par-dessus les menus)"}">${state.osTermFull ? "⤡" : "⤢"}</button>`;
  bar.querySelectorAll("[data-tab]").forEach((e) => e.onclick = (ev) => {
    if (ev.target.dataset.close || ev.target.classList.contains("ost-tab-x")) return;
    t.active = e.dataset.tab; osTermPaintTabs(); osTermMountActive();
  });
  bar.querySelectorAll("[data-close]").forEach((e) => e.onclick = (ev) => { ev.stopPropagation(); osTermClose(e.dataset.close); });
  bar.querySelectorAll("[data-nm]").forEach((e) => e.ondblclick = () => osTermRename(e.dataset.nm, e));
  if (el("ost-add")) el("ost-add").onclick = () => { osTermNew(); osTermPaintTabs(); osTermMountActive(); };
  if (el("ost-full-btn")) el("ost-full-btn").onclick = osTermToggleFull;
}

// osTermMountActive : attache l'onglet actif au conteneur. Démarre le PTY au premier montage ; sinon
// ré-attache l'instance xterm existante (persistance en revenant sur la vue).
function osTermMountActive() {
  const s = osTermActive(); const box = el("ost-termbox"); if (!s || !box) return;
  box.innerHTML = ""; box.appendChild(s.wrap);
  // Reprise du focus clavier au moindre clic (ex. après un aller-retour vers le navigateur pour
  // la connexion claude, qui vole le focus à la webview).
  box.onmousedown = () => { const a = osTermActive(); if (a && a.started) setTimeout(() => { try { a.term.focus(); } catch (_) {} }, 0); };
  if (!s.started) {
    s.started = true;
    s.term.open(s.wrap);
    try { s.fit.fit(); } catch (_) {}
    const cols = s.term.cols || 80, rows = s.term.rows || 24;
    // Entrée clavier → PTY.
    s.term.onData((d) => { App().OSBoxPtyWrite(s.id, d).catch(() => {}); });
    s.term.onResize(({ cols, rows }) => { App().OSBoxPtyResize(s.id, cols, rows).catch(() => {}); });
    // Sortie PTY → terminal (base64 → octets, UTF-8 intact).
    if (window.runtime && window.runtime.EventsOn) {
      const offData = window.runtime.EventsOn("osterm:data:" + s.id, (b64) => { try { s.term.write(b64ToBytes(b64)); } catch (_) {} });
      const offExit = window.runtime.EventsOn("osterm:exit:" + s.id, () => { s.exited = true; try { s.term.write("\r\n\x1b[90m[session terminée — fermez l'onglet]\x1b[0m\r\n"); } catch (_) {} osTermPaintTabs(); });
      s.off = () => { try { offData && offData(); } catch (_) {} try { offExit && offExit(); } catch (_) {} };
    }
    App().OSBoxPtyStart(s.id, cols, rows).catch((e) => { try { s.term.write("\r\n\x1b[31m" + cleanErr(e) + "\x1b[0m\r\n"); } catch (_) {} });
  } else {
    try { s.fit.fit(); } catch (_) {}
  }
  setTimeout(() => { osTermFitActive(); try { s.term.focus(); } catch (_) {} }, 30);
}

function osTermFitActive() {
  const s = osTermActive(); if (!s || !s.started) return;
  try { s.fit.fit(); App().OSBoxPtyResize(s.id, s.term.cols, s.term.rows).catch(() => {}); } catch (_) {}
}

function osTermClose(id) {
  const t = osTermState();
  const i = t.sessions.findIndex((s) => s.id === id); if (i < 0) return;
  const s = t.sessions[i];
  App().OSBoxPtyClose(id).catch(() => {});
  try { s.off && s.off(); } catch (_) {}
  try { s.term.dispose(); } catch (_) {}
  t.sessions.splice(i, 1);
  if (!t.sessions.length) osTermNew();
  if (t.active === id) t.active = t.sessions[Math.max(0, i - 1)].id;
  osTermPaintTabs(); osTermMountActive();
}

// osTermRename : édition inline du nom de l'onglet (look geek, pas de prompt bloquant).
function osTermRename(id, span) {
  const t = osTermState(); const s = t.sessions.find((x) => x.id === id); if (!s) return;
  const inp = document.createElement("input");
  inp.className = "ost-tab-edit"; inp.value = s.name;
  span.replaceWith(inp); inp.focus(); inp.select();
  const commit = () => { const v = inp.value.trim(); if (v) s.name = v; osTermPaintTabs(); };
  inp.onkeydown = (e) => { if (e.key === "Enter") commit(); else if (e.key === "Escape") osTermPaintTabs(); };
  inp.onblur = commit;
}
