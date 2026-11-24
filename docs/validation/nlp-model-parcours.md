# Parcours du modèle NLP

Ce document donne une vue d'ensemble du travail réalisé pour construire,
évaluer et améliorer le classifieur NLP de BlueWhiteThreat. Les rapports datés
du même répertoire conservent les détails techniques et les résultats complets.

## 1. Objectif

Le modèle doit reconnaître les textes utiles à la veille sur les menaces :
compromissions, ventes d'accès, fuites de données, malware, ransomware et autres
signaux CTI. Il doit distinguer ces textes du bruit rencontré dans la collecte :
boutiques généralistes, contenus sans rapport, documentation et pages trop
pauvres.

## 2. Constitution des données

Les données ont été collectées depuis plusieurs familles de sources :

- flux réel de l'application ;
- questions et discussions publiques ;
- rapports CERT-EU et CERT-FR ;
- catalogue CISA KEV ;
- avis de sécurité GitHub ;
- corpus de forums publié sur Zenodo ;
- observations de sites cibles ;
- revendications ransomware.live, ajoutées comme réserve non annotée.

Chaque document conserve sa provenance et son groupe. Les contenus trop courts,
dupliqués ou bloqués sont écartés. La présence dans une source n'est jamais
transformée automatiquement en étiquette d'entraînement.

## 3. Annotation

Deux modèles locaux annotent indépendamment les documents selon une politique
CTI versionnée. Un document entre dans un corpus étiqueté uniquement lorsqu'ils
sont d'accord. Les désaccords et incertitudes restent des abstentions. Les textes
trop longs sont découpés ; le parent reçoit un consensus seulement si tous ses
segments terminés portent la même classe.

Cette annotation constitue une référence exploratoire. Une revue humaine reste
nécessaire pour les frontières difficiles et avant une qualification de
production.

## 4. Préparation du corpus

La préparation applique :

1. normalisation du texte ;
2. suppression des doublons exacts ;
3. regroupement des contenus proches par shingles ;
4. conservation d'un même événement ou parent dans un seul split ;
5. plafonds par source pour éviter qu'une source domine ;
6. contrôle des quatre cellules classe × style : positifs et négatifs,
   conversationnels et officiels.

Le principal manque actuel concerne les **positifs conversationnels et
commerciaux proches du flux réel**, notamment en français. Les négatifs de style
officiel doivent également être renforcés.

## 5. Entraînement

Le classifieur utilise une représentation TF-IDF et une régression logistique.
Les expériences comparent de manière bornée le vocabulaire, la régularisation et
la pondération de la classe positive. Une partie distincte sert à la calibration
du score. Le seuil n'est jamais changé après lecture d'un test final.

Les versions principales ont suivi cette progression :

- modèle déployé historique : rappel réel très faible ;
- v3 : rappel élevé mais beaucoup trop de faux positifs ;
- v4 : meilleur compromis sur la validation interne ;
- v7 : ajout de discussions de forums, utile sur quelques positifs réels mais
  légèrement inférieur à v4 sur la validation figée.

## 6. Évaluation

Les validations sont séparées de l'entraînement et groupées pour limiter les
fuites entre sources ou événements. Avant chaque test réel, un reçu enregistre
les empreintes du lot, des annotations et des modèles. Les scores sont calculés
seulement après ce gel.

Les mesures suivies sont la précision, le rappel, le F1 et la matrice
TP/FP/FN/TN. Les résultats par source et par style sont examinés en plus du score
global, car un bon résultat moyen peut masquer une dépendance au format de la
source.

## 7. Résultats actuels

- v4 obtient environ **87,8 % de F1** sur sa validation figée.
- v7 obtient environ **86,8 % de F1**, avec 90 % de rappel sur cette validation.
- v7 détecte deux des trois anciens positifs réels, contre un pour v4, mais ce
  contrôle est seulement diagnostique.
- Sur le lot réel v5, les trois modèles ratent une offre francophone de services
  de compromission et reconnaissent correctement un négatif.

Aucun candidat n'est promu : les tests réels restent trop petits et la
généralisation vers le style cible n'est pas encore suffisante.

## 8. Collecte active

Le crawler et le scraper donnent la priorité aux expressions CTI fortes. Les
termes ambigus demandent un second signal. Cette priorité change uniquement
l'ordre de traitement : elle ne filtre pas les données selon le modèle. Les
contrôles de contenu et de fermes restent appliqués avant persistance.

ransomware.live apporte une réserve importante de revendications réelles. Une
première collecte mensuelle a conservé **432 textes substantiels provenant de
56 groupes**, avec une longueur médiane de 403 caractères. Les descriptions
substantielles sont conservées sans leur injecter automatiquement
une classe. Les mentions trop courtes sont rejetées et les métadonnées de groupe,
pays ou secteur ne sont pas ajoutées au texte, afin de réduire les raccourcis de
source. Cette réserve devra être annotée, dédupliquée et équilibrée avec des
négatifs comparables avant entraînement.

Deux lots de développement couvrant 297 documents ont ensuite été annotés. Ils
fournissent 217 consensus : 18 positifs et 199 négatifs. Cette distribution
montre qu'une revendication agrégée contient souvent seulement une présentation
de la victime et ne doit pas être considérée positive par provenance. Un
candidat v8 exploratoire ajoute les 18 positifs et 36 négatifs difficiles à v7.
Il conserve 87,8 % de F1 sur l'ancienne validation interne. Les 135 documents
restants ont été gelés séparément avant annotation comme contrôle indépendant ;
ils ne participent pas à l'entraînement de v8. L'annotation donne seulement 49
accords (2 positifs, 47 négatifs), 45 abstentions et 41 désaccords. Sur ces
accords, v8 obtient 1 TP, 5 FP, 1 FN et 42 TN (F1 25 %), contre 1/23/1/24 pour
v7. v8 réduit donc les faux positifs, mais ce contrôle trop petit, concentré et
faiblement consensuel reste diagnostique. Il est consommé et aucun modèle n'est
promu.

## 9. Prochaines étapes

1. Étendre l’annotation automatique consensuelle à plusieurs sources et styles.
2. Collecter un nouveau test réparti entre groupes, langues et styles.
3. Constituer davantage de positifs conversationnels francophones et anglophones.
4. Compléter les négatifs de style officiel et commercial.
5. Conserver séparément les abstentions et désaccords automatiques.
6. Entraîner un nouveau candidat sans réutiliser les tests consommés.
7. Le tester sur un nouveau lot réel, figé avant scoring, avec assez de positifs
   et de négatifs dans chaque strate critique.

## 10. Documents de référence

- `nlp-retraining-2026-10-19.md` : réentraînement et comparaisons initiales ;
- `nlp-automatic-v2-2026-10-19.md` : protocole d'annotation automatique ;
- `nlp-source-audit-2026-10-19.md` : audit des sources ;
- `nlp-balanced-audit-2026-10-19.md` : audit du corpus équilibré ;
- `nlp-balanced-training-2026-10-19.md` : expériences, tests réels et verdicts.

## 11. Expérience sans revue humaine : v9

Pour mesurer l'effet des négatifs difficiles, v9 reprend v7 et ajoute tous les
217 accords automatiques issus des 297 documents de développement
ransomware.live : 18 positifs et 199 négatifs. Sur un nouveau lot de septembre,
gelé avant annotation et scoring, les annotateurs automatiques trouvent 17
positifs et 139 négatifs consensuels.

v8 obtient le meilleur F1 (62,5 %) avec 15 TP, 16 FP, 2 FN et 123 TN. v9 devient
trop conservateur : 5 TP, 2 FP, 12 FN et 137 TN, soit 41,7 % de F1. L'expérience
montre que l'ajout massif de négatifs améliore la précision mais détruit le
rappel. La suite conservera donc un échantillonnage équilibré et cherchera surtout
des positifs variés. Aucun modèle n'est promu et le lot de septembre est consommé.

## 12. Sources élargies et nouvelles représentations

Un audit multi-source est maintenant consigné dans
`nlp-source-expansion-plan-2026-10-19.md`. Une première réserve de 1 200 avis
GitHub a été collectée sans étiquette de provenance. Elle complète les 432 textes
ransomware.live d'octobre, les 610 de septembre et 699 nouveaux textes d'août.

Le modèle TF-IDF utilise déjà mots, bigrammes et 4-grammes de caractères. Une
validation croisée sur les 217 accords automatiques de développement sélectionne
la variante `stable_terms_word_reserve`, un multiplicateur positif de 1,5 et
L2=0,001. Son estimation de développement est de 60 % de précision, 100 % de
rappel et 75 % de F1.

Un encodeur MiniLM multilingue figé (révision enregistrée, 384 dimensions) a été
testé avec une simple régression logistique comme seule tête entraînée. Son
meilleur résultat hors pli est de 61,5 % de précision, 44,4 % de rappel et 51,6 %
de F1. Cette première variante sémantique est donc nettement trop faible en
rappel et n'est pas destinée à la production.

Un nouveau contrôle d'août de 200 documents et 69 groupes a été gelé avant
annotation. Aucun chevauchement exact ou par shingles avec les données
d'entraînement n'a été détecté. L’annotation automatique donne 169 consensus
(28 positifs et 141 négatifs), soit 84,5 % de couverture.

Sur ces consensus, la variante TF-IDF figée obtient **73,0 % de précision,
96,4 % de rappel et 83,1 % de F1** (27 TP, 10 FP, 1 FN, 131 TN). La tête
MiniLM figée obtient **85,7 % de précision, 64,3 % de rappel et 73,5 % de F1**
(18 TP, 3 FP, 10 FN, 138 TN). Le TF-IDF franchit donc les seuils ponctuels,
mais le lot ne contient que 28 positifs consensuels, sous le minimum de 50 du
protocole principal ; la borne basse Wilson du rappel est 82,3 %. Le test est
consommé, aucun paramètre n’est ajusté et aucune promotion n’est effectuée.

## 13. Boucle multi-source et modèle promu du 20 octobre

Trois itérations automatiques ont ensuite appliqué la même discipline : figer le
lot et le candidat, obtenir deux annotations locales indépendantes, mesurer sans
retoucher le seuil, puis intégrer seulement les tests consommés à l’itération
suivante. La politique v4 impose un choix binaire et une preuve textuelle à chaque
annotateur, tout en conservant leurs désaccords hors des références.

L’itération 1 a révélé une couverture insuffisante et un fort déficit sur les
avis techniques. L’itération 2 a corrigé la couverture et atteint 86,0 % de
rappel global, mais seulement 75,0 % sur ransomware.live. Après intégration de
ces erreurs consommées, l’itération 3 dispose de 1 051 consensus de développement
et est évaluée sur un nouveau lot de 400 documents temporellement et logiquement
disjoint.

Le test final fournit 331 consensus, 79 positifs et 252 négatifs. Le candidat
obtient 75,8 % de précision, 91,1 % de rappel et 82,8 % de F1. Les seuils globaux
prédéfinis passent, ainsi que les tests logiciels. Ce candidat devient le
`model.json` local courant, avec l’empreinte SHA-256
`1fd5dd5e5531c5b2b80586ae6b1bb23b0f0daa105a232d7f2c76c03fccc36458`.
Le rapport détaillé et les limites par source figurent dans
`nlp-iterative-audit-2026-10-20.md` et `model.json.evaluation.json`.

## 14. Audit du candidat prospectif du 22 octobre

Un nouveau flux réel de 1 396 documents a été séparé avant annotation en 1 096
documents de développement et une réserve temporelle scellée de 300 documents
(SHA-256 `bb7537350df5eeb22971c2c60b99cf3ffb87269747868c947c71d89b5d6a6b86`).
L'annotation locale à trois modèles a produit 1 076 consensus, dont 198 positifs,
soit 98,18 % de couverture. Les modèles déjà figés restent trop imprécis sur ce
flux : leur F1 est compris entre 53,27 % et 54,38 %. Aucun n'est promu.

L'audit du candidat suivant a trouvé une anomalie d'implémentation déterminante :
la boucle calculant les prédictions hors pli de la tête partagée était
accidentellement conditionnée à l'activation de la tête darkweb expérimentale.
Avec cette option désactivée, les 3 181 exemples représentatifs GitHub et
ransomware.live conservaient un score initial nul ; seul le sous-ensemble
darkweb, remplacé ensuite par sa tête dédiée, recevait de vrais scores. Le seuil
zéro, le rappel de 100 % et les métriques de l'itération 25 sont donc invalides
et ne décrivent pas les performances du modèle.

La condition a été corrigée afin que les cinq plis de la tête partagée soient
toujours entraînés. La tête darkweb reste optionnelle. Toute la suite Go du
classifieur passe après correction. L'itération 25 est rejetée, `model.json`
n'est pas remplacé et la réserve de 300 documents demeure scellée. La prochaine
étape est de relancer l'entraînement sous un nouveau numéro d'itération, puis de
contrôler la distribution des scores hors pli avant toute lecture de la réserve.

### Résultat corrigé : itération 26

Après correction, les cinq plis produisent des scores non nuls et variables pour
les trois sources. Le candidat sélectionné utilise la configuration
`darkweb_error_indicators`, L2=0,001 et un poids positif de 1. Sur les 4 257
documents représentatifs hors pli, son seuil global de 0,3271 donne 50,04 % de
précision, 96,46 % de rappel et 65,90 % de F1. La précision moyenne ordonnée
(average precision) atteint 77,18 %.

La politique expérimentale par source améliore le résultat global à 59,89 % de
précision, 90,44 % de rappel et 72,06 % de F1. GitHub et ransomware.live passent
les objectifs par source, mais darkweb reste à 39,22 % de précision pour 90,10 %
de rappel. Le critère exigeant que chaque source atteigne 50 % de précision
échoue donc. Le candidat n'est pas promu et la réserve prospective demeure
scellée. La prochaine boucle doit cibler les faux positifs darkweb sans ajuster
le modèle sur cette réserve.

### Audit d'erreurs après l'itération 26

