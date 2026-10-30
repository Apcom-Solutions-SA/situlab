// Frontal GUI de SituLab — tableau de bord vivant « distant ⇄ local ».
// N'appelle QUE le noyau Go (window.go.main.App.*). Aucune logique métier ici :
// la parité GUI/CLI est structurelle (même core.Service des deux côtés).

const App = () => window.go.main.App;
const el = (id) => document.getElementById(id);
// Corbeille discrète pour les actions « supprimer / retirer » des listes et cartes (1.17) : icône seule,
// atténuée, rouge au survol (classe .icon-del). Définie ici (1er script) → disponible partout. Le texte
// « supprimer » encombrait les rangées ; l'intitulé passe en title/aria-label pour rester accessible.
const ICO_TRASH = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 7h16M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2M6 7l1 13a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-13M10 11v6M14 11v6"/></svg>`;

// --- Registre d'icônes UNIQUE (1.18.3) ------------------------------------------------------------
// Une seule bibliothèque : trait 1.7, viewBox 24, currentColor, MONOCHROME. La couleur est réservée
// au SENS (états : synchro, santé, « en cours »), JAMAIS à une icône d'action. Remplace les emoji,
// rendus et colorés par l'OS (cause du « parfois couleur, parfois monochrome »). Les <svg> inline
// sont autorisés par la CSP (script-src 'self' : ce n'est pas du script). ico("folder"[, taille]).
const ICON_PATHS = {
  folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>',
  file: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8Z"/><path d="M14 3v5h5"/>',
  repo: '<path d="M12 3 3 7.5 12 12l9-4.5Z"/><path d="M3 7.5v9L12 21l9-4.5v-9"/><path d="M12 12v9"/>',
  eye: '<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="2.6"/>',
  eyeOff: '<path d="M3 3l18 18"/><path d="M10.6 5.2A10 10 0 0 1 12 5c6.4 0 10 7 10 7a18 18 0 0 1-3 3.9M6.1 6.2A18 18 0 0 0 2 12s3.6 7 10 7a10 10 0 0 0 3.9-.8"/><path d="M9.5 9.6a2.6 2.6 0 0 0 3.6 3.6"/>',
  branch: '<circle cx="6" cy="6" r="2.3"/><circle cx="6" cy="18" r="2.3"/><circle cx="18" cy="8" r="2.3"/><path d="M6 8.3v7.4"/><path d="M18 10.3a6 6 0 0 1-6 6H8.3"/>',
  trash: '<path d="M4 7h16M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2M6 7l1 13a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-13M10 11v6M14 11v6"/>',
  save: '<path d="M5 3h11l3 3v13a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2Z"/><path d="M7 3v5h8V3"/><rect x="7" y="13" width="10" height="6" rx="1"/>',
  clock: '<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/>',
  cloud: '<path d="M7 18a4 4 0 0 1-.5-7.97A5.5 5.5 0 0 1 17 9.5a3.5 3.5 0 0 1 .5 8Z"/>',
  robot: '<rect x="4" y="8" width="16" height="11" rx="2.5"/><path d="M12 4.5V8M8.5 13h.01M15.5 13h.01M2.5 12.5v2M21.5 12.5v2"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.2-3.2"/>',
  upload: '<path d="M12 16V4M7 9l5-5 5 5"/><path d="M5 20h14"/>',
  download: '<path d="M12 4v12M7 11l5 5 5-5"/><path d="M5 20h14"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  check: '<path d="M5 12.5 10 17 19 7"/>',
  chevronLeft: '<path d="M14 6l-6 6 6 6"/>',
  chevronRight: '<path d="M10 6l6 6-6 6"/>',
  chevronUp: '<path d="M6 14l6-6 6 6"/>',
  chevronDown: '<path d="M6 10l6 6 6-6"/>',
  gear: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-2.7 1.1V21a2 2 0 0 1-4 0v-.1a1.6 1.6 0 0 0-2.7-1.1l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.6 1.6 0 0 0 4.6 15H4.5a2 2 0 0 1 0-4h.1a1.6 1.6 0 0 0 1.1-2.7l-.1-.1A2 2 0 1 1 8.4 5.4l.1.1a1.6 1.6 0 0 0 2.7-1.1V4.5a2 2 0 0 1 4 0v.1a1.6 1.6 0 0 0 2.7 1.1l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-1.1 2.7Z"/>',
  link: '<path d="M10 14a5 5 0 0 0 7 0l2.5-2.5a5 5 0 0 0-7-7L11 5"/><path d="M14 11a5 5 0 0 0-7 0L4.5 13.5a5 5 0 0 0 7 7L13 19"/>',
  warning: '<path d="M12 3 2 20h20L12 3Z"/><path d="M12 9v5M12 17h.01"/>',
  shield: '<path d="M12 3 5 6v6c0 4 3 7 7 9 4-2 7-5 7-9V6Z"/>',
  shared: '<path d="M7 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6ZM2 20a5 5 0 0 1 10 0"/><path d="M15 8a3 3 0 1 1 0 6M22 20a5 5 0 0 0-5-5"/>',
  pen: '<path d="M4 20h4L19 9a2 2 0 0 0-3-3L5 17Z"/><path d="M14 6l3 3"/>',
  tag: '<path d="M3 11.5V5a2 2 0 0 1 2-2h6.5L21 12.5 12.5 21Z"/><circle cx="7.5" cy="7.5" r="1.3"/>',
  external: '<path d="M14 4h6v6M20 4l-9 9"/><path d="M19 13v6a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h6"/>',
  lock: '<rect x="4.5" y="10" width="15" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>',
  palette: '<path d="M12 3a9 9 0 0 0 0 18c1.4 0 2-.9 2-1.9a2 2 0 0 1 2-2h1.2A3.8 3.8 0 0 0 21 13.3C21 7.6 17 3 12 3Z"/><circle cx="7.5" cy="12" r="1" fill="currentColor" stroke="none"/><circle cx="10" cy="8" r="1" fill="currentColor" stroke="none"/><circle cx="15" cy="8.5" r="1" fill="currentColor" stroke="none"/>',
  puzzle: '<path d="M9 4a2 2 0 1 1 4 0h3a1 1 0 0 1 1 1v3a2 2 0 1 1 0 4v3a1 1 0 0 1-1 1h-3a2 2 0 1 0-4 0H6a1 1 0 0 1-1-1v-3a2 2 0 1 0 0-4V5a1 1 0 0 1 1-1Z"/>',
  activity: '<path d="M3 12h4l2 6 4-14 2 8h6"/>',
  help: '<circle cx="12" cy="12" r="9"/><path d="M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.8.4-1 .9-1 1.7"/><path d="M12 17h.01"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  desktop: '<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/>',
  rocket: '<path d="M5 15c-1.5 1.5-2 5-2 5s3.5-.5 5-2M9.5 12.5l2 2M14.5 3.5C18 2 21 5 19.5 8.5 18 12 9.5 16.5 9.5 16.5l-2-2S6 6 9.5 3.6c1.6-1.1 3.5-1 5-.1Z"/><circle cx="14.5" cy="8.5" r="1.3"/>',
  key: '<circle cx="8" cy="15" r="4"/><path d="M11 12l8-8M17 4l2 2M14.5 6.5l2 2"/>',
  unlock: '<rect x="4.5" y="10" width="15" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 7.7-1.5"/>',
  chat: '<path d="M4 5h16a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H9l-4 4V6a1 1 0 0 1 1-1Z"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a15 15 0 0 1 0 18 15 15 0 0 1 0-18Z"/>',
  switch: '<path d="M16 3h5v5M21 3l-8 8M8 21H3v-5M3 21l8-8"/>',
  // balai (1.18.5) : « purger » = NETTOYER les données accumulées, pas supprimer l'agent.
  broom: '<path d="M20 4 10.5 13.5"/><path d="M9 12 12 15 9 19 5 16Z"/><path d="M10 13 6.3 17"/><path d="M11 14 7.7 18"/>',
};
function ico(name, size) {
  const p = ICON_PATHS[name]; if (!p) return "";
  const s = size || 16;
  return `<svg class="ico" viewBox="0 0 24 24" width="${s}" height="${s}" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${p}</svg>`;
}
const esc = (s) => (s == null ? "" : String(s).replace(/[&<>"'\/]/g, (c) =>
  ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;", "/": "&#47;" }[c])));

// pageHero (1.18.4, retour #1) : en-tête de page « hero » décliné par univers — même langage que la
// carte Agent IA, paramétré par la teinte d'univers via --hue (kernel/corpus/spokes/backup/accent).
//   hue       : clé de couleur CSS sans le préfixe (ex. "kernel") → --hue:var(--kernel).
//   icoHTML   : SVG déjà rendu (NAV_ICO.* ou ico()).
//   title     : titre (texte simple, déjà sûr).
//   sub       : sous-titre optionnel (phrase sobre).
//   actionsHTML : boutons à droite (IDs conservés pour les bindings existants).
function pageHero(hue, icoHTML, title, sub, actionsHTML) {
  return `<div class="page-hero" style="--hue:var(--${hue || "accent"})">
    <div class="page-hero-ico">${icoHTML}<span class="page-hero-ring" aria-hidden="true"></span></div>
    <div class="page-hero-txt"><div class="view-title">${esc(title)}</div>${sub ? `<div class="page-hero-sub">${sub}</div>` : ""}</div>
    ${actionsHTML ? `<div class="page-hero-actions">${actionsHTML}</div>` : ""}
  </div>`;
}

// klinkViz (1.18.4) : visualisation futuriste et dynamique d'un lien KERNEL <-> SPOKE.
//   selfKind  : "kernel" | "spoke" — le nœud de gauche, l'entité de la page courante.
//   selfLabel : son nom.
//   links     : [{kind, label}] des entités reliées (vide = aucun lien → état dormant, sans flux).
//   emptyText : libellé quand aucun lien.
// Fil ANIMÉ (point qui circule) quand un lien existe ; au repos et éteint sinon (même code que le
// schéma CORPUS). Réutilise l'animation pcxFlow. NAV_ICO est résolu à l'appel (défini ailleurs).
function klinkViz(selfKind, selfLabel, links, emptyText, hideSelf) {
  const node = (kind, label) => `<span class="kl-node kl-${kind === "kernel" ? "kernel" : "spoke"}">${kind === "kernel" ? NAV_ICO.kernel : NAV_ICO.spokes}<span class="kl-nname" title="${esc(label)}">${esc(label)}</span></span>`;
  const active = !!(links && links.length);
  const self = hideSelf ? "" : node(selfKind, selfLabel);
  const wire = `<span class="kl-wire${active ? "" : " off"}" aria-hidden="true"><i></i></span>`;
  if (!active) {
    return `<div class="klink off${hideSelf ? " noself" : ""}">${self}${wire}<span class="kl-empty">${esc(emptyText)}</span></div>`;
  }
  return `<div class="klink${hideSelf ? " noself" : ""}">${self}${wire}<span class="kl-others">${links.map((l) => node(l.kind, l.label)).join("")}</span></div>`;
}

// --- Cartes « noyau » KERNEL/SPOKE (1.18.4) : le cœur du logiciel, traité premium ------------------
// Métaphore : un NOYAU (KERNEL) et ses SATELLITES (spokes) en orbite, reliés par un flux animé. La même
// constellation sert les deux pages, centrée sur l'entité courante.

// constellHTML : mini-constellation « noyau -> satellites ». coreIcon/coreLabel = le noyau (gauche) ;
// sats = [{icon, label, self}] satellites (droite) ; cut = liens rompus ; emptyText si aucun satellite.
function constellHTML(coreIcon, coreLabel, sats, cut, emptyText) {
  const core = `<div class="cst-core${coreLabel ? "" : " nolabel"}${cut ? " empty" : ""}">${coreIcon}${coreLabel ? `<span class="cst-core-l" title="${esc(coreLabel)}">${esc(coreLabel)}</span>` : ""}<i class="cst-orbit" aria-hidden="true"></i></div>`;
  const branch = (inner, isCut) => `<div class="cst-branch"><span class="cst-wire${isCut ? " cut" : ""}" aria-hidden="true"><i></i></span>${inner}</div>`;
  const body = (sats && sats.length)
    ? sats.map((s) => branch(s.html || `<span class="cst-sat${s.self ? " self" : ""}">${s.icon}<span>${esc(s.label)}</span></span>`, cut)).join("")
    : branch(`<span class="cst-empty">${esc(emptyText)}</span>`, true);
  return `<div class="cst">${core}<div class="cst-branches">${body}</div></div>`;
}

// kcoreCardHTML : carte « noyau » premium. Purpose-built (ne réutilise pas le classeur générique) mais
// préserve les data-* pour la délégation de clic : data-open-project sur la carte, data-remove sur la
// corbeille ; les boutons d'édition (data-run / data-parent) sont fournis dans constellation/entry.
function kcoreCardHTML(kind, p, badge, constellation, entry, removeTitle) {
  const atl = p.atelier || {};
  const v = classeurVerdict(p);
  const icon = kind === "kernel" ? NAV_ICO.kernel : NAV_ICO.spokes;
  return `<div class="kcore kcore-${kind}" data-open-project="${esc(p.name)}" role="button" tabindex="0" aria-label="${esc(p.name)} (${esc(badge)}) : ouvrir le détail">
    <div class="kcore-head">
      <span class="kcore-medal">${icon}<i class="kcore-orbit" aria-hidden="true"></i></span>
      <div class="kcore-id">
        <div class="kcore-name">${esc(p.name)}<span class="kcore-badge">${esc(badge)}</span></div>
        <div class="kcore-sub"><span class="kcore-k">atelier</span> <span class="org">${esc(atl.owner || "")}/</span>${esc(atl.repo || "")}</div>
      </div>
      <button type="button" class="btn-icon danger kcore-remove" data-remove="${esc(p.name)}" title="${esc(removeTitle)}" aria-label="${esc(removeTitle)}">${ICO_TRASH}</button>
    </div>
    <div class="kcore-status">${chealthHTML(p)}<span class="kcore-state" data-tone="${v.tone}">${esc(v.label)}</span></div>
    <div class="kcore-section">${constellation}</div>
    <div class="kcore-section kcore-entry">${entry}</div>
  </div>`;
}

// kcoreKeydown : ouvre le détail au clavier (Entrée/Espace) sur une carte noyau focalisée — le classeur
// était un <button>, la carte noyau est un div[role=button], on rétablit l'accès clavier.
function kcoreKeydown(ev) {
  if (ev.key !== "Enter" && ev.key !== " ") return;
  const card = ev.target.closest(".kcore[data-open-project]");
  if (!card || ev.target.closest("button")) return;
  ev.preventDefault();
  if (typeof openProject === "function") openProject(card.dataset.openProject);
}

// --- Mise en cohérence KERNEL <-> SPOKE (1.18.4) : l'IA remonte les enrichissements d'un spoke dans le
// KERNEL, fichier par fichier, sous validation humaine (« les spokes nourrissent, ne polluent pas »).
// SituLab ne fait QUE proposer un prompt à copier : aucune mutation, aucun PIN ici. ------------------

// Glyphe « cohérence » (flèche circulaire). Monochrome (pas d'emoji).
const CO_ICON = `<svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 2.7-6.4"/><path d="M3 4v5h5"/></svg>`;

// satSpoke : satellite spoke d'une constellation, avec son bouton « mise en cohérence » (⟲) discret.
//   nav=true : le corps de la pastille navigue vers le détail (data-goto). La cohérence passe toujours
//   par le petit bouton (data-coherence + data-proc = fichier de gestion des spokes du KERNEL parent).
function satSpoke(name, procFile, nav) {
  const chip = nav
    ? `<button type="button" class="cst-sat cst-nav" data-goto="${esc(name)}" title="Ouvrir « ${esc(name)} »">${NAV_ICO.spokes}<span>${esc(name)}</span></button>`
    : `<span class="cst-sat">${NAV_ICO.spokes}<span>${esc(name)}</span></span>`;
  const coh = `<button type="button" class="cst-coh" data-coherence="${esc(name)}" data-proc="${esc(procFile || "")}" title="Mise en cohérence du KERNEL avec « ${esc(name)} »" aria-label="Mise en cohérence avec ${esc(name)}">${CO_ICON}</button>`;
  return `<span class="cst-satwrap">${chip}${coh}</span>`;
}

// coherencePromptText : le prompt à coller dans l'agent, adapté terminal / MCP (logique habituelle,
// sobre). procFile = fichier de gestion des spokes lié au KERNEL (facultatif).
function coherencePromptText(kind, exe, spokeName, procFile) {
  exe = exe || "situlab";
  const isMcp = kind === "mcp";
  const acces = isMcp
    ? "Tu me pilotes SituLab par les outils MCP « situlab » : lectures libres, toute écriture distante passe par mon PIN sur le poste."
    : `SituLab est ici : "${exe}". Appelle-le par ce chemin complet (ex. "${exe}" --help). Toute écriture distante passe par mon PIN sur le poste.`;
  const reche = isMcp
    ? "Récupère le spoke avec les outils SituLab (repère-le d'abord sur la carte de mes dépôts, puis clone-le)."
    : "Récupère le spoke avec SituLab (repère-le d'abord par « situlab map », puis clone-le).";
  const lines = [
    `Mets le KERNEL en cohérence avec le spoke « ${spokeName} » : consolidation périodique, sans fermeture.`,
    "",
    acces,
  ];
  if (procFile) lines.push(`Suis la procédure de gestion des spokes : "${procFile}".`);
  lines.push(reche);
  lines.push("Compare-le au KERNEL et propose-moi les remontées fichier par fichier ; je valide chacune. Les spokes nourrissent, ne polluent pas : rien n'entre dans le KERNEL sans ma validation. La fusion dans le KERNEL (merge --no-ff) et toute écriture distante restent sous mon PIN.");
  return lines.join("\n");
}

// coherenceModal : modale sobre proposant le prompt (bascule terminal / MCP + copie). Copie seule.
async function coherenceModal(spokeName, procFile) {
  let exe = "situlab";
  try { exe = (await App().ExePath()) || "situlab"; } catch (_) {}
  let kind = "cli";
  openModal(`Mise en cohérence : ${esc(spokeName)}`, `<div class="field">
    <p class="sub">Un prompt à coller dans votre agent : il remonte les enrichissements de ce spoke dans le KERNEL, fichier par fichier, sous votre validation. Aucune écriture sans votre PIN.</p>
    <div class="seg coh-seg">
      <button type="button" class="seg-btn active" data-k="cli">${ico("desktop", 13)} Terminal</button>
      <button type="button" class="seg-btn" data-k="mcp">${ico("puzzle", 13)} MCP</button>
    </div>
    <pre class="filebox coh-pre" id="coh-pre" style="user-select:text;white-space:pre-wrap"></pre>
  </div>`, () => { copyPrompt(coherencePromptText(kind, exe, spokeName, procFile), { close: false, title: "Prompt de mise en cohérence" }); });
  el("modal-ok").textContent = "Copier le prompt";
  el("modal-cancel").textContent = "Fermer";
  const seg = document.querySelector(".coh-seg");
  const paint = () => { const p = el("coh-pre"); if (p) p.textContent = coherencePromptText(kind, exe, spokeName, procFile); };
  if (seg) seg.querySelectorAll("[data-k]").forEach((b) => b.onclick = () => { kind = b.dataset.k; seg.querySelectorAll("[data-k]").forEach((x) => x.classList.toggle("active", x === b)); paint(); });
  paint();
}
const cleanErr = (e) => String((e && e.message) || e).replace(/^Error:\s*/, "");

// --- Réactivité : stale-while-revalidate (1.18.3) -------------------------------------------------
// swr(clé, chargeur, peindre, opts) peint D'ABORD le dernier état connu (mémoire, puis disque si
// persistant), rafraîchit en arrière-plan via chargeur() (réseau) et ne repeint QUE si la signature
// change. Supprime la « boîte vide » du premier affichage. opts.persist=false → cache MÉMOIRE seul
// (le CORPUS n'est jamais écrit sur disque — souveraineté, Option A ; le cœur refuse de toute façon).
//   peindre(donnée, { stale })  stale=true : donnée connue (peut-être un peu ancienne) ; false : fraîche.
//   opts.alive()  optionnel : renvoie false si la vue a changé (on cesse de peindre).
const __swrMem = new Map();
async function swr(key, loader, paint, opts) {
  opts = opts || {};
  const alive = opts.alive || (() => true);
  const persist = opts.persist !== false;
  let had = false;
  if (__swrMem.has(key)) {
    try { paint(__swrMem.get(key).data, { stale: true }); had = true; } catch (_) {}
  } else if (persist) {
    try {
      const raw = await App().UICacheLoad(key);
      if (raw && alive()) {
        const data = JSON.parse(raw);
        __swrMem.set(key, { data, sig: raw });
        paint(data, { stale: true }); had = true;
      }
    } catch (_) {}
  }
  let fresh;
  try { fresh = await loader(); }
  catch (e) { if (had) { const m = __swrMem.get(key); return m && m.data; } throw e; }
  if (!alive()) return fresh;
  const sig = JSON.stringify(fresh);
  const prev = __swrMem.get(key);
  const changed = !prev || prev.sig !== sig;
  __swrMem.set(key, { data: fresh, sig });
  if (changed) {
    try { paint(fresh, { stale: false }); } catch (_) {}
    if (persist) { try { App().UICacheSave(key, sig); } catch (_) {} }
  }
  return fresh;
}
// swrPeek : donnée connue en mémoire (sans réseau), ou undefined. swrDrop : invalide une clé.
function swrPeek(key) { const e = __swrMem.get(key); return e && e.data; }
function swrDrop(key) { __swrMem.delete(key); }

// --- Palette de tags / pastilles partagée (P2 tags colorés + P8 icône projet), v1.18.2 ---------
// Clés stables stockées en config ; le rendu (couleur) vit ici. Lisibles en thème clair et sombre.
const TAG_PALETTE = [
  { key: "slate", bg: "#64748b" }, { key: "blue", bg: "#3b82f6" }, { key: "teal", bg: "#14b8a6" },
  { key: "green", bg: "#22c55e" }, { key: "amber", bg: "#f59e0b" }, { key: "orange", bg: "#f97316" },
  { key: "red", bg: "#ef4444" }, { key: "pink", bg: "#ec4899" }, { key: "purple", bg: "#a855f7" },
  { key: "cyan", bg: "#06b6d4" },
];
const TAG_COLOR = (key) => (TAG_PALETTE.find((c) => c.key === key) || TAG_PALETTE[0]).bg;
// tagAutoColor : couleur déterministe quand aucune n'est choisie (hash du nom → palette).
function tagAutoColor(name) {
  let h = 0; const s = String(name || "");
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return TAG_PALETTE[h % TAG_PALETTE.length].key;
}
// tagPillHTML : pastille de tag colorée. colorKey vide → couleur auto déterministe.
function tagPillHTML(name, colorKey, extraClass) {
  const c = TAG_COLOR(colorKey || tagAutoColor(name));
  return `<span class="tag-pill2 ${extraClass || ""}" style="--tc:${c}"><i class="tp-dot"></i>${esc(name)}</span>`;
}
// initialsOf : 1-2 initiales d'un nom (pour la pastille d'icône projet).
function initialsOf(name) {
  const parts = String(name || "").trim().split(/[\s\-_.]+/).filter(Boolean);
  if (!parts.length) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}
// projectIconHTML : rend l'icône d'un projet. « color:<key>|<initiales> », « upload:<dataURL> » (v1.18.2),
// emoji/picto ou dataURL (legacy), sinon une pastille auto (initiales + couleur déterministe).
function projectIconHTML(icon, name, size) {
  const sz = size || 34;
  const box = (inner, style) => `<span class="proj-ico" style="width:${sz}px;height:${sz}px;${style || ""}">${inner}</span>`;
  const s = String(icon || "");
  if (s.startsWith("color:")) {
    const rest = s.slice(6), bar = rest.indexOf("|");
    const key = bar >= 0 ? rest.slice(0, bar) : rest;
    const ini = bar >= 0 ? rest.slice(bar + 1) : initialsOf(name);
    return box(esc(ini), `background:${TAG_COLOR(key)};color:#fff;font-weight:700;font-size:${Math.round(sz * 0.42)}px`);
  }
  if (s.startsWith("upload:")) return box(`<img src="${esc(s.slice(7))}" alt="" />`);
  if (s.startsWith("data:")) return box(`<img src="${esc(s)}" alt="" />`);
  if (s) return box(esc(s), `font-size:${Math.round(sz * 0.6)}px`);
  return box(esc(initialsOf(name)), `background:${TAG_COLOR(tagAutoColor(name))};color:#fff;font-weight:700;font-size:${Math.round(sz * 0.42)}px`);
}

const CODELEN = 6; // longueur du PIN et des codes 2FA (chiffres)
// codeInput transforme un conteneur en CODELEN cases d'un chiffre (PIN / 2FA) : avance
// automatique, Backspace recule, collage d'un code complet, et appelle onComplete(code) dès
// que toutes les cases sont remplies (plus de bouton à cliquer). mask=true masque (PIN).
function codeInput(mount, mask, onComplete, autofocus) {
  mount.innerHTML = "";
  mount.className = "code-input";
  mount.setAttribute("role", "group");
  mount.setAttribute("aria-label", mask ? "Code PIN à 6 chiffres" : "Code à 6 chiffres");
  const boxes = [];
  for (let i = 0; i < CODELEN; i++) {
    const b = document.createElement("input");
    b.type = mask ? "password" : "text";
    b.inputMode = "numeric"; b.maxLength = 1; b.autocomplete = "off"; b.className = "code-box";
    b.setAttribute("aria-label", "Chiffre " + (i + 1));
    boxes.push(b); mount.appendChild(b);
  }
  const read = () => boxes.map((b) => b.value).join("");
  const maybe = () => { if (onComplete && boxes.every((b) => b.value)) onComplete(read()); };
  boxes.forEach((b, i) => {
    b.oninput = () => { b.value = b.value.replace(/\D/g, "").slice(0, 1); b.classList.toggle("filled", !!b.value); if (b.value && i < CODELEN - 1) boxes[i + 1].focus(); maybe(); };
    b.onkeydown = (e) => { if (e.key === "Backspace" && !b.value && i > 0) boxes[i - 1].focus(); };
    b.onpaste = (e) => { e.preventDefault(); const t = ((e.clipboardData || window.clipboardData).getData("text") || "").replace(/\D/g, "").slice(0, CODELEN); for (let j = 0; j < t.length; j++) { boxes[j].value = t[j]; boxes[j].classList.add("filled"); } boxes[Math.min(t.length, CODELEN - 1)].focus(); maybe(); };
  });
  if (autofocus) {
    boxes[0].setAttribute("autofocus", ""); // indice natif pour la WebView
    // Met le curseur dans la 1re case. À l'ouverture de l'app la fenêtre n'a parfois pas
    // encore le focus clavier (focus() échoue alors) : on insiste par salves pendant ~3 s,
    // et dès que la fenêtre reçoit le focus, jusqu'à ce qu'une case soit réellement active.
    try { App().FocusWindow(); } catch (e) {} // donne le focus clavier à la fenêtre native
    const f = () => {
      if (!document.body.contains(boxes[0])) return true; // écran remplacé → stop
      try { window.focus(); } catch (e) {}
      boxes[0].focus({ preventScroll: true });
      return document.activeElement === boxes[0];
    };
    requestAnimationFrame(f);
    let n = 0;
    const iv = setInterval(() => { if (f() || ++n > 30) clearInterval(iv); }, 100);
    const onWin = () => { if (f()) window.removeEventListener("focus", onWin); };
    window.addEventListener("focus", onWin);
    setTimeout(() => window.removeEventListener("focus", onWin), 8000);
  }
  return { read, clear: () => { boxes.forEach((b) => (b.value = "")); if (boxes[0]) boxes[0].focus(); } };
}
// La session peut se re-verrouiller en cours d'usage (timeout d'inactivité) : on détecte
// l'erreur « session verrouillée » et on repropose le déverrouillage au lieu de boucler.
const isLocked = (e) => /verrouill/i.test(cleanErr(e));
// isAuthError : l'erreur vient d'un jeton refusé par le serveur (révoqué/expiré/droits), pas d'un
// simple réseau coupé. Déclenche la proposition de RECONNEXION (recréer l'accès).
const isAuthError = (e) => /accès refusé|token invalide|non autoris|unauthoriz|forbidden|identifiants|401|403/i.test(cleanErr(e));
// pingInstance : état réel d'une connexion — "ok", "auth" (jeton refusé), "net" (injoignable).
// Un verrouillage n'est pas une déconnexion : on ne stigmatise pas la connexion dans ce cas.
async function pingInstance(name) {
  try { await App().Whoami(name); return "ok"; }
  catch (e) { if (isLocked(e)) return "ok"; return isAuthError(e) ? "auth" : "net"; }
}
let relocking = false;
// pinModalOpen : vrai tant qu'une fenêtre PIN DÉCLENCHÉE PAR LE DÉMON (situlab:pin-request) est à
// l'écran. Cette fenêtre déverrouille DÉJÀ la session (le démon fait Unlock/gatePIN avec le PIN saisi) :
// pendant qu'elle est ouverte, le poll d'arrière-plan ne doit PAS empiler par-dessus son propre écran de
// déverrouillage « verrouillé », sinon l'utilisateur ressaisit son PIN pour rien (double demande).
let pinModalOpen = false;
let lockGrace = false;
// handleLock : une lecture a échoué sur « session verrouillée ». AVANT de montrer l'écran de
// déverrouillage STANDARD, on laisse une courte fenêtre à une éventuelle demande de PIN d'AGENT
// (situlab:pin-request) de prendre la main : si c'est l'IA qui provoque ce verrou, c'est SA fenêtre
// contextuelle qui doit s'afficher (avec ce qui est demandé), et elle doit ÉPARGNER l'écran standard.
// L'écran standard ne reste donc que pour un verrouillage SANS demande d'agent (inactivité, ouverture
// humaine). Le démon ré-émet sa demande jusqu'à réponse (voir prompter.go), ce qui garantit qu'une
// demande d'agent en cours finit par lever pinModalOpen pendant cette fenêtre de grâce.
async function handleLock() {
  if (relocking || lockGrace) return;
  if (pinModalOpen) return; // la fenêtre agent gère déjà le (dé)verrouillage
  lockGrace = true;
  for (let i = 0; i < 14; i++) {
    if (pinModalOpen) { lockGrace = false; return; } // une demande d'agent a pris la main
    await sleep(60);
  }
  lockGrace = false;
  if (pinModalOpen || relocking) return;
  relocking = true;
  stopLive();
  lockedScreen(); // page accueillante « session verrouillée » (gèle l'app, bouton de déverrouillage)
}

// Modèle d'état : une seule source de vérité UI (le code d'état vient du noyau).
// Chaque état = libellé clair + icône + classe couleur + infobulle (terme git) + conseil.
const STATES = {
  uptodate:   { label: "À jour",                  ico: "✓", cls: "s-uptodate",  tip: "Local et serveur synchronisés" },
  ahead:      { label: "À envoyer",               ico: "↑", cls: "s-ahead",     tip: "Des points de sauvegarde locaux ne sont pas encore en ligne (git push)", hint: "Envoyez vos modifications en ligne." },
  behind:     { label: "Nouveautés en ligne",     ico: "↓", cls: "s-behind",    tip: "Le serveur a avancé (git pull)", hint: "Récupérez les nouveautés du serveur." },
  diverged:   { label: "À réunir",                ico: "⇅", cls: "s-diverged",  tip: "Local et serveur ont tous deux avancé", hint: "Les deux côtés ont changé, à réunir." },
  dirty:      { label: "Modifications en cours",  ico: "●", cls: "s-dirty",     tip: "Fichiers modifiés non encore enregistrés (git commit)", hint: "Des changements ne sont pas encore enregistrés." },
  detached:   { label: "Tête détachée",           ico: "⚑", cls: "s-detached",  tip: "HEAD détachée (git, avancé)" },
  unreadable: { label: "Illisible",               ico: "⚠", cls: "s-unreadable", tip: "Le dossier local n'a pas pu être lu" },
  nolocal:    { label: "Pas sur ce poste",        ico: "○", cls: "s-nolocal",   tip: "Dépôt en ligne non copié localement" },
};

const state = {
  instance: "",
  view: "kernel", // accueil = KERNEL (mémoire) ; puis projects | repos | corpus | backup | help | settings | detail
  search: "",
  org: "",
  detail: null,
  repos: null,     // dernier dashboard (cache mémoire)
  repoSig: "",     // signature anti-scintillement
  online: true,
  repoState: "",   // filtre « État » du catalogue de dépôts (1.9.11)
  repoProject: "", // filtre « Projet » du catalogue : "" | "in" | "out" (1.9.11)
  repoTab: "",     // onglet d'emplacement du catalogue : "" (tous) | "locale" | "atelier" | "vitrine" (1.9.11)
  projectsIndex: null, // index « instance/owner/repo -> nom de projet » (atelier+vitrines, zéro réseau)
  users: [],       // cache admin
  advanced: false, // mode avancé : révèle les étapes séparées (envoyer/récupérer/enregistrer), la
                   //               matrice des versions de travail, les outils git locaux (mise de
                   //               côté) et les réglages de dépôt (v1.3)
  checkedAt: 0,    // horodatage du dernier tableau de bord reçu (« vérifié il y a X min »)
  readOnly: false, // mode lecture seule (v1.4) : masque les actions de mutation + bannière
  directory: null, // cache de l'annuaire consolidé (v1.4)
  dirSearch: "",   // filtre de recherche de l'annuaire
  repoView: "grid", // présentation du catalogue de dépôts : "grid" | "list" (v1.5.0)
};
try { if (localStorage.getItem("situlab.repoView") === "list") state.repoView = "list"; } catch (e) {}
// Préférence « mode avancé » mémorisée par poste (confort d'affichage, sans incidence sécurité).
try { state.advanced = localStorage.getItem("situlab.advanced") === "1"; } catch (e) {}
let liveTimer = null;
let liveBusy = false;
let liveTick = 0;

document.addEventListener("DOMContentLoaded", boot);

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// Splash d'ouverture : affiché jusqu'à ce que l'app ou l'onboarding soit prêt,
// avec un minimum pour que le motif terminal soit perçu (pas un flash).
const bootStart = Date.now();
function hideBoot() {
  const b = el("boot"); if (!b) return;
  const wait = Math.max(0, 850 - (Date.now() - bootStart));
  setTimeout(() => b.classList.add("hidden"), wait);
}

// Le runtime Wails peuple window.go.main.App juste après le chargement : on attend
// qu'il soit prêt (jusqu'à ~5 s) plutôt que de supposer qu'il l'est au DOMContentLoaded.
async function boot() {
  applyTheme(readTheme());
  applyZoom(readZoom());
  for (let i = 0; i < 100; i++) {
    if (window.go && window.go.main && window.go.main.App) return init();
    await sleep(50);
  }
  showFatal("Le pont applicatif n'a pas démarré. Fermez puis rouvrez SituLab ; si le problème persiste, vérifiez que WebView2 est installé.");
}

// playPinChime (1.18.4, retour #15) : petit « cling » à l'arrivée d'une demande de PIN déclenchée par
// un agent. Ces demandes passent TOUJOURS par situlab:pin-request (jamais par les actions in-app, qui
// utilisent pinConfirm) : le son cible donc exactement le cas CLI/MCP. Généré par Web Audio — aucun
// fichier, aucune contrainte CSP media-src. Best-effort : toute erreur est silencieuse.
function playPinChime() {
  try {
    const AC = window.AudioContext || window.webkitAudioContext;
    if (!AC) return;
    const ctx = new AC();
    if (ctx.state === "suspended" && ctx.resume) { try { ctx.resume(); } catch (_) {} }
    const now = ctx.currentTime;
    const gain = ctx.createGain();
    gain.connect(ctx.destination);
    gain.gain.setValueAtTime(0.0001, now);
    gain.gain.exponentialRampToValueAtTime(0.18, now + 0.015);
    gain.gain.exponentialRampToValueAtTime(0.0001, now + 0.6);
    [[880, 0], [1320, 0.08]].forEach(([f, t]) => {
      const o = ctx.createOscillator();
      o.type = "sine";
      o.frequency.value = f;
      o.connect(gain);
      o.start(now + t);
      o.stop(now + 0.62);
    });
    setTimeout(() => { try { ctx.close(); } catch (_) {} }, 900);
  } catch (_) {}
}

let pinEventBound = false;
async function init() {
  try { el("version").textContent = "SituLab " + (await App().AppVersion()); } catch (e) {}
  // Le démon (cette fenêtre) demande le PIN sur le bureau quand une commande CLI/agent
  // doit déverrouiller ou confirmer une écriture. Le PIN ne quitte jamais l'app. On ne lie
  // l'écoute qu'une fois (init peut être rejoué après un déverrouillage).
  if (!pinEventBound) {
    try { window.runtime.EventsOn("situlab:pin-request", (payload) => {
      // Charge utile {reason, detail, action, intent}. Compat : une ancienne chaîne seule reste gérée.
      // intent (Point 4) = phrase LIBRE de l'agent, montrée distinctement des actions constatées.
      const p = (payload && typeof payload === "object") ? payload : { reason: payload };
      // Un seul « cling » par demande : les ré-émissions (toutes les 500 ms jusqu'à réponse) retombent
      // ici, mais pinModalOpen est déjà vrai → on ne sonne qu'à la première (retour #15).
      if (!pinModalOpen) playPinChime();
      pinRequestModal(p.reason || "Autoriser l'opération", p.detail || null, p.action || "", p.intent || "", p.deadlineMs || 0);
    }); pinEventBound = true; } catch (e) {}
  }
  let instances = [];
  try { instances = await App().GetInstances(); }
  catch (e) { return showFatal(cleanErr(e)); }
  // La session est verrouillée au démarrage (la clé ne vit que dans le process) : si un PIN
  // existe, il faut déverrouiller avant toute lecture, sinon le dashboard échoue sans issue.
  const pinSet = await App().PinSet().catch(() => false);
  if (!pinSet) {
    // Pas de PIN : fraîche install → onboarding (PIN + 1re instance). Après un reset de PIN
    // (instances conservées mais déconnectées) → recréer un PIN, puis reconnecter.
    if (instances && instances.length > 0) return createPinScreen();
    return showOnboarding();
  }
  if (!(await App().Unlocked().catch(() => false))) {
    // Démarrage à froid : si c'est une demande d'AGENT qui a réveillé l'app, le démon ré-émet
    // "situlab:pin-request" ; on lui laisse une courte fenêtre pour afficher SA fenêtre contextuelle
    // (avec ce qui est demandé) plutôt que l'écran de déverrouillage standard. Sinon (ouverture
    // humaine, sans demande en cours), on bascule sur l'écran standard.
    for (let i = 0; i < 14 && !pinModalOpen; i++) await sleep(60);
    if (pinModalOpen) { hideBoot(); return; } // la fenêtre agent prend la main ; elle rejouera l'app après saisie
    return unlockScreen(() => init()); // après déverrouillage, on rejoue init
  }
  if (!instances || instances.length === 0) showOnboarding();
  else showApp(instances);
}

// createPinScreen : recréer un PIN après « PIN oublié » (les instances existent encore,
// déconnectées). Pas de saisie d'instance ici — on reconnecte ensuite depuis l'app.
function createPinScreen() {
  hideBoot();
  el("app").classList.add("hidden");
  const ob = el("onboarding");
  ob.classList.remove("hidden");
  ob.innerHTML = `<div class="connect-card">
    <img src="logo.svg" class="connect-logo" alt="" />
    <h1>SituLab</h1>
    <p class="tagline">Choisissez un nouveau PIN (6 chiffres).</p>
    <p class="sub">Vos instances seront à reconnecter juste après (identifiant + mot de passe).</p>
    <label>Votre PIN</label><div id="np1"></div>
    <label>Confirmer</label><div id="np2"></div>
    <div id="np-err" class="inline-error hidden" role="alert"></div></div>`;
  const c1 = codeInput(el("np1"), true, null, true);
  const c2 = codeInput(el("np2"), true, async () => {
    const err = el("np-err"); err.classList.add("hidden");
    const a = c1.read(), b = c2.read();
    if (a.length < CODELEN) return;
    if (a !== b) { err.textContent = "Les PIN ne correspondent pas."; err.classList.remove("hidden"); c2.clear(); return; }
    try { await App().SetPin("", a); location.reload(); }
    catch (e) { err.textContent = cleanErr(e); err.classList.remove("hidden"); }
  });
}

// unlockScreen demande le PIN pour ouvrir la session (retour dans l'app, ou onboarding avec
// un PIN déjà défini mais aucune instance). Réutilise la carte de connexion.
function unlockScreen(onDone) {
  hideBoot();
  el("app").classList.add("hidden");
  const ob = el("onboarding");
  ob.classList.remove("hidden");
  ob.innerHTML = `<div class="connect-card">
    <img src="logo.svg" class="connect-logo" alt="" />
    <h1>SituLab</h1>
    <p class="tagline">Entrez votre PIN pour déverrouiller vos accès.</p>
    <div id="u-pin"></div>
    <div id="unlock-error" class="inline-error hidden" role="alert"></div></div>`;
  let busy = false;
  const ci = codeInput(el("u-pin"), true, async (code) => {
    if (busy) return; busy = true;
    const err = el("unlock-error"); err.classList.add("hidden");
    try { await App().Unlock(code); }
    catch (e) { err.textContent = cleanErr(e); err.classList.remove("hidden"); busy = false; ci.clear(); return; }
    // Rechargement : DOM propre (cet écran a remplacé le markup d'onboarding) puis init() rejoue.
    location.reload();
  }, true);
  void onDone; // conservé pour compat d'appel ; le rechargement rejoue le bon écran
}

// lockedScreen : page accueillante affichée quand la session se VERROUILLE pendant que l'app est
// ouverte (délai d'inactivité). Elle GÈLE l'usage (le markup de l'app est masqué, remplacé par cette
// carte) pour qu'aucune action ne parte sur des données périmées, explique calmement l'état, et offre
// un bouton explicite qui lance la saisie du PIN. Distincte de l'écran de déverrouillage d'ouverture :
// ici on NE présume pas que l'utilisateur veut déjà taper son PIN — on l'informe d'abord, il déclenche.
function lockedScreen() {
  hideBoot();
  stopLive(); // plus aucun appel d'arrière-plan tant que c'est verrouillé
  el("app").classList.add("hidden");
  const ob = el("onboarding");
  ob.classList.remove("hidden");
  ob.innerHTML = `<div class="connect-card">
    <img src="logo.svg" class="connect-logo" alt="" />
    <h1>SituLab</h1>
    <div class="lock-badge" aria-hidden="true">${ico("lock", 14)}</div>
    <p class="tagline">Session verrouillée</p>
    <p class="sub">Par sécurité, vos accès ont été protégés après une période d'inactivité. Vos jetons restent chiffrés sur ce poste. Déverrouillez avec votre PIN pour reprendre là où vous en étiez.</p>
    <button class="btn btn-primary" id="lk-unlock">${ico("unlock", 15)} Déverrouiller</button></div>`;
  el("lk-unlock").onclick = () => unlockScreen(() => location.reload());
}

function showFatal(msg) {
  hideBoot();
  el("onboarding").classList.add("hidden");
  el("app").classList.remove("hidden");
  el("view").innerHTML = surfaceError("SituLab n'a pas pu démarrer", msg, null);
}

