# Pistes à creuser (2026-11-05)

Issues ouvertes pendant l'audit de poids (`experiments/cmd/auditweights`) et la relecture de
l'historique CV (`docs/cv-*.md`, `model.json.evaluation.json`). La piste du
seuil par source (écart de précision `github-advisories` 0,70 vs
`ransomware.live` 0,92) est traitée séparément — voir message de session, pas
ce fichier. Ce qui suit est resté sans décision.

**Statut (2026-11-06) :** points 1, 3 et 5 investigués et documentés
(sections "Investigué"/"Résolu"/"Fait" inline ci-dessous) — mesures et
implémentations faites, mais aucune décision de réentraînement, promotion ou
changement d'architecture prise : ça reste à la charge de l'utilisateur.
Points 2, 4, 6-11 inchangés depuis leur dernière mise à jour.

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

**Investigué (2026-11-06), bloqué — pas une impasse de recherche mais un
manque d'infra à trancher par l'utilisateur :**
- `data/reviewed-template-fold-families-20261014.json` n'existe plus sur le
  disque, et son historique git est vide (`git log --all -- <ce fichier>` ne
  retourne rien) : ce fichier n'a jamais été committé, c'était un artefact
  éphémère d'une session précédente. Même chose pour
  `docs/cv-embeddinggemma-20261015.md` (cité par le doc 18 comme la première
  comparaison EmbeddingGemma-vs-Nomic) et pour les répertoires de run bruts
  `data/cv-embeddinggemma-20261018/` et `data/cv-nomic-repeat-20261018/` :
  tous disparus, jamais tracés par git.
- Plus largement, **aucun script ou commande dans ce dépôt n'appelle
  EmbeddingGemma ou Nomic** (`grep -rl "embeddinggemma\|nomic-embed"` sur tout
  le code est vide) : la génération des embeddings pour ces runs a été faite
  par un script ad hoc d'une session antérieure, jamais committé, et ce
  script lui-même n'existe plus. Le code qui *consomme* un cache d'embeddings
  figé existe bien (`experiments/cmd/cvclassweights/embeddings.go`, flags `-embeddings`/
  `-hybrid` dans `experiments/cmd/cvclassweights/main.go`), mais il n'y a plus rien en
  amont pour produire ce cache.
- Le seul chemin de génération d'embeddings qui survit dans le dépôt est
  `experiments/python/embedding_head_experiment.py`, et il encode avec
  `sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2` — **un
  troisième modèle, différent à la fois d'EmbeddingGemma et de Nomic**, donc
  pas substituable sans changer ce que "encodeur multilingue" désigne dans ce
  point.
- Le paquet `sentence-transformers` n'est pas installé dans l'environnement
  Python courant (`ModuleNotFoundError`), et aucun poids EmbeddingGemma/Nomic
  n'est présent dans le cache HuggingFace local (2,0 Go de cache, mais rien
  qui corresponde à ces deux noms) : les régénérer suppose un téléchargement
  de plusieurs Go et potentiellement un accès à une API d'embedding —
  décision d'infra/coût qui n'est pas la mienne à prendre unilatéralement.

**Conclusion (avant mesure) :** ce point ne peut pas être complété dans la
session actuelle sans l'une de ces décisions utilisateur : (a) autoriser
l'installation de `sentence-transformers` + le téléchargement des poids
EmbeddingGemma et Nomic en local pour régénérer un cache d'embeddings et
refiger une nouvelle `fold-families`, ou (b) accepter de comparer le TF-IDF
de référence contre le modèle `paraphrase-multilingual-MiniLM-L12-v2` déjà
utilisable localement (signal différent de ce que le point demandait à
l'origine), ou (c) retrouver/reconstituer le script de génération
EmbeddingGemma/Nomic d'origine si une copie existe hors de ce dépôt (poste
de travail, autre machine).

