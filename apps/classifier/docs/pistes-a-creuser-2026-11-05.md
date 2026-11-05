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
