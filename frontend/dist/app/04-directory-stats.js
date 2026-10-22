/* ---------------- Annuaire consolidé multi-instances (v1.4) ---------------- */
async function renderDirectory() {
  if (!state.isAdmin) {
    el("view").innerHTML = `<div class="view-head"><div class="view-title">Annuaire</div></div>` +
      blankslate("☰", "Réservé aux administrateurs", "L'annuaire réunit les comptes de toutes vos instances Forgejo. Il faut un compte administrateur pour l'ouvrir.");
    return;
  }
  el("view").innerHTML = userMgmtHeader("directory") + `
    <div class="view-sub" style="margin:-6px 0 10px">Tous les comptes de vos instances Forgejo, réunis — recherche par nom ou e-mail sur toutes les instances à la fois.</div>
    <div class="toolbar"><input id="dir-search" type="text" placeholder="Rechercher une personne…" aria-label="Rechercher" value="${esc(state.dirSearch || "")}" /></div>
    <div id="dir-body"><div class="skeleton"></div></div>`;
  wireUserMgmtTabs();
  el("dir-search").oninput = (e) => { state.dirSearch = e.target.value; paintDirectory(); };
  try {
    state.directory = await App().ConsolidatedDirectory();
    paintDirectory();
  } catch (e) {
    if (isLocked(e)) return handleLock();
    el("dir-body").innerHTML = surfaceError("Impossible de charger l'annuaire", cleanErr(e), () => renderDirectory());
  }
}

const DIR_BADGE = { actif: "ver", suspendu: "warn", inactif: "muted", absent: "" };
function paintDirectory() {
  const d = state.directory;
  const body = el("dir-body");
  if (!d || !body) return;
  const okInst = (d.instances || []).filter((i) => i.ok);
  const ko = (d.instances || []).filter((i) => !i.ok);
  const q = (state.dirSearch || "").trim().toLowerCase();
  // e.search regroupe login + tous les e-mails/noms distincts (toutes instances) : la recherche
  // transverse trouve donc la personne même si son e-mail/nom diffère d'une instance à l'autre.
  const entries = (d.entries || []).filter((e) => !q || (e.search || e.login.toLowerCase()).includes(q));
  const koNote = ko.length
    ? `<div class="callout warn">Non interrogées : ${ko.map((i) => `${esc(i.name)} (${esc(i.error || "indisponible")})`).join(" · ")}</div>` : "";
  if (!okInst.length) {
    body.innerHTML = koNote + blankslate("☰", "Aucune instance interrogeable", "Aucune de vos instances Forgejo n'a pu être interrogée en administration. Reconnectez un compte administrateur.");
    return;
  }
  if (!entries.length) {
    body.innerHTML = koNote + blankslate("☺", "Aucun résultat", q ? "Aucun compte ne correspond à cette recherche." : "Aucun compte trouvé sur vos instances.");
    return;
  }
  const head = `<div class="dir-row dir-head"><div class="dir-who">Personne</div>${okInst.map((i) => `<div class="dir-cell">${esc(i.name)}</div>`).join("")}</div>`;
  const rows = entries.map((e) => {
    const cells = okInst.map((i) => {
      const st = e.presence[i.name] || "absent";
      return `<div class="dir-cell"><span class="pill ${DIR_BADGE[st] || ""}">${esc(st)}</span></div>`;
    }).join("");
    return `<div class="dir-row">
      <div class="dir-who"><b>${esc(e.login)}</b>${e.admin ? ' <span class="pill ver">admin</span>' : ""}<div class="muted">${esc(e.full_name || "")}${e.email ? " · " + esc(e.email) : ""}</div></div>
      ${cells}</div>`;
  }).join("");
  body.innerHTML = koNote + `<div class="dir-table">${head}${rows}</div>`;
}

