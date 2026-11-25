# Corpus d'entraînement équilibré — 2026-10-19

## Résultat

Le corpus prêt pour l'entraînement contient **400 documents**, avec un équilibre
strict de **200 positifs / 200 négatifs** :

- `apps/classifier/data/balanced-training-2026-10-19/balanced-corpus-400.jsonl`
- export tabulaire : `balanced-corpus-400.jsonl.csv`
- manifeste reproductible : `balanced-corpus-400.jsonl.manifest.json`

Répartition des positifs : CISA 95, GitHub Advisories 69, CERT-EU 25,
CERT-FR 11. Répartition des négatifs : Ask Ubuntu 70, Super User 70,
Security StackExchange 50, observations cibles 10. Les plafonds empêchent une
source unique de constituer la majorité du corpus.

## Construction et contrôle

1. Collecte de 1 800 questions inédites sur les pages 10 à 15 de trois sources.
2. Sélection déterministe de 240 candidats, plafonnée à 80 par source.
3. Annotation indépendante par `qwen3:8b` et `gemma4:e2b` : 220 consensus,
   dont 210 négatifs et 10 positifs; 16 désaccords et 4 abstentions écartés.
4. Segmentation de 19 documents dépassant 12 000 caractères en 64 passages
   cohérents de 9 000 caractères au plus. La seconde annotation produit 60
   consensus (47 positifs, 13 négatifs); seuls 25 positifs CERT-EU sont retenus.
5. Sélection des seuls consensus concordants avec plafonds par source.

Les deux jeux de test déjà consommés (`campaign/test.jsonl` et
`retraining/test.jsonl`) sont exclus par groupe, texte normalisé et SimHash
(distance de Hamming <= 3). Quatre recouvrements exacts/de groupe ont ainsi été
écartés. Le même audit contre les 2 325 lignes de `training_data.csv` donne zéro
recouvrement exact et zéro quasi-doublon au seuil 3. Dans le corpus final, zéro
doublon exact subsiste et un quasi-doublon candidat a été écarté.

## Autorité des labels

Ces labels proviennent de l'accord de deux annotateurs locaux figés. Ils forment
une base d'entraînement contrôlée, mais ne remplacent pas une revue humaine des
cas sensibles. Les désaccords et abstentions restent hors corpus. Aucun seuil
n'a été ajusté et aucun test consommé n'a été relu pour entraîner le modèle.

## Reproduction

Les scripts ajoutés sont :

- `scripts/prepare_balanced_pool.py` pour les exclusions et plafonds initiaux ;
- `scripts/segment_documents.py` pour les textes longs ;
- `scripts/build_balanced_corpus.py` pour l'équilibrage, la déduplication et le
  contrôle de fuite vers les tests.

Cette étape ne promeut aucun modèle. Un nouvel entraînement devra rester séparé
des jeux de test figés et repasser le protocole de qualification complet.

## Candidat v2 et nouveau test aveugle

Une revue de contenu déterministe a porté sur 20 positifs et 20 négatifs, puis
sur les titres des 50 négatifs Security StackExchange. Cinq textes dont le corps
décrit explicitement une attaque, un logiciel malveillant, une injection ou une
technique d'évasion contredisaient leur label négatif. Ils ont été exclus, sans
réétiquetage, puis remplacés par cinq consensus négatifs disponibles. Le corpus
reste à 200/200. Le détail est dans `content-audit.json`.

Le corpus a ensuite été séparé par groupe en 300 documents d'entraînement
(150/150) et 100 de validation (50/50), sans groupe commun. La déduplication
SimHash du trainer réduit les 300 entrées à 230 groupes d'entraînement. Parmi
les quatre régularisations prédéfinies, `l2=0.001` et le seuil 0,518670 ont été
sélectionnés uniquement sur la validation : 50 TP, 50 TN, aucun FP/FN.

Un test final distinct a été collecté sur des pages StackExchange 16 à 20 et sur
50 entrées KEV encore inutilisées. Sur 200 textes, les annotateurs donnent 182
consensus (91 %), dont 56 positifs et 126 négatifs. Le seuil et le modèle étaient
figés avant le calcul des scores. Le premier essai d'écriture du reçu avait un
chemin relatif dupliqué ; il a échoué, tandis que la commande de score chaînée a
continué. Les scores ont été conservés et immédiatement empreintés dans le reçu,
avec cette anomalie explicitement enregistrée.

| Modèle | Précision | Rappel | F1 | TP/FP/FN/TN |
|---|---:|---:|---:|---:|
| Nouveau candidat | 100 % | 91,1 % | 95,3 % | 51/0/5/126 |
| Candidat précédent | 67,9 % | 98,2 % | 80,3 % | 55/26/1/100 |
| Déployé | 0 % | 0 % | 0 % | 0/1/56/125 |

