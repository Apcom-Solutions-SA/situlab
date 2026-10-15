/* ---------------- Gestionnaire de tokens ---------------- */
const SCOPES = [
  ["read:repository", "Lire les dépôts"], ["write:repository", "Modifier les dépôts"],
  ["read:issue", "Lire les issues"], ["write:issue", "Modifier les issues"],
  ["read:user", "Lire le profil"], ["read:organization", "Lire les organisations"],
];
const SCOPE_LABEL = Object.fromEntries(SCOPES.map(([s, l]) => [s, l]));
// scopeText rend une portée en langage clair ; un slug inconnu est traduit grossièrement.
const scopeText = (s) => SCOPE_LABEL[s] || String(s).replace(/^read:/, "lire : ").replace(/^write:/, "modifier : ").replace(/^admin:/, "admin : ");
// Un token nommé « SituLab@… » est celui que SituLab a créé pour lui-même à l'onboarding.
const isSitulabToken = (name) => /^SituLab@/.test(String(name || ""));
// Instance courante + indicateur 2FA (pour ne demander que le code 2FA le cas échéant).
const curInst = () => (state.instances || []).find((i) => i.name === state.instance) || {};
const curIsGitHub = () => curInst().provider === "github";
const needs2FA = () => !!curInst().two_fa;
// Badge/icône Forgejo (git branch) — signale un login EXTERNE, style volontairement distinct.
const FJ_ICON = `<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true"><path d="M7 4.5a2 2 0 1 0 0 4 2 2 0 0 0 0-4Zm0 11a2 2 0 1 0 0 4 2 2 0 0 0 0-4Zm10-11a2 2 0 1 0 0 4 2 2 0 0 0 0-4ZM7 8.5v7m0-4h5a4 4 0 0 0 4-4" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/></svg>`;
const GH_ICON = `<svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg>`;
// forgejoBlock enveloppe DES CHAMPS d'identifiants Forgejo dans un bloc entièrement thématisé
// « Forgejo » (teal, encadré) : on voit d'emblée que c'est un login EXTERNE, pas le PIN SituLab.
const forgejoBlock = (fieldsHtml, url, variant) => {
  if (variant === "github") {
    return `<div class="forgejo-block github-block">
      <div class="fj-head"><span class="fj-badge gh">${GH_ICON} GitHub</span> Connexion par jeton</div>
      <div class="fj-sub">Jeton d'accès personnel GitHub — chiffré par votre PIN${url ? ` · <a class="lnk" data-ext="${esc(url)}">${esc(url)}</a>` : ""}.</div>
      ${fieldsHtml}</div>`;
  }
  return `<div class="forgejo-block">
  <div class="fj-head"><span class="fj-badge">${FJ_ICON} Forgejo</span> Connexion à votre serveur git</div>
  <div class="fj-sub">Vos identifiants de compte <b>Forgejo</b> — pas votre PIN SituLab. Une question&nbsp;? Votre administrateur Forgejo${url ? ` · <a class="lnk" data-ext="${esc(url)}">${esc(url)}</a>` : ""}.</div>
  ${fieldsHtml}</div>`;
};

// tokenHelp : aide CONTEXTUELLE au type de jeton selon le système git. GitHub = jeton personnel à
// créer soi-même (scopes précis) ; Forgejo/Gitea = SituLab le crée depuis le login. Repliable.
function tokenHelp(provider) {
  if (provider === "github") {
    return `<details class="tok-help"><summary>🔑 Quel jeton créer sur GitHub ?</summary>
      <ol class="tok-steps">
        <li>Ouvrez <a class="lnk" data-ext="https://github.com/settings/personal-access-tokens">GitHub → Settings → Developer settings → Personal access tokens</a>.</li>
        <li>Choisissez <b>Fine-grained token</b> (recommandé) ou <b>Token (classic)</b>.</li>
        <li><b>Fine-grained</b> : « Repository access » = les dépôts voulus (ou « All ») ; « Permissions » → <b>Contents : Read and write</b> (ajoutez <b>Issues</b> et <b>Pull requests</b> si vous les gérez).<br><b>Classic</b> : cochez simplement le scope <b>repo</b>.</li>
        <li><b>Generate</b>, puis <b>copiez</b> le jeton (<code>github_pat_…</code> ou <code>ghp_…</code>) et collez-le ci-dessus.</li>
      </ol>
      <div class="tok-note">GitHub interdit à une application de créer le jeton à votre place — d'où cette étape manuelle. Il est chiffré par votre PIN.</div>
    </details>`;
  }
  return `<details class="tok-help"><summary>🔑 Et le jeton sur Forgejo / Gitea ?</summary>
    <div class="tok-note">Rien à créer à la main : avec votre <b>identifiant</b>, votre <b>mot de passe</b> (et votre <b>code 2FA</b> si activé), SituLab génère pour vous un jeton d'accès, chiffré par votre PIN.</div>
    <ul class="tok-steps">
      <li>Portées demandées : <b>lecture/écriture</b> des dépôts, des issues, du profil et des organisations.</li>
      <li>Votre compte doit avoir les droits sur les dépôts visés (et « administrateur » si vous gérez les comptes).</li>
      <li>Vous le retrouverez côté serveur dans <i>Paramètres → Applications → Jetons d'accès</i>.</li>
    </ul>
  </details>`;
}

