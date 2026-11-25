// collectseeds tire une liste bornée d'adresses .onion depuis des fichiers
// markdown publics du dépôt CTI clearnet fastfire/deepdarkCTI, et les ajoute
// à la queue Redis du crawler via AddURLToQueue (dédup/priorité/CSAM déjà
// gérés par cette fonction). Remplace l'approvisionnement manuel ponctuel de
// seed_urls.txt par un mécanisme répétable, dans le même esprit que
// collectadvisories : aucune authentification, aucun contournement de
// limite. Peut tourner soit en invocation manuelle unique, soit en mode
// -loop (relance hebdomadaire bornée) si on veut un vrai scheduling.
//
// Sans la puissance de calcul de Google, ce tool reste volontairement
// minimal : pas d'enrichissement via les PRs GitHub ouvertes (contrairement
// à l'ancien package deepdarkCTI du scraper, jamais branché), juste une
// extraction regex des adresses .onion depuis un nombre borné de fichiers.
package main

import (
	"context"
	"flag"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/seeddata"
)

// knownFiles liste les fichiers markdown du dépôt fastfire/deepdarkCTI
// confirmés présents (vérifié via l'API GitHub le 2026-11-06). Seuls ces noms
// sont acceptés par -files, pour éviter de transformer ce flag en moyen de
// faire fetcher une URL arbitraire sur raw.githubusercontent.com.
var knownFiles = map[string]bool{
	"commercial_services.md":    true,
	"counterfeit_goods.md":      true,
	"cve_most_exploited.md":     true,
	"defacement.md":             true,
	"discord.md":                true,
	"exploits.md":               true,
	"forum.md":                  true,
	"maas.md":                   true,
	"malware_samples.md":        true,
	"markets.md":                true,
	"methods.md":                true,
	"others.md":                 true,
	"phishing.md":               true,
	"ransomware_gang.md":        true,
	"rat.md":                    true,
	"search_engines.md":         true,
	"telegram_infostealer.md":   true,
	"telegram_threat_actors.md": true,
}

// defaultFiles cible les catégories deepdarkCTI les plus riches en adresses
// .onion pour amorcer un crawl darkweb CTI. Levier ① (élargir les points
// d'entrée) : au-delà des forums/marketplaces/ransomware, on moissonne aussi
// phishing, RAT, échantillons malware, contrefaçon, defacement et le fourre-tout
// "others" — chaque catégorie supplémentaire ouvre des régions du graphe que le
// link-following seul n'atteint jamais. Rester <= maxFilesAllowed.
const defaultFiles = "forum.md,markets.md,ransomware_gang.md,maas.md,exploits.md,search_engines.md,phishing.md,rat.md,malware_samples.md,counterfeit_goods.md,defacement.md,others.md"

// extraSources complète les fichiers deepdarkCTI par des annuaires et trackers
// CTI clearnet publics (levier ② : diversifier les sources de seeds). Liste en
// dur, même raison que knownFiles : ne jamais fetcher une URL arbitraire. Ces
// endpoints sont best effort -- un 404/erreur réseau est logué et la collecte
// continue sur les autres sources (vérifier/ajuster les chemins si l'un casse).
var extraSources = []string{
	"https://dark.fail/",
	"https://raw.githubusercontent.com/joshhighet/ransomwatch/main/groups.json",
	// Le README deepdarkCTI agrège des .onion de toutes les catégories (y
	// compris celles non listées dans defaultFiles) : une porte d'entrée large
	// et stable, même dépôt que rawBaseURL.
	"https://raw.githubusercontent.com/fastfire/deepdarkCTI/main/README.md",
	// ransomware.live : tracker CTI public de gangs ransomware, publie les
	// adresses .onion des sites de fuite. Best effort -- vérifier/ajuster le
	// chemin s'il casse (l'API versionne).
	"https://ransomware.live/",
}

