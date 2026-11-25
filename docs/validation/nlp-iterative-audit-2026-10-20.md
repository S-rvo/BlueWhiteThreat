# Boucle d’amélioration NLP — audit du 20 octobre 2026

## But et règle de boucle

Chaque itération suit le même ordre : **audit → hypothèse → application sur le
développement → gel du candidat et du seuil → test indépendant unique → décision**.
Un test lu est consommé. Ses textes et labels restent exclus des entraînements et
réglages ultérieurs. Une absence de qualification ouvre une nouvelle itération,
avec un nouveau lot indépendant. L’annotation reste entièrement automatique.

## Audit après le contrôle d’août

Le candidat TF-IDF obtient 73,0 % de précision, 96,4 % de rappel et 83,1 % de F1
sur 169 consensus. Le résultat ponctuel est bon, mais trois limites empêchent sa
promotion :

1. seulement 28 positifs consensuels, contre 50 requis ;
2. une seule famille de source, `ransomware.live` ;
3. la borne basse Wilson du rappel est 82,3 %, sous l’objectif.

L’analyse des onze divergences TF-IDF/référence révèle aussi un problème plus
fondamental : dix sont comptées comme faux positifs, mais plusieurs textes parlent
explicitement d’un dump serveur vendu, de données extraites, d’une publication
annoncée, d’une demande ou d’une faille. Les deux annotateurs avaient interprété
« mécanisme de menace concret » trop strictement et classé ces compromissions en
négatif. La mesure sous-estime donc probablement la précision réelle du candidat.
Le problème prioritaire est la cohérence de la référence automatique, pas le
seuil du classifieur.

## Décision et application — itération 1

Une politique d’annotation v3 séparée a été créée, sans toucher aux campagnes
figées. Elle rend explicites les cas « données volées/extraites », vente de dump,
publication, extorsion et accès compromis. Elle conserve l’abstention et exige une
preuve textuelle verbatim. Aucun score NLP n’est envoyé aux annotateurs.

Contrôle diagnostique sur les onze divergences consommées :

- 7 nouveaux consensus et 4 désaccords ;
- 6 des consensus correspondent désormais explicitement à des compromissions ;
- les cas encore ambigus deviennent majoritairement des désaccords plutôt que de
  faux négatifs automatiques.

Ce diagnostic sert uniquement à valider la politique ; il ne réouvre pas le test
d’août et ne modifie aucun score publié.

Données préparées pour la première boucle :

- 499 documents d’août hors test, réservés au développement v3 ;
- 660 nouveaux documents de juillet collectés sans label de provenance ;
- un prochain test de 400 documents gelé avant annotation et entraînement :
  200 revendications de juillet couvrant 58 acteurs et 200 avis GitHub couvrant
  des groupes CVE distincts ;
- 989 avis GitHub restent dans une réserve de développement disjointe.

Le prochain candidat sera choisi uniquement par validation croisée hors pli sur
le développement. Le lot indépendant restera absent de l’entraînement, du choix
des caractéristiques et du seuil.

## Critères de décision

La qualification demande simultanément : couverture de consensus ≥ 80 %, au
moins 50 positifs et 50 négatifs consensuels, précision ≥ 50 %, rappel ≥ 90 %,
absence de fuite détectée et résultats publiés par source. La promotion de
production reste une opération séparée : un résultat mono-source ou une forte
instabilité entre sources relance la boucle.

## Pistes classées pour les boucles suivantes

1. augmenter les positifs et négatifs difficiles automatiquement annotés jusqu’à
   plusieurs milliers, avec plafonds par source et événement ;
2. ajouter CERT-FR/CERT-EU, CISA KEV, MITRE ATT&CK, MISP/CIRCL, ThreatFox et des
   textes conversationnels afin de tester la généralisation ;
3. conserver mots, bigrammes et n-grammes de caractères, qui dominent actuellement
   l’encodeur compact ;
4. retester un encodeur multilingue figé seulement après augmentation du corpus ;
5. mesurer systématiquement les métriques et intervalles par source, langue et
   style ;
6. conserver les LLM locaux comme annotateurs, avec version de politique et
   empreintes figées, plutôt que comme modèle de production.

## Résultat de l’itération 1

Le candidat sélectionné sur 427 consensus de développement atteint hors pli
94,2 % de précision, 90,7 % de rappel et 92,5 % de F1. Il a ensuite été figé
avec son seuil avant toute annotation du test indépendant de 400 documents.

Le test produit 249 consensus : 69 positifs et 180 négatifs. La couverture est
seulement de 62,3 %, sous les 80 % exigés. Sur les consensus, le candidat obtient
85,7 % de précision, 52,2 % de rappel et 64,9 % de F1. Par source :

- ransomware.live : 90,0 % de précision, 81,8 % de rappel, 85,7 % de F1 ;
- GitHub Advisories : 75,0 % de précision, 25,0 % de rappel, 37,5 % de F1.

**Décision : aucune promotion.** Le test est consommé. L’écart montre un
surapprentissage de style : le développement récent contient des revendications,
mais pas assez d’avis techniques. Il confirme aussi une faiblesse de la chaîne
d’annotation sur les avis longs : 19 documents dépassent la limite de contexte et
58 réponses individuelles omettent leur preuve, ce qui transforme une grande
partie du lot en abstentions.

## Audit et décision — itération 2

Deux corrections sont retenues sans modifier le candidat ni le test consommé :

1. intégrer au prochain développement des avis GitHub, avec groupes CVE disjoints
   des tests ;
2. utiliser une politique d’annotation v4 séparée : chaque annotateur doit choisir
   0 ou 1 et au moins un segment de preuve, tandis que leur désaccord conserve
   l’incertitude ; la limite d’entrée passe de 14 à 48 ko dans la même fenêtre de
   16 384 tokens.

La prochaine itération devra sélectionner ses paramètres hors pli sur un
développement multi-source, puis utiliser un nouveau test temporel et disjoint.

## Résultat de l’itération 2

La politique v4 a supprimé les échecs de format et de contexte observés à
l’itération 1. Sur 899 documents de développement multi-source, 720 consensus
exploitables ont été obtenus (224 positifs, 496 négatifs ; couverture 80,1 %).
Le candidat sélectionné hors pli atteint 81,5 % de précision, 90,6 % de rappel et
85,8 % de F1 sur ce développement.

Le test indépendant, figé avant entraînement et composé de 200 revendications
ransomware.live de juin et 200 avis GitHub disjoints, produit 331 consensus
(82,8 %), dont 107 positifs et 224 négatifs. Résultat global : 92 VP, 25 FP,
15 FN, 199 VN ; précision 78,6 %, rappel 86,0 %, F1 82,1 %. Par source :

- ransomware.live : précision 97,3 %, rappel 75,0 %, F1 84,7 % ;
- GitHub Advisories : précision 70,0 %, rappel 94,9 %, F1 80,6 %.

**Décision : aucune promotion.** La couverture, les effectifs et la précision
passent, mais le rappel global reste inférieur à 90 %. Le test est consommé.
L’audit attribue 12 des 15 faux négatifs aux revendications ransomware.live :
formulations courtes ou indirectes de données volées, exfiltrées, chiffrées,
échantillons disponibles et demandes de rançon. L’itération suivante doit
intégrer ces erreurs consommées au développement, renforcer ces variantes et
être jugée sur un nouveau mois temporellement disjoint ; le seuil de l’itération
2 reste inchangé.

## Résultat et promotion de l’itération 3

Le développement incorpore les références consommées de l’itération 2 et atteint
1 051 consensus (331 positifs, 720 négatifs). La sélection hors pli obtient
83,0 % de précision, 90,0 % de rappel et 86,4 % de F1, avec un seuil de
0,488787 figé avant le nouveau test.

Le test indépendant comprend 200 publications ransomware.live découvertes en mai
(64 acteurs, maximum 8 par acteur) et 200 avis GitHub aux groupes disjoints. Il
produit 331 consensus sur 400 (82,8 %), dont 79 positifs et 252 négatifs. Résultat :
72 VP, 23 FP, 7 FN, 229 VN ; précision 75,8 %, rappel 91,1 %, F1 82,8 %.

Par source : ransomware.live atteint 92,3 % de précision et 82,8 % de rappel sur
29 positifs ; GitHub Advisories atteint 69,6 % de précision et 96,0 % de rappel
sur 50 positifs. Toutes les portes globales prédéfinies passent. Après réussite
des tests Go classifier/crawler/scraper, des 24 tests Python, de la compilation
Python et de `git diff --check`, le candidat a été copié vers `model.json` et son
rapport indépendant vers `model.json.evaluation.json`. Empreinte SHA-256 du modèle
promu : `1fd5dd5e5531c5b2b80586ae6b1bb23b0f0daa105a232d7f2c76c03fccc36458`.

La limite restante est documentée : la référence est un consensus de deux LLM
locaux et le rappel ransomware par source ne porte que sur 29 positifs. La
surveillance de production doit donc conserver les métriques par source et par
période, sans retoucher ce résultat indépendant.

## Vérification dans la pipeline réelle

Le service `classifier-prd` a été redémarré avec le dossier du classifieur monté
en lecture seule. L’empreinte de `/data/model.json` dans le conteneur correspond
à celle du candidat promu. Le journal confirme le chargement de la version
`provisional-oof-advisories-20261020T103652Z`.

Sur l’état MongoDB observé après le passage complet : 365 articles au total,
352 contenus non vides classés par cette version, aucun contenu non vide restant,
114 verdicts positifs, 238 négatifs et 60 placés dans la zone d’incertitude. Les
13 documents restants ont un contenu vide et sont explicitement ignorés. Aucun
verdict d’une ancienne version ne subsiste. Ce contrôle valide le trajet réel
modèle monté → chargement → sélection MongoDB → inférence → écriture du verdict.

Un instantané ultérieur, pris pendant que la collecte continuait, contient 353
documents classés, 115 positifs et 61 dans la zone d'incertitude. Sa distribution
est conservée dans `production-monitoring-snapshot.json`. Elle sert de référence
opérationnelle pour détecter une dérive, mais ne constitue pas une mesure de
précision faute de nouvelles références indépendantes.

