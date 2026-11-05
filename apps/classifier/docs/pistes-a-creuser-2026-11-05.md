# Pistes à creuser (2026-11-05)

Issues ouvertes pendant l'audit de poids (`cmd/auditweights`) et la relecture de
l'historique CV (`docs/cv-*.md`, `model.json.evaluation.json`). La piste du
seuil par source (écart de précision `github-advisories` 0,70 vs
`ransomware.live` 0,92) est traitée séparément — voir message de session, pas
ce fichier. Ce qui suit est resté sans décision.

## 1. Comparaison directe encodeur multilingue vs meilleur candidat TF-IDF

`docs/cv-embeddinggemma-20261018.md` confirme sur 3 graines indépendantes
(20261016/17/18) qu'EmbeddingGemma gagne net sur Nomic aux rappels élevés
(0,65-0,75), avec perte au contraire aux rappels bas. Mais **aucune comparaison
n'a jamais été faite contre le meilleur candidat TF-IDF actuel** (celui promu
en production) — seulement encodeur-vs-encodeur. Tant que cette comparaison
n'existe pas, on ne sait pas si le gain vaut le coût d'infra (encodeur figé,
cache, dépendance supplémentaire) par rapport à rester en TF-IDF pur.

À faire : relancer le protocole `cv-embeddinggemma` avec le candidat TF-IDF
de référence actuel comme troisième colonne de comparaison, même graines,
même familles de plis figées (`data/reviewed-template-fold-families-20261014.json`).

## 2. Cluster forums non-anglais : gain ciblé mais pas net au niveau modèle

`docs/cv-analyse-fp-fn-20261018.md` : `--script-reserve` (250 places réservées
aux caractéristiques non latines) fait remonter le score de 7 FN sur 8 du
cluster multilingue, mais la matrice de confusion globale ne bouge quasiment
pas (41/36 et 37/37 vs 41/36 et 37/37) — le gain est compensé ailleurs dans
les 573 documents d'entraînement. Déjà conclu « non concluant » deux fois
(`cv-reserve-ecritures-20261015.md`, puis `cv-analyse-fp-fn-20261018.md`).

Piste non essayée : au lieu d'une réserve de vocabulaire généraliste, tester
un `min-df` plus bas spécifiquement pour les tokens non-ASCII (plutôt qu'une
réserve de places dans le budget chi-deux), pour voir si le mécanisme de
rejet est vraiment la compétition chi-deux ou le plancher `min-df` lui-même.

## 3. Deux FN sur contenu quasi vide (pages de connexion) : problème de collecte, pas de modèle

Les deux "Black Hat Chat" dans le cluster FN (`cv-analyse-fp-fn-20261018.md`)
sont des pages de connexion de ~1200 caractères, sans texte à apprendre. Si
ces documents restent dans les jeux d'entraînement/test, ils plafonnent le
rappel mesurable sans qu'aucun levier lexical ou d'encodeur ne puisse agir.

À transmettre au crawler/scraper plutôt qu'au NLP : soit obtenir un accès
authentifié à ces pages, soit les exclure explicitement du calcul de rappel
(comme `bootstrap.IsLinkDirectory` exclut déjà les pages d'annuaire de liens,
cf. `cmd/main/main.go`).

## 4. `min-df` par défaut en production : écart jamais audité

L'audit initial (session précédente) a noté que `cmd/train/main.go` appelle
`tfidf.Fit` (minimum de fréquence documentaire = 1), alors que les runs CV
documentés dans `docs/cv-*.md` utilisent systématiquement `min-df 10`. Le
modèle de production (`provisional-oof-advisories-20261020T103652Z`) vient
d'un pipeline OOF différent (`cvclassweights`), dont le `min-df` effectif n'a
pas été vérifié directement dans `model.json`. À clarifier : quel `min-df` a
réellement produit le vocabulaire de production, et si ce n'est pas 10,
pourquoi l'écart avec les runs CV documentés n'a jamais été discuté.