// ctiSearchKeywords pilote le levier ③ : chaque mot-clé devient une requête de
// recherche Ahmia (clearnet) dont on moissonne les résultats .onion -- on
// emprunte la couverture d'un moteur Tor existant au lieu de ne compter que
// sur le link-following, tari dès que la composante connexe est explorée.
// Aligné sur les signaux CTI déjà priorisés par le crawler (queuePriority).
var ctiSearchKeywords = []string{
	"ransomware", "leak", "market", "forum", "stealer", "breach", "exploit",
	"carding", "fraud", "credentials", "dump", "botnet", "malware", "phishing",
	"hacking", "cvv", "database", "counterfeit", "drugs", "vendor", "trojan",
	"rat", "darkweb", "paste",
}

// Depuis 2026 Ahmia protège sa recherche par un champ caché anti-bot (nom ET
// valeur randomisés à chaque chargement, lié à un cookie de session) : sans
// lui, tout GET /search/?q= est redirigé 302 vers l'accueil. On récupère donc
// d'abord l'accueil (cookie posé dans le jar partagé du client), on en parse
// les champs via un parseur HTML, puis on les injecte dans chaque requête.
// Sur échec de parsing on ignore Ahmia (best
// effort). Ces URLs restent en dur (ahmiaHomeURL/ahmiaSearchBase) -- comme
// knownFiles, aucune URL arbitraire opérateur n'est fetchée.
const (
	ahmiaHomeURL    = "https://ahmia.fi/"
	ahmiaSearchBase = "https://ahmia.fi/search/?q="
)

