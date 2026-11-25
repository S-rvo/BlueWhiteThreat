# Plan de corrections — audit du crawler (2026-11-10)

Plan d'exécution destiné à un agent (Codex / Claude Code). Il fait suite à un
audit complet en lecture seule de `apps/crawler` (3 933 lignes Go). Build,
`go vet` et tous les tests passent au départ : **toute régression est de ton
fait**. Exécuter les lots dans l'ordre, un commit par lot.

---

## 0. Règles non négociables (protocole du repo, voir `AGENTS.md`)

- `git fetch origin` avant **chaque** commit (hook `pre-commit` rejette si le
  dernier fetch date de plus de 10 min).
- Jamais `git add -A` ni `git add .` — lister les fichiers un par un.
- Message : `type(scope): description`, type ∈
  `feat|fix|docs|chore|tune|security|refactor|test|perf` (hook `commit-msg`).
- Jamais de push, `--no-verify`, `--amend` sur commit poussé, `reset --hard`.
- Ne pas toucher à `apps/classifier/**` (surtout `model.json*`).
- Validation après chaque lot, depuis `apps/crawler` :
  `go build ./... && go vet ./... && go test ./...`

### ⚠️ Pré-requis bloquant : l'arbre de travail est déjà sale

`apps/crawler` contient ~12 fichiers modifiés non committés + 5 fichiers
nouveaux (`internal/db/rendered.go`, `internal/db/host.go`, plusieurs tests)
qui sont du travail antérieur **validé mais jamais committé** — et ce sont
exactement les fichiers que ce plan modifie (`cmd/main.go`,
`internal/db/frontier.go`, `internal/db/redis.go`…).

**Avant le Lot 1, demander à l'utilisateur** s'il faut d'abord committer cet
en-cours (recommandé : un commit séparé du travail existant, pour ne pas
mélanger l'ancien durcissement et les correctifs de ce plan dans les mêmes
commits). Ne pas décider à sa place.

### Infrastructure de test existante (à réutiliser, ne pas réinventer)

- Les tests Redis utilisent **miniredis** : voir le pattern dans
  `internal/db/redis_freshness_test.go` (≈ l.22) :
  `server := miniredis.RunT(t)` + `RedisClient = redis.NewClient(...)` +
  `t.Cleanup(...)`. Les scripts Lua du projet tournent sur miniredis.
- Les nouveaux tests de frontière vont dans `internal/db/` à côté des
  existants (`hardening_test.go`, `improvements_test.go`).

---

## Lot 1 — Les URLs bloquées (CSAM/farm) ne doivent plus recycler tous les 30 jours

**Commit : `fix(crawler): sortir les URLs bloquées du cycle de replanification`**

### Problème

`cmd/main.go` ≈ l.172-177 (`crawlerWorker`) : quand une URL dépilée est
bloquée, le code appelle :

```go
db.FinishClaim(url, token, true, 30*24*time.Hour, http.StatusForbidden)
```

Dans le Lua de `FinishClaim` (`internal/db/frontier.go` ≈ l.84-100) :
`success=1` mais le status 403 n'est ni 2xx ni 304 → la branche succès est
sautée, `fetch_failure_total` est incrémenté, **et la dernière ligne
`ZADD crawler:scheduled` replanifie l'URL à +30 j**. Résultat : une URL
bloquée est re-claimée, re-bloquée et re-replanifiée **pour toujours**.

### Correctif

1. Dans `internal/db/frontier.go`, ajouter une fonction `DropClaim(u, token
   string) error` sur le modèle exact de `FinishClaim` (même garde Lua
   token + lease, même retour `expired or superseded lease` si `v == 0`).
   Son Lua, atomique :
   - même garde que `FinishClaim` l.85 (token + score lease) sinon `return 0` ;
   - `HDEL crawler:tokens`, `ZREM crawler:inflight` ;
   - libérer l'hôte : `HSET crawler:host_ready host now` +
     `ZADD crawler:hosts now host` (hôte = `HostKey(u)` passé en ARGV, comme
     dans `FinishClaim`) ;
   - `ZADD crawler:blocked now u` (dead-letter dédiée, distincte de
     `crawler:dead` qui signifie « échecs réseau répétés ») ;
   - `HDEL crawler:attempts u` ;
   - `HINCRBY crawler:stats blocked_dropped_total 1` ;
   - **ne PAS** faire le `ZADD crawler:scheduled` final.