Les contrôles statiques `go vet ./...` passent également dans classifier,
crawler et scraper. Les 24 Mo d'artefacts datés (corpus, votes, reçus et modèles
intermédiaires) restent locaux grâce à la règle `/data/*2026*`; aucun n'est suivi
par Git. Le modèle précédent est conservé avec son manifeste dans
`iteration-2026-10-20/rollback/`, ce qui rend le retour arrière reproductible.

## Réserve indépendante suivante

La collecte suivante a été figée avant tout score du modèle. Elle contient 1 000
documents sans label automatique : 500 publications ransomware.live de septembre
et octobre et 500 avis GitHub. La sélection exclut les identifiants, groupes et
textes normalisés déjà rencontrés dans 17 229 enregistrements de travail. Les
revendications couvrent 73 acteurs, avec un maximum de 15 documents retenus par
acteur. Son empreinte SHA-256 est
`d0b1cb3dae0ca81d3296398e66ebec14ffd238e42a3ab48b590627791af6d55d`.

Pendant cette préparation, un défaut du collecteur GitHub a été détecté : l'API
des avis utilise des curseurs `after` dans son en-tête `Link` et ignore le
paramètre numérique `page`. L'option trompeuse `page-from` a été remplacée par
`skip-pages`, qui traverse réellement la chaîne de curseurs avant d'écrire. La
réserve et son manifeste restent dans `iteration-4-reserve/`, hors Git. Ils ne
doivent entrer ni dans l'entraînement ni dans un choix de paramètres avant leur
annotation et leur consommation comme nouveau test indépendant.

## Audit exploratoire de l'itération 4

Les deux annotateurs ont terminé les 1 000 documents : 815 consensus, 184
désaccords et une réponse invalide, soit 81,5 % de couverture. Le lot contient
244 positifs et 571 négatifs parmi les consensus.

Une erreur d'orchestration a toutefois invalidé le statut indépendant formel de
ce passage : la commande destinée à écrire le reçu pré-score utilisait un chemin
préfixé deux fois depuis `apps/classifier`. Cette étape a échoué, puis la commande
de score suivante du même shell s'est exécutée. Un reçu postérieur décrit
explicitement l'incident et le test est marqué consommé. Les résultats ci-dessous
sont donc exploratoires et ne permettent aucune promotion ni modification de
seuil.

Avec le modèle actuellement déployé et son seuil inchangé : 200 VP, 61 FP, 44
FN et 510 VN ; précision 76,6 %, rappel 82,0 % et F1 79,2 %. Par source :

- GitHub Advisories : précision 76,9 %, rappel 88,2 %, F1 82,2 % ;
- ransomware.live : précision 75,8 %, rappel 67,6 %, F1 71,4 %.

Le rappel minimal de 90 % aurait échoué même avec un protocole valide. L'audit
des 44 faux négatifs trouve notamment 25 textes liés aux données volées ou
divulguées, 16 à la rançon/extorsion et 14 aux accès ou identifiants. Le prochain
cycle doit incorporer ce lot désormais consommé au développement, renforcer ces
formulations courtes et sélectionner le seuil uniquement hors pli. Toute mesure
de confirmation devra utiliser un nouveau lot, avec un reçu créé et vérifié par
une commande qui s'arrête immédiatement si sa création échoue.

## Itération 5

Le développement atteint 1 866 consensus (575 positifs, 1 291 négatifs). La
sélection hors pli du candidat donne 69,5 % de précision, 90,1 % de rappel et
78,5 % de F1, avec un seuil de 0,497813 fixé avant le test.

Le nouveau test disjoint contient 400 documents équilibrés par source. Les deux
annotateurs initiaux produisent 313 consensus et 87 désaccords, soit 78,25 % de
couverture. Avant tout score NLP, une règle de départage a été figée : un
troisième modèle local vote uniquement sur ces désaccords et la majorité binaire
fait référence. Les 87 réponses sont valides, portant la couverture à 100 %.
Le reçu pré-score a ensuite été créé et vérifié avant l'évaluation des deux
modèles (SHA-256 du reçu :
`7089897bedd15fdfbb725e578e78680d4c5f9cefb2c778aef50e8c96af017d0d`).

Sur 178 positifs et 222 négatifs, le candidat obtient 143 VP, 39 FP, 35 FN et
183 VN : précision 78,6 %, rappel 80,3 %, F1 79,4 %. Le modèle déployé obtient
respectivement 84,5 %, 73,6 % et 78,7 %. Le candidat améliore donc le rappel et
le F1 global, mais reste sous le rappel minimal de 90 %.

Par source, le candidat atteint 85,7 % de rappel sur GitHub Advisories et 69,5 %
sur ransomware.live. La porte de rappel par source (80 %) échoue également.
**Décision : aucune promotion.** L'audit des 35 faux négatifs relève surtout 21
cas liés aux données volées ou divulguées, 10 aux accès ou identifiants et 9 aux
vulnérabilités ou exploits. Le modèle de production et son seuil restent
inchangés.

## Itération 6 — encodeur multilingue gelé

Les 400 références consommées de l'itération 5 ont été ajoutées au
développement, qui contient désormais 2 266 documents étiquetés (753 positifs,
1 513 négatifs). Un encodeur
`paraphrase-multilingual-MiniLM-L12-v2` à révision figée a été testé sans
fine-tuning ; seule une tête de régression logistique est entraînée. La sélection
à cinq plis choisit `C=10`, un poids positif de 1,5 et un seuil de 0,273160.

Les résultats hors pli sont 53,3 % de précision, 90,0 % de rappel et 67,0 % de
F1. Ils sont nettement inférieurs au candidat TF-IDF de l'itération 5 sur le
même type de sélection (69,5 %, 90,1 %, 78,5 %). **Décision : arrêter cette
branche avant tout test indépendant.** Le modèle de production reste inchangé
et aucun nouveau lot de test n'est consommé.

## Itération 6 — contrainte de rappel par source

La sélection du seuil TF-IDF a été renforcée : les prédictions hors pli doivent
maintenant atteindre au moins 90 % de rappel global et 90 % de rappel pour
chaque source comportant au moins 20 positifs. Le vocabulaire reste ajusté à
l'intérieur de chaque pli et aucun test indépendant n'a été lu.

Sur les 2 266 documents de développement, la configuration retenue
(`stable_terms_word_reserve`, L2=0,001, poids positif=1) fixe le seuil à
0,398695. Elle obtient 60,1 % de précision, 96,0 % de rappel et 73,9 % de F1.
Par source, le rappel atteint 99,6 % sur GitHub Advisories et 90,3 % sur
ransomware.live, mais au prix de 480 faux positifs, dont 438 sur GitHub.

La contrainte élimine bien le masquage d'une source faible, mais le candidat est
moins discriminant que les candidats précédents et ne justifie pas de consommer
un nouveau test indépendant. **Décision : arrêter cette variante avant test et
ne pas promouvoir.** Le prochain essai doit améliorer la séparation des
négatifs GitHub sans réduire le rappel ransomware, par exemple avec une
calibration ou des têtes spécifiques aux sources évaluées entièrement hors pli.

## Itération 7 — seuils spécifiques aux sources

Une expérience hors pli a conservé exactement le même score TF-IDF tout en
figeant une frontière par source : 0,564924 pour GitHub Advisories et 0,394367
pour ransomware.live, avec le seuil global comme repli pour toute source
inconnue. Sur le développement, cette règle obtenait 77,1 % de précision,
90,4 % de rappel et 83,3 % de F1. Le support optionnel de ces seuils a été ajouté
à l'artefact et à la pipeline avec compatibilité ascendante.

Un nouveau test disjoint de 400 documents (200 par source) a été sélectionné et
gelé avant annotation. Les deux annotateurs locaux ont produit 325 consensus et
75 désaccords, soit 81,25 % de couverture. Le seuil minimal de couverture étant
atteint, seuls les consensus ont été utilisés. Un reçu pré-score vérifié
(`ab7f7f4339b44067a4813885ac48f84f2383afe54086a11e96fa999c5d39ebf0`)
a figé le candidat, le test, les références et les portes avant toute inférence.

Sur 117 positifs et 208 négatifs, le candidat obtient 105 VP, 32 FP, 12 FN et
176 VN : précision 76,6 %, rappel 89,74 % et F1 82,7 %. Le modèle déployé obtient
76,3 %, 76,9 % et 76,6 %. Par source, le rappel candidat vaut 91,4 % sur GitHub
et 87,2 % sur ransomware.live. Les portes par source à 80 % passent, mais la
porte globale de rappel à 90 % échoue d'un exemple. **Décision : aucune
promotion et aucun ajustement post-test.** Le test est désormais consommé ; les
44 erreurs ont été enregistrées pour alimenter le prochain audit de
développement.

## Audit de qualité des références après l'itération 7

L'examen des 44 erreurs a révélé un défaut systémique de l'annotation v4 : pour
27 avis GitHub prédits positifs par le candidat, les deux annotateurs avaient
répondu négatif tout en expliquant explicitement que le texte décrivait une
vulnérabilité. Cela contredit la règle écrite qui classe une vulnérabilité ou un
exploit concret comme positif. Ces références restent figées pour le résultat
de l'itération 7, qui n'est ni recalculé ni promu.

La politique v5 remplace le choix direct de classe par six critères atomiques
(incident, données compromises, accès compromis, campagne/service malveillant,
vulnérabilité/exploit concret et défense liée à une menace). Le programme
calcule ensuite la classe positive par OU logique. Les tests vérifient qu'un
annotateur ne peut plus reconnaître une vulnérabilité tout en renvoyant la
classe négative.

Sur les 44 erreurs réannotées uniquement comme diagnostic post-test, v5 produit
25 consensus et 19 désaccords. Parmi les 27 anciens faux positifs GitHub, 16
sont désormais des consensus positifs et 11 restent en désaccord ; 43 des 54
votes individuels sont positifs et 42 cochent explicitement le critère de
vulnérabilité ou exploit. Ce résultat confirme un problème de qualité de la
référence v4 plutôt qu'un simple excès de sensibilité du NLP. La prochaine
boucle utilisera v5 pour constituer le développement et les futurs tests ; elle
ne réinterprétera pas rétroactivement les résultats déjà consommés.

