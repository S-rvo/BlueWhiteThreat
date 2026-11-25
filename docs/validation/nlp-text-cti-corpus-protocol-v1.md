# Protocole corpus `text_cti_evidence` v1 — gel du 27 octobre 2026

## Cible gelée

Le classifieur texte vise `text_cti_evidence` : le contenu principal visible
décrit-il explicitement une preuve/action de cybermenace utile à la veille ?
La classe positive couvre incident/intrusion, ransomware/extorsion, données
dérobées ou publiées, accès compromis offert, campagne/service malveillant,
vulnérabilité/exploit concret, ou défense liée à une menace concrète. La classe
négative exige que le texte principal soit lisible et relève d'un sujet ordinaire
sans aucun de ces signaux. Le texte court, tronqué, contradictoire ou ambigu est
`abstain`, jamais transformé en négatif par défaut.

`structured_claim_relevance` reste un axe séparé, hors de ce corpus. L'appartenance
à une source, un endpoint ou une liste de victimes n'est jamais une étiquette de
vérité et ne suffit pas à attribuer `text_cti_evidence=1`. Les champs source,
date, acteur, URL et strate sont conservés comme provenance/métadonnées seulement.

## Annotation automatique

Le protocole d'annotation v9 est gelé par empreintes dans chaque run. Trois
modèles locaux distincts votent sur le texte complet et des critères atomiques;
au moins deux décisions valides concordantes constituent un weak consensus.
Désaccord, sortie invalide, contexte insuffisant ou absence d'évidence donnent
une abstention/pending, sans arbitrage humain. Les labels portent
`label_authority=majority of three local AI models under policy v9; not human
truth`. Le modèle NLP n'est jamais inclus dans le prompt. Les votes, politiques,
reçus de consommation, plans et empreintes sont conservés.

## Échantillonnage et séparation

- Geler source, intervalle temporel, critères lexicaux, limites de longueur,
  graine et manifeste avant annotation; sélectionner sans scores NLP.
- Les échantillons lexicaux servent au développement seulement. Conserver aussi
  le flux source non filtré pour les audits de sélection et de prévalence.
- Dédupliquer les IDs, textes normalisés, shingles de production et groupes
  d'acteurs/contenu avant le split. Les conflits de label restent exclus et
  consignés. Aucun groupe, acteur ou contenu quasi-identique ne traverse les
  partitions.
- Développement et challenge externe sont collectés séparément. Le test reste
  fermé jusqu'au gel du candidat, de son seuil et du rapport attendu; une seule
  lecture, puis il est consommé. Pas de réentraînement ni de réglage de seuil
  après sa lecture.

## Portes de données et de promotion

Avant tout candidat évalué : couverture de consensus >=80 %, au moins 50
consensus de chaque classe au total, et au moins 10 de chaque classe dans
chaque source et strate déclarées. Le protocole de campagne existant préenregistre
les objectifs précision >=0,50 et rappel >=0,90 sur validation puis test,
ainsi que leurs résultats par source/strate. Une classe absente d'un groupe
fait échouer la campagne, même si le score global est élevé. Un test composé de
weak consensus IA ne donne qu'une qualification exploratoire, pas une vérité
terrain indépendante; son autorité est toujours explicitée.

La production reste au modèle SHA-256
`1fd5dd5e5531c5b2b80586ae6b1bb23b0f0daa105a232d7f2c76c03fccc36458`, seuil
`0.4887874623244017`, jusqu'à une campagne prospective complète qui passe
toutes les portes. Tout candidat est stocké sous un chemin distinct; aucun
commit/push n'est implicite.

## État d'application

Ce contrat fixe la cible des prochaines collectes sans réécrire les anciens
labels/corpus ni promouvoir de modèle. Les données précédentes gardent leur
provenance et leur statut expérimental. La première nouvelle collecte de ce
cycle est le lot Security StackExchange borné avant le 30 juin 2024, dont le
filtre lexical et l'audit sont consignés dans
`data/iteration-2026-10-27-source-refresh/`.
