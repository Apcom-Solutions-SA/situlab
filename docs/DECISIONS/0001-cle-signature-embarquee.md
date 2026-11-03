# ADR 0001 — Clé de signature de garantie embarquée

- **Statut** : OUVERT (décision de gouvernance à trancher)
- **Date** : 2026-11-01
- **Portée** : signature « SituLab approved » (attestation d'origine des commits)

## Contexte

Chaque commit créé via SituLab porte une attestation d'origine, vérifiée **localement** contre une
racine APCOM épinglée. Deux modes coexistent :

- **Serveur (T3)** : la clé privée reste **hors du poste** ; c'est la barre la plus haute.
- **Repli local** : une clé de signature (`internal/attest/situlab_sign.key`) est **embarquée dans le
  binaire** (`go:embed`) pour fonctionner hors ligne.

Comme le binaire est distribué et le dépôt public, **cette clé est extractible**. Le repli local est
donc un **marqueur d'origine**, pas une garantie cryptographique : sous le même compte système, un faux
sceau est forgeable. Cette limite est déjà documentée (`SECURITY.md`).

## Problème

Une **clé privée versionnée dans un dépôt public** se lit, au premier coup d'œil, comme une erreur de
débutant — même quand elle est intentionnelle. C'est un risque de **réputation** à l'ouverture
communautaire, distinct de la question de sécurité (qui, elle, est déjà assumée et documentée).

## Options

1. **Statu quo + documentation forte** (actuel). La clé reste embarquée ; `SECURITY.md` et cet ADR
   expliquent que le repli local est un marqueur, non une garantie. Coût nul, perception à gérer.
2. **Injection au build (CI)**. Sortir la clé de l'historique public ; l'injecter au moment du build
   depuis un secret CI. Le binaire reste identique. Réduit le risque réputation ; complexifie le build
   local (clé à fournir) et ne change rien à l'extractibilité du binaire.
3. **Renommer le concept**. Ne plus parler de « signature » pour le repli local mais de « marqueur
   d'origine », pour couper court au malentendu. Peu coûteux, honnête.
4. **Supprimer le repli local**. Ne garder que la signature **serveur (T3)** ; hors ligne = « non
   vérifié », assumé. Plus simple et plus honnête, au prix de la fonctionnalité hors ligne.
5. **Clés par utilisateur**. Chaîne de confiance réelle par signataire. La plus robuste, la plus lourde.

## Recommandation de l'atelier

Combiner **(3) renommage** + **(2) injection CI** à court terme : le binaire ne change pas, le dépôt
public ne porte plus de clé privée, et le vocabulaire ne promet plus une garantie qu'il ne tient pas.
**(4)/(5)** sont des évolutions de fond à décider séparément.

## Décision

**À trancher par la gouvernance.** Tant que ce n'est pas tranché, le comportement reste l'option 1, avec
la limite documentée dans `SECURITY.md`.