**Fait (2026-11-06), option (b) retenue :** `sentence-transformers` installé
dans un venv local dédié (`.venv-embedding-experiment/`, non committé, dans
`.gitignore`). Comparaison appariée sur le même CSV réel que le point 5
(`data/reviewed-with-text-20261106.csv`, 2325 lignes) :
- TF-IDF de production figé (vocabulaire/IDF de `model.json`, extrait par
  l'outil jetable `experiments/cmd/dumptfidf`) vs `paraphrase-multilingual-MiniLM-L12-v2`
  (encodeur figé, aucun fine-tuning).
- Même protocole pour les deux représentations : seule la tête de régression
  logistique est réentraînée par pli (`class_weight="balanced"`), 5 plis
  stratifiés par label, 3 graines indépendantes (20261106/07/08).
  Implémentation : `experiments/python/point1_tfidf_vs_minilm.py`, résultats bruts dans
  `data/point1-tfidf-vs-minilm-20261106/results.json` (non committés).

**Résultat net, stable sur les 3 graines : le TF-IDF de production bat
MiniLM sur toute la ligne**, pas un résultat mitigé —

| Métrique | TF-IDF (prod, figé) | MiniLM multilingue |
|---|---|---|
| Average Precision | 0,904–0,907 | 0,866–0,867 |
| FP min à rappel 0,65 | 37–40 | 70–78 |
| FP min à rappel 0,75 | 55–59 | 114–127 |
| FP min à rappel 0,85 | 105–111 | 188–210 |
| FP min à rappel 0,90 | 148–157 | 255–269 |
| F1 @ seuil 0,5 | 0,842–0,844 | 0,784–0,795 |

Le TF-IDF produit environ deux fois moins de faux positifs que MiniLM à
chaque rappel cible, de façon stable sur les 3 graines.

**Réserves, à lire avant d'agir sur ce résultat :**
1. Ce n'est pas EmbeddingGemma. Le gain net précédent d'EmbeddingGemma sur
   Nomic (`docs/cv-embeddinggemma-20261018.md`) était faible et seulement à
   rappel élevé (0,65-0,75) — rien ne garantit qu'EmbeddingGemma se
   comporterait comme MiniLM ici. Ce résultat montre qu'un encodeur
   multilingue générique ne bat pas automatiquement le TF-IDF de ce domaine,
   pas qu'aucun encodeur ne le pourrait.
2. Asymétrie de fuite potentielle en faveur du TF-IDF : son vocabulaire et
   ses poids IDF ont été choisis sur des données d'entraînement de
   production qui recoupent probablement ce CSV d'évaluation (même souci
   déjà documenté aux points 5 et 9) — c'est un avantage que MiniLM n'a pas
   (encodeur générique, jamais exposé à ce corpus, poids entièrement figés
   avant même de savoir que ce corpus existe). Donc l'écart réel pourrait
   être plus petit qu'il n'apparaît ici.
3. Le TF-IDF inclut des n-grammes de caractères (l33tspeak, variantes de
   frappe) auxquels un encodeur de phrases généraliste n'est pas
   spécifiquement entraîné — plausible explication structurelle de l'écart,
   pas juste un artefact de fuite.

**Conclusion :** même en tenant compte de la réserve 2, l'écart est trop
large (quasi du simple au double en FP à chaque rappel cible, 3/3 graines)
pour être entièrement expliqué par la fuite — le TF-IDF de production reste
la meilleure option connue actuellement pour ce corpus. Aucune décision de
réentraînement ou de changement d'architecture n'a été prise : ceci reste un
résultat à documenter, pas une action à exécuter seul.

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

**Résolu (2026-11-06) :** c'est bien la compétition chi-deux, pas le
plancher `min-df`. Ajout de `tfidf.FitWithNonLatinMinDF` (minimum abaissé
pour les tokens non-Latins seuls, **sans** budget réservé — ils rivalisent
normalement pour les 5000 places sur le même classement chi-deux que tout le
reste) et d'un flag `-nonlatin-min-df` dans `experiments/cmd/cvclassweights` pour le
tester via le même harnais que `-script-reserve`. Diagnostic sur
`training_data.csv` (2325 documents, proxy — le snapshot gelé
`data/confirmation-campaign/` du protocole CV de référence est absent de ce
worktree, comme déjà noté pour la piste identity_fraud ci-dessus) : abaisser
le seuil de 10 à 3 pour les tokens non-Latins admet 15896 nouveaux candidats,
mais **un seul** survit à la compétition chi-deux pour les 5000 places
(vocabulaire non-Latin total : 1 → 2 tokens). Le plancher `min-df` n'est donc
pas le goulot — même en l'abaissant, l'écrasante majorité des tokens
non-Latins perd la course chi-deux face aux tokens latins/structurels
existants. Ça explique pourquoi `--script-reserve` (qui force une réserve
plutôt que de laisser rivaliser) était le seul des deux mécanismes à changer
quoi que ce soit, et pourquoi même ce gain forcé restait localisé sans
bouger la matrice de confusion globale (point déjà conclu « non concluant »
deux fois) : le signal non-Latin existe mais est structurellement trop rare
et trop dispersé pour rivaliser à armes égales dans un budget chi-deux
partagé avec le reste du vocabulaire.

## 3. Deux FN sur contenu quasi vide (pages de connexion) : problème de collecte, pas de modèle

