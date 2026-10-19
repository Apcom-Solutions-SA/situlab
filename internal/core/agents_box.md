# SituLab Bridge — SafeRoom (agent IA)

Tu es dans la **SafeRoom** : un Linux jetable, isolé du poste. **SituLab tourne sur le poste** ; tu le
pilotes d'ici **par le pont**, sans clé ni jeton. Le PIN surgit sur le poste — il n'est jamais à toi.

## Piloter SituLab

- `situlab <commande>` fonctionne ici : tout est **relayé au poste** par le pont (`situlab-bridge`).
- `situlab-bridge connect` teste la liaison. Échec → l'humain autorise le pont dans le pare-feu
  (poste : Réglages ▸ OS de travail).
- `situlab map --output json` = l'organisation du poste. `situlab <groupe> --help` = les commandes exactes.

## Ouvrir un KERNEL depuis la SafeRoom

- `situlab kernel list` — les KERNEL, l'actif marqué `*`.
- `situlab kernel cat [nom]` — **le contenu** du fichier d'ouverture, relayé par le pont : lis-le, suis-le.
  (N'utilise pas `kernel open` ici : il renvoie un chemin du poste, illisible depuis la SafeRoom.)
- Un seul KERNEL : propose à l'humain de l'ouvrir. Plusieurs : demande lequel.

## Règle

Prépare, puis lance : SituLab décide si le PIN est requis et répond — **fait**, **refusé**, **délai**
(relance). Ne demande jamais le PIN, ne le contourne jamais.