## Itération 8 — développement nettoyé par critères atomiques

La réannotation v5 des 400 documents consommés produit 299 consensus et 101
désaccords (74,75 %). Comme il s'agit désormais de développement, seuls les
consensus ont été ajoutés ; aucune résolution artificielle des désaccords n'a
été appliquée. Le développement atteint 2 565 références : 928 positives et
1 637 négatives.

La validation croisée a aussi été corrigée pour répartir chaque combinaison
source/classe entre les cinq plis. Avec des seuils spécifiques aux sources, le
meilleur diagnostic hors pli atteint 76,7 % de précision, 90,2 % de rappel et
82,9 % de F1. Le rappel est de 90,0 % sur GitHub et 90,5 % sur ransomware.live.
Le résultat est stable, mais ne dépasse pas le F1 hors pli de l'itération 7
(83,3 %). **Décision : ne pas consommer de nouveau test et ne pas promouvoir.**
La prochaine branche devra changer la capacité du modèle plutôt que déplacer
encore les seuils, notamment en évaluant des têtes réellement spécialisées par
source avec un repli global.

## Itération 9 — nouveaux négatifs réels de production

Après consommation du test réel, un nouvel export temporel a exclu ses 352
empreintes normalisées. Il contient 84 documents distincts, collectés ensuite,
et a été gelé sous le SHA-256
`66c0e1b8da73b0b2c19e6300cb8ed48aab039a4f901c858f30ad61cea7818fc3`.
L'annotation v5 produit 67 consensus (60 négatifs, 7 positifs), 16 désaccords
et une abstention. Aucun désaccord n'a été forcé.

Le développement atteint ainsi 2 632 consensus : 935 positifs et 1 697
négatifs. Avec le seuil global, la validation hors pli atteint 62,5 % de
précision, 95,7 % de rappel et 75,6 % de F1. Sur les 67 pages réelles seules,
elle obtient 20,0 % de précision et 100 % de rappel. Le diagnostic par source
place la frontière `darkweb_page` à 0,753886 et obtient 50,0 % de précision,
100 % de rappel et 66,7 % de F1, mais sur seulement sept positifs et sept faux
positifs.

Le score global avec seuils par source est 76,4 % de précision, 90,3 % de
rappel et 82,7 % de F1, légèrement inférieur à l'itération 8 (82,9 %). Le
support réel est trop faible pour figer une frontière fiable. **Décision : ne
pas ouvrir de nouveau test, ne pas promouvoir et continuer l'accumulation
temporelle de pages réelles**, en visant davantage de positifs et de négatifs
de commerce Tor avant la prochaine validation croisée.

## Itération 10 — seconde tranche temporelle réelle

Une seconde tranche, postérieure et sans chevauchement normalisé avec les 436
pages déjà gelées, contient 93 pages réelles (SHA-256
`e72f69f6a35b8a702c8761f81b14a52c78388a5a2aa4cbe3a926b62c1c1d97e1`).
V5 fournit 65 consensus : 54 négatifs et 11 positifs ; 28 désaccords sont
exclus. Le développement total monte à 2 697 consensus, soit 946 positifs et
1 751 négatifs. Il contient désormais 132 consensus `darkweb_page`, dont 18
positifs.

Au seuil global, la validation hors pli donne 62,1 % de précision, 95,8 % de
rappel et 75,3 % de F1. Sur `darkweb_page`, elle donne 27,7 % de précision et
100 % de rappel. Le seuil spécialisé diagnostique améliore la précision à
55,0 %, mais le rappel chute à 61,1 % (11/18). Comme cette strate reste sous le
minimum pré-déclaré de 20 positifs, elle n'est pas couverte par la contrainte de
rappel par source. Le diagnostic global spécialisé tombe par ailleurs à
89,96 % de rappel et 82,54 % de F1.

**Décision : aucune promotion et aucun test indépendant.** Cette itération
révèle qu'un seuil de source choisi avec moins de 20 positifs est trop instable.
La prochaine boucle doit d'abord dépasser ce support, puis exiger explicitement
90 % de rappel sur `darkweb_page`; déplacer encore son seuil maintenant serait
un ajustement sur un échantillon insuffisant.

## Audit de capacité après l'itération 10

Le principal plafond n'est pas le nombre brut de paramètres. Le modèle dispose
déjà de mots, bigrammes de mots, n-grammes de caractères, signaux structurels,
TF logarithmique, normalisation L2 et sélection supervisée de 5 000 variables.
Ses coefficients les plus positifs restent toutefois dominés par des termes
génériques (`data`, `hack`, `card`, `leak`, `buy`, mention de prix). Cela
explique la confusion persistante entre une menace ciblée et le commerce Tor
générique. L'encodeur multilingue gelé déjà essayé n'a pas corrigé ce défaut.

L'audit identifie quatre priorités, dans cet ordre :

1. **Référence réelle** : accumuler chronologiquement au moins 50 à 100
   positifs `darkweb_page`, avec plusieurs centaines de négatifs réels répartis
   entre marchés, crypto, escrow, annuaires, guides et boutiques. Les 18
   positifs actuels ne suffisent pas à estimer un seuil de source.
2. **Validation sans mémorisation** : remplacer les plis par document par des
   plis groupés selon l'hôte ou la campagne, puis ajouter une validation
   temporelle. Des pages proches issues du même site ne doivent pas apparaître
   à la fois dans l'apprentissage et dans le pli évalué.
3. **Capacité mieux répartie** : mesurer séparément mots, bigrammes et caractères,
   puis tester un espace creux plus large avec des budgets distincts. Le plafond
   partagé de 5 000 variables met aujourd'hui en concurrence ces trois familles.
4. **Objectif de production** : suivre PR-AUC, précision à rappel >= 90 % et
   faux positifs par volume journalier. Un F1 sur corpus artificiellement
   équilibré ne représente pas la prévalence réelle.

Une anomalie de sélection a été corrigée pendant cet audit : le diagnostic de
seuil par source pouvait choisir un optimum F1 ne respectant pas 90 % de rappel,
puis sérialiser ce seuil malgré un support inférieur à 20 positifs. La sélection
maximise désormais la précision sous contrainte explicite de rappel, publie le
support et l'éligibilité par source, et n'ajoute au candidat que les seuils dont
le support atteint le minimum. Les sources insuffisamment représentées utilisent
le seuil global. Cette correction ne modifie ni le modèle de production ni les
résultats historiques.

La prochaine décision mesurable est donc : collecter et annoter sans revue
humaine une nouvelle tranche temporelle jusqu'à dépasser le support réel, puis
comparer une validation groupée/temporelle au protocole actuel. Aucun nouveau
test indépendant ne doit être consommé tant qu'une amélioration hors pli robuste
n'est pas observée.

## Itération 11 — validation groupée et troisième tranche réelle

Un nouvel export temporel a exclu les 529 pages déjà gelées. Il contient 81
pages nouvelles, sous le SHA-256
`de0532b5c989e7263348f79951a2a1d189ec247a49b20ed11f299ec661f5f629`.
L'annotation v5 donne 60 consensus (53 négatifs et 7 positifs) et 21
désaccords exclus. Le développement atteint 2 757 consensus : 953 positifs et
1 804 négatifs. La strate `darkweb_page` contient maintenant 192 pages, dont
25 positives, et dépasse donc pour la première fois le support minimal de 20.

La validation croisée conserve désormais toutes les pages d'un même groupe
d'hôte ou de campagne dans un seul pli. Les métadonnées temporelles sont aussi
propagées dans le rapport. Cette mesure réduit le risque de mémoriser le style
d'un site présent simultanément dans l'apprentissage et dans l'évaluation.

Le meilleur candidat groupé (`stable_terms_word_reserve`, L2=0,001, poids
positif 1) obtient au seuil global 61,18 % de précision, 95,91 % de rappel et
74,70 % de F1. Avec les seuils par source, le diagnostic obtient 75,20 % de
précision, 90,35 % de rappel et 82,08 % de F1. Sur `darkweb_page`, la frontière
0,606463 donne 38,33 % de précision, 92 % de rappel et 54,12 % de F1 (23 VP,
37 FP, 2 FN et 130 VN).

Le rappel réel est désormais mesurable sous la porte de 90 %, mais la précision
reste trop basse et le F1 global demeure inférieur aux itérations 7 et 8.
**Décision : aucune promotion et aucun nouveau test indépendant.** La prochaine
boucle doit augmenter la diversité positive réelle vers 50 à 100 exemples et
tester une capacité de représentation différente plutôt qu'un nouveau
déplacement de seuil.

## Audit critique de l'itération 11

L'audit du fichier réellement évalué nuance la qualification « groupée ». Les
2 757 documents de consensus ont 2 757 valeurs de `group` distinctes. La règle
anti-fuite fonctionne techniquement, mais elle ne regroupe donc encore aucun
document. Les réclamations ransomware utilisent l'identifiant de réclamation
plutôt que l'acteur, et les avis GitHub utilisent le CVE plutôt qu'un projet ou
un fournisseur. Les pages de pipeline sont bien groupées par hôte, mais
l'échantillon n'a qu'une page conservée par hôte. Le résultat de l'itération 11
doit ainsi rester considéré comme une validation stratifiée par document, et
non comme la preuve d'une généralisation à des acteurs ou sites inconnus.

La dimension temporelle n'est pas encore testée non plus. `collected_at`
correspond surtout à l'heure d'ingestion : les données GitHub et ransomware ont
été importées sur moins d'une journée, et les 192 pages darkweb sur environ
trois heures. Pour ransomware.live, `source_date` est la date pertinente et doit
être utilisée pour une coupure chronologique. Un vrai test temporel devra
entraîner sur les périodes anciennes et évaluer exclusivement une période
postérieure.

Autres limites mesurées :