2. Dans `cmd/main.go` (`crawlerWorker`), remplacer l'appel `FinishClaim(...)`
   du bloc bloqué par `db.DropClaim(url, token)` (adapter le message de log).

### Tests (miniredis, dans `internal/db/`)

- Après `AddURLToQueue` + `ClaimURL` + `DropClaim` : l'URL n'est membre ni de
  `crawler:scheduled`, ni de `crawler:inflight`, ni de `urls_to_crawl` ; elle
  est dans `crawler:blocked` ; `crawler:tokens` ne la contient plus.
- `DropClaim` avec un mauvais token retourne l'erreur de lease et ne touche à
  rien.
- Vérifier qu'un `ClaimURL` suivant ne ressort jamais cette URL.

---

## Lot 2 — `processURL` : les échecs de stockage doivent relâcher la lease

**Commit : `fix(crawler): libérer la lease en échec sur erreur de stockage dans processURL`**

### Problème

Dans `cmd/main.go` (`processURL`), plusieurs chemins d'erreur font `return`
**sans** `FinishClaim` : la lease traîne jusqu'à expiration (10 min), l'hôte
reste verrouillé, et surtout `crawler:attempts` n'est pas incrémenté — une
erreur Redis/Neo4j persistante sur une URL boucle sans jamais atteindre le
dead-letter. Chemins concernés (numéros de ligne approximatifs) :

- l.214-218 : échec `db.FetchMetadata` ;
- l.242-245 : échec `db.SaveURL` (Neo4j) ;
- l.251-255 : échec `db.AddURLToQueue` en milieu de boucle (liens
  partiellement enqueués) ;
- l.261-264 : échec `db.SaveLinks` ;
- l.267-270 : échec `db.SaveClaimMetadata`.

### Correctif

1. Ajouter dans `cmd/main.go` un helper :

   ```go
   // releaseForRetry relâche la lease en échec : la politesse hôte est rendue,
   // le compteur de tentatives avance, le dead-letter reste atteignable.
   func releaseForRetry(url, token string) {
       if e := db.FinishClaim(url, token, false, db.RetryDelay(url)); e != nil {
           log.Printf("release: %v", e)
       }
   }
   ```

2. Appeler `releaseForRetry(url, token)` juste avant chacun des `return`
   listés ci-dessus (après le `log.Printf` existant).
3. **Ne pas toucher** aux `return` sur `workCtx.Err() != nil` : ce sont les
   chemins d'arrêt/perte de lease, le comportement actuel (laisser expirer)
   est voulu.
4. Le chemin d'échec de fetch existant (l.227-237) fait déjà la bonne chose
   (RetryDelay + Retry-After) — ne pas le modifier.

Nota : si `SaveClaimMetadata` échoue parce que la lease a été reprise,
`FinishClaim` échouera aussi avec `expired or superseded lease` — c'est
attendu et sans danger, le log suffit.

### Tests

`cmd/main_test.go` existe déjà (non tracké) — regarder son pattern. Si
`processURL` n'est pas testable unitairement sans refactor, la validation
`go build && go vet && go test` suffit pour ce lot ; ne pas lancer de gros
refactor de testabilité ici.

---

## Lot 3 — Supprimer le pipeline legacy mort (~400 lignes)

**Commit : `refactor(crawler): supprimer le pipeline pré-leases mort`**

### Constat (vérifié par grep : zéro appelant hors tests)

L'ancien chemin pré-leases coexiste avec `ClaimURL`/`FinishClaim` et fait
croire à deux pipelines. À supprimer :

