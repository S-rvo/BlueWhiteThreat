# Audit — pourquoi le classifieur ne s'améliore pas sur la vraie pipeline ?

Audit diagnostique des deux tests réels consommés, notamment la tranche
prospective du 30 octobre. Ce document n'ajuste aucun modèle ni seuil. Les pages
de test ne seront pas réutilisées pour l'entraînement.

## Diagnostic en une phrase

Le modèle confond encore des **indices commerciaux génériques** (prix, cartes,
ventes, boutiques et jeux) avec des indices CTI explicites. Le candidat ransomware.live
n'a pas amélioré le classement des pages de production : son gain de rappel vient
surtout d'un seuil beaucoup plus permissif, qui multiplie les faux positifs.

## Faits mesurés sur le test prospectif

Sur 705 consensus automatiques (74 positifs, 631 négatifs; 10,5 % de positifs) :

| Mesure | Production | Candidat |
|---|---:|---:|
| Précision / rappel / F1 | 22,66 / 85,14 / 35,80 % | 16,12 / 93,24 / 27,49 % |
| TP / FP / FN / TN | 63 / 215 / 11 / 416 | 69 / 359 / 5 / 272 |
| Pages signalées positives | 278/705 (39,4 %) | 428/705 (60,7 %) |
| Taux de faux positifs sur négatifs | 34,1 % | 56,9 % |
| AUROC / average precision | 0,838 / 0,471 | 0,821 / 0,426 |

Le candidat ajoute 6 vrais positifs mais aussi **144 faux positifs**. Son seuil
sélectionné sur le développement ransomware.live (0,3485) est nettement inférieur
au seuil production (0,4888). Cependant, le résultat AUROC/AP inférieur montre
que ce n'est **pas seulement un mauvais seuil** : le classement candidat des
pages de production est lui aussi moins bon. Sur le test rétrospectif antérieur,
les deux modèles dépassaient 92 % de rappel mais manquaient déjà la précision;
ce second test confirme le problème sur des données ultérieures.

## Causes racines étayées

### 1. Décalage entre développement et distribution réelle

Le développement candidat comptait 107 positifs sur 498 consensus ransomware.live
(21,5 %); le test réel n'en compte que 10,5 %. Le CSV de base d'entraînement
contient 28,0 % de positifs. Dans le flux réel, 89,5 % des consensus sont négatifs.
Le lot de développement n'est donc pas représentatif du taux de base ni du
bruit commercial rencontré dans la collecte. Cela réduit la précision, mais ne
suffit pas à expliquer seul le taux de faux positifs élevé : la représentation
lexicale sépare aussi imparfaitement les deux classes.

### 2. Raccourcis de vocabulaire commercial

- 248/631 négatifs contiennent une mention de prix; c'est le cas de 162/215 faux
  positifs production (75 %).
- 150/631 négatifs déclenchent l'indicateur de modèle `marketplace_boilerplate`;
  81/215 faux positifs production en ont un.
- Les motifs casino apparaissent dans 39 négatifs et 37 sont signalés positifs
  par la production. Les motifs betting apparaissent dans 35 négatifs et 34 sont
  signalés positifs.
- Le poids de `s_price_mention` est fortement positif dans les deux modèles
  (+1,388 production, +1,586 candidat). Dans le candidat, `s_financial_fraud_market`
  vaut +0,724, et `s_financial_fraud_market_without_evidence` vaut +0,377;
  `s_marketplace_boilerplate` vaut +0,490. Les signaux génériques de prix et de
  commerce peuvent donc faire monter le score sans preuve explicite de
  compromission.

C'est cohérent avec des termes que le contrat d'annotation ne considère pas
suffisants à eux seuls : prix, prépayé/contrefaçon, casino, boutique ou vente
commerciale. Le modèle voit des mots associés à la menace mais ne modélise pas
assez le contexte qui distingue une demande de rançon ou des données volées d'un
produit ordinaire.

### 3. Le candidat ransomware.live a optimisé le mauvais compromis

