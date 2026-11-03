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
      <div class="fj-sub">Jeton d'accès personnel GitHub, chiffré par votre PIN${url ? ` · <a class="lnk" data-ext="${esc(url)}">${esc(url)}</a>` : ""}.</div>
      ${fieldsHtml}</div>`;
  }
  return `<div class="forgejo-block">
  <div class="fj-head"><span class="fj-badge">${FJ_ICON} Forgejo</span> Connexion à votre serveur git</div>
  <div class="fj-sub">Vos identifiants de compte <b>Forgejo</b>, pas votre PIN SituLab. Une question&nbsp;? Votre administrateur Forgejo${url ? ` · <a class="lnk" data-ext="${esc(url)}">${esc(url)}</a>` : ""}.</div>
  ${fieldsHtml}</div>`;
};

// tokenHelp : aide CONTEXTUELLE au type de jeton selon le système git. GitHub = jeton personnel à
// créer soi-même (scopes précis) ; Forgejo/Gitea = SituLab le crée depuis le login. Repliable.
function tokenHelp(provider) {
  if (provider === "github") {
    return `<details class="tok-help"><summary>${ico("key", 14)} Quel jeton créer sur GitHub ?</summary>
      <ol class="tok-steps">
        <li>Ouvrez <a class="lnk" data-ext="https://github.com/settings/personal-access-tokens">GitHub → Settings → Developer settings → Personal access tokens</a>.</li>
        <li>Choisissez <b>Fine-grained token</b> (recommandé) ou <b>Token (classic)</b>.</li>
        <li><b>Fine-grained</b> : « Repository access » = les dépôts voulus (ou « All ») ; « Permissions » → <b>Contents : Read and write</b> (ajoutez <b>Issues</b> et <b>Pull requests</b> si vous les gérez).<br><b>Classic</b> : cochez simplement le scope <b>repo</b>.</li>
        <li><b>Generate</b>, puis <b>copiez</b> le jeton (<code>github_pat_…</code> ou <code>ghp_…</code>) et collez-le ci-dessus.</li>
      </ol>
      <div class="tok-note">GitHub interdit à une application de créer le jeton à votre place, d'où cette étape manuelle. Il est chiffré par votre PIN.</div>
    </details>`;
  }
  return `<details class="tok-help"><summary>${ico("key", 14)} Et le jeton sur Forgejo / Gitea ?</summary>
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

