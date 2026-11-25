# Contrat de labels — proposition v1, non appliquée

## Problème observé
Le corpus mélange deux questions : « le texte lui-même décrit-il un signal CTI ? » et « la source structurée rapporte-t-elle un événement/avis CTI ? ». La première est observable dans le contenu ; la seconde peut être vraie même si le titre/texte est pauvre. Les audits montrent des divergences : 492/550 anciens négatifs GitHub ont au moins un marqueur littéral vulnérabilité/impact/attaque, alors que 240/355 anciens positifs ransomware.live n'ont aucun des marqueurs testés. Les regex ne sont que des indicateurs : présence ≠ vérité, absence ≠ négatif.

## Deux cibles à ne plus fusionner
1. `text_cti_evidence`: label sur le texte visible uniquement. Positif si le texte décrit explicitement une vulnérabilité/mécanisme, intrusion, compromission, données dérobées/publiées, offre d'accès compromis, campagne malveillante ou défense liée à une menace. Si le contenu omet l'évidence ou est trop court/ambigu, `abstain`, pas négatif par défaut.
2. `structured_claim_relevance`: label faible séparé tiré d'un enregistrement source typé (p. ex. avis GHSA authentique, enregistrement du endpoint victim). Cela signifie « source structurée rapporte ce type de claim », et non « le claim est vrai » ni « le texte visible contient sa preuve ». Conserver `label_authority`, endpoint/type et date.

## Usage
- Ne jamais convertir les deux axes en une même classe d'entraînement sans décision explicite sur l'objectif produit.
- Pour un classifieur texte-only, n'entraîner que `text_cti_evidence`; source/date/acteur restent métadonnées de split et d'audit.
- Si le produit veut reconnaître des enregistrements sources, traiter `structured_claim_relevance` dans une voie distincte, avec métriques distinctes et absence d'affirmation de vérité.
- Les deux axes peuvent être positifs, contradictoires ou indéterminés ; conserver ces états séparément.
- Les regex/LLM peuvent proposer des candidats, jamais remplacer la provenance. Pas de revue humaine demandée ; tout cas non déterministe reste abstention/weak label.

## Porte avant entraînement
Le propriétaire du modèle doit choisir et figer une cible de production. Ensuite versionner un nouveau corpus avec labels par axe, conserver les reçus, détecter doublons/conflits avant split, fixer à l'avance des jeux développement et test, et n'utiliser aucun score/test pour corriger rétroactivement les labels. Sans choix de cible et sans label de test traçable, aucun résultat de précision/rappel ne vaut validation.

Cette proposition n'altère aucun label/corpus/modèle, ne consulte pas la réserve et n'autorise aucune promotion.