Sur son développement faible, le candidat satisfait les objectifs OOF de
sélection, mais le seuil/configuration sont sélectionnés sur les mêmes données
qui servent à mesurer ces OOF : estimation optimiste, et source moins proche du
flux cible. Le poids positif de 1,5 et le seuil bas privilégient le rappel. Sur
le test réel, le candidat prédit positif pour 61 % des pages et manque toujours
la précision de 50 %. Son AUROC et son average precision sont inférieurs à la
production sur la tranche prospective; l'ajout ransomware.live n'a donc pas
produit la séparation attendue sur le domaine réel.

### 4. Le texte complet de page est bruyant

Le médian de longueur est 1 249 caractères pour les négatifs consensuels, mais
3 931 caractères pour les faux positifs de production. Les faux positifs riches
en texte peuvent cumuler annonces, navigation, contenu de boutique et prix. Le
pipeline semble donc devoir examiner la qualité d'extraction du contenu principal,
pas seulement ajouter des n-grammes ou changer le modèle. Cette explication est
une hypothèse étayée par les longueurs et motifs de texte, à vérifier sur le pool
de développement sans réutiliser les tests.

### 5. Les labels automatiques restent incertains

Sur 69/215 faux positifs production, au moins un des trois annotateurs a coché
un critère positif alors que la majorité donne la classe négative. Cela révèle
une frontière ambiguë et limite la précision de la mesure; ce n'est toutefois
pas suffisant pour expliquer à lui seul des centaines de faux positifs. Les
consensus ne sont pas une vérité terrain humaine.

## Décision et plan recommandé

1. **Ne rien promouvoir et ne pas retoucher les seuils** : deux tests réels
   consommés échouent au critère de précision; garder le modèle production intact.
2. Finir l'annotation du pool de développement post-test de 91 pages, puis
   l'élargir avec les nouvelles pages si les exemples ou classes sont trop peu
   nombreux. Ce pool, distinct des deux tests, ne sert qu'au diagnostic et au
   développement.
3. Renforcer sur ce seul développement les négatifs difficiles représentatifs
   (annonces, casino/pari, boutiques, cartes prépayées/contrefaites, prix et
   pages de profil) et des positifs contrastifs où la compromission ou les
   données volées sont explicitement revendiquées. Ne jamais étiqueter par la
   source seule.
4. Auditer l'extraction du corps principal et revoir la sémantique des signaux
   `price`, marketplace et financial-fraud. Tester les interactions avec une
   preuve explicite de compromission plutôt que considérer un prix comme un
   signal positif autonome.
5. Comparer les changements en validation groupée source/hôte et temporelle,
   avec taux de base proche du flux. Geler un seul candidat et son seuil avant un
   **nouveau** test prospectif. Promotion uniquement si précision >=50 % et
   rappel >=90 % sont simultanément franchis avec au moins 50 consensus de
   chaque classe.

Le pool de développement séparé (91 documents) est annoté : 88 consensus, 2 abstentions, 1 désaccord. Le petit pilote v9/v10 et sa décision de ne pas scaler sont documentés dans `nlp-small-pilot-method-2026-10-30.md`. Les artefacts privés restent sous `apps/classifier/data/iteration-2026-10-27-source-refresh/prospective-production-pipeline-comparison-v1/`.
Les mesures et empreintes détaillées de l'audit du test sont dans
`snapshot-02-error-audit.json`.

## Correctif expérimental commencé

Le vectoriseur ajoute maintenant deux caractéristiques nouvelles :
`s_price_mention_with_evidence` et `s_price_mention_without_evidence`. La
caractéristique historique de prix est conservée pour compatibilité; ces
interactions sont destinées au réentraînement d'un candidat isolé, pas à une
modification manuelle de poids. Les interactions réutilisent le motif d'évidence
déjà présent, sans en élargir la définition dans ce correctif.

Tests unitaires `internal/tfidf` et `internal/classify` passés après ce
changement. Les nouveaux jetons ne sont présents dans aucun des deux modèles
figés; les scores de production et des tests ne changent donc pas. Le
`model.json` production demeure strictement inchangé. Ce correctif est
expérimental : il faut annoter le pool post-test, entraîner un nouveau candidat
avec ses propres exemples, puis mesurer uniquement en validation développement
groupée avant tout futur test prospectif. Il ne qualifie ni ne promeut un modèle.