function adminRow(u) {
  const st = userStatus(u);
  const self = u.login === state.login;
  const actions = self
    ? `<span class="muted" style="font-size:12px">vous</span>`
    : `<button class="btn btn-ghost btn-sm" data-act="detail" data-u="${esc(u.login)}">Droits</button>`
      + `<button class="btn btn-ghost btn-sm" data-act="susp" data-u="${esc(u.login)}" data-on="${u.prohibit_login ? "" : "1"}">${u.prohibit_login ? "Réactiver" : "Suspendre"}</button>`
      + `<button class="btn btn-ghost btn-sm btn-danger" data-act="del" data-u="${esc(u.login)}">Supprimer</button>`;
  return `<div class="srow">
    <div><b>${esc(u.login)}</b> <span class="pill ${u.is_admin ? "ver" : ""}">${u.is_admin ? "Administrateur" : "Standard"}</span>
      <span class="pill ${st.cls}">${st.k}</span> <span class="muted">${esc(u.email || "")}</span></div>
    <div class="srow-acts">${actions}</div></div>`;
}

function wireAdminRows() {
  document.querySelectorAll("#users [data-act]").forEach((b) => {
    const login = b.dataset.u;
    if (b.dataset.act === "detail") b.onclick = () => userDetail(login);
    else if (b.dataset.act === "susp") b.onclick = () => suspendUser(login, b.dataset.on === "1");
    else if (b.dataset.act === "del") b.onclick = () => deleteUser(login);
  });
}

function suspendUser(login, suspend) {
  const reason = suspend ? `Suspendre le compte « ${login} »` : `Réactiver le compte « ${login} »`;
  const intro = suspend
    ? `<div class="callout">La personne ne pourra plus se connecter. <b>Rien n'est effacé</b> : ses dépôts et son historique restent. Vous pourrez la réactiver à tout moment.</div>`
    : `<div class="callout">La personne pourra de nouveau se connecter et utiliser son compte.</div>`;
  openModal(suspend ? "Suspendre l'accès" : "Réactiver l'accès", `<div class="field">${intro}</div>`, async () => {
    pinConfirm(reason, async (pin) => {
      await App().AdminSuspendUser(state.instance, login, suspend, pin);
      closeModal();
      renderAdmin();
    });
  });
  el("modal-ok").textContent = "Continuer";
}

function deleteUser(login) {
  openModal("Supprimer définitivement", `<div class="field">
    <div class="callout warn"><b>Action irréversible.</b> Le compte « ${esc(login)} » sera supprimé de Forgejo. Pour simplement bloquer l'accès sans rien perdre, préférez <b>Suspendre</b>.</div>
    <label style="flex-direction:row;align-items:center"><input id="del-purge" type="checkbox" style="width:auto;margin-right:8px" />Supprimer aussi tous ses dépôts</label>
    <small>Sans la case, la suppression échoue si la personne possède encore des dépôts (sécurité Forgejo).</small>
    <label>Pour confirmer, tapez l'identifiant <b>${esc(login)}</b><input id="del-confirm" placeholder="${esc(login)}" /></label>
    <div class="inline-error hidden" id="del-error"></div></div>`, async () => {
    const er = el("del-error");
    if (el("del-confirm").value.trim() !== login) { er.textContent = "L'identifiant saisi ne correspond pas."; er.classList.remove("hidden"); return; }
    const purge = el("del-purge").checked;
    pinConfirm(`Supprimer définitivement « ${login} »`, async (pin) => {
      await App().AdminDeleteUser(state.instance, login, purge, pin);
      closeModal();
      renderAdmin();
    });
  });
  el("modal-ok").textContent = "Continuer";
}

function createUserModal() {
  openModal("Nouvel utilisateur", `<div class="field">
    <label>Identifiant<input id="uu" placeholder="ex. j.martin" /><small>Nom de connexion de la personne, sans espace.</small></label>
    <label>E-mail<input id="ue" type="email" placeholder="prenom.nom@exemple.fr" /></label>
    <label>Mot de passe initial<input id="up" type="password" /><small>À communiquer à la personne ; elle devra le changer à sa première connexion.</small></label>
    <label style="flex-direction:row;align-items:center"><input id="ua" type="checkbox" style="width:auto;margin-right:8px" />Donner les droits d'administrateur</label>
    <small>Sans la case, le compte est « Standard » : il utilise SituLab sans gérer les autres.</small>
    <div class="inline-error hidden" id="u-error"></div>
    ${aiInviteHTML()}</div>`, async () => {
    const er = el("u-error");
    const login = el("uu").value.trim(), email = el("ue").value.trim(), pwd = el("up").value, admin = el("ua").checked;
    if (!login || !email || !pwd) { er.textContent = "Identifiant, e-mail et mot de passe initial sont nécessaires."; er.classList.remove("hidden"); return; }
    pinConfirm(`Créer le compte « ${login} »`, async (pin) => {
      await App().AdminCreateUser(state.instance, login, email, pwd, admin, pin);
      closeModal();
      openModal("Compte créé", `<div class="field"><div class="callout">Transmettez à la personne son identifiant « ${esc(login)} » et le mot de passe provisoire que vous venez de saisir. <b>Elle devra le changer à sa première connexion.</b></div></div>`, () => { closeModal(); renderAdmin(); });
    });
  });
  el("modal-ok").textContent = "Continuer";
  wireAiInvite(promptUserCreate);
}