// serverPageURL : URL de gestion côté serveur selon le fournisseur et le type de page.
// GitHub n'expose pas les jetons/les comptes par API → on y renvoie l'utilisateur par un bouton.
function serverPageURL(inst, kind) {
  const u = (inst && inst.url ? inst.url : "").replace(/\/+$/, "");
  const gh = inst && (inst.provider === "github" || /github\.com/i.test(u));
  if (gh) {
    const base = "https://github.com";
    if (kind === "tokens") return base + "/settings/tokens";
    if (kind === "users") return base + "/settings/organizations";
    return base + "/settings";
  }
  if (!u) return "";
  if (kind === "tokens") return u + "/user/settings/applications";
  if (kind === "users") return u + "/-/admin/users";
  return u + "/user/settings";
}

// serverLinkBtn : bouton « gérer sur le serveur ↗ » ouvrant la page du fournisseur dans le navigateur.
// Via data-openurl + écouteur délégué (la CSP script-src 'self' interdit les onclick inline).
function serverLinkBtn(inst, kind, label) {
  const url = serverPageURL(inst, kind);
  if (!url) return "";
  return `<button class="btn btn-ghost btn-sm" data-openurl="${esc(url)}">${esc(label || "Gérer sur le serveur")} ↗</button>`;
}
// safeOpenURL n'ouvre dans le navigateur systeme qu'une URL http(s) EXPLICITE (defense en profondeur
// contre file://, javascript:, data:… qu'un contenu Markdown/README non fiable tenterait d'injecter).
function safeOpenURL(u) {
  if (typeof u === "string" && /^https?:\/\//i.test(u.trim())) {
    try { window.runtime.BrowserOpenURL(u.trim()); } catch (_) {}
  }
}
document.addEventListener("click", (e) => {
  const b = e.target.closest && e.target.closest("[data-openurl]");
  if (b) safeOpenURL(b.dataset.openurl);
});