Les deux "Black Hat Chat" dans le cluster FN (`cv-analyse-fp-fn-20261018.md`)
sont des pages de connexion de ~1200 caractères, sans texte à apprendre. Si
ces documents restent dans les jeux d'entraînement/test, ils plafonnent le
rappel mesurable sans qu'aucun levier lexical ou d'encodeur ne puisse agir.

À transmettre au crawler/scraper plutôt qu'au NLP : soit obtenir un accès
authentifié à ces pages, soit les exclure explicitement du calcul de rappel
(comme `bootstrap.IsLinkDirectory` exclut déjà les pages d'annuaire de liens,
cf. `cmd/main/main.go`).

**Résolu (2026-11-06) :** implémenté la deuxième option — pas d'accès
authentifié disponible pour l'instant (hors de portée immédiate), mais
`bootstrap.IsGatedLoginPage` (`internal/bootstrap/label.go`) détecte
désormais ces pages de connexion quasi vides par le même principe que
`IsLinkDirectory` : un plafond de longueur (2000 caractères) combiné à des
marqueurs de mur d'authentification (« password » + « username »/« login »/
« sign in », ou des locutions comme « forgot your password », « remember
me »). Câblé dans les quatre mêmes points d'appel qu'`IsLinkDirectory`
(`cmd/bootstrap`, `cmd/export`, `cmd/exportpipeline`, `cmd/main`) pour que
ces pages soient exclues de l'entraînement/export ET classées directement
`Relevant: false` à l'inférence, en gardant train/inference cohérents (même
raisonnement déjà en commentaire pour `IsLinkDirectory` dans
`cmd/main/main.go`). Testé unitairement (`TestIsGatedLoginPage`) avec un cas
positif (mur de connexion court), un cas négatif (mention de login sans mot
de passe) et un cas négatif de non-régression (article long mentionnant
« password »/« login » dans un contexte de fuite de données, qui ne doit pas
être exclu). `go build`/`go vet`/`go test` verts sur les packages concernés.
Changement non committé (standing rule : jamais commit sans demande
explicite) ; aucune donnée d'entraînement/modèle régénérée — la décision de
relancer bootstrap/export/retrain avec ce filtre reste à l'utilisateur.

## 4. `min-df` par défaut en production : écart jamais audité

L'audit initial (session précédente) a noté que `cmd/train/main.go` appelle
`tfidf.Fit` (minimum de fréquence documentaire = 1), alors que les runs CV
documentés dans `docs/cv-*.md` utilisent systématiquement `min-df 10`. Le
modèle de production (`provisional-oof-advisories-20261020T103652Z`) vient
d'un pipeline OOF différent (`cvclassweights`), dont le `min-df` effectif n'a
pas été vérifié directement dans `model.json`. À clarifier : quel `min-df` a
réellement produit le vocabulaire de production, et si ce n'est pas 10,
pourquoi l'écart avec les runs CV documentés n'a jamais été discuté.

**Résolu (2026-11-05) :** le modèle de production vient de
`data/iteration-2026-10-20/iteration-3-candidate.json`
(`model_version`/`promoted_at` identiques), dont le rapport compagnon
`iteration-3-candidate.json.selection.json` montre `"feature_config":
"stable_terms_word_reserve"` — **min-df=2** (réserve de 1500 mots), pas
min-df=1 (défaut `tfidf.Fit` de `cmd/train/main.go`) ni min-df=10 (runs CV
documentés). Les trois pipelines (`cmd/train`, runs CV, `cmd/traincandidate`
réel) ont donc chacun un `min-df` différent, jamais réconcilié — mais testé
en session (voir point 9) : ajouter un candidat min-df=10 à la grille de
`cmd/traincandidate` ne change rien (précision quasi identique, légèrement
pire), donc l'écart de `min-df` lui-même n'explique pas de perte de
performance mesurable. Le vrai problème découvert est ailleurs — voir point 9.

## 5. Relier l'audit de poids à un vrai jeu de validation stratifié par source

`experiments/cmd/auditweights -eval`/`-ablate` n'a été testé qu'avec un CSV synthétique
(smoke test). Pour relier les termes à risque élevé repérés dans le top 200
(`c_xxxx`, et les stopwords à poids fort `is`/`in`/`english`/`sign`/`please`)
à un résultat mesuré, il faut un vrai CSV `id,text,label` stratifié par
source — notamment un sous-ensemble `github-advisories` pur, pour vérifier si
ces features à risque sont surreprésentées dans ses faux positifs. Bloqué
par l'absence de texte joint dans `data/reviewed_labels.csv` (qui n'a que
`id,label,source`) : il faut retrouver/reconstituer le texte source de ces
IDs, probablement via `data/iteration-2026-10-27-source-refresh/` qui semble
contenir des exports `github-advisories` plus récents.

