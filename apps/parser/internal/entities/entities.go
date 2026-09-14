package entities

type Entities struct {
	IOCs struct {
		IPs     []string `json:"ips" bson:"ips"`
		Hashes  []string `json:"hashes" bson:"hashes"`
		Domains []string `json:"domains" bson:"domains"`
		URLs    []string `json:"urls" bson:"urls"`
	} `json:"iocs" bson:"iocs"`
	CVEs           []string `json:"cves" bson:"cves"`
	Malware        []string `json:"malware" bson:"malware"`
	ThreatActors   []string `json:"threat_actors" bson:"threat_actors"`
	Infrastructure []string `json:"infrastructure" bson:"infrastructure"`
}

func (e Entities) IsEmpty() bool {
	return len(e.IOCs.IPs) == 0 && len(e.IOCs.Hashes) == 0 &&
		len(e.IOCs.Domains) == 0 && len(e.IOCs.URLs) == 0 &&
		len(e.CVEs) == 0 && len(e.Malware) == 0 &&
		len(e.ThreatActors) == 0 && len(e.Infrastructure) == 0
}