async function renderTokens() {
  if (curIsGitHub()) {
    el("view").innerHTML = `<div class="view-head"><div class="view-title">Jetons</div>
        <div class="view-sub">GitHub ne permet pas à une application de lister ou créer vos jetons — ça se gère sur github.com.</div></div>
      <div class="panel" style="max-width:560px"><div class="panel-h">Jetons d'accès personnels (GitHub)</div>
        <p class="sub">Vos <b>PAT</b> (Personal Access Tokens) se créent, se listent et se révoquent directement sur GitHub → <i>Settings → Developer settings → Personal access tokens</i>.</p>
        <div class="set-row-c" style="justify-content:flex-start">${serverLinkBtn(curInst(), "tokens", "Ouvrir les jetons GitHub")}</div>
      </div>`;
    return;
  }
  const title = `<div class="view-head view-head-row"><div><div class="view-title">Jetons Forgejo</div>
    <div class="view-sub">Vos jetons sur « ${esc(state.instance)} ». Un jeton hérite de vos droits.</div></div>
    ${serverLinkBtn(curInst(), "tokens", "Gérer sur le serveur")}</div>`;
  const twoFA = needs2FA();
  // Compte SANS 2FA : le mot de passe mémorisé (chiffré par le PIN) donne l'accès direct —
  // le PIN suffit, pas de re-saisie. On tente une fois ; échec → panneau.
  if (!twoFA && !state.tokStored && !state._tokTried) {
    state._tokTried = true;
    try { await App().TokenList(state.instance, state.login, "", ""); state.tokStored = true; }
    catch (e) { /* pas de mot de passe mémorisé → panneau */ }
  }
  if (!state.tokStored) {
    // Compte 2FA : on ne demande QUE le code 2FA (le mot de passe vient du trousseau).
    el("view").innerHTML = title + `
      <div class="panel" style="max-width:460px">
        <div class="panel-h">${twoFA ? "Code à deux facteurs" : "Déverrouiller"}</div>
        <p class="sub">${twoFA ? "Votre compte utilise la double authentification : saisissez le code 2FA du moment (il n'est pas mémorisable). Votre mot de passe, lui, est déjà connu de SituLab (chiffré par votre PIN)." : "Saisissez le mot de passe du compte pour gérer les jetons."}</p>
        ${forgejoBlock(`<div class="field">
          ${twoFA ? "" : `<label>Mot de passe de « ${esc(state.login || "")} »<input id="tok-pwd" type="password" /></label>`}
          <label>Code 2FA ${twoFA ? "" : `<span class="muted">(si votre compte en a un)</span>`}</label><div id="tok-otp" class="git-code"></div>
          <div class="inline-error hidden" id="tok-err"></div>
        </div>`, curInst().url)}
        ${twoFA ? "" : `<button class="btn btn-primary" id="tok-unlock">Déverrouiller</button>`}
      </div>`;
    const go = async () => {
      const pwd = twoFA ? "" : el("tok-pwd").value, otp = ciOtp.read(); const er = el("tok-err");
      try {
        await App().TokenList(state.instance, state.login, pwd, otp);
        state.tokOtp = otp; state.tokStored = true;
        // Mémorise le mot de passe (upgrade des instances créées avant) → plus de re-saisie.
        try { await App().RememberPassword(state.instance, pwd, !!otp); const ci2 = curInst(); if (ci2) ci2.two_fa = !!otp; } catch (e) {}
        renderTokens();
      } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); ciOtp.clear(); }
    };
    // Compte 2FA : validation auto dès les 6 chiffres. Sinon : bouton (après le mot de passe).
    const ciOtp = codeInput(el("tok-otp"), false, twoFA ? () => go() : null, twoFA);
    if (el("tok-unlock")) el("tok-unlock").onclick = go;
    if (el("tok-pwd")) { el("tok-pwd").onkeydown = (e) => { if (e.key === "Enter") go(); }; setTimeout(() => el("tok-pwd").focus(), 30); }
    return;
  }
  el("view").innerHTML = title + `
    <div class="toolbar"><button class="btn btn-primary btn-sm" id="tok-ia">🤖 Jeton pour mon IA</button>
      <button class="btn btn-ghost btn-sm" id="tok-new">＋ Nouveau token</button>
      <button class="btn btn-ghost btn-sm" id="tok-lock">Verrouiller</button></div>
    <div id="tok-list"><div class="skeleton"></div></div>`;
  el("tok-new").onclick = () => tokenCreateModal();
  el("tok-ia").onclick = () => tokenCreateModal({ ia: true, name: iaTokenName(), scopes: IA_SCOPES });
  el("tok-lock").onclick = () => { state.tokPwd = null; state.tokOtp = null; state.tokStored = false; state._tokTried = false; renderTokens(); };
  try {
    const toks = await App().TokenList(state.instance, state.login, state.tokPwd || "", state.tokOtp || "");
    const active = await App().ActiveTokenName(state.instance).catch(() => ""); // le SEUL à protéger
    el("tok-list").onclick = async (ev) => {
      const del = ev.target.closest("[data-del]"); if (!del) return;
      tokenDeleteConfirm(+del.dataset.del, del.dataset.name);
    };
    // On sépare le JETON DE SESSION de SituLab (celui qu'il utilise pour cette connexion) du
    // reste : il n'est pas administrable ici, on le met au second plan, discret.
    const list = toks || [];
    const sess = list.find((t) => t.name === active);
    const mine = list.filter((t) => t.name !== active);
    const pillsOf = (t) => (t.scopes || []).length ? t.scopes.map((s) => `<span class="pill">${esc(scopeText(s))}</span>`).join(" ") : `<span class="muted">portée par défaut</span>`;
    const rows = mine.length ? mine.map((t) =>
      `<div class="srow"><div><b>${esc(t.name)}</b> ${pillsOf(t)}</div><button class="lnk danger" data-del="${t.id}" data-name="${esc(t.name)}">supprimer${isSitulabToken(t.name) ? " (résidu)" : ""}</button></div>`
    ).join("") : `<p class="muted">Aucun jeton pour relier d'autres outils. Créez-en un avec « Nouveau token ».</p>`;
    const sessBlock = sess
      ? `<div class="sess-token"><span class="sess-dot"></span><div><div class="sess-name">Jeton de session SituLab</div><div class="sess-sub">Créé et géré par SituLab pour cette connexion. Non modifiable ici — révocable uniquement depuis Forgejo, par un administrateur.</div></div></div>`
      : "";
    el("tok-list").innerHTML = rows + sessBlock;
  } catch (e) { state.tokPwd = null; state.tokOtp = null; state.tokStored = false; state._tokTried = false; el("tok-list").innerHTML = surfaceError("Session expirée", cleanErr(e), () => renderTokens()); }
}

