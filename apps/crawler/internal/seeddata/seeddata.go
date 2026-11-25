// Package seeddata expose la liste statique d'adresses .onion de secours,
// partagée entre cmd/main (seed initial au démarrage) et cmd/collectseeds
// (repli si la source CTI clearnet ne répond plus).
package seeddata

import _ "embed"

//go:embed seed_urls.txt
var SeedURLsFile string