L'échec darkweb ne vient pas seulement du classifieur. La tête spécialisée
sépare très peu les classes : ses scores hors pli restent entre 0,455 et 0,545,
et les médianes des vrais et faux positifs sont proches. Ses coefficients les
plus positifs portent sur `hack`, `hacking`, `cards`, `account`, `credit` et
`bitcoin`. Elle apprend donc surtout le vocabulaire générique des marchés
illicites plutôt que les preuves de fuite, d'accès initial ou de rançongiciel.

Les 437 faux positifs sont majoritairement des boutiques de cartes prépayées,
de contrefaçon, de cryptomonnaies, de paris ou des annuaires darkweb. Ils sont
souvent longs (médiane 3 372 caractères), ce qui multiplie les signaux lexicaux
faibles. Plus important, des pages quasiment identiques de cartes et de fraude
reçoivent parfois des consensus opposés : un regroupement automatique par les
200 premiers caractères normalisés trouve 25 documents dans six groupes ayant
des labels contradictoires. Certains modèles apprennent alors comme positifs les
mêmes patrons que l'évaluation compte comme négatifs.

Enfin, les plis sont groupés par hôte, mais les clones de boutiques changent
fréquemment d'adresse. Des doublons exacts et des variantes de même template se
retrouvent donc dans plusieurs plis. L'estimation actuelle peut rester optimiste
malgré la séparation par hôte.

Avant une itération 27, le plan retenu est : créer des composantes de
quasi-doublons sur l'ensemble du développement, placer chaque composante dans un
seul pli, exclure de la sélection les composantes dont les consensus se
contredisent, et renforcer automatiquement la règle d'annotation distinguant
une boutique criminelle générique d'une preuve explicite de compromission. Le
corpus contradictoire ne doit pas être arbitrairement réétiqueté. La réserve
scellée ne doit toujours pas être consultée pendant ces corrections.

### Audit de la validation après l'itération 27

Contrôle des artefacts : 4 279 documents retenus, répartis dans 2 739 composantes
provenance/contenu ; aucune de ces composantes n'est divisée entre plusieurs
plis. Les 16 exclusions contradictoires sont distinctes des 27 abstentions ou
désaccords d'annotation. La sélection représentative porte sur 4 241 documents,
les 38 documents d'enrichissement ciblé restant hors sélection.

Limites à garder explicites :

- Les résultats sont des estimations de développement, pas un test indépendant.
  Les seuils et 32 configurations sont sélectionnés sur ces mêmes prédictions
  hors pli ; leur maximum comporte donc un biais de sélection.
- Exclure les composantes contradictoires améliore la cohérence, mais réduit la
  couverture des cas difficiles. Une hausse future des métriques devra être
  distinguée d'une simple suppression d'exemples ambigus.
- Une composante regroupe des textes reliés par similarité transitive : le plus
  grand groupe a 28 documents et une similarité minimale entre extrémités de
  0,671. Cela protège les plis mais n'établit pas l'identité sémantique de tous
  leurs membres. Un préfixe commun seul n'établit pas non plus un conflit réel
  d'annotation ; les 25 cas signalés à l'audit précédent sont des suspects,
  pas 25 erreurs de label confirmées.
- Les nouveaux indicateurs de marchés sont des caractéristiques apprises, pas
  des règles imposant le label négatif. Une offre de données volées dans une
  boutique reste un signal positif selon la politique actuelle.
- La consigne v6 accepte des services malveillants et des identifiants volés :
  déclarer toutes les boutiques criminelles négatives changerait la définition
  de la tâche. Aucun changement de politique ni réétiquetage automatique n'a
  été appliqué. La réannotation promise reste à réaliser seulement après avoir
  explicité ce périmètre, sans confondre désaccord et erreur.

Décision : conserver le candidat 27 hors production ; terminer l'ablation 28
(sans tête spécialisée) avant toute autre modification. Les fichiers de code
et de données utilisés par ce processus actif restent figés pendant son
exécution. La réserve indépendante n'est pas utilisée pour cet audit.

### Résultat de l'ablation 28 — tête partagée seule

Les cinq plis sont terminés. À données et groupes identiques à l'itération 27,
retirer la tête spécialisée darkweb fait passer les faux positifs darkweb de
453 à 391, sans changer les 276 vrais positifs ni les 30 faux négatifs. La
précision darkweb progresse de 37,86 % à 41,38 %, le rappel reste à 90,20 % et
le F1 passe de 53,33 % à 56,73 %. La tête spécialisée est donc défavorable dans
cette comparaison de développement.

La politique par source atteint globalement 60,67 % de précision, 90,16 % de
rappel et 72,53 % de F1. GitHub reste à 70,41 % / 90,15 % et ransomware.live
à 70,95 % / 90,14 % (précision / rappel). Le seuil global seul donne 76,23 % de
rappel : ces objectifs globaux passent uniquement avec les seuils par source.
Darkweb reste sous le minimum de 50 % de précision ; aucune promotion ni lecture
de la réserve n'est décidée. Ces mesures restent des résultats de développement.

### Audit quantitatif de l'itération 28 — 23 octobre

Un balayage indépendant de tous les seuils observés sur les scores hors pli
confirme que le problème darkweb n'est pas résolu par un seuil différent : le
rappel maximal atteignable avec au moins 50 % de précision est de 83,66 %, sous
l'objectif de 90 %. Il s'agit d'un diagnostic de développement ; les seuils du
candidat restent inchangés. Au seuil actuellement retenu, il faudrait enlever
au moins 115 des 391 faux positifs sans perdre les 276 vrais positifs pour
atteindre 50 % de précision. L'ablation a déjà retiré 62 faux positifs : c'est
une amélioration mesurée, pas encore une résolution.

Les intervalles Wilson nominaux à 95 % donnent pour darkweb 37,70–45,16 % de
précision et 86,35–93,05 % de rappel. Ils ne corrigent ni la sélection des
seuils ni les corrélations entre clones. Les bornes basses du rappel sont aussi
sous 90 % sur GitHub (87,64 %) et ransomware.live (86,60 %). Les seuils par
source ont été choisis pour frôler 90 % sur le développement ; leur marge de
robustesse reste faible. Ces intervalles ne constituent pas une certification
sur des données indépendantes.

Priorités : (1) clarifier le périmètre CTI sans déclarer arbitrairement toutes
les fraudes négatives ; (2) réannoter automatiquement les cas contradictoires
selon une politique versionnée et mesurer la couverture perdue ; (3) comparer
sur les mêmes plis une meilleure représentation ou une régularisation adaptée,
avec tête partagée comme référence ; (4) préenregistrer une marge de rappel et
une évaluation indépendante par composante. Les réglages et la calibration
restent ajustés sur le développement : les probabilités calibrées ne doivent
pas être présentées comme garanties en production.

### Application du plan — clarification v7 et réannotation ciblée

Les 16 documents réellement exclus pour contradiction sont extraits dans un
nouveau lot figé, avec leurs anciens labels sauvegardés séparément. La politique
v7 précise qu'une carte prépayée, un solde, un paiement anonyme ou une offre
commerciale générique ne prouvent pas une compromission. En revanche, une offre
explicitement fondée sur des données volées, des identifiants compromis ou un
service cybermalveillant concret reste positive. Les catégories CTI existantes
ne sont pas supprimées.

Une nouvelle campagne indépendante utilise les trois modèles locaux figés
(Qwen, Gemma et Llama). Elle reçoit les textes complets, sans les anciens labels
ni les scores NLP. Les votes et empreintes sont conservés. Aucun nouveau label
n'est encore fusionné au corpus : il faut d'abord comparer changements,
abstentions et cohérence des composantes. Les cinq tests de la nouvelle version
d'annotation passent. Les scripts historiques et leurs reçus sont conservés.

La comparaison de représentations reste l'étape suivante ; aucun résultat de
modèle supplémentaire n'est annoncé avant la fin de cette campagne. Les
artefacts de campagne sont dans
`apps/classifier/data/iteration-2026-10-23-label-audit/`.

### Corpus stabilisé et comparaison commune — 23 octobre

La réannotation v7 est terminée : 48 votes, 16 décisions majoritaires, dont
3 unanimes et 13 à deux voix contre une ; 7 labels changent par rapport à v6.
Après contrôle exact et par shingles, 8 des 16 documents peuvent être réintégrés
à des groupes cohérents. Deux composantes totalisant 8 documents restent
contradictoires et sont exclues, sans règle de départage arbitraire. Le nouveau
corpus de développement contient 4 287 consensus, contre 4 279 retenus lors de
l'itération 28. L'ancien corpus et les anciens votes ne sont pas écrasés.

Une comparaison de représentations est lancée sur des plis communs regroupant
provenance ET composantes de contenu : mots (1–2 grammes), caractères
(3–5 grammes), leur combinaison et MiniLM multilingue figé avec régression
logistique. Chaque représentation teste trois régularisations préenregistrées.
Le vocabulaire lexical est appris dans chaque pli ; les documents ciblés
peuvent entraîner mais ne sélectionnent pas les seuils. Le plan, ses empreintes,
les plis et les scores hors pli sont conservés dans
`apps/classifier/data/iteration-2026-10-23-representation-comparison/`.

Cette comparaison utilise uniquement le développement, sans base historique :
elles sont comparables entre elles mais leurs chiffres ne doivent pas être
comparés directement à l'itération 28. L'encodeur agrège tous les blocs successifs
de 100 mots ; son tokeniseur peut encore tronquer un bloc très sous-tokenisé.
Aucun modèle n'est promu automatiquement ; la réserve reste scellée. Les tests
de préparation existants passent et trois tests de métriques sont ajoutés.

### Premiers résultats de la comparaison de représentations

Les neuf essais lexicaux sont terminés ; cinq passent les objectifs ponctuels
sur chacune des trois sources. Parmi eux, la combinaison mots/caractères avec
C=10 obtient le meilleur F1 global : précision 71,11 %, rappel 90,13 %, F1
79,49 %, average precision 88,45 %. Darkweb : 60,52 % de précision, 90,29 % de
rappel (279 TP, 182 FP, 30 FN, 1 092 TN). GitHub : 75,09 % / 90,05 % ;
ransomware.live : 75,12 % / 90,14 % (précision / rappel).

La variante caractères seule, C=10, atteint la meilleure précision darkweb
parmi ces essais : 62,14 % à 90,29 % de rappel, mais un F1 global inférieur
(78,02 %). La combinaison reste donc une référence provisoire, pas une sélection
finale avant la fin de MiniLM. Les empreintes du corpus correspondent au plan et
aucune composante de contenu ni aucun groupe de provenance déclaré n'est partagé
entre plis. L'encodage MiniLM est encore en cours. Aucune promotion n'est faite.
Les estimations sont toujours affectées par la sélection sur les scores OOF et
ne prouvent pas la performance sur un flux futur.

### Audit critique des résultats lexicaux — 23 octobre

