// backfillcorpus réinjecte dans la frontière du crawler les hôtes .onion
// mentionnés en texte brut dans le corpus MongoDB du scraper mais jamais
// découverts par le crawler lui-même. Le scraper voit du contenu rendu en
// JavaScript que le crawler (HTML brut) ne verra jamais : avant que le
// pipeline continu crawler:rendered_links ne soit déployé des deux côtés,
// ce gisement historique reste inexploité (mesuré le 2026-11-10 :
// 2670 hôtes distincts inconnus de la frontière sur 4440 mentionnés).
//
// Outil one-shot, dans le même esprit que collectseeds : tout passe par
// AddURLToQueue, donc dédup, filtre CSAM, caps par hôte et budget de
// découverte s'appliquent exactement comme pour n'importe quel lien.
// L'entrée est un fichier d'hôtes (un par ligne, v3 uniquement), produit
// typiquement par une extraction regex sur la collection article.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/apps/crawler/internal/db"
)

// onionV3 n'accepte que des hôtes v3 nus : pas d'URL complète, pas de
// sous-domaine, pas de variante défangée -- l'extraction amont a déjà
// normalisé, tout le reste est rejeté et compté.
var onionV3 = regexp.MustCompile(`^[a-z2-7]{56}\.onion$`)

func main() {
	file := flag.String("file", "", "fichier d'hôtes .onion (un par ligne, obligatoire)")
	dryRun := flag.Bool("dry-run", false, "valider et compter sans rien écrire dans Redis")
	flag.Parse()
	if err := run(*file, *dryRun); err != nil {
		log.Fatalf("backfillcorpus: %v", err)
	}
}

func run(path string, dryRun bool) error {
	if path == "" {
		return fmt.Errorf("-file est obligatoire")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	hosts := map[string]bool{}
	invalid := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if line == "" {
			continue
		}
		if !onionV3.MatchString(line) {
			invalid++
			continue
		}
		hosts[line] = true
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	log.Printf("%d hôtes valides, %d lignes rejetées", len(hosts), invalid)
	if dryRun {
		log.Printf("dry-run : aucune écriture Redis")
		return nil
	}

	if err := db.InitRedis(); err != nil {
		return fmt.Errorf("connexion Redis: %w", err)
	}
	defer db.CloseRedis()

	ctx := context.Background()
	before, _ := db.GetQueueSize()
	breakdown := map[string]int{}
	for host := range hosts {
		reason, err := db.AddURLToQueue(ctx, "http://"+host)
		if err != nil {
			log.Printf("Erreur ajout en queue %s: %v", host, err)
			continue
		}
		breakdown[reason]++
	}
	after, _ := db.GetQueueSize()

	reasons := make([]string, 0, len(breakdown))
	for r := range breakdown {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)
	for _, r := range reasons {
		log.Printf("  %s: %d", r, breakdown[r])
	}
	log.Printf("Queue: %d -> %d", before, after)
	return nil
}