- l'intervalle de Wilson à 95 % de la précision `darkweb_page` est environ
  27,1–51,0 %, et celui du rappel 75,0–97,8 % ; 25 positifs permettent
  d'appliquer la porte, mais pas de conclure avec précision ;
- 21 pages sur 81 de la dernière tranche sont des désaccords exclus. Cette
  exclusion peut favoriser les cas faciles et doit être rapportée comme une
  couverture de 74,1 %, en plus des métriques sur consensus ;
- sur les 5 000 variables du candidat, 2 695 sont des n-grammes de caractères,
  1 500 des mots, 803 des bigrammes et seulement 2 des signaux structurels. Le
  plafond partagé compresse particulièrement les expressions et les structures ;
- les poids positifs restent dominés par `data`, `hack`, `card`, `leak`,
  `files` et les mentions de prix. Le modèle apprend encore des marqueurs de
  commerce clandestin plutôt que la relation entre victime, compromission et
  action menaçante ;
- les deux meilleures configurations hors pli ne diffèrent que de 0,08 point
  de F1. Ce faible écart ne justifie pas de conclure que la réserve de mots est
  intrinsèquement supérieure.

Les prochains changements doivent donc être évalués dans cet ordre : créer de
vrais groupes (`source_actor` pour ransomware, projet/éditeur pour les avis,
hôte pour les pages), ajouter une coupure par `source_date`, mesurer la
couverture des annotateurs, puis réaliser une ablation avec budgets séparés
pour mots, bigrammes, caractères et structures. Une variante ne pourra ouvrir
un test indépendant que si elle améliore la précision darkweb sous rappel >=
90 % sur plusieurs plis groupés et sur la tranche temporelle la plus récente.

## Itération 12 — groupes d'acteurs effectifs et ablation des variables

Le regroupement ransomware a été corrigé : 1 404 documents sont désormais
répartis entre 99 acteurs, contre 1 404 identifiants uniques auparavant. Les
collecteurs futurs utilisent aussi l'acteur pour ransomware.live, le package et
l'écosystème pour GitHub Advisories, et conservent la date source. Les plis
placent un groupe entier dans une seule partition et équilibrent conjointement
les strates source/classe, y compris pour les groupes contenant les deux classes.

Une troisième famille de variables réserve de la capacité aux mots, bigrammes,
caractères et structures. Elle est retenue de très peu : son artefact contient
1 800 mots, 1 500 bigrammes, 1 695 fragments de caractères et 5 indicateurs
structurels. Au seuil global, la validation réellement groupée atteint 54,33 %
de précision, 96,75 % de rappel et 69,58 % de F1. Avec les seuils par source,
elle atteint 69,33 % de précision, 90,14 % de rappel et 78,38 % de F1.
`darkweb_page` reste à 36,51 % de précision et 92 % de rappel ; ransomware.live
tombe à 69,85 % de précision et 90,19 % de rappel lorsque les acteurs sont
disjoints entre plis.

Cette baisse confirme que les validations précédentes bénéficiaient de styles
d'acteurs vus dans plusieurs plis. La variante équilibrée ne dépasse la variante
sans réserve que de 0,03 point de F1 global, écart négligeable. **Décision : ne
pas promouvoir et ne pas ouvrir de test indépendant.** Le diagnostic groupé
devient désormais la référence minimale pour les décisions suivantes.

## Audit de la collecte réelle avant le résultat de l'itération 13

La quatrième tranche réelle contient 264 pages, dont 220 consensus, 41
désaccords et 3 abstentions : sa couverture est de 83,33 %. Les consensus ne
fournissent que 6 positifs contre 214 négatifs, soit un rendement positif de
2,73 %. Le développement contient maintenant 412 pages `darkweb_page`, dont 31
positives. Au rendement observé, une collecte purement aléatoire demanderait
encore environ 700 pages annotées pour seulement atteindre 50 positifs.

Le désaccord des annotateurs est systématique : Qwen produit 47 décisions
positives, contre 7 pour Gemma. Parmi les 41 désaccords, les critères les plus
fréquents dans les votes sont le service ou la campagne malveillante (27 votes)
et les données volées ou divulguées (24 votes). L'exclusion des désaccords ne
constitue donc pas un bruit aléatoire ; elle retire surtout les cas positifs
ambigus et rend la référence de développement conservatrice. Les résultats
restent valides selon la règle v5 figée, mais le prochain protocole devra figer
avant annotation un vote majoritaire à trois modèles ou mesurer séparément la
sensibilité aux deux annotateurs. Aucun désaccord de la tranche actuelle ne sera
réétiqueté après observation.

La collecte aléatoire de racines onion est désormais le principal goulot. Les
264 pages proviennent de 264 hôtes distincts, ce qui apporte de bons négatifs
difficiles mais aucune répétition permettant d'évaluer la généralisation à de
nouvelles pages d'un site connu. Le scraper reste stable autour de 400 PID, mais
les journaux montrent de nombreux délais Tor, échecs SOCKS et services disparus.
La prochaine collecte doit privilégier prospectivement des sources à meilleur
rendement positif et plusieurs chemins publics par hôte, tout en conservant un
échantillon aléatoire séparé pour estimer la prévalence réelle.

Priorités issues de cet audit : (1) terminer l'itération 13 groupée ; (2) ne pas
promouvoir sur la seule croissance des négatifs ; (3) créer une politique v6 à
trois votes figée avant le prochain lot ; (4) séparer un flux ciblé pour enrichir
l'apprentissage d'un flux aléatoire réservé à l'estimation de production ; (5)
atteindre au moins 50 positifs et plusieurs pages par hôte avant une nouvelle
évaluation indépendante.

## Application de l'audit — protocole v6 et flux séparés

Une politique prospective v6 a été créée sans modifier les campagnes v5. Elle
utilise trois modèles locaux figés (`qwen3:8b`, `gemma4:e2b`, `llama3.2:3b`) et
une majorité de deux décisions valides. La classe reste dérivée des six critères
atomiques et chaque vote conserve ses preuves. Les tests couvrent notamment un
vote positif 2 contre 1. Les anciens désaccords ne sont pas réinterprétés.

Le crawler de production reste indépendant du modèle. Un échantillonneur séparé
constitue uniquement un flux d'enrichissement de développement : les expressions
fortes sont retenues directement, tandis que `leak`, `database`, `access`,
`credentials`, `dump` ou `files` exigent un second signal CTI. Il n'attribue
aucune classe et inscrit le motif d'échantillonnage dans chaque document.

Après exclusion des 874 pages déjà gelées, un export aléatoire de 47 nouvelles
pages a été figé. Quatre pages satisfont la règle ciblée. V6 obtient quatre
majorités, dont trois positives et une négative, soit un rendement positif de
75 % sur ce très petit diagnostic. Les 43 autres pages forment une réserve
aléatoire explicitement disjointe (SHA-256
`76de4c52fec6258b0dcfef848ef5c571dffb94af6181cc918d522ef952e00dd5`).
Elles ne seront pas mélangées à l'enrichissement et pourront mesurer la
prévalence ou alimenter une évaluation chronologique ultérieure.

Ce premier résultat valide la séparation des flux, mais quatre documents ne
suffisent pas pour conclure sur le rendement futur. La collecte ciblée doit
continuer jusqu'à fournir au moins 19 positifs supplémentaires, afin d'atteindre
50 positifs réels au total, tandis que la réserve aléatoire reste intacte.

## Itération 13 — effet de 220 nouvelles références réelles

Après ajout de 220 consensus réels (6 positifs et 214 négatifs), le développement
contient 2 977 références : 959 positives et 2 018 négatives. La couverture
cumulée rapportée est 93,38 % et l'Average Precision hors pli globale 82,11 %.

La validation par acteurs choisit de nouveau `stable_terms_word_reserve` avec
L2=0,001 et poids positif 1. Au seuil global, elle atteint 51,87 % de précision,
96,77 % de rappel et 67,54 % de F1. Avec les seuils par source, elle atteint
67,53 % de précision, 90,20 % de rappel et 77,23 % de F1. Pour `darkweb_page`,
le seuil 0,542756 produit 29 VP, 93 FP, 2 FN et 288 VN : précision 23,77 %,
rappel 93,55 % et F1 37,91 %.

Les nouveaux négatifs améliorent la représentativité mais ne rendent pas la
frontière darkweb plus discriminante ; la précision baisse à mesure que le flux
réel remplace les corpus équilibrés. La variante à budgets de variables séparés
est également inférieure de 0,20 point de F1 global au meilleur candidat.
**Décision : aucune promotion et aucun test indépendant.** La prochaine boucle
se concentre sur l'enrichissement positif v6 déjà séparé du flux aléatoire et
sur une représentation relationnelle ou sémantique, pas sur un nouveau réglage
de seuil TF-IDF.

## Audit d'étanchéité après l'itération 13

La comparaison normalisée entre le socle d'entraînement fixe et les 2 977
documents de développement a révélé **59 textes exactement communs**, dont 43
portent également le même identifiant. Il s'agit principalement d'avis GHSA
présents à la fois parmi les positifs faibles historiques et dans le
développement. Ces textes entraient donc dans le socle de chacun des cinq plis :
les mesures OOF des itérations précédentes, particulièrement celles de la source
GitHub, sont optimistes et ne constituent pas une validation étanche. Ce constat
ne modifie ni le modèle actuellement déployé ni les résultats des tests
indépendants déjà consommés.

Le chemin `--cross-fit` exclut désormais du socle tout texte dont le contenu
normalisé existe dans le développement, avant la création des plis. Le rapport
enregistre le nombre sous
`training_documents_excluded_for_exact_development_overlap`; les fichiers
sources et reçus restent intacts. Un test de régression couvre la normalisation
et l'exclusion. Les anciens rapports restent conservés comme historique, mais
une nouvelle exécution étanche est requise avant toute comparaison ou
promotion. Une recherche de quasi-doublons et l'amélioration des groupes
historiques GitHub/darkweb restent les deux contrôles suivants.

## Itération 14 — nouvelle mesure après exclusion du chevauchement