Le passage des objectifs par document ne suffit pas à conclure à la robustesse.
Un diagnostic donnant le même poids total à chaque composante de provenance et
contenu (au lieu d'un poids égal par document) change le résultat de mots +
caractères C=10 : darkweb 57,91 % de précision / 88,25 % de rappel ;
ransomware.live 73,49 % / 87,86 % ; GitHub 75,09 % / 90,00 %. Les rappels
pondérés par groupe de darkweb et ransomware.live sont sous 90 %. Ce diagnostic
n'était pas un critère de sélection préenregistré : il ne remplace pas les
métriques annoncées, mais montre la sensibilité au poids des sites clonés ou
des acteurs fréquemment observés. Ransomware.live ne représente que 99 groupes
pour plus de mille documents.

Autres limites bloquant une promotion directe :

- Le seuil de chaque source et la régularisation sont choisis sur les mêmes
  scores OOF que les métriques publiées. Une validation imbriquée ou un nouveau
  test indépendant est nécessaire pour une estimation non sélectionnée.
- La clarification v7 n'a été appliquée qu'aux 16 cas contradictoires : le corpus
  mélange des annotations v6 et v7. Cette politique doit être auditée aussi sur
  un échantillon de cas auparavant cohérents, sans sélectionner sur les scores
  du modèle, avant de conclure à une correction générale des labels.
- Le script conserve des scores et les embeddings, mais pas encore un modèle
  final réentraîné/exporté ni une vérification de parité avec l'inférence Go.
  Le TF-IDF scikit-learn diffère du TF-IDF Go : ses performances ne sont pas
  transférables en copiant des poids dans `model.json`.
- Le plan consigne le corpus, les plis et la révision MiniLM, mais pas encore
  l'empreinte du script ni toutes les versions de dépendances. La campagne
  active reste inchangée ; cette traçabilité devra être complétée séparément
  avant un entraînement final.

Décision d'audit : résultats prometteurs de développement, non déployables en
l'état. Terminer MiniLM, figer la représentation retenue, vérifier la stabilité
par groupe et la cohérence d'annotation, puis créer un artefact d'inférence et
son protocole indépendant avant d'ouvrir la réserve. Aucun seuil n'est modifié
à la suite de ce diagnostic.

### Candidat lexical figé et vérifications d'inférence

La comparaison est complète : mots + caractères C=10 reste la référence choisie
parmi les variantes satisfaisant les objectifs par document. Un pipeline Python
isolé est réentraîné sur les 4 287 documents retenus et sauvegardé dans
`apps/classifier/data/iteration-2026-10-23-frozen-lexical/`. Son manifeste conserve
les empreintes du corpus, du plan, des scripts et de l'artefact, ainsi que les
versions Python, scikit-learn, NumPy et joblib. Le chargement vérifie l'empreinte
de l'artefact local de confiance ; une source inconnue donne une abstention,
jamais un seuil arbitraire.

Les prédictions avant sauvegarde, après rechargement et par la CLI sont identiques
à 1e-12 près sur les 4 287 textes. Ce contrôle est une parité Python de
sérialisation/exécution, pas une parité avec le moteur Go. Les scores après
réentraînement sont des scores d'entraînement et ne remplacent pas les métriques
OOF. Six tests unitaires de métriques et de contrat d'inférence passent.

L'audit de stabilité par groupe est enregistré avec 500 rééchantillonnages
conditionnels, et un diagnostic de seuil par pli ; aucun n'est présenté comme
une validation imbriquée ou indépendante. Le critère pondéré par groupe reste
en échec, donc l'artefact est expérimental, non intégré à la production.

Une campagne v7 supplémentaire est lancée sur 24 textes auparavant cohérents :
huit composantes distinctes par source, sélectionnées par ordre de hachage figé,
sans scores NLP et hors des 16 cas déjà réannotés. Les anciens labels sont
cachés aux annotateurs et sauvegardés séparément. Ce contrôle mesure si la
clarification de politique se généralise hors des seules contradictions.
La réserve finale demeure scellée et `model.json` n'est pas remplacé.

### Pont expérimental Go vers le candidat Python

Un adaptateur séparé `internal/experimentallexical` et une commande hors ligne
`experiments/cmd/lexicalbridge` permettent à Go d'appeler le pipeline Python figé via JSONL.
Le processus est borné par un contexte/timeout ; les résultats sont contrôlés
(nombre, ordre et identifiants, score fini entre 0 et 1, cohérence score/seuil/
verdict, abstention pour source inconnue). Les tests couvrent résultat normal,
abstention, identifiant erroné, verdict incohérent, score invalide, JSON invalide,
résultat manquant, panne du sous-processus et annulation. Ils passent.

Ce pont ne reproduit pas TF-IDF en Go : il conserve exactement l'inférence
scikit-learn derrière un contrat Go. Il n'est pas branché au classifieur de
production et recharge Python et le modèle par lot ; ses performances de
latence et de mémoire nécessitent un benchmark avant toute intégration. Un
contrôle de parité sur les 4 287 textes de développement est lancé séparément.

La campagne v7 sur les 24 textes cohérents a subi un délai réseau après 15
votes. Elle reprend avec les reçus existants, au maximum trois tentatives et
sans changement de script ou de politique. Aucun label ni seuil n'est modifié
à cause de cet incident technique.

Le contrôle Go → Python est terminé : 4 287 résultats, aucun écart de score,
aucun désaccord de verdict ou de seuil avec la CLI Python. Le reçu est conservé
sous `iteration-2026-10-23-frozen-lexical/go-bridge-parity.json`. Cela valide le
pont d'appel hors ligne, pas une implémentation native Go ni un test indépendant.

### Audit v7 hors des contradictions : résultat complet

Les 72 votes sur 24 documents sont terminés. Les 24 décisions ont une majorité,
dont 17 unanimes et 7 à deux voix contre une. Trois labels changent, tous dans
les huit avis GitHub : trois anciens négatifs deviennent positifs. Aucun label
ne change parmi les huit textes darkweb et les huit ransomware.live de cet
échantillon. Les nouveaux labels ne sont pas automatiquement appliqués.

Ce contrôle ne montre donc pas d'instabilité générale sur les seuls 16 textes
darkweb/ransomware.live vérifiés, mais l'échantillon reste petit. En revanche,
les 3 changements sur 8 avis GitHub signalent un problème d'annotation à
investiguer avant d'utiliser cette strate comme référence fiable. Il reste à
examiner leurs critères et preuves textuelles, élargir l'audit GitHub si le
signal se confirme, puis figer une nouvelle version cohérente du corpus. Aucun
réentraînement sur la réserve ni promotion ne découle de cet audit.

### Cause des désaccords GitHub et extension de l'audit

L'examen des trois changements retrouve des vulnérabilités concrètes : XXE
http4s-scala-xml, lecture hors limites du parseur AIOHTTP et contournement de
`secure_dir` par lien symbolique dans Smarty. La politique inclut ces cas sans
exiger d'incident avéré. Sur les deux décisions non unanimes, Gemma raisonne
pourtant comme si une vulnérabilité sans incident devait être négative, tandis
que Qwen et Llama appliquent le critère de vulnérabilité concrète. C'est un
signal de mauvaise application de la politique par l'annotateur, pas une règle
justifiant de rendre positifs tous les documents issus de GitHub.

Une extension aveugle de 32 composants GitHub distincts est préparée sous
`iteration-2026-10-23-github-policy-audit`. La sélection est déterministe par
hash, sans score NLP ni label et exclut les composants du précédent audit.
Les anciens labels sont conservés à part, hors de l'entrée annotateur. Le
script v7 reste figé ; 96 votes sont attendus. Aucun overlay automatique,
réentraînement, changement de seuil ou accès à la réserve n'est prévu.

### Audit GitHub élargi : résultat du 24 octobre

Les 96 votes sur 32 nouveaux documents sont terminés : 16 décisions unanimes
et 16 à deux voix contre une. La majorité v7 change 16 anciens négatifs en
positifs ; les comptes passent de 11 positifs / 21 négatifs à 27 / 5. Les
écarts à la majorité sont de 10 pour Gemma, 4 pour Qwen et 2 pour Llama.
Le reçu détaillé est `iteration-2026-10-23-github-policy-audit/comparison-summary.json`.

Le bruit d'annotation GitHub est donc confirmé au-delà du premier petit
échantillon. Ces majorités automatiques ne constituent toutefois pas une
vérité indépendante, et la moitié des décisions restent non unanimes.
Les résultats antérieurs sur cette strate nécessitent une revalidation ;
aucun gain de modèle n'est déduit de ces changements de référence. Avant un
réentraînement, la prochaine étape est de calibrer l'application des critères
sur des cas positifs et négatifs explicites, puis de versionner les annotations
corrigées sans écraser les anciennes. Aucun overlay n'est appliqué ici,
aucun seuil ni modèle de production n'est changé, et la réserve reste scellée.

### Calibration contrôlée des consignes v7 et v8

Une version séparée `experiments/annotations/annotation_v8.py` précise qu'une vulnérabilité concrète
reste pertinente sans incident observé, mais qu'un bug générique, un titre
CVE sans explication ou une simple mention de sécurité ne suffisent pas.
Le script v7 et ses campagnes restent intacts. Les cinq tests structurels
v7 et les cinq tests v8 passent ; ce ne sont pas des tests de qualité LLM.

Une comparaison v7/v8 est lancée sur 12 cas synthétiques équilibrés (6 positifs,
6 négatifs), avec labels attendus sauvegardés hors de l'entrée annotateur,
sous `iteration-2026-10-24-annotation-calibration`. Les mêmes trois modèles
votent sous chaque consigne, soit 72 votes attendus. Le critère préfixé est
12 majorités correctes sur 12, avec erreurs par modèle également rapportées.
Ces exemples contrôlent la compréhension de la politique ; ils ne mesurent
pas la performance sur des sources réelles et ne justifient pas de promotion.
Même un succès devra être suivi d'une vérification sur de nouveaux documents
réels, sans consultation de la réserve. Aucun overlay de labels n'est appliqué.

### Résultat calibration et contrôle sur textes réels

La comparaison contrôlée est terminée (72 votes). La majorité v7 obtient
10/12 ; elle manque les cas de contournement par symlink et d'accès aux données
d'un autre tenant. Ses erreurs individuelles : Gemma 4, Llama 2, Qwen 1.
La v8 obtient 12/12 en majorité et chaque annotateur obtient également 12/12.
Le reçu est `iteration-2026-10-24-annotation-calibration/comparison-result.json`.

Ce succès mesure la compréhension de 12 exemples synthétiques conçus pour
la politique, pas une généralisation indépendante ni une performance NLP.
La v8 reste figée pour la suite. Un contrôle de cohérence sur 24 nouveaux
textes réels (8 par source) est lancé sous
`iteration-2026-10-24-v8-real-source-audit`, soit 72 votes supplémentaires.
La sélection par hash exclut les composants des audits précédents et ceux
des contradictions initiales ; labels et scores sont absents des entrées.
Ce contrôle doit rapporter accords, changements et preuves, sans transformer
la majorité automatique en vérité indépendante. Aucun overlay automatique,
modèle de production modifié ni ouverture de la réserve.

### Contrôle v8 sur 24 nouveaux textes : terminé

Les 72 votes sont complets : 22 décisions unanimes et 2 à deux voix contre
une. Par source : darkweb 7/8 unanimes, aucun changement ; GitHub 8/8 unanimes,
3 anciens négatifs deviennent positifs ; ransomware.live 7/8 unanimes,
un ancien positif devient négatif. Les huit textes GitHub de cet échantillon
sont positifs et les huit ransomware.live négatifs : ce petit échantillon
n'est donc pas une mesure équilibrée de performance par source.

La v8 montre un meilleur accord apparent que dans les audits précédents,
mais les documents sont différents : aucun gain causal v7→v8 n'est établi
sur textes réels. Les majorités restent des annotations automatiques, pas
une vérité indépendante. Le reçu est conservé dans
`iteration-2026-10-24-v8-real-source-audit/comparison-summary.json`.
Prochaine étape : comparer les deux consignes sur les mêmes textes réels
et examiner les preuves des changements avant de préparer un corpus versionné.
Aucun overlay ni changement du modèle de production n'a été appliqué.

### Comparaison appariée et limite des majorités

Une campagne témoin v7 est lancée sur les mêmes 24 textes que la v8, sans
modifier les deux scripts figés. Cela permet une comparaison appariée des
votes et critères, mais ne fournit toujours pas de vérité indépendante.

L'examen des quatre changements v8 est conservé dans
`changed-label-evidence-review.json`. Les trois avis GitHub décrivent bien
un mécanisme et un impact de vulnérabilité. En revanche, le texte
`ransomware-live:2b655367e71aaac50d8592d1` décrit des dossiers personnels et
porte la mention « NOT PAID — EXPOSED ». Qwen y reconnaît une exposition de
données ; les deux autres annotateurs ne retiennent aucun critère positif.
La décision majoritaire négative est donc suspecte au regard de la politique
et reste en désaccord, sans overlay. Le bon accord global ne suffit pas à
valider les labels ; ce cas doit entrer dans une future vérification ciblée,
avec contrôles négatifs, sans retoucher la v8 pendant sa comparaison.

### Comparaison appariée v7/v8 : résultat

Les deux campagnes sur les mêmes 24 textes sont terminées (144 votes).
L'unanimité passe de 17/24 sous v7 à 22/24 sous v8. Une seule décision
majoritaire change entre les consignes : `ghsa:GHSA-frj6-5rhh-vwfw`, négative
sous v7 et positive sous v8. Le reçu est `paired-comparison.json`.
Cela établit un meilleur accord sur cet échantillon, pas une amélioration
mesurée d'exactitude. Le cas ransomware.live précédemment signalé reste une
limite connue. Aucun label ni modèle de production n'est remplacé.

### Diagnostic ciblé exposition/extorsion

Le seul changement majoritaire v7→v8 sur les textes appariés concerne
Nodemailer : désactivation de la vérification TLS permettant l'interception
de secrets et tokens OAuth. Le positif v8 est cohérent avec le mécanisme
explicite ; le statut « duplicate/withdrawn » n'annule pas la description.

Pour comprendre le faux négatif potentiel « NOT PAID — EXPOSED », un diagnostic
v8 inchangée est lancé sur 8 cas synthétiques : revendications courtes et
explicites d'exposition, extorsion et vente de données, avec contrôles négatifs
sur factures impayées, prestations d'enquête et navigation. Les réponses
attendues sont séparées de l'entrée annotateur. Ce test est conçu après une
erreur observée : c'est un diagnostic, pas une validation indépendante.
Les reçus sont sous `iteration-2026-10-24-exposure-calibration` ; aucun label
réel ni seuil n'est modifié pendant ce contrôle.

### Diagnostic exposition : échec ciblé confirmé

Les 24 votes sur 8 cas sont terminés. La majorité obtient 7/8 : le cas court
contenant des données personnelles et « NOT PAID — EXPOSED » reste négatif.
Gemma et Llama ignorent le signal d'exposition, Qwen le reconnaît. Les versions
plus explicites d'exfiltration/extorsion et les quatre contrôles négatifs sont
correctement traités. Le critère fixé (8/8) échoue ; reçu :
`iteration-2026-10-24-exposure-calibration/comparison-result.json`.

La v8 n'est donc pas validée pour une réannotation générale. Le problème est
localisé à l'interprétation des revendications abrégées, pas à une confusion
générale entre toute facture impayée et une fuite. La suite recommandée est
une nouvelle consigne versionnée qui explicite ce contraste, puis une
régression couvrant tous les cas contrôlés et des textes réels non vus.
Aucun label réel, seuil ou modèle de production n'est modifié à ce stade.

### Consigne v9 et régression complète des cas contrôlés

Une version séparée v9 précise les revendications abrégées d'exposition :
une description de dossiers non publics accompagnée de « NOT PAID — EXPOSED »
constitue une revendication de publication, sans devoir préciser le piratage.
Elle exclut explicitement les factures seules, prestations ordinaires,
mentions de navigation et citations niant une exposition. Les v7/v8 restent
inchangées. Les 15 tests structurels des trois versions passent.

La v9 est lancée sur les 20 cas contrôlés cumulés (60 votes attendus), avec
critère fixé à 20/20 majorités correctes et rapport des erreurs par modèle.
Le lot comprend le cas ayant motivé la clarification : c'est une régression
de développement, pas une validation indépendante. Reçus sous
`iteration-2026-10-24-v9-regression`. Aucun overlay, entraînement ou changement
de production n'est effectué sur la base de ce lancement.

### Régression v9 : correction locale, échec global

Les 60 votes sont terminés : majorité correcte sur 19/20 cas. La revendication
abrégée d'exposition est corrigée, mais le cas de contrôle d'accès inter-tenant
`calibration:03` régresse. Gemma commet deux erreurs, Llama une, Qwen aucune
sur ces seuls exemples. Le critère 20/20 échoue : ne pas sélectionner Qwen
comme vérité sur la base de ce petit lot ni valider v9 globalement.

Les changements successifs de consigne montrent une instabilité ; on suspend
les corrections opportunistes du prompt. Une politique de stabilité est figée
avant le prochain contrôle : un label n'est stable que si les trois modèles
sont unanimes sous v8 ET v9, sur la même classe. Sinon abstention, jamais
négatif par défaut. Ce filtre mesure une cohérence, pas une vérité ; il peut
sur-sélectionner les cas faciles, ce qui devra être mesuré par source et classe.

La v9 est lancée sur les mêmes 24 textes réels déjà annotés v8 (72 votes),
avec protocole `v8-v9-stability-protocol.json` sauvegardé avant les résultats.
Les v8/v9 restent figées, aucun overlay automatique ni réentraînement.

### Résultat du filtre de stabilité v8/v9

Les 72 nouveaux votes sont terminés. Sur les 24 mêmes documents, 21 satisfont
l'unanimité des six décisions et 3 restent en abstention. Répartition : darkweb
7/8 stables (7 négatifs), GitHub 8/8 (8 positifs), ransomware.live 6/8
(6 négatifs). Reçu : `v8-v9-stability-result.json`.

La couverture est de 87,5 %, mais aucun positif darkweb ou ransomware.live
n'est conservé dans ce petit lot. Le filtre risque donc d'écarter précisément
les exemples importants et ambigus : il ne suffit pas à produire un corpus
équilibré et ne doit pas devenir un sélecteur automatique d'entraînement.
L'accord n'est pas une preuve d'exactitude. Il faut mesurer ensuite sa
couverture sur un échantillon stratifié par source ET ancien label, hors
réserve, avant toute campagne de réannotation générale. Aucun overlay ni
réentraînement n'est appliqué à partir de ces 21 décisions.

### 25 octobre : contrôle équilibré de la couverture

Un nouveau lot de 24 composants non vus dans les audits précédents est
sélectionné : 4 anciens positifs et 4 anciens négatifs par source. Les anciens
labels servent uniquement à stratifier, pas de vérité de référence, et sont
masqués aux annotateurs. L'ordre de sélection est fixé par hash sans score NLP.
Les v8/v9 inchangées tournent successivement, soit 144 votes attendus.

Le protocole, les empreintes et les reçus sont sous
`iteration-2026-10-25-balanced-stability-audit`. La règle reste l'accord de
six votes, sinon abstention. Le rapport devra montrer la couverture de chacune
des six strates et les changements, sans convertir les abstentions en négatifs
ni prendre ce lot équilibré pour la prévalence réelle. Aucun entraînement,
overlay de labels ou accès à la réserve n'est déclenché par ce lancement.

### Audit équilibré v8/v9 : résultat du 25 octobre

Les 144 votes sont complets. Sur 24 textes (4 anciens positifs et 4 négatifs
par source), seuls 15/24 (62,5 %) ont la même classe dans les six votes v8/v9 ;
9/24 restent abstentions selon le filtre préfixé. Couverture stable par strate :
darkweb 2/4 anciens négatifs et 2/4 positifs ; GitHub 3/4 négatifs (tous trois
changent en positif) et 4/4 positifs ; ransomware.live 3/4 négatifs et 1/4
positifs. Les reçus détaillés sont dans
`iteration-2026-10-25-balanced-stability-audit/comparison-result.json`.

Le filtre ne jette donc pas tous les anciens positifs : il en conserve 7/12
sur cet échantillon, mais la stabilité reste basse et la strate GitHub révèle
encore des changements systématiques. Les anciens labels ne sont pas des
vérités et le lot est petit ; ces chiffres ne donnent ni précision ni rappel.
Décision : ne pas créer un corpus d'entraînement à partir des majorités v8/v9
ni réentraîner maintenant. Geler les consignes, conserver les abstentions et
les reçus, puis établir une référence fiable séparée avant de comparer des
modèles. Réserve scellée, production inchangée.

### Audit de cohérence du signal GitHub Advisory

L'audit porte maintenant sur les 1 213 documents GitHub-Advisories du corpus
de développement. Tous ont un identifiant `ghsa:` ; 663 sont déjà positifs et
550 négatifs. Or la définition de tâche inclut explicitement les vulnérabilités
concrètes. Un balayage lexical trouve des marqueurs de sécurité dans 1 122/1 213,
un mécanisme/impact dans 1 116/1 213 et versions/correctifs/affectations dans
1 015/1 213. Les 91 cas sans les termes de sécurité du balayage incluent
malgré tout des titres et descriptions de vulnérabilités, ce qui montre les
limites des regex ; le compte n'est pas un test d'exactitude.

Une proposition de weak-label séparée a été construite pour les 1 213 entrées :
`iteration-2026-10-25-source-label-audit/proposed-ghsa-overlay.jsonl`.
Elle propose le critère `concrete_vulnerability_or_exploit` positif pour les
avis officiels, ce qui ferait passer les 550 anciens négatifs de cette source
en positifs. C'est cohérent avec le périmètre déclaré mais demeure un label
faible fondé sur la provenance de l'avis, pas une vérité indépendante de
l'impact sur chaque cas. La proposition n'est ni fusionnée au corpus ni utilisée
pour entraîner. Son hash et les comptages sont dans `audit-summary.json`.

Décision expérimentale : arrêter de traiter ces avis comme des négatifs
ordinaires dans une prochaine version de corpus ; comparer explicitement le
scénario weak-label au scénario historique sur le jeu de développement groupé,
mais ne pas annoncer de gain ni promouvoir sans un test externe propre.
Aucun script d'annotation antérieur n'est modifié. Aucun seuil n'est retouché,
aucune réserve consultée, aucun modèle de production remplacé.

### Sensibilité word+char à l'overlay GitHub

Comparaison terminée sur 4 287 documents, plis groupés identiques, TF-IDF
mots+caractères et C=10 fixés. Les 550 changements produisent une AP OOF de
0,8845 sous les labels historiques et 0,9796 sous le weak overlay. Ce gain
apparent est attendu en partie parce que tous les avis GitHub deviennent une
classe unique : précision/rappel par source ne constituent plus une mesure
valide de discrimination de cette source. Au rappel imposé de 0,90, la
précision darkweb descend de 0,605 à 0,548 ; ransomware.live de 0,751 à 0,728.
Les seuils sont choisis sur les mêmes prédictions OOF, donc sélection-biaisés.

Conclusion : l'overlay rend le problème artificiellement plus facile sur la
source GitHub, mais ne démontre aucune amélioration sur des labels vrais et
semble détériorer les diagnostics des deux autres sources. Il n'est pas
fusionné ni promu. Ne pas utiliser l'AP 0,9796 comme résultat de qualité.
Les résultats complets et limites sont dans
`iteration-2026-10-25-source-label-audit/sensitivity-comparison.json`.
Prochaine décision : conserver GHSA comme strate positive/faiblement annotée
séparée, plutôt que contaminer les labels négatifs d'entraînement ; n'évaluer
qu'avec des labels cohérents et un test externe indépendant.

### Généralisation entre sources : audit leave-one-source-out

Audit terminé sur labels historiques, TF-IDF mots+caractères, C=10 et seuil
fixe 0,5, en entraînant sur deux sources et en testant sur la troisième :

- Darkweb (train 2 666, test 1 621) : AP 0,532 ; P 0,499, R 0,566.
- GitHub advisory (train 3 074, test 1 213) : AP 0,735 ; P 0,864, R 0,154.
- ransomware.live (train 2 834, test 1 453) : AP 0,705 ; P 1,000, R 0,082.

Les mauvais rappels en transfert indiquent une forte dépendance au domaine
et/ou des politiques de labels différentes. Ce test utilise les labels
historiques bruyants, donc ce n'est pas une mesure d'exactitude opérationnelle.
Il ne contredit pas les résultats groupés au sein des mêmes sources : la
question évaluée est différente. Détails dans
`iteration-2026-10-25-source-label-audit/leave-one-source-out.json`.

Décision : conserver l'évaluation intra-source pour comparer les classifieurs
quand les labels sont cohérents, mais ajouter un test inter-source comme garde
contre la dépendance à la provenance. Ne pas changer le seuil pour masquer
l'échec de transfert. Étape suivante : harmoniser les définitions/labels entre
sources puis mesurer à nouveau, sur plis groupés figés et test externe séparé.
Pas de promotion ni accès à la réserve.

### Ablation modèle partagé vs un modèle par source

Test terminé sur les mêmes plis groupés intra-source, labels historiques,
TF-IDF mots+caractères, C=10 et seuil fixe 0,5. F1 partagé → séparé :
Darkweb 0,772 → 0,776 (rappel 0,729 → 0,747) ; GitHub 0,829 → 0,820
(rappel 0,857 → 0,805) ; ransomware.live 0,730 → 0,808 (rappel 0,614 → 0,721).
L'AP par source est respectivement 0,840→0,832 ; 0,920→0,918 ; 0,896→0,923.
Détails et compteurs sous `iteration-2026-10-25-source-label-audit/per-source-models.json`.

Cela suggère un bénéfice de spécialisation pour ransomware.live, mais pas une
amélioration universelle ; labels historiques, tests intra-source uniquement,
et pas de vérité indépendante. Décision : garder le modèle partagé comme
référence, conserver le modèle ransomware séparé comme candidat expérimental
à tester sur collecte future et test externe ; ne pas déployer ni régler les
seuils sur ces résultats. Aucune réserve lue.

### Incertitude appariée par groupes : modèle séparé

Sur les prédictions OOF appariées du test précédent, bootstrap 5 000 fois des
groupes de contenu/provenance au sein de chaque source, seuil fixé à 0,5.
Delta F1 (séparé − partagé), IC percentile 95 % conditionnel : Darkweb
+0,004 [-0,020 ; +0,028] ; GitHub -0,010 [-0,024 ; +0,005] ;
ransomware.live +0,077 [+0,028 ; +0,127]. Reçu :
`iteration-2026-10-25-source-label-audit/paired-group-bootstrap.json`.

Le bénéfice ransomware semble persister au rééchantillonnage des groupes
(99 groupes), tandis que les différences darkweb/GitHub sont faibles ou
compatibles avec zéro. Cet intervalle est conditionnel aux prédictions déjà
obtenues : il ne réentraîne pas les modèles et ne couvre donc pas toute
l'incertitude d'entraînement ; les labels restent bruyants. Décision : garder
le candidat spécialisé ransomware pour un futur test prospectif indépendant,
mais aucune promotion sur ce bootstrap seul. Les seuils restent fixes.

### Candidat ransomware spécialisé figé

L'ablation précédente a été refaite et ses prédictions OOF sauvegardées pour
bootstrap. Un pipeline lexical spécifique ransomware.live est maintenant
réentraîné sur les 1 453 documents historiques (355 positifs, 1 098 négatifs),
C=10 et seuil 0,5 inchangés, puis sérialisé dans
`iteration-2026-10-25-source-label-audit/ransomware-specialist-candidate/`.
La parité après rechargement est exacte sur 32 textes. Le manifeste conserve
hashes, versions, hyperparamètres et avertissement sur les labels bruyants.
C'est un artefact hors production, sans intégration.

Un protocole de test prospectif est pré-écrit à côté : au moins 300 documents
futurs, même seuil et mêmes textes pour candidat et référence, métriques et
bootstrap apparié par groupes. Il requiert des labels déterministes/tracés
selon les critères de contenu, pas les votes LLM ni le score du modèle ; si ces
labels manquent, le résultat sera inconclusif. Aucun test n'est encore exécuté
et aucune promotion décidée. La réserve reste scellée.

### Benchmark d'inférence locale du candidat spécialisé

Le candidat sérialisé fait 732 Ko. Sur la machine de développement (CPU, modèle
chaud, sklearn predict_proba, corpus local utilisé comme charge seulement),
latence p95 par lot : 1,07 ms (lot 1), 7,10 ms (16), 24,73 ms (64), 88,83 ms
(256) lors de la première mesure ; débit mesuré environ 1,5–3,0 k documents/s.
RSS maximal corrigé : environ 154 Mo. Une seconde mesure a donné des valeurs
proches, non un benchmark de production. Reçu sous
`ransomware-specialist-candidate/benchmark.json`.

Ce benchmark exclut le démarrage du processus, l'API, la traduction de données,
la charge concurrente et l'environnement de production. Le candidat est donc
techniquement léger pour un essai batch local, mais aucun coût opérationnel
production n'est établi. Pas de modification du runtime ni déploiement.

### Runner hors ligne pour le candidat expérimental

`score_ransomware_candidate.py` vérifie le hash de l'artefact et le manifeste
avant chargement, exige des identifiants/textes valides, rejette les IDs
répétés, utilise exclusivement le seuil figé 0,5 et refuse d'écraser une
sortie existante. Il n'écrit que les IDs, scores, décisions et hash modèle,
pas le texte brut. Cinq tests passent : chemin nominal, IDs dupliqués, sortie préexistante, hash
du manifeste incohérent et texte vide. Cet outil appelle un pickle/joblib de confiance locale et
reste strictement expérimental ; aucune intégration production.

### Collecte prospective ransomware.live : données encore insuffisantes

Le collecteur API existant a récupéré le mois d'octobre : 562 textes non
annotés, sans labels automatiques. La déduplication contre les 1 453 textes
d'entraînement trouve 347 recouvrements ID/texte et 215 contenus distincts.
Parmi les contenus distincts datés du 21 octobre ou après (après la dernière
collecte d'entraînement connue), seuls 113 existent ; la séparation stricte
par acteur/provenance laisse 4 textes de 2 acteurs inconnus. Le reçu de
comptage et les IDs sont dans `prospective-collection-audit.json`, les quatre
textes non annotés en fichier privé mode 0600.

C'est trop peu pour le test prospectif pré-enregistré (300 documents), et les
labels vérifiables indépendamment manquent toujours. Le test est donc déclaré
inconclusif ; aucun score du candidat n'est calculé sur ces 4 cas, aucun label
n'est dérivé de la présence sur ransomware.live, et rien n'entre à l'entraînement.
Il faut accumuler une collecte prospective plus longue et des annotations
traçables avant d'évaluer la promotion. Production et réserve inchangées.

### Audit sur acteurs jamais vus : faible rappel proxy, pas de promotion

La collecte 22 mois fournit 1 093 textes qui n'ont ni ID/contenu exact ni acteur
en commun avec les 1 453 exemples d'entraînement. Avant les prédictions, un
protocole exploratoire est fixé : les entrées de l'endpoint « victims » sont
uniquement des weak positives (revendications de victimes), seuil commun 0,5,
aucun ajustement. Ce n'est ni une vérité factuelle ni un test temporel prospectif ;
ce jeu tout-positif ne permet pas d'estimer la précision.

Résultat : le candidat spécialisé dépasse le seuil sur 121/1 093 (rappel proxy
11,1 %), le modèle partagé sur 103/1 093 (9,4 %). Delta apparié +1,65 point,
IC bootstrap acteur 95 % [+0,15 ; +3,54]. Avec le seuil source gelé du modèle
partagé (0,1299), son rappel proxy monte à 23,9 %, toujours loin du rappel
souhaité, mais ce point est secondaire et les seuils ne sont pas comparables.
Par année, candidat 9,8 % en 2025 et 17,6 % en 2026. Reçu :
`unseen-actor-recall-challenge/result.json`.

Ce test faible révèle surtout un décalage entre la définition des anciens
labels et l'ensemble de revendications du endpoint, ainsi qu'un rappel très
bas sur acteurs exclus. Il ne suffit pas à conclure que ces pages sont toutes
pertinentes ni à recalibrer le seuil. Décision : aucun déploiement ; garder le
candidat expérimental, revoir le contrat de label et accumuler une référence
traçable. La validation de précision reste impossible sur ce lot positif seul.

### Audit du contenu textuel du challenge d'acteurs

Un balayage regex agrégé (signal diagnostique seulement) révèle que 994/1 093
textes n'ont aucun des termes explicites ransomware/extorsion, fuite/exposition,
attaque/compromission ; seulement 17 contiennent un terme d'attaque/compromission
et 85 un terme de fuite/exposition. Le rappel proxy du candidat est 70,6 % sur
les 17 textes portant un terme attaque/compromission, 37,6 % sur les 85 portant
un terme fuite/exposition, 8,2 % sur les 994 sans ces termes. Ces familles se
recouvrent et les regex ne constituent pas des annotations.

Conclusion importante : l'appartenance à l'endpoint victimes n'est pas
suffisante comme label positif d'un classifieur du contenu textuel. Le collecteur
réduit volontairement chaque document au titre et à la description ; si ces
champs ne mentionnent pas l'incident, le texte seul n'étaye pas le label. Le
faible rappel ne peut donc pas être interprété comme une simple erreur NLP ;
il reflète aussi une inadéquation entre le weak label endpoint et le signal
textuel fourni. Le test d'acteurs est reclassé « diagnostic de couverture du
signal, inconclusif pour l'exactitude ». La règle demeure : ne pas entraîner
ou promouvoir sur ce weak label. Il faut obtenir des descriptions qui étayent
explicitement l'événement ou décider formellement que le modèle consomme des
métadonnées structurées séparées du texte. Aucun score/seuil modifié.

### Boucle suivante : séparation des deux contrats de label

L'audit stratifié des marqueurs montre une contradiction de schéma : dans
GitHub, 492/550 anciens négatifs contiennent au moins un marqueur littéral de
vulnérabilité/impact/attaque ; dans ransomware.live, 240/355 anciens positifs
n'en contiennent aucun. Les correspondances regex restent imparfaites et ne
sont pas des labels, mais elles indiquent que le corpus confond probablement
l'évidence lisible dans le texte et la signification d'un enregistrement source.

J'ai écrit une proposition de contrat à deux axes dans
`docs/validation/nlp-label-contract-proposal.md` : evidence textuelle CTI et
pertinence faible d'un claim structuré. Aucun label n'est converti ni entraîné.
La prochaine boucle est définie : figer la cible produit, créer un corpus par
axe avec abstentions explicites, puis répéter l'évaluation groupée et externe
sur la cible voulue. Jusque-là, tous les candidats restent expérimentaux.

### Première version du weak-label texte-only : couverture insuffisante

Un labeler déterministe prudent a été écrit et testé sur des exemples contrôlés.
Il n'utilise pas la source pour décider : positifs = mécanisme de vulnérabilité
ET impact, ou compromission/exposition avec contexte de revendication ; négatifs
= deux indices de contenu ordinaire et aucun marqueur CTI ; tout le reste =
abstention. Sept tests unitaires passent.

Sur 4 287 documents, il propose 891 positifs, seulement 40 négatifs et 3 356
abstentions. Par source : darkweb 158+/40-/1 423 abstentions ; GitHub
573+/0-/640 abstentions ; ransomware.live 160+/0-/1 293 abstentions. Ces labels
restent des propositions regex imparfaites (négation, paraphrases et preuves
implicites) et ne sont pas entraînés. Le manque de négatifs montre que la
politique de texte-only exclut les faibles inférences, comme voulu ; le corpus
actuel ne fournit pas une base équilibrée fiable sans source indépendante de
textes ordinaires négatifs et des labels CTI traçables.

Prochaine boucle : améliorer les règles négatives uniquement par fixtures
contrôlées, puis auditer la couverture et les erreurs de règles. Tant que la
strate négative est insuffisante ou que la précision des weak labels n'est
pas estimée, ne pas entraîner ce corpus. Le reçu est
`text-evidence-weak-label-summary.json`; la proposition n'altère pas les
labels historiques.

### Boucle v3 : réduire les faux négatifs faibles par abstention

L'audit des désaccords v2 a montré que des offres de comptes/accès compromis
pouvaient tomber du côté « négatif ordinaire ». La v3 ajoute une règle
positive explicite pour les offres d'accès compromis, et un veto d'abstention
si des indices CTI ambigus (compte, identifiants, accès, malware, fuite,
exposition, etc.) coexistent avec des termes ordinaires. En conséquence,
« troubleshooting » d'un compte email reste abstention plutôt que négatif.

Les 11 fixtures unitaires passent. Sur les mêmes 4 287 textes, v3 propose
893 positifs, 239 négatifs et 3 155 abstentions : darkweb 160+/172-/1 289
abstentions ; ransomware.live 160+/67-/1 226 ; GitHub 573+/0-/640. Le niveau
de couverture négative est plus élevé mais demeure trop fragile pour fournir
des labels d'entraînement : les règles lexicales ne valident ni la vérité de
l'événement ni sa pertinence. Les résultats complets et empreintes sont dans
`text-evidence-weak-label-summary-v3.json`; les exemples ligne à ligne sont
dans `text-evidence-weak-labels-v3.jsonl`. Aucune annotation historique,
recette de consommation, configuration de seuil ou modèle n'a été modifiée;
aucun entraînement ni promotion n'a eu lieu.

### Boucle v4 : test adversarial de la négation et des démentis

Des fixtures synthétiques ont révélé un défaut de v3 : des phrases niant une
exploitation, une vulnérabilité ou une fuite pouvaient être classées positives
par simple cooccurrence des mots. La v4 ajoute un veto d'abstention pour les
marqueurs explicites de négation/démenti et trois tests de non-régression,
dont le cas légitime « not paid — exposed » qui doit rester positif. Les 14
tests passent.

Sur les mêmes 4 287 textes, v4 produit 529 positifs, 239 négatifs et 3 519
abstentions : darkweb 122+/172-/1 327 abstentions ; ransomware.live
155+/67-/1 231 ; GitHub 252+/0-/961. La chute des positifs GitHub confirme
que la v3 surestimait l'évidence par cooccurrence. La v4 reste une règle
lexicale imparfaite : les proportions ne mesurent pas précision/rappel, et
aucun de ces labels faibles n'est admissible comme vérité terrain à lui seul.
Fichiers : `text-evidence-weak-labels-v4.jsonl` et
`text-evidence-weak-label-summary-v4.json`. Aucun entraînement, changement de
seuil ou promotion n'a eu lieu.

### Boucle v5 : démentis incomplets et événements allégués

Une seconde passe sur les fixtures a trouvé deux trous : « do not claim »
n'était pas couvert, et le mot « alleged » pouvait quand même déclencher un
positif par cooccurrence. Le veto inclut désormais les démentis, formulations
« alleged/unconfirmed/unverified », et l'abstention n'est plus contournée par
le seul marqueur d'exposition abrégée. Les 16 tests passent, y compris le
contrôle positif « not paid — exposed ».

La v5 donne 466 positifs, 239 négatifs et 3 582 abstentions : darkweb
105+/172-/1 344 abstentions ; ransomware.live 148+/67-/1 238 ; GitHub
213+/0-/1 000. Le progrès est ici une baisse des faux positifs plausibles,
pas une validation de la qualité réelle. Les fichiers immuables v5 sont
`text-evidence-weak-labels-v5.jsonl` et
`text-evidence-weak-label-summary-v5.json`. Toujours aucun entraînement,
réglage de seuil ou promotion.

### Boucle v6 : scénarios hypothétiques et descriptions de service

Les fixtures ont ensuite exposé des positifs sur des scénarios fictifs,
services de réponse à incident et enquêtes encore indécises. Ajout d'un veto
d'abstention pour ces contextes (ainsi que « potential », « tabletop »,
« simulation », « under investigation »). Les 19 tests passent.

Sur 4 287 textes, v6 propose 439 positifs, 239 négatifs et 3 609 abstentions
(darkweb 93+/172-/1 356 ; ransomware.live 147+/67-/1 239 ; GitHub
199+/0-/1 014). Cette réduction reflète un filtre de prudence; elle ne prouve
pas que les exemples restants sont corrects. Fichiers de v6 :
`text-evidence-weak-labels-v6.jsonl` et
`text-evidence-weak-label-summary-v6.json`. Ils restent hors entraînement.

### Boucle v6.1 : comparaison descriptive aux anciens labels

J'ai ajouté un audit reproductible qui compare les propositions non-abstain
aux labels historiques, par source, sans les traiter comme référence vraie.
Couverture : 678/4 287 (15,8 %). Parmi ces 678, accord historique 556/678
(82,0 %) ; ransomware.live 209/214 (97,7 %), darkweb 226/265 (85,3 %),
GitHub 121/199 (60,8 %). L'écart GitHub est un signal fort de désaccord de
contrat/taxonomie, pas une preuve que l'une des annotations est correcte.
Ces résultats confirment la décision : aucune fusion automatique avec les
labels historiques et aucun entraînement. Rapport :
`text-evidence-weak-label-alignment-v6.json`; calcul :
`apps/classifier/experiments/python/audit_weak_label_alignment.py` (2 tests passent).

### Boucle v7 : contamination des pages catalogue

L'analyse agrégée des désaccords a isolé une source de faux positifs dans les
pages darkweb concaténées : catégories, répertoires de marketplaces, « show
all/recommended », mentions de cartes vendues mais explicitement décrites
comme non volées. La v7 met ces structures en abstention et couvre les
négations « isn't/aren't ». Les 24 tests des deux suites passent.

v7 : 393 positifs, 239 négatifs, 3 655 abstentions (75+/172-/1 374 darkweb ;
147+/67-/1 239 ransomware.live ; 171+/0-/1 042 GitHub). L'accord avec les
labels historiques parmi 632 non-abstentions est 532/632 (84,2 %), sans
constituer une métrique de vérité terrain. Darkweb s'améliore à 89,5 %,
GitHub reste à 59,6 % ; aucune conclusion de qualité réelle n'en découle.
Cela renforce la décision de ne pas automatiser l'entraînement. Résultats :
`text-evidence-weak-label-summary-v7.json` et
`text-evidence-weak-label-alignment-v7.json`.

### Boucle v8 : les marchés illicites ne sont pas des négatifs ordinaires

Le reliquat des désaccords négatifs darkweb contenait des offres sensibles
(services de violence, cartes clonées, faux documents, drogues/armes) mêlées
à des mots de catalogue ordinaires. La v8 ajoute ces termes au veto d'un
négatif faible : le document devient abstention, pas positif. Les 25 tests
passent.

v8 : 393 positifs, 141 négatifs, 3 753 abstentions. Accord historique sur
534 labels non abstentionnistes : 439 (82,2 %), sans vérité terrain. Par
source, couverture 9,3 % darkweb, 14,7 % ransomware.live et 14,1 % GitHub.
Le faible volume restant est préférable à des négatifs de catalogue douteux;
ça ne donne toujours pas de corpus équilibré de confiance. Rapports :
`text-evidence-weak-label-summary-v8.json` et
`text-evidence-weak-label-alignment-v8.json`. Rien n'est entraîné.

### Boucle v9 : annonces explicites de publication

Un audit ciblé des désaccords (sans revue humaine) a lancé l'annotation locale
v9 à trois modèles. Le lot de 95 désaccords est trop lent pour traiter en une
fois : seuls les quatre premiers ont reçu les trois votes, les 91 autres
restent pending. Ces quatre votes convergent tous vers positif; un exemple
ransomware.live disait explicitement qu'un volume de données d'entreprise et
des données personnelles seraient téléversés. C'est un échantillon minuscule,
sélectionné sur désaccord, avec des annotateurs IA corrélés : pas une
validation.

La règle v9 reconnaît ce patron d'exposition annoncée, en plus des formulations
« not paid — exposed » déjà couvertes. Le test synthétique correspondant et le
contrôle de téléversement ordinaire passent. Au total, 27 tests passent. Sur
4 287 documents, v9 donne 469 positifs, 138 négatifs et 3 680 abstentions;
l'accord aux labels historiques est 515/607 (84,8 %), non une exactitude.
Pour ransomware.live seul, 284/286 (99,3 %) correspond aux labels historiques,
car le patron ajouté correspond souvent à la définition de cette source; cela
ne transforme pas cette source en vérité terrain. Résultats v9 :
`text-evidence-weak-label-summary-v9.json`,
`text-evidence-weak-label-alignment-v9.json`. Votes exploratoires incomplets :
`v8-disagreement-ai-consensus/` (91 pending). Pas d'entraînement.

### Boucle v10 : texte spéculatif généré par un annotateur

Le reliquat de deux désaccords ransomware.live contient des descriptions
explicitement balisées `[AI generated]` et des spéculations (« possibly from
a ransomware leak site ») plutôt que des preuves tirées du texte source. La
v10 transforme ces marqueurs d'incertitude en abstention. Les 28 tests passent.

v10 : 467 positifs, 138 négatifs, 3 682 abstentions. Sur les propositions
non-abstentionnistes, l'accord aux labels historiques est 515/605 (85,1 %);
ransomware.live atteint 284/284 parce que les deux descriptions spéculatives
sont exclues. Cet accord n'est pas de la précision : la source fournit aussi
le label historique et son taux ne mesure pas la vérité. Rapports :
`text-evidence-weak-label-summary-v10.json` et
`text-evidence-weak-label-alignment-v10.json`. Aucune annotation historique
ni aucun modèle n'a changé.

### Boucle v10.1 : distinguer erreur de règle et désaccord de cible

L'audit enrichi des motifs de désaccord montre que, parmi les 69 GitHub où
v10 propose positif contre négatif historique, 68 contiennent un marqueur de
mécanisme de vulnérabilité et 66 un marqueur d'impact; au moins 65 ont les
deux (inclusion-exclusion). À l'inverse, les désaccords ransomware.live sont
maintenant éliminés des exemples admissibles par les règles v10. Cela pointe
moins vers un défaut lexical qu'un possible écart de cible : avis de
vulnérabilité CTI versus signal ransomware uniquement. Les compteurs sont des
cooccurrences regex, pas une validation sémantique.

Décision de travail pour la suite : garder `text_cti_evidence` (pertinence CTI
explicite) comme axe candidat du classifieur général, et traiter une cible
ransomware-only à part; ne pas convertir les anciens labels ni promouvoir un
modèle tant que des jeux de validation autonomes ne séparent pas ces axes.
Ce choix de travail s'appuie sur le README « classifieur de pertinence » et ne
modifie pas le contrat ou le modèle de production. Les compteurs détaillés
sont dans `text-evidence-weak-label-alignment-v10.json`.

### Boucle suivante : format de corpus séparant les deux axes

J'ai matérialisé l'axe de travail en un export v10 à schéma explicite : label
faible texte-only (`candidate_label`, null pour abstention), provenance et
ancien label conservés dans des champs séparés, empreinte du texte vérifiée,
métadonnées interdites comme caractéristiques. Le drapeau
`training_eligible=false` est appliqué à chaque document. L'export contient
4 287 lignes (467+, 138-, 3 682 abstentions) et est conservé en mode privé;
il ne remplace aucune donnée existante. Ses empreintes et le compte rendu sont
dans `text-cti-evidence-candidates-v10-summary.json`. Trois tests dédiés
valident l'abstention null, la séparation des anciens labels et le contrôle
d'intégrité. Il s'agit d'un artefact d'audit, pas d'un corpus prêt à entraîner.

### Évaluation externe indépendante — 25 octobre 2026 (partielle)

Un lot test externe figé de 200 documents a été constitué sans score modèle,
après exclusion des quasi-doublons avec les références de développement. Le
modèle de production et son seuil (0,488787...) sont restés figés. L'annotation
locale par consensus Qwen/Gemma a traité 97 documents : 91 consensus, 1
abstention et 5 désaccords; 103 restent en attente. Un document long a dépassé
le délai d'inférence Qwen, interrompant le passage complet. Sur les 91
consensus uniquement : TP=0, FP=1, TN=87, FN=3 (accuracy 95,6 %, précision et
rappel observés 0 %). Le lot est très déséquilibré en labels (4 positifs); ces
chiffres sont donc instables et les labels de consensus IA ne sont pas vérité
terrain. Ce résultat partiel ne qualifie pas le modèle et ne justifie aucune
promotion, modification de seuil ou réentraînement. Fichiers privés :
`data/iteration-2026-10-25-independent-validation/` (manifest, test figé,
consensus, scores et `fixed-model-partial-test-results.json`).

Actualisation de l'évaluation, même date : l'annotation a été poursuivie en
lots distincts après les délais Qwen, sans toucher au jeu figé. Bilan agrégé :
192/200 documents ont reçu deux votes, dont 173 consensus, 18 désaccords et
1 abstention; 8 documents très longs sont restés non annotés après délais ou
pour éviter de relancer les mêmes dépassements. Sur les 173 consensus, le
modèle fixe donne TP=0, FP=4, TN=163, FN=6 (accuracy 94,2 %, précision et
rappel observés 0 %). Il y a 10 positifs consensus au total et aucun n'est
récupéré par le modèle; les labels restent toutefois consensus IA, non vérité
terrain, et 8 documents manquent. Le chiffre est un signal exploratoire, pas
une évaluation définitive ni un motif de réglage/promotion. Résultats détaillés
et identifiants non annotés : `fixed-model-partial-test-results.json` (privé).

### Audit d'erreurs après évaluation (diagnostic seulement)

Les 10 désaccords modèle/consensus se répartissent en 6 faux négatifs (3 avis
CERT-FR, 3 questions StackExchange Security) et 4 faux positifs (questions
StackExchange Security/Superuser/AskUbuntu). Cela suggère une difficulté de
transfert entre documents d'avis structurés et questions techniques, mais la
lecture porte sur un petit échantillon sélectionné et des labels IA. Le test
ayant été consulté, ces cas ne serviront ni à ajuster le seuil, ni à entraîner
ou sélectionner un candidat. Les identifiants et scores sont dans le rapport
privé `fixed-model-error-audit.json`. Décision : figer ce test et ne rien
promouvoir; toute nouvelle comparaison devra employer un nouveau jeu externe
préenregistré, après choix du contrat de labels et du protocole, avec modèles
et seuils prédéfinis.

### Partition validation — audit au seuil de production fixe

Le lot validation (200) a été annoté par le même protocole : 175 consensus,
24 désaccords, 1 abstention, aucun document en attente. Sans ajustement ni
entraînement, au seuil 0,488787... : TP=0, FP=5, TN=162, FN=8 sur les 175
consensus (8 positifs). Comme sur le test indépendant, le modèle ne récupère
aucun positif consensus; les seuls positifs prédits sont des faux positifs
selon ces labels. Cela confirme un signal de décalage entre le comportement
actuel du classifieur et la cible de l'annotation, pas une preuve absolue car
les labels sont IA et les positifs restent rares. Les petits sous-groupes
français sont descriptifs seulement. Rapports privés :
`fixed-model-validation-results.json` et annotations v5. Aucun seuil ni poids
n'a été modifié. Le test indépendant reste consommé; toute nouvelle version
exige un nouveau test figé avant d'ouvrir ses labels.

### Campagne source-stratifiée CISA / questions techniques — 25 octobre 2026

Pour tester un seuil candidat sans réutiliser les jeux déjà consultés, une
nouvelle campagne a été figée : 200 documents de validation et 200 de test,
répartis entre `en_security` et `en_general`. Le pool comprend 1 756 entrées du
catalogue CISA KEV et des questions techniques publiques. KEV indique qu'une
vulnérabilité est connue comme exploitée dans la nature; une entrée KEV n'est
pas, à elle seule, un exemple positif de menace CTI/ransomware, et l'absence du
catalogue n'est pas une preuve négative. Les entrées CISA restent donc des
candidates de validation, jamais des labels d'entraînement automatiques.

La validation a donné 198 consensus IA et 2 désaccords. Le protocole figé a
proposé le seuil expérimental 0,3589956 (poids identiques au modèle de
production). Sur cette validation : TP=90, FP=8, FN=10, TN=90, précision
91,8 %, rappel 90 %. Le test indépendant a ensuite été consommé une seule
fois : 195 consensus, 5 désaccords, couverture 97,5 %. Au seuil de production
0,4887875 : TP=49, FP=1, FN=49, TN=96; précision 98 %, rappel 50 %. Au seuil
candidat : TP=87, FP=14, FN=11, TN=83; précision 86,1 %, rappel 88,8 %,
F1=87,4 %. Statut automatisé : `not_qualified`; aucune promotion, aucun
réentraînement et aucun changement du seuil de production.

Ce test est équilibré et stratifié par source, pas représentatif de la
prévalence de production. Les 98 consensus `en_security` n'ont aucun négatif,
et les 97 `en_general` n'ont aucun positif : les métriques par stratum ne
permettent donc pas d'estimer les deux erreurs dans chaque groupe. Les labels
sont l'accord de deux modèles locaux, pas une vérité humaine. Artefacts privés
et empreintes : `data/iteration-2026-10-25-cisa-challenge/frozen-campaign/`
(`test-result.json`, `test-consumption.json`, manifeste, calibration et
annotations). Le snapshot KEV est dans
`data/iteration-2026-10-25-independent-campaign-2/cisa-kev/`.

Audit de protocole après lecture : la séparation complète label/stratum rend
ce lot impropre à estimer une performance transférable, même si les comptes
globaux satisfont la taille minimale. Le test reste consommé et ses scores ne
servent pas au réglage. Pour les futures campagnes, `automatic_campaign.py`
exige maintenant au moins 10 consensus de chaque classe à la fois dans chaque
source et chaque strate, en validation comme en test; l'absence de recouvrement
empêche la qualification. La validation de cette campagne confirme le
confondant : CISA KEV 99 positifs / 0 négatif, Superuser 0 / 44, AskUbuntu
1 / 54 (consensus IA). Les tests automatisés vérifient le rejet du cas
confondu et l'acceptation d'un support croisé. La sélection des campagnes
suivantes répartit désormais les documents à parts égales, sans score ni label,
entre les familles de source présentes dans le pool. Validation locale : 8
tests Python et `go test ./...` dans `apps/classifier` passent.

### Nouvelle tentative à source commune — 26 octobre 2026

Pour obtenir un échantillon où les deux classes peuvent apparaître dans la
même source, le collecteur public Stack Exchange a pris les pages 21–25 du site
Security : 499 documents, sans sélection par score. Ils ont été combinés aux
documents encore non réservés des pages 11–20; les textes de plus de 4 000
caractères ont été écartés avant annotation. Après exclusion des références
entraînement/relecture et des deux campagnes précédentes, le gel a retenu 400
questions, toutes `source=security` / `stratum=en_security`, en deux partitions
de 200. L'annotation v5 de validation est terminée : 165 consensus (9 positifs,
156 négatifs), 35 désaccords, couverture 82,5 %. Le seuil de repli .410231 donne
TP=7, FP=12, FN=2, TN=144 (précision 36,8 %, rappel 77,8 %, F1=50 %); les
contraintes de calibration sont irréalisables, et le minimum de 10 positifs
consensus par source/strate échoue (9 seulement). Le test est resté fermé et
non consommé. Conclusion : un échantillon aléatoire de questions Security
Stack Exchange est majoritairement hors cible pour la veille CTI, même dans
une source unique; il ne justifie ni modèle candidat, ni entraînement, ni
promotion. Artefacts privés : `data/iteration-2026-10-26-source-overlap/`.