// IA_SCOPES : droits conseillés pour un assistant IA de développement (lecture + écriture
// des dépôts et des issues ; aucun droit d'administration).
const IA_SCOPES = ["read:repository", "write:repository", "read:issue", "write:issue", "read:user"];

function tokenCreateModal(preset) {
  const ia = preset && preset.ia;
  const presetScopes = (preset && preset.scopes) || null;
  const isChecked = (s) => presetScopes ? presetScopes.includes(s) : (s === "read:repository" || s === "write:repository");
  const checks = SCOPES.map(([s, l]) => `<label style="flex-direction:row;align-items:center" title="${s}"><input type="checkbox" class="sc" value="${s}" style="width:auto;margin-right:8px"${isChecked(s) ? " checked" : ""} />${esc(l)}</label>`).join("");
  const repoNames = (state.repos || []).filter((r) => r.has_remote).map((r) => r.full_name);
  const repoChecks = repoNames.map((n) => `<label style="flex-direction:row;align-items:center"><input type="checkbox" class="rp" value="${esc(n)}" style="width:auto;margin-right:8px" />${esc(n)}</label>`).join("");
  openModal(ia ? "Créer un jeton pour mon IA" : "Nouveau token", `<div class="field">
    ${ia ? `<div class="callout">Ce jeton donne à votre assistant IA l'accès à vos dépôts et issues (lecture + écriture), <b>sans</b> droits d'administration. Les envois resteront confirmés par <b>votre PIN</b>.</div>` : ""}
    <label>Nom<input id="tk-name" placeholder="ex. situlab-poste" value="${ia ? esc((preset && preset.name) || "") : ""}" /></label>
    <div class="blk-h" style="margin-top:4px">Portées</div>${checks}
    <div class="blk-h" style="margin-top:4px">Accès aux dépôts</div>
    <label style="flex-direction:row;align-items:center"><input type="radio" name="tkmode" id="tk-all" value="all" checked style="width:auto;margin-right:8px" />Tous mes dépôts (selon mes droits)</label>
    <label style="flex-direction:row;align-items:center"><input type="radio" name="tkmode" value="some" style="width:auto;margin-right:8px" />Dépôts spécifiques</label>
    <p class="sub" style="margin-top:4px">Un jeton peut agir sur tous vos dépôts, ou seulement sur ceux que vous cochez — préférez la seconde option s'il ne sert qu'à un projet.</p>
    <div id="tk-repos" class="repo-picker hidden">${repoChecks || '<span class="muted">Vos dépôts ne sont pas encore chargés — ouvrez d\'abord l\'onglet Dépôts.</span>'}</div>
    ${needs2FA() ? forgejoBlock(`<label>Code 2FA du moment</label><div id="tk-otp" class="git-code"></div>`, curInst().url) : ""}
    <div class="inline-error hidden" id="tk-err"></div></div>`, async () => {
    const er = el("tk-err");
    const scopes = [...document.querySelectorAll(".sc:checked")].map((c) => c.value);
    const repos = el("tk-all").checked ? [] : [...document.querySelectorAll(".rp:checked")].map((c) => c.value);
    const tkname = el("tk-name").value.trim();
    if (!tkname) { er.textContent = "Donnez un nom au jeton."; er.classList.remove("hidden"); return; }
    if (!el("tk-all").checked && repos.length === 0) { er.textContent = "Cochez au moins un dépôt, ou choisissez « Tous mes dépôts »."; er.classList.remove("hidden"); return; }
    const otp = ciOtp ? ciOtp.read() : (state.tokOtp || "");
    // v1.5.0 : plus de PIN ici — l'authentification est le mot de passe Forgejo (+ 2FA).
    try {
      const val = await App().TokenCreate(state.instance, state.login, state.tokPwd || "", otp, tkname, scopes, repos, "");
      closeModal();
      renderTokens();
      openModal("Jeton créé", `<div class="field"><div class="callout warn">⚠ Copiez ce jeton maintenant : pour votre sécurité, il ne sera plus jamais réaffiché. Perdu, il faudra le supprimer et en créer un autre.</div>
        <pre class="filebox" id="tk-val" style="user-select:text;white-space:pre-wrap">${esc(val)}</pre>
        <button class="btn btn-ghost btn-sm" type="button" id="tk-copy">Copier le jeton</button>
        ${ia ? `<button class="btn btn-primary btn-sm" type="button" id="tk-prompt" style="margin-top:6px">Configurer mon agent IA</button>` : ""}</div>`, () => closeModal());
      const cp = el("tk-copy"); if (cp) cp.onclick = async () => { try { await navigator.clipboard.writeText(val); toast("Jeton copié", "ok"); } catch (e) { toast("Copie impossible — sélectionnez le texte puis Ctrl+C", "err"); } };
      const pb = el("tk-prompt"); if (pb) pb.onclick = () => aiAgentModal();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); if (ciOtp) ciOtp.clear(); }
  });
  el("modal-ok").textContent = "Continuer";
  document.querySelectorAll('input[name="tkmode"]').forEach((r) => r.onchange = () => el("tk-repos").classList.toggle("hidden", el("tk-all").checked));
  const ciOtp = needs2FA() ? codeInput(el("tk-otp"), false, null) : null;
}

