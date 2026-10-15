# Journal des modifications — SituLab

Format : [Keep a Changelog](https://keepachangelog.com/fr/). Versions : SemVer.

> Phase de développement initiale. Le détail des versions **antérieures à 1.11.0** a été condensé
> (voir « Historique antérieur ») : il traçait les allers-retours de conception, sans valeur pour le
> lecteur. Les versions récentes restent détaillées.

## [1.16.0] — Durcissement sécurité + SafeRoom (terminal & pont)

### Durcissement sécurité (audit interne)
- **Intégrité de la politique de sécurité.** La politique qui décide *quand* exiger le PIN et *quel
  périmètre* l'IA peut toucher (rôles, politique PIN par connexion, dossiers autorisés, lecture seule)
  est désormais **signée** (clé d'appareil). Une réécriture directe de `config.json` hors de SituLab
  n'emporte pas de signature valide : le cœur retombe alors en **position fermée** (PIN exigé, lecture
  seule) au lieu d'appliquer une politique falsifiée. SituLab re-signe à chaque changement légitime.
- **Canal local authentifié.** Les commandes parlent au démon via un **jeton d'autorisation** présenté
  en tête de connexion (plus aucune connexion anonyme servie sur le socket). Le PIN, lui, ne transite
  toujours jamais par ce canal.
- **Fenêtre de déverrouillage** par défaut réduite de 15 à **5 min** (secrets déchiffrés exposés moins
  longtemps ; réglable dans Réglages).
- **Audit des faux-positifs** : le navigateur interne (WebView2) ne reçoit **aucun secret** cloud/git
  (ils restent chiffrés au trousseau, protégés par le PIN) ; le verrou anti-bruteforce du PIN reste
  autoritaire **en mémoire** du démon (voir `SECURITY.md` pour les limites assumées côté même-compte OS).

> Note de mise à jour : après installation de 1.16, **fermez puis rouvrez SituLab** (le canal local du
> démon change de protocole ; une fenêtre d'une version antérieure doit être relancée).

### SafeRoom — terminal & pont (ex-« OS de travail »)
- **Terminal réel** multi-onglets dans l'OS séparé (vrai pseudo-terminal), pour y piloter une IA en
  ligne de commande ; les sessions survivent à la navigation dans l'app.
- **SituLab Bridge** : canal box ⇄ SituLab repensé en **session complète** (tout le CLI relayé, flux
  temps réel). **Aucune clé dans la box** ; le PIN reste géré sur le poste.
- Détection **en direct** des logiciels installés dans la box ; entrée **SafeRoom** dans le menu.

## [1.15.0] — « OS de travail séparé » (opt-in)

### OS de travail séparé — Réglages ▸ OS de travail (OPT-IN, désactivé par défaut)
- Faire travailler une IA (Claude Code…) dans un **système Linux séparé et jetable** (WSL2 sous
  Windows), **sans jamais l'installer sur le poste ni lui donner les clés**. SituLab reste la **seule
  porte** vers vos dépôts et vos clouds (avec PIN). Rien n'est créé tant que l'option n'est pas activée.
- **Machine à états** (Non activé → Préparation → Prêt → Arrêté → Erreur), **écran d'avertissement**
  franc avant installation (taille ~1–2 Go, durée, connexion stable), **Démarrer / Arrêter / Réinitialiser**.
- **Isolation effective** : la box est créée avec `wsl.conf` **automount=false + interop off** → aucun
  fichier de l'hôte n'est visible depuis la box (**test d'acceptation intégré**). Aucun dossier hôte monté.
- **Image Ubuntu téléchargée à l'activation** (jamais bundlée) → installeur léger ; le module **ne pèse
  rien** tant que l'option est OFF.
- **Terminal de la box** (onglet OS) + **cartes logicielles** (Installer Claude Code, Node).
- **Canal hôte ⇄ box authentifié par jeton éphémère** (fermeture de la faille P2) : relais minuscule et
  **sans clé** dans la box ; démon et clés restent sur l'hôte.

> Limites assumées : backend **Windows/WSL2 uniquement** en 1.15 (macOS/Linux en 1.16+). Le
> provisioning et le canal sont à **valider sur matériel réel**. Le trajet des dépôts box↔hôte (sans
> montage) reste une décision produit à trancher (v1.16+).

## [1.14.0] — « BACKUP clé-en-main »

### BACKUP — tunnel de sauvegarde
- **Plans en quelques clics** (seul le nom se tape) : **générique** (portée dynamique — tous mes KERNEL /
  spokes / KERNEL+spokes / tous mes dépôts : tout nouveau dépôt conforme entre automatiquement dans la
  boucle) ou **ciblé** (liste figée via un sélecteur filtré KERNEL/spoke/autre + recherche).
- **Forme au choix** : **bundle git** (léger, historique complet, re-clonable) ou **miroir** (fichiers
  déposés **lisibles** dans le cloud). **Couche** local / atelier / les deux.
- **Destination par arbre** : on navigue les dossiers du cloud et on crée au besoin (créé à la sauvegarde).
- **Routines** : manuel, **à l'ouverture**, **à la fermeture** (automatiques pour les clouds sans PIN).
- **Journal** des sauvegardes à statut visuel (succès/échec, dépôts traités, forme, déclencheur, date).
- Plans modifiables à tout moment ; les plans hérités (mono-dépôt) restent lisibles et exécutables.

### Mise en route (affinages)
- Le **fichier d'ouverture** est proposé **juste après** la désignation d'un KERNEL (plus de retour
  confus à la mise en route) ; l'avancement se rafraîchit aussitôt.
- Question **vitrine** plus visible, et **implicite** si une vitrine (connexion production) est déjà
  connectée.
- L'**ampoule** laisse place au **picto KERNEL** partout (mise en route, backup, visite).

## [1.13.0] — « Contextualisation, modules, mise en route »

### Assistant IA & AGENTS.md
- **`AGENTS.md` refondu en carte/script** (≈ 406 → 95 lignes) : il **cadre et oriente** (PIN non-absolu,
  réflexe « ouverture KERNEL », notions local/atelier/vitrine, mémoire situative → manifeste.apcom.app)
  et **pointe vers `situlab --help` / `situlab map`** au lieu de recopier la référence des commandes.
- **Page Assistant IA en cards** (carte AGENTS.md / brancher l'agent / notes de collaboration).
- **Bibliothèque de prompts** révisée (sans PIN décrété — SituLab arbitre) + nouvelles entrées :
  faire le point (`map`), ouvrir un KERNEL, déposer un résumé dans le CORPUS, lancer une sauvegarde.

### Modules — adapter l'interface à son usage
- **Nouvel onglet Réglages ▸ Modules** : préréglages **Essentiel / Complet** + groupes activables
  (Accès IA, Git avancé, SPOKES, CORPUS, BACKUP). Persistés au cœur, appliqués aux menus en direct.
  Le « mode avancé » devient le module **Git avancé**.
- **Primo-paramètres à l'installation** : page NSIS (tout coché par défaut, réductible) → un *seed*
  JSON consommé par l'app au premier lancement.

### Mise en route & visite guidée
- **Onboarding canalisant** : page **« Mise en route »** (timeline d'étapes calculées sur l'état réel —
  connecter un atelier, appairer, désigner un KERNEL ; spokes & backup facultatifs). Tant que les étapes
  **obligatoires** ne sont pas faites, les menus de domaine sont **bridés** (fail-open : jamais de blocage
  si l'état est illisible).
- **Visite guidée réancrée** sur le menu latéral (les anciennes cibles de la page « Dépôts » retirée en
  1.11 étaient mortes) ; relançable depuis l'Aide.

## [1.12.0] — « Identité, lisibilité, cohérence »

### Identité visuelle par domaine
- **Une couleur par univers**, ressentie dès le menu principal **et** dans les cartes : KERNEL
  (améthyste), SPOKES (sarcelle), CORPUS (or), BACKUP (jade). Teintes distinctes de la palette d'état
  (bleu/vert/ambre/rouge) et des emplacements git (local cyan · atelier rose · vitrine violet).
- **Pastilles KERNEL / spoke** des cartes : fin de l'ampoule 💡 et du 🛰 — on réutilise les **pictos
  SVG du menu**, teintés à la couleur de leur domaine (cartes Dépôts git, KERNEL, SPOKES, rattachement).

### Dépôts git — lisibilité & organiseur
- **Delta local↔atelier directionnel** sur le triptyque compact des cartes : flèche + nombre colorés
  (↑N à publier / ↓N à récupérer / ⇅ divergé), pour saisir d'un coup d'œil qui est en avance ou en retard.
- **Organiseur « Organiser mes dépôts »** : barre de **recherche** + **facettes** combinables (Tous ·
  À appairer · Sur le poste · Appairés) avec compteurs ; **marque de forge** (GitHub / Forgejo) dans
  les colonnes atelier & vitrine (on voit où vit le dépôt).
- **« Récupérer ici » plug-and-play** : nom de dossier pré-rempli (éditable) + emplacement choisi au
  sélecteur natif, avec aperçu du chemin — plus aucun chemin à taper.

### Assistant IA — prompts
- Les prompts ne **décrètent plus le PIN** : c'est SituLab qui, selon la politique de la connexion,
  indique si une validation est requise et confirme l'exécution. Les prompts de création de dépôt
  **proposent un nom** dérivé du dépôt existant.

### Signature — garantie d'origine par le serveur (zéro geste)
- **Signature serveur par défaut** : tout commit et toute **promotion** créés via SituLab sont signés
  par le **service APCOM** (clé privée jamais sur le poste, jamais remise à l'agent), posés en **trailer
  immuable** → **visibles dans l'historique en ligne**. Aucune clé ni cert à installer côté client.
- Nouvel endpoint serveur `/api/sign-preimage` (attestation v2 sur le pré-image du commit). L'historique
  affiche le **sceau** et « N/M commits signés » directement depuis le message en ligne (sans clone).
- Fiche projet : bouton **« Publier la version → atelier »** (pousse le tag local ; « Synchroniser »
  envoyait les commits mais pas les tags).

### Réglages & Aide
- **Header allégé** : l'identité du compte ne s'affiche plus dans la barre (le compte se gère dans
  Réglages ▸ Connexions git) ; seule la présence en ligne est indiquée.
- **Mode lecture seule** : libellé clarifié — couvre git **et** cloud (le cœur l'appliquait déjà aux
  deux, uniformément GUI/CLI/MCP).
- **« Effacer mes données »** : description exhaustive et exacte (connexions git **et** cloud, trousseau
  chiffré par le PIN, réglages, classement KERNEL/spokes/tags/appairages/fichiers d'ouverture).
- **Diagnostic** : la ligne Connexions montre git **et** cloud, avec des **liens** vers les onglets
  respectifs ; le bloc « autres accès git » n'apparaît que si un accès concurrent est **réellement
  détecté**, et précise alors où il est, à quoi il donne accès, et pourquoi le retirer.
- **Aide** : « À propos » rapatrié dans les Réglages (version dans Versions, licence dans Licence —
  **sans doublon**) ; l'Aide ne garde que le guide et la relance de la visite guidée. « Comprendre git »
  gagne un **retour contextuel** vers Réglages ▸ Connexions git.

### Documentation
- `README.md` resserré ; `SECURITY.md` rendu factuel (contact `gregory.liand@apcom.ch`, sans suite de
  versions) ; `CHANGELOG.md` épuré (versions < 1.11.0 condensées, sans dates) ; docs techniques relues.

## [1.11.1] — 2026-10-14 — « Correctif CI : test multiplateforme »

### Corrigé
- **`TestBaseName` échouait sur Linux** (CI `go test -race`) : `baseName` s'appuyait sur `filepath.Base`,
  qui ne reconnaît pas « \ » comme séparateur hors Windows — un chemin Windows n'y était donc pas
  découpé. Le dernier segment est désormais extrait en coupant sur « \ » **et** « / », quel que soit l'OS.
  Aucun changement fonctionnel de l'application ; correctif de la **porte qualité** de la CI.

## [1.11.0] — 2026-10-14 — « KERNEL : la mémoire, lisible — et nav simplifiée »

### Dépôts git — appairer, classer, ouvrir (le service, pas le gestionnaire)
- **Appairer ses dépôts** : SituLab relie *clone local ↔ atelier ↔ vitrine*. L'organiseur propose les
  appariements (local↔atelier par l'URL d'`origin`, exact ; atelier↔vitrine par le nom, suggéré),
  l'humain confirme, archive ou appaire à la main. **Alerte** « N à appairer » (bouton + pastille header).
- **La page Dépôts remplace l'ancienne vue de gestionnaire** : trois chapitres — **Vos KERNEL**,
  **Mes spokes**, **Dépôts git non classés** — plus des chapitres par **tag projet** (classement libre).
- **KERNEL & spokes désignés depuis les dépôts appairés** (cartes), marqués d'une **pastille + surbrillance**
  dans Dépôts git. Un KERNEL est **unique par dépôt** (pas par dossier) ; un spoke référence son **KERNEL parent**.
- **Fichier d'ouverture (run)** par KERNEL et par spoke, choisi dans un **arbre de fichiers filtrable** :
  le point d'entrée que l'**IA locale** doit lire.

### La carte SituLab pour les IA (CLI & MCP)
- `situlab kernel open` / `situlab spoke open <nom>` — donne le **fichier d'ouverture** (chemin absolu
  prêt à lire) : réflexe « ouverture KERNEL ». MCP : `kernel_open`, `spoke_open`.
- `situlab map` (MCP `situlab_map`) — la **couche de classement** SituLab : rôle (KERNEL/spoke/normal),
  **tag**, et liens **local↔atelier↔vitrine** de chaque dépôt, pour qu'un agent comprenne le poste sans deviner.
- `kernel list` / `spoke list` affichent le fichier d'ouverture ; `AGENTS.md` documente le modèle de liens
  (origin = atelier ; vitrine = production par promotion) et le réflexe d'ouverture.

### KERNEL — la mémoire, sortie du bruit
- **Nouveau menu KERNEL** (premier niveau) : le dépôt qui *est* votre mémoire, présenté pour ce qu'il est.
- **Désigner son/ses KERNEL** (geste de fenêtre, jamais exposé au socket/CLI/MCP), **état en langage
  humain** (à jour / notes à enregistrer / à récupérer), **deux gestes simples** (récupérer sans PIN ;
  enregistrer = récupérer→enregistrer→envoyer en un seul PIN), carte du contenu, sauvegarde CORPUS
  rattachable. **Parité** fenêtre + CLI `kernel …` + MCP `kernel_*` (désignation exclue).

### SPOKES — projections de mission (3ᵉ pilier)
- **Sous-menu KERNEL ▸ Spokes** : projection d'une mission dérivée d'un KERNEL (dépôt local + KERNEL
  parent + mission), état lisible, récupérer/enregistrer sous le même portillon PIN. **Désignation en
  fenêtre.** Parité CLI `spoke …` + MCP `spoke_*`.

### Navigation & santé des connexions
- **Menu principal KERNEL · CORPUS · SPOKES · BACKUP** (icônes monoline cohérentes) ; Dépôts git + Aide
  + Réglages en pied ; Tokens / Gestion des utilisateurs / Comprendre git rejoignent les Réglages.
- **Pastilles de santé** (header) git & cloud (vert/orange/rouge) ; un clic emmène se reconnecter.

### CORPUS — fiabilité & révision UI
- URL WebDAV reconstruite automatiquement (Nextcloud/kDrive), messages de connexion clairs, tunnel
  d'ajout pas à pas avec liens dynamiques, glisser-déposer, **accès IA par arbre** (autorisations +
  exclusions, le plus spécifique gagne) avec picto robot coloré cohérent.

## Historique antérieur (résumé)

Versions **1.0 → 1.10** — fondations posées pendant le développement initial, résumées par thème (le
détail horodaté a été retiré, sans valeur pour le lecteur) :

- **CORPUS (1.10)** — clouds de fichiers souverains (Nextcloud, kDrive) via WebDAV maison, contrôle
  d'accès IA par dossier fail-closed, politique PIN par cloud, premières sauvegardes git → cloud (bundle).
- **Projets, promotion & tableau de bord (1.7–1.9)** — projets atelier ↔ vitrine, promotion guidée
  atelier → vitrine (code/tag/release), graphe de branches à voies Local · Atelier · Vitrine, accueil
  « classeurs » sans réseau + détail par projet, panneau git (branches, fusion, worktrees).
- **Signature « SituLab approved » (1.9.x)** — attestation d'origine des commits : d'abord note git,
  puis trailer immuable ; garantie logicielle (identité unique embarquée) ; option signature serveur.
- **Git complet pour les agents (1.6)** — merge/rebase/cherry-pick/revert/reset/amend, staging sélectif,
  push-force sûr, auto-fetch de fond ; gestes locaux sans PIN, distants réécrits sous portillon PIN.
- **Rôle, consentement & PIN (1.7)** — rôle atelier/vitrine par connexion, politique PIN par action,
  fenêtre de consentement enrichie ; réglages jamais exposés au socket/CLI/MCP.
- **Sécurité du trousseau (1.0–1.2)** — tokens chiffrés par le PIN (Argon2id → KEK → DEK, AES-256-GCM),
  jeton hors `argv` et scopé à l'hôte, http:// public refusé, CSP durcie ; le PIN n'est stocké nulle
  part et ne transite jamais par l'agent.
- **Console multi-instances (1.4)** — annuaire Forgejo consolidé, mode lecture seule au cœur (fail-closed,
  fenêtre uniquement), remise à blanc (`purge`, désinstalleur avancé).
- **Prise en main (1.3–1.5)** — bouton « Synchroniser » (un seul PIN), situations guidées, mode avancé,
  onboarding + visite guidée, tableau de bord liste/grille, analyse d'équipe, `AGENTS.md`.
- **Accès & fondations (1.1, 1.6.1)** — GitHub en plus de Forgejo/Gitea, 3ᵉ accès MCP à parité, CRUD
  complet ; Go `go.mod` 1.25, Wails v2.16.0, CI de release multi-OS (Windows NSIS, macOS dmg, Linux tar.gz).
