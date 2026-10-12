# Journal des modifications — SituLab

Format : [Keep a Changelog](https://keepachangelog.com/fr/). Versions : SemVer.

## [1.9.9] — 2026-10-11

### Interne — maintenabilité du frontend
- **`frontend/dist/main.js` (4835 lignes) découpé en 11 modules** `app/NN-*.js` chargés dans l'ordre par
  `index.html` (plus gros module : ~970 lignes). Découpe **pure** aux frontières de section : aucune
  édition, aucun réordonnancement — la concaténation des modules dans l'ordre est **identique** au
  `main.js` d'origine (non-régression prouvée + `node --check` par fichier). Scripts classiques (portée
  globale partagée, pas de modules ES) ; le bootstrap ne s'exécute qu'au `DOMContentLoaded`, donc après
  le chargement de tous les scripts. **CSP inchangée** (`script-src 'self'`, chaque module same-origin).
  Aucun changement fonctionnel ni visuel : préparation du terrain pour la suite.

## [1.9.8] — 2026-10-11

### Projets — page d'accueil « classeurs » (légère) + détail par projet
- La page Projets est **éclatée** : un **accueil « classeurs »** qui se peint **sans aucun appel réseau**
  (identité, topologie local/atelier/vitrine, état du **clone local** lu sur le disque) — fini la lenteur
  due au balayage réseau de chaque carte. Un clic ouvre le **détail d'un seul projet**, qui, lui, vérifie
  en ligne (tableau 3 colonnes Local · Atelier · Vitrine). Glyphe de santé **statique** et **honnête** :
  il ne prétend jamais « à jour » (ce qui exigerait le réseau), il montre le dernier état connu localement.
- Nouveau cœur : `ProjectsLight` (accueil, zéro réseau) et `ProjectBoardOne` (détail d'un projet, réseau ciblé).

### Panneau git dans le détail d'un projet
- **Branches** : basculer, **fusionner** dans la branche courante (conflit → message guidé + « annuler la
  fusion »), **supprimer**. Supprimer une branche **jamais fusionnée** déclenche une **alerte visuelle forte**
  (le travail serait perdu) et n'est forcée qu'après confirmation explicite ; une branche fusionnée se
  supprime simplement. Un badge « fusionnée / non fusionnée » est calculé localement (git merge-base).
- **Copies de travail (worktrees)** : lister, **créer** (nouvelle branche ou branche existante, dans un
  autre dossier) et **retirer** ses worktrees — plusieurs branches ouvertes en parallèle depuis un seul
  dépôt (idéal pour des sessions IA simultanées), avec un rappel clair **clone vs worktree**.
- Gestes **locaux** (branche, fusion, worktree) : **sans PIN** (gardés par le mode lecture seule du cœur).
  Exposés aux trois frontaux (GUI, CLI `git branch-delete` / `worktree-*`, MCP `git_worktree_*`).

### Fenêtre PIN — intention de l'agent (Point 4)
- Un agent (CLI `--intent`, ou n'importe quel outil MCP via l'argument `intent`) peut joindre **sa phrase
  d'intention** à une demande. La fenêtre PIN l'affiche **distinctement** (« ce que l'agent dit vouloir
  faire ») **à côté** de ce que **SituLab constate** réellement. Le message de l'agent est **déclaratif** et
  n'autorise rien ; seul le PIN autorise, et il ne transite jamais par le socket.

### Page « Comprendre git »
- Section **Copies de travail** enrichie : visuel **clone vs worktree**, repère « clone ou worktree ? »,
  et renvoi au panneau git du projet où l'on manipule ses worktrees.

## [1.9.7] — 2026-10-11

### Signature « SituLab approved » — IMMUABLE (trailer de commit)
- L'attestation d'origine passe de la **note git** (effaçable) à un **trailer de commit** « SituLab-Approved: »
  **immuable** : il fait partie de l'objet commit et voyage avec lui (plus de note à pousser/récupérer),
  ce qui supprime le mode de panne « commit signé mais aucun badge ». La signature couvre un **pré-image
  canonique** du commit (arbre + parents + auteur + committer + corps sans trailer), pas le SHA final —
  toute altération du commit casse la vérification, transplanter le trailer échoue. Chaîne de certificat
  APCOM et racine épinglée **inchangées**. Vérification trailer-d'abord, repli note v1 (commits anciens /
  mode serveur). `commit.gpgsign=false` ajouté au durcissement git.

### Historique (UX)
- Onglet Historique refondu : **timeline aérée** — niveau 1 = quoi (sujet) / qui / quand, corps **dépliable**
  (corrige l'affichage où le message s'écrasait dans une colonne de 76 px). La légende texte est remplacée
  par un **sceau « certified »** (bouclier + coche) apposé sur les commits vérifiés, avec infobulle d'origine ;
  le trailer d'attestation est masqué à l'affichage.

### Fenêtre PIN (consentement humain)
- Session verrouillée sans PIN de politique : l'invite **unique** de déverrouillage porte désormais le
  **contexte de l'action** (reason + aperçu), au lieu d'un générique « Déverrouiller SituLab ».
- La fenêtre PIN s'**adapte au type d'action** (couleur + icône) : **suppression en rouge** (poubelle,
  bouton de confirmation rouge, avertissement « destructive »), **flux source → destination** pour un envoi
  ou une promotion. Le texte précise que SituLab **constate** l'action et que seul le PIN l'autorise
  (clé `action` émise par le démon, pur habillage).

### Page « Comprendre git » + Aide
- Nouvelle page **Comprendre git** (nav latérale) : le vocabulaire git en clair, groupé (bases, branches,
  annuler/réparer, synchroniser, promouvoir, copies de travail — **clone vs worktree**, signature) ; chaque
  geste avec un petit visuel et un renvoi à l'écran SituLab. **Aide** refondue en nav latérale (Guide / À propos).

