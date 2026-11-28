package entities

import (
	"strings"
	"unicode"
)

// Limites au-delà desquelles une valeur est une phrase ou un titre d'offre,
// pas un nom d'acteur.
const (
	maxActorWords = 6
	maxActorRunes = 60
)

// genericActorWords : rôles, catégories de produits et services que le modèle
// range à tort parmi les acteurs (« fraudsters », « Cloned cards »,
// « Hackers For Rent »). Une valeur composée uniquement de ces mots, avec au
// moins un de ces mots, est rejetée.
var genericActorWords = wordSet(`
	access account accounts academic attack attacker attackers attacks ato
	bet bitcoin black btc buy card carding cards casino chipped cloned
	counterfeit counterfeits cracked credit criminal criminals crypto
	cybercriminal cybercriminals ddos deep details dump dumps electronics
	engineering ethical fraudster fraudsters gift hack hacked hacker hackers
	hacking hacks hat hats hacktivist hacktivists hire key keys litecoin
	malware manager media money pen paypal phisher phishers phishing
	platinum prepaid rat remote rent scammer scammers seed seller sellers
	skimmer skimmers smartphone smartphones sport stolen tester testers
	tool tools transfer transfers voucher vouchers wallet wallets web
	website websites whatsapp
	android balance carders cash clone computer computers cryptocurrency cvv
	darkweb development email emails examination expert experts facebook
	hitman icloud instagram iphone mobile phone phones scam scams tracking
	visa
`)

// fillerActorWords ne suffisent pas à disqualifier un nom (« Anonymous » reste
// un acteur), mais ne le sauvent pas quand ils accompagnent des mots génériques
// (« anonymous group of hackers »).
var fillerActorWords = wordSet(`
	a and anonymous best cheap for free group in lifetime license new of on
	groups known online pro private professional real team the to usa verified with
`)

// genericActorPhrases : expressions génériques dont un mot isolé reste un nom
// plausible (« Social Hacker » est un alias de vendeur).
var genericActorPhrases = wordSet(`socialengineering`)

// KeepPlausibleActors retire des acteurs les valeurs qui ne sont pas des noms :
// adresses e-mail, phrases, rôles et catégories génériques. Un titre d'offre
// (« FraudGPT PRO - Lifetime license ») est ramené au nom qui le précède. Les
// doublons de casse sont fusionnés sur la première forme rencontrée.
func KeepPlausibleActors(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, raw := range names {
		name := actorName(raw)
		if name == "" || !plausibleActor(name) {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

// actorName normalise les espaces et coupe un titre d'offre à son nom.
func actorName(raw string) string {
	name := strings.Join(strings.Fields(raw), " ")
	for _, sep := range []string{" - ", " – ", " — ", " | "} {
		if i := strings.Index(name, sep); i > 0 {
			name = name[:i]
		}
	}
	return strings.TrimSpace(name)
}

func plausibleActor(name string) bool {
	if strings.Contains(name, "@") {
		return false
	}
	if len(strings.Fields(name)) > maxActorWords || len([]rune(name)) > maxActorRunes {
		return false
	}
	if looksLikeSentence(name) {
		return false
	}
	tokens := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(tokens) == 0 {
		return false
	}
	if genericActorPhrases[strings.Join(tokens, "")] {
		return false
	}
	generic := false
	for _, tok := range tokens {
		switch {
		case genericActorWords[tok]:
			generic = true
		case fillerActorWords[tok] || isDigits(tok):
		default:
			return true
		}
	}
	return !generic
}

// looksLikeSentence repère une ponctuation de phrase : virgule, point-virgule,
// ou « . ! ? » en fin de valeur ou suivis d'un espace. « LockBit 3.0 » passe.
func looksLikeSentence(name string) bool {
	if strings.ContainsAny(name, ",;") {
		return true
	}
	r := []rune(name)
	for i, c := range r {
		if c == '.' || c == '!' || c == '?' {
			if i == len(r)-1 || unicode.IsSpace(r[i+1]) {
				return true
			}
		}
	}
	return false
}

func isDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return s != ""
}

func wordSet(list string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(list) {
		set[w] = true
	}
	return set
}