const (
	rawBaseURL      = "https://raw.githubusercontent.com/fastfire/deepdarkCTI/main/"
	maxFilesAllowed = 18      // tout knownFiles peut être demandé (18 catégories deepdarkCTI)
	maxBodyBytes    = 8 << 20 // 8 MiB : couvre les pages d'annuaire et les listes/recherches Ahmia, plus volumineuses qu'un simple markdown
	fetchTimeout    = 15 * time.Second

	// loopInterval est la cadence de re-fetch PAR DÉFAUT en mode -loop,
	// surchargeable par -interval. La liste CTI source ne change pas vite,
	// mais la frontière, elle, se draine en minutes : une cadence trop lente
	// laisse le crawler à sec entre deux collectes (incident du 2026-11-10,
	// où le hebdomadaire ne re-semait pas après un wipe de l'historique).
	loopInterval = 7 * 24 * time.Hour

	// minLoopInterval protège les sources CTI (ahmia.fi, deepdarkCTI) d'un
	// re-fetch abusif : au minimum une heure entre deux collectes.
	minLoopInterval = time.Hour
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	filesFlag := flag.String("files", defaultFiles, "fichiers markdown deepdarkCTI à parcourir (bornés, séparés par des virgules)")
	dryRun := flag.Bool("dry-run", false, "n'écrit rien dans Redis, affiche seulement les adresses .onion trouvées")
	loop := flag.Bool("loop", false, fmt.Sprintf("relance périodiquement (voir -interval) au lieu de s'exécuter une seule fois"))
	interval := flag.Duration("interval", loopInterval, fmt.Sprintf("en mode -loop, délai entre deux collectes (borné à >= %s)", minLoopInterval))
	revive := flag.Int("revive-dead", 0, "réactiver au plus 100 échecs de plus de 7 jours par collecte (0 désactive)")
	searchBudget := flag.Int("search-budget", 24, "maximum de requêtes Onion Search Engine par collecte (1..100)")
	flag.Parse()
	if *searchBudget < 1 || *searchBudget > 100 {
		return fmt.Errorf("search-budget must be 1..100")
	}
	if *revive < 0 || *revive > 100 {
		return fmt.Errorf("revive-dead must be 0..100")
	}

	if *loop && *interval < minLoopInterval {
		return fmt.Errorf("interval trop court (%s < %s): protège les sources CTI d'un re-fetch abusif", *interval, minLoopInterval)
	}

	files := strings.Split(*filesFlag, ",")
	if len(files) > maxFilesAllowed {
		return fmt.Errorf("trop de fichiers demandés (%d > %d)", len(files), maxFilesAllowed)
	}
	for i, f := range files {
		files[i] = strings.TrimSpace(f)
		if !knownFiles[files[i]] {
			return fmt.Errorf("fichier inconnu ou non autorisé: %q", files[i])
		}
	}

	if !*loop {
		return runOnce(ctx, files, *dryRun, *revive, *searchBudget)
	}
	for {
		if err := runOnce(ctx, files, *dryRun, *revive, *searchBudget); err != nil {
			log.Printf("Erreur collectseeds: %v", err)
		}
		log.Printf("Prochaine collecte dans %s", *interval)
		timer := time.NewTimer(*interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// sourceURLs assemble la part statique des sources clearnet moissonnées à
// chaque passage : fichiers deepdarkCTI et annuaires/trackers (levier ②). Les
// requêtes de recherche Ahmia (levier ③) sont ajoutées séparément par
// ahmiaSearchURLs car elles exigent un jeton résolu à l'exécution. Toutes les
// URLs sont en dur -- comme knownFiles, aucune URL arbitraire opérateur.
func sourceURLs(files []string) []string {
	sources := make([]string, 0, len(files)+len(extraSources))
	for _, f := range files {
		sources = append(sources, rawBaseURL+f)
	}
	sources = append(sources, extraSources...)
	return sources
}

// ahmiaSearchURLs résout le formulaire de recherche Ahmia (voir
// ahmiaTokenPattern) et construit une URL de recherche par mot-clé CTI, jeton
// anti-bot inclus. Best effort : sur accueil injoignable ou jeton introuvable,
// log et renvoie nil -- la collecte continue sur les autres sources. Le client
// passé doit porter le cookie jar partagé pour que le cookie de session posé
// par l'accueil accompagne les requêtes de recherche.
func ahmiaSearchURLs(client *http.Client) []string {
	resp, err := client.Get(ahmiaHomeURL)
	if err != nil {
		log.Printf("Ahmia: accueil injoignable, recherche ignorée: %v", err)
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		log.Printf("Ahmia: lecture accueil échouée, recherche ignorée: %v", err)
		return nil
	}
	if resp.StatusCode != http.StatusOK || len(body) > maxBodyBytes {
		return nil
	}
	token := hiddenSearchFields(body)
	if token == "" {
		log.Printf("Ahmia: missing search fields")
		return nil
	}

	urls := make([]string, 0, len(ctiSearchKeywords))
	for _, kw := range ctiSearchKeywords {
		urls = append(urls, ahmiaSearchBase+url.QueryEscape(kw)+"&"+token)
	}
	return urls
}

// onionLocationHarvester capte l'en-tête Onion-Location sur chaque réponse HTTP
// de la chaîne, y compris les redirections 3xx intermédiaires que le client
// suit automatiquement (levier ④). Sans ce transport, un miroir Tor annoncé sur
// un 302 serait perdu car fetchOnionAddresses ne voit que la réponse finale.
type onionLocationHarvester struct {
	ctx   context.Context
	base  http.RoundTripper
	found map[string]bool
}

func (h *onionLocationHarvester) RoundTrip(req *http.Request) (*http.Response, error) {
	cleanup := func() {}
	if h.ctx != nil {
		ctx, cancel := context.WithCancel(req.Context())
		stop := context.AfterFunc(h.ctx, cancel)
		if h.ctx.Err() != nil {
			cancel()
		}
		cleanup = func() { stop(); cancel() }
		req = req.Clone(ctx)
	}
	resp, err := h.base.RoundTrip(req)
	if err != nil {
		cleanup()
		return resp, err
	}
	if resp != nil {
		extractSeeds(resp.Header.Get("Onion-Location"), h.found)
		resp.Body = &cleanupBody{ReadCloser: resp.Body, cleanup: cleanup}
	} else {
		cleanup()
	}

	return resp, err
}

func runOnce(ctx context.Context, files []string, dryRun bool, revive, searchBudget int) error {
	found := map[string]bool{}
	provenance := map[string]string{}
	sourceCounts := map[string]int{}
	attribute := func(source string) {
		if _, ok := sourceCounts[source]; !ok {
			sourceCounts[source] = 0
		}
		for u := range found {
			if provenance[u] == "" {
				provenance[u] = source
				sourceCounts[source]++
			}
		}
	}
	// Jar partagé : Ahmia lie son jeton anti-bot à un cookie de session posé
	// par l'accueil (voir ahmiaSearchURLs). Transport harvester : capte les
	// Onion-Location sur toute la chaîne de redirection (levier ④).
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("cookie jar: %w", err)
	}
	client := &http.Client{
		Timeout:   fetchTimeout,
		Jar:       jar,
		Transport: &onionLocationHarvester{ctx: ctx, base: http.DefaultTransport, found: found},
	}
	sources := sourceURLs(files)
	sources = append(sources, ahmiaSearchURLs(client)...)
	attribute(ahmiaHomeURL)
	fetchFailures := 0
	for _, source := range sources {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := fetchOnionAddresses(client, source, found)
		label := sourceLabel(source)
		attribute(label)
		if err != nil {
			log.Printf("Erreur fetch %s", label)
			fetchFailures++
			continue
		}
		log.Printf("%s: %d adresses .onion trouvées", label, n)
	}

	if key := os.Getenv("ONION_SEARCH_API_KEY"); key != "" {
		apiClient := &http.Client{Timeout: fetchTimeout}
		if err := collectSearch(ctx, apiClient, key, ctiSearchKeywords, searchBudget, 2*time.Second, found); err != nil {
			log.Printf("Onion Search Engine: %v", err)
		}
	}

	attribute("onionsearchengine-api")
	fallbackUsed := false
	if len(found) == 0 && fetchFailures == len(sources) {
		// La source réseau est totalement indisponible (dépôt déplacé, panne
		// GitHub...). Plutôt que de ne rien ajouter silencieusement, on
		// retombe sur la même liste statique de secours que le crawler
		// utilise au premier démarrage -- dédup déjà assurée par AddURLToQueue.
		log.Printf("Toutes les sources réseau ont échoué, repli sur la liste statique de secours")
		for _, line := range strings.Split(seeddata.SeedURLsFile, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			extractSeeds(line, found)
		}
		fallbackUsed = true
	}

	attribute("embedded-seeds")
	log.Printf("Total adresses .onion uniques: %d", len(found))

	// Admit before probing: transient Tor outages must not discard discoveries.
	urls := make([]string, 0, len(found))
	for u := range found {
		urls = append(urls, u)
	}
	sort.Strings(urls)

	if dryRun {
		for _, addr := range urls {
			fmt.Println(addr)
		}
		return nil
	}

	if err := db.InitRedis(); err != nil {
		return fmt.Errorf("connexion Redis: %w", err)
	}
	defer db.CloseRedis()
	if revive > 0 {
		n, err := db.ReviveDeadURLs(ctx, 7*24*time.Hour, revive)
		if err != nil {
			return err
		}
		log.Printf("Dead-letter réactivées: %d", n)
	}

	before, _ := db.GetQueueSize()
	breakdown := map[string]int{}
	admissionErrors := 0
	for _, addr := range urls {
		reason, err := db.AddURLToQueue(ctx, addr)
		if err != nil {
			admissionErrors++
			log.Printf("Erreur ajout en queue %s: %v", addr, err)
			continue
		}
		if reason == db.QueueReasonAdded || reason == db.QueueReasonCapacity || reason == "budget" {
			if err := db.RecordSeedDiscovery(ctx, addr, provenance[addr]); err != nil {
				admissionErrors++
				log.Printf("seed provenance: %v", err)
			}
		}
		breakdown[reason]++
	}
	after, _ := db.GetQueueSize()
	log.Printf("Queue size: %d -> %d", before, after)
	log.Printf("Détail: ajoutées=%d doublons=%d bloquées=%d fermes=%d capacité=%d",
		breakdown[db.QueueReasonAdded], breakdown[db.QueueReasonDuplicate],
		breakdown[db.QueueReasonBlocked], breakdown[db.QueueReasonFarm],
		breakdown[db.QueueReasonCapacity])

	for source, n := range sourceCounts {
		if err := db.RedisClient.HSet(ctx, "crawler:seed_sources", source, n).Err(); err != nil {
			log.Printf("source metrics: %v", err)
		}
	}
	recordStats(len(found), fallbackUsed, breakdown)
	if admissionErrors > 0 {
		return fmt.Errorf("%d admissions failed", admissionErrors)
	}
	return nil
}

// recordStats alimente le hash "crawler:stats" déjà exposé par /stats, pour
// pouvoir détecter depuis l'extérieur qu'une collecte a bien eu lieu (et
// quand) sans avoir à éplucher les logs.
func recordStats(totalFound int, fallbackUsed bool, breakdown map[string]int) {
	ctx := context.Background()
	for _, reason := range []string{"storage", "trap", "budget", "capacity"} {
		db.RedisClient.HSet(ctx, "crawler:stats", "collectseeds_last_"+reason, breakdown[reason])
	}
	db.RedisClient.HIncrBy(ctx, "crawler:stats", "collectseeds_runs", 1)
	db.RedisClient.HSet(ctx, "crawler:stats", "collectseeds_last_run_unix", time.Now().Unix())
	db.RedisClient.HSet(ctx, "crawler:stats", "collectseeds_last_found", totalFound)
	db.RedisClient.HSet(ctx, "crawler:stats", "collectseeds_last_added", breakdown[db.QueueReasonAdded])
	db.RedisClient.HSet(ctx, "crawler:stats", "collectseeds_last_duplicate", breakdown[db.QueueReasonDuplicate])
	db.RedisClient.HSet(ctx, "crawler:stats", "collectseeds_last_blocked", breakdown[db.QueueReasonBlocked])
	db.RedisClient.HSet(ctx, "crawler:stats", "collectseeds_last_farm", breakdown[db.QueueReasonFarm])
	if fallbackUsed {
		db.RedisClient.HIncrBy(ctx, "crawler:stats", "collectseeds_fallback_used_total", 1)
	}
}

// fetchOnionAddresses récupère une source clearnet publique (fichier markdown
// CTI, annuaire, JSON de tracker, résultats de recherche Ahmia) et en extrait
// les adresses .onion : par regex sur le corps, et via l'en-tête HTTP
// Onion-Location si la source annonce son miroir Tor (levier ④).
func fetchOnionAddresses(client *http.Client, source string, found map[string]bool) (int, error) {
	resp, err := client.Get(source)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("statut HTTP %d", resp.StatusCode)
	}

	before := len(found)
	extractSeeds(resp.Header.Get("Onion-Location"), found)

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return 0, err
	}
	if len(body) > maxBodyBytes {
		return 0, fmt.Errorf("source exceeds body limit")
	}
	extractSeeds(string(body), found)
	return len(found) - before, nil
}

// HTML parsing tolerates attribute order, single quotes and escaped values.
func hiddenSearchFields(body []byte) string {
	z := html.NewTokenizer(strings.NewReader(string(body)))
	fields := url.Values{}
	for {
		switch z.Next() {
		case html.ErrorToken:
			return fields.Encode()
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			if tok.Data != "input" {
				continue
			}
			attrs := map[string]string{}
			for _, a := range tok.Attr {
				attrs[a.Key] = a.Val
			}
			if strings.EqualFold(attrs["type"], "hidden") && attrs["name"] != "" && attrs["name"] != "q" {
				fields.Add(attrs["name"], attrs["value"])
			}
		}
	}
}

type cleanupBody struct {
	io.ReadCloser
	cleanup func()
}

func (b *cleanupBody) Close() error { defer b.cleanup(); return b.ReadCloser.Close() }

func sourceLabel(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid-source"
	}
	q := u.Query().Get("q")
	u.RawQuery = ""
	u.Fragment = ""
	if q != "" {
		u.RawQuery = url.Values{"q": {q}}.Encode()
	}
	return u.String()
}