function tokenDeleteConfirm(id, name) {
  let ciOtp;
  openModal("Supprimer le token", `<div class="field">
    <p class="sub">Supprimer le token « ${esc(name)} » ? Les outils qui l'utilisent cesseront de fonctionner.</p>
    ${needs2FA() ? forgejoBlock(`<label>Code 2FA du moment</label><div id="td-otp" class="git-code"></div>`, curInst().url) : ""}
    <div class="inline-error hidden" id="td-err"></div></div>`, async () => {
    const otp = ciOtp ? ciOtp.read() : (state.tokOtp || "");
    const er = el("td-err");
    try {
      await App().TokenDelete(state.instance, state.login, state.tokPwd || "", otp, id, "");
      closeModal(); toast("Token supprimé", "ok"); renderTokens();
    } catch (e) { er.textContent = cleanErr(e); er.classList.remove("hidden"); if (ciOtp) ciOtp.clear(); }
  });
  ciOtp = needs2FA() ? codeInput(el("td-otp"), false, null, true) : null;
  el("modal-ok").textContent = "Supprimer"; el("modal-ok").classList.add("danger-btn");
}

// iaTokenName : nom par défaut, identifiable et révocable, du jeton dédié à l'IA.
function iaTokenName() { return "IA-SituLab"; }

// firstPromptText : message de PREMIÈRE PRISE DE CONTACT, volontairement léger et ouvert — pas de
// liste de dépôts ni de commandes imposées. On dit à l'IA où est l'outil, de lire la fiche
// AGENTS.md (mode d'emploi + collaboration), de regarder l'état et de nous dire ce qu'elle en
// pense. exe = chemin complet de l'exécutable (résout le « pas dans le PATH »). kind = "mcp" | "cli".
function firstPromptText(kind, exe) {
  exe = exe || "situlab";
  const launch = kind === "mcp"
    ? "- Tu me pilotes SituLab via le serveur MCP « situlab » (ses outils apparaissent chez toi)."
    : `- SituLab est installé ici : "${exe}". Lance-le en ligne de commande par ce chemin complet (il n'est pas forcément dans le PATH), par ex. "${exe}" --help.`;
  return [
    "Salut ! On va travailler ensemble avec SituLab, mon utilitaire git. Le principe : toi tu prépares, moi je garde la main — je valide par mon PIN.",
    "",
    launch,
    "- Près de mes dépôts, tu trouveras un fichier AGENTS.md : c'est à la fois le mode d'emploi pour toi ET notre fiche de collaboration. Lis-le en premier.",
    "- Ensuite, regarde l'état de mes dépôts et dis-moi simplement ce que tu comprends et ce que tu proposes.",
    "- Règle d'or : ne me demande jamais mon PIN et ne cherche pas à le contourner. Rien ne part en ligne sans ma validation.",
    "",
    "Pour commencer : dis-moi ce que tu as compris de SituLab, et propose-moi ce qu'on pourrait noter dans AGENTS.md sur notre façon de travailler (tu peux l'enrichir au fil de l'eau).",
  ].join("\n");
}