// infoModal : modale de simple lecture, un seul bouton « Fermer ».
function infoModal(title, html) {
  openModal(title, html, () => closeModal());
  el("modal-ok").textContent = "Fermer";
  el("modal-cancel").classList.add("hidden");
}

// VISIBILITY : qui peut voir le profil de la personne.
const VISIBILITY = { public: "profil public", limited: "profil visible des membres connectés", private: "profil privé" };

// droitsResume : phrases claires de ce que la personne peut faire et voir.
function droitsResume(u) {
  const out = [];
  if (u.is_admin) out.push("Peut tout gérer sur ce serveur : comptes, dépôts et réglages.");
  else out.push("Compte standard : utilise SituLab pour ses dépôts, sans gérer les autres.");
  if (u.restricted) out.push("Accès restreint : ne voit que les dépôts qui lui sont explicitement partagés.");
  if (u.prohibit_login) out.push("Connexion suspendue : ne peut pas se connecter en ce moment.");
  const m = u.max_repo_creation;
  if (m === 0) out.push("Ne peut pas créer de nouveaux dépôts.");
  else if (m > 0) out.push(`Peut créer jusqu'à ${m} dépôt${m > 1 ? "s" : ""}.`);
  else out.push("Création de dépôts selon le réglage du serveur.");
  out.push(VISIBILITY[u.visibility] || "profil public");
  return out;
}

// userDetail : fiche « Droits » — ce que la personne peut faire et voir, + ses dépôts.
function userDetail(login) {
  const u = (state.users || []).find((x) => x.login === login) || { login };
  const st = userStatus(u);
  infoModal(`Droits de « ${login} »`, `<div class="field">
    <div class="drow"><span class="pill ${u.is_admin ? "ver" : ""}">${u.is_admin ? "Administrateur" : "Standard"}</span>
      <span class="pill ${st.cls}">${st.k}</span> <span class="muted">${esc(u.email || "")}</span></div>
    <ul class="droits">${droitsResume(u).map((s) => `<li>${esc(s)}</li>`).join("")}</ul>
    <div class="meta-grid">
      <div><span class="muted">Créé le</span><br>${esc(fmtDate(u.created)) || "—"}</div>
      <div><span class="muted">Dernière connexion</span><br>${esc(fmtDate(u.last_login)) || "—"}</div>
    </div>
    <h4 class="sub-h">Dépôts accessibles</h4>
    <div class="muted" style="font-size:11.5px;margin-bottom:6px">Ce que la personne possède ou peut consulter, y compris via ses organisations et équipes.</div>
    <div id="ud-repos"><div class="skeleton"></div></div></div>`);
  App().AdminUserRepos(state.instance, u.id).then((repos) => {
    const box = el("ud-repos"); if (!box) return;
    box.innerHTML = (repos || []).length
      ? (repos || []).map((r) => `<div class="repo-mini"><b>${esc(r.full_name)}</b> ${r.private ? '<span class="lock">🔒</span>' : ""}<span class="pill sm">${r.private ? "privé" : "public"}</span></div>`).join("")
      : `<div class="muted">Aucun dépôt accessible.</div>`;
  }).catch((e) => { const box = el("ud-repos"); if (box) box.innerHTML = `<div class="muted">${esc(cleanErr(e))}</div>`; });
}