## 5. Relier l'audit de poids à un vrai jeu de validation stratifié par source

`cmd/auditweights -eval`/`-ablate` n'a été testé qu'avec un CSV synthétique
(smoke test). Pour relier les termes à risque élevé repérés dans le top 200
(`c_xxxx`, et les stopwords à poids fort `is`/`in`/`english`/`sign`/`please`)
à un résultat mesuré, il faut un vrai CSV `id,text,label` stratifié par
source — notamment un sous-ensemble `github-advisories` pur, pour vérifier si
ces features à risque sont surreprésentées dans ses faux positifs. Bloqué
par l'absence de texte joint dans `data/reviewed_labels.csv` (qui n'a que
`id,label,source`) : il faut retrouver/reconstituer le texte source de ces
IDs, probablement via `data/iteration-2026-10-27-source-refresh/` qui semble
contenir des exports `github-advisories` plus récents.

## 6. Chantier `iteration-2026-10-27-source-refresh` : pas encore conclu

Ce dossier (postérieur à la promotion du 20/10) contient déjà des audits
`security-stackexchange`, `ransomware-live`, `official-cti-with-cisa` —
diversification de sources en cours, sans doc de synthèse ni décision actée
pour l'instant (aucun fichier `.md` de conclusion, contrairement aux chantiers
d'octobre). Vu le point sur le seuil par source, il serait plus efficace de
concentrer ce chantier sur `github-advisories` spécifiquement plutôt que de
diversifier uniformément — mais ça reste une priorisation à confirmer, pas
une conclusion déjà actée comme les points 1-3 ci-dessus.

## 7. `github-advisories` n'a aucun chemin d'ingestion vers la collection live `article`

Vérifié en base dev (`mongodb_service`, collection `article`, seule collection
existante) : `source_type` ne contient que `darkweb_page` (7583) et `null`
legacy (102) — zéro document `github-advisories`/`advisory`, et aucune
collection `focus_match`. Le service `scraper` tournant en continu
(`docker-compose.yaml`) n'appelle que `sites.RunDarkwebScraper()`
(`apps/scraper/cmd/main.go`) sur un ticker. `collectadvisories` (ainsi que
`collectcti`, `collecteval`, `collectransomwarelive`, `collectreleases`,
`collectvalidation`) sont des binaires CLI autonomes sous `cmd/`, jamais
invoqués par un cron, un service docker-compose ou un workflow CI — recherche
confirmée sans résultat dans `.github/workflows/` et le reste du repo. Le
commentaire d'en-tête de `collectadvisories/main.go` le confirme explicitement :
« collects a bounded, unscored reserve of public advisories... never writes
production ».

Conséquence : les 151 documents `github-advisories` de
`model.json.evaluation.json` viennent d'un corpus offline construit
manuellement pour l'entraînement/évaluation, pas d'un flux live. Un seuil par
source pour `github-advisories` (piste 1, traitée en session) n'aurait donc
aucun effet en production tant que ce chemin d'ingestion n'existe pas — ce
n'est pas un bug de câblage à corriger mais une pipeline d'ingestion live à
construire si on veut vraiment que cette source soit classifiée en continu.
À trancher avant d'investir dans le calcul du seuil : est-ce que
`github-advisories` doit rester un stratum d'évaluation ponctuel, ou devenir
une source ingérée en continu (ce qui redéfinirait aussi la priorité du
chantier du point 6 ci-dessus) ?

## 8. (Priorité la plus haute) Le modèle de production n'a jamais été évalué sur son vrai trafic