Audit de réserve pour éviter toute réutilisation : les 1 621 documents
`darkweb_page` et les 1 453 documents `ransomware.live` de l'export weak-label
v10 recoupent par identifiant les corpus développement/validation/test déjà
présents. Ils ne sont donc pas candidats à un nouveau test aveugle; leurs
labels regex restent `training_eligible=false`. La prochaine collecte devra
apporter des documents réellement nouveaux, idéalement depuis une même source
où la politique texte-only produit assez de positifs et de négatifs.

### Vérification d’une collecte ransomware.live fraîche — 27 octobre 2026

Une collecte complète du mois d’octobre via l’API a retenu 583 enregistrements
(sans étiquette automatique). Le rapprochement exact des identifiants avec les
artefacts locaux antérieurs en retrouve 562; seuls 21 sont nouveaux, répartis
entre 13 groupes d’acteurs. Ces 21 nouveaux textes n’ont pas de doublon de
texte normalisé exact dans les références indexées. La réserve est trop petite
et trop regroupée pour lancer une évaluation représentative ou un entraînement
utile; aucune annotation supplémentaire ni évaluation du modèle n’a été lancée.

Un sondage exploratoire séparé, fait sur une collecte limitée de 500 entrées et
filtré à un document par acteur puis par proximité avec les références, avait
fourni 26 acteurs indépendants. L’annotation automatique v5 a donné 23 accords
(7 positifs, 16 négatifs) et 3 désaccords, soit 88,5 % de couverture. Ce signal
montre que la source contient les deux classes au niveau du texte, mais reste
uniquement un diagnostic de viabilité de source : il n’est ni un jeu de test,
ni une donnée d’entraînement, ni une mesure de performance. La collecte large
confirme qu’il faut d’abord obtenir une réserve prospective plus importante et
vérifier ses groupes avant toute nouvelle campagne.