L'exécution étanche a effectivement retiré **57 documents du socle déjà
dédupliqué** (les 59 occurrences observées lors de l'audit brut correspondaient
à 57 entrées effectives). Le candidat choisit `all_terms`, L2=0,001 et un poids
positif de 1. Au seuil global, il atteint 52,83 % de précision, 96,45 % de
rappel et 68,27 % de F1 ; l'Average Precision est 82,11 %. Avec les seuils par
source, il atteint 69,40 % de précision, 90,09 % de rappel et 78,40 % de F1.

Sur `darkweb_page`, le seuil spécifique produit 28 VP, 88 FP, 3 FN et 293 VN,
soit 24,14 % de précision, 90,32 % de rappel et 38,10 % de F1. L'étanchéité ne
provoque donc pas l'effondrement global redouté, mais la précision darkweb reste
le défaut dominant. Le gain par rapport à l'itération 13 est trop faible et il
n'existe pas encore de nouvelle évaluation indépendante : **aucune promotion**.
Le prochain effort porte sur les quasi-doublons, les groupes historiques et
l'enrichissement en vrais positifs darkweb v6, plutôt que sur un nouveau réglage
du seuil.

## Itération 15 — exclusion des quasi-doublons inter-ensembles

Le contrôle par shingles de cinq mots à 80 % de Jaccard a retiré 64 documents
supplémentaires du socle, en plus des 57 correspondances exactes. Il reste 4 813
documents dans le réentraînement final. Le candidat revient à
`stable_terms_word_reserve`, L2=0,001 et poids positif 1.

La mesure OOF étanche atteint 53,25 % de précision, 96,45 % de rappel et 68,62 %
de F1 ; l'Average Precision progresse à 82,63 %. Avec les seuils par source, le
résultat atteint 69,61 % de précision, 90,30 % de rappel et 78,62 % de F1. Pour
`darkweb_page`, le seuil spécifique donne 29 VP, 91 FP, 2 FN et 290 VN : 24,17 %
de précision, 93,55 % de rappel et 38,41 % de F1.

La suppression de la fuite ne dégrade pas les mesures et améliore légèrement
les agrégats, mais l'écart avec l'itération 14 reste faible et la précision
darkweb ne progresse pratiquement pas. **Aucune promotion et aucun test
indépendant.** Le prochain levier significatif est la qualité et la diversité
des positifs darkweb, suivie d'une validation chronologique réellement nouvelle.

## Audit après l'itération 15

Le développement est très déséquilibré par domaine : 31 positifs seulement sur
412 pages darkweb, contre 612 sur 1 161 avis GitHub et 316 sur 1 404 entrées
ransomware.live. Les pages darkweb couvrent en outre moins de douze heures de
collecte et un seul document par hôte. La mesure actuelle teste donc de nouveaux
hôtes, mais ni la dérive temporelle ni plusieurs pages d'un même service.

La sélection de la famille de variables est instable et les écarts sont
négligeables : à l'itération 15, `stable_terms_word_reserve` ne dépasse
`all_terms` que d'environ 0,03 point de F1. Les changements de vainqueur entre
les itérations 13, 14 et 15 ne démontrent donc pas une supériorité structurelle.
Une règle de simplicité ou une comparaison répétée par groupes doit précéder
tout élargissement de la grille d'hyperparamètres.

L'agrégation des seuils par source appliquait aussi dans le rapport le seuil
diagnostique lorsqu'une source n'avait pas les 20 positifs requis, tandis que
l'artefact aurait utilisé le seuil global. Les trois sources actuelles sont
éligibles : les chiffres de l'itération 15 restent donc inchangés. Le calcul est
néanmoins corrigé et un test vérifie désormais le repli vers le seuil global.

Priorités : (1) accumuler prospectivement au moins 100 positifs darkweb issus de
plusieurs périodes et plusieurs pages par hôte ; (2) réserver un bloc
chronologique jamais utilisé pour la sélection ; (3) mesurer la variance par
répétitions groupées ; (4) analyser automatiquement les faux positifs par
famille de contenu. Aucun nouveau test indépendant ni promotion avant ces
conditions.

## Application complète du plan — itération 16 prospective

Un nouvel export réel de 545 pages, disjoint du développement antérieur, a été
figé. Les 150 pages les plus récentes constituent une réserve chronologique
intacte à partir du 21 octobre 2026 09:41:12 UTC (SHA-256
`7f05baf763486b88400e855305c741bc9aea9b2a946522e3546dde3e2bcf3f59`).
Les 395 pages antérieures forment uniquement le flux de développement.

L'échantillonneur ciblé a retenu 40 pages de ce flux : 9 par expression forte
et 31 par terme ambigu accompagné d'un second signal CTI. L'annotation v6 à
trois modèles a produit 38 consensus, aucune divergence et deux abstentions de
contexte. Les consensus ajoutent 23 positifs et 15 négatifs ; le nombre de
positifs darkweb disponibles pour le développement passe ainsi de 31 à 54. La
réserve chronologique n'a reçu ni annotation ni score de sélection.

Le prochain rapport de candidat conserve maintenant chaque prédiction OOF avec
son identifiant, sa source et son groupe. Un audit reproductible calcule 1 000
rééchantillonnages par groupes, leurs intervalles à 95 %, et classe les faux
positifs par familles lexicales. L'itération 16 réentraîne actuellement le
candidat avec exclusion des doublons exacts et proches. Les objectifs de 100
positifs darkweb, de plusieurs périodes et de plusieurs pages par hôte ne sont
pas encore atteints : ce passage reste du développement et aucune promotion
n'est autorisée sur son seul résultat.

## Résultat de l'itération 16

Le développement contient 3 015 consensus, dont 54 positifs darkweb. Le socle
final comprend 4 840 documents après exclusion de 57 correspondances exactes et
75 quasi-doublons. Le candidat sélectionné utilise les budgets équilibrés de
familles, L2=0,001 et un poids positif de 1. Son seuil global donne 53,17 % de
précision, 96,54 % de rappel et 68,57 % de F1 ; l'Average Precision atteint
82,84 %.

Avec les seuils par source, le résultat global est de 68,58 % de précision,
90,22 % de rappel et 77,92 % de F1. La source darkweb atteint 49 VP, 106 FP,
5 FN et 290 VN, soit **31,61 % de précision, 90,74 % de rappel et 46,89 % de
F1**. Par rapport à l'itération 15, l'ajout prospectif améliore nettement la
précision et la F1 darkweb sans sacrifier le rappel.

Le bootstrap de 1 000 rééchantillonnages groupés estime cependant pour le
darkweb un intervalle à 95 % de 24,18–39,29 % en précision, 82,05–98,00 % en
rappel et 37,93–54,94 % en F1. L'incertitude reste donc élevée. Les faux positifs
se concentrent notamment dans le contenu technique/hébergement, les annuaires
ou places de marché, la finance/crypto, la confidentialité et le vocabulaire de
sécurité générique. **Aucune promotion et aucun test chronologique** avant
d'atteindre le support prospectif prévu.

## Correction d'interprétation après l'itération 16

Les 31,61 % de précision darkweb mélangeaient le flux représentatif historique
et les 38 documents volontairement ciblés, dont 23 positifs. Cette prévalence
enrichie ne représente pas la production et rend la comparaison directe avec
l'itération 15 invalide. Sur les 412 pages historiques uniquement, le seuil de
l'itération 16 donne 29 VP, 98 FP, 2 FN et 283 VN : 22,83 % de précision,
93,55 % de rappel et 36,71 % de F1. Le flux ciblé donne séparément 71,43 % de
précision, 86,96 % de rappel et 78,43 % de F1 ; il est utile à l'apprentissage,
pas à l'estimation de prévalence.

Le protocole est corrigé : les documents ciblés participent à l'entraînement
des plis et au réentraînement final, mais ils sont exclus du choix de la famille,
de l'hyperparamètre, des seuils et des métriques de sélection. Leur diagnostic
reste séparé. L'itération 17 refait la sélection sur le seul flux représentatif.
L'interprétation optimiste précédente reste documentée pour la traçabilité mais
ne justifie aucune promotion.

## Itération 17 — sélection représentative corrigée

La sélection a été rejouée sur **2 977 documents représentatifs**. Les 38
documents issus de l'échantillonnage ciblé participent toujours à
l'apprentissage, mais plus au choix du modèle, du seuil ou aux métriques de
référence. Le réentraînement final utilise 4 840 documents après exclusion de
57 doublons exacts et 75 quasi-doublons. Le modèle retenu est
`balanced_feature_families`, avec L2=0,001 et un poids positif de 1.

Au seuil global, la sélection représentative atteint 52,79 % de précision,
96,66 % de rappel et 68,29 % de F1 ; l'Average Precision est 83,11 %. Avec les
seuils par source, elle atteint **70,10 % de précision, 90,20 % de rappel et
78,89 % de F1** (865 VP, 369 FP, 94 FN, 1 649 VN). Le bootstrap groupé à 1 000
répétitions donne des intervalles à 95 % de 64,69–75,21 % en précision,
87,90–92,47 % en rappel et 75,04–82,33 % en F1.

Sur le darkweb représentatif uniquement, le résultat est de **27,18 % de
précision, 90,32 % de rappel et 41,79 % de F1** (28 VP, 75 FP, 3 FN, 306 VN).
Les intervalles groupés restent larges : 19,10–35,83 % en précision,
79,49–100 % en rappel et 31,37–51,52 % en F1. Par rapport à l'itération 15,
les faux positifs darkweb passent de 91 à 75 et la F1 de 38,41 % à 41,79 %,
mais l'incertitude ne démontre pas encore un gain robuste.

Le flux ciblé est publié séparément comme diagnostic : 76,00 % de précision,
82,61 % de rappel et 79,17 % de F1. Il confirme l'utilité pédagogique des
exemples ajoutés sans être présenté comme une mesure de production. **Aucune
promotion** : la réserve chronologique de 150 pages reste intacte et le support
darkweb représentatif demeure trop faible. La prochaine boucle doit augmenter
les positifs représentatifs multi-périodes et multi-pages par hôte, puis refaire
la même sélection gelée avant un unique test chronologique.

## Audit après l'itération 17

