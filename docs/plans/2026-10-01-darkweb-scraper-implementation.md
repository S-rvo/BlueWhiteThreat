# Scraper générique de contenu darkweb — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remplacer le chemin par défaut du scraper (actuellement un fixture de test `localhost:8081`) par un vrai scraper qui consomme les URLs `.onion` découvertes par le crawler (set Redis `urls_crawled`), récupère leur contenu via `chromedp` derrière un proxy Tor, gère optionnellement un login par identifiants fournis, et écrit le résultat dans `MongoDB.article` avec la source conservée.

**Architecture:** Nouveau package `apps/scraper/internal/db` (client Redis) + nouvelles fonctions dans `apps/scraper/internal/sites` (config par site, proxy Tor, orchestrateur). `cmd/main.go` appelle le nouvel orchestrateur à la place de l'ancien `sites.RunScraper()`.

**Tech Stack:** Go 1.24, `chromedp` (déjà présent), `github.com/redis/go-redis/v9` (à ajouter), `go.mongodb.org/mongo-driver` (déjà présent).

**Design de référence:** `docs/plans/2026-10-01-darkweb-scraper-design.md`

---

## Prérequis avant de commencer

Travailler dans un worktree dédié (skill `superpowers:using-git-worktrees`), branche `feature/darkweb-scraper`.

Pour les tests d'intégration Redis (Task 2), il faut Redis up :
```bash
docker compose up -d redis
```

---

### Task 1: Ajouter la dépendance Redis au module scraper

**Files:**
- Modify: `apps/scraper/go.mod`, `apps/scraper/go.sum`

**Step 1: Ajouter la dépendance**

```bash
cd apps/scraper && go get github.com/redis/go-redis/v9@v9.7.3
```

**Step 2: Vérifier que ça compile**

Run: `cd apps/scraper && go build ./...`
Expected: pas d'erreur (aucun code ne l'utilise encore, juste vérifier que le module résout).

**Step 3: Commit**

```bash
git add apps/scraper/go.mod apps/scraper/go.sum
git commit -m "chore(scraper): ajoute la dépendance go-redis"
```

---

### Task 2: Client Redis + diff des URLs non-scrapées

**Files:**
- Create: `apps/scraper/internal/db/redis.go`
- Test: `apps/scraper/tests/redis_test.go`

**Contexte:** Le crawler (`apps/crawler/internal/db/redis.go`) ajoute les URLs crawlées dans le set Redis `urls_crawled` via `SAdd`. Ce scraper doit lire ce set, le comparer à son propre set `urls_scraped`, et ne traiter que la différence.

**Step 1: Write the failing test**

```go
// apps/scraper/tests/redis_test.go
package scraper_tests

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
)

func testRedisClient(t *testing.T) *redis.Client {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	client := redis.NewClient(&redis.Options{Addr: host + ":6379"})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis non disponible (%v) — lancer `docker compose up -d redis`", err)
	}
	return client
}

func TestGetUnscrapedURLs_ReturnsOnlyNewURLs(t *testing.T) {
	ctx := context.Background()
	client := testRedisClient(t)
	defer client.FlushDB(ctx)

	client.SAdd(ctx, "urls_crawled", "http://a.onion", "http://b.onion", "http://c.onion")
	client.SAdd(ctx, "urls_scraped", "http://a.onion")

	got, err := db.GetUnscrapedURLs(ctx, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]bool{"http://b.onion": true, "http://c.onion": true}
	if len(got) != 2 {
		t.Fatalf("got %d urls, want 2: %#v", len(got), got)
	}
	for _, u := range got {
		if !want[u] {
			t.Errorf("unexpected url in result: %s", u)
		}
	}
}

func TestMarkURLScraped_AddsToScrapedSet(t *testing.T) {
	ctx := context.Background()
	client := testRedisClient(t)
	defer client.FlushDB(ctx)

	if err := db.MarkURLScraped(ctx, client, "http://a.onion"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	isMember, err := client.SIsMember(ctx, "urls_scraped", "http://a.onion").Result()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isMember {
		t.Error("url not marked as scraped")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `docker compose up -d redis && cd apps/scraper && go test ./tests/... -run TestGetUnscrapedURLs -v`
Expected: FAIL — `undefined: db.GetUnscrapedURLs` (le package n'existe pas encore)

**Step 3: Write minimal implementation**

```go
// apps/scraper/internal/db/redis.go
package db