/* ---------------- Statistiques : analyse d'équipe (v1.5.0) ---------------- */
const statsCtx = { charts: [], report: null, metric: "commits", excluded: {}, progressOff: null };

function statsDestroyCharts() { statsCtx.charts.forEach((c) => { try { c.destroy(); } catch (e) {} }); statsCtx.charts = []; }

// statsPresetRange renvoie {since, until} (ISO) pour un préréglage de période.
function statsPresetRange(preset) {
  const now = new Date(), y = now.getFullYear(), m = now.getMonth(), d = now.getDate();
  const startOfWeek = (ref) => { const x = new Date(ref); const dow = (x.getDay() + 6) % 7; x.setDate(x.getDate() - dow); x.setHours(0, 0, 0, 0); return x; };
  let s, e;
  if (preset === "week") { s = startOfWeek(now); e = now; }
  else if (preset === "lastweek") { e = startOfWeek(now); s = new Date(e); s.setDate(s.getDate() - 7); }
  else if (preset === "month") { s = new Date(y, m, 1); e = now; }
  else if (preset === "lastmonth") { s = new Date(y, m - 1, 1); e = new Date(y, m, 1); }
  else return null;
  return { since: s.toISOString(), until: e.toISOString() };
}

async function renderStats() {
  if (!state.isAdmin) {
    el("view").innerHTML = `<div class="view-head"><div class="view-title">Statistiques</div></div>` +
      blankslate("▤", "Réservé aux administrateurs", "L'analyse d'équipe couvre l'activité de tous les comptes.");
    return;
  }
  statsDestroyCharts();
  el("view").innerHTML = userMgmtHeader("stats") + `
    <div class="view-sub" style="margin:-6px 0 10px">Qui a produit quoi, où et quand sur « ${esc(state.instance)} ». L'analyse interroge le serveur en arrière-plan ; le rapport est enregistré (« état au… ») et rejouable.</div>
    <div class="stats-config">
      <div class="sc-row"><span class="sc-lab">Période</span>
        <div class="seg" id="stats-presets" role="group">
          <button data-p="week">Semaine en cours</button><button data-p="lastweek">Semaine passée</button>
          <button data-p="month" class="on">Mois en cours</button><button data-p="lastmonth">Mois dernier</button>
          <button data-p="custom">Personnalisé</button>
        </div>
      </div>
      <div class="sc-row hidden" id="stats-custom"><span class="sc-lab">Du</span><input type="date" id="stats-from" /><span class="sc-lab">au</span><input type="date" id="stats-to" /></div>
      <div class="sc-row"><button class="btn btn-primary" id="stats-run">Lancer l'analyse</button>
        <span class="muted" id="stats-range-lbl"></span></div>
    </div>
    <div id="stats-progress" class="hidden"></div>
    <div id="stats-out"></div>
    <div class="blk" style="margin-top:18px"><div class="blk-h">Analyses enregistrées</div><div id="stats-history"><div class="muted">—</div></div></div>`;

  let preset = "month";
  const setPreset = (p) => {
    preset = p;
    document.querySelectorAll("#stats-presets button").forEach((b) => b.classList.toggle("on", b.dataset.p === p));
    el("stats-custom").classList.toggle("hidden", p !== "custom");
    const r = p === "custom" ? null : statsPresetRange(p);
    el("stats-range-lbl").textContent = r ? `du ${fmtDay(r.since)} au ${fmtDay(r.until)}` : "choisissez deux dates";
  };
  document.querySelectorAll("#stats-presets button").forEach((b) => b.onclick = () => setPreset(b.dataset.p));
  setPreset("month");
  wireUserMgmtTabs();
  el("stats-run").onclick = () => statsLaunch(preset);
  renderStatsHistory();
}