### Audit des flux CTI officiels — 27 octobre 2026

Le collecteur borné a pris 10 documents de chaque flux CERT-EU et CERT-FR ainsi
que 100 entrées KEV liées aux ransomwares. Le flux CERT-FR exposait des liens
`www.cert.cyber.gouv.fr`; le contrôle d’hôte autorisait le domaine nu, mais pas
son alias `www`. L’allowlist a été étendue à cet alias exact sous `/cti/`, avec
un test unitaire. `go test ./cmd/collectcti` passe.

Le lot contient 120 lignes non annotées. Le rapprochement retrouve 110
identifiants déjà présents; les 10 identifiants restants ont tous un texte
normalisé déjà vu. Bilan : aucune nouveauté textuelle effective. Le fil CERT-FR
répète donc des archives déjà collectées, et le lot KEV ne constitue pas des
documents de classification adaptés à lui seul. Rien n’a été annoté, entraîné
ou évalué, et aucun seuil/modèle n’a changé. Prochaine décision : chercher une
source réellement nouvelle et à styles mixtes plutôt que d’élargir ces flux
archivés.

### Rafraîchissement des discussions Security Stack Exchange — 27 octobre 2026

Une collecte score-blind des cinq pages les plus récentes (500 questions)
contient 498 identifiants déjà présents dans les artefacts locaux. Deux
questions seulement sont nouvelles, toutes deux distinctes par texte exact.
Ce lot est trop petit pour un diagnostic annoté; il reste non annoté et hors
entraînement/test. Les prochains gains ne viendront pas d’une nouvelle lecture
des mêmes pages : il faut une source conversationnelle différente et un
protocole de collecte validé avant d’ouvrir un autre lot.