- `internal/db/redis.go` : `GetNextURLFromQueue` (et son sampling
  `SRandMemberN`), `MarkURLAsCrawled` ;
- `internal/db/frontier.go` : `SaveFetchMetadata` (l.122-124) — attention,
  **garder `FetchMetadata`** (l.125-127) qui est utilisé par `processURL` ;
- `internal/db/neo4j.go` : `SaveLink` (l.52-71, le singulier) — **garder
  `SaveLinks`/`saveLinkBatch`** ;
- `internal/crawler/crawler.go` : `Crawler()` (l.204-207) et `Fetch()`
  (l.50-52) **si et seulement si** plus aucun appelant après suppression de
  `Crawler()` ; garder `fetch`/`fetchContext` (utilisés par les tests avec
  proxy injecté) et `FetchContext` (utilisé par `processURL`).

### Précautions obligatoires

1. Re-vérifier les appelants juste avant de supprimer (l'arbre bouge) :

   ```sh
   grep -rn --include='*.go' -E 'GetNextURLFromQueue|MarkURLAsCrawled|SaveFetchMetadata|SaveLink\(|crawler\.Crawler\(|crawler\.Fetch\(' apps/
   ```

   Vérifier aussi `apps/scraper` et `apps/classifier` (modules séparés mais
   prudence).
2. Tests à adapter :
   - `tests/crawler_test.go` appelle `crawler.Crawler(...)` → réécrire sur
     `FetchContext` ou supprimer le test s'il ne teste que le wrapper.
   - `internal/db/redis_freshness_test.go` :
     `TestMarkURLAsCrawled_RecordsTimestamp` (≈ l.38). **Attention** : ces
     tests protègent l'invariant `urls_crawled ⊆ urls_crawled_at` (contrat
     public avec le scraper, commit `20827cb`). Supprimer uniquement le test
     du chemin legacy ; **conserver impérativement** les tests de l'invariant
     sur le chemin `FinishClaim`.
3. Supprimer aussi le paramètre mort `maxDepth` : toujours `0`, jamais
   utilisé. Retirer de `startCrawler`, `crawlerWorker`, `processURL`
   (`cmd/main.go`) et adapter `cmd/main_test.go` (l.13 :
   `startCrawler(ctx, 0)`).

---

## Lot 4 — `RecordHostFetch` : ne plus compter les status 0

**Commit : `fix(crawler): exclure le status 0 des statistiques par hôte`**

`internal/db/frontier.go` l.186-208 : `processURL` (cmd/main.go l.221) appelle
`RecordHostFetch(url, result.Status, ...)` même quand `err != nil`, où
`result.Status` vaut souvent 0 → des champs `status_0` polluent
`crawler:host_stats:*` (visible dans les logs de prod).

Correctif minimal, dans `RecordHostFetch` :

```go
if status != 0 {
    p.HIncrBy(ctx, key, fmt.Sprintf("status_%d", status), 1)
}
```

Le compteur `errors` (champ `failed`) couvre déjà ces cas. Test miniredis :
un appel avec `status=0, failed=true` n'écrit pas de champ `status_0` mais
incrémente `requests` et `errors`.

---

## Lot 5 — Borner les clés Redis à croissance infinie

**Commit : `fix(crawler): borner farm:prefixes et crawler:page par TTL`**

1. **`farm:prefixes:<hash>`** (`internal/db/farmdetect.go`) : SAdd sans
   expiration. Ajouter `EXPIRE 30 j` rafraîchi à chaque SAdd (TTL glissant).
   Sans risque : la décision de blocage est déjà persistée à part dans
   `dynamic_blocked_hashes` (permanent, ne pas y toucher) ; le set de préfixes
   ne sert qu'à atteindre le seuil de détection.
