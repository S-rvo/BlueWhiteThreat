# Qualifier le classifieur et mesurer l’extraction séparément

**État :** nouveau lot humain à constituer. Le
[smoke test](validation/2026-11-24-pipeline-smoke.md) vérifie les échanges entre
composants ; il n’est pas un résultat de qualification.

## Classifieur : un nouveau lot prospectif et annoté en aveugle

1. Consulter le [registre](../apps/classifier/EXPERIMENTS.md). Écarter les
   réserves consommées, les données d’entraînement et de réglage, et les
   documents déjà inspectés. Figer le modèle exact à évaluer, sa politique,
   ses seuils effectifs et leurs empreintes. Le modèle de référence et un
   éventuel candidat doivent être nommés explicitement, sans promotion.
2. Constituer un lot sur une **nouvelle période**, sur des sources approuvées et
   selon une règle d’échantillonnage écrite. Garder les groupes
   hôte/acteur/événement et quasi-doublons hors de tout jeu déjà utilisé.
   Conserver à part un diagnostic ciblé des cas rares ; ne pas le mélanger à
   l’estimation représentative de production.
3. Avant d’ouvrir les prédictions, fixer la taille, les critères par source,
   les effectifs minimum de vrais positifs et de prédictions positives,
   ainsi que les seuils et la règle de décision. Un lot insuffisant reste
   **non concluant** ; pas de prolongation opportuniste. Le protocole de
   [confirmation gelée](confirmation-nlp.md) décrit les outils, les contrôles
   et les intervalles ; son candidat historique et son dossier par défaut ne
   sont pas la nouvelle campagne.
4. Faire annoter le lot par deux personnes indépendamment, sans score ni verdict
   modèle. Conserver l’accord brut et les désaccords ; les arbitrer en aveugle
   avec la politique CTI écrite. Rapporter le nombre d’exclus/indéterminés et
   la couverture, au lieu de les masquer. Un consensus LLM n’est pas un gold
   humain.
5. Évaluer une seule fois avec les réglages gelés : TP/FP/TN/FN, précision/
   rappel et intervalles de confiance à 95 % avec leurs dénominateurs ;
   rapporter aussi les faux positifs à rappel comparable lorsque la
   comparaison a été prévue. Si le lot est corrélé par groupe, rapporter la
   limite des intervalles naïfs ou utiliser une incertitude par groupe fixée
   avant la mesure. Archiver manifeste, critères, agrégats, décision et statut
   de réserve dans le registre ; conserver le texte et les annotations hors Git.

## Parser : comparer les entités émises à un gold distinct

Pour ne pas mélanger les erreurs du filtre et celles du parser, comparer
d’abord le parser sur les documents humains pertinents, y compris les documents
sans entité. Une deuxième mesure de la chaîne complète est possible sur tous
les documents, en identifiant séparément les erreurs de filtrage. Un échec de
transport/extraction ne doit pas être encodé comme une réussite vide : rapporter
la couverture et corriger le jeu avant l’évaluation stricte.

Depuis `apps/parser` :

```sh
go run ./cmd/evalentities \
  -gold /chemin/prive/gold.jsonl \
  -predictions /chemin/prive/predictions.jsonl \
  -version extractor-modele-config-figes
```

Une ligne JSON par document ; le schéma suit `entities.Entities` :

```json
{"id":"doc-001","entities":{"iocs":{"ips":["192.0.2.42"]},"cves":["CVE-2024-1234"]}}
{"id":"doc-002","entities":{}}
```

Les deux fichiers doivent avoir exactement les mêmes IDs, une fois chacun.
Un résultat réussi mais vide s’écrit `"entities":{}`. Les espaces externes
sont retirés ; la casse n’est pas comparée pour les domaines, hashes et CVE.
Pas d’alias, ni de dédoublonnage entre documents ; les répétitions d’une entité
dans un même document ne comptent qu’une fois. Les prédictions doivent être
celles émises après validation par le parser, sans nettoyage supplémentaire
par l’évaluateur.

Le rapport JSON contient les empreintes SHA-256 des entrées, la version
déclarée, les documents, TP/FP/FN, précision/rappel/F1 par catégorie et micro.
Une métrique sans dénominateur vaut `null`, pas 0 ou 1. Aucun texte ni ID
individuel n’est copié dans le rapport. L’outil ne produit ni les annotations
humaines ni les prédictions et n’appelle aucun modèle.