L'intégrité immédiate est correcte : les 150 pages de la réserve chronologique
sont toujours présentes avec le SHA-256 annoncé, sans identifiant ni texte
normalisé commun avec l'entrée de développement. Les 3 015 prédictions OOF ont
des identifiants, dates et groupes renseignés. Les 39 tests Python et l'ensemble
des tests Go passent ; `git diff --check` ne signale aucune erreur.

Le principal risque reste statistique. Le seuil darkweb est choisi et mesuré
sur seulement **31 positifs représentatifs**. Le minimum technique de 20 rend
donc cette source éligible trop tôt, et les intervalles larges confirment
l'instabilité. En outre, le choix parmi 24 configurations et l'optimisation des
seuils utilisent les mêmes prédictions OOF : les métriques publiées sont des
métriques de sélection, pas une estimation indépendante. Le bootstrap actuel
conditionne sur le modèle et les seuils déjà choisis ; il ne mesure pas
l'incertitude supplémentaire introduite par toute la procédure de sélection.

La calibration de Platt est aussi ajustée sur les 3 015 prédictions, y compris
les 38 documents ciblés dont la prévalence positive est artificiellement forte.
Cela ne change pas le verdict, fondé sur le score brut, mais peut biaiser
`calibrated_score` si cette valeur est consommée comme probabilité. Elle doit
être ajustée uniquement sur le flux représentatif, ou faire l'objet d'une
calibration séparée par source avec un support suffisant.

Enfin, le corpus darkweb représentatif ne contient encore qu'une page par
groupe/hôte dans cet export. Le groupement empêche les fuites connues, mais ne
mesure pas la corrélation entre plusieurs pages d'un même service ni la dérive
sur plusieurs fenêtres temporelles. La fusion de quasi-doublons compare chaque
nouveau document au seul représentant de son cluster ; elle est dépendante de
l'ordre et peut manquer une chaîne de similarités transitives.

Plan prioritaire : (1) relever l'éligibilité d'un seuil par source à au moins
100 positifs représentatifs et conserver le seuil global jusque-là ; (2)
calibrer sur le seul flux représentatif ; (3) collecter plusieurs fenêtres et
plusieurs pages par hôte ; (4) rendre la déduplication proche transitive ; (5)
après gel complet du modèle et des seuils, annoter puis lire une seule fois la
réserve chronologique. La collecte supplémentaire doit précéder une nouvelle
extension de la grille ou un modèle plus complexe.

## Application de l'audit — itération 18

Les trois corrections structurelles sont appliquées. La calibration de Platt
utilise maintenant uniquement les 2 977 documents du flux représentatif. Un
seuil spécifique exige désormais au moins 100 positifs représentatifs ; avec
31 positifs, le darkweb utilise donc volontairement le seuil global. La
déduplication proche construit des composantes connexes transitives à partir
d'un index inversé de shingles, avec un test couvrant une chaîne A–B–C.

La déduplication transitive retire sept groupes supplémentaires : le socle
final passe de 4 840 à 4 833 documents. L'itération 18 sélectionne
`stable_terms_word_reserve`, L2=0,001 et poids positif 1. Au seuil global, elle
atteint 53,25 % de précision, 96,45 % de rappel et 68,62 % de F1 ; l'Average
Precision est 82,88 %.

Les seuils spécialisés restent actifs pour GitHub et ransomware.live, qui ont
respectivement 612 et 316 positifs. Le résultat représentatif agrégé est alors
de 64,11 % de précision, 90,72 % de rappel et 75,13 % de F1. Cette baisse par
rapport à l'itération 17 vient de la politique prudente appliquée au darkweb,
pas d'une régression comparable du classement : au seuil global, le darkweb
donne 31 VP, 196 FP, 0 FN et 185 VN, soit 13,66 % de précision et 24,03 % de F1.
Le seuil darkweb diagnostique mais non déployable donnerait 26,61 % de
précision, 93,55 % de rappel et 41,43 % de F1.

**Aucune promotion.** Le résultat montre explicitement le coût de ne plus
sur-ajuster un seuil avec 31 positifs. Le prochain gain dépend désormais des
données : il faut atteindre 100 positifs darkweb représentatifs antérieurs au
gel du test, ou constituer une nouvelle chronologie complète avec une nouvelle
réserve postérieure. Les 150 pages de la réserve existante restent intactes et
ne sont toujours ni annotées ni lues par la sélection.

## Audit après l'itération 18

Les corrections sont fonctionnelles et les suites Go/Python restent vertes.
La calibration représentative améliore nettement les prédictions OOF globales :
le Brier passe de 0,13860 à 0,11664, la log-loss de 0,43979 à 0,35898 et l'ECE
à dix classes de 0,14784 à 0,03250. Elle reste toutefois imparfaite par source :
l'ECE darkweb est encore 0,18583 et le Brier ransomware.live se dégrade
légèrement de 0,09410 à 0,09927. Une calibration propre à une source ne devra
être envisagée qu'avec un support indépendant suffisant.

Une incohérence de présentation subsiste dans l'artefact candidat :
`validation.objective_met=true` décrit le seuil global agrégé sélectionné,
alors que la politique effective avec repli darkweb produit seulement 13,66 %
de précision sur cette source. Ce champ ne doit pas être interprété comme une
validation de chaque source ni comme une autorisation de promotion. Un futur
rapport doit distinguer explicitement l'objectif global de sélection, l'objectif
de la politique effective et les contraintes par source.

Le bootstrap reste conditionnel à la configuration déjà choisie et ne couvre
pas l'incertitude de sélection parmi 24 variantes. La calibration est elle-même
ajustée et évaluée sur les mêmes prédictions OOF ; ses chiffres sont donc des
diagnostics de développement. Une calibration croisée imbriquée ou un bloc de
calibration représentatif séparé sera nécessaire avant production.

Enfin, la fermeture transitive corrige la dépendance à l'ordre, mais une chaîne
de documents peut maintenant réunir deux extrémités qui ne sont pas directement
similaires. Il faut enregistrer la taille et la similarité minimale de chaque
composante, puis plafonner ou auditer automatiquement les composantes anormales.
Les fichiers de travail NLP restent non suivis par Git conformément à l'absence
de commit demandée ; ils doivent être inclus explicitement dans une future
revue avant intégration pour éviter de perdre le protocole et ses tests.

## Itération 19 — intégrité du rapport et calibration imbriquée

Le rapport distingue maintenant trois notions : l'objectif global de
sélection, l'objectif global de la politique effective et l'atteinte des mêmes
cibles pour chaque source. Sur cette itération, les deux objectifs globaux sont
atteints, mais l'objectif toutes sources est explicitement faux : GitHub et
ransomware.live passent, le darkweb échoue. Le champ historique
`validation.objective_met` ne doit donc plus être lu isolément.

Chaque prédiction OOF conserve désormais son pli. L'audit ajuste cinq
calibrateurs sur quatre plis et mesure chaque calibrateur sur le cinquième. Ce
diagnostic imbriqué donne un Brier de **0,11703**, une log-loss de **0,36049**
et un ECE à dix classes de **0,03135**. Il est très proche du diagnostic ajusté
sur tout l'OOF et réduit le risque que le bon résultat de calibration soit un
simple artefact de réutilisation des mêmes exemples.

La déduplication publie 1 954 composantes, dont 90 multi-documents. La plus
grande contient 30 documents et deux composantes dépendent réellement d'une
chaîne transitive. L'audit a également révélé que l'ordre d'itération d'une map
Go pouvait changer l'ordre des unions, puis légèrement les résultats
numériques. Les candidats sont maintenant triés et les unions choisissent une
racine canonique, afin de restaurer la reproductibilité.

L'itération 19 reste proche de l'itération 18 : AP 82,89 %, et politique
effective représentative à 63,88 % de précision, 90,72 % de rappel et 74,97 %
de F1. Le darkweb au seuil global reste à 13,60 % de précision et 100 % de
rappel. **Aucune promotion** : ces modifications rendent la mesure plus fiable,
mais n'ajoutent toujours pas les positifs darkweb représentatifs manquants.

## Audit après l'itération 19

Un biais de sélection plus important a été quantifié dans les annotations. Sur
3 226 documents, 207 restent en désaccord et quatre en abstention. La couverture
de consensus atteint 95,71 % pour GitHub et 96,63 % pour ransomware.live, mais
seulement **80,36 % pour le darkweb** : 106 désaccords et quatre abstentions y
sont exclus. Les métriques darkweb portent donc sur les cas les plus faciles et
ne décrivent pas tout le flux représentatif. Ce biais de non-réponse doit être
traité avant toute promotion, indépendamment du nombre de positifs.

L'artefact de l'itération 19 a par ailleurs été écrit quinze secondes avant le
correctif rendant les unions déterministes. Ses métriques restent un diagnostic
valide de cette exécution, mais elles ne prouvent pas la reproductibilité du
code corrigé. Une itération 20 identique doit produire deux rapports successifs
dont les champs numériques, hors version et horodatage, ont la même empreinte.

Les tests répétés vingt fois passent. La calibration imbriquée et la distinction
des objectifs restent correctes. Les prochains contrôles bloquants sont donc :
(1) réannoter automatiquement les 211 cas non résolus avec un panel figé et une
règle de majorité préenregistrée ; (2) publier les membres des deux composantes
transitives et de la composante de taille 30 ; (3) vérifier deux exécutions
reproductibles après le correctif ; (4) recalculer les métriques en incluant les
cas nouvellement arbitrés. La réserve chronologique reste exclue de ces étapes.

## Itération 20 — résolution automatique des non-réponses et reproductibilité

Les 211 annotations non résolues de l'itération 16 ont été soumises au panel v6
figé (`qwen3:8b`, `gemma4:e2b`, `llama3.2:3b`). Les 633 votes ont produit 204
nouveaux consensus (172 positifs et 32 négatifs), quatre désaccords et trois
abstentions. La couverture du développement passe ainsi de 3 015/3 226
(93,46 %) à **3 219/3 226 (99,78 %)**, sans revue humaine et sans lecture de la
réserve chronologique.