2. **`crawler:page:<url>`** : une hash par URL crawlée, à vie. Ajouter
   `EXPIRE 90 j` rafraîchi à chaque écriture, aux deux points d'écriture :
   le Lua de `SaveClaimMetadata` (`frontier.go` ≈ l.153-167) et le
   `HSET crawler:page:... interval_seconds` dans le Lua de `FinishClaim`
   (≈ l.95). Conséquence si la clé expire : perte des validateurs
   etag/modified → le prochain passage fait un GET complet au lieu d'un GET
   conditionnel. Acceptable et auto-réparant.

Tests miniredis : après `SaveClaimMetadata`, `TTL(crawler:page:<u>) > 0` ;
après le SAdd farm, `TTL(farm:prefixes:<h>) > 0`. (miniredis supporte
`FastForward` pour tester l'expiration si besoin.)

---

## Lot 6 (optionnel, plus gros) — Propager `context.Context` dans le package `db`

**Commit : `refactor(crawler): propager le contexte dans les opérations Redis`**

`internal/db/redis.go` déclare un `ctx = context.Background()` de niveau
package utilisé par **toutes** les opérations Redis : aucune annulation à
l'arrêt (SIGTERM), aucun timeout applicatif.

Approche mécanique, à faire seulement si les lots 1-5 sont verts :

1. Ajouter `ctx context.Context` en premier paramètre des fonctions du chemin
   chaud : `ClaimURL`, `FinishClaim`, `DropClaim`, `RenewClaim`,
   `SaveClaimMetadata`, `RetryDelay`, `FetchMetadata`, `AddURLToQueue`,
   `RecordHostFetch`, `RecordProcessingDuration`, `RequeueStaleURLs`,
   `DrainRenderedLinks`, `SaveURL`/`SaveLinks` (Neo4j : remplacer leurs
   `context.Background()` internes par le ctx reçu, en conservant les
   `WithTimeout(30s)` par-dessus).
2. Dans `cmd/main.go`, passer `ctx` (boucle worker) ou `workCtx`
   (`processURL`) selon le site d'appel.
3. Mettre à jour tous les tests (ils passeront `context.Background()`).
4. Le `ctx` package-level peut rester pour les fonctions de setup
   (`InitRedis`, `SetupRedisQueues`, `ReconcileFrontier`) dans un premier
   temps — ne pas chercher l'exhaustivité, chercher l'annulation du chemin
   chaud.

Si le diff devient tentaculaire, découper par fichier en plusieurs commits
`refactor(crawler): ...`.

---

## Lot 7 — Cosmétique

**Commit : `chore(crawler): nettoyage mineur post-audit`**

1. `internal/crawler/transport.go` : la condition gzip se parse
   `a || (b && c)` alors que l'intention est `(a || b) && c`. Ajouter les
   parenthèses explicites :

   ```go
   if (resp.Header.Get("Content-Encoding") == "gzip" || strings.HasSuffix(req.URL.Path, ".gz")) && !resp.Uncompressed {
   ```

   Sans effet pratique connu (Go supprime `Content-Encoding` quand il
   décompresse lui-même) mais lève l'ambiguïté. Vérifier que les tests de
   `transport` passent toujours.
2. Supprimer le binaire compilé non tracké `apps/crawler/collectseeds`
   (9,4 Mo) et ajouter `apps/crawler/collectseeds` au `.gitignore` approprié.

---

## Hors périmètre de ce plan (décisions utilisateur, ne pas faire)

- **Reconstruire/redéployer les images Docker** : les conteneurs tournent sur
  une image du 2026-11-09 qui ne contient rien de tout ceci. C'est l'étape la
  plus importante en valeur, mais elle touche la prod → uniquement sur ordre
  explicite de l'utilisateur.
- L'API `:8080` sans auth/CORS `*` : bénin tant que le port n'est pas publié
  (vérifié) — documenté, pas d'action.
- Tout ce qui concerne le scraper (anomalie de persistance en cours
  d'investigation séparée) et le classifier.

## Check-list finale (après le dernier lot)

```sh
cd apps/crawler
gofmt -l .           # doit être vide
go build ./... && go vet ./... && go test ./...
git log --oneline -8 # un commit par lot, messages conformes
git status           # aucun fichier étranger embarqué
```
