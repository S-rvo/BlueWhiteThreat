package deepdarkCTI

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/S-rvo/BlueWhiteThreat/internal/utils"
)

// Helper pour lire SCRAP_FILES depuis l'env (ou ALL)
func getFilesToScrape() []string {
    files := os.Getenv("SCRAP_FILES")
    if files == "" || files == "ALL" {
        // liste complète par défaut
        return []string{
            "defacement.md", "discord.md", "exploits.md", "others.md","phishing.md",
        }
    }
    // clean/trim
    list := strings.Split(files, ",")
    res := make([]string, 0, len(list))
    for _, f := range list {
        name := strings.TrimSpace(f)
        if name != "" {
            res = append(res, name)
        }
    }
    return res
}

// ScrapeAll renvoie un *seul* tableau enrichi des ajouts PR
func ScrapeAll() ([]TableEntry, error) {
    files := getFilesToScrape()
    allEntries := []TableEntry{}
    for _, file := range files {
        log.Printf("Scraping file: %s", file)
        url := fmt.Sprintf("https://raw.githubusercontent.com/fastfire/deepdarkCTI/main/%s", file)
        entries, err := ParseMarkdownTable(url, file)
        if err != nil {
            log.Printf("Erreur scraping %s : %v", file, err)
            continue
        }
        allEntries = append(allEntries, entries...)
    }

    // Tentative d'enrichissement via PR : si ça foire, on log mais on retourne quand même allEntries
    var enriched []TableEntry
    defer func() {
        if r := recover(); r != nil {
            log.Printf("Panic lors de l'enrichissement PR (ignoré): %v", r)
        }
    }()
    // Gérer les erreurs éventuelles de enrichWithPullRequests proprement
    func() {
        defer func() {
            if r := recover(); r != nil {
                log.Printf("Panic lors de l'enrichissement PR: %v (on continue avec les données brutes)", r)
            }
        }()
        enriched = enrichWithPullRequests(allEntries, files)
    }()
    if len(enriched) == 0 {
        log.Printf("Avertissement : enrichissement PR impossible, on retourne uniquement le contenu des fichiers.")
        return allEntries, nil
    }
    return enriched, nil
}

// ParseMarkdownTable lit un fichier markdown et parse la table
func ParseMarkdownTable(url, sourceFile string) ([]TableEntry, error) {
    resp, err := utils.SafeGetURL(url)
    if err != nil {
        return nil, fmt.Errorf("get %s : %w", url, err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
    }

    return parseMarkdownTableFromReader(resp.Body, sourceFile)
}

func extractNameAndUrl(cell string) (string, string) { // TODO: le faire en regexp (deja essayé, trop dur)
    // Recherche les positions des crochets et parenthèses
    startName := strings.Index(cell, "[")
    endName := strings.Index(cell, "]")
    startUrl := strings.Index(cell, "(")
    endUrl := strings.Index(cell, ")")
    if startName != -1 && endName != -1 && startUrl != -1 && endUrl != -1 && endName < startUrl {
        name := cell[startName+1 : endName]
        url := cell[startUrl+1 : endUrl]
        return name, url
    }

    fmt.Println("Aucune correspondance trouvée")
    return "", ""
}

// Coupe en cellules, en gardant les vides et retire le premier & dernier "" si la ligne commence/finit par |
func parseMarkdownColumns(line string) []string {
    cells := strings.Split(line, "|")
    // On retire la cellule vide de début/fin de ligne s'il y a, car markdown crée souvent |cell|cell|cell|
    if len(cells) > 0 && strings.TrimSpace(cells[0]) == "" {
        cells = cells[1:]
    }
    if len(cells) > 0 && strings.TrimSpace(cells[len(cells)-1]) == "" {
        cells = cells[:len(cells)-1]
    }
    // On TRIM chaque cellule mais on garde les vides :
    for i := range cells {
        cells[i] = strings.TrimSpace(cells[i])
    }
    return cells
}

func parseMarkdownTableFromReader(r io.Reader, sourceFile string) ([]TableEntry, error) {
    scanner := bufio.NewScanner(r)
    entries := []TableEntry{}
    for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Ignore header et séparateur de colonne, toujours :
		if strings.HasPrefix(line, "|---") || strings.Contains(line, "-----") {
			continue
		}
		// Ignore l'entête (1ère ligne) si "Name|Status" etc (adapte si tes titres changent)
		if strings.HasPrefix(line, "|Name|Status|Description|") {
			continue
		}
		if strings.HasPrefix(line, "|") {
			cells := parseMarkdownColumns(line)
			// On remplit en tolérant les champs vides comme il faut
			name, url, status, description := "", "", "", ""
			if len(cells) > 0 {
				name, url = extractNameAndUrl(cells[0])
			}
			if len(cells) > 1 {
				status = cells[1]
			}
			if len(cells) > 2 {
				description = cells[2]
			}
			// NE GÈRE que les vraies données (pas lignes vides/séparateurs)
			if name != "" {
				entries = append(entries, TableEntry{
					Name:        name,
					URL:         url,
					Status:      status,
					Description: description,
					SourceFile:  sourceFile,
				})				
			}
		}
	}
    if err := scanner.Err(); err != nil {
        return nil, err
    }
    return entries, nil
}