Deux entraînements strictement identiques ont ensuite été exécutés après le
correctif de déterminisme. Leurs rapports de sélection sont identiques octet
pour octet (SHA-256
`3c76f85bf89ca78a42dbff4b6437416a6f3cbd3a88a461d0fcf61057ea90e397`).
Les modèles ne diffèrent que par leur identifiant de version horodaté ; tous les
paramètres numériques et métadonnées d'évaluation sont identiques. Le défaut de
reproductibilité observé avant l'itération 20 est donc corrigé.

La variante retenue utilise les familles de caractéristiques équilibrées,
`L2=0,001` et un poids positif de 1,5. Son AP OOF est de **81,17 %**. Avec la
politique effective, l'ensemble des 3 219 consensus atteint **66,67 % de
précision, 90,29 % de rappel et 76,70 % de F1**. Sur le flux représentatif, les
résultats sont 66,54 %, 90,19 % et 76,58 %. Le darkweb représentatif progresse à
**42,34 % de précision, 91,30 % de rappel et 57,85 % de F1** ; son intervalle
bootstrap de précision à 95 % est [36,08 %, 48,15 %].

La couverture n'est désormais plus le problème principal. La contrainte globale
est satisfaite, mais la contrainte par source reste fausse car la précision
darkweb demeure sous 50 %, y compris à la borne haute de l'intervalle bootstrap.
La calibration imbriquée se dégrade légèrement (Brier 0,13501, log-loss 0,40890,
ECE 0,04719), ce qui confirme un changement de distribution après résolution
des cas difficiles. **Aucune promotion** n'est effectuée. La prochaine boucle
doit réduire les faux positifs darkweb, principalement les pages de répertoire,
d'hébergement, de finance/crypto et les descriptions de victimes sans preuve
explicite d'incident, tout en conservant un rappel supérieur ou égal à 90 %.

## Audit après l’itération 20

L’itération résout le biais de non-réponse mesurable : 99,78 % des documents ont
maintenant un consensus. Les sept cas restants sont tous des pages darkweb
(quatre désaccords et trois abstentions), donc le résidu est faible mais encore
concentré sur la source la plus difficile. Les deux exécutions identiques
confirment la reproductibilité du rapport ; le test indépendant et la réserve
chronologique n’ont pas été lus.

Le blocage principal est désormais la séparabilité des classes par source. Avec
115 positifs darkweb représentatifs, le seuil spécifique est admissible mais ne
trouve aucun point satisfaisant simultanément précision ≥ 50 % et rappel ≥ 90 %.
Au point retenu, il reste 143 faux positifs contre 105 vrais positifs. La borne
haute bootstrap de la précision (48,15 %) demeure sous la cible, ce qui rend
l’échec statistiquement plus convaincant qu’aux itérations précédentes.

L’analyse d’erreur montre une forte concentration dans les répertoires et places
de marché, les pages techniques/hébergement, la finance/crypto, les textes de
confidentialité et le vocabulaire de sécurité générique. Les catégories se
chevauchent et ne doivent pas être additionnées. Le rapport publie la liste des
faux positifs mais pas celle des faux négatifs : ce manque limite l’audit du
compromis de rappel et doit être corrigé avant la prochaine comparaison.

Les risques méthodologiques restants sont : (1) les seuils et la variante sont
choisis puis décrits sur le même OOF, ce qui laisse un optimisme de sélection ;
(2) les 204 arbitrages viennent du même panel automatique et ne constituent pas
une vérité indépendante ; (3) la calibration imbriquée se dégrade après ajout
des cas difficiles ; (4) la grande composante de quasi-doublons contient 30
documents et nécessite encore une mesure de similarité minimale/diamètre, pas
seulement la publication des identifiants ; (5) l’amélioration n’est pas encore
confirmée sur un futur flux chronologique scellé.

Décision : conserver le modèle de production. La prochaine itération doit
ajouter au rapport les faux négatifs, mesurer la stabilité des seuils par
bootstrap imbriqué, auditer le diamètre des composantes, puis comparer une
famille de caractéristiques conçue pour les faux positifs darkweb. La réserve
chronologique ne sera ouverte qu’après gel complet du candidat et du protocole.

## Itération 21 — application du plan post-audit

Le rapport d'audit publie désormais les faux négatifs complets et une estimation
bootstrap imbriquée de la stabilité du seuil darkweb. La déduplication mesure
également la similarité minimale et le diamètre maximal des composantes. La plus
grande composante a une similarité minimale de 0,7359 (diamètre 0,2641), ce qui
confirme un effet de chaîne réel mais limité ; cette mesure sera conservée comme
alarme de dérive.

Quatre indicateurs structurels, ajustés uniquement sur les données de
développement, ont été ajoutés à la famille candidate : gabarits de place de
marché, mentions institutionnelles, gabarits d'hébergement et preuves explicites
de compromission. Une configuration réservant davantage de capacité à ces
indicateurs a aussi été comparée. La sélection conserve toutefois la famille
équilibrée existante, avec `L2=0,001` et un poids positif de 1. Le nouveau
candidat atteint une AP OOF de 81,69 %, mais sa politique effective ne progresse
pas : flux représentatif P 66,43 %, R 90,27 %, F1 76,54 % ; darkweb
représentatif P 42,23 %, R 92,17 %, F1 57,92 %.

Le bootstrap imbriqué du seuil darkweb donne un intervalle [0,3902, 0,5454]. La
cible conjointe précision ≥ 50 % / rappel ≥ 90 % n'est atteinte que dans 6,5 %
des rééchantillonnages. La calibration s'améliore légèrement (Brier 0,13310,
log-loss 0,40406, ECE 0,04541), sans résoudre la séparabilité darkweb. Les 111
faux négatifs sont maintenant disponibles dans l'artefact d'audit.

Décision : **aucune promotion**. Les indicateurs proposés ne réduisent pas les
faux positifs darkweb et le nouveau candidat est légèrement inférieur à
l'itération 20 sur la précision/F1 effective. La réserve chronologique reste
scellée, car ouvrir un test final pour un candidat qui échoue déjà les critères
de développement consommerait inutilement ce test.

## Audit après l’itération 21

L’échec des indicateurs structurels apporte une information utile : les indices
isolés ne portent pas un sens négatif stable. Dans le modèle final,
`marketplace_boilerplate` reçoit même un poids positif (+0,245), car une place de
marché peut aussi vendre des accès ou des données. `hosting_boilerplate` est
quasi neutre, `corporate_boilerplate` est négatif et `compromise_evidence` est
nettement positif. Une simple liste de mots négatifs ne séparera donc pas les
pages darkweb ; il faut représenter les interactions entre le type de page et
la présence d’une preuve CTI.

La nouvelle configuration réservée aux erreurs darkweb se classe cinquième des
variantes faisables (P 54,65 % au seuil global de sélection), derrière la famille
équilibrée (P 55,10 %). L’amélioration d’AP et de calibration ne se transforme
pas en amélioration au point opérationnel. Le seuil darkweb est instable et la
cible conjointe ne passe que 6,5 % des bootstraps : ajuster encore le seuil serait
une optimisation fragile, pas une correction du modèle.

Les nouveaux arbitrages sont fortement positifs : 84/103 pour le darkweb,
49/52 pour GitHub et 39/49 pour ransomware.live. Cela est cohérent avec un pool
constitué des anciens désaccords, mais ce pool n’est pas un échantillon de
prévalence. Il améliore la couverture des frontières sans fournir une estimation
indépendante de la fréquence naturelle des classes. Les sept cas encore exclus
restent tous darkweb.

Une ambiguïté de rapport subsiste : le minimum de 100 positifs concerne
l’éligibilité des seuils spécifiques, tandis que la sélection globale protège le
rappel des sources dès 20 positifs. Ces deux règles doivent recevoir des noms
séparés dans l’artefact afin d’éviter une interprétation erronée.

La prochaine expérience à plus fort rendement est une architecture hiérarchique
figée : tronc lexical commun, puis petite tête logistique propre au darkweb,
comparée à une tête partagée. Elle devra être évaluée avec les mêmes plis de
groupes et une sélection imbriquée. Des interactions explicites
`type_de_page × preuve_CTI` peuvent constituer une variante moins complexe. Une
nouvelle collecte de négatifs darkweb représentatifs reste prioritaire ; ajouter
encore des avis GitHub ou des annonces ransomware apportera peu au défaut actuel.

## Itération 22 — interactions type de page × preuve CTI

Des interactions explicites ont été ajoutées entre trois types de gabarits
(marketplace, institutionnel, hébergement) et la présence ou l'absence d'une
preuve CTI explicite. Les deux minima auparavant ambigus sont aussi publiés
séparément : 100 positifs pour rendre un seuil de source admissible, 20 positifs
pour appliquer la contrainte de rappel par source pendant la sélection globale.

La famille équilibrée reste sélectionnée. Les interactions ne changent pas le
résultat darkweb : P 42,23 %, R 92,17 %, F1 57,92 %. Le flux représentatif baisse
très légèrement à P 66,36 %, R 90,36 %, F1 76,53 % et l'AP passe de 81,69 % à
81,68 %. La cible darkweb ne passe que 6,7 % des bootstraps. Plusieurs
interactions avec preuve sont trop rares pour survivre au minimum de fréquence ;
les autres reçoivent de faibles poids. Cette représentation ne résout donc pas
le chevauchement lexical.

Décision : aucune promotion et réserve chronologique toujours scellée. La voie
suivante doit être évaluée comme expérience architecturale : tête logistique
propre au darkweb entraînée uniquement dans chaque pli, avec encodeur ajusté sans
le pli tenu à l'écart. Elle ne pourra devenir candidate de production qu'après
ajout explicite et testé de la source dans le contrat d'inférence ; en attendant,
elle restera un diagnostic hors production.

## Audit après l’itération 22

Le contrat d’inférence a été revérifié dans le code. Contrairement à la réserve
formulée à la fin de l’itération 22, le chemin de production transmet déjà
`ArticleContent.SourceType` à `ClassifySource`, avec repli des anciens documents
sans source vers `darkweb_page`. Une tête propre à la source est donc compatible
avec le contrat d’entrée actuel ; elle demanderait surtout une extension
rétrocompatible du schéma du modèle (`source_heads`) et du classifieur.

