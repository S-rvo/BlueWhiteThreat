# Campagne autonome NLP v2 — 19 octobre 2026

Suivi : [issue #34](https://github.com/S-rvo/BlueWhiteThreat/issues/34).

Cette campagne remplace le contrat d'annotation défaillant du premier pilote,
pas ses votes. Les anciennes tentatives restent archivées. Elle utilise un accord
IA comme référence exploratoire ; ses scores ne sont pas une certification humaine.

## Corrections et contrôles de développement

`annotation_v2.py` demande un JSON contraint par un schéma. Les modèles choisissent
un ou deux segments numérotés ; le programme reconstruit lui-même les citations et
leurs positions Unicode. Il ne demande plus aux petits modèles de recopier des
citations et ne présente pas une déclaration de lecture comme une preuve.
Le texte complet est fourni, sans troncature ; une limite de contexte, une réponse
invalide, une abstention ou un désaccord ne devient jamais une étiquette négative.

Les deux annotateurs sont `qwen3:8b` et `gemma4:e2b`, exécutés localement par Ollama.
Leurs empreintes, les prompts, le schéma, les paramètres et les textes sont liés
par des empreintes SHA-256. Ils ne reçoivent aucun score du classifieur évalué.
Les réponses sont enregistrées après chaque appel ; le verrou et le plan figé
permettent la reprise sans réétiqueter les décisions pour améliorer l'accord.

Contrôles effectués avant l'annotation du nouveau jeu :

- Dix textes de développement : 20 réponses exploitables, dix accords.
- 24 textes synthétiques déjà connus : 48 réponses exploitables et 24 accords
  correspondant aux étiquettes de diagnostic, dont 12 positifs et 12 négatifs.

Ces contrôles sont exclus des nouvelles partitions et ne sont pas une estimation
de la qualité sur le Web. Deux modèles peuvent partager la même erreur.

## Collecte et sélection

`apps/scraper/cmd/collectvalidation` a constitué un nouveau pool de 1 408 documents :
700 questions security, 600 questions superuser/askubuntu, 30 fils de forums
Developpez et 78 avis/alertes CERT-FR. Les avis complètent les forums ; ils ne sont
pas comptés comme des discussions de forums. La collecte utilise les API/flux et
pages publics, un agent identifié, des requêtes bornées et espacées, sans compte,
sans contournement ni relance des accès refusés. Le filtre partagé de sûreté est
appliqué avant persistance ; les corps HTML passent par l'extracteur du scraper.
Ce filtre lexical n'est pas une garantie de détection sémantique complète.

Les références d'entraînement, les 24 diagnostics, les dix textes de développement
et les IDs corrigés historiques sont exclus. Le contrôle lexical SimHash écarte
les distances inférieures ou égales à dix, contre les références puis entre
candidats ; les groupes de fils ne traversent pas les partitions. Ce contrôle
ne garantit pas l'indépendance sémantique de deux récits d'un même événement.

Il reste 544 documents éligibles : 254 questions sécurité, 271 questions générales,
sept fils français et douze avis français. Le tirage déterministe, sans consulter
les scores ni les annotations, réserve 200 textes de validation et 200 de test.

| Strate | Validation | Test |
|---|---:|---:|
| Questions sécurité EN | 95 | 95 |
| Questions générales EN | 95 | 96 |
| Fils de forums FR | 4 | 3 |
| Avis/alertes FR | 6 | 6 |

La faible taille française interdit une conclusion solide par langue. Ce corpus
public ne démontre pas la qualité sur les forums Tor ni les annonces d'accès initial.

## Évaluation figée

`automatic_campaign.py` fige les partitions et l'artefact de production corrigé.
`cmd/evaldocuments` calcule les empreintes ou les scores du cœur du classifieur Go,
sans lire de labels et sans écrire dans MongoDB/Redis. Sur les 24 diagnostics,
les sorties hôte et conteneur sont exactement identiques. La mesure de qualité
porte sur le classifieur ; elle ne mesure pas le rappel de découverte du crawler.

Le seuil est choisi exclusivement sur les accords de validation : maximiser la
précision sous rappel >= 90 % et précision >= 50 %, puis départager par rappel
et seuil décroissant. Si aucune solution n'existe, le meilleur F1 est conservé
comme diagnostic et la contrainte reste déclarée non satisfaite.

Le test réservé est ensuite consommé une seule fois. Le rapport compare le seuil
actuel et le candidat sur les mêmes accords, affiche TP/FP/TN/FN, précision,
rappel, F1, transmission et intervalles Wilson à 95 % pour précision/rappel. Il
sépare les strates et langues, et affiche les désaccords et abstentions exclus
des métriques. La qualification exploratoire exige aussi une couverture d'accord
>= 80 % sur chaque partition et au moins 50 accords de chaque classe dans le test.
Les intervalles décrivent l'échantillonnage conditionnel aux accords IA ; ils ne
prennent pas en compte leurs erreurs communes.

Les poids et le seuil de production ne sont pas remplacés par cette campagne.
Un candidat exporté conserve une provenance de validation IA explicite.

## Artefacts et reproduction

Les textes, citations, votes, modèle figé et candidat restent dans le répertoire
ignoré `apps/classifier/data/automatic-v2-2026-10-19/`. Les essais de développement
ont chacun leur plan ; leurs votes ne sont pas écrasés. Le manifeste du nouveau
jeu est dans `campaign/manifest.json`. Les résultats agrégés seront publiés avec
leur état réel, y compris en cas de non-qualification.

Depuis `apps/classifier`, pour reprendre une phase interrompue :

```sh
python3 -B scripts/annotation_v2.py \
  --input data/automatic-v2-2026-10-19/campaign/validation.jsonl \
  --out data/automatic-v2-2026-10-19/campaign/validation-annotations
# Seulement après annotation complète et si calibration.json n'existe pas :
python3 -B scripts/automatic_campaign.py calibrate \
  --dir data/automatic-v2-2026-10-19/campaign
# Annoter test.jsonl avec la même commande et --out test-annotations.
# Seulement si test-consumption.json et test-result.json n'existent pas :
python3 -B scripts/automatic_campaign.py evaluate \
  --dir data/automatic-v2-2026-10-19/campaign
```

Ne pas effacer les reçus pour optimiser de nouveau sur le test. Une prochaine
amélioration doit utiliser une nouvelle collecte indépendante.

L'orchestrateur `scripts/run_automatic_campaign.py --dir <campaign>` enchaîne
ces phases et reprend les votes déjà enregistrés. Il vérifie une calibration
existante et retourne un résultat terminé sans rouvrir le test. Un reçu de test
sans rapport complet reste une exécution interrompue explicitement en erreur ;
il ne disparaît pas automatiquement pour permettre un nouveau réglage.
La CI du classifieur exécute les contrôles Go et Python sans télécharger de modèle
ni accéder à Ollama. Les campagnes longues restent des exécutions explicites.

## Recalcul des verdicts historiques

La politique d'inférence est passée de `review-reasons-v2` à
`legacy-vectorizer-v3`. La correction de vectorisation conserve les poids du
modèle ; sans cette nouvelle version de politique, les anciens scores auraient
pu rester en base malgré la correction du binaire. Le filtre existant reprend
automatiquement ces documents et enregistre la politique corrigée. Le test MongoDB
vérifie qu'un résultat de l'ancienne politique est sélectionné avec la même version
de modèle, puis cesse de l'être après son remplacement par un résultat courant.

Le nouvel essai réel extraction navigateur → MongoDB → NLP → mise à jour du texte
passe après déploiement de cette politique. Les 13 documents présents ont été
reclassés avec `legacy-vectorizer-v3`. L'exemple CTI synthétique reste un faux
négatif au seuil actuel (score 0,526527 contre seuil 0,581961) ; le test de transport
le signale et ne le transforme pas en réussite sémantique. Le négatif obtient
0,056291 ; l'annuaire est exclu à zéro ; le texte modifié reçoit bien le nouveau
score avec la même version de modèle. Redis de test isolé, fixtures nettoyées.

La sélection sur validation donne 184 accords (19 positifs, 165 négatifs), neuf
désaccords et sept abstentions. Le seuil seul ne satisfait pas les contraintes ;
son meilleur F1 est 0,30. Une [expérience distincte de réentraînement](nlp-retraining-2026-10-19.md)
utilise ensuite ces accords comme développement, avec validation croisée et un
nouveau test réservé. Les résultats du test initial restent inchangés et séparés.

## Clôture de l'évaluation figée

La comparaison finale des trois modèles s'est exécutée sans réentraînement ni
ajustement après lecture des tests. Sur le test original, 169/200 documents ont
un consensus (84,5 %, 19 désaccords, 12 abstentions). Le modèle déployé obtient
un rappel de 0 % et un F1 de 0 ; le seuil seul, une précision de 9,1 %, un rappel
de 20,0 % et un F1 de 12,5 % ; le candidat réentraîné, une précision de 75,0 %,
un rappel de 60,0 % et un F1 de 66,7 % (9 TP, 3 FP, 6 FN, 151 TN).

Sur le test ciblé équilibré, 178/200 documents ont un consensus (89,0 %, 17
désaccords, cinq abstentions). Le candidat atteint 96,4 % de précision, 97,6 %
de rappel, 97,0 % de F1 et 96,8 % de spécificité (81 TP, 3 FP, 2 FN, 92 TN).
Le seuil seul atteint 51,5 % de précision et 20,5 % de rappel ; le modèle déployé
ne détecte aucun des 83 positifs consensuels.

Le verdict préétabli est **`not_qualified`** : la contrainte OOF et le rappel du
test original échouent. Aucune sélection n'a été faite sur les tests et le
candidat n'est pas promu (`selection_on_test: false`,
`production_promotion: false`). Le détail, les intervalles Wilson, les strates et
les empreintes figurent dans
`apps/classifier/data/automatic-v2-2026-10-19/retraining/comparison-result.json`.
