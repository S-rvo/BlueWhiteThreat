# Confirmation du classifieur NLP

Le candidat retenu est `model_human.json`, provisoire. La validation connue
(précision 0,636 ; rappel 0,848 sur 116 documents) a servi à le choisir : elle
ne constitue pas la confirmation finale. Les 107 documents déjà consultés sont
un benchmark de développement. Aucun outil ci-dessous ne promeut un modèle.

## 1. Figer le candidat avant de collecter les annotations

Depuis `apps/classifier` :

```sh
CANDIDATE_MODEL_PATH=model_human.json go run ./cmd/prepareconfirmation
```

Le dossier `data/confirmation-campaign/` contient une copie du modèle, du corpus
complet de développement, un manifeste SHA-256 et un modèle de CSV vide.
Le seuil, la version du modèle et les sources d'inférence/prétraitement sont
liés au manifeste. Un dossier existant n'est jamais écrasé. Les empreintes
permettent de détecter un changement accidentel ; ce n'est pas une archive
inviolable. Conserver le dossier, son manifeste et le code ensemble.

La photographie du corpus doit inclure TOUS les textes déjà utilisés pour
entraîner, régler ou inspecter les candidats, même ceux sans annotation humaine.
Le programme ne peut pas deviner des expériences menées hors de ce corpus.
Ne pas réentraîner le candidat après cette étape. Une modification du code
contrôlé fait refuser la confirmation ; elle demande une nouvelle décision
expérimentale explicite, pas une mise à jour manuelle de l'empreinte.

## 2. Préparer un lot nouveau sans consulter les scores

Fournir un CSV `id,text` provenant d'une nouvelle collecte, filtrée par les
exclusions habituelles du projet. Conserver hors du CSV les métadonnées de
source/date et documenter la période et la population échantillonnées. Ne pas
utiliser la file d'active learning : elle est sélectionnée selon les scores.
Ne pas reprendre des documents déjà lus lors du développement sous un nouvel ID.

```sh
CONFIRMATION_POOL=/chemin/nouveaux_documents.csv \
CONFIRMATION_SIZE=300 \
go run ./cmd/prepareconfirmation
```

La taille de 300 est un lot exploratoire par défaut, pas une garantie de pouvoir
certifier 99 % de précision. L'incertitude dépend surtout du nombre de
prédictions positives. Définir le volume et les critères d'acceptation AVANT de
voir les résultats ; ne pas ajouter des lots jusqu'à obtenir un résultat favorable.

L'outil mélange les IDs de manière reproductible (graine 42), sans modèle ni
labels, rejette les IDs connus, doublons normalisés et voisins SimHash (distance
<= 10), puis ne conserve qu'un représentant par voisinage dans le lot. Il compare
chaque candidat à tous les textes de référence : aucun raccourci par représentant
de cluster. La sortie précise le nombre retenu et les exclusions rencontrées.

`selection.json` fige les IDs et empreintes avant annotation. `annotations.csv`
contient le texte et une colonne `label` vide. Un second tirage dans la même
campagne est refusé. La détection SimHash est lexicale : elle peut écarter des
textes distincts et manquer des paraphrases. Vérifier aussi les sources et les
événements recopiés avant le gel du lot ; ce filtrage définit la population
évaluée et doit être documenté.

## 3. Annoter humainement

- `1` : contenu contenant une information CTI exploitable selon la politique
  éditoriale du projet (ex. revendication précise de fuite, offre d'accès initial,
  détails d'une campagne malveillante ou d'une vulnérabilité).
- `0` : navigation, publicité générique, tutoriel général, contenu hors périmètre
  sans information exploitable. La simple présence de mots CTI ne suffit pas.
- Une mention n'établit pas la véracité d'une revendication ni la malveillance
  d'une IP : annoter la pertinence, pas une attribution certaine.
- Documenter les cas ambigus, les faire arbitrer sans consulter les scores ;
  ne pas forcer une étiquette uniquement pour terminer le CSV. Confirmer ces
  consignes avec l'analyste responsable avant de commencer.

Remplir uniquement `label`, exactement `0` ou `1`. Conserver tous les documents
sélectionnés et leurs textes. Une ligne manquante, un ID répété, une annotation
vide/incorrecte ou un texte remplacé fait échouer la confirmation. Ne pas importer
ces annotations dans `reviewed_labels.csv` avant la confirmation.

## 4. Confirmer une fois

```sh
go run ./cmd/confirmfinal
```

Sans fichier d'annotations : message « nothing scored », aucun résultat inventé.
Avec un CSV incomplet : erreur, sans consommer la campagne. Après tous les
contrôles, `attempt.json` réserve la campagne AVANT le calcul. `result.json`
contient TP/FP/TN/FN, métriques, empreintes du lot/modèle/corpus/code et intervalles
Wilson à 95 % pour précision et rappel (null si le dénominateur est nul).

Le journal partagé `data/.confirmation-history/` refuse la réutilisation de
documents consommés dans une autre campagne du même répertoire, même sous un
nouvel ID si leur texte est identique ou proche. Les réservations sont exclusives :
une confirmation simultanée ou répétée est refusée. Ne pas déplacer/supprimer ce
journal pour contourner une erreur. Il n'existe pas de protection contre une
suppression volontaire des fichiers ou un journal placé dans un autre répertoire.

Un crash après réservation laisse la campagne consommée. Auditer le problème ;
ne pas effacer `attempt.json` ou un verrou pour relancer aveuglément. Les résultats
ne sont affichés qu'après leur écriture réussie. Les anciens journaux CSV restent
des archives et ne constituent pas ce nouveau mécanisme de réservation.

Le rapport indique que l'objectif de précision 99 % est étayé seulement si la
borne basse de l'intervalle atteint 0,99. Cet intervalle suppose un échantillon
suffisamment indépendant et représentatif ; il ne corrige pas un biais de collecte.
Le rappel doit aussi être examiné. Aucune promotion automatique en production.

## État actuel

Les outils sont prêts. Il reste à fournir une nouvelle collecte indépendante et
ses annotations humaines. Un fichier vide n'est pas un jeu d'évaluation et un
label produit par un assistant ne doit pas être présenté comme une annotation
humaine. Ne pas annoncer de nouvelle précision finale avant cette étape.
