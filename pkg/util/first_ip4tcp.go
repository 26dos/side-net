package util

import "github.com/multiformats/go-multiaddr"

// FirstIP4TCP picks the first /ip4/x.x.x.x/tcp/port from a list of multiaddrs.
func FirstIP4TCP(maddrs []string) (ip, port string, ok bool) {
	for _, s := range maddrs {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil { continue }
		ipv4, err1 := ma.ValueForProtocol(multiaddr.P_IP4)
		tcp,  err2 := ma.ValueForProtocol(multiaddr.P_TCP)
		if err1 == nil && err2 == nil && ipv4 != "" && tcp != "" {
			return ipv4, tcp, true
		}
	}
	return "", "", false
}
