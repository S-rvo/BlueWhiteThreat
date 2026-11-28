package entities

import (
	"strings"
	"unicode"
)

// KeepGrounded retire les indicateurs (IP, hash, domaine, URL, CVE) absents du
// texte source. Un LLM local invente parfois des URL plausibles ou recopie des
// fragments de JSON ; ces valeurs ne doivent pas entrer dans le graphe.
// La comparaison ignore la casse et un éventuel « / » final. Malware, acteurs
// et infrastructure sont des noms libres : ils sont comparés sur leurs seules
// lettres et chiffres, pour accepter « APT 28 » face à « APT28 » ou
// « LockBit-3.0 » face à « LockBit 3.0 ».
func KeepGrounded(in Entities, text string) Entities {
	lower := strings.ToLower(text)
	inText := func(v string) bool {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			return false
		}
		if strings.Contains(lower, v) {
			return true
		}
		trimmed := strings.TrimRight(v, "/")
		return trimmed != "" && trimmed != v && strings.Contains(lower, trimmed)
	}
	out := in
	out.IOCs.IPs = filter(in.IOCs.IPs, inText)
	out.IOCs.Hashes = filter(in.IOCs.Hashes, inText)
	out.IOCs.Domains = filter(in.IOCs.Domains, inText)
	out.IOCs.URLs = filter(in.IOCs.URLs, func(v string) bool { return looksLikeURL(v) && inText(v) })
	out.CVEs = filter(in.CVEs, inText)
	compact := alnum(text)
	nameInText := func(v string) bool {
		v = alnum(v)
		return v != "" && strings.Contains(compact, v)
	}
	out.Malware = filter(in.Malware, nameInText)
	out.ThreatActors = filter(in.ThreatActors, nameInText)
	out.Infrastructure = filter(in.Infrastructure, nameInText)
	return out
}

// alnum ne garde que les lettres (en minuscules) et les chiffres.
func alnum(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// looksLikeURL écarte les adresses e-mail et les fragments sans hôte
// (« ), », « hashes ») que le modèle range parfois dans les URL.
func looksLikeURL(v string) bool {
	v = strings.TrimSpace(v)
	if strings.ContainsAny(v, " \t\n\"'[]{}<>") {
		return false
	}
	host := v
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	} else if strings.Contains(host, "@") {
		return false
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if strings.Contains(host, "@") {
		host = host[strings.LastIndex(host, "@")+1:]
	}
	return strings.Contains(host, ".") && !strings.HasPrefix(host, ".") && !strings.HasSuffix(host, ".")
}