Le rayon d’impact reste modéré mais réel : persistance/validation du modèle,
construction de `Classifier`, choix du score et de la calibration dans
`ClassifySource`, tests de compatibilité des anciens modèles, outils
d’évaluation et campagnes de confirmation. Ces derniers utilisent parfois
`Classify(text)` sans source ; ils devraient lire et transmettre `source` pour
ne pas évaluer silencieusement la tête globale. Les seuils propres aux sources
montrent que ce routage est déjà testé en production, mais pas encore pour des
poids propres aux sources.

Les plis sont correctement équilibrés : chaque pli contient 22 à 25 positifs et
78 à 83 négatifs darkweb. Une tête darkweb entraînée sur quatre plis disposerait
donc d’environ 90 positifs et 320 négatifs par ajustement. C’est suffisant pour
un diagnostic fortement régularisé, mais encore faible pour une grande tête ou
une recherche large d’hyperparamètres. La comparaison doit rester limitée et
préenregistrée.

L’analyse des erreurs confirme que le problème n’est pas exclusivement darkweb :
les 110 faux négatifs représentatifs se répartissent en 65 GitHub, 35
ransomware.live et 10 darkweb, tandis que les 526 faux positifs comprennent 256
GitHub, 153 darkweb et 117 ransomware.live. Les seuils maintiennent néanmoins
les deux premières sources au-dessus des objectifs ; la tête spécialisée doit
donc être limitée au darkweb pour ne pas dégrader ces résultats.

Enfin, l’itération 22 ne fournit pas une nouvelle preuve indépendante : ses
interactions ont été conçues après lecture des erreurs puis mesurées sur le même
OOF de développement. La légère différence avec l’itération 21 est un diagnostic
de développement, pas un gain généralisable. La réserve chronologique doit
rester scellée jusqu’à ce qu’une tête darkweb franchisse les contraintes avec
une marge bootstrap suffisante et une sélection imbriquée.

## Itération 23 — tête logistique darkweb

Une tête darkweb fixe et préenregistrée a été évaluée. Elle partage le
vectoriseur ajusté dans chaque pli, mais ses poids sont entraînés uniquement sur
les documents darkweb des quatre plis d'apprentissage. Aucun hyperparamètre de
cette tête n'a été choisi sur ses résultats : `L2=0,1`, pondération équilibrée.
Le schéma de modèle accepte une map optionnelle `source_heads`; les anciens
modèles restent compatibles et `ClassifySource` conserve la tête globale pour
les sources absentes ou une dimension incohérente. Toute la suite Go et les 41
tests Python passent.

Le résultat est inférieur à la tête partagée. Sur le darkweb représentatif :
P 41,43 %, R 90,43 %, F1 56,83 %, contre P 42,23 %, R 92,17 %, F1 57,92 % à
l'itération 22. La borne haute bootstrap de précision est 47,39 % et la cible
conjointe ne passe que 7,8 % des bootstraps. Le flux représentatif global baisse
à P 66,23 %, R 90,19 %, F1 76,38 % ; AP 81,49 %. La calibration imbriquée se
dégrade également (Brier 0,13893, log-loss 0,42004, ECE 0,05096).

La spécialisation de la tête seule ne compense donc pas la faiblesse des données
et de la représentation darkweb. **Aucune promotion** et aucun test sur la
réserve chronologique. Les poids propres à la source restent expérimentaux ; ils
ne doivent pas devenir le comportement par défaut des futurs entraînements sans
option explicite.

## Audit après l’itération 23

Le verdict de non-promotion est confirmé : la tête darkweb régresse sur le flux
représentatif et la borne haute de précision darkweb (47,39 %) reste sous la
cible de 50 %. Il n’existe donc aucune justification pour consommer la réserve
chronologique. Son empreinte reste
`7f05baf763486b88400e855305c741bc9aea9b2a946522e3546dde3e2bcf3f59`.

L’audit méthodologique relève cinq points à corriger avant une nouvelle
comparaison :

1. L’expérience de tête spécialisée n’est pas entièrement imbriquée. La tête est
   bien ajustée dans chaque pli et son hyperparamètre est fixe, mais la
   configuration du vectoriseur partagé avait déjà été choisie sur le même OOF.
   Le résultat est donc un diagnostic de développement, pas une estimation
   indépendante de généralisation.
2. `threshold_stability` choisit puis mesure le seuil sur chaque même
   rééchantillonnage bootstrap. Il mesure la variabilité par resubstitution ; ce
   n’est pas une validation bootstrap imbriquée. Le taux de passage de 7,8 % est
   descriptif et optimiste. Les mentions antérieures de « bootstrap imbriqué »
   dans ce journal doivent être lues avec cette correction.
3. La calibration globale de l’itération 23 est ajustée sur des scores hybrides :
   scores de la tête darkweb pour cette source, scores de la tête partagée pour
   les autres. La calibration darkweb propre remplace bien cette calibration à
   l’inférence, mais GitHub, ransomware.live et les sources inconnues héritent
   d’un calibrateur global contaminé par une autre échelle de scores. Le
   calibrateur global doit être ajusté sur les OOF de la tête partagée ; les
   calibrateurs propres doivent rester associés à chaque tête.
4. Une tête de source dont la dimension diffère du vecteur est ignorée
   silencieusement, alors que son seuil spécifique continue d’être appliqué à la
   tête partagée. Un artefact incohérent pourrait donc produire une politique
   hybride non évaluée. Le chargement doit valider vocabulaire, IDF, poids
   globaux, poids des têtes, seuils et paramètres de calibration, puis retourner
   une erreur explicite en cas d’incohérence.
5. Le résumé `validation` de l’artefact décrit la sélection de la tête partagée
   (`objective_met=true`, P 55,10 %, R 96,55 %) et non la politique hybride
   effective (P 66,23 %, R 90,19 %), laquelle échoue la contrainte darkweb. Le
   schéma doit distinguer explicitement métriques de sélection, métriques de la
   politique effective et éligibilité à la promotion.

Le rayon d’impact contient aussi un défaut de mesure :
`internal/confirmation/run.go` et certains outils utilisent encore
`Classify(text)` sans transmettre la source. Ils évalueraient la tête partagée
même pour un artefact à têtes spécialisées. Tous les chemins de confirmation et
d’audit doivent soit transmettre la source, soit rejeter une évaluation qui la
perd.

Priorité recommandée : corriger d’abord ces défauts d’évaluation et de contrat,
ajouter leurs tests de régression, puis constituer un nouveau lot prospectif de
développement darkweb. La prochaine comparaison devra utiliser une sélection
extérieure réellement imbriquée ou ce nouveau lot gelé. Continuer à concevoir et
mesurer des variantes sur le même OOF augmenterait surtout le surajustement de
la procédure. La piste à meilleur rendement reste ensuite une représentation
préentraînée compacte ou des embeddings gelés avec une petite tête, plutôt
qu’une nouvelle tête lexicale sur les mêmes données.

## Itération 24 — application des corrections d’audit

Les défauts de contrat et de mesure relevés après l’itération 23 ont été
corrigés sans réentraîner ni promouvoir de modèle. Le chargement et la
sauvegarde valident désormais les dimensions du vocabulaire, de l’IDF, des
poids partagés et des têtes, les indices, les seuils, les calibrateurs et les
valeurs non finies. Une tête incompatible ne peut plus conserver son seuil tout
en utilisant silencieusement le score partagé.

Le calibrateur global d’un candidat à tête spécialisée est maintenant ajusté
uniquement sur les OOF de la tête partagée. La calibration propre au darkweb
reste attachée à cette tête. Le résumé de validation distingue aussi les
métriques de sélection des métriques de la politique effective et expose une
éligibilité de promotion séparée.

La confirmation finale transporte maintenant `source` dans son CSV scellé,
vérifie que la source n’a pas changé, l’exige lorsqu’un candidat possède une
tête ou un seuil propre à une source, et calcule la matrice de confusion à partir
de `ClassifySource`. Elle mesure donc réellement la politique exécutée plutôt
qu’un seuil global appliqué à tous les scores.

Enfin, la stabilité du seuil utilise désormais une sélection sur les groupes
bootstrap *in-bag* et une mesure sur les groupes *out-of-bag*. Appliquée aux
prédictions figées de l’itération 23, cette mesure honnête donne un taux de
passage darkweb de **0,6 %**, contre 7,8 % avec la resubstitution antérieure.
L’intervalle de précision OOB est [33,33 %, 51,85 %] et celui du rappel
[75,61 %, 100 %]. Ce résultat renforce la décision de non-promotion ; il ne
constitue pas une nouvelle comparaison de modèle.

La suite Go complète et les 41 tests Python passent. L’artefact corrigé est
`iteration-24-corrected-oof-audit.json`. La réserve chronologique reste scellée
et son empreinte est inchangée.

## Itération 25 — nouveau flux prospectif darkweb

Un nouvel export aveugle de la vraie pipeline a été créé le 22 octobre, sans
verdict NLP et en excluant les 3 226 textes du développement précédent. Il
contient 1 396 pages darkweb distinctes. La séparation chronologique a été gelée
avant annotation : 1 096 documents pour le développement prospectif et les 300
plus récents pour une nouvelle réserve. Cette réserve n’est pas lue pendant le
développement ; son empreinte est
`bb7537350df5eeb22971c2c60b99cf3ffb87269747868c947c71d89b5d6a6b86`.

L’annotation automatique v6 par majorité de trois modèles locaux figés est en
cours sur les 1 096 documents de développement. Les textes sont fournis en
entier, les preuves sont reconstruites par le programme, aucun score du NLP
n’entre dans les prompts et aucune revue humaine n’est prévue. Après consensus,
le lot servira d’abord à une comparaison prospective du modèle lexical actuel,
puis à une expérience d’embeddings compacts gelés avec petite tête logistique.
La réserve historique et la nouvelle réserve chronologique restent scellées
jusqu’au gel d’un candidat qui franchit les critères de développement.