async function renderStatsHistory() {
  const box = el("stats-history"); if (!box) return;
  try {
    const list = await App().ListTeamReports();
    box.innerHTML = (list && list.length)
      ? list.map((r) => `<div class="srow"><div><b>État au ${esc(fmtDate(r.created_at))}</b> <span class="muted">· ${esc(fmtDay(r.since))} → ${esc(fmtDay(r.until))} · ${r.commits} commits · ${r.authors} personnes</span></div>
          <button class="btn btn-ghost btn-sm" data-rep="${esc(r.id)}">Ouvrir</button></div>`).join("")
      : `<div class="muted">Aucune analyse pour l'instant — lancez-en une ci-dessus.</div>`;
    box.querySelectorAll("[data-rep]").forEach((b) => b.onclick = async () => {
      try { const rep = await App().LoadTeamReport(b.dataset.rep); statsRenderReport(rep); window.scrollTo(0, 0); }
      catch (e) { toast(cleanErr(e), "err"); }
    });
  } catch (e) { box.innerHTML = `<div class="muted">${esc(cleanErr(e))}</div>`; }
}

async function statsLaunch(preset) {
  const range = preset === "custom"
    ? { since: el("stats-from").value ? new Date(el("stats-from").value + "T00:00:00").toISOString() : "", until: el("stats-to").value ? new Date(el("stats-to").value + "T23:59:59").toISOString() : "" }
    : statsPresetRange(preset);
  if (!range || !range.since || !range.until) { toast("Choisissez une période", "err"); return; }
  const prog = el("stats-progress"); el("stats-out").innerHTML = "";
  prog.classList.remove("hidden");
  prog.innerHTML = `<div class="stats-run-wait"><div class="spin"></div><div><b>Analyse en cours…</b><div class="muted" id="stats-prog-lbl">préparation…</div><div class="progbar"><span id="stats-prog-bar"></span></div></div></div>
    <div class="callout" style="margin-top:10px">⏳ Cette génération peut prendre <b>plusieurs minutes</b>. Vous pouvez <b>continuer à travailler</b> dans SituLab (changez de page sans souci) — le rapport apparaîtra dans « Analyses enregistrées » dès qu'il est prêt. <b>Ne fermez pas l'application</b> avant la fin (l'analyse s'arrêterait).</div>`;
  // Progression en arrière-plan (évènement du noyau).
  try { if (statsCtx.progressOff) statsCtx.progressOff(); } catch (e) {}
  try {
    statsCtx.progressOff = window.runtime.EventsOn("report:progress", (p) => {
      if (!p) return;
      const pct = p.total ? Math.round((p.done / p.total) * 100) : 0;
      const bar = el("stats-prog-bar"), lbl = el("stats-prog-lbl");
      if (bar) bar.style.width = pct + "%";
      if (lbl) lbl.textContent = p.repo ? `dépôt ${p.done}/${p.total} — ${p.repo}` : `${p.done}/${p.total} dépôts`;
    });
  } catch (e) {}
  const runBtn = el("stats-run"); if (runBtn) runBtn.disabled = true;
  try {
    const rep = await App().AnalyzeTeam(state.instance, [], range.since, range.until);
    // L'utilisateur a pu changer de page pendant l'analyse : le rapport est enregistré côté noyau
    // (il est dans l'historique). On ne met à jour l'affichage QUE si on est encore sur Statistiques.
    if (state.view !== "stats" || !el("stats-out")) return;
    prog.classList.add("hidden");
    statsCtx.excluded = {};
    statsRenderReport(rep);
    renderStatsHistory();
  } catch (e) {
    if (state.view !== "stats" || !el("stats-out")) return; // page quittée : l'erreur n'a plus de cible
    prog.classList.add("hidden");
    if (isLocked(e)) return handleLock();
    el("stats-out").innerHTML = surfaceError("Analyse impossible", cleanErr(e), null);
  } finally {
    const rb = el("stats-run"); if (rb) rb.disabled = false;
    try { if (statsCtx.progressOff) { statsCtx.progressOff(); statsCtx.progressOff = null; } } catch (e) {}
  }
}

