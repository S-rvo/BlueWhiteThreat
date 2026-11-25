# Audit des sources NLP — 19 octobre 2026

Suivi : [issue #34](https://github.com/S-rvo/BlueWhiteThreat/issues/34).

## Besoin réel

Le classifieur doit reconnaître des contenus CTI pertinents, notamment des récits
d'incident, des activités de menace et des offres ou discussions d'accès initial.
Le précédent candidat apprenait surtout sur des avis de vulnérabilité structurés.
Son excellent résultat sur un test avis/questions, puis son rappel de 60 % sur le
test original, indiquent un décalage de domaine. Une source n'est utile que si
elle apporte du langage proche de la cible ; sa réputation ne suffit pas à lui
donner automatiquement une étiquette positive.

## Audit

| Source potentielle | Signal | Proximité avec la cible | Accès et structure | Décision |
|---|---|---|---|---|
| Publications Threat Intelligence du CERT-EU | Récits de campagnes, acteurs, incidents et tendances | Moyenne : CTI narrative, mais style institutionnel et non conversationnel | RSS officiel, pages HTML publiques ; dix publications récentes dans le flux | Collecter pour annotation par sections ; exclure les rapports entiers trop longs |
| Rapports CTI du CERT-FR | Récits techniques en français | Moyenne à bonne pour le vocabulaire CTI FR ; faible proximité avec une annonce de forum | RSS officiel et pages publiques | Utiliser uniquement les consensus, avec groupe par rapport |
| CISA Known Exploited Vulnerabilities | Exploitation observée et indicateur ransomware explicite | Faible à moyenne : excellent signal d'exploitation, texte très structuré | JSON officiel versionné, stable et déduplicable par CVE | Utiliser comme signal auxiliaire d'apprentissage, jamais comme test représentatif ni label automatique |
| CISA Cybersecurity Advisories / CSAF | Rapports et TTP de menaces, mais beaucoup d'avis produit/ICS | Moyenne pour les rapports de campagne, faible pour les avis produit | Dépôt CSAF public et pages officielles | Candidat futur après filtrage par type ; éviter de dupliquer KEV/GHSA |
| Flux MISP du CERT-FR | Événements et indicateurs techniques | Faible pour un classifieur de prose : nombreux hashes, domaines et attributs | Flux JSON MISP public | Ne pas injecter dans ce modèle textuel ; utile à un détecteur d'IOC séparé |
| GitHub Security Advisories, NVD et avis produit | Descriptions de vulnérabilités propres et abondantes | Faible : source déjà surreprésentée dans l'essai précédent | APIs structurées | Ne plus augmenter cette famille pour corriger le rappel conversationnel |
| Stack Exchange / forums techniques généralistes | Questions ambiguës et vocabulaire technique courant | Bonne source de négatifs difficiles, faible source de vrais positifs CTI | APIs/RSS publics | Conserver pour les négatifs difficiles et la mesure, pas comme réserve positive |
| Publications réelles des sources ciblées | Langage, bruit et intentions correspondant au flux de production | Très forte | Seulement via la collecte applicative et ses règles existantes | Priorité pour la prochaine collecte d'observation ; grouper par fil/source et annoter sans score NLP |

Références publiques vérifiées : [catalogue KEV de la CISA](https://www.cisa.gov/known-exploited-vulnerabilities-catalog),
[définition des avis CISA](https://www.cisa.gov/news-events/cybersecurity-advisories),
[dépôt CSAF officiel](https://github.com/cisagov/CSAF),
[publications CTI du CERT-EU](https://cert.europa.eu/publications/threat-intelligence/),
[description de ses flux RSS](https://cert.europa.eu/blog/read-feed-and-lead-fresh-features-for-our-cyber-fans),
[flux CTI du CERT-FR](https://cert.cyber.gouv.fr/cti/feed/) et
[flux MISP du CERT-FR](https://misp.cert.ssi.gouv.fr/feed-misp/).

## Collecte et contrôle réalisés

`cmd/collectcti` a collecté dans un espace privé, sans écriture en production :

- 10 rapports CERT-EU ;
- 10 rapports CERT-FR ;
- 100 entrées KEV marquées par la CISA comme liées à une campagne ransomware.

Les 120 documents ont des groupes et empreintes uniques. Aucun n'a reçu de label
à partir de sa source. Le modèle actuellement déployé les rejette tous au seuil
0,581961 : score moyen 0,249 pour CERT-EU, 0,029 pour CERT-FR et 0,135 pour KEV.
Ce diagnostic confirme une lacune de vocabulaire ou de domaine, mais ne suffit pas
à déclarer les documents pertinents.

Les deux annotateurs locaux ont ensuite produit 105 consensus, quatre désaccords
et onze abstentions, soit 87,5 % de couverture :

| Source | Positifs consensuels | Désaccords | Abstentions | Observation |
|---|---:|---:|---:|---|
| CERT-EU | 0 | 1 | 9 | Neuf rapports dépassent le contrat de contexte ; unité documentaire inadaptée |
| CERT-FR | 5 | 3 | 2 | Signal utile mais faible volume et accord imparfait |
| CISA KEV ransomware | 100 | 0 | 0 | Accord élevé, avec risque d'erreur commune lié au format très explicite |

Les rapports CERT-EU complets ne sont donc pas injectés. Une collecte ultérieure
devra les segmenter en sections cohérentes, conserver un groupe commun et placer
toutes les sections d'un rapport dans une seule partition.

## Utilisation dans un candidat suivant

Après exclusion des groupes connus et voisins lexicaux de l'entraînement, du
développement et des deux tests consommés, 63 des 105 positifs consensuels sont
nouveaux ; 42 voisins sont exclus. Ils sont ajoutés aux 100 avis faibles précédents
dans un supplément de **163 positifs d'apprentissage**, sans incorporer les tests.

La validation croisée à cinq plis compare 16 combinaisons fixées de vocabulaire,
pondération positive et L2. Le meilleur résultat OOF passe de 64,9 % à **68,4 % de
F1**, avec 68,4 % de précision et 68,4 % de rappel (13 TP, 6 FP, 6 FN). La solution
retient le vocabulaire complet, un multiplicateur positif de 1,5, L2 = 0,001 et
un seuil de 0,551267.

L'objectif de rappel de 90 % reste inaccessible. Cette amélioration de
développement ne qualifie pas le candidat et ne justifie aucune promotion. Elle
montre que les sources officielles apportent un signal utile, mais ne corrigent
pas le manque principal de textes réellement proches des publications ciblées.
Le prochain test doit provenir d'une nouvelle collecte indépendante et inclure
des sources conversationnelles regroupées par fil.

Artefacts locaux : `data/source-audit-2026-10-19/`. Candidat exploratoire :
SHA-256 `9ac0b77c3da1cd9635125dede8478565c774509a3f42efe7e3b1da0399cf28c6`.
`production_promotion` et `test_read` valent `false`.

## Observation du flux onion existant

Le pipeline Tor déjà actif a été interrogé sans reprendre ses décisions NLP.
MongoDB contenait 25 pages avec contenu ; après retrait de deux pages trop
courtes et déduplication exacte, 21 documents ont été exportés dans un lot privé.
Les hôtes et URL sont remplacés par des identifiants hachés et le score de
production n'est pas transmis aux annotateurs.

L'annotation indépendante produit 12 consensus, sans désaccord, et neuf
abstentions, soit 57,1 % de couverture. Les consensus comprennent seulement
**deux positifs** et dix négatifs. Trois pages dépassent le budget de contexte ;
les autres abstentions proviennent surtout de pages d'index ou de contexte
insuffisant. Ce lot est trop petit et trop peu positif pour entraîner ou tester
un nouveau modèle. Il n'est donc injecté nulle part.

Ce résultat confirme expérimentalement que parcourir largement Tor n'apporte pas
automatiquement une bonne source d'apprentissage. La file actuelle provient en
grande partie de répertoires et de fermes de liens. La collecte réelle reste
cependant indépendante du modèle : aucun filtre de vocabulaire CTI n'est appliqué
par le crawler. La sélection par expressions fortes ou signaux multiples doit
être réalisée uniquement lors de la préparation privée d'un lot d'annotation ou
d'entraînement. Les contrôles de contenu bloqué et de fermes restent appliqués
avant collecte, et les reçus et documents antérieurs sont conservés.

Artefacts locaux : `data/target-observation-2026-10-19/`. Aucun nouveau test,
entraînement ou changement de production n'est déclenché par cette observation.

## Lot privé de 500 documents

La collecte complémentaire atteint désormais exactement **500 textes uniques** :

- 21 observations provenant du pipeline cible ;
- 379 publications CTI officielles uniques (CERT-EU, CERT-FR et CISA KEV) ;
- 100 avis publics relus par GitHub, conservés comme réserve de négatifs
  difficiles et non comme positifs automatiques.

Le fichier `data/source-audit-2026-10-19/diverse-pool-500.jsonl` est non scoré et
ne contient aucun label déduit de la provenance. Les 500 identifiants sont
dérivés des empreintes du texte et les doublons exacts sont exclus. La forte
proportion de sources institutionnelles est explicitement conservée dans le
champ `pool_role` : ce lot atteint l'objectif de volume pour l'annotation, mais
ne prétend pas reproduire à lui seul la distribution de production. Aucun de
ces documents n'est encore affecté au test ou à l'entraînement.

## Extension ransomware.live

Un collecteur borné utilise l'API publique v2 sans transformer l'appartenance à
la source en classe. Il rejette les entrées de moins de 100 caractères et garde
le nom de la victime avec la description observable ; le groupe, le pays et le
secteur restent des métadonnées afin de limiter les raccourcis de source.

La collecte d'octobre 2026 contient 432 documents substantiels issus de 56
groupes, avec une longueur médiane de 403 caractères. Aucun doublon exact après
normalisation n'a été trouvé. Deux groupes fournissent toutefois 159 documents :
l'échantillon d'annotation est donc plafonné à cinq documents par groupe. Une
sélection déterministe de 120 documents couvrant 40 groupes a été créée. Elle a
d'abord été conservée sans étiquette et sans modifier les entraînements
existants.

L'annotation a finalement couvert deux lots de développement, soit 297
documents. Elle produit 217 consensus : 18 positifs et 199 négatifs, plus 42
abstentions et 38 désaccords. Sur le premier lot figé avant scoring, v7 retrouve
les cinq positifs mais produit 35 faux positifs sur 84 négatifs ; v4 ne retrouve
qu'un positif et produit 21 faux positifs. Ces données apportent donc à la fois
des positifs ciblés et surtout des négatifs difficiles du même style.

Le candidat exploratoire v8 ajoute les 18 positifs et un échantillon
déterministe de 36 négatifs à v7, après contrôle de proximité avec entraînement
et validation. Il conserve le F1 interne de v4 à 87,8 %. Les 135 documents non
utilisés ont été gelés comme contrôle indépendant avant leur annotation et tout
scoring de v8. Ce contrôle produit 49 consensus seulement : 2 positifs, 47
négatifs, 45 abstentions et 41 désaccords. Sur les accords, v8 obtient 1 TP, 5
FP, 1 FN et 42 TN (F1 25 %), contre 1/23/1/24 pour v7. Les 135 documents sont
très concentrés sur deux groupes (129 documents), ce qui limite encore la portée
du résultat. Le contrôle est consommé ; il indique une réduction des faux
positifs, pas une qualification. Aucun candidat n'est promu.