Le nouveau candidat franchit les seuils globaux prédéfinis. Toutefois, cinq de
ses six faux négatifs positifs proviennent de Security StackExchange : rappel de
1/6 sur cette sous-strate, contre 50/50 sur KEV. Cette forte confusion
source/classe rend le score global trop optimiste. **Le candidat n'est donc pas
promu**. L'étape suivante est d'ajouter des positifs conversationnels et des
négatifs de style avis officiel, puis de recommencer avec un autre test vierge.

Artefacts : `candidate-v2/candidate.json`, `final-test/test-consumption.json`,
`final-test/final-result.json` et `final-test/model-comparison.json`.
## Itération cible v7 et nouveau contrôle du flux

Un corpus externe de forums, publié sous CC-BY-4.0 sur Zenodo (record
`21991378`), a été conservé avec son manifeste et son empreinte. Les textes ont
été expurgés par leur auteur ; aucune étiquette n'a été déduite de la source.
Sur 102 fils utilisables, les deux annotateurs locaux donnent 64 accords : 34
positifs et 30 négatifs, avec neuf abstentions et 29 désaccords. Ces 64 accords
ont été ajoutés au jeu d'apprentissage du candidat v4 pour former le candidat
v7 (202 textes après déduplication).

Sur la validation figée de 80 documents, v7 obtient 36 TP, 7 FP, 4 FN et 33 TN :
précision 83,72 %, rappel 90,00 % et F1 86,75 %, avec L2 = 0,5 et seuil
0,498007. Le résultat est légèrement inférieur au F1 de v4 (87,80 %), mais v7
dispose de davantage de signal conversationnel dans le domaine cible.

Un diagnostic sur l'ancien lot réel déjà consommé fait passer la détection des
trois positifs de 1/3 pour v4 à 2/3 pour v7. Les 40 négatifs de ce lot font déjà
partie de l'apprentissage de v4 et v7 : leurs 40 TN ne constituent donc pas une
mesure de généralisation. Cet essai reste explicitement diagnostique.

Le flux actif a ensuite fourni six documents entièrement nouveaux. Le reçu a été
figé avant le scoring. Un document trop long a été découpé en trois segments :
les deux annotateurs sont unanimement négatifs sur chacun. La nouvelle agrégation
conservative n'accorde un label parent que si tous les segments terminés portent
le même consensus ; elle laisse les cas mixtes ou incertains non résolus. L'accord
local couvre ainsi quatre documents, tous négatifs ; deux restent en abstention.
Le modèle déployé et v4 classent correctement les quatre consensus, tandis que
v7 produit un faux positif. Ce lot ne contient
aucun positif consensuel et ne mesure donc ni le rappel ni la qualification.

Conclusion : v7 n'est pas promu. Il apporte un signal encourageant sur deux des
trois positifs réels antérieurs, mais sa validation n'améliore pas v4 et le lot
vierge disponible est trop petit et sans positif. La prochaine décision exige un
nouveau lot réel gelé comprenant suffisamment de consensus positifs et négatifs.
Les artefacts sont dans `candidate-v7/` et `real-world-v4/`.

## Ordonnancement de la collecte active

La collecte générale reste indépendante du classifieur : aucune URL n'est
admise ou rejetée d'après une prédiction NLP. Le crawler maintient désormais un
index Redis dérivé pour servir d'abord les URL contenant une expression CTI
forte (`initial-access`, `access-broker`, `ransomware`, etc.). Les termes
ambigus (`access`, `leak`, `database`, etc.) ne sont prioritaires qu'avec un
second signal CTI. L'index est reconstruit au démarrage afin de couvrir aussi
le stock antérieur ; la file générale demeure la source de vérité et assure le
repli lorsque l'index est vide. Le scraper applique le même ordre et borne ses
cycles à 300 URL par défaut (`SCRAPER_BATCH_SIZE`).

Le premier passage a vidé les 26 URL fortement signalées qui se trouvaient
dans le stock d'environ 90 000 URL. Elles étaient principalement des alias de
fermes ou des services indisponibles : ce premier contrôle n'avait produit aucun
nouveau document exploitable dans MongoDB. Le filtre de contenu a continué à
rejeter les pages bloquées avant persistance. À ce contrôle, MongoDB contenait
88 entrées, dont 85 avec contenu. Ce résultat confirme que la priorité
d'URL réduit le délai de vérification, mais ne remplace pas l'audit de contenu
ni la recherche de nouvelles sources positives. Aucun candidat n'est promu.

## Lot réel v5 après reprise du flux