Le jeu de test indépendant qui a servi à promouvoir le modèle actuel
(`model.json.evaluation.json`, iteration 3, 400 documents,
`data/iteration-2026-10-20/iteration-3-independent-test-400.jsonl`) est
composé à 100 % de `ransomware.live` (200) et `github-advisories` (200) —
confirmé en comptant les paires `(source, source_type)` du fichier. **Zéro
document `darkweb_page`.** Or le point 7 ci-dessus établit que le seul flux
qui tourne réellement en continu en production est `sites.RunDarkwebScraper()`,
qui ne produit que du `darkweb_page` (7583 documents en base dev). Les
métriques de promotion (precision 0,758 / recall 0,911) ne mesurent donc
littéralement rien sur ce que le modèle classe en vrai ; `github-advisories`,
en plus de ne jamais être ingéré en continu (point 7), n'a même jamais servi
qu'à évaluer un modèle qui ne le voit jamais en production.

À faire, avant toute autre piste de ce document (1 à 7 inclus) : construire un
jeu d'évaluation étiqueté à partir d'un échantillon stratifié de vrais
`darkweb_page` tirés de la collection `article` live, selon le protocole
décrit dans `docs/annotation-policy-v1.md`. L'outillage existe déjà pour la
partie extraction (`cmd/exportpipeline`, qui gèle un corpus non étiqueté
depuis le pipeline réel) et pour la partie campagne/annotation
(`scripts/automatic_campaign.py` + `scripts/annotate_autoeval.py`, qui
pilotent des annotateurs IA locaux via Ollama — confirmé disponible en local,
modèles `qwen3:8b` et autres déjà chargés). Le point bloquant est que
`exportpipeline` ne renseigne pas les champs `stratum`/`language` qu'exige
`automatic_campaign.py freeze` pour la stratification — à définir avant de
pouvoir lancer une vraie campagne sur du `darkweb_page` (par ex. stratifier
par langue détectée et/ou par host plutôt que par type de source, puisque
tout vient de la même source ici).

Tant que ce point n'est pas traité, toute optimisation issue des points 1-7
(seuil par source, encodeur multilingue, min-df, audit de poids) s'appuie sur
une mesure qui ne représente pas le trafic réel.

**Mise à jour (2026-11-05, campagne exécutée) :** `cmd/exportpipeline` a été
étendu avec une stratification heuristique par langue détectée (script
disposable, aucune dépendance ajoutée), produisant un pool de 4100 documents
`darkweb_page` live. `scripts/automatic_campaign.py freeze` puis
`scripts/run_automatic_campaign.py` (annotation IA locale `qwen3:8b` +
`gemma4:e2b`, calibrate, puis evaluate du test gelé) ont tourné de bout en
bout sur ce pool — voir `data/darkweb-live-eval-2026-11-05/` (gitignoré,
contenu brut sensible).

Résultat sur le split test (200 docs, 140 en consensus IA, couverture 70 %,
54 abstentions, 6 désaccords) : **au seuil de production actuel (0,4888),
precision = 0,04, recall = 1,0 (tp=1, fp=24, fn=0, tn=115)** — sur 25
documents remontés comme menace, 24 sont des faux positifs. Le seuil
recalibré par la campagne (0,7626, choisi en repli car l'objectif
recall≥0,9/precision≥0,5 était infaisable sur la validation) donne
precision=0, recall=0 (pire). Statut formel de la campagne :
`not_qualified` — seulement 1 à 4 documents positifs en consensus sur 140,
largement sous le minimum statistique (`per_stratum_class=10`) que le
protocole exige lui-même pour certifier un chiffre.

Donc ces chiffres ne sont pas certifiables au sens du protocole, mais l'écart
avec la precision annoncée au moment de la promotion (0,758) est bien assez
large (0,04 vs 0,758) pour ne pas être un artefact d'échantillonnage : sur le
trafic réel, le modèle de production remonte presque uniquement des faux
positifs. Prochaine étape : élargir le pool/l'échantillon pour dépasser les
minimums statistiques (en particulier obtenir plus de documents positifs
réels côté `darkweb_page`, ce qui demande probablement une recherche ciblée
plutôt qu'un tirage aléatoire, le forum darkweb étant très majoritairement du
bruit/commerce/pages mortes), avant de pouvoir recalibrer un seuil de
production fiable pour cette source.