// Ajoute les ajouts des PR, sans supprimer d'existant et sans doublons
func enrichWithPullRequests(entries []TableEntry, whitelistFiles []string) []TableEntry {
    existing := make(map[string]struct{})
    for _, e := range entries {
        key := e.Name + e.URL
        existing[key] = struct{}{}
    }

    log.Printf("Vérification des PR")
    resp, err := fetchOpenPRs()
    if err != nil {
        log.Printf("Erreur récupération PR : %v", err)
        return entries
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        body, _ := io.ReadAll(resp.Body)
        log.Printf("Erreur GitHub API: %s", string(body))
        return entries
    }

    // Optionnel : bloquer si plus de quota
    if resp.Header.Get("X-RateLimit-Remaining") == "0" {
        log.Println("RATE LIMIT atteint, désactivez ou ralentissez, ou ajoutez un token !")
        return entries
    }

    var prs []struct {
        Number int    `json:"number"`
        Title  string `json:"title"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
        log.Printf("Erreur decode PR : %v", err)
        return entries
    }

    for _, pr := range prs {
        files, diffs := getPRFilesAndDiffs(pr.Number, whitelistFiles)
        // log.Printf("Nombre de diffs trouvés : %d", len(diffs))
        for i, diff := range diffs {
            // log.Printf("Traitement du diff %d, fichier: %s", i, diff.FileName)
            // log.Printf("Nombre de lignes ajoutées: %d", len(diff.AddedLines))
            for _, added := range diff.AddedLines {
                // log.Printf("Ligne brute ajoutée: %s", added)
                cells := parseMarkdownColumns(added)
                // log.Printf("Nombre de cellules après parsing: %d", len(cells))
                for i := range cells {
                    cells[i] = strings.TrimSpace(cells[i])
                }
                // Vérifie si toutes les cellules sont vides
                allEmpty := true
                for _, cell := range cells {
                    if cell != "" {
                        allEmpty = false
                        break
                    }
                }
                if allEmpty {
                    log.Printf("Ignoré: toutes les cellules sont vides")
                    continue
                }

                // Remplit les champs avec les cellules disponibles
                name, url, status, description := "", "", "", ""
                if len(cells) > 0 {
                    name, url = extractNameAndUrl(cells[0])
                }
                if len(cells) > 1 {
                    status = cells[1]
                }
                if len(cells) > 2 {
                    description = cells[2]
                }

                // Continue seulement si on a au moins un nom
                if name == "" {
                    log.Printf("Ignoré: pas de nom trouvé")
                    continue
                }

                // log.Printf("Cellules trouvées: [%s] [%s] [%s] [%s]", name, url, status, description)
                sourceFile := diff.FileName
                if sourceFile == "" && i < len(files) {
                    sourceFile = files[i]
                }
                // log.Printf("%s", name)
                entry := TableEntry{
                    Name:        name,
                    URL:         url,
                    Status:      status,
                    Description: description,
                    SourceFile:  sourceFile,
                }
                key := entry.Name + entry.URL
                if _, ok := existing[key]; !ok {
                    entries = append(entries, entry)
                    existing[key] = struct{}{}
                }
            }
        }
    }
    return entries
}

// Appel GitHub AUTH avec gestion token + logs quota.
func fetchOpenPRs() (*http.Response, error) {
    url := "https://api.github.com/repos/fastfire/deepdarkCTI/pulls?state=open&per_page=100"
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; deepdarkCTI-bot/1.0)")
    req.Header.Set("Accept", "application/vnd.github+json")

    token := utils.GetEnvOrDefault("API_GITHUB_TOKEN", "")
    if token != "" {
        req.Header.Set("Authorization", "Bearer "+token)
        log.Printf("Token GitHub configuré (longueur: %d)", len(token))
    } else {
        log.Println("ATTENTION: Pas de GITHUB_TOKEN dans les variables d'environnement (quotas très faibles).")
    }

    resp, err := utils.HttpClient.Do(req)
    if err != nil {
        return nil, err
    }
    if resp.StatusCode != 200 {
        body, _ := io.ReadAll(resp.Body)
        log.Printf("Erreur GitHub API: %s", string(body))
        return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
    }

    log.Printf("X-RateLimit-Remaining: %s / X-RateLimit-Limit: %s",
        resp.Header.Get("X-RateLimit-Remaining"),
        resp.Header.Get("X-RateLimit-Limit"))

    return resp, nil
}

func getPRFilesAndDiffs(prNumber int, whitelistFiles []string) ([]string, []PRDiff) {
    filesURL := fmt.Sprintf("https://api.github.com/repos/fastfire/deepdarkCTI/pulls/%d/files", prNumber)
    resp, err := utils.SafeGetURL(filesURL)
    if err != nil {
        log.Printf("Erreur requête fichiers PR: %v", err)
        return nil, nil
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        body, _ := io.ReadAll(resp.Body)
        log.Printf("Erreur GitHub API fichiers PR (HTTP %d): %s", resp.StatusCode, string(body))
        return nil, nil
    }

    var data []struct {
        Filename string `json:"filename"`
        Patch    string `json:"patch"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        log.Printf("Erreur décodage fichiers PR: %v", err)
        return nil, nil
    }

    var files []string
    var diffs []PRDiff
    for _, f := range data {
        for _, wanted := range whitelistFiles {
            if strings.HasSuffix(f.Filename, wanted) {
                files = append(files, f.Filename)
                added, _ := parsePatch(f.Patch)
                diffs = append(diffs, PRDiff{
                    FileName:   f.Filename,
                    AddedLines: added,
                })
                break
            }
        }
    }
    return files, diffs
}

func parsePatch(patch string) (added, removed []string) {
    scanner := bufio.NewScanner(strings.NewReader(patch))
    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "+|") && !strings.HasPrefix(line, "+++") {
            added = append(added, strings.TrimPrefix(line, "+"))
        }
        if strings.HasPrefix(line, "-|") && !strings.HasPrefix(line, "---") {
            removed = append(removed, strings.TrimPrefix(line, "-"))
        }
    }
    return
}