import (
	"context"
	"os"

	"github.com/redis/go-redis/v9"
)

// ConnectRedis se connecte à l'instance Redis partagée avec le crawler.
func ConnectRedis() *redis.Client {
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6379"
	}
	return redis.NewClient(&redis.Options{
		Addr:     host + ":" + port,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
}

// GetUnscrapedURLs renvoie les URLs présentes dans "urls_crawled" (rempli par
// le crawler) mais absentes de "urls_scraped" (rempli par ce scraper).
func GetUnscrapedURLs(ctx context.Context, client *redis.Client) ([]string, error) {
	return client.SDiff(ctx, "urls_crawled", "urls_scraped").Result()
}

// MarkURLScraped marque une URL comme traitée pour ne pas la retraiter au tick suivant.
func MarkURLScraped(ctx context.Context, client *redis.Client, url string) error {
	return client.SAdd(ctx, "urls_scraped", url).Err()
}
```

**Step 4: Run test to verify it passes**

Run: `cd apps/scraper && go test ./tests/... -run 'TestGetUnscrapedURLs|TestMarkURLScraped' -v`
Expected: PASS (2 tests)

**Step 5: Commit**

```bash
git add apps/scraper/internal/db/redis.go apps/scraper/tests/redis_test.go
git commit -m "feat(scraper): client Redis et diff des URLs crawlées non-scrapées"
```

---

### Task 3: Config par site (`sites.json`)

**Files:**
- Create: `apps/scraper/internal/sites/config.go`
- Create: `apps/scraper/data/sites.json`
- Test: `apps/scraper/tests/config_test.go`

**Contexte:** Fichier éditable listant, par domaine `.onion`, les sélecteurs et noms de variables d'env pour un login optionnel. Un domaine absent du fichier = scraping anonyme.

**Step 1: Write the failing test**

```go
// apps/scraper/tests/config_test.go
package scraper_tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
)