// aiAgentModal : « faisons connaissance avec votre agent IA » (v1.4.2). L'utilisateur choisit son
// type d'agent (terminal/CLI ou système/MCP) ; SituLab produit le prompt de première connexion
// sur mesure, la config MCP à coller, et propose d'installer le guide AGENTS.md.
function aiAgentModal() {
  infoModal("Faisons connaissance avec votre agent IA", `
    <p class="sub">SituLab est fait pour que votre assistant <b>prépare</b> le travail et que <b>vous</b> validiez par votre PIN. Lequel utilisez-vous&nbsp;?</p>
    <div class="ai-choice">
      <button type="button" class="ai-pick" data-k="cli"><span class="ai-pick-ic">🖥️</span><b>Un agent en terminal</b><span class="sub">Claude Code, agent shell… il tape des commandes <code>situlab</code>.</span></button>
      <button type="button" class="ai-pick" data-k="mcp"><span class="ai-pick-ic">🧩</span><b>Un agent système (MCP)</b><span class="sub">Claude Desktop, Cursor… il se connecte à <code>situlab mcp</code>.</span></button>
    </div>
    <div id="ai-out"></div>`);
  document.querySelectorAll(".ai-pick").forEach((b) => b.onclick = () => aiAgentShow(b.dataset.k));
}

async function aiAgentShow(kind) {
  document.querySelectorAll(".ai-pick").forEach((b) => b.classList.toggle("active", b.dataset.k === kind));
  const out = el("ai-out");
  if (!out) return;
  let exe = "situlab";
  try { exe = (await App().ExePath()) || "situlab"; } catch (e) {}
  let mcpBlock = "", cfg = "";
  if (kind === "mcp") {
    cfg = JSON.stringify({ mcpServers: { situlab: { command: exe, args: ["mcp"] } } }, null, 2);
    mcpBlock = `<div class="ai-step"><div class="ai-step-h">1. Ajoutez SituLab à votre agent MCP</div>
      <p class="sub">Collez ceci dans la configuration MCP de votre agent (Claude Desktop, Cursor…) :</p>
      <pre class="filebox" style="user-select:text;white-space:pre-wrap">${esc(cfg)}</pre>
      <button type="button" class="btn btn-ghost btn-sm" id="ai-cfg">Copier la configuration</button></div>`;
  }
  const a = kind === "mcp" ? "2" : "1", b = kind === "mcp" ? "3" : "2";
  out.innerHTML = mcpBlock + `
    <div class="ai-step"><div class="ai-step-h">${a}. Dites bonjour à votre agent</div>
      <p class="sub">Copiez ce message et collez-le dans votre assistant : il lui dit où est SituLab, l'invite à lire la fiche de collaboration et à vous donner son premier avis. Léger, ouvert — pas de commandes imposées.</p>
      <button type="button" class="btn btn-primary btn-sm" id="ai-prompt">Copier le message de présentation 🤖</button></div>
    <div class="ai-step"><div class="ai-step-h">${b}. Déposez la fiche de collaboration</div>
      <p class="sub"><code>AGENTS.md</code> est à la fois le mode d'emploi de votre agent ET votre fiche de collaboration : déposez-la dans votre projet (les agents la lisent automatiquement), votre IA la lira au démarrage et pourra l'enrichir avec vous. Modifiable aussi dans Réglages.</p>
      <button type="button" class="btn btn-ghost btn-sm" id="ai-doc">Déposer AGENTS.md dans un dossier…</button></div>`;
  if (el("ai-cfg")) el("ai-cfg").onclick = () => copyPrompt(cfg, { close: false, title: "Configuration MCP" });
  el("ai-prompt").onclick = () => copyPrompt(firstPromptText(kind, exe), { close: false, title: "Message de présentation" });
  el("ai-doc").onclick = installDirectives;
}

