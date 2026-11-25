# Réentraînement autonome du classifieur — 19 octobre 2026

Suivi : [issue #34](https://github.com/S-rvo/BlueWhiteThreat/issues/34).
La [campagne v2](nlp-automatic-v2-2026-10-19.md) décrit la collecte initiale et le
contrat des annotations. Cette expérience distincte développe un candidat ; elle
ne remplace pas les étiquettes ou le test de cette campagne.

## Pourquoi le seuil seul ne suffit pas

Les deux modèles locaux s'accordent sur 184 des 200 textes de validation :
19 positifs, 165 négatifs, neuf désaccords et sept abstentions. Couverture : 92 %.
Le maximum de précision sous rappel >= 90 % et précision >= 50 % est inaccessible.
Le meilleur F1 de l'ancien modèle sur ces accords est 0,30 au seuil 0,129360 :
9 TP, 32 FP, 10 FN, 133 TN. Ce résultat de sélection de seuil ne mesure pas la
performance générale et ne justifie pas un déploiement de ce seuil.

Un premier candidat, entraîné sur le corpus historique et cent nouveaux avis de
vulnérabilités, atteint 0,50 de F1 sur cette validation ; les objectifs restent
inaccessibles. Cette tentative est archivée séparément, sans remplacement de poids
en production.

## Données supplémentaires et provenance

Le collecteur `cmd/collectadvisories` utilise l'[API publique des avis globaux
GitHub](https://docs.github.com/en/rest/security-advisories/global-advisories#list-global-security-advisories),
qui permet la lecture des avis publics sans authentification. Huit requêtes
bornées et espacées ont produit 800 descriptions d'avis publiés. Un autre lot de
1 000 questions générales publiques provient des pages suivantes de l'API Stack
Exchange. Les accès refusés ou limites d'API arrêtent la collecte ; aucun compte
ni contournement. Le filtre partagé de sûreté précède la persistance locale.

Après exclusions contre l'entraînement historique, les diagnostics, les textes de
développement et les **400 documents initiaux**, il reste 258 avis et 434 questions
novels selon le contrôle lexical. Les groupes CVE évitent de séparer les avis
portant le même identifiant ; cela ne garantit pas l'indépendance de récits sans
identifiant commun.

Le tirage fixé avant tout score réserve :

- 100 avis identifiés par CVE pour l'apprentissage, avec une étiquette positive
  **faible**, dérivée de leurs métadonnées publiques ; ce ne sont pas des labels
  de test validés par l'IA ;
- 100 autres avis et 100 questions générales pour un nouveau test indépendant.

Les deux annotateurs jugent réellement le nouveau test. Une source d'avis ne
produit pas automatiquement une étiquette positive dans ce test. Sa composition
équilibrée est un test ciblé ; sa précision brute n'est pas la précision attendue
sur le flux de production.

## Développement par validation croisée

Les 184 accords initiaux deviennent des données de développement/apprentissage
pour cette seconde tentative. Ils **ne sont plus une validation indépendante**
du nouveau candidat. Les deux tests réservés restent hors apprentissage.

`cmd/traincandidate --cross-fit` répartit les textes de développement sur cinq
plis, stratifiés par classe. Pour chaque pli, vocabulaire, IDF et poids sont appris
sans les textes retenus dans ce pli. Les références historiques et les cent avis
faiblement étiquetés servent uniquement à l'apprentissage. Les scores hors pli
choisissent le seuil et une régularisation parmi quatre valeurs fixées à l'avance :
0,001, 0,01, 0,1 et 0,5 ; 4 000 époques, taux d'apprentissage 2, poids de classes
équilibrés. Le modèle final est ensuite réentraîné avec tous les textes de
développement. Les scores peuvent varier avec ce réentraînement ; le test
indépendant est indispensable.

Le meilleur candidat de développement choisit L2 = 0,001 et seuil = 0,497129 :
précision 66,67 %, rappel 63,16 %, F1 64,86 %, soit 12 TP, 6 FP, 7 FN et 159 TN.
Il ne satisfait pas l'objectif de rappel. La sélection d'hyperparamètres sur ces
scores ajoute un optimisme possible : ils ne sont pas présentés comme une mesure
de test. Le modèle final contient 1 356 textes d'apprentissage après regroupements.

Ses poids, son seuil et son empreinte sont figés **avant tout résultat des tests**.
Le plan de comparaison prévoit le modèle actuellement déployé, son seuil ajusté
sur validation et le nouveau candidat. Aucun modèle n'est choisi après examen du
test et aucun test n'est retiré pour améliorer le résultat.

## Contrôle diagnostique et essais réservés

Sur les 24 diagnostics synthétiques connus, le candidat produit 8 TP, 1 FP, 4 FN
et 11 TN, contre 0 TP et 12 FN pour le modèle précédent corrigé. Les 720 inférences
répétées donnent exactement les mêmes décisions et scores. C'est un diagnostic
sur des textes déjà connus de l'expérimentation, pas une estimation indépendante.
L'artefact agrégé est `nlp-candidate-synthetic-repeats-2026-10-19.json`.

`evaluate_candidate.py freeze` a figé les trois artefacts de modèle avant les
résultats. `run_candidate_comparison.py` attend la fin de la campagne initiale,
annote le nouveau test, puis produit une seule comparaison préétablie sur les deux
lots. Il conserve couvertures, effectifs, matrices de confusion, intervalles
Wilson et métriques par strate. Les citations, textes, scores individuels et
poids restent locaux dans `data/automatic-v2-2026-10-19/retraining/`.

La qualification est exploratoire : les références restent des accords IA.
Les objectifs de précision/rappel, couverture et effectifs sont vérifiés ; le
nouveau test exige aussi une spécificité d'au moins 95 %. Un objectif non atteint
reste affiché comme tel. Aucun modèle insuffisamment qualifié n'est promu et
aucune intervention humaine n'est nécessaire pour exécuter ces contrôles.

## Résultat final de la comparaison figée

La comparaison s'est terminée sur les deux tests réservés, sans nouveau réglage.
L'artefact agrégé est `retraining/comparison-result.json` et le plan exécuté porte
l'empreinte SHA-256
`2afec081c024db4e5aac6f5e28f34149d48ac39ba439eba9192f6b87591d4a58`.

Sur le test original, les annotateurs produisent 169 consensus sur 200 documents
(couverture 84,5 %), avec 19 désaccords et 12 abstentions. Les résultats globaux
sur ces 169 accords sont :

| Modèle figé | TP | FP | FN | TN | Précision | Rappel | F1 | Transmission |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Déployé, seuil 0,581961 | 0 | 0 | 15 | 154 | indéfinie | 0,0 % | 0,0 % | 0,0 % |
| Seuil seul, 0,129360 | 3 | 30 | 12 | 124 | 9,1 % | 20,0 % | 12,5 % | 19,5 % |
| Réentraîné, seuil 0,497129 | 9 | 3 | 6 | 151 | 75,0 % | 60,0 % | 66,7 % | 7,1 % |

Pour le candidat réentraîné, l'intervalle Wilson à 95 % vaut [46,8 % ; 91,1 %]
pour la précision et [35,7 % ; 80,2 %] pour le rappel. Le rappel ponctuel reste
donc sous l'objectif préétabli de 90 %.

Sur le test ciblé équilibré, les annotateurs produisent 178 consensus sur 200
documents (couverture 89,0 %), avec 17 désaccords et cinq abstentions :

| Modèle figé | TP | FP | FN | TN | Précision | Rappel | F1 | Transmission |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Déployé, seuil 0,581961 | 0 | 0 | 83 | 95 | indéfinie | 0,0 % | 0,0 % | 0,0 % |
| Seuil seul, 0,129360 | 17 | 16 | 66 | 79 | 51,5 % | 20,5 % | 29,3 % | 18,5 % |
| Réentraîné, seuil 0,497129 | 81 | 3 | 2 | 92 | 96,4 % | 97,6 % | 97,0 % | 47,2 % |

Le candidat atteint une spécificité de 96,8 % sur ce test ciblé. Ses intervalles
Wilson à 95 % sont [90,0 % ; 98,8 %] pour la précision et [91,6 % ; 99,3 %]
pour le rappel. Cette distribution équilibrée ne permet pas d'estimer la
précision en production.

Verdict final : **non qualifié** (`not_qualified`). Les contrôles OOF et de rappel
sur le test original échouent, même si les contrôles du test ciblé réussissent.
`production_promotion` et `selection_on_test` valent tous deux `false` : le
candidat n'est pas promu et les poids ainsi que le seuil de production restent
inchangés. Les faibles effectifs positifs et français, la référence par accord IA
et l'absence de données de forums Tor demeurent des limites majeures.

## Préparation de l'itération suivante

Le test consommé n'est pas réutilisé pour régler le candidat échoué. Le moteur de
validation croisée prépare désormais une famille bornée, déclarée avant le
prochain test indépendant, qui compare en interne :

- le vocabulaire complet actuel et un vocabulaire filtrant les termes vus une
  seule fois tout en réservant 1 500 places aux mots ;
- le poids positif équilibré actuel et un multiplicateur positif de 1,5 ;
- les quatre régularisations L2 déjà prévues.

Chaque combinaison reconstruit son vocabulaire, ses IDF et ses poids sans le pli
retenu. Le rapport de sélection enregistre la configuration de caractéristiques
et le multiplicateur positif, afin qu'une amélioration de rappel ne soit pas
attribuée implicitement au seuil. Cette modification ne change aucun artefact
figé, aucun reçu de consommation et aucun paramètre de production. Une nouvelle
collecte indépendante reste nécessaire avant toute qualification ou promotion.

L'[audit des sources du 19 octobre](nlp-source-audit-2026-10-19.md) a ensuite
évalué CERT-EU, CERT-FR, CISA KEV et plusieurs réserves potentielles. Après
annotation et exclusion des voisins des données déjà consommées, 63 nouveaux
positifs consensuels ont été retenus. Le candidat exploratoire correspondant
atteint 68,4 % de F1 et 68,4 % de rappel hors pli, contre 64,9 % et 63,2 % pour
le précédent. Il reste sous l'objectif de rappel de 90 %, n'a lu aucun ancien
test et n'est pas promu. Le résultat confirme que les sources CTI officielles
apportent un signal auxiliaire, mais que la prochaine collecte doit prioriser
des textes conversationnels réellement proches du flux cible.