La poursuite du scraper a ensuite persisté quatre documents avec contenu après
`ObjectId 6ad66f69874c8c5c25ac4111`. Ils ont été exportés dans un nouveau lot,
annotés sans score NLP, puis figés avec les empreintes du lot, du consensus et
des trois modèles avant tout calcul. Deux pages de 9 et 20 caractères restent
en abstention. Les deux textes substantiels donnent un consensus positif pour
une offre francophone de services de compromission et un consensus négatif pour
une boutique de cannabis.

Les modèles déployé, v4 et v7 donnent exactement le même résultat sur ces deux
consensus : 0 TP, 0 FP, 1 FN et 1 TN. Les trois ratent donc le nouveau positif
conversationnel/commercial. Ce lot minuscule ne qualifie aucun modèle, mais il
confirme directement la lacune déjà identifiée sur les positifs du style cible.
Il reste réservé au diagnostic et n'est ajouté ni à l'apprentissage ni au réglage
du seuil. Les artefacts et le reçu antérieur au scoring sont dans
`real-world-v5/`. Aucun candidat n'est promu.

## Extension ransomware.live et candidat v8

Le collecteur a conservé 432 descriptions substantielles d'octobre 2026. Deux
lots de développement totalisant 297 documents ont produit 217 consensus : 18
positifs et 199 négatifs. Le candidat v8 ajoute à v7 ces 18 positifs et 36
négatifs difficiles sélectionnés avant réentraînement. Il conserve 87,80 % de
F1 sur la validation interne historique, qui n'est pas un nouveau test.

Les 135 documents restants ont été gelés avant annotation et avant scoring. Les
deux annotateurs locaux ne s'accordent que sur 49 documents (36,3 %) : 2
positifs et 47 négatifs, avec 45 abstentions et 41 désaccords. Le lot est très
concentré (129/135 documents viennent de deux groupes) et les accords automatiques
ne constituent pas une vérité humaine ; les résultats sont donc diagnostiques.

| Modèle | Précision | Rappel | F1 | TP/FP/FN/TN |
|---|---:|---:|---:|---:|
| Déployé | 0 % | 0 % | 0 % | 0/0/2/47 |
| v4 | 0 % | 0 % | 0 % | 0/6/2/41 |
| v7 | 4,17 % | 50 % | 7,69 % | 1/23/1/24 |
| v8 | 16,67 % | 50 % | 25,00 % | 1/5/1/42 |

v8 réduit fortement les faux positifs de v7 sur ce contrôle (5 contre 23) mais
ne détecte qu'un des deux positifs consensuels. Le faible nombre de positifs, la
faible couverture de consensus et la concentration par groupe excluent toute
qualification. Aucun modèle n'est promu et ce contrôle est désormais consommé.
Artefacts : `../ransomware-live-2026-10-19/holdout-score-receipt.json` et
`../ransomware-live-2026-10-19/holdout-diagnostic-result.json`.

## Candidat v9 et contrôle automatique de septembre

Une expérience v9 a ajouté à v7 les **217 consensus automatiques** des deux lots
de développement ransomware.live : 18 positifs et 199 négatifs. Elle vise à
mesurer l'effet d'une forte quantité de négatifs du même style. Sur la validation
interne historique, v9 conserve 87,80 % de F1. Cette mesure n'est pas une
qualification nouvelle.

Un mois différent a ensuite fourni 610 textes substantiels de septembre 2026,
répartis sur 65 groupes. Aucun chevauchement exact ou par shingles n'a été trouvé
avec les 421 entrées brutes de v9. Avant annotation, 200 documents ont été gelés
par tour de rôle déterministe entre groupes, avec un maximum de cinq documents
par groupe, ainsi que les empreintes des quatre modèles.

Les annotateurs automatiques produisent 156 consensus (17 positifs, 139 négatifs),
25 abstentions et 19 désaccords. Il s'agit d'une autorité automatique, pas d'une
vérité humaine.

| Modèle | Précision | Rappel | F1 | TP/FP/FN/TN |
|---|---:|---:|---:|---:|
| Déployé | 50,00 % | 5,88 % | 10,53 % | 1/1/16/138 |
| v4 | 20,00 % | 58,82 % | 29,85 % | 10/40/7/99 |
| v8 | 48,39 % | 88,24 % | **62,50 %** | 15/16/2/123 |
| v9 | **71,43 %** | 29,41 % | 41,67 % | 5/2/12/137 |

L'ajout de tous les négatifs rend v9 trop conservateur : il réduit les faux
positifs à deux, mais perd dix vrais positifs par rapport à v8. v8 fournit le
meilleur compromis sur ce lot, tout en restant juste sous les objectifs de 50 %
de précision et 90 % de rappel. Aucun candidat n'est promu. La prochaine
expérience devra préserver le ratio de v8, enrichir les positifs diversifiés et
évaluer une représentation plus robuste, sans régler quoi que ce soit sur ce lot
désormais consommé.