// repoWebURL : URL web d'un dépôt pour « Ouvrir en ligne ». Priorité à html_url (fourni par l'API),
// sinon on la CONSTRUIT depuis la base de l'instance courante + owner/repo — le détail ouvert DEPUIS UN
// PROJET n'a pas de html_url (il valait ""), d'où un bouton mort. GitHub : base web (github.com), jamais
// l'API. Renvoie "" si rien d'exploitable (safeOpenURL ignore alors proprement).
function repoWebURL(d) {
  if (!d) return "";
  if (d.html_url) return d.html_url;
  const r = (state.repos || []).find((x) => x.full_name === d.full_name);
  if (r && r.html_url) return r.html_url;
  const inst = curInst();
  let base = (inst && inst.url ? inst.url : "").replace(/\/+$/, "");
  if (inst && (inst.provider === "github" || /github\.com/i.test(base))) base = "https://github.com";
  if (base && d.owner && d.repo) return base + "/" + encodeURIComponent(d.owner) + "/" + encodeURIComponent(d.repo);
  return "";
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
        <div class="view-sub">GitHub ne permet pas à une application de lister ou créer vos jetons, ça se gère sur github.com.</div></div>
      <div class="panel" style="max-width:560px"><div class="panel-h">Jetons d'accès personnels (GitHub)</div>
        <p class="sub">Vos <b>PAT</b> (Personal Access Tokens) se créent, se listent et se révoquent directement sur GitHub → <i>Settings → Developer settings → Personal access tokens</i>.</p>
        <div class="set-row-c" style="justify-content:flex-start">${serverLinkBtn(curInst(), "tokens", "Ouvrir les jetons GitHub")}</div>
      </div>`;
    return;
  }
  const title = `<div class="view-head view-head-row"><div><div class="view-title">Jetons Forgejo</div>
    <div class="view-sub">Vos jetons sur « ${esc(state.instance)} ». Un jeton hérite de vos droits.</div></div>
    ${serverLinkBtn(curInst(), "tokens", "Gérer sur le serveur")}</div>`;
  // Changement de connexion : la session « jetons » (identité, mot de passe/OTP retenus en vol, drapeaux
  // de déverrouillage) appartient à UNE instance. On peut arriver ici directement depuis la carte d'une
  // AUTRE connexion (lien « jetons ») : sans réinitialiser, on s'authentifierait sous le mauvais compte
  // avec des identifiants d'une instance tierce. On réaligne donc l'identité et on repart propre.
  if (state.tokInstance !== state.instance) {
    state.tokInstance = state.instance;
    state.tokStored = false; state._tokTried = false; state.tokPwd = null; state.tokOtp = null;
    try { const me = await App().Whoami(state.instance); if (me && me.login) state.login = me.login; } catch (e) {}
  }
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
    <div class="toolbar"><button class="btn btn-primary btn-sm" id="tok-ia">${ico("robot", 14)} Jeton pour mon IA</button>
      <button class="btn btn-ghost btn-sm" id="tok-new">${ico("plus", 14)} Nouveau token</button>
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
      `<div class="srow"><div><b>${esc(t.name)}</b> ${pillsOf(t)}</div><button class="icon-del" data-del="${t.id}" data-name="${esc(t.name)}" title="Supprimer le jeton « ${esc(t.name)} »${isSitulabToken(t.name) ? " (résidu SituLab)" : ""}" aria-label="Supprimer le jeton">${ICO_TRASH}</button></div>`
    ).join("") : `<p class="muted">Aucun jeton pour relier d'autres outils. Créez-en un avec « Nouveau token ».</p>`;
    const sessBlock = sess
      ? `<div class="sess-token"><span class="sess-dot"></span><div><div class="sess-name">Jeton de session SituLab</div><div class="sess-sub">Créé et géré par SituLab pour cette connexion. Non modifiable ici, révocable uniquement depuis Forgejo, par un administrateur.</div></div></div>`
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
    <p class="sub" style="margin-top:4px">Un jeton peut agir sur tous vos dépôts, ou seulement sur ceux que vous cochez : préférez la seconde option s'il ne sert qu'à un projet.</p>
    <div id="tk-repos" class="repo-picker hidden">${repoChecks || '<span class="muted">Vos dépôts ne sont pas encore chargés, ouvrez d\'abord l\'onglet Dépôts.</span>'}</div>
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
      // 2FA : conserver le code qui VIENT d'être accepté pour le rafraîchissement de liste qui suit —
      // sinon renderTokens rejouerait l'ANCIEN code de déverrouillage (expiré) et renverrait l'utilisateur
      // à l'écran 2FA juste après une création réussie.
      if (otp) state.tokOtp = otp;
      closeModal();
      renderTokens();
      openModal("Jeton créé", `<div class="field"><div class="callout warn">⚠ Copiez ce jeton maintenant : pour votre sécurité, il ne sera plus jamais réaffiché. Perdu, il faudra le supprimer et en créer un autre.</div>
        <pre class="filebox" id="tk-val" style="user-select:text;white-space:pre-wrap">${esc(val)}</pre>
        <button class="btn btn-ghost btn-sm" type="button" id="tk-copy">Copier le jeton</button>
        ${ia ? `<button class="btn btn-primary btn-sm" type="button" id="tk-prompt" style="margin-top:6px">Configurer mon agent IA</button>` : ""}</div>`, () => closeModal());
      const cp = el("tk-copy"); if (cp) cp.onclick = async () => { try { await navigator.clipboard.writeText(val); toast("Jeton copié", "ok"); } catch (e) { toast("Copie impossible, sélectionnez le texte puis Ctrl+C", "err"); } };
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
      if (otp) state.tokOtp = otp; // conserver le code 2FA accepté pour le rafraîchissement de liste
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
function firstPromptText(kind, exe, docPath) {
  exe = exe || "situlab";
  const isMcp = kind === "mcp";
  // Message SOBRE et TÉLÉDIRIGÉ (1.18.4, retour #4) : moins de prose, et il amorce seul le travail —
  // l'agent liste KERNEL + SPOKES et PROPOSE lequel ouvrir, sans que j'aie à taper « xxx go ». Adapté
  // au type d'agent (MCP = outils, terminal = commandes « situlab »). Désigner l'actif reste un geste
  // de fenêtre : l'agent propose, l'humain choisit.
  const acces = isMcp
    ? "Tu me pilotes SituLab par les outils MCP « situlab » : lectures libres, toute écriture distante passe par mon PIN sur le poste."
    : `SituLab est ici : "${exe}". Appelle-le par ce chemin complet (ex. "${exe}" --help). Toute écriture distante passe par mon PIN sur le poste.`;
  const fiche = docPath
    ? `Lis d'abord ma fiche de collaboration : "${docPath}".`
    : "Lis d'abord le fichier AGENTS.md près de mes dépôts.";
  const amorce = isMcp
    ? "Puis liste mes KERNEL et mes SPOKES (outils kernel / spoke, ou la carte de mes dépôts) et propose-moi lequel ouvrir, sans attendre que je te le dise."
    : "Puis liste mes KERNEL et mes SPOKES (« situlab kernel list », « situlab spoke list ») et propose-moi lequel ouvrir, sans attendre que je te le dise.";
  return [
    "On travaille ensemble avec SituLab, mon utilitaire git : tu prépares, je garde la main (je valide par mon PIN quand il le faut).",
    "",
    acces,
    fiche,
    amorce,
  ].join("\n");
}

// renderAssistantIA : vue de 1er niveau « Agent IA » (v1.17, besoin #1). Sortie des Réglages pour
// devenir une entrée du menu principal. Deux temps clairs : (1) la fiche de collaboration AGENTS.md
// administrée « à la source » (besoin #8), (2) brancher son agent (le choix CLI/MCP, inline ici).
async function renderAssistantIA() {
  // Tableau de bord à cartes (v1.17) : en-tête compact + deux cartes côte à côte (fiche AGENTS.md /
  // brancher son agent). Le détail de configuration n'apparaît qu'en pleine largeur APRÈS le choix du
  // type d'agent — au repos la page tient sans scroll vertical.
  el("view").innerHTML = `
    <div class="ia-hero">
      <div class="ia-hero-ico">${ico("robot", 30)}<span class="ia-hero-ring" aria-hidden="true"></span></div>
      <div class="ia-hero-txt"><div class="view-title">Agent IA</div>
        <div class="ia-hero-sub">Branchez votre IA à SituLab. Elle lit votre fiche, cartographie votre mémoire, et agit sous votre PIN.</div></div>
    </div>
    <div class="ia-steps">
      <div class="ia-step" id="ia-step1">
        <div class="ia-step-h"><span class="ia-step-n" id="ia-n1">1</span><span>Fiche de collaboration</span></div>
        <div id="ia-doc-state" class="ia-step-body"><div class="skeleton"></div></div>
      </div>
      <div class="ia-step" id="ia-step2">
        <div class="ia-step-h"><span class="ia-step-n" id="ia-n2">2</span><span>Brancher mon agent</span></div>
        <div id="ia-branch" class="ia-step-body"><div class="skeleton"></div></div>
      </div>
      <div class="ia-step ia-purge" id="ia-purge">
        <button type="button" class="ia-purge-head" id="ia-purge-toggle" aria-expanded="false">
          <span class="ia-purge-ic">${ico("broom", 16)}</span>
          <span class="ia-purge-ttl">Purger les données des agents IA de ce poste</span>
          <span class="ia-purge-caret" aria-hidden="true"><svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6-6 6"/></svg></span>
        </button>
        <div class="ia-purge-body acc-collapsed" id="ia-purge-body">
          <p class="sub">Les agents IA accumulent sessions, journaux et caches, et n'effacent rien d'eux-mêmes. Remise à blanc franche : seuls le login et le minimum qui fait tourner l'agent sont gardés. Irréversible ; vos dépôts et fichiers ne sont pas touchés.</p>
          <div id="agent-purge-list"><div class="skeleton"></div></div>
        </div>
      </div>
    </div>`;
  let exists = false, docPath = "";
  try { exists = await App().AgentsDocExists(); } catch (_) {}
  try { docPath = await App().AgentsDocPath(); } catch (_) {}
  renderIaDocState(exists, docPath);
  renderIaBranch(exists);
  // Progression visuelle : étape 1 validée (✓) dès que la fiche est à la source ; étape 2 active alors.
  if (el("ia-n1")) el("ia-n1").classList.toggle("done", exists);
  if (el("ia-n1")) el("ia-n1").textContent = exists ? "✓" : "1";
  if (el("ia-step2")) el("ia-step2").classList.toggle("ia-step-off", !exists);
  // Card « Effacer les données des agents IA » : accordéon, déplié au clic (sa place est ici, sur la
  // page Agent IA, plus dans les Réglages). La liste des agents se peint à la première ouverture.
  const pt = el("ia-purge-toggle");
  if (pt) {
    let loaded = false;
    pt.onclick = () => {
      const card = el("ia-purge"), body = el("ia-purge-body");
      const open = card.classList.toggle("open");
      body.classList.toggle("acc-collapsed", !open);
      pt.setAttribute("aria-expanded", open ? "true" : "false");
      if (open && !loaded) { loaded = true; renderAgentPurge(); }
    };
  }
}

// renderIaBranch : la carte « Brancher mon agent » est VERROUILLÉE tant que la fiche n'est pas déposée
// (1.18.1) — pas de branchement sans fiche. Une fois déposée, les deux choix d'agent apparaissent.
function renderIaBranch(exists) {
  const box = el("ia-branch"); if (!box) return;
  if (!exists) {
    box.innerHTML = `<div class="ia-locked"><span class="ia-locked-ic" aria-hidden="true">${ico("lock", 22)}</span><span class="ia-locked-t">Déposez d'abord votre <b>fiche de collaboration</b> (étape&nbsp;1) pour pouvoir brancher votre agent.</span></div>`;
    return;
  }
  box.innerHTML = `<div class="ia-choice">
      <button type="button" class="ia-agent-card on" data-k="cli">
        <span class="ia-agent-ic">${ico("desktop", 22)}</span>
        <span class="ia-agent-txt"><span class="ia-agent-t">Agent en terminal</span><span class="ia-agent-s">Un assistant qui lance des commandes « situlab » (Claude Code, Codex…).</span></span>
        <span class="ia-agent-check" aria-hidden="true">${ico("check", 14)}</span>
      </button>
      <button type="button" class="ia-agent-card" data-k="mcp">
        <span class="ia-agent-ic">${ico("puzzle", 22)}</span>
        <span class="ia-agent-txt"><span class="ia-agent-t">Agent système (MCP)</span><span class="ia-agent-s">Un agent qui pilote SituLab par ses outils (Claude Desktop, Cursor…).</span></span>
        <span class="ia-agent-check" aria-hidden="true">${ico("check", 14)}</span>
      </button>
    </div>
    <div id="ai-out" class="ia-tab-body"></div>`;
  const pick = (k) => { box.querySelectorAll(".ia-agent-card").forEach((t) => t.classList.toggle("on", t.dataset.k === k)); aiAgentShow(k); };
  box.querySelectorAll(".ia-agent-card").forEach((b) => b.onclick = () => pick(b.dataset.k));
  pick("cli");
}

// renderIaDocState : boucle d'état de la fiche à la source (besoin #8), rendue DANS la carte (sans
// encart imbriqué, pour un tableau de bord net). Absente → une seule action : l'enregistrer. Présente →
// statut ✓ + le chemin exact (à donner à l'agent) + consulter / mettre à jour / déposer.
function renderIaDocState(exists, docPath) {
  const box = el("ia-doc-state"); if (!box) return;
  if (!exists) {
    box.innerHTML = `<p class="sub" style="margin:0 0 10px">Pas encore enregistrée. Enregistrez-la pour que votre agent la lise au chemin indiqué, maintenant et après chaque mise à jour.</p>
      <div class="ia-card-act"><button class="btn btn-primary btn-sm" id="ia-doc-save">Enregistrer la fiche à la source</button></div>`;
    el("ia-doc-save").onclick = saveAgentsDocToSource;
    return;
  }
  box.innerHTML = `<div class="ia-ok">✓ Enregistrée à la source</div>
    <code class="ia-pathbox" title="Chemin à donner à votre agent (déjà inclus dans le message de présentation)">${esc(docPath)}</code>
    <div class="ia-card-act">
      <button class="btn btn-ghost btn-sm" id="ia-doc-view">Consulter</button>
      <button class="btn btn-ghost btn-sm" id="ia-doc-resave">Mettre à jour</button>
      <button class="btn btn-ghost btn-sm" id="ia-doc-deposit" title="Déplacer le fichier agent dans un autre dossier">Déplacer dans un autre dossier…</button>
      <button class="icon-del" id="ia-doc-del" title="Supprimer la fiche et refaire la mise en route" aria-label="Supprimer la fiche de collaboration">${ICO_TRASH}</button>
    </div>`;
  el("ia-doc-view").onclick = async () => { try { infoModal("AGENTS.md", `<div class="md-render" style="max-height:60vh;overflow:auto">${mdToHTML(await App().Doc())}</div>`); } catch (_) {} };
  el("ia-doc-resave").onclick = saveAgentsDocToSource;
  el("ia-doc-deposit").onclick = installDirectives;
  el("ia-doc-del").onclick = deleteAgentsDoc;
}

// deleteAgentsDoc : supprime la fiche à la source → re-verrouille « Brancher mon agent » et permet de
// refaire la procédure autant de fois qu'on veut (1.18.1).
function deleteAgentsDoc() {
  openModal("Supprimer la fiche de collaboration ?", `<div class="field"><div class="callout warn">La fiche <code>AGENTS.md</code> enregistrée à la source sera supprimée. « Brancher mon agent » se reverrouille ; vous pourrez la redéposer et recommencer autant de fois que nécessaire.</div></div>`, async () => {
    try { await App().DeleteAgentsDocFromSource(); closeModal(); toast("Fiche supprimée", "ok"); renderAssistantIA(); }
    catch (e) { toast(cleanErr(e), "err"); }
  });
  if (el("modal-ok")) { el("modal-ok").textContent = "Supprimer"; el("modal-ok").classList.add("danger-btn"); }
}

async function saveAgentsDocToSource() {
  try {
    const p = await App().SaveAgentsDocToSource();
    toast("Fiche enregistrée : " + shortPath(p), "ok");
    renderAssistantIA();
  } catch (e) { toast(cleanErr(e), "err"); }
}

// aiAgentModal : point d'entrée historique « faisons connaissance avec votre agent IA ». Depuis 1.17,
// toute la mise en route IA (fiche AGENTS.md + brancher son agent) vit dans la page « Agent IA » du
// menu principal : on y renvoie simplement, plutôt que de dupliquer le parcours dans une modale.
function aiAgentModal() {
  try { closeModal(); } catch (e) {}
  navTo("ia");
}

async function aiAgentShow(kind) {
  document.querySelectorAll(".ai-pick").forEach((b) => b.classList.toggle("active", b.dataset.k === kind));
  const out = el("ai-out");
  if (!out) return;
  let exe = "situlab";
  try { exe = (await App().ExePath()) || "situlab"; } catch (e) {}
  // Chemin de la fiche administrée à la source (si elle a été enregistrée) : on l'inclut dans le
  // message de présentation pour que l'agent la lise au bon endroit (v1.17, besoin #8).
  let docPath = "";
  try { if (await App().AgentsDocExists()) docPath = await App().AgentsDocPath(); } catch (e) {}
  let mcpBlock = "", cfg = "";
  if (kind === "mcp") {
    cfg = JSON.stringify({ mcpServers: { situlab: { command: exe, args: ["mcp"] } } }, null, 2);
    mcpBlock = `<div class="ai-step"><div class="ai-step-h">1. Ajoutez SituLab à votre agent MCP</div>
      <p class="sub">Collez ceci dans la configuration MCP de votre agent (Claude Desktop, Cursor…) :</p>
      <pre class="filebox" style="user-select:text;white-space:pre-wrap">${esc(cfg)}</pre>
      <button type="button" class="btn btn-ghost btn-sm" id="ai-cfg">Copier la configuration</button></div>`;
  }
  // La fiche AGENTS.md (dépôt, chemin à la source) est gérée par la carte « Fiche de collaboration »
  // au-dessus : on ne la redouble plus ici (c'était redondant). Ne reste que le message de présentation,
  // qui pointe déjà vers la fiche enregistrée à la source.
  const step1 = kind === "mcp" ? "2" : "1";
  out.innerHTML = mcpBlock + `
    <div class="ai-step"><div class="ai-step-h">${step1}. Dites bonjour à votre agent</div>
      <p class="sub">Copiez ce message et collez-le dans votre assistant : il lui dit où est SituLab, l'invite à lire la fiche de collaboration (au chemin enregistré à la source), puis à lister vos KERNEL et spokes et à vous proposer lequel ouvrir.</p>
      <button type="button" class="btn btn-primary btn-sm" id="ai-prompt">Copier le message de présentation</button></div>`;
  if (el("ai-cfg")) el("ai-cfg").onclick = () => copyPrompt(cfg, { close: false, title: "Configuration MCP" });
  el("ai-prompt").onclick = () => copyPrompt(firstPromptText(kind, exe, docPath), { close: false, title: "Message de présentation" });
}

// installDirectives : choisit un dossier et y écrit le guide AGENTS.md pour l'agent de l'utilisateur.
async function installDirectives() {
  try {
    const dir = await App().PickFolder();
    if (!dir) return;
    const p = await App().InstallAgentsDoc(dir);
    toast("Fichier agent placé dans : " + shortPath(p), "ok");
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
    toast("Prompt copié, collez-le dans votre IA", "ok");
    if (close) closeModal();
  } catch (e) {
    infoModal(opts.title || "Prompt pour votre IA", `<p class="sub">Copie automatique refusée : sélectionnez puis copiez (Ctrl+C), et collez dans votre IA :</p><pre class="filebox" style="user-select:text;white-space:pre-wrap">${esc(text)}</pre>`);
  }
}

// aiInviteHTML : bloc « laissez votre IA le faire » (même habillage partout). À placer dans
// une modale CRUD ; appeler ensuite wireAiInvite(prompt) après openModal pour brancher le clic.
function aiInviteHTML(sub) {
  return `<div class="ai-invite">
    <div class="ai-invite-txt"><span class="ai-bot">${ico("robot", 16)}</span><div><b>Vous pouvez déléguer ce geste à votre IA.</b><div class="sub">${esc(sub || "Copiez le prompt et collez-le dans votre IA. SituLab gère seul la validation si elle est nécessaire.")}</div></div></div>
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
  // Identité alignée sur l'instance COURANTE : on peut arriver ici directement depuis la carte d'une
  // AUTRE connexion (lien « utilisateurs »). Sans réaligner, le gate admin et le login refléteraient une
  // instance tierce (droits mal évalués, actions sous le mauvais compte). Rafraîchi seulement au changement.
  if (state.admInstance !== state.instance) {
    state.admInstance = state.instance;
    try { const me = await App().Whoami(state.instance); if (me) { state.login = me.login; state.isAdmin = !!me.is_admin; } } catch (e) {}
  }
  // Réservé aux administrateurs : sinon, message doux plutôt qu'une erreur sèche.
  if (!state.isAdmin) {
    el("view").innerHTML = `<div class="view-head"><div class="view-title">Administration</div></div>` +
      blankslate("⌘", "Réservé aux administrateurs", "Cet espace sert à gérer les comptes de l'équipe. Votre compte n'a pas les droits pour l'ouvrir, demandez à un administrateur si vous pensez que c'est une erreur.");
    return;
  }
  // Compte admin, mais l'accès SituLab n'a pas (encore) le droit de gestion : on propose de
  // l'activer à la demande (v1.4.2 — plus de case « admin » à la connexion).
  const hasScope = await App().AdminScopeFor(state.instance).catch(() => true);
  if (!hasScope) {
    el("view").innerHTML = userMgmtHeader("admin") +
      `<div class="callout"><b>Activer l'administration ?</b><div class="sub">Votre compte est administrateur, mais votre accès SituLab actuel n'inclut pas encore le droit de gérer les comptes. Activez-le pour continuer : SituLab recrée votre accès avec ce droit (identifiant + mot de passe + votre PIN).</div>
      <button class="btn btn-primary btn-sm" id="activate-admin" style="margin-top:10px">Activer l'administration</button></div>`;
    wireUserMgmtTabs();
    el("activate-admin").onclick = () => reconnectModal(state.instance, { admin: true });
    return;
  }
  el("view").innerHTML = userMgmtHeader("admin") + `
    <div class="view-sub" style="margin:-6px 0 10px">Gérer les comptes qui peuvent utiliser « ${esc(state.instance)} ».</div>
    <div class="toolbar">
      <button class="btn btn-primary btn-sm" id="new-user">${ico("plus", 14)} Nouvel utilisateur</button>
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

