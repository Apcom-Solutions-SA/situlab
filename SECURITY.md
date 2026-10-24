# Politique de sécurité — SituLab

© 2026 APCOM Solutions SA — licence Apache-2.0.

## Signaler une vulnérabilité

Merci de **ne pas** ouvrir d'issue publique pour une faille de sécurité.
Écrivez à **gregory.liand@apcom.ch** avec : une description, les étapes de reproduction, la version
(`situlab doctor`) et l'impact estimé. Accusé de réception sous quelques jours ouvrés, suivi jusqu'au
correctif.

## Ce qui est garanti

- **Jetons chiffrés au repos** : trousseau local `keyring.json`, clé dérivée du PIN
  (Argon2id → KEK → DEK, AES-256-GCM). Le PIN n'est stocké nulle part. Vaut pour les jetons git **et**
  les mots de passe d'application cloud (CORPUS).
- **Le PIN ne transite jamais par le socket ni par MCP.** Il est saisi uniquement dans une fenêtre sur
  le poste ; un agent (CLI/MCP) ne peut ni autoriser une écriture ni définir le PIN.
- **Le jeton n'est jamais remis au client** : c'est le démon qui appelle l'API. L'en-tête
  d'authentification git passe par l'environnement, jamais par `argv`.
- **git durci** : hooks/fsmonitor/credential.helper neutralisés, transports ext/file interdits,
  `GIT_TERMINAL_PROMPT=0`, diff externe/textconv désactivés.
- **http:// refusé** pour un hôte public (jeton en clair) ; autorisé seulement en local/privé.
- **Rôle & politique PIN par connexion** : *atelier* (envoi/issues autonomes, admin sous PIN) ou
  *production* (tout sous PIN). L'exigence de PIN d'un envoi suit le **serveur du dépôt réellement
  poussé**, pas la connexion nommée par l'appelant (fail-closed si ambigu). Rôle et politique se règlent
  **en fenêtre uniquement**. La promotion atelier → production est toujours confirmée par le PIN.
- **Accès IA au CORPUS borné** : l'agent ne voit que les dossiers cloud explicitement autorisés
  (allowlist + exclusions, fail-closed), appliqué à la frontière agent.
- **Mode lecture seule** : bloque toute mutation (git et cloud) au cœur, uniformément GUI/CLI/MCP.

## Limites connues (assumées)

- **PIN à 6 chiffres.** Le trousseau résiste à Argon2id, mais 6 chiffres restent cassables **hors ligne**
  si `keyring.json` est volé. Le verrou 5 essais / 60 s ne protège que l'interface. → Chiffrer le disque
  (BitLocker/FileVault) ; ne pas laisser traîner `keyring.json`.
- **Processus de même utilisateur.** Session déverrouillée, le jeton déchiffré vit en mémoire du démon ;
  un programme du **même compte OS** peut le lire. → Faire tourner les agents sous un autre compte / bac à
  sable ; auto-verrouillage court (défaut 5 min depuis 1.16).
  *Durcissement 1.16 :* la **politique** (quand exiger le PIN, périmètre, lecture seule) est désormais
  **signée** (clé d'appareil) et le canal du démon exige un **jeton** — une réécriture directe de
  `config.json` ou une connexion anonyme au socket est détectée et retombe en **position fermée**. Résidu
  assumé : la clé d'appareil (`device.key`) étant lisible par le même compte OS, un attaquant du **même
  compte disposant d'exécution de code** peut encore forger une signature (comme il peut déjà dumper la
  mémoire du démon déverrouillé) — c'est une **défense en profondeur**, pas une frontière contre un
  attaquant même-compte déterminé. Le vrai socle reste le **PIN** (jamais sur disque) et le coût Argon2id.
  *Sauvegarde :* la clé d'appareil (`device.key`) fait partie de l'**état local du poste**. Sauvegardez le
  **dossier de configuration entier** (pas seulement `config.json`). En cas de clé renouvelée (restauration
  partielle, migration de machine), la politique apparaît « non validée » ; on la **re-valide** depuis
  Réglages (après avoir vérifié rôles/périmètres/PIN) — jamais automatiquement.
- **Verrou anti-bruteforce du PIN (`pinguard.json`).** Le compteur fait autorité **en mémoire** du démon
  (seul écrivain) : une réécriture du fichier pendant qu'il tourne n'a aucun effet. La persistance sur
  disque n'est qu'un **confort** pour survivre à un redémarrage ; un process du même compte OS peut la
  réinitialiser (supprimer le fichier) — limite assumée. La vraie protection hors ligne reste le **coût
  Argon2id** (m=64 Mo, t=3, p=4), pas ce verrou d'interface.
- **Autres identifiants git du poste.** Une clé SSH ou un gestionnaire d'identifiants déjà présent sur la
  machine permet un accès git **indépendant** de SituLab : la confirmation par PIN ne cloisonne un dépôt
  que si le poste n'a pas d'autre moyen de s'authentifier. SituLab ne cherche pas à inventorier ces accès
  (une énumération serait de toute façon incomplète : agent SSH chargé, clé au nom non standard, jetons
  d'autres outils). La seule garantie de cloisonnement reste l'**isolement OS** : faire tourner les agents
  sous un autre compte / en bac à sable.
- **Signature de garantie (ci-dessous).** Non infalsifiable en local.

## Signature de garantie « SituLab approved »

Chaque commit créé dans SituLab porte une attestation d'origine, vérifiée localement contre une racine
APCOM épinglée. Garantie **non bloquante** : elle rend visible qui respecte la discipline « passer par
SituLab », elle ne rejette rien. Elle est portée par un **trailer de commit immuable** (fait partie de
l'objet, voyage avec lui).

**Limite assumée (local).** Un attaquant sous le même compte OS, session déverrouillée, peut lire la clé
et forger un faux ✅. On ne peut pas forger sans la clé ; on peut seulement se priver du ✅ (commettre hors
SituLab) — c'est le signal voulu. Le mode **signature serveur** (clé privée hors du poste, + audit,
rate-limit, révocation) élève cette barre. `situlab doctor` / `attest status` indiquent l'état réel du
binaire (signature active, racine de test, ou inactive).

## Distribution

Les binaires **ne sont pas signés** à ce stade (SmartScreen sous Windows, Gatekeeper sous macOS peuvent
avertir).