// installDirectives : choisit un dossier et y écrit le guide AGENTS.md pour l'agent de l'utilisateur.
async function installDirectives() {
  try {
    const dir = await App().PickFolder();
    if (!dir) return;
    const p = await App().InstallAgentsDoc(dir);
    toast("Directives enregistrées : " + shortPath(p), "ok");
  } catch (e) { toast(cleanErr(e), "err"); }
}

/* ---------------- Invite IA réutilisable (pour chaque action CRUD) ---------------- */
// copyPrompt copie un prompt, signale à l'utilisateur d'aller le coller dans son IA, et
// (par défaut) ferme le dialogue en cours. opts.close=false pour ne pas fermer (ex. Aide,
// ou modale « Jeton créé » qui doit rester visible). Repli visuel si le presse-papier est refusé.
async function copyPrompt(text, opts) {
  opts = opts || {};
  const close = opts.close !== false;
  try {
    await navigator.clipboard.writeText(text);
    toast("Prompt copié 🤖 — collez-le dans votre IA", "ok");
    if (close) closeModal();
  } catch (e) {
    infoModal(opts.title || "Prompt pour votre IA", `<p class="sub">Copie automatique refusée — sélectionnez puis copiez (Ctrl+C), et collez dans votre IA :</p><pre class="filebox" style="user-select:text;white-space:pre-wrap">${esc(text)}</pre>`);
  }
}

// aiInviteHTML : bloc « laissez votre IA le faire » (même habillage partout). À placer dans
// une modale CRUD ; appeler ensuite wireAiInvite(prompt) après openModal pour brancher le clic.
function aiInviteHTML(sub) {
  return `<div class="ai-invite">
    <div class="ai-invite-txt"><span class="ai-bot">🤖</span><div><b>Pas à l'aise ? Laissez votre IA le faire.</b><div class="sub">${esc(sub || "Votre agent est là pour ça. Copiez le prompt et collez-le dans votre IA — SituLab gère seul la validation si elle est nécessaire.")}</div></div></div>
    <button type="button" class="btn btn-ghost btn-sm" id="ai-copy">Copier le prompt pour mon IA</button>
  </div>`;
}
// wireAiInvite branche le bouton de l'invite sur le prompt donné (texte, ou fonction qui le produit).
function wireAiInvite(prompt) {
  const b = el("ai-copy"); if (!b) return;
  b.onclick = () => copyPrompt(typeof prompt === "function" ? prompt() : prompt);
}

// Prompts par action — directifs, avec la commande CLI exacte. Le PIN n'est JAMAIS décrété ici :
// c'est SituLab qui, selon la politique de la connexion, répond s'il faut une validation et confirme
// l'exécution (ou signale le délai dépassé). L'agent ne présume rien à ce sujet.
function aiHeader(verb) {
  const inst = curInst();
  const name = inst.name || state.instance;
  return `${verb} via SituLab (instance « ${name} »). Utilise la CLI \`situlab\` avec \`--instance "${name}"\`. Exécute la commande : SituLab t'indiquera lui-même si une validation est requise et te confirmera quand c'est fait.`;
}
function promptRepoCreate(owner, suggest) {
  const orgFlag = owner ? ` --org "${owner}"` : "";
  const nm = (suggest || "").trim();
  return [aiHeader("Crée un dépôt distant"), "",
    `Commande : situlab repo create "${nm || "<nom-du-depot>"}" --description "<description>" --private${orgFlag}`,
    nm ? `- Nom proposé : « ${nm} » (dérivé du dépôt existant ; ajuste si besoin).` : "- Choisis un nom cohérent avec le dépôt existant.",
    owner ? `- Ce serveur crée les dépôts dans l'organisation « ${owner} » (garde --org).` : "- Si le serveur refuse l'espace personnel, ajoute --org \"<organisation>\".",
    "- Enlève --private pour un dépôt public.",
  ].join("\n");
}
function promptIssueCreate(owner, repo) {
  const slug = owner && repo ? `${owner}/${repo}` : "<owner>/<repo>";
  return [aiHeader("Ouvre une issue"), "",
    `Commande : situlab issue create ${slug} --title "<titre>" --body "<description>"`,
    "- Propose-moi un titre et une description clairs avant de lancer.",
  ].join("\n");
}
function promptRepoDelete(owner, repo) {
  const slug = owner && repo ? `${owner}/${repo}` : "<owner>/<repo>";
  return [aiHeader("Supprime un dépôt distant"), "",
    `Commande : situlab repo delete ${slug}`,
    "- Action irréversible : confirme bien le dépôt avec moi avant de lancer.",
  ].join("\n");
}
function promptUserCreate() {
  return [aiHeader("Crée un compte utilisateur (administration)"), "",
    'Commande : situlab admin user-create "<identifiant>" --email "<email>" --password "<mot-de-passe-provisoire>"',
    "- Ajoute --admin uniquement si la personne doit être administratrice.",
    "- Le compte devra changer son mot de passe à la première connexion.",
  ].join("\n");
}