### Performances & sécurité
- **Projets** : l'état des projets ne re-balaye plus tous les dépôts locaux pour chaque projet — index
  remote→chemin calculé **une seule fois** (fin de la boucle O(projets × dépôts), cause de lenteur).
- **IPC** : échéances d'inactivité/écriture par connexion + plafond de connexions simultanées + `Dial` borné.
- **Verrou PIN persistant** : le verrou anti-bruteforce (5 essais / 60 s) **survit à un redémarrage** de
  l'app (fichier dédié `pinguard.json`, écriture atomique, chargement fail-safe ; jamais dans `config.json`).
- **mdToHTML** : échappement des guillemets + ouverture externe restreinte aux schémas **http(s)** (`safeOpenURL`).

### Reporté à 1.9.8
- Éclatement de la page Projets (accueil « classeurs » + détail par projet) ; panneau git enrichi des projets
  (suppression de branche avec alerte non-mergée, merge en interface, promotion local↔atelier, clones/worktrees
  manipulables) ; découpage de `frontend/dist/main.js` en modules.

## [1.9.6] — 2026-10-10

### Signature serveur (« agent-proof »)
- **Mode « signature serveur » par connexion** : l'empreinte « SituLab approved » peut être demandée au
  **service de signature APCOM** (`https://situememoring.com/api/sign`) au lieu d'être signée en local. La
  **clé privée ne quitte jamais le serveur** → un agent sur le poste ne peut **rien forger hors ligne**.
  C'est **le démon** qui fait l'appel HTTPS, avec le **jeton Forgejo** qu'il détient déjà (jamais remis au
  client/agent) ; l'autorisation côté service = accès au dépôt visé. Best-effort non bloquant (comme le local).
- Réglage **`SignMode` (local | server) + `SignServerURL`** par connexion, **réglable en fenêtre uniquement**
  (Réglages → Connexions → « Rôle & PIN ») — non exposé socket/CLI/MCP (doctrine PIN).
- **Révocation** : récupération périodique de `/.well-known/situlab-revoked.json` → `RevokedSerials` (best-effort).
- **Vérification INCHANGÉE** (hors ligne, racine embarquée) : le cert de service est accepté tel quel ;
  `attest status` affiche « approved par … Service de signature ».
- Scope : **Forgejo (git.apcom.app)** d'abord ; GitHub plus tard.

## [1.9.5] — 2026-10-10

### Signature = GARANTIE LOGICIEL (refonte du modèle)
- **C'est le logiciel qui signe, plus l'utilisateur.** Une **identité unique « SituLab »** embarquée
  signe tous les commits créés via SituLab — **pas de certificat nominatif, pas de cérémonie par
  utilisateur** (`internal/attest.SoftwareSigner`). Minté une fois par la gouvernance avec la CA :
  **`situlab attest gen-software --signer-key <ca.key> --issuer <ca.cert.json>`**, puis embarqué.
- Repli automatique sur l'ancien cert d'appareil nominatif tant que l'identité logicielle n'est pas mintée.
- **Limite assumée** (garantie logiciel LOCALE) : une clé embarquée est extractible → marqueur
  d'authenticité « fait via SituLab », pas une preuve inviolable face à un attaquant local. La version
  **inviolable = signature côté serveur (T3)** — directives serveur déposées pour situememoring.
- Diagnostic + badge GUI reflètent l'état « Signature logicielle active / à minter ».

## [1.9.4] — non publié

> Lots résiduels : tunnel de connexion à cartes/logos, bibliothèque de prompts, diagnostic vivant.

### Tunnel de connexion (cartes/logos)
- L'ajout d'une connexion est désormais un **tunnel par étapes** : nom → **type de serveur** (cartes
  GitHub / Forgejo-Gitea avec logos) → **rôle** (Atelier / Vitrine) → jeton (avec aide et URL) →
  écran **« Connexion active »**. Le rôle est posé dès la création (`SetInstanceRole`).

### Bibliothèque de prompts (Assistant IA)
- **Prompts réutilisables** (accordéons éditables) dans Réglages → Assistant IA : des messages prêts
  à coller dans votre IA. Prompts **par défaut livrés** avec SituLab (évoluent aux MàJ) + vos
  **personnalisations** (modifier, masquer, ajouter) stockées en config et **jamais écrasées à la
  mise à jour**. Copier / Modifier / Réinitialiser / Masquer-Supprimer.

### Diagnostic vivant
- La section Diagnostic liste désormais des **contrôles lisibles** (git, PIN, session, connexions,
  coffre OS, **signature** racine réelle/test, autres accès git concurrents) avec **statut, explication
  et action**.

## [1.9.3] — non publié

> Lots différés de la 1.9.2, menés en run autonome : badge d'attestation GUI, historique par fichier +
> timeline, parité GitHub (lecture), refonte Réglages (onglets + tunnel connexion + assistant IA), perf.

## [1.9.2] — non publié

### Page Dépôts & détail (LOT H — partiel)
- **Visualiseur de fichier multi-onglets** : onglet **Interprété** (rendu Markdown maison sûr, et HTML en
  iframe sandboxée sans script), onglet **Texte brut**, boutons **Ouvrir dans le navigateur** et
  **Télécharger**. Entrées de contenu enrichies (`html_url`/`download_url`).
- **Protection « ne plus suivre »** : confirmation explicite ; le dossier local n'est jamais supprimé.
- **Lisibilité non-initiés** : nom de dépôt mis en avant, organisation en préfixe discret ; multi-clone
  (« N copies ») explicité.
- *Restant LOT H : onglet historique par fichier (diff), Activité pédagogique, Historique en timeline.*