**Résolu (2026-11-06) :** le texte a été retrouvé, pas dans
`iteration-2026-10-27-source-refresh` (ses jsonl `github-advisories`/CTI
n'utilisent pas le même schéma d'id — `cert-eu-cti:...`, pas d'ObjectID hex —
et ne contiennent aucun des IDs de `reviewed_labels.csv`) mais dans
`data/automatic-eval-2026-10-19/campaign/reference.csv`, qui partage le même
schéma d'ID et couvre 2325/2339 lignes de `reviewed_labels.csv` (14 IDs
introuvables dans tout le dépôt, abandonnés — a priori des documents
supprimés depuis). Les labels des deux fichiers concordent à 100 % sur les
2325 lignes communes, donc aucun arbitrage de confiance n'a été nécessaire.
CSV réel écrit : `data/reviewed-with-text-20261106.csv` (id,text,label,
2325 lignes) — non committé.

**Stratification par source — abandonnée, et c'est cohérent avec les points
6/7 ci-dessous** : vérifié que `_id` d'aucune de ces 2325 lignes n'existe
dans la collection Mongo live `article` (`ObjectId` cherché directement,
`NOT FOUND`), et qu'aucun fichier source local (jsonl/CSV, y compris
`training_data.csv` à la racine) ne porte de métadonnée `source`/`source_type`
pour ces IDs au-delà de `id,text,label`. Ce jeu de 2325 documents est donc
very probablement une capture figée du corpus `darkweb_page` humainement
revu (cohérent avec les clusters FN/FP du point 2 — noms de forums, pas de
CVE/GHSA : aucune occurrence de `GHSA-` dans tout le fichier) ; il ne contient
structurellement aucun document `github-advisories`, ce qui recoupe la
conclusion déjà actée au point 7 (cette source n'a jamais eu de chemin
d'ingestion live) — chercher un sous-ensemble `github-advisories` pur dans ce
jeu n'aurait donc rien pu trouver, et ce n'est pas une lacune de cette
reconstruction mais la confirmation d'un fait déjà établi.

**Mesure réelle obtenue** (`go run ./experiments/cmd/auditweights -model model.json -eval
data/reviewed-with-text-20261106.csv -top 170 -ablate N`), qui remplace le
smoke test synthétique :
- Baseline sur ce vrai CSV (n=2325) : précision 0,778, rappel 0,917, F1 0,842
  — à comparer aux 0,830/? documentés côté OOF (point 9) : cet écart de
  précision confirme, sur données réelles cette fois, le risque de fuite déjà
  suspecté (ce CSV recoupe probablement en partie les données d'entraînement
  elles-mêmes, donc ce chiffre n'est pas un vrai holdout — à ne pas citer
  comme performance de production).
- `-ablate 20` (les 20 plus hauts risques, tous des termes CTI légitimes —
  `data`, `c_hack`, `c_leak`, `stolen`, `cards`, `exfiltrated`...) : précision
  +0,054, rappel **-0,083**. Donc même les termes les plus "à risque" par le
  score `|weight|*idf` portent un vrai signal — les zéroer coûte du rappel
  net plus qu'il n'achète de précision.
- `-ablate 170` (jusqu'aux stopwords suspects `is`/`in`/`english`/`sign`/
  `please` identifiés dans ce point) : précision +0,126, rappel **-0,428**,
  F1 -0,207. Perte nette sévère : les stopwords à poids fort sont entremêlés
  avec des centaines de termes CTI légitimes dans ce classement par risque,
  et les zéroer tous ensemble détruit largement plus de signal réel qu'il
  n'élimine de bruit. Conclusion : le score de risque seul ne sépare pas
  proprement "terme mémorisé" de "terme généralisable" — isoler juste les 5
  stopwords suspects (sans toucher aux ~165 termes CTI légitimes autour)
  nécessiterait une modification du flag `-ablate` pour cibler des termes par
  nom plutôt que par rang, non faite ici (changement de code non demandé).

## 6. Chantier `iteration-2026-10-27-source-refresh` : pas encore conclu

Ce dossier (postérieur à la promotion du 20/10) contient déjà des audits
`security-stackexchange`, `ransomware-live`, `official-cti-with-cisa` —
diversification de sources en cours, sans doc de synthèse ni décision actée
pour l'instant (aucun fichier `.md` de conclusion, contrairement aux chantiers
d'octobre).

**Décision (2026-11-05) :** ne pas concentrer ce chantier sur
`github-advisories`. La décision du point 7 (ci-dessous) confirme que cette
source reste hors production — y investir un seuil ou une diversification
dédiée n'a aucun effet sur le trafic réel. Le chantier `source-refresh` est
donc dépriorisé sous les points liés à `darkweb_page` (2, 3, 4, 5, 8) tant
qu'il ne sert qu'à améliorer la qualité d'un jeu d'évaluation offline plutôt
qu'à mesurer ou améliorer ce que le modèle voit réellement en production.

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
**Décision (2026-11-05) :** `github-advisories` reste un stratum
d'évaluation ponctuel, pas une source ingérée en continu. Construire une
pipeline d'ingestion live pour cette source est un chantier d'infra neuf,
non justifié par un besoin produit déjà exprimé, et irait à l'encontre de
la priorité établie au point 8 : la seule chose qui compte en production
aujourd'hui, c'est `darkweb_page`, le seul flux réellement vivant. Tant
qu'aucun besoin business explicite de surveillance continue des advisories
n'est formulé, ne pas investir ici.

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

**Mise à jour (2026-11-05, tentative d'enrichissement par score — plafond
structurel atteint) :** pour obtenir plus de positifs, tentative de
restratifier le pool par bande de score du modèle de production
(`score_high`/`score_mid`/`score_low`) au lieu de la langue, afin de
concentrer l'échantillon là où le modèle est confiant. Deux échecs de
`automatic_campaign.py freeze` ont révélé un plafond structurel, pas un bug
de script :

- Les documents à score élevé sont majoritairement des quasi-doublons du jeu
  d'entraînement lui-même : sur une bande resserrée de 370 documents
  `score_high`, 357/370 (96 %) sont exclus par `freeze()` comme `known_id`
  ou `reference_near_duplicate`. Le score élevé du modèle corrèle donc
  surtout avec « ce document ressemble à ce que le modèle a mémorisé », pas
  avec un signal généralisable indépendant.
- Un nouvel export complet (`cmd/exportpipeline -limit 0`) confirme que le
  corpus `darkweb_page` live dédupliqué contient exactement **4100**
  documents uniques, identique à l'export précédent — rien de nouveau n'est
  arrivé depuis. Sur ces 4100, seuls **580** passent le filtre de nouveauté
  de `freeze()` (hors entraînement/fixtures/doublons internes), et la
  première campagne en a déjà consommé **400**. Il ne reste donc qu'environ
  **180** documents réellement neufs et éligibles dans tout le corpus live
  actuel — largement sous les 400 qu'exige une seconde campagne
  indépendante, quelle que soit la stratification choisie (confirmé en
  repassant à une stratification par tercile de score sur tout le pool
  restant : seulement 222 éligibles au total).

Conclusion : le problème n'est pas la méthode d'échantillonnage mais le
volume du corpus live lui-même. Une seconde campagne vraiment indépendante
n'est pas possible tant que le crawler n'a pas accumulé davantage de pages
`darkweb_page` nouvelles (le flux `sites.RunDarkwebScraper()` tourne en
continu mais le volume net de contenu *nouveau et non dupliqué* est faible).
**Décision (2026-11-05) :** on n'écarte pas d'office le besoin d'un corpus
plus profond. Mais tant qu'il n'a pas grossi, forcer une deuxième campagne
en réutilisant les mêmes ~180 documents éligibles et les mêmes deux
annotateurs IA (option « relâcher l'indépendance ») n'apporterait aucune
preuve nouvelle — ce serait la même mesure déguisée en mesure indépendante,
donc plus trompeur que l'honnêteté actuelle du statut `not_qualified`. On
retient donc le résultat de la première campagne (precision 0,04 / recall
1,0 au seuil de production, non certifié statistiquement mais l'écart avec
0,758 est trop large pour être du bruit) comme la meilleure mesure
disponible, et on ne retente pas de seconde campagne avant que le corpus
`darkweb_page` ait matériellement grossi. Vérification simple avant de
retenter : `exportpipeline -limit 0` puis comparer le total dédupliqué à
4100 — pas la peine de rebâtir un pool enrichi si ce chiffre n'a pas
sensiblement augmenté.

## 9. (Priorité la plus haute, à égalité avec le point 8) Les métriques OOF de promotion (precision 0,830) semblent inflées par une fuite train/dev via quasi-doublons

En cherchant une vraie amélioration du modèle (tester un candidat min-df=10
dans `cmd/traincandidate/crossfit.go`), tentative de reproduire d'abord le
run exact qui a produit le modèle de production
(`iteration-3-candidate.json`, mêmes fichiers d'entrée : `training_data.csv`,
`iteration-3-development-combined.jsonl`,
`iteration-3-development-consensus.json`, `-cross-fit`). **La reproduction
ne retombe pas sur les métriques de promotion :**

| | seuil | precision | recall |
|---|---|---|---|
| Production promue (`iteration-3-candidate.json.selection.json`) | 0,4888 | 0,830 | 0,900 |
| Reproduction avec le code actuel (mêmes données, même config `stable_terms_word_reserve`) | 0,318 | 0,560 | 0,952 |

Testé sur les 4 configs existantes + un 5e candidat min-df=10 ajouté pour
l'occasion : **toutes** donnent un plafond de précision autour de 0,52-0,56
à rappel ~0,95 — pas une seule config ne s'approche de 0,830. Le candidat
min-df=10 n'apporte rien (0,559 vs 0,560, dans le bruit) : l'hypothèse
« mauvais min-df » du point 4 est donc écartée comme explication.

Cause identifiée : `cmd/traincandidate` (jamais commité en git — aucun
historique) dépend de `internal/dedup/shingles.go`
(`MergeContentDuplicatesWithDiagnostics`, fusion par quasi-doublons à 80 %
de similarité Jaccard sur shingles de 5 tokens), également non commité et
visiblement ajouté/modifié après la promotion du 20/10 (le rapport actuel
contient des clés de diagnostic — `near_duplicate_component_diagnostics`,
`experimental_source_threshold_metrics` — absentes du rapport de production,
et le nombre de documents d'entraînement après fusion diffère légèrement :
2901 vs 2908). Cette fusion conditionne le regroupement
`provenance_and_near_duplicate_connected_component` utilisé pour assigner
les plis de la validation croisée groupée à 5 plis. Si cette fusion était
absente ou plus faible au moment du run de production, des quasi-doublons
ont pu se retrouver dans des plis différents (train vs dev tenu à l'écart),
ce qui permet au modèle de « reconnaître » un document qu'il a presque déjà
vu plutôt que de généraliser — inflation classique de précision par fuite.

Ce n'est pas démontré de façon causale stricte (pas de comptage direct des
paires quasi-dupliquées qui auraient traversé la frontière train/dev dans le
run original — impossible à refaire à l'identique puisque le code qui a
produit `iteration-3-candidate.json` n'existe dans aucune révision git), mais
la cohérence avec le point 8 (precision live mesurée à 0,04 vs 0,758 promis)
est frappante : les deux pointent vers la même histoire — le modèle promu
doit une bonne partie de son score apparent à de la mémorisation de
quasi-doublons plutôt qu'à un signal généralisable.

**Décision (2026-11-05) :** documenter maintenant, ne pas retrancher/retrainer
la production dans la foulée — c'est une décision de promotion/produit, pas
un correctif technique à prendre seul.

**Correction (2026-11-05, suite) — l'hypothèse quasi-doublons ci-dessus est
quantitativement insuffisante, et une 2e hypothèse testée est réfutée :**

1. Les diagnostics déjà présents dans la reproduction contredisent l'ampleur
   nécessaire pour expliquer l'écart. `training_documents_excluded_for_near_development_overlap: 0`
   et `development_near_duplicate_component_diagnostics` ne montrent que
   **4 composantes multi-documents sur 1047, toutes de taille 2**, dans
   l'ensemble de développement de 1051 documents. Une poignée de paires
   quasi-dupliquées ne peut pas expliquer un basculement TP/FP/FN/TN de
   (298/61/33/659) à (315/248/16/472) — l'écart est trop petit en magnitude
   pour la fuite train/dev décrite plus haut comme cause principale.

2. Deuxième piste testée et **réfutée par expérience contrôlée** :
   `internal/tfidf/structural.go` (non commité) ajoute 7 nouveaux indicateurs
   structurels (boilerplate marketplace/corporate/hosting, fraude financière,
   gambling, annuaire darkweb, preuve de compromission) et des features de
   conjonction (`<page_type>_with_evidence`/`_without_evidence`), appelés
   sans condition par `terms()` — donc actifs même pour la config gagnante
   `stable_terms_word_reserve`, pas seulement pour les configs à
   `familyBudgets`. Hypothèse : ce changement de vocabulaire (pas la fuite
   CV) expliquerait l'écart. **Test :** `git stash` sur ce seul fichier pour
   revenir à sa version commitée (4 indicateurs de base, sans conjonctions),
   rebuild, même commande de reproduction exacte. **Résultat : aucun
   changement significatif** — precision 0,5597/recall 0,949
   (tp=314/fp=247/fn=17/tn=473) contre 0,5595/0,952 (tp=315/fp=248/fn=16/tn=472)
   avec le code complet. Écart dans le bruit. Hypothèse réfutée.

**État actuel : les deux causes examinées (fuite CV par quasi-doublons,
nouveaux indicateurs structurels) sont toutes les deux insuffisantes pour
expliquer l'écart de ~27 points de précision.** La cause réelle reste
non identifiée. Candidats non encore testés : (a) le jeu de poids/L2 candidat
ou la logique `selectThresholdBySource`/`better()` elle-même a pu changer
depuis le 20/10 (pas seulement `shingles.go`/`structural.go`) ; (b)
`training_data.csv` à cette date a pu différer de sa version actuelle même
si ses derniers commits précèdent le 20/10 (modifications non commitées
possibles à l'époque, maintenant perdues) ; (c) le jeu
`iteration-3-development-combined.jsonl`/`-consensus.json` a pu lui-même
être retouché après la promotion. Sans accès à un snapshot git du code et
des données au 20/10, une reproduction exacte n'est probablement pas
possible — seule une nouvelle campagne d'évaluation live (point 8) donne un
signal fiable sur la performance réelle actuelle du modèle promu.

## 10. Les seuils par source, déjà câblés dans le code courant, récupèrent la précision perdue au point 9 — mais ne mesurent toujours pas le trafic réel (voir correctif ci-dessous)

En creusant pourquoi la reproduction plafonne à precision 0,56 malgré un
rappel confortable (0,95), `selected_metrics_by_source` de
`repro-iteration-3-candidate.json.selection.json` montre que l'écart vient
presque entièrement d'une seule source :

| source | precision | recall | fp |
|---|---|---|---|
| `github-advisories` | 0,458 | 0,989 | 205 |
| `ransomware.live` | 0,768 | 0,910 | 43 |

Le seuil global unique (0,318) est tiré vers le bas pour satisfaire la
contrainte de rappel ≥0,9 sur `github-advisories`, ce qui noie `ransomware.live`
sous les faux positifs de l'autre source. `cmd/traincandidate/crossfit.go`
calcule déjà un seuil par source (`sourceSpecificThresholds`, sur les mêmes
prédictions OOF groupées à 5 plis que la sélection globale — donc pas de
fuite supplémentaire par rapport à la méthode déjà utilisée pour le seuil
unique) et l'écrit dans le candidat sous `source_thresholds` /
`experimental_source_threshold_metrics`. Appliqué :

| | seuil | precision | recall |
|---|---|---|---|
| Seuil global unique (`selected`) | 0,318 | 0,560 | 0,952 |
| Seuils par source (`github-advisories`: 0,546, `ransomware.live`: 0,317) | — | **0,807** | **0,909** |

0,807/0,909 est à ~2 points de precision de la promotion du 20/10
(0,830/0,900) — **sur le code actuel, de façon reproductible**, sans dépendre
du mystère non résolu du point 9. De plus, `internal/model/persist.go`
(`SourceThresholds` + `Validate()`) et `internal/classify/classify.go`
(ligne ~118 : bascule sur le seuil de la source si présent) savent déjà
appliquer ce champ à l'inférence — ce n'est pas un chantier à construire,
juste un candidat déjà produit par `-cross-fit` qui n'a jamais été
promu avec ce champ rempli (le `model.json` et le `iteration-3-candidate.json`
actuellement en production ont tous les deux `source_thresholds: null`).

**Réserve honnête :** ce seuillage ajoute un degré de liberté par source,
sélectionné sur le même échantillon OOF fini que le seuil global — un
optimisme résiduel (sur-ajustement multiple-comparaisons) est possible,
probablement plus faible qu'entre point 9 (0,56) et la promotion (0,830)
mais non nul. Seule une vraie validation indépendante (nouvelle campagne
vivante, point 8) confirmerait le chiffre hors échantillon.

**Correctif (2026-11-05, suite immédiate) — ce levier ne répare probablement
pas le vrai problème.** En vérifiant la composition des sources de
`iteration-3-development-combined.jsonl` (le jeu qui alimente tout l'OOF des
points 9 et 10) : **uniquement `github-advisories` et `ransomware.live`,
zéro `darkweb_page`.** `training_data.csv` n'a même pas de colonne `source`.
Le point 8 a déjà établi que le flux de production réel ne produit que du
`darkweb_page`, et que sur un vrai échantillon de trafic `darkweb_page` le
modèle actuel mesure precision=0,04 (tp=1, fp=24). Autrement dit : le point
10 répare la précision sur deux sources qui, d'après le point 8, ne
représentent quasiment pas le trafic que le modèle voit en vrai — l'une
d'elles (`github-advisories`) n'a même aucun chemin d'ingestion vers la
production (point 7). Le gain 0,56 → 0,807 est réel et reproductible, mais
porte sur la mauvaise distribution. Il ne faut pas s'attendre à ce qu'il
déplace le chiffre de 0,04 sur `darkweb_page`.

**Conséquence pour la priorisation :** le vrai verrou reste le point 8 — il
n'existe aujourd'hui aucun jeu d'évaluation étiqueté avec assez de positifs
`darkweb_page` réels pour mesurer quoi que ce soit de fiable (campagne
`not_qualified`, 1 à 4 positifs/140, et la tentative d'enrichissement par
score a buté sur 96 % de quasi-doublons d'entraînement). Tant que ce jeu
n'existe pas, ni le point 9 ni le point 10 ne peuvent être traduits en gain
mesurable sur le trafic réel — l'étape suivante utile n'est pas d'entraîner
un nouveau candidat avec seuils par source, mais de débloquer point 8 :
obtenir de vrais positifs `darkweb_page` par recherche ciblée (mots-clés
identifiants une page de compromission : fuite de données, accès initial,
rançon, etc., combinés à une exclusion explicite des quasi-doublons
d'entraînement) plutôt que par échantillonnage aléatoire ou par score du
modèle actuel.

## 11. Le verrou volumétrique du point 8 a une cause identifiée : le crawler est à l'arrêt depuis son premier passage

En creusant le point 8 (pourquoi le pool `darkweb_page` est plafonné à 4100
documents), `docker logs crawler_service` montre le conteneur en boucle
`restart: unless-stopped` toutes les ~60s depuis la fin de son unique passage
(« Queue is empty. Crawler has completed. Total URLs crawled: 22923 ») — il
ne découvre plus aucune URL neuve. Ce n'est pas un problème de volume de
cibles .onion, c'est un problème de frontière de découverte : avant le
fetch HTTP, `apps/crawler/internal/crawler/crawler.go` filtrait déjà les
liens suivis avec `isRelevantLink` (mots-clés CTI dans l'URL/texte du lien),
ce qui limite drastiquement les pages découvertes dès la première passe —
une fois ce sous-graphe épuisé, la queue Redis reste vide pour de bon.

**Un correctif existait déjà, non committé, non buildé, dans l'arbre de
travail** (probablement une tentative antérieure dans cette même session,
jamais finalisée) :
- `crawler.go` : supprime le filtre `isRelevantLink` sur le suivi de lien —
  la pertinence CTI se décide en aval, pas à la découverte ; les contrôles
  CSAM/fermes bloquées restent appliqués avant toute requête.
- `redis.go` (crawler et scraper) : ajoute un index de priorité
  (`urls_to_crawl_priority`) par mots-clés de scoring, au-dessus de la queue
  durable `urls_to_crawl` (source de vérité inchangée).
- `scraper/internal/sites/page.go` + `Darkweb.go` : tague chaque page
  sauvegardée avec `source_type: "darkweb_page"` et réécrit
  `SaveScrapedPage` en pipeline Mongo atomique (ne préserve `relevance` que
  si le contenu est inchangé).
- `docker-compose.yaml` : retire le conteneur sidecar `classifier-model-sync`
  et passe `classifier-prd` en bind-mount direct (lecture seule) du dépôt au
  lieu d'un volume Docker.

**Vérifié (2026-11-05/06) :** `go vet`/`go build`/`go test ./...` passent sans
erreur sur `apps/crawler` et `apps/scraper` avec ces changements — aucune
régression sur les tests existants. Aucun de ces tests n'exerce le nouveau
comportement (suppression du filtre, priorité, écriture Mongo) : ça confirme
seulement « ça compile et rien de connu n'a cassé », pas que le nouveau
comportement est correct. Les conteneurs n'ont pas été rebuild ni redéployés
— décision explicitement laissée à l'utilisateur (changement de périmètre de
crawl sur le dark web + écriture Mongo, sur une infra qui tourne réellement).

**Pourquoi c'est la piste la plus directe vers une amélioration réelle du
modèle :** points 9/10 sont bloqués par l'absence de signal `darkweb_page`
(point 8), et point 8 est bloqué par l'absence de nouvelles pages collectées.
Ce correctif attaque directement cette cause racine. Tant qu'il n'est pas
déployé et laissé tourner assez longtemps pour regarnir le pool au-delà de
4100 documents, aucune nouvelle campagne d'évaluation ni aucun nouveau
candidat entraîné sur du signal `darkweb_page` réel n'est possible — on
tournerait en rond sur le même pool épuisé.

**Décision à prendre par l'utilisateur :** rebuild + redéploiement des
conteneurs `crawler`/`scraper` (et le changement `docker-compose.yaml`
associé pour `classifier-prd`), puis surveillance pour confirmer que le
crawler recommence à découvrir des URLs neuves.