### Audit de faisabilité RansomLook — 27 octobre 2026

La documentation publique expose `GET /api/posts?days=7` et ne présente pour
chaque entrée récente que `group_name`, `post_title` et `discovered`. La lecture
bornée du flux fournit 295 posts, 294 titres distincts et 49 groupes. Par
comparaison normalisée avec les textes ransomware.live déjà archivés, 153
titres apparaissent déjà dans un document antérieur. Les 142 autres ne sont
que des titres de victimes, sans contenu narratif ou preuve textuelle utile au
classifieur; les récupérer seuls créerait des exemples courts et fortement
identifiants plutôt qu’un corpus qualitatif. Décision : ne pas collecter ni
annoter ce flux pour l’entraînement; RansomLook peut servir de provenance ou
de piste de découverte, mais il faut trouver une source exposant des textes
complets, puis faire le contrôle de droits, de doublons et de qualité avant
toute intégration.

### Nouvelle source en texte intégral : CISA Cybersecurity Advisories — 27 octobre 2026

Après le rejet de RansomLook comme corpus textuel, le flux officiel CISA
`/cybersecurity-advisories/cybersecurity-advisories.xml` a été ajouté au
collecteur CTI. La liste d’hôtes/chemins est restreinte à
`www.cisa.gov/news-events/cybersecurity-advisories/`, avec tests positifs et
négatifs. Le collecteur a téléchargé 10 articles complets (les 10 entrées du
flux), de 15 226 à 71 412 caractères. Les 10 ont un identifiant et une URL
inédits, et aucun texte normalisé exact n’existait dans les artefacts indexés.
L’extraction est du contenu long; les articles CISA sont décrits par l’agence
comme des rapports détaillés sur les menaces, TTP et indicateurs
([page CISA](https://www.cisa.gov/news-events/cybersecurity-advisories)).

Décision : garder les 10 articles dans la réserve privée non annotée. Leur
provenance n’est pas un label et le volume limité ne justifie pas de promotion.
Leur grande longueur implique une annotation segmentée et un coût notable; on
ne les injecte pas dans l’entraînement sans consensus automatique préalable.
La collecte comprend aussi les flux CERT/KEV utilisés dans l’audit précédent;
seuls les 10 articles `cisa-advisory` sont nouveaux dans cette tentative.

Annotation automatique v9 des 10 articles CISA : 7 consensus positifs, 3
abstentions `context_limit`, aucune négative ni divergence, couverture 70 %.
Le seuil de 48 000 octets du prompt écarte les trois plus longs textes sans
troncature. Les sept consensus sont des candidats de développement positifs,
pas une validation de vérité terrain; une source seule sans négatifs ne peut
qualifier le classifieur. Ils restent séparés des données d’entraînement
actuelles jusqu’à un audit de doublons/groupes et une prochaine construction
équilibrée. Artefacts : `data/iteration-2026-10-27-source-refresh/`.

Trace d’exécution : une première annotation lancée par erreur sur la réserve
combinée de 31 entrées a été interrompue après un seul vote Qwen sur un document
CERT-EU. Son plan et son vote incomplet sont conservés dans
`cti-combined-v9-interrupted/`; ils n’entrent ni dans le consensus CISA, ni dans
l’entraînement ou une évaluation.

Contrôle de chevauchement de développement : les 7 consensus positifs ont été
comparés aux 2 325 lignes de `training_data.csv` avec les deux déduplications
de production. Il n’y a aucun texte normalisé identique, ni correspondance des
shingles à Jaccard ≥ 0,8. SimHash signale 239 paires à distance ≤ 10; un
échantillon de ces références correspond à des pages très différentes
(plateforme chiffrée, forum, page CAPTCHA, échange de crypto), ce qui confirme
que ce seuil produit ici des collisions et ne doit pas servir seul à supprimer
des textes. Rapport détaillé : `cisa-only-v9/training-overlap-audit.json`.

Décision de boucle : ne pas entraîner/promotionner sur ce lot. Il apporte
7 positifs automatiques, aucun négatif et 3 abstentions; même sans doublon de
production détecté, il ne permet pas un gain équilibré ni une qualification
indépendante. Conserver le lot comme réserve de développement non injectée.
Prochaine tentative : trouver une source texte intégral comportant les deux
classes selon le contrat d’étiquetage, puis auditer doublons, groupes et
couverture avant d’ouvrir un jeu prospectif. Le modèle et son seuil restent
inchangés.

### Annotation segmentée des trois abstentions CISA — 27 octobre 2026

Pour récupérer les trois abstentions `context_limit` sans tronquer le texte,
les trois articles concernés ont été découpés aux frontières de paragraphes
(31 segments au total) puis soumis au même script/politique v9 et aux trois
modèles locaux figés. Résultat : 30/31 segments en consensus, un segment en
abstention, aucun désaccord; après agrégation stricte, un article récupère un
consensus positif, un reste abstenu à cause d’un segment incertain et un est
mixte entre segments. Le corpus passe donc de 7 à **8 positifs de développement**,
mais ne gagne aucun négatif. Reçu complet et agrégation dans `cisa-segment-v9/`.
Contrôle strict contre les 2 325 textes d’entraînement : 0 doublon exact et
0 correspondance shingles ≥ 0,8; 246 paires SimHash ≤ 10 (même signal de
collision précédemment constaté). Décision inchangée : garder les huit textes
hors entraînement et ne pas promouvoir; la classe négative manque toujours.
Cette campagne n’est ni un test NLP ni une modification de l’entraînement.

### Limite de pagination Stack Exchange — 27 octobre 2026

Une tentative bornée des pages 26–30 de Security Stack Exchange n’a conservé
aucun document : l’API publique répond `403 access_denied` dès la page 26 sans
clé/app enregistrée. La documentation API fixe à 25 la page maximale pour
l’accès anonyme et indique les filtres de dates comme méthode de découpage des
résultats. Le reçu vide et son erreur sont préservés dans
`security-stackexchange-pages-26-30.jsonl.summary.json`; aucun contournement ni
nouvelle annotation n’a été tenté. Pour poursuivre sans identifiant API,
l’exploration devrait utiliser des fenêtres temporelles disjointes, pages 1–25
par fenêtre, puis dédupliquer avant toute annotation.

### Fenêtre temporelle Stack Exchange — 27 octobre 2026

Le collecteur a été étendu pour appliquer un `max` de date de création à la
requête Security et borner l’accès anonyme aux pages 1–25; un test vérifie
l’URL de fenêtre. En partant strictement avant la date la plus ancienne de la
page 25, une collecte score-blind de cinq pages a retenu 500 questions,
longueur médiane 814 caractères. Les 500 identifiants sont nouveaux dans les
794 artefacts antérieurs indexés. Comparaison de production avec le CSV
d’entraînement : 0 identique normalisé et 0 doublon shingles ≥ 0,8. SimHash
≤ 10 signale 3 426 paires, mesure trop permissive déjà connue et ignorée pour
la déduplication.

Pour tester la viabilité CTI sans annoter les 500 questions hors-cible, une
requête lexicale figée avant annotation a retenu 46 textes ≤ 4 000 caractères
mentionnant explicitement ransomware, breach, compromise, stolen, access
broker, initial access, malware, ransom note ou extortion (un texte long a été
exclu). Ce sous-ensemble est un lot de développement biaisé par la recherche
lexicale, pas un test ni une estimation de prévalence. L’annotation v9 est
terminée : 46 consensus (14 positifs, 32 négatifs), aucun désaccord ni
abstention; une voix Llama invalide a été compensée par l’accord des deux autres
modèles. Les labels sont faibles, pas vérité humaine. Les 46 textes enrichis
sont conservés avec `training_eligible=false` dans
`security-stackexchange-strong-signal-v9/development-labeled.jsonl`. Aucun
score NLP n’est utilisé, aucun test n’est ouvert et rien n’est injecté dans
l’entraînement. Décision : source viable pour quelques positifs et négatifs
difficiles, mais l’échantillon filtré et de taille modeste n’autorise pas une
qualification; conserver comme réserve de développement.

### Seconde fenêtre temporelle Security Stack Exchange — 27 octobre 2026

La fenêtre suivante, bornée avant l’horodatage le plus ancien du lot
précédent, fournit 500 autres questions. Les 500 IDs sont nouveaux dans les
837 artefacts antérieurs, et l’audit de production face à l’entraînement donne
0 texte normalisé identique et 0 doublon shingles ≥ 0,8. SimHash ≤ 10 signale
3 685 paires et reste ignoré pour la déduplication. La même sélection lexicale
fixée à l’avance retient 41 textes courts (un texte long écarté). Annotation v9
terminée : 41 consensus (7 positifs, 34 négatifs), sans abstention, désaccord
ni vote invalide. Les labels restent faibles et
`training_eligible=false` dans `security-stackexchange-older-strong-signal-v9/`.
Comparaison croisée avec l’entraînement et les 46 consensus de la première
fenêtre : aucun doublon exact ou shingles à Jaccard ≥ 0,8; aucun doublon
interne entre les 87 documents retenus.

Au total, les deux sélections lexicales fournissent **87 consensus (21
positifs, 66 négatifs)**. Pour préparer une réserve strictement équilibrée,
j’ai gardé les 21 positifs et sélectionné 21 négatifs par ordre SHA-256
déterministe d’ID. L’artefact de 42 documents est
`security-stackexchange-42-balanced-development-reserve.jsonl`; chaque ligne
reste marquée `training_eligible=false`. Cette construction ne touche pas le
corpus actuel et n’est pas représentative : elle sert uniquement à un éventuel
essai candidat ultérieur.

### Score exploratoire sur la réserve Security StackExchange — 27 octobre 2026

Après gel des 42 IDs équilibrés (21/21), le modèle de production figé a été
appliqué sans recalibrage et sans modifier le seuil `0.4887874623244017`. Sur
les labels faibles v9, il prédit 6/21 positifs et 19/21 négatifs correctement
(6 TP, 15 FN, 2 FP, 19 TN; précision 75 %, rappel 28,6 %). Le score médian est
0,4006 pour les positifs et 0,3420 pour les négatifs : séparation modeste et
fort chevauchement. Cet échantillon lexicalisé et annoté par consensus local
n’est pas un test indépendant ni une vérité terrain; ces chiffres servent
uniquement à diagnostiquer le transfert de source, sans régler le seuil.

Décision : ne pas promouvoir et ne pas ajouter ces lignes au corpus. Les
21 positifs manqués justifient une analyse d’erreurs et un candidat séparé,
mais pas un ajustement sur ces mêmes 42 documents. Scores reproductibles
(ID/score/décision seulement) :
`apps/classifier/data/iteration-2026-10-27-source-refresh/security-stackexchange-42-frozen-production-scores.jsonl`.
Le SHA-256 du modèle avant et après est identique :
`1fd5dd5e5531c5b2b80586ae6b1bb23b0f0daa105a232d7f2c76c03fccc36458`.

### Diagnostic lexical des erreurs sur la réserve — 27 octobre 2026

J’ai ventilé les erreurs au seuil figé avec six familles de termes indicatives,
qui se chevauchent. Parmi les 17 textes contenant `malware`/`trojan`/`virus`,
les 7 annotés positifs sont tous manqués; les textes contenant des marqueurs
`credential/password/account/login/token` ont 5/8 positifs détectés, ceux de
`data theft/leak` 6/11, et ceux de compromission/incident 4/13. Un seul texte
sur deux contenant ransomware/ransom est positif et aucun des deux n’est
classé positif. Ces petits effectifs et labels faibles ne permettent pas de
conclure à une capacité générale; ils signalent surtout un décalage entre le
vocabulaire/sens de cette source Q&A et le domaine de production du modèle.

Artefact sans extraits de texte ni réglage de seuil :
`apps/classifier/data/iteration-2026-10-27-source-refresh/security-stackexchange-42-error-breakdown.json`.
Les catégories sont lexicales, non exclusives, et servent uniquement à orienter
la suite. Décision : ne pas apprendre directement de cette réserve. Prochaine
boucle recommandée : constituer une nouvelle collecte prospective à partir
d’une source davantage centrée sur les incidents ransomware/CTI, sélectionner
les documents par dates/IDs plutôt que par score NLP, puis vérifier l’accord
sémantique des annotations et le recouvrement avant de créer un candidat isolé.

### Gel de la cible et première collecte multi-source additionnelle — 27 octobre 2026

Le protocole `docs/validation/nlp-text-cti-corpus-protocol-v1.md` gèle désormais
la cible des prochaines itérations à `text_cti_evidence` : preuve textuelle d'un
signal CTI, indépendante de l'appartenance à une source ou à un endpoint. L'axe
`structured_claim_relevance` reste séparé. L'annotation automatique v9,
consensus local de trois modèles avec abstentions conservées, est décrite comme
weak label et non vérité humaine. Le contrat n'altère aucun corpus historique,
modèle ou seuil.

Une troisième fenêtre Security StackExchange, bornée avant le 30 juin 2024,
a fourni 499 questions score-blind. Le même filtre lexical préenregistré que
les deux précédentes fenêtres en retient 50 textes courts; 9 textes longs sont
écartés. Les 50 IDs et textes normalisés retenus n'ont aucun doublon exact avec
le `training_data.csv` ou les 87 exemples consensus précédents. Ils restent
`training_eligible=false` jusqu'à l'annotation et l'audit de groupes. Reçu brut,
sélection et compteurs dans `data/iteration-2026-10-27-source-refresh/`.
L'annotation automatique v9 de ces 50 documents est en cours; aucun score du
classifieur n'a guidé la sélection.

Pour rechercher un texte complet différent des Q&A, `collectcti` accepte
maintenant le flux public SANS Internet Storm Center Handler's Diary. Le
collecteur a récupéré 10 entrées SANS, 10 CISA, 10 CERT-EU, 25 CERT-FR et 1
entrée KEV auxiliaire (56 lignes). Le contrôle exact retrouve 45 textes déjà
présents dans les références locales; les 10 entrées SANS sont nouvelles dans
les artefacts indexés et varient de 471 à 35 591 caractères. Un test d'allowlist
n'accepte que `isc.sans.edu/diary/rss/`. SANS indique que le flux expose un RSS
complet ([page officielle des flux](https://isc.sans.edu/xml.html)); le flux
actuel déclare une licence Creative Commons Attribution-Noncommercial 3.0 US.
Par conséquent, ces textes sont gardés comme réserve d'audit/développement,
`training_eligible=false` jusqu'à l'examen de compatibilité de licence; la
présence dans le flux ne fait pas label. Aucun contenu n'est annoté ou utilisé
pour entraîner à ce stade.

Audit de provenance complémentaire : les fenêtres Stack Exchange antérieures
ont borné l'API par `max`, mais leur schéma ne conservait pas le timestamp exact
`creation_date`. Le collecteur écrit désormais `source_date` UTC à partir de la
réponse API; un test couvre conversion et absence de date. Les lots déjà créés
ne sont pas réécrits. `go test ./...` passe dans `apps/scraper`.

Annotation terminée : les 50 nouveaux candidats reçoivent 50 consensus v9/50,
11 positifs et 39 négatifs, sans désaccord ni abstention. Avec les 87 consensus
précédents, la réserve de développement atteint 137 (32 positifs, 105 négatifs);
une réserve équilibrée déterministe de 64 (32/32) a été créée, toujours avec
`training_eligible=false`. Les labels sont consensus IA faibles et la sélection
lexicale reste biaisée. Audit contre l'entraînement : 0 égalité normalisée et
0 doublon shingles de production; aucune égalité/quasi-égalité shingles interne
sur les 137. SimHash ≤10 produit 1 352 paires et n'est pas utilisé comme règle
de déduplication.

À seuil de production inchangé (0,488787), diagnostic descriptif sur les 137 :
TP=8, FP=12, FN=24, TN=93 (précision 40 %, rappel 25 %, F1 30,8 %). Le lot est
un échantillon développement lexicalisé, annoté automatiquement, donc ces
chiffres ne sont ni une performance générale ni un test indépendant. Ils
confirment que ce modèle ne transfère pas bien à cette population de questions
CTI-candidates; ils ne servent pas à régler le seuil. Reçu, réserves, scores et
audits dans `data/iteration-2026-10-27-source-refresh/`.

Décision de boucle : ne pas promouvoir et ne pas réinjecter automatiquement les
labels. On a maintenant assez de weak labels pour un essai OOF purement
exploratoire, mais pas pour une qualification : les 137 sont tous issus du
filtre lexical, alors que le seuil doit être validé sur le flux réel préfiltré
et le test externe reste à construire. La prochaine étape est de mesurer un
candidat en CV groupée sur le flux lexicalisé, en gardant un flux aléatoire
séparé pour la calibration/qualification; ne pas confondre ces métriques ni
ouvrir un test consommé. Les résultats d'entraînement restent candidats privés.

## Expérience exploratoire OOF sur le corpus ciblé — 27 octobre 2026

Le nouveau chemin `--include-targeted-development` autorise explicitement le corpus
Stack Exchange filtré lexicalement uniquement pour une sélection OOF exploratoire.
Le test indépendant reste fermé. Cinq plis groupés ont terminé sur 137 consensus IA
faibles (32 positifs, 105 négatifs) ; le meilleur choix exploratoire est
`balanced_feature_families`, L2=0,001, poids positif=1,5, seuil=0,444252.
Sur ses prédictions OOF sélectionnées : précision 36,96 %, rappel 53,13 %, F1
43,59 % (17 TP, 29 FP, 15 FN, 76 TN), AP=0,3504. Les objectifs précision >=50 %
et rappel >=90 % ne sont pas atteints. La sélection et l'évaluation utilisent le
même corpus OOF, donc ces chiffres sont optimistes et ne constituent pas une mesure
d'indépendance ni une qualification. Le fichier de poids est strictement isolé
sous `data/iteration-2026-10-27-source-refresh/`; `production_promotion=false` et
`test_read=false`. Production/seuil inchangés.

### Collecte élargie ransomware.live — fenêtre de 24 mois — 27 octobre 2026

Pour disposer d'une réserve plus large sans transformer l'appartenance à la source
 en label, `collectransomwarelive` a été borné à 24 mois (novembre 2024–octobre
2026), avec cadence de requêtes espacée, plafond porté de 5 000 à 25 000 et
étiquetage explicitement désactivé. L'API a fourni 12 225 lignes retenues et
5 510 lignes écartées/dupliquées. Les données sont conservées en mode 0600 sous
`data/iteration-2026-10-27-source-refresh/ransomware-live-24mo-full-retry-unscored.jsonl`;
SHA-256 `fe7f03bd3e93655506193f1b316bc9f970d7f4e7083c9e47db2d277117090006`.

Comparaison aux JSONL locaux antérieurs : 6 685 IDs et 6 981 textes normalisés
avaient déjà été vus ; 5 540 IDs sont nouveaux. Le lot couvre 198 acteurs; il
contient encore 25 doublons de texte normalisé internes. Ce n'est donc ni 12 225
exemples indépendants, ni 12 225 positifs. Aucun document n'est annoté ni admis
à l'entraînement. Un premier essai à 5 000 lignes avait tronqué le pool; une
requête rapprochée a reçu HTTP 429, après quoi la collecte a été relancée à
cadence plus lente. Les tests du collecteur passent. Décision : traiter cet
export comme un grand pool brut, auditer/dédupliquer puis échantillonner par
acteur et date avant annotation; construire un test futur séparé. La fenêtre
maximale du collecteur est 24 mois, pas l'archive historique entière.

### Pilote d'annotation de la réserve ransomware.live — 27 octobre 2026

Après déduplication des textes normalisés connus, un échantillon déterministe de
500 documents nouveaux a été tiré sur 157 acteurs et les mois disponibles
(maximum quatre documents par acteur). La première tentative v9 sur 500 a été interrompue après un vote Qwen (73,8 s);
le reçu partiel est conservé. Elle a ensuite été reprise avec le même plan figé
et a terminé : 500/500 ont reçu les trois votes; 498 consensus (107 positifs, 391
négatifs) et 2 désaccords. Trois réponses étaient invalides, mais les votes
restants ont permis un consensus majoritaire. Un pilote distinct de 30 documents (30 acteurs, distribution temporelle, textes
courts) a ensuite reçu trois votes locaux : 29 consensus (10 positifs, 19
négatifs), 1 désaccord, 100 % de votes terminés. Ce sont des weak labels, pas une
vérité terrain.

Diagnostic au modèle et seuil de production figés : TP=5, FP=1, FN=5, TN=18;
précision 83,3 %, rappel 50 %, F1 62,5 %. Échantillon de 29 trop petit et labels
IA trop faibles pour qualifier la source ou promouvoir un modèle. Les 30 lignes
annotées restent `training_eligible=false`; le désaccord est préservé. Aucun
réentraînement, réglage du seuil ou changement de production. Artefacts dans
`data/iteration-2026-10-27-source-refresh/` (`ransomware-live-24mo-novel-v9-pilot-30/`,
`ransomware-live-24mo-novel-v9-pilot-30-labeled.jsonl` et diagnostic du modèle).

### Résultat complet de l'annotation de 500 ransomware.live — 27 octobre 2026

La campagne v9 reprise s'est achevée sur les 500 documents tirés par acteur/date :
1 500 votes locaux, 498 consensus (107 positifs, 391 négatifs), 2 désaccords,
0 abstention/pending après consensus. Trois réponses isolées étaient invalides.
Les consensus restent des weak labels. Le modèle de production figé au seuil
0,488787 obtient sur ces 498 accords TP=60, FP=18, FN=47, TN=373 : précision
76,92 %, rappel 56,07 %, F1 64,86 %. C'est un diagnostic développement ciblé sur
une source, non une mesure représentative de production ou une qualification;
aucun réglage de seuil ou promotion n'en découle.

Une réserve équilibrée distincte de 214 textes a été construite (107 positifs,
107 négatifs; désaccords exclus), avec négatifs sélectionnés par acteur et date.
Toutes les lignes restent `training_eligible=false`; elle n'a pas encore servi à
l'entraînement. Consensus, votes, reçus, scores et réserve équilibrée sont dans
`data/iteration-2026-10-27-source-refresh/`. La production reste intacte.

### Candidat isolé ransomware.live — CV groupée sur acteurs — 27 octobre 2026

Un candidat privé a été entraîné sur les références historiques et les 498 consensus
v9; ses cinq plis OOF gardent les groupes d'acteurs ensemble. Sélection exploratoire :
`balanced_feature_families`, L2=0,001, poids positif=1,5, seuil=0,348493.
Sur l'OOF utilisé pour sélectionner la configuration : TP=97, FP=88, FN=10, TN=303,
précision 52,43 %, rappel 90,65 %, F1 66,44 %, AP 66,84 %. La contrainte de
sélection est satisfaite sur ce développement faible, mais les labels sont des
consensus IA et le seuil/configuration ont été sélectionnés sur ces mêmes données;
ces résultats sont optimistes et ne qualifient pas un déploiement.

Comparaison descriptive sur le même lot : production figée P=76,92 %, R=56,07 %,
F1=64,86 %; candidat OOF P=52,43 %, R=90,65 %, F1=66,44 %. Aucun test indépendant
nouveau n'a été lu. Le candidat reste isolé (`production_promotion=false`), le
modèle/seuil de production n'ont pas changé. Prochaine étape : figer ce candidat,
puis le comparer une seule fois à un futur lot prospectif indépendant, après
annotation aveugle et séparation par acteurs; ne pas retoucher le seuil entre-temps.
Artefacts : `ransomware-live-498-actor-grouped-exploratory-candidate.json` et son
`.selection.json` dans le répertoire d'itération du 27 octobre.

### Contrôle initial du flux post-gel — 27 octobre 2026

Après gel des empreintes et des seuils, une nouvelle lecture de l'API d'octobre
a trouvé 597 documents, dont seulement 2 publiés après la date maximale du
corpus de développement. L'un réutilise un acteur déjà présent en développement;
l'autre est un seul document d'un nouvel acteur. L'échantillon est très inférieur
au minimum fixé de 50 consensus par classe : aucun texte n'a été annoté, aucun
score calculé et le test reste fermé. Le candidat et le seuil restent gelés; il
faut accumuler de nouvelles publications/acteurs avant l'évaluation unique.
Audit dans `prospective-ransomware-live-comparison-v1/october-refresh-audit.json`.

### Test rétrospectif sur la vraie pipeline — 29 octobre 2026

Un export des pages réellement collectées par la pipeline a été dédupliqué
contre les textes connus, puis gelé sans lire les verdicts du classifieur. Les
cinq textes inspectés pendant le contrôle initial et leurs groupes ont été
exclus; les groupes hôtes répétés ont également été écartés. Le lot final contient
1 537 pages provenant de la fenêtre du 22 au 27 octobre. Il s'agit d'un test
rétrospectif, pas d'un flux recueilli après le gel. L'annotation aveugle par trois
modèles locaux est terminée : 1 512 consensus (235 positifs, 1 277 négatifs),
23 abstentions et 2 désaccords, soit 98,37 % de couverture consensus.

À seuils déjà figés, sur ces 1 512 consensus : le modèle de production obtient
P=32,68 %, R=92,77 %, F1=48,34 % (218 TP, 449 FP, 17 FN, 828 TN). Le candidat
ransomware.live obtient P=22,09 %, R=97,87 %, F1=36,05 % (230 TP, 811 FP, 5 FN,
466 TN). Tous deux passent le seuil de rappel fixé à 90 %, mais échouent au
minimum de précision de 50 %. Le candidat produit 811 faux positifs et ne
franchit pas le critère de promotion. Décision : conserver le modèle de
production sans modification; ne pas réentraîner ni retoucher le seuil sur ce
test consommé.

Les références sont des consensus faibles d'annotation automatique, pas une
vérité humaine. L'exclusion de texte normalisé exact a été appliquée, mais un
audit indépendant des quasi-doublons sémantiques n'a pas été réalisé. Les reçus,
votes, scores et le résultat détaillé sont conservés sous
`data/iteration-2026-10-27-source-refresh/actual-pipeline-retrospective-holdout-v1/`;
le JSON de comparaison contient les empreintes des modèles et des fichiers de
score. Ce diagnostic montre surtout une forte sur-prédiction positive, déjà
présente en production et amplifiée par le candidat; toute amélioration devra
traiter les faux positifs sur un nouveau jeu indépendant, sans calibrer sur ce
lot consommé.

### Test prospectif sur la vraie pipeline — tranche 02 — 30 octobre 2026

Une tranche indépendante de 722 pages, toutes collectées après le gel et réparties
sur 722 groupes hôtes, a terminé l'annotation v9 à l'aveugle : 705 consensus
(74 positifs, 631 négatifs), 15 abstentions, 2 désaccords, couverture 97,65 %.
Aux seuils gelés, production : P=22,66 %, R=85,14 %, F1=35,80 % (63 TP, 215 FP,
11 FN, 416 TN); candidat : P=16,12 %, R=93,24 %, F1=27,49 % (69 TP, 359 FP,
5 FN, 272 TN). La production échoue aux seuils de précision et rappel; le candidat
échoue à la précision minimale de 50 %. **Aucune promotion**, aucun seuil ajusté;
l'artefact production reste intact. Les labels sont des consensus de trois LLM
locaux, pas une vérité humaine. Rapport détaillé :
`nlp-prospective-pipeline-2026-10-30.md`; données et empreintes dans
`data/iteration-2026-10-27-source-refresh/prospective-production-pipeline-comparison-v1/`.
Prochaine étape : développer contre les faux positifs sur une réserve séparée,
sans réutiliser les deux tests consommés, puis prévoir un nouveau test prospectif.

### Audit des causes de faux positifs — 30 octobre 2026

Les deux vrais tests indiquent une précision faible; le candidat n'améliore pas
le classement prospectif (AUROC/AP 0,821/0,426 contre 0,838/0,471 production) et
son seuil inférieur augmente les alertes positives à 61 % du flux. Les erreurs
sont concentrées autour d'indices génériques de marché, prix, cartes et jeux :
37/39 mentions casino et 34/35 mentions betting dans les négatifs sont signalées
positives par la production. `s_price_mention` a un poids fort positif dans les
deux modèles. La distribution du développement (21,5 % positifs ransomware.live)
est plus positive que le flux réel (10,5 %), et sa source ne reproduit pas assez
les négatifs commerciaux de la pipeline. L'audit complet, les limites et la
suite recommandée sont dans `nlp-why-no-improvement-2026-10-30.md`. Les tests
restent consommés, aucune correction de seuil ni promotion n'est faite.
