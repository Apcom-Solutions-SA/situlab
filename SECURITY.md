# Politique de sécurité — SituLab

© 2026 APCOM Solutions SA — licence Apache-2.0.

## Signaler une vulnérabilité

Merci de ne pas ouvrir d'issue publique. Écrivez à **gregory.liand@apcom.ch** avec une description, les
étapes de reproduction, la version (`situlab doctor`) et l'impact estimé. Accusé de réception sous
quelques jours ouvrés, suivi jusqu'au correctif.

## Ce qui est garanti

- **Jetons chiffrés au repos.** Trousseau local, clé dérivée du PIN (Argon2id puis AES-256-GCM). Le PIN
  n'est stocké nulle part ; il *est* la clé. Cela vaut pour les jetons git et les accès cloud (CORPUS).
- **Le PIN ne transite jamais par l'agent.** Il se saisit uniquement dans une fenêtre sur le poste ; un
  agent (CLI ou MCP) ne peut ni autoriser une écriture ni définir le PIN.
- **Le jeton n'est jamais remis au client** : c'est le démon qui appelle l'API, et l'authentification ne
  passe jamais par la ligne de commande.
- **git durci** : hooks, credential.helper, transports ext et file, invites, tout est neutralisé ; une
  URL en `http://` est refusée pour un hôte public.
- **Consentement par connexion** : rôle atelier ou production, politique PIN par action, réglée dans la
  fenêtre uniquement. La promotion de l'atelier vers la production est toujours confirmée par le PIN.
- **Accès IA au CORPUS borné** : l'agent ne voit que les dossiers cloud explicitement autorisés, fermé
  par défaut.
- **Mode lecture seule** : il bloque toute mutation (git et cloud) au cœur, de façon identique pour la
  fenêtre, la CLI et le MCP.

## Limites connues (assumées)

SituLab est une application en espace utilisateur : elle ne protège pas contre un attaquant qui contrôle
déjà votre compte du système d'exploitation.

- **PIN à six chiffres.** Il résiste grâce à Argon2id, mais reste cassable hors ligne si le trousseau est
  volé. Chiffrez donc votre disque (BitLocker, FileVault).
- **Session déverrouillée.** Le jeton déchiffré vit en mémoire ; un programme du même compte du système
  peut le lire. Faites tourner vos agents sous un autre compte ou en bac à sable ; l'auto-verrouillage
  est court par défaut.
- **Autres accès git du poste.** Une clé SSH ou un gestionnaire d'identifiants déjà présent permet un
  accès git indépendant de SituLab. Le seul vrai cloisonnement reste l'isolement par le système.
- **Signature « SituLab approved »** (ci-dessous) : elle n'est pas infalsifiable en local.

La vraie protection hors ligne reste le PIN, qui n'est jamais sur disque, et le coût d'Argon2id.

## Signature de garantie « SituLab approved »

Chaque commit créé dans SituLab porte une attestation d'origine, vérifiée localement contre une racine
APCOM épinglée. Cette garantie ne bloque rien : elle rend visible qui respecte la discipline de passer
par SituLab.

**Limite assumée.** Sous le même compte du système, session déverrouillée, la clé embarquée est lisible :
un faux sceau est forgeable en local. Le mode de signature côté serveur, dont la clé privée reste hors du
poste, élève cette barre. `situlab doctor` indique l'état réel du binaire.

## Distribution

Les binaires ne sont pas signés à ce stade : SmartScreen (Windows) et Gatekeeper (macOS) peuvent avertir.