func writeTempSitesConfig(t *testing.T, content string) string {
	dir := t.TempDir()
	path := filepath.Join(dir, "sites.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}

func TestLoadSiteConfigs_ParsesKnownDomain(t *testing.T) {
	path := writeTempSitesConfig(t, `{
		"exampleforum.onion": {
			"login_url": "http://exampleforum.onion/login",
			"username_selector": "#username",
			"password_selector": "#password",
			"submit_selector": "button[type=submit]",
			"username_env": "SITE_EXAMPLEFORUM_USER",
			"password_env": "SITE_EXAMPLEFORUM_PASS"
		}
	}`)

	configs, err := sites.LoadSiteConfigs(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, ok := configs["exampleforum.onion"]
	if !ok {
		t.Fatal("expected domain not found in configs")
	}
	if cfg.LoginURL != "http://exampleforum.onion/login" {
		t.Errorf("got LoginURL=%q", cfg.LoginURL)
	}
	if cfg.UsernameEnv != "SITE_EXAMPLEFORUM_USER" {
		t.Errorf("got UsernameEnv=%q", cfg.UsernameEnv)
	}
}

func TestLoadSiteConfigs_UnknownDomainAbsent(t *testing.T) {
	path := writeTempSitesConfig(t, `{}`)

	configs, err := sites.LoadSiteConfigs(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := configs["unknown.onion"]; ok {
		t.Error("expected unknown domain to be absent")
	}
}

func TestLoadSiteConfigs_MissingFileReturnsEmptyMap(t *testing.T) {
	configs, err := sites.LoadSiteConfigs("/path/does/not/exist.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("expected empty map, got %#v", configs)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd apps/scraper && go test ./tests/... -run TestLoadSiteConfigs -v`
Expected: FAIL — `undefined: sites.LoadSiteConfigs`

**Step 3: Write minimal implementation**

```go
// apps/scraper/internal/sites/config.go
package sites

import (
	"encoding/json"
	"os"
)

// SiteConfig décrit comment se connecter (optionnellement) à un site .onion
// avant d'en récupérer le contenu.
type SiteConfig struct {
	LoginURL         string `json:"login_url"`
	UsernameSelector string `json:"username_selector"`
	PasswordSelector string `json:"password_selector"`
	SubmitSelector   string `json:"submit_selector"`
	UsernameEnv      string `json:"username_env"`
	PasswordEnv      string `json:"password_env"`
}

// LoadSiteConfigs charge la config par domaine depuis un fichier JSON.
// Si le fichier n'existe pas, renvoie une map vide (aucun site configuré
// pour le login — comportement par défaut : scraping anonyme).
func LoadSiteConfigs(path string) (map[string]SiteConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]SiteConfig{}, nil
	}
	if err != nil {
		return nil, err
	}

	var configs map[string]SiteConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, err
	}
	return configs, nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd apps/scraper && go test ./tests/... -run TestLoadSiteConfigs -v`
Expected: PASS (3 tests)

**Step 5: Create le fichier de config par défaut (vide)**

```json
// apps/scraper/data/sites.json
{}
```

**Step 6: Commit**

```bash
git add apps/scraper/internal/sites/config.go apps/scraper/data/sites.json apps/scraper/tests/config_test.go
git commit -m "feat(scraper): config par site pour login optionnel (sites.json)"
```

---

### Task 4: Rotation de proxy Tor

**Files:**
- Create: `apps/scraper/internal/sites/proxy.go`
- Test: `apps/scraper/tests/proxy_test.go`

**Contexte:** Même principe que `apps/crawler/internal/crawler/crawler.go` (rotation `tor1/tor2/tor3`), mais pour configurer le flag `--proxy-server` de chromedp plutôt que colly.

**Step 1: Write the failing test**

```go
// apps/scraper/tests/proxy_test.go
package scraper_tests

import (
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
)

func TestNextTorProxy_RotatesThroughAllProxies(t *testing.T) {
	rotator := sites.NewTorProxyRotator([]string{"tor1:9050", "tor2:9050", "tor3:9050"})

	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		seen[rotator.Next()] = true
	}

	if len(seen) != 3 {
		t.Errorf("expected to see all 3 proxies over 6 calls, got %#v", seen)
	}
}

func TestNextTorProxy_SingleProxyAlwaysReturnsIt(t *testing.T) {
	rotator := sites.NewTorProxyRotator([]string{"tor1:9050"})

	if got := rotator.Next(); got != "tor1:9050" {
		t.Errorf("got %q, want %q", got, "tor1:9050")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd apps/scraper && go test ./tests/... -run TestNextTorProxy -v`
Expected: FAIL — `undefined: sites.NewTorProxyRotator`

**Step 3: Write minimal implementation**

```go
// apps/scraper/internal/sites/proxy.go
package sites

import "sync"

// TorProxyRotator fait tourner les requêtes entre plusieurs proxies SOCKS5 Tor
// (même principe que la rotation tor1/tor2/tor3 du crawler).
type TorProxyRotator struct {
	mu      sync.Mutex
	proxies []string
	next    int
}

func NewTorProxyRotator(proxies []string) *TorProxyRotator {
	return &TorProxyRotator{proxies: proxies}
}

// Next renvoie le prochain proxy "host:port" dans la rotation.
func (r *TorProxyRotator) Next() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.proxies[r.next%len(r.proxies)]
	r.next++
	return p
}
```

**Step 4: Run test to verify it passes**

Run: `cd apps/scraper && go test ./tests/... -run TestNextTorProxy -v`
Expected: PASS (2 tests)

**Step 5: Commit**

```bash
git add apps/scraper/internal/sites/proxy.go apps/scraper/tests/proxy_test.go
git commit -m "feat(scraper): rotation de proxy Tor pour chromedp"
```

---

### Task 5: Modèle Mongo `ScrapedPage` + upsert

**Files:**
- Create: `apps/scraper/internal/sites/page.go`
- Test: `apps/scraper/tests/page_test.go`

**Contexte:** Réutilise `db.SaveAllEntries` (déjà générique par réflexion), qui construit son filtre d'upsert en mettant en minuscule le nom du champ Go — il faut donc que le champ unique s'appelle `URL` pour matcher le filtre `"url"`.

**Step 1: Write the failing test**

```go
// apps/scraper/tests/page_test.go
package scraper_tests

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
)

func TestSaveScrapedPage_UpsertsIntoMongo(t *testing.T) {
	if os.Getenv("MONGO_HOST") == "" {
		t.Skip("MONGO_HOST non défini — lancer `docker compose up -d mongodb` et exporter les vars MONGO_*")
	}

	page := sites.ScrapedPage{
		URL:       "http://test-page.onion",
		Source:    "http://test-page.onion",
		ScrapedAt: time.Now().UTC(),
		Content:   "contenu de test",
	}

	n, err := sites.SaveScrapedPage(page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 upserted doc, got %d", n)
	}

	client, err := db.ConnectMongo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Disconnect(context.Background())

	var got bson.M
	err = client.Database("BlueWhiteThreat").Collection("article").
		FindOne(context.Background(), bson.M{"url": "http://test-page.onion"}).Decode(&got)
	if err != nil {
		t.Fatalf("document not found: %v", err)
	}
	if got["content"] != "contenu de test" {
		t.Errorf("got content=%v", got["content"])
	}

	client.Database("BlueWhiteThreat").Collection("article").
		DeleteOne(context.Background(), bson.M{"url": "http://test-page.onion"})
}
```

**Step 2: Run test to verify it fails**

Run: `cd apps/scraper && go test ./tests/... -run TestSaveScrapedPage -v`
Expected: FAIL — `undefined: sites.ScrapedPage` / `undefined: sites.SaveScrapedPage`

**Step 3: Write minimal implementation**

```go
// apps/scraper/internal/sites/page.go
package sites

import (
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
)

// ScrapedPage est le contenu brut d'une page .onion récupérée par le scraper,
// avec sa source conservée pour vérification par un analyste CTI.
type ScrapedPage struct {
	URL       string    `bson:"url"`
	Source    string    `bson:"source"`
	ScrapedAt time.Time `bson:"scraped_at"`
	Content   string    `bson:"content"`
}

// SaveScrapedPage upsert une page scrapée dans MongoDB.article, par URL unique.
func SaveScrapedPage(page ScrapedPage) (int, error) {
	return db.SaveAllEntries("BlueWhiteThreat", "article", []ScrapedPage{page}, []string{"URL"})
}
```

**Step 4: Run test to verify it passes**

Run:
```bash
docker compose up -d mongodb
export MONGO_HOST=localhost MONGO_PORT=27017 MONGO_INITDB_ROOT_USERNAME=devroot MONGO_INITDB_ROOT_PASSWORD=devpassword
cd apps/scraper && go test ./tests/... -run TestSaveScrapedPage -v
```
Expected: PASS (1 test)

**Step 5: Commit**

```bash
git add apps/scraper/internal/sites/page.go apps/scraper/tests/page_test.go
git commit -m "feat(scraper): modèle ScrapedPage et upsert Mongo"
```

---

### Task 6: Récupération de page avec chromedp + proxy Tor + login optionnel

**Files:**
- Create: `apps/scraper/internal/sites/fetch.go`
- Test: `apps/scraper/tests/fetch_test.go`

**Contexte:** Pas de test automatisé contre un vrai site `.onion` (dépendance réseau externe, pas adapté à un test répété). On teste uniquement la construction des options chromedp (proxy bien positionné) — la récupération réelle sera vérifiée manuellement (Task 8).

**Step 1: Write the failing test**

```go
// apps/scraper/tests/fetch_test.go
package scraper_tests

import (
	"strings"
	"testing"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/sites"
)

func TestBuildChromedpOptions_SetsProxyServerFlag(t *testing.T) {
	opts := sites.BuildChromedpOptions("tor1:9050")

	found := false
	for _, opt := range opts {
		if strings.Contains(fmtOption(opt), "socks5://tor1:9050") {
			found = true
		}
	}
	if !found {
		t.Error("expected proxy-server flag pointing at socks5://tor1:9050 among options")
	}
}
```

Note : `chromedp.ExecAllocatorOption` n'est pas directement inspectable (c'est une closure). Plutôt que d'introspecter la closure, on teste indirectement via une fonction exportée qui renvoie la valeur du flag construite, séparée de son application :

```go
// remplace le test ci-dessus par :
func TestProxyServerFlagValue(t *testing.T) {
	got := sites.ProxyServerFlagValue("tor1:9050")
	want := "socks5://tor1:9050"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd apps/scraper && go test ./tests/... -run TestProxyServerFlagValue -v`
Expected: FAIL — `undefined: sites.ProxyServerFlagValue`

**Step 3: Write minimal implementation**

```go
// apps/scraper/internal/sites/fetch.go
package sites

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// ProxyServerFlagValue construit la valeur du flag --proxy-server pour chromedp
// à partir d'un proxy Tor "host:port".
func ProxyServerFlagValue(torProxy string) string {
	return "socks5://" + torProxy
}

// BuildChromedpOptions construit les options d'allocation chromedp avec le
// proxy Tor donné, en mode headless (même pattern que DarkThreat.go).
func BuildChromedpOptions(torProxy string) []chromedp.ExecAllocatorOption {
	return append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("blink-settings", "imagesEnabled=false"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.ProxyServer(ProxyServerFlagValue(torProxy)),
	)
}

// FetchPage navigue vers targetURL via Tor (proxy déjà posé dans ctx par
// BuildChromedpOptions au niveau de l'appelant), effectue un login optionnel
// si cfg est non-nil, puis renvoie le texte brut du body.
func FetchPage(ctx context.Context, targetURL string, cfg *SiteConfig) (string, error) {
	if cfg != nil {
		username := envOrEmpty(cfg.UsernameEnv)
		password := envOrEmpty(cfg.PasswordEnv)
		if err := chromedp.Run(ctx,
			chromedp.Navigate(cfg.LoginURL),
			chromedp.WaitVisible(cfg.UsernameSelector, chromedp.ByQuery),
			chromedp.SendKeys(cfg.UsernameSelector, username, chromedp.ByQuery),
			chromedp.SendKeys(cfg.PasswordSelector, password, chromedp.ByQuery),
			chromedp.Click(cfg.SubmitSelector, chromedp.ByQuery),
		); err != nil {
			return "", fmt.Errorf("login failed for %s: %w", targetURL, err)
		}
	}

	var content string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.Text("body", &content, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("fetch failed for %s: %w", targetURL, err)
	}
	return content, nil
}
```

Ajouter le helper `envOrEmpty` (pas de valeur par défaut nécessaire — un nom de variable vide signifie "pas de login configuré pour ce champ") :

```go
// dans apps/scraper/internal/sites/fetch.go, en bas du fichier
import "os"

func envOrEmpty(name string) string {
	if name == "" {
		return ""
	}
	return os.Getenv(name)
}
```

**Step 4: Run test to verify it passes**

Run: `cd apps/scraper && go test ./tests/... -run TestProxyServerFlagValue -v`
Expected: PASS

**Step 5: Vérifier que tout compile**

Run: `cd apps/scraper && go build ./...`
Expected: pas d'erreur

**Step 6: Commit**

```bash
git add apps/scraper/internal/sites/fetch.go apps/scraper/tests/fetch_test.go
git commit -m "feat(scraper): récupération de page via chromedp + proxy Tor + login optionnel"
```

---

### Task 7: Orchestrateur `RunDarkwebScraper` + branchement dans `main.go`

**Files:**
- Create: `apps/scraper/internal/sites/Darkweb.go`
- Modify: `apps/scraper/cmd/main.go`
- Modify: `apps/scraper/.env.example`

**Contexte:** Boucle unique (pas de ticker ici — le ticker existe déjà dans `main.go`) qui : récupère les URLs non-scrapées, pour chacune détermine son domaine, charge sa config, fetch, upsert, marque scrapée. Une erreur sur une URL est loggée et n'interrompt pas les suivantes (cf. design, gestion des erreurs).

**Step 1: Implémentation (pas de test unitaire dédié — orchestrateur fin qui compose les briques déjà testées ; couvert par une vérification manuelle en Task 8)**

```go
// apps/scraper/internal/sites/Darkweb.go
package sites

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
)

func sitesConfigPath() string {
	if p := os.Getenv("SITES_CONFIG_PATH"); p != "" {
		return p
	}
	return "./data/sites.json"
}

func torProxies() []string {
	return []string{"tor1:9050", "tor2:9050", "tor3:9050"}
}

// RunDarkwebScraper traite toutes les URLs découvertes par le crawler et pas
// encore scrapées : récupère leur contenu via Tor, puis les persiste en Mongo.
func RunDarkwebScraper() {
	ctx := context.Background()
	redisClient := db.ConnectRedis()
	defer redisClient.Close()

	urls, err := db.GetUnscrapedURLs(ctx, redisClient)
	if err != nil {
		log.Printf("Erreur lecture des URLs non-scrapées: %v", err)
		return
	}
	if len(urls) == 0 {
		log.Println("Aucune nouvelle URL à scraper")
		return
	}
	log.Printf("%d nouvelles URLs à scraper", len(urls))

	configs, err := LoadSiteConfigs(sitesConfigPath())
	if err != nil {
		log.Printf("Erreur chargement sites.json: %v", err)
		configs = map[string]SiteConfig{}
	}

	rotator := NewTorProxyRotator(torProxies())

	for _, targetURL := range urls {
		if err := scrapeOne(ctx, targetURL, configs, rotator); err != nil {
			log.Printf("Erreur scraping %s: %v", targetURL, err)
			continue
		}
		if err := db.MarkURLScraped(ctx, redisClient, targetURL); err != nil {
			log.Printf("Erreur marquage %s comme scrapée: %v", targetURL, err)
		}
	}
}

func scrapeOne(ctx context.Context, targetURL string, configs map[string]SiteConfig, rotator *TorProxyRotator) error {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("url invalide: %w", err)
	}

	var cfg *SiteConfig
	if c, ok := configs[parsed.Hostname()]; ok {
		cfg = &c
	}

	allocCtx, cancelAlloc := newChromedpContext(ctx, rotator.Next())
	defer cancelAlloc()

	content, err := FetchPage(allocCtx, targetURL, cfg)
	if err != nil {
		return err
	}

	page := ScrapedPage{
		URL:       targetURL,
		Source:    targetURL,
		ScrapedAt: time.Now().UTC(),
		Content:   content,
	}
	if _, err := SaveScrapedPage(page); err != nil {
		return fmt.Errorf("échec sauvegarde Mongo: %w", err)
	}
	return nil
}
```

Ajouter `newChromedpContext` dans `fetch.go` (sépare la création du contexte chromedp de la logique de fetch, pour que `Darkweb.go` n'ait pas à connaître `chromedp.NewExecAllocator`) :

```go
// à ajouter dans apps/scraper/internal/sites/fetch.go
import "github.com/chromedp/chromedp"

// newChromedpContext crée un contexte chromedp configuré avec le proxy Tor donné.
// L'appelant doit appeler la fonction cancel retournée pour libérer les ressources.
func newChromedpContext(parent context.Context, torProxy string) (context.Context, context.CancelFunc) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(parent, BuildChromedpOptions(torProxy)...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	return ctx, func() {
		cancelCtx()
		cancelAlloc()
	}
}
```

**Step 2: Vérifier que ça compile**

Run: `cd apps/scraper && go build ./...`
Expected: pas d'erreur

**Step 3: Brancher dans `main.go`**

Modifier `apps/scraper/cmd/main.go` pour appeler `sites.RunDarkwebScraper()` à la place de `sites.RunScraper()` (l'ancien fixture `DarkThreat.go` reste dans le code, non supprimé, mais n'est plus le chemin par défaut) :

```go
// apps/scraper/cmd/main.go — remplacer les deux occurrences de sites.RunScraper()
log.Printf("Lancement du scraper (interval: %d min)", DefaultInterval)
sites.RunDarkwebScraper()

ticker := time.NewTicker(time.Duration(DefaultInterval) * time.Minute)
defer ticker.Stop()

for range ticker.C {
	log.Printf("⏰ Exécution programmée du scraper à %s", time.Now().Format("15:04:05"))
	sites.RunDarkwebScraper()
}
```

**Step 4: Ajouter les nouvelles variables d'env à l'exemple**

Ajouter à `apps/scraper/.env.example` :
```
REDIS_HOST=
REDIS_PORT=
REDIS_PASSWORD=
SITES_CONFIG_PATH=
```

**Step 5: Vérifier que tout compile**

Run: `cd apps/scraper && go build ./...`
Expected: pas d'erreur

**Step 6: Commit**

```bash
git add apps/scraper/internal/sites/Darkweb.go apps/scraper/internal/sites/fetch.go apps/scraper/cmd/main.go apps/scraper/.env.example
git commit -m "feat(scraper): orchestrateur RunDarkwebScraper, branché en chemin par défaut"
```

---

### Task 8: Câblage docker-compose + vérification manuelle de bout en bout

**Files:**
- Modify: `docker-compose.yaml` (service `scraper` : ajouter `depends_on: redis`, variables d'env `REDIS_HOST=redis`/`REDIS_PORT=6379`)

**Step 1: Modifier docker-compose.yaml**

Dans le service `scraper` (actuellement `depends_on: [mongodb]`), ajouter :
```yaml
  scraper:
    build:
      context: ./apps/scraper
      dockerfile: Dockerfile.dev
    container_name: scraper_service
    volumes:
      - ./apps/scraper:/app
    depends_on:
      - mongodb
      - redis
    environment:
      - REDIS_HOST=redis
      - REDIS_PORT=6379
    env_file:
      - ./apps/scraper/.env
```

**Step 2: Lancer la chaîne complète**

```bash
docker compose up -d tor1 tor2 tor3 redis neo4j mongodb
docker compose up -d --build crawler
# attendre la fin du crawl (le crawler s'arrête seul quand la queue Redis est vide)
docker compose up -d --build scraper
```

**Step 3: Vérifier dans les logs du scraper qu'au moins une page a été récupérée**

Utiliser `Monitor` (pas de `sleep` chaîné) pour suivre les logs :
```bash
docker logs -f scraper_service | grep --line-buffered "nouvelles URLs à scraper\|Erreur scraping"
```

**Step 4: Vérifier le contenu en Mongo**

```bash
docker exec -it mongodb_service mongosh -u devroot -p devpassword --authenticationDatabase admin \
  --eval 'db.getSiblingDB("BlueWhiteThreat").article.find({source: {$exists: true}}).pretty()'
```
Expected: au moins un document avec `url`, `source`, `scraped_at`, `content` non vide.

**Step 5: Commit**

```bash
git add docker-compose.yaml
git commit -m "chore(compose): branche Redis sur le service scraper"
```

---

## Hors périmètre de ce plan (rappel du design)

- Boucle de priorisation recrawl basée sur le classifieur (issue #9).
- Brute-force / contournement de sécurité / inscription automatique.
- Pagination ou exploration multi-pages au sein d'un même site.
