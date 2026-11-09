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
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/seeddata"
	"golang.org/x/net/proxy"
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

// defaultFiles cible les catégories les plus pertinentes pour amorcer un
// crawl darkweb CTI (forums, marketplaces, groupes ransomware), plutôt que
// les 5 fichiers déjà référencés (mais jamais branchés) côté scraper.
const defaultFiles = "forum.md,markets.md,ransomware_gang.md"

const (
	rawBaseURL      = "https://raw.githubusercontent.com/fastfire/deepdarkCTI/main/"
	maxFilesAllowed = 6
	maxBodyBytes    = 2 << 20 // 2 MiB : une page markdown ne dépasse jamais cette taille
	fetchTimeout    = 15 * time.Second

	// loopInterval borne la fréquence de re-fetch en mode -loop : la liste
	// CTI source ne change pas assez vite pour justifier plus qu'un passage
	// hebdomadaire, et on n'a pas la capacité réseau/Redis pour plus.
	loopInterval = 7 * 24 * time.Hour

	// livenessWorkers borne le nombre de vérifications Tor concurrentes :
	// le dépôt CTI référence des adresses historiques (marketplaces/gangs
	// saisis ou fermés) sans garantie de fraîcheur -- sans ce filtre, un
	// lot de seeds peut être presque entièrement mort (constaté le
	// 2026-11-08). Réparti sur les 3 proxies tor1-3 déjà utilisés par le
	// crawler (voir internal/crawler/crawler.go) pour ne pas saturer un seul
	// proxy.
	livenessWorkers = 12

	// livenessTimeout couvre la construction du circuit Tor, observée
	// jusqu'à ~13s pour un service vivant lors du diagnostic manuel.
	livenessTimeout = 20 * time.Second
)

var onionPattern = regexp.MustCompile(`(?i)[a-z2-7]{16,56}\.onion`)

var torProxies = []string{"tor1:9050", "tor2:9050", "tor3:9050"}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	filesFlag := flag.String("files", defaultFiles, "fichiers markdown deepdarkCTI à parcourir (bornés, séparés par des virgules)")
	dryRun := flag.Bool("dry-run", false, "n'écrit rien dans Redis, affiche seulement les adresses .onion trouvées")
	loop := flag.Bool("loop", false, fmt.Sprintf("relance toutes les %s au lieu de s'exécuter une seule fois", loopInterval))
	flag.Parse()

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
		return runOnce(files, *dryRun)
	}
	for {
		if err := runOnce(files, *dryRun); err != nil {
			log.Printf("Erreur collectseeds: %v", err)
		}
		log.Printf("Prochaine collecte dans %s", loopInterval)
		time.Sleep(loopInterval)
	}
}

func runOnce(files []string, dryRun bool) error {
	found := map[string]bool{}
	client := &http.Client{Timeout: fetchTimeout}
	fetchFailures := 0
	for _, file := range files {
		n, err := fetchOnionAddresses(client, file, found)
		if err != nil {
			log.Printf("Erreur fetch %s: %v", file, err)
			fetchFailures++
			continue
		}
		log.Printf("%s: %d adresses .onion trouvées", file, n)
	}

	fallbackUsed := false
	if len(found) == 0 && fetchFailures == len(files) {
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
			line = strings.TrimPrefix(line, "http://")
			line = strings.TrimPrefix(line, "https://")
			found[strings.ToLower(line)] = true
		}
		fallbackUsed = true
	}

	log.Printf("Total adresses .onion uniques: %d", len(found))

	found = filterLiveAddresses(found)
	log.Printf("Adresses vivantes après vérification Tor: %d", len(found))

	if dryRun {
		for addr := range found {
			fmt.Println("http://" + addr)
		}
		return nil
	}

	if err := db.InitRedis(); err != nil {
		return fmt.Errorf("connexion Redis: %w", err)
	}
	defer db.CloseRedis()

	before, _ := db.GetQueueSize()
	for addr := range found {
		if err := db.AddURLToQueue("http://" + addr); err != nil {
			log.Printf("Erreur ajout en queue %s: %v", addr, err)
		}
	}
	after, _ := db.GetQueueSize()
	log.Printf("Queue size: %d -> %d", before, after)

	recordStats(len(found), fallbackUsed)
	return nil
}

// recordStats alimente le hash "crawler:stats" déjà exposé par /stats, pour
// pouvoir détecter depuis l'extérieur qu'une collecte a bien eu lieu (et
// quand) sans avoir à éplucher les logs.
func recordStats(totalFound int, fallbackUsed bool) {
	ctx := context.Background()
	db.RedisClient.HIncrBy(ctx, "crawler:stats", "collectseeds_runs", 1)
	db.RedisClient.HSet(ctx, "crawler:stats", "collectseeds_last_run_unix", time.Now().Unix())
	db.RedisClient.HSet(ctx, "crawler:stats", "collectseeds_last_found", totalFound)
	if fallbackUsed {
		db.RedisClient.HIncrBy(ctx, "crawler:stats", "collectseeds_fallback_used_total", 1)
	}
}

func fetchOnionAddresses(client *http.Client, file string, found map[string]bool) (int, error) {
	resp, err := client.Get(rawBaseURL + file)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("statut HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return 0, err
	}

	matches := onionPattern.FindAllString(string(body), -1)
	before := len(found)
	for _, m := range matches {
		found[strings.ToLower(m)] = true
	}
	return len(found) - before, nil
}

// filterLiveAddresses sonde chaque adresse via Tor et élimine celles qui ne
// répondent pas (service saisi/fermé). N'importe quelle réponse HTTP compte
// comme "vivante", y compris une erreur applicative (4xx/5xx) : seul l'échec
// de connexion/résolution au niveau Tor (HSDir introuvable, timeout...)
// disqualifie une adresse -- même logique que le diagnostic manuel qui a
// identifié ce problème (un 500 de Facebook onion valait comme succès).
func filterLiveAddresses(addrs map[string]bool) map[string]bool {
	type probeResult struct {
		addr  string
		alive bool
	}

	jobs := make(chan string)
	results := make(chan probeResult)
	var wg sync.WaitGroup

	for i := 0; i < livenessWorkers; i++ {
		client, err := newTorHTTPClient(torProxies[i%len(torProxies)])
		if err != nil {
			log.Printf("Erreur création client Tor: %v", err)
			continue
		}
		wg.Add(1)
		go func(client *http.Client) {
			defer wg.Done()
			for addr := range jobs {
				results <- probeResult{addr: addr, alive: isReachable(client, addr)}
			}
		}(client)
	}

	go func() {
		for addr := range addrs {
			jobs <- addr
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	live := map[string]bool{}
	deadCount := 0
	for r := range results {
		if r.alive {
			live[r.addr] = true
		} else {
			deadCount++
		}
	}
	log.Printf("Vérification de vivacité Tor: %d vivantes, %d injoignables (sur %d)", len(live), deadCount, len(addrs))
	return live
}

func newTorHTTPClient(proxyAddr string) (*http.Client, error) {
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("le dialer SOCKS5 pour %s ne supporte pas le contexte", proxyAddr)
	}
	return &http.Client{
		Transport: &http.Transport{DialContext: contextDialer.DialContext},
		Timeout:   livenessTimeout,
	}, nil
}

func isReachable(client *http.Client, addr string) bool {
	req, err := http.NewRequest(http.MethodGet, "http://"+addr, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}