function statsRenderReport(rep) {
  statsCtx.report = rep;
  statsCtx.excluded = {}; // repartir sans exclusions (y compris à l'ouverture depuis l'historique)
  statsDestroyCharts();
  const out = el("stats-out"); if (!out) return;
  const t = rep.totals || {};
  const kpis = `<div class="kpis">
    <div class="kpi"><div class="kpi-n">${t.commits || 0}</div><div class="kpi-l">commits</div></div>
    <div class="kpi"><div class="kpi-n" style="color:var(--ok)">+${(t.additions || 0).toLocaleString("fr-CH")}</div><div class="kpi-l">lignes ajoutées</div></div>
    <div class="kpi"><div class="kpi-n" style="color:var(--danger)">−${(t.deletions || 0).toLocaleString("fr-CH")}</div><div class="kpi-l">lignes supprimées</div></div>
    <div class="kpi"><div class="kpi-n">${t.authors || 0}</div><div class="kpi-l">personnes</div></div>
    <div class="kpi"><div class="kpi-n">${t.repos || 0}</div><div class="kpi-l">dépôts actifs</div></div>
  </div>`;
  out.innerHTML = `
    <div class="report-head"><div><b>État au ${esc(fmtDate(rep.created_at))}</b> <span class="muted">· ${esc(fmtDay(rep.since))} → ${esc(fmtDay(rep.until))} · ${rep.repos_scanned} dépôts analysés</span></div>
      <button class="btn btn-ghost btn-sm" id="stats-print">Exporter / imprimer</button></div>
    ${kpis}
    <div class="chart-grid">
      <div class="chart-card"><div class="chart-h">Par personne <span class="seg sm" id="stats-metric"><button data-m="commits" class="on">commits</button><button data-m="additions">+ lignes</button><button data-m="deletions">− lignes</button></span></div><div class="chart-wrap"><canvas id="ch-authors"></canvas></div></div>
      <div class="chart-card"><div class="chart-h">Activité par jour</div><div class="chart-wrap"><canvas id="ch-date"></canvas></div></div>
      <div class="chart-card"><div class="chart-h">Horaires d'activité (heure locale des commits)</div><div class="chart-wrap"><canvas id="ch-hour"></canvas></div></div>
      <div class="chart-card"><div class="chart-h">Dépôts les plus actifs</div><div class="chart-wrap"><canvas id="ch-repos"></canvas></div></div>
    </div>
    <div class="blk"><div class="blk-h">Détail par personne <span class="muted">(cliquez pour inclure/exclure des graphiques)</span></div><div id="stats-authors-tbl"></div></div>`;
  el("stats-print").onclick = () => window.print();
  el("stats-metric").querySelectorAll("button").forEach((b) => b.onclick = () => {
    statsCtx.metric = b.dataset.m;
    el("stats-metric").querySelectorAll("button").forEach((x) => x.classList.toggle("on", x === b));
    statsDrawAuthors(rep);
  });
  statsDrawAuthors(rep);
  statsDrawDate(rep);
  statsDrawHour(rep);
  statsDrawRepos(rep);
  statsDrawAuthorTable(rep);
}

// Palette + fabrique de graphiques Chart.js (thème-aware via les variables CSS).
function cssVar(n) { return getComputedStyle(document.documentElement).getPropertyValue(n).trim() || "#4b7bec"; }
function mkChart(id, cfg) {
  const cv = el(id); if (!cv || typeof Chart === "undefined") return;
  // Chart.js v4 refuse de réutiliser un canvas occupé : on détruit l'instance précédente d'abord
  // (statsDrawAuthors est ré-appelée à chaque filtre métrique/exclusion).
  const prev = Chart.getChart(cv);
  if (prev) { try { prev.destroy(); } catch (e) {} statsCtx.charts = statsCtx.charts.filter((c) => c !== prev); }
  cfg.options = Object.assign({ responsive: true, maintainAspectRatio: false, plugins: { legend: { display: false } } }, cfg.options || {});
  statsCtx.charts.push(new Chart(cv.getContext("2d"), cfg));
}
function statsActiveAuthors(rep) { return (rep.authors || []).filter((a) => !statsCtx.excluded[a.login || a.name]); }