/* ---------------- Administration ---------------- */
// userMgmtHeader : en-tête commun « Gestion des utilisateurs » + barre d'onglets (Admin / Annuaire
// / Statistiques). active = vue en cours. wireUserMgmtTabs() branche la navigation entre onglets.
function userMgmtHeader(active) {
  const tabs = [["admin", "Administration"], ["directory", "Annuaire"], ["stats", "Statistiques"]];
  return `<div class="view-head"><div class="view-title">Gestion des utilisateurs</div></div>
    <div class="um-tabs">${tabs.map(([v, l]) => `<button class="um-tab${v === active ? " active" : ""}" data-umv="${v}">${esc(l)}</button>`).join("")}</div>`;
}
function wireUserMgmtTabs() { document.querySelectorAll(".um-tab").forEach((t) => t.onclick = () => navTo(t.dataset.umv)); }

async function renderAdmin() {
  // Réservé aux administrateurs : sinon, message doux plutôt qu'une erreur sèche.
  if (!state.isAdmin) {
    el("view").innerHTML = `<div class="view-head"><div class="view-title">Administration</div></div>` +
      blankslate("⌘", "Réservé aux administrateurs", "Cet espace sert à gérer les comptes de l'équipe. Votre compte n'a pas les droits pour l'ouvrir — demandez à un administrateur si vous pensez que c'est une erreur.");
    return;
  }
  // Compte admin, mais l'accès SituLab n'a pas (encore) le droit de gestion : on propose de
  // l'activer à la demande (v1.4.2 — plus de case « admin » à la connexion).
  const hasScope = await App().AdminScopeFor(state.instance).catch(() => true);
  if (!hasScope) {
    el("view").innerHTML = userMgmtHeader("admin") +
      `<div class="callout"><b>Activer l'administration ?</b><div class="sub">Votre compte est administrateur, mais votre accès SituLab actuel n'inclut pas encore le droit de gérer les comptes. Activez-le pour continuer — SituLab recrée votre accès avec ce droit (identifiant + mot de passe + votre PIN).</div>
      <button class="btn btn-primary btn-sm" id="activate-admin" style="margin-top:10px">Activer l'administration</button></div>`;
    wireUserMgmtTabs();
    el("activate-admin").onclick = () => reconnectModal(state.instance, { admin: true });
    return;
  }
  el("view").innerHTML = userMgmtHeader("admin") + `
    <div class="view-sub" style="margin:-6px 0 10px">Gérer les comptes qui peuvent utiliser « ${esc(state.instance)} ».</div>
    <div class="toolbar">
      <button class="btn btn-primary btn-sm" id="new-user">＋ Nouvel utilisateur</button>
      ${serverLinkBtn(curInst(), "users", "Gérer sur le serveur")}
    </div>
    <div id="users"><div class="skeleton"></div></div>`;
  wireUserMgmtTabs();
  el("new-user").onclick = createUserModal;
  try {
    const users = await App().AdminUsers(state.instance);
    state.users = users || [];
    el("users").innerHTML = state.users.length ? state.users.map((u) => adminRow(u)).join("")
      : blankslate("☺", "Aucun compte pour l'instant", "Ajoutez la première personne avec « ＋ Nouvel utilisateur », en haut de l'écran.");
    wireAdminRows();
  } catch (e) {
    if (isLocked(e)) return handleLock();
    el("users").innerHTML = surfaceError("Impossible de charger les comptes", cleanErr(e), () => renderAdmin());
  }
}

// statut lisible d'un compte : suspendu prime, puis inactif, puis actif.
function userStatus(u) {
  if (u.prohibit_login) return { k: "suspendu", cls: "warn" };
  if (!u.active) return { k: "inactif", cls: "muted" };
  return { k: "actif", cls: "ver" };
}

