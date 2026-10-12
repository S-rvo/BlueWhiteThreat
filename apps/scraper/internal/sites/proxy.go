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