function statsDrawAuthors(rep) {
  const m = statsCtx.metric, top = statsActiveAuthors(rep).slice(0, 12);
  mkChart("ch-authors", {
    type: "bar",
    data: { labels: top.map((a) => a.login || a.name || "?"), datasets: [{ data: top.map((a) => a[m] || 0), backgroundColor: cssVar("--accent"), borderRadius: 4 }] },
    options: { indexAxis: "y", scales: { x: { beginAtZero: true } } },
  });
}
function statsDrawDate(rep) {
  const dates = Object.keys(rep.by_date || {}).sort();
  mkChart("ch-date", {
    type: "line",
    data: { labels: dates, datasets: [{ data: dates.map((d) => rep.by_date[d]), borderColor: cssVar("--accent"), backgroundColor: "transparent", tension: .3, pointRadius: 2, fill: false }] },
    options: { scales: { y: { beginAtZero: true } } },
  });
}
function statsDrawHour(rep) {
  mkChart("ch-hour", {
    type: "bar",
    data: { labels: Array.from({ length: 24 }, (_, h) => h + "h"), datasets: [{ data: (rep.by_hour || []), backgroundColor: cssVar("--info"), borderRadius: 3 }] },
    options: { scales: { y: { beginAtZero: true } } },
  });
}
function statsDrawRepos(rep) {
  const top = (rep.repos || []).slice(0, 10);
  mkChart("ch-repos", {
    type: "bar",
    data: { labels: top.map((r) => r.full_name), datasets: [{ data: top.map((r) => r.commits), backgroundColor: cssVar("--ok"), borderRadius: 4 }] },
    options: { indexAxis: "y", scales: { x: { beginAtZero: true } } },
  });
}
function statsDrawAuthorTable(rep) {
  const box = el("stats-authors-tbl"); if (!box) return;
  box.innerHTML = (rep.authors || []).map((a) => {
    const k = a.login || a.name, off = statsCtx.excluded[k];
    return `<div class="srow stats-arow${off ? " off" : ""}" data-a="${esc(k)}">
      <div><b>${esc(a.login || a.name || "?")}</b>${a.login && a.name && a.login !== a.name ? ` <span class="muted">${esc(a.name)}</span>` : ""}</div>
      <div class="muted">${a.commits} commits · <span style="color:var(--ok)">+${a.additions}</span> / <span style="color:var(--danger)">−${a.deletions}</span> · ${a.repos} dépôts</div></div>`;
  }).join("") || `<div class="muted">Aucune activité sur la période.</div>`;
  box.querySelectorAll(".stats-arow").forEach((row) => row.onclick = () => {
    const k = row.dataset.a; statsCtx.excluded[k] = !statsCtx.excluded[k];
    row.classList.toggle("off", !!statsCtx.excluded[k]);
    statsDrawAuthors(rep);
  });
}

async function refreshWhoami() {
  const w = el("whoami"), live = el("live");
  const was = state.online;
  try {
    const u = await App().Whoami(state.instance);
    state.online = true; state.offlineReason = null;
    state.isAdmin = !!u.is_admin;
    state.login = u.login;
    const gh = curIsGitHub();
    // Header allégé (1.16.4) : plus de badge « en ligne » ici — les pastilles Git ✓ / Cloud ✓ portent
    // seules l'état des connexions. On conserve toute la logique state.online (elle pilote l'écran de
    // reconnexion) ; seul l'indicateur visuel disparaît.
    if (w) w.innerHTML = "";
    if (live) live.classList.remove("off");
    // Admin et Tokens sont propres à Forgejo (API absente sur GitHub) ; leurs accès vivent désormais
    // dans Réglages → Connexions git (1.11). On garde les toggles best-effort si les éléments existent.
    const na = el("nav-admin"); if (na) na.classList.toggle("hidden", gh || !u.is_admin);
    const nt = document.querySelector('.nav-item[data-view="tokens"]'); if (nt) nt.classList.toggle("hidden", gh);
  } catch (e) {
    if (isLocked(e)) { handleLock(); return; }
    state.online = false;
    state.offlineReason = isAuthError(e) ? "auth" : "net";
    // Plus de badge texte dans le header (1.16.4) : l'état hors-ligne est porté par les pastilles
    // Git ✓ / Cloud ✓ et par l'écran de reconnexion (state.online). On vide simplement la zone.
    if (w) w.innerHTML = "";
    if (live) live.classList.add("off");
  }
  // Bascule d'état (en ligne ⇄ hors ligne) : si l'écran courant est bridé, on le (dé)verrouille
  // aussitôt — la boucle reste cohérente sans attendre une action de l'utilisateur.
  if (was !== state.online && !OFFLINE_OK_VIEWS.includes(state.view)) render();
}

