# Politique de sécurité — SituLab

© 2026 APCOM Solutions SA — licence Apache-2.0.

## Signaler une vulnérabilité

Merci de **ne pas** ouvrir d'issue publique pour une faille de sécurité.
Écrivez à **security@apcom.ch** (ou au contact indiqué sur apcom.ch) avec :
une description, les étapes de reproduction, la version, et l'impact estimé.
Nous accusons réception sous quelques jours ouvrés et tenons informé jusqu'au correctif.

## Modèle de menace (ce qui est garanti)

- **Jetons chiffrés au repos** : trousseau local `keyring.json`, clé dérivée du PIN
  (Argon2id → KEK → DEK, AES-256-GCM). Le PIN n'est stocké nulle part.
- **Le PIN ne transite jamais par le socket ni par MCP** : il est saisi uniquement dans une
  fenêtre sur le poste. Un agent (CLI/MCP) ne peut pas autoriser une écriture ni définir le PIN.
- **Le jeton d'instance n'est jamais remis au client** ; c'est le démon qui appelle l'API.
  Depuis la 1.2, l'en-tête d'authentification git passe par l'**environnement** (jamais `argv`).
- **git durci** : hooks/fsmonitor/credential.helper neutralisés, transports ext/file interdits,
  `GIT_TERMINAL_PROMPT=0`, diff externe/textconv désactivés.
- **http:// refusé** pour un hôte public (jeton en clair) ; autorisé seulement en local/privé.
- **Rôle & politique PIN par connexion (v1.7)** : chaque connexion est *atelier* ou *production*,
  avec une politique PIN par classe d'action. Défauts : atelier autonome (envoi/issues sans PIN),
  production tout sous PIN. L'exigence de PIN d'un **envoi** suit le **dépôt réellement poussé**
  (son serveur), pas la connexion nommée par l'appelant → un agent ne peut pas pousser en production
  via une connexion atelier (fail-closed si le serveur du dépôt est inconnu ou ambigu). La création/
  suppression de **jetons** passe aussi par la politique PIN. Rôle et politique se règlent **dans la
  fenêtre uniquement** (jamais via socket/CLI/MCP). La **promotion** atelier→production est toujours
  confirmée par le PIN.

## Limites connues (assumées, à documenter côté utilisateur)

- **Robustesse du PIN (6 chiffres).** Le trousseau au repos résiste à Argon2id, mais un PIN à
  6 chiffres (1 million de combinaisons) reste cassable **hors ligne** si le fichier `keyring.json`
  est volé (de quelques heures à quelques jours selon la machine). Le verrou 5 essais / 60 s ne
  protège que l'interface, pas le fichier. → **Atténuations** : chiffrer le disque (BitLocker,
  FileVault), ne pas laisser traîner `keyring.json`. Une phrase de passe est à l'étude pour une
  version ultérieure.
- **Processus de même utilisateur.** Tant que la session est déverrouillée, le jeton déchiffré
  vit en mémoire du démon. Un programme du **même compte OS** peut lire cette mémoire. → **Parade** :
  faire tourner les agents sous un autre compte / en bac à sable ; activer « verrouiller après
  écriture » et un auto-verrouillage court.
- **Autres identifiants git du poste.** Si le poste a une clé SSH ou un gestionnaire
  d'identifiants git, un accès git **indépendant** de SituLab reste possible. L'écran
  **Réglages → Sécurité** tente de le détecter, mais cette détection est **indicative** :
  elle ne voit pas un agent SSH déjà chargé, une clé au nom non standard, ni les jetons
  rangés par d'autres outils. Son absence d'alerte ne garantit donc pas qu'aucun accès
  parallèle n'existe.
- **Gestes git locaux sur un chemin arbitraire.** Les commandes git locales (commit, reset,
  discard, merge…) opèrent sur le chemin fourni, même s'il n'est pas un dépôt « suivi » par SituLab.
  La frontière protectrice reste le **mode lecture seule** (bloque toute mutation) et la politique
  PIN pour le distant, pas une liste blanche de chemins.
- **Mode lecture seule.** Il protège contre les **erreurs de manipulation** (une écriture
  involontaire pendant qu'on explore). Ce n'est pas une barrière de sécurité : un programme
  tournant sous le **même compte OS** peut modifier `config.json` et le désactiver. La vraie
  parade reste l'isolation de compte / bac à sable (voir ci-dessus).

## Signature de garantie « SituLab approved » — limite assumée (T2)

Chaque commit créé dans SituLab porte une attestation d'origine (clé d'appareil ed25519 chiffrée par
le PIN, certifiée par une racine APCOM épinglée), vérifiée localement. C'est une garantie **non
bloquante** : elle rend visible qui respecte la discipline, elle ne rejette rien.

**Ce n'est pas infalsifiable en local.** Un attaquant **déterminé sous le même compte OS** peut lire
la clé d'appareil dans la mémoire du démon déverrouillé et forger un faux « approved ». C'est le même
plafond que partout ailleurs dans SituLab ; on le **borne** par des certificats à **courte durée** et
une **révocation** (serials). Seul un modèle **T3** (signature produite côté serveur, clé jamais sur le
poste) fermerait ce vecteur — hors périmètre de cette phase. On ne peut pas *forger* un faux ✅ sans le
certificat ; on peut seulement *se priver* du ✅ (commettre hors SituLab) — ce qui est le signal voulu.

## Distribution

Les binaires **ne sont pas signés** à ce stade (SmartScreen sous Windows, Gatekeeper sous macOS
peuvent avertir).
