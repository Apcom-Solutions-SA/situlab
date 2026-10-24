# Journal des modifications — SituLab

Format : [Keep a Changelog](https://keepachangelog.com/fr/). Versions : SemVer.

> Phase de développement initiale. Le détail des versions **antérieures à 1.11.0** a été condensé
> (voir « Historique antérieur ») : il traçait les allers-retours de conception, sans valeur pour le
> lecteur. Les versions récentes restent détaillées.

## [1.18.0] — Béton armé : première version de production (sécurité, robustesse, UX durcies)

> **Note de mise à niveau.** La politique de sécurité est signée plus strictement (l'empreinte couvre
> désormais le type/l'URL/l'utilisateur de chaque cloud). Au PREMIER lancement de la 1.18, la signature
> précédente n'est plus reconnue : SituLab retombe en **position fermée** (PIN exigé, lecture seule) et
> demande de **re-valider la politique dans Réglages** (bouton « revalider »). Geste unique, puis tout
> reprend normalement.

### Sécurité
- **Sceau d'origine = garantie VÉRIFIÉE uniquement.** L'historique n'affiche plus le sceau vert sur la
  simple présence du trailer (forgeable, insensible à la révocation). Trois états honnêtes : **vert**
  (vérifié cryptographiquement en local), **ambre** (déclaré mais non encore vérifié — copie locale
  requise), **rouge** (invalide ou révoqué). La vérification locale fait autorité et rétrograde une
  prétention invalide.
- **Attestation serveur vérifiée avant d'être scellée** : le poste refuse (fail-closed) de poser un
  sceau qu'il ne sait pas vérifier lui-même (chaîne → racine épinglée, fenêtre, révocation, signature).
- **Frontière de l'agent verrouillée** (fail-closed) : accès cloud/sauvegarde bornés aux seuls dépôts
  suivis, jamais au dossier de configuration ; chemin cloud final (et non seulement le dossier) gardé.
- **Intégrité de la politique renforcée** : écriture atomique de la clé d'appareil, régénération
  fail-closed sur clé corrompue, empreinte de politique couvrant les champs anti-redirection de cloud.
- **Verrouillage « après chaque écriture » complet** : les mutations lancées depuis la fenêtre
  reverrouillent la session comme le fait le démon (le jeton ne reste plus déchiffré en mémoire).
- **PIN faux signalé** : la modale de confirmation vérifie le PIN (throttlé) et reste ouverte pour
  ressaisie au lieu de se refermer en silence.
- **Secrets hors ligne de commande** : `token create/delete`, `admin user-create` et `token list` (CLI)
  demandent le mot de passe en saisie masquée et avertissent si un secret est passé en option.
- **Divers** : verrou anti-PIN trivial, icône de projet validée + échappée (anti-XSS), trousseau
  préservé à la reconnexion (sceau et mot de passe mémorisés intacts), `go vet` Windows, Go **1.27.3**
  (correctif GO-2026-6809 crypto/x509), **govulncheck bloquant** en CI.

### Corrigé
- **Gestes git** : tous les gestes locaux et les pushs prennent le verrou par dépôt (plus de course avec
  le fetch de fond). Rebase en conflit : bouton **Poursuivre** en plus d'Annuler.
- **Suppression d'un appairage** : les vitrines d'abord, l'atelier (source) en dernier — plus de
  « source détruite, vitrines orphelines ».
- **Rapports d'équipe** : les dépôts inaccessibles sont comptés et le rapport marqué incomplet.
- **Versions** : `LatestTag` choisit la vraie dernière version sémantique sur GitHub ; le bouton
  « Publier » n'émet plus de tag parasite nommé d'après un commit.
- **Projets/détail** : carte repeinte en entier après fetch/geste (compteurs à jour), « Ouvrir en
  ligne » fonctionne depuis un projet, tableaux Markdown rendus (AGENTS.md).
- **Jetons/identité** : identité et session réalignées sur la connexion courante ; le code 2FA accepté
  est réutilisé pour le rafraîchissement ; `token list` passe par l'auth basique (Forgejo).
- **Sauvegardes** : « 0 archive » survit à l'édition, état « partiel » atteignable, renommage sans
  doublon, bonne méthode choisie par type de plan (plus d'erreur masquée).
- **Corpus/Organiser** : suppression cloud confirmée même sans PIN ; suppression en lot qui ne tait plus
  les échecs.
- **Démarrage** : première peinture non bloquée par le réseau ; organisations réelles proposées à la
  création d'un dépôt ; connexion en échec n'envoie plus en silence vers l'écran 2FA.

### Modifié
- **Tunnel de sauvegarde** : l'étape « couche » (sans effet réel) est retirée — pas de contrôle trompeur.

### Retiré
- Code mort : champ `sign_mode`, liaison `App.ListTokens` et son chemin par jeton (remplacé par l'auth
  basique), fonction orpheline d'état des spokes.

## [1.17.0] — Agent IA en menu principal, Réglages réorganisés, fenêtre PIN avec compte à rebours

### Ajouté
- **Agent IA — entrée du menu principal** : sorti des Réglages, il a sa propre page. Deux temps :
  la **fiche de collaboration AGENTS.md administrée « à la source »** (copie canonique dans le dossier
  de configuration, qui **survit aux mises à jour**) et **brancher son agent** (CLI/MCP). Boucle
  d'état : fiche absente → « Enregistrer à la source » ; présente → chemin exact à donner à l'agent.
- **Réglages — onglet « Aide »** : le guide SituLab est désormais accessible dans les Réglages (il
  quitte le menu latéral), avec relance de la visite guidée.
- **Fenêtre PIN — compte à rebours visible** : on voit le temps restant pour valider ; à l'expiration,
  la fenêtre bascule proprement en « délai écoulé » au lieu de rester figée.
- **Sauvegardes — suivi visuel permanent** : un indicateur flottant (sur toutes les vues) montre la
  sauvegarde en cours (plan + dépôt i/n), puis ✓/⚠/✕ ; à la fermeture, un écran plein « ne forcez pas la
  fermeture » s'affiche pendant les sauvegardes de fin, la fenêtre se ferme ensuite toute seule.
- **Sauvegardes — rétention** : choisir d'**écraser** le précédent (0/1/2/3 archives rangées dans un
  sous-dossier `archive/`, les trop anciennes purgées automatiquement) ou de **tout conserver**
  (cumulatif, avec un avertissement : le cloud s'alourdit). Tracé au Journal par dépôt.
- **Confirmation de suppression partout** : toute suppression ouvre un dialogue soigné et cohérent qui
  énonce clairement les conséquences (ce qui est supprimé, ce qui n'est **pas** touché) avant d'agir —
  sur 100 % des actions de suppression (connexions, plans, projets, KERNEL, spokes, dépôts, branches…).

### Modifié
- **Réglages — « À propos »** : les onglets « Versions » et « Licence » fusionnent en un seul.
- **Réglages — connexions git & cloud** : cartes refondues avec **logo de marque** (GitHub, Forgejo,
  Nextcloud, kDrive) et **pastille d'état reprise du header** (Connectée / Déconnectée / Joignable…),
  même présentation des deux côtés.
- **Agent IA** : renommé (ex-« Assistant IA »), picto robot (celui des accès IA cloud).
- **Sauvegardes sans PIN** : une sauvegarde est une routine additive — elle ne demande plus le PIN et
  se fait à l'ouverture, à la fermeture ou sur action manuelle (les routines automatiques ne sont plus
  silencieusement ignorées quand le cloud exigeait un PIN).
- **« Effacer mes données »** : boucle vérifiée et complète (toutes connexions git **et** cloud, fiche
  AGENTS.md à la source explicitement effacée, classement, trousseau, coffre OS, WebView2, localStorage).
- **Module « Git avancé » cohérent** : le panneau git du détail projet (matrice des branches, rebase,
  worktrees, réglages de dépôt) ne s'affiche que si le module est actif — comme le détail de dépôt.
- **Message de première connexion** réécrit : direct, sans formules superflues ; il pointe vers la
  fiche à la source.
- **Mode lecture seule** : la bascule reflète l'état **autoritaire** du cœur (plus le cache), et en cas
  de doute affiche le côté sûr (coché).
- **Agent IA — tableau de bord** : la page passe en cartes (Fiche AGENTS.md / Brancher mon agent
  côte à côte ; la configuration n'apparaît qu'après le choix du type d'agent) — peu de scroll au repos.
  L'étape « Déposer la fiche » du flux agent, redondante avec la carte Fiche, est retirée.
- **Suppressions — corbeilles discrètes** : dans toutes les listes et cartes (connexions git & cloud,
  jetons, utilisateurs, projets, branches, worktrees, vitrines, KERNEL, spokes, plans de sauvegarde,
  fichiers CORPUS), l'action « supprimer / retirer » devient une petite icône corbeille (intitulé en
  info-bulle) au lieu d'un lien texte — rangées d'actions plus lisibles.

### Performance
- **Démarrage** : première peinture **immédiate** — plus d'enchaînement d'appels réseau (Whoami, santé
  cloud, Doctor) avant le premier rendu ; l'enrichissement se fait en arrière-plan.
- **Affichage des dépôts** : le dernier tag d'une vitrine est lu **sans paginer** tout l'historique des
  versions (nouvel appel ciblé) ; **santé des clouds testée en parallèle** (un cloud lent ne bloque
  plus les autres) ; sonde « en ligne » périodique restreinte aux vues qui en dépendent.
- **Délai de la fenêtre PIN** : une **source unique** (plus de « 120 s » recopié en dur) ; l'indice CLI
  porte le délai max, parsable en `--output json` pour une boucle d'agent nette.

### Corrigé
- **BACKUP — tunnel de création** : choisir un dépôt ciblé ou un dossier cloud n'écrase plus le tunnel ;
  le plan est enfin créé (`SaveBackupPlan` atteint).
- **Instantanéité git** : un geste local (branche créée/basculée/fusionnée/supprimée) se reflète tout de
  suite dans la colonne « Local » ; rapatriement best-effort des refs distantes à l'ouverture d'un projet.
- **Sauvegarde manuelle — fenêtre PIN figée** : après un PIN accepté, la fenêtre de confirmation restait
  ouverte (elle ne se fermait pas). Corrigé **au centre** (toute action confirmée par PIN ferme désormais
  sa fenêtre) ; « Sauvegarder maintenant » affiche aussi un état « en cours » puis le résultat.
- **Santé cloud** : résultat mutualisé quelques secondes (cache court) — plusieurs vues qui la
  demandaient coup sur coup (pastilles + CORPUS + Réglages) ne refont plus le test réseau à chaque fois.
- **Sauvegarde — succès/échecs/pannes remontent de bout en bout** : vérification de chaque envoi (le
  fichier est confirmé présent et non vide sur le cloud ; un 2xx sans persistance devient un échec franc),
  détection d'une panne cloud avant de commencer, création de dossier qui remonte les vraies erreurs,
  Journal enrichi (destination + dépôts réussis + échecs détaillés) et état de la dernière exécution
  (✓ / ⚠ / ✕) sur chaque carte de plan.

### Retiré
- **OS de travail « SafeRoom » (WSL)** : fonction expérimentale (ALPHA) **abandonnée** et **entièrement
  retirée** — code (box WSL, pont `situlab-bridge`, terminal xterm), entrée de menu, onglet Réglages et
  documentation associée (SECURITY.md). L'assistance IA passe par l'**Agent IA** (CLI/MCP) et la fiche
  AGENTS.md.

### Connu / différé
- Le champ de configuration **SignMode** reste non lu en production (mode de signature non
  sélectionnable) — à câbler ultérieurement. Cache des tags/branches par dépôt : différé.

## [1.16.4] — Réglages allégés, SafeRoom au premier plan, flux git plus complet

### Ajouté
- **Dépôts git — filtre « Branche locale »** : trier les clones selon la branche sur ce poste
  (branche par défaut / branche de travail `work/*` / HEAD détaché). Zéro réseau.
- **Panneau git — « rebaser ici »** : rejoue la branche courante au sommet d'une autre (réaligner une
  branche de travail sur l'atelier à jour). Geste local, conflit guidé (`GitRebase` enfin exposé).
- **SafeRoom — bande du pont au sommet** : message pour l'agent, test de liaison et autorisation
  pare-feu sont désormais là où l'on travaille (collage dans le terminal du dessous), plus dans les
  Réglages. Chargée après le terminal (n'en retarde pas l'affichage).

### Modifié
- **SafeRoom — terminal bien plus lisible** : shell de login (prompt « SafeRoom » contrasté, couleurs,
  locale UTF-8, bannière, alias) provisionné dans la box ; palette ANSI haut contraste, police plus
  grande et interligne aéré côté xterm.
- **Réglages — Assistant IA** épuré : suppression de la bibliothèque de prompts et des Notes de
  collaboration ; on conserve la Carte de contextualisation et « Brancher mon agent ».
- **Réglages — OS de travail** allégé (le pont a migré vers la SafeRoom) ; mention situememoring.com
  retirée de l'onglet Versions.
- **Header** : suppression du badge « en ligne » (redondant avec les pastilles Git ✓ / Cloud ✓).

### Performance
- **Fiche projet** : les versions de l'atelier et des vitrines sont récupérées **en parallèle** (au
  lieu d'enchaîner les appels) et le graphe réutilise les tags déjà fournis par le board — ouverture
  d'un projet nettement plus rapide, surtout avec une vitrine GitHub lente.

### Corrigé
- **Branches — cohérence graphe ↔ panneau** : une branche présente seulement sur l'atelier ne
  « disparaît » plus du panneau Local ; elle est signalée avec un renvoi vers l'onglet Atelier.
- **Diagnostic** : retrait de l'avertissement « Autres accès git » (et de la détection associée) ;
  `SECURITY.md` mis en cohérence (la limite réelle reste documentée).

## [1.16.3] — Graphe de promotion : pastilles d'écart au premier plan

### Corrigé
- **Fiche projet — graphe LOCAL↔ATELIER↔VITRINE** : les pastilles d'écart (✓ / flèche + nombre) se
  confondaient avec les cartes de palier. La colonne-gap et ses badges passent explicitement au-dessus
  des cartes (z-index), avec un badge opaque et détaché (ombre) pour qu'il flotte nettement.

## [1.16.2] — CI : empaquetage Windows (makensis)

### Corrigé
- **Build des installateurs — job windows** : localisation robuste de `makensis` après
  `choco install nsis`. Le PATH de l'étape d'installation n'étant pas repris et le binaire n'étant ni
  dans `Program Files(x86)\NSIS` ni dans `Program Files\NSIS`, `$makensis` pointait un chemin
  inexistant et l'empaquetage échouait (« term not recognized »). On cherche désormais le shim
  Chocolatey et le dossier lib du paquet, avec repli par recherche récursive et échec explicite si
  introuvable. Correctif CI uniquement — aucun changement applicatif.

## [1.16.1] — Assistant IA : ouverture KERNEL proactive · SafeRoom paramétrée

### Assistant IA (kit)
- **Ouverture KERNEL proposée d'emblée.** `AGENTS.md` et le message de présentation orientent l'agent,
  au premier contact, vers `situlab kernel list` puis une **proposition d'ouverture** du KERNEL (ou la
  question « lequel ? » s'il y en a plusieurs) — au lieu d'un réflexe purement réactif.
- **`situlab kernel cat [nom]`** : imprime le **contenu** du fichier d'ouverture (lecture seule, sans
  PIN). Permet d'ouvrir un KERNEL depuis un agent qui ne partage pas le disque du poste (SafeRoom), là
  où `kernel open` ne renvoie qu'un chemin illisible pour lui.

### SafeRoom — pont paramétré de bout en bout
- **Doc agent déposée dans la box** à l'installation du pont (`/root/AGENTS.md`, `/etc/situlab/AGENTS.md`)
  + **bannière** (`/etc/motd`) : l'agent sait immédiatement qu'il pilote SituLab par le pont et comment
   ouvrir un KERNEL. Corrige le « aucune doc trouvée / bridge non paramétré ».
- **Raccourci `situlab`** posé dans la box (`/usr/local/bin/situlab` → relaie vers `situlab-bridge`) :
  les commandes `situlab <cmd>` fonctionnent telles quelles dans la SafeRoom.
- **Pare-feu mieux accompagné** (Réglages ▸ OS de travail) : explication, bouton d'autorisation
  automatique (admin) **et** commande PowerShell à copier-coller ; message de présentation pour l'agent
  à copier-coller.
- Doc + raccourci + bannière rejoués à chaque (re)synchronisation du pont.
- **Détection de la passerelle WSL robuste** : lecture de `ip route show default` en argv pur + parsing
  côté Go (fin d'un faux « adresse de l'hôte introuvable » dû à un pipeline quoté corrompu par wsl.exe),
  avec ré-essai ~40 s pour un démarrage à froid.

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
