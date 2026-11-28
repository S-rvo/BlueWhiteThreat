package entities

import (
	"slices"
	"testing"
)

// Valeurs relevées dans le graphe de production (ThreatActor, 2026-11-28).
func TestKeepPlausibleActors_DropsNoise(t *testing.T) {
	noise := []string{
		// rôles
		"fraudsters", "Professional hackers", "Hacktivists", "cybercriminals",
		"Phishers", "black hats", "Hacker", "Pen testers", "Ethical Hackers",
		"scammers in deep web", "DDoS attacker", "anonymous group of hackers",
		"Professional Hacker for Hire", "Hackers For Rent", "Hire Online Hackers",
		"Rent A Hacker",
		// produits et catégories
		"Cloned cards", "CASINO ACCOUNTS", "PLATINUM CARDS", "DUMPS CARDS",
		"REAL SELLER CARDS", "MEDIA MANAGER", "SPORT BET", "CRYPTO VOUCHER",
		"CARD-2-CARD", "CARD TO CARD", "Gift cards", "Electronics",
		"Money transfers", "Money counterfeits", "Carding", "Private keys",
		"Stolen Bitcoin", "Hacked Bitcoin", "Cracked BTC", "Litecoin Wallets",
		"Buy Cloned Cards", "Free money", "Hacked seed", "Websites Hacking",
		"WhatsApp Hacking", "Smartphones Hacking", "Academic Hacking",
		"HACK SMARTPHONE", "Social Engineering", "DDOS attack",
		"RAT (Remote access tool)", "ATO", "HACKED PAYPAL ACCOUNTS",
		"Hacked PayPal accounts and credit cards details",
		"Prepaid & Cloned Cards", "chipped cards", "Phishing websites and skimmers",
		"Instagram hacking", "Hire a hitman", "Hire a darkweb hacker", "Prepaid Visa Cards",
		"USA CVV KNOWN BALANCE", "criminal_groups", "carders", "Clone Cards",
		// phrases
		"All our products are verified on a daily basis.",
		"Our main problem is we cannot cashout",
		"We would say it?s 95% safe, but there is always a risk",
		"team of computer experts carries out paypal phishing attacks",
		// e-mails
		"gen_v3@jabber.sk", "authenticgroup@pm.me", "imperial@airmail.cc",
		// vide
		"", "   ",
	}
	if got := KeepPlausibleActors(noise); len(got) != 0 {
		t.Fatalf("bruit conservé : %q", got)
	}
}

func TestKeepPlausibleActors_KeepsNames(t *testing.T) {
	names := []string{
		"Social Hacker", "FraudGPT", "C R E A M", "True Gamer", "BunnyMoney",
		"EuroCash", "Amazon Land", "iStore", "CardsKingdom", "DarkPharma",
		"Payforia", "North_squad", "Shadow_Net_403", "White House Market",
		"briansclub", "Anonymous", "APT28", "LockBit 3.0", "Lazarus Group",
	}
	if got := KeepPlausibleActors(names); !slices.Equal(got, names) {
		t.Fatalf("noms perdus :\n got %q\nwant %q", got, names)
	}
}

func TestKeepPlausibleActors_TrimsOfferTitlesAndDedupes(t *testing.T) {
	in := []string{
		"FraudGPT", "fraudgpt", "FraudGPT - Lifetime",
		"FraudGPT PRO - Lifetime license, priority support",
		"Casino Accounts", "  Amazon   Land ", "amazon land",
	}
	want := []string{"FraudGPT", "FraudGPT PRO", "Amazon Land"}
	if got := KeepPlausibleActors(in); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestValidateAndClean_FiltersActors(t *testing.T) {
	in := Entities{ThreatActors: []string{"fraudsters", "BunnyMoney", "x@y.io"}}
	if got := ValidateAndClean(in).ThreatActors; !slices.Equal(got, []string{"BunnyMoney"}) {
		t.Fatalf("got %q", got)
	}
}