### Page Projets : colonnes & branches (LOT C)
- **Colonne Vitrine conditionnelle** : plus de colonne « Aucune vitrine » vide quand le projet est
  local+atelier seul — à la place, un bouton **« ＋ Connecter une vitrine »** qui ouvre le tunnel
  directement au choix de la vitrine.
- **Branche inexistante vs vide** : une branche absente d'un git n'affiche plus un trait grisé + bulle
  vide (qui laissait croire qu'elle existait) mais **aucun trait et un picto « absent »** ; une branche
  présente sans version garde son trait plein et sa bulle.
- **Principales vs secondaires** : `main`/`master`/branche de prod en gras ; les secondaires (feat/work)
  atténuées, avec un marqueur **« en cours » (orange)** tant qu'elles ne sont pas réunies vers l'atelier.

### Tunnel de projet : bug d'icône, icônes enrichies, icône maison (LOT D)
- **Correction du bug « changer d'icône efface le nom »** (création ET modification) : la sélection
  d'icône ne re-rend plus tout l'écran (le nom saisi mais non commité était perdu) — bascule de classe,
  focus et caret préservés.
- **Liste d'icônes enrichie** (32 emojis) + **icône maison** : importer une image et la **cadrer en
  carré** (éditeur canvas : glisser pour positionner, curseur pour zoomer) → vignette 96×96 stockée en
  data URL dans le projet, rendue partout (carte, récap).

### Couleur Vitrine — cohérence chromatique
- Nouveau token `--vitrine` (violet) pour l'IDENTITÉ de la vitrine/production, **distinct de l'orange**
  (`--warn`) désormais réservé à « travail en cours / en retard ». Appliqué aux badges de rôle, à la
  colonne Vitrine du graphe projet, au header et au tunnel.

### Options de promotion atelier → vitrine (LOT B)
- **Méthode de promotion au cas par cas** (`project promote --mode`) : **code** (met à jour la branche
  vitrine sans tag), **tag** (défaut : code + tag ; sur GitHub, le tag **déclenche la CI** qui bâtit les
  installeurs et publie la Release), **release** (code + tag + création explicite de l'objet Release via
  l'API, avec notes de version ; les installeurs restent produits par la CI).
- **Clarification « promouvoir ≠ publier une Release »** portée dans le résumé de chaque promotion et
  dans l'aide CLI/MCP — évite la récidive du trou 1.9.0 (version promue mais invisible faute de Release).
- Propagé sur les trois surfaces (GUI binding, CLI `--mode/--notes`, MCP `project_promote`).

### Processus agent IA ↔ démon ↔ GUI (LOT K)
- **Verrou par dépôt** : une action d'agent (push/pull/synchro/enregistrement) et la récupération de
  fond (`FetchAllLocal`) ne lancent plus jamais deux processus git concurrents sur le même `.git` — le
  fetch de fond **saute** un dépôt occupé au premier plan (`tryLockRepo`). Fin des échecs transitoires
  de verrou de refs et des états lus pendant une écriture concurrente.
- **Quittance PIN explicite pour l'agent** : nouvelle issue typée `PINConfirmed / PINRefused /
  PINTimedOut / PINUnavailable`. Le **délai écoulé** (`ErrPINTimeout`) est désormais DISTINCT du refus
  (`ErrPINNeeded`) — la CLI/MCP dit « relance et valide » au lieu de « refusé ». Le PIN reste saisi
  dans la fenêtre uniquement (invariant inchangé) ; délai de garde affiché (120 s).
- **Réactivité CLI** : en mode texte, une mutation signale immédiatement sur stderr qu'une fenêtre PIN
  peut s'ouvrir sur le poste (l'agent ne paraît plus figé). En `--output json`, silence préservé.

### Signature de garantie « SituLab approved » — finalisation (LOT A)
- **Attestation désormais VISIBLE en ligne** : la ref de notes `refs/notes/situlab` est **poussée** à
  chaque envoi (push/synchro) et **récupérée** à chaque fetch/pull — auparavant la signature restait
  purement locale, donc invisible pour les autres postes et la vitrine (cause du « tourne sans servir »).
- **Chaîne de confiance à deux niveaux** : racine APCOM (hors ligne) → **autorité intermédiaire** →
  certificat d'appareil. Vérification locale remontant la chaîne jusqu'à la racine épinglée ; `IsCA`
  couvert par la signature (pas de promotion appareil→autorité) ; profondeur bornée à une intermédiaire.
- **Outil d'émission hors ligne** (gouvernance) : `situlab attest gen-root` / `gen-ca` / `issue` —
  manipulent les clés privées dans des fichiers que l'humain contrôle, **jamais** via le démon/socket.
  Certificats d'appareil à **90 jours** par défaut.
- **Avertissement racine de TEST** : `attest status` (CLI) et binding GUI `AttestUsingTestRoot` signalent
  tant que la racine embarquée est la racine de test (forgeable), à remplacer avant mise en prod.

## [1.9.1] — 2026-10-09

### Ajouté
- **PATH à l'installation (Windows)** : l'installeur ajoute `C:\Program Files\SituLab` au **PATH
  utilisateur** (retiré proprement à la désinstallation) → `situlab` est appelable depuis n'importe
  quel terminal, sans chemin complet. Essentiel pour les agents (CLI/MCP). PATH utilisateur et non
  système, pour éviter toute troncature du PATH par NSIS.

### Modifié
- **Tableau de bord — une seule carte par dépôt distant** : les clones locaux multiples d'un même
  dépôt sont regroupés dans une carte unique, listés en sous-lignes (dossier, **type worktree/clone**
  détecté via `.git`, branche, écart, état) ; chaque sous-ligne ouvre le détail de son clone. Fini
  les N cartes pour un même dépôt. Nouveau champ `RepoView.LocalKind`.
- **Page Projets rafraîchie en temps réel** : branchée sur la boucle « en direct » (signature
  anti-flicker), comme le tableau de bord — les changements distants apparaissent sans devoir quitter
  puis revenir sur la page.

## [1.9.0] — 2026-10-09 — « SituLab approved »

### Ajouté
- **Signature de garantie « SituLab approved »** (attestation d'origine, non bloquante) : chaque commit
  CRÉÉ dans SituLab est signé (clé d'appareil ed25519, chiffrée par le PIN dans le trousseau, certifiée
  par une racine APCOM épinglée) et vérifiable localement — on *voit* qui respecte la discipline « passer
  par SituLab », sans rien bloquer. Nouveau paquet `internal/attest` (racine embarquée, certificat
  d'appareil, attestation anti-rejeu portée par une note git `refs/notes/situlab`). CLI `attest
  status/genkey/install-cert`, outil MCP `attest_status`, bindings GUI. Émission du certificat par la
  gouvernance (racine actuelle = racine de **test** ; la vraie racine la remplacera). Limite T2 assumée :
  voir `SECURITY.md`.
- **Suppression de branche distante** : `repo delete-branch` (CLI), `repo_delete_branch` (MCP), binding
  GUI — refuse la branche par défaut. Pratique pour nettoyer les branches de travail fusionnées.

### Modifié
- **Dates en format suisse `jj.mm.aaaa`** dans toute l'interface et sur le site situememoring.com.

### Corrigé
- **Désinstalleur** : les accents de la question « supprimer vos données ? » étaient illisibles
  (`.nsi` UTF-8 sans BOM lu en ANSI par NSIS) — BOM ajouté et la CI le préserve (`-Encoding utf8BOM`).

## [1.8.2] — 2026-10-09 — « Pipeline de publication »

### Corrigé
- **CI de release (Windows)** : l'empaquetage NSIS échouait car `makensis.exe` était cherché dans
  `Program Files` alors que Chocolatey l'installe (32 bits) dans `Program Files (x86)`. L'appel est
  désormais robuste (shim PATH, puis chemins connus) et vérifie le code de sortie. macOS et Linux
  se construisaient déjà. Première Release publique des trois installeurs.

## [1.8.1] — 2026-10-09 — « Promotion stricte »

### Corrigé
- **Promotion** : le **tag de version est désormais obligatoire** sur la vitrine — une version promue y
  est toujours étiquetée (`PushTagTo`), pour que l'équilibre se lise par version.
- **Flux imposé** : la promotion **refuse** de publier si le clone local a des commits non publiés sur
  l'atelier, et vérifie que la version est **présente sur l'atelier** (`TagPublished`). On ne promeut
  jamais du local vers la vitrine : toujours **local → atelier**, puis **atelier → vitrine**.
- **Contraste** : la pastille de version « atelier en retard » (ambre) devenait illisible (texte ambre
  sur fond ambre) — fond corrigé, version lisible.

## [1.8.0] — 2026-10-09 — « Projets = tableau de bord »

### Ajouté
- **Projets devient le tableau de bord** (page d'accueil) : pour chaque projet, un **graphe par
  branches** répartit l'état sur trois boîtes **Local · Atelier · Vitrine** (production), avec l'icône
  du palier (poste, GitHub/Forgejo), le tag de version dans la bulle et la nuance d'état (à jour / en
  cours / en retard de N versions). Backend `core.ProjectBoards`.
- **Promotion guidée** : l'utilisateur choisit **quelle version** de **quelle branche d'atelier** publier
  vers **quelle branche de vitrine** ; PIN toujours requis. CLI `project promote --from/--version/--to`,
  outil MCP correspondant.
- **Création et édition de projet** : création avec **atelier seul** ou **atelier + vitrine** ; un projet
  se **renomme**, change d'**atelier**, gagne ou perd des **vitrines**, et reçoit une **icône**.
- **Déconnexion d'un serveur** détectée (jeton révoqué en ligne) : les écrans qui dépendent de la
  connexion sont **figés** avec un chemin de reconnexion clair, tandis que **Projets** et **Réglages**
  restent ouverts ; la boîte d'un palier injoignable est **grisée** avec un bouton *Reconnecter*.
- **Aide au jeton selon le système git** dans l'ajout/reconnexion d'une connexion (GitHub : créer un
  PAT avec le bon accès ; Forgejo/Gitea : jeton créé par SituLab depuis le login).
- **Parité CLI/MCP** : `release list` / `release_list`, édition de PR en CLI, édition d'issue en MCP.

### Sécurité & qualité (audit pré-mise en production)
- **Clé maîtresse (DEK)** zéroïsée après chaque confirmation de PIN d'écriture (plus de copie en clair
  abandonnée dans le tas).
- **Mode lecture seule** couvre désormais toutes les mutations de configuration (projets, dépôts suivis,
  clone, fiche IA), sur les trois surfaces.
- **Le PIN n'a plus aucune représentation sur le socket** : champs retirés d'`Args`, méthode de
  définition par IPC supprimée.
- **Promotion** : `FetchURL` ne confond plus « dépôt introuvable / accès refusé » avec « branche
  absente » — la vitrine n'est jamais écrasée en la croyant vierge.
- **Jeton jamais exposé** : l'URL du distant affichée est masquée si elle contient des identifiants ;
  les domaines de chiffrement du trousseau sont désormais versionnés et non ambigus (compatibilité
  préservée avec les trousseaux existants).
- **Analyse d'équipe** réservée aux connexions Forgejo/Gitea (l'API GitHub ne fournit pas les volumes
  de lignes par commit), avec un indicateur de **résultat partiel**.
- Retrait de **code mort** et de **duplications** (noyau, surfaces, client API, frontend), correction
  d'un doublon `splitFull` qui cassait l'ouverture du détail de certains dépôts, et assainissement des
  commentaires et de la documentation.

### Corrigé
- « Ouvrir le détail » (page Dépôts) : délégation sur le conteneur stable + résolution par index vivant
  — fini les non-réponses intermittentes dues aux rafraîchissements « en direct ».

## [1.7.4] — 2026-10-09 — « Auto-nettoyage des clones supprimés »

### Corrigé
- Un clone local dont le **dossier a disparu** (supprimé/purgé) n'est plus affiché comme « Illisible /
  À vérifier » : SituLab **cesse automatiquement de le suivre** (retiré de la config au scan du tableau
  de bord). On ne retire que sur « dossier inexistant », jamais sur un volume temporairement
  indisponible (droits/déconnexion).

## [1.7.3] — 2026-10-09 — « Passe UI/UX »

Rendu plus pro, sans refonte : fondations de mise en page + écrans relus.

### Ajouté
- **Modale « Nouveau projet »** : on **choisit** le dépôt atelier et les dépôts production dans des
  menus listant les dépôts **auxquels on a accès** (par connexion) — fini la saisie owner/repo.

### Modifié (UI/UX)
- **Fenêtre** plus grande par défaut (1280×860, min 1024×640).
- **Colonne de contenu maîtrisée** : plus d'étirement sur fenêtre large ; formulaires (Réglages,
  Projets) en colonne étroite.
- **Cases à cocher** sorties de la règle globale des champs (fini les « carrés » bordés) ; **vrais
  interrupteurs** pour les réglages booléens.
- **Hauteur de contrôle unifiée** (boutons/inputs/selects/segments), **composant segmenté** fusionné
  (un seul état actif, hover), **barre d'outils** du tableau de bord réalignée (wrap, hauteurs égales,
  plus de bouton tronqué).
- Contraste du petit texte relevé ; liens plus cliquables ; vue Projets alignée (titre, bouton
  primaire, état vide).

## [1.7.2] — 2026-10-09 — « Promotion atelier → production »

Publier une version de l'atelier vers la production, proprement, sans jamais donner de jeton à une IA.

### Ajouté
- **Promotion** : publie la version courante de l'atelier vers une production du projet en **un
  commit propre** (l'arbre de l'atelier, pas les micro-commits) via `git commit-tree` — sans toucher
  la branche de travail. Push en fast-forward sur la base prod (pas de force aveugle). Vers la
  production → **PIN** selon la politique de la connexion. L'IA prépare, l'humain confirme.
- Surfaces : `project promote --prod <i>` (CLI), `project_promote` (MCP), bouton + modale PIN (GUI).
- **Marqueur d'équilibre** : la version promue est mémorisée et affichée.

## [1.7.1] — 2026-10-09 — « Projets atelier ↔ production »

### Ajouté
- **Projets** : relient un dépôt **atelier** (dev) à 1..N dépôts **production** (vitrines). La prod
  n'est jamais poussée depuis le local.
- **Vue d'équilibre** : par version (tag) — atelier vs chaque prod : *équilibré* ou *en retard de N
  versions*. CLI `project list|status`, MCP `project_list`/`project_status`, vue GUI dédiée.
- Création des liens en **fenêtre** uniquement (la classification est un choix humain).

## [1.7.0] — 2026-10-09 — « Rôle & consentement »

### Ajouté
- **Rôle par connexion** : atelier (dev) ou production (vitrine), avec **badge** au tableau de bord.
- **Politique PIN par connexion** (par action : envoi / issues / admin dépôt / admin comptes). Défauts :
  atelier autonome (envoi & issues sans PIN, admin sous PIN), production tout sous PIN. Personnalisable
  par cases (préréglages fournis). Réglée en **fenêtre** uniquement.
- **Consentement enrichi** : la modale PIN montre la **branche**, les **commits** à envoyer, les
  **fichiers modifiés** et surtout les **nouveaux fichiers non suivis** — fini la validation à l'aveugle.

### Modifié (sécurité)
- `core.authorize(instance, action, pin)` remplace le portillon PIN binaire sur toutes les mutations
  distantes ; le démon décide de demander le PIN selon la politique (le cœur revérifie, fail-closed).
  Les réglages de rôle/politique/projets ne sont **pas** exposés au socket/CLI/MCP.

## [1.6.1] — 2026-10-09 — « Entretien toolchain »

Montée d'entretien, sans toucher à l'archi, au noyau unique ni au modèle PIN.

- **Toolchain Go** : directive `go.mod` portée à **1.25.0** (minimum de langage ; `go mod tidy`
  l'a lui-même fixée) ; retrait de l'épinglage `GOTOOLCHAIN=go1.23.4`. Build et tests vérifiés sous
  Go 1.27 local.
- **Wails** : `v2.9.2 → v2.16.0` (dernière lignée **v2** ; pas de migration v3, qui est en beta sans
  GA et exigerait un chantier séparé). Dépendances associées remontées (`x/crypto`, `x/sys`, `x/term`…).
- **Correction de doc/CI** : l'affirmation « Go ≥ 1.27 ne compile pas Wails 2.9.2 » était **fausse**
  (vérifiée : build OK sous Go 1.27). CI `GO_VERSION` → `1.25`, Wails CLI release → `v2.16.0`,
  `govulncheck` repassé en `@latest`.
- **Vérifié** : `go build ./...`, `go vet`, `go test ./...` (8 paquets) verts ; binaire prod
  `version`/`doctor` et `dashboard --output json` fonctionnels.

## [1.6.0] — 2026-10-09 — « Git complet pour les agents »

Un seul noyau, plus de gestes git — sans affaiblir le modèle de sécurité. Les gestes **locaux**
(qui ne touchent pas au distant) ne demandent **aucun PIN** ; les gestes **distants réécrits**
passent par le même portillon PIN que `push`/`sync`. Exposés en **CLI et MCP** (l'IA travaille là) ;
la fenêtre reste volontairement au niveau simple.

### Ajouté
- **Réunir des branches** : `merge` (+ `merge-abort`), `rebase` (+ `rebase-continue`/`rebase-abort`).
  Comble la plus grosse limite : on peut enfin intégrer une branche ou réconcilier soi-même.
- **Nettoyer l'historique** : `cherry-pick` (+ continue/abort), `revert` (non destructif),
  `reset` (soft|mixed|hard ; `hard` derrière `--confirm` en CLI), `amend` (réécrit le dernier commit).
- **Commits soignés** : `commit-paths` — **staging sélectif** (n'enregistre que les chemins listés,
  fini le risque d'embarquer un fichier parasite).
- **Pousser après réécriture** : `push-force` (`--force-with-lease`, le « force » sûr) et `push-tag`
  (publication d'un tag) ; `tag-create` crée le tag en local.
- **MCP** : `git_merge`, `git_rebase`, `git_cherry_pick`, `git_revert`, `git_reset`, `git_amend`,
  `git_commit_paths`, `git_tag_create`, `git_push_force`, `git_push_tag` (+ les `…_continue`/`…_abort`).
- **Auto-fetch de fond** : SituLab récupère les nouveautés distantes toutes les ~7 min (best-effort,
  sans PIN), pour que « nouveautés en ligne » se mette à jour sans action manuelle.
- **Détection de git absent** : bandeau clair + lien d'installation si git manque sur le poste.

### Sûreté (audit adversarial v1.6.0)
- **Marques de conflit — garde-fou étendu à tous les enregistrements** : `commit`, `amend` et
  `commit-paths` refusent désormais d'enregistrer si des fichiers en index portent des marques de
  conflit (détection `git diff --cached --check`), fermant la faille où `git add -A` levait l'état
  « non fusionné ». L'invariant « jamais de marques enregistrées/publiées » couvre maintenant
  toutes les surfaces d'écriture, pas seulement `sync`.
- **`push-force` réellement sûr** : ajout de `--force-if-includes` à `--force-with-lease`, sinon
  l'auto-fetch de fond pouvait faire avancer le bail et laisser écraser un commit distant.
- **Staging sélectif durci** : `commit-paths` rejette les motifs de pathspec git (`:/`, `:(glob)`,
  `:(exclude)`) qui auraient contourné le périmètre des chemins annoncés.
- **Auto-fetch** : garde anti-chevauchement (un seul fetch de fond à la fois), moins d'appels git
  redondants par dépôt, et choix d'instance par hôte qui ne « devine » plus en cas d'ambiguïté
  (évite d'utiliser le jeton d'un compte pour un dépôt d'un autre compte du même serveur).

### Corrigé
- **Synchroniser — garde-fou de conflit (bloquant)** : en cas de conflit sur une même ligne,
  « Synchroniser » **s'arrête** après récupération et renvoie un message guidé, au lieu d'enregistrer
  puis publier des fichiers contenant des marques de conflit. Un conflit n'est **jamais** enregistré
  ni envoyé (sentinelles `ErrConflict`/`ErrDirtyBehind`, détection des entrées non fusionnées).

### Documentation & CI
- **SECURITY.md** : retrait de la mention des empreintes SHA-256 (non produites) ; détection des
  identifiants git présentée comme **indicative** (ne voit pas l'agent SSH, les clés au nom non
  standard, les jetons d'autres outils) ; **mode lecture seule** reclarifié (protège des erreurs de
  manipulation, pas d'un agent sous le même compte OS).
- **CI** : `govulncheck` figé à `@v1.1.4` (compatible Go 1.23.4) au lieu de `@latest`.

## [1.5.1] — 2026-10-09 — « Onboarding IA affiné »

- **Contraste** corrigé dans la visite guidée / modale « agent IA » (texte lisible en thème sombre ;
  contrastes vérifiés dans tout le parcours).
- **Message de première connexion** repensé : plus léger et ouvert — on indique à l'IA **où est
  SituLab** (chemin complet de l'exécutable, règle le « pas dans le PATH »), on l'invite à lire la
  fiche et à **donner son avis**, sans lui imposer de commandes ni mentionner de dépôt. Idem MCP.
- **Fiche de collaboration** : `AGENTS.md` déposé près de vos dépôts est désormais présenté comme
  le **mode d'emploi de votre IA + votre fiche de collaboration** — l'IA la lit au démarrage et
  peut l'**enrichir** ; vous l'éditez dans **Réglages → Assistant IA** (vos notes y sont reprises).
- **Message plus clair** quand un dossier n'est pas (encore) un dépôt git : indique le cas d'un
  sous-module à initialiser ou d'un worktree dont le dépôt principal manque.

## [1.5.0] — 2026-10-09 — « Tableau de bord, analyse d'équipe & visuel »

Montée en puissance pour les non-développeurs : plus de visuel, moins de texte.

### Analyse d'équipe (Statistiques)
- **Rapport d'équipe** lancé à la demande : qui a produit quoi, sur quels dépôts, par période
  (semaine/mois en cours ou passé, ou dates libres). KPIs (commits, lignes +/−, personnes, dépôts),
  **graphiques** (par personne, activité par jour, horaires d'activité, dépôts les plus actifs),
  **filtre** par métrique et par personne, **export/impression**. Lancé en **arrière-plan** avec
  barre de progression ; chaque rapport est **enregistré** (« état au… ») et **rejouable**.
  Graphiques via **Chart.js** embarqué (hors-ligne).

### Tableau de bord
- **Vue liste ou grille** au choix (mémorisée), **regroupement logique** (modifications en cours /
  à envoyer / à récupérer / à réunir / à vérifier / à jour / pas sur ce poste), **masquage** en un
  clic de « pas sur ce poste ».
- **Plus de numéros de commit** sur les cartes : tag de version + date, ou date seule.
- **Correctif** : un dépôt ne peut plus apparaître en double (appariement local↔distant durci,
  dédoublonnage des chemins suivis).

### Branches — graphe visuel
- La liste de branches devient un **graphe à voies** : une ligne par branche, deux nœuds
  « En ligne » / « Ce poste » reliés par un segment qui montre l'écart (à envoyer / à récupérer /
  à réunir / à jour), avec tags et dates. Dans le tableau de bord (accordéon) et la fiche dépôt.

### Gestion des utilisateurs & réglages
- « Administration » devient **« Gestion des utilisateurs »** et regroupe **3 onglets** :
  Administration · Annuaire · Statistiques (Annuaire et Statistiques quittent le menu principal).
- **Jetons** : création/suppression sous **2FA + session** (plus de PIN par action ; le mode
  lecture seule reste bloquant).
- **Réglages** entièrement **restructurés** en sections claires (Compte & sécurité, Connexions,
  Apparence & affichage, Diagnostic, Zone sensible).

## [1.4.2] — 2026-10-09 — « Connexion & onboarding simplifiés »

Retours d'usage sur la prise en main. Objectif : moins de jargon, plus d'accompagnement.

### Connexion
- **Nom de la connexion** mis en avant (champ « héros ») en tête d'étape.
- **Fin de la case « Mon compte est administrateur »** : le jeton créé est désormais **standard**
  (moindre privilège). Le droit d'administration est **déduit** de l'API et accordé **à la demande**
  via « Activer l'administration » dans l'onglet Administration (recrée le jeton avec le droit).
- **Afficher le mot de passe** : la case est remplacée par une **icône œil** dans le champ.

### Onboarding
- **Visite guidée** repensée : cartes plus grandes et plus chaleureuses, fond assombri pour les
  faire ressortir, en-tête à icône, pastilles de progression.
- **Nouvelle étape finale « Faisons connaissance avec votre agent IA »** (rejouable depuis l'Aide) :
  l'utilisateur choisit son type d'assistant — **terminal (CLI)** ou **système (MCP)** — et SituLab
  produit sur mesure le **message de première connexion** (règle d'or : l'IA prépare, l'humain
  débloque par son PIN), la **configuration MCP** à coller, et propose d'**installer le guide
  `AGENTS.md`** dans un dossier (lu automatiquement par la plupart des agents).

## [1.4.1] — 2026-10-09 — « Désinstallation avancée & remise à blanc »

Pour un outil qui garde des jetons, une désinstallation standard laissait derrière elle les
données utilisateur (config, trousseau chiffré, cache WebView2, jeton résiduel du coffre Windows) —
comportement normal de Windows, mais indésirable ici. Trois moyens d'effacer proprement :

- **Désinstalleur avancé** : à la désinstallation, propose « Supprimer aussi vos données SituLab ? »
  (Non par défaut, pour préserver en vue d'une réinstallation). Si Oui → retire `%AppData%\SituLab`
  et le dossier de données WebView2.
- **« Effacer mes données de ce poste »** (Réglages → Sécurité) : efface connexions, trousseau,
  réglages et jetons du coffre OS, puis repart sur l'accueil. Confirmation par saisie.
- **`situlab purge --confirm`** (CLI, scriptable) : même effacement, en ligne de commande ; tourne
  hors de la fenêtre (refuse si la fenêtre SituLab est ouverte) et énumère le coffre Windows — ce
  que le désinstalleur NSIS ne sait pas faire.

Note : sous élévation (compte administrateur différent de l'utilisateur), le désinstalleur peut ne
pas voir les données du bon profil ; `situlab purge`, lancé par l'utilisateur, est la méthode sûre.

## [1.4.0] — 2026-10-09 — « Console multi-instances »

Phase 3 de la feuille de route : faire de l'administration multi-instances un atout, en gardant
l'humain au-dessus. Priorité donnée aux garde-fous (que la revue qualifie d'« indispensables »)
et à l'annuaire consolidé, plutôt qu'à l'ensemble des fonctions d'administration.

### Console
- **Annuaire consolidé** : les comptes de **toutes** les instances Forgejo réunis en une seule
  liste, avec la présence par instance (actif / suspendu / inactif / absent) et une recherche par
  nom ou e-mail sur toutes les instances à la fois. Lecture seule et tolérant : une instance sans
  droits admin ou injoignable est signalée sans faire échouer le reste. GUI (onglet *Annuaire*,
  réservé aux administrateurs) + `situlab admin directory`.

### Garde-fous
- **Mode lecture seule** (Réglages → Sécurité) : « consulter sans risque de clic malheureux ».
  Bloque TOUTE mutation **au cœur** (point unique `assertWritable` + `gatePIN`), sur les trois
  surfaces : envoi, synchronisation, suppression, administration, changement de connexion, ET les
  opérations git locales qui modifient l'arbre (enregistrer, récupérer, mettre de côté, créer/
  changer de branche, annuler). Les lectures (consulter, « vérifier le serveur ») restent permises.
  Refus **avant** toute vérification de PIN, avec bandeau permanent et masquage des actions.
  **Fail-closed** : une config illisible est traitée comme lecture seule active. **Réglage de la
  fenêtre uniquement** : un agent (CLI/MCP) ne peut pas le désactiver.
- **Confirmation renforcée** des actions irréversibles (déjà en place) : retaper le nom de
  l'élément en plus du PIN pour supprimer un dépôt ou un compte.

### Corrections
- **Pagination des comptes** (`ListUsers`) : l'administration et l'annuaire ne se limitaient plus
  aux 50 premiers comptes — au-delà, des comptes étaient perdus et la présence « absent » de
  l'annuaire pouvait être fausse.
- **Bindings fenêtre manquants** : « Copier sur mon poste » (clone) et les outils « Git local »
  (créer/changer de branche, mettre de côté, réappliquer, annuler) appelaient des liaisons Wails
  inexistantes — désormais exposées. Recherche de l'annuaire élargie à tous les e-mails/noms vus
  sur chaque instance. Nettoyage des liaisons Wails générées obsolètes.

Version 1.3.0 -> 1.4.0. PIN 6 chiffres conservé. Toolchain Go 1.23.4 / Wails 2.9.2 figé.

## [1.3.0] — 2026-10-08 — « Pour les non-développeurs »

Phase 2 de la feuille de route : rendre l'outil lisible et sûr pour qui ne connaît pas git.

### Expérience
- **Bouton « Synchroniser »** : un seul geste pour le cas courant — récupérer les nouveautés
  (avance rapide, modifications en cours mises de côté puis réappliquées), enregistrer, puis envoyer
  en ligne, sous une **seule confirmation PIN**. L'ordre « récupérer d'abord » évite de transformer
  le cas d'équipe banal (j'édite pendant qu'un collègue publie) en divergence. Disponible aussi côté
  agent : `situlab repo sync <dossier>` et l'outil MCP `repo_sync`. Quand il n'y a que des nouveautés
  à recevoir (rien à envoyer), un bouton « Récupérer » sans PIN est proposé.
- **Situations guidées** : les cas délicats (changements croisés « à réunir », ancienne version
  consultée, dossier non suivi, dépôt sans serveur associé) renvoient une phrase claire de ce qu'il
  faut faire, au lieu d'une sortie git brute.
- **Mode avancé** (Réglages → Affichage) : épuré par défaut ; le mode avancé révèle les étapes
  séparées (envoyer / récupérer / enregistrer) et la matrice des versions de travail. Préférence
  mémorisée par poste.
- **Lexique sans jargon** : les actions visibles parlent français clair (« Enregistrer une
  version », « Versions de travail »…), le terme git restant en infobulle.
- **État plus vrai** : chaque tableau de bord indique « vérifié il y a X min ».

### Performance
- **Boucle temps réel éco-ressources** : suspendue quand la fenêtre est masquée/réduite, ralentie
  après 2 minutes sans interaction, relancée aussitôt au retour — fin des `git status` et requêtes
  serveur en continu pour une fenêtre laissée en arrière-plan.

## [1.2.0] — 2026-10-08 — « Sûr & distribuable »

Phase 1 de la feuille de route d'optimisation (rapport de revue du 2026-10-08) :
sécurité d'abord, puis fiabilité de la distribution. Le PIN à 6 chiffres est conservé (le passage
à une phrase de passe est reporté et documenté dans `SECURITY.md`).

### Sécurité
- **Jeton hors `argv` et scopé à l'hôte** : l'en-tête d'authentification git passe désormais par
  l'environnement (`GIT_CONFIG_*`), plus par la ligne de commande — invisible dans la liste des
  processus — et il est attaché à l'URL de l'instance (`http.<url>.extraHeader`), de sorte qu'une
  éventuelle redirection vers un autre hôte ne reçoit pas le jeton (S2).
- **http:// refusé** pour un hôte public (jeton en clair), autorisé seulement en local/privé ;
  seul le nom `localhost` exact (ou une IP loopback/privée) est reconnu comme local, un simple
  suffixe `.localhost` ne suffit plus (S4).
- **`git diff` durci** (`--no-ext-diff --no-textconv`) + `diff.external=` global : un dépôt piégé
  ne peut plus exécuter de pilote de diff/textconv (S5).
- **Mémorisation du mot de passe explicite et respectée partout** : case à cocher claire à
  l'onboarding (chiffré par le PIN), préférence conservée par instance et honorée aussi lors du
  déverrouillage du gestionnaire de jetons et des reconnexions — plus de mémorisation silencieuse
  si l'utilisateur l'a refusée (S3).
- **Diagnostic des accès git** : Réglages → Sécurité signale les autres identifiants git du poste
  (clé SSH, gestionnaire d'identifiants aux scopes *system* **et** *global* — indispensable pour
  repérer Git Credential Manager sous Windows) qui échappent à la confirmation PIN.
- **CSP durcie** : retrait de `'unsafe-inline'` de `script-src` — un contenu injecté ne peut plus
  exécuter de script.

### Distribution & gouvernance
- `SECURITY.md` (politique de signalement + limites assumées), ce `CHANGELOG.md`.
- CI de qualité (`.github/workflows/ci.yml`) : `gofmt`, `go vet`, tests (`-race`),
  `govulncheck`, `golangci-lint` ; config `.golangci.yml` ; Dependabot (`gomod` + actions).
- **Mise à jour des dépendances / du toolchain reportée** : les `golang.org/x/*` récents exigent
  Go ≥ 1.26, incompatible avec le Go 1.23.4 imposé par Wails 2.9.2. Monter Go **et** Wails ensemble
  est un chantier à part (risque de régression GUI à valider) ; en attendant, `govulncheck` en CI
  signale tout CVE réellement actionnable.

## [1.1.0] — 2026-10-08

- Support **GitHub** (en plus de Forgejo/Gitea), onboarding par PAT.
- 3ᵉ accès **MCP** (`situlab mcp`, stdio) à parité CLI/GUI, mutations sous PIN.
- CRUD complet : dépôts (org, renommer, visibilité, archiver), issues (labels/assignés/jalons),
  pull requests (détail/diff/revue/fusion), releases/tags, git local (branches/annuler/log/stash).
- Onboarding en tunnel + visite guidée ; invites IA ; verrouillage après écriture.
- Licence **Apache-2.0** embarquée. Pipeline CI de build multi-OS (à valider au 1er run).
