# Journal des modifications — SituLab

Format : [Keep a Changelog](https://keepachangelog.com/fr/). Versions : SemVer.

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
