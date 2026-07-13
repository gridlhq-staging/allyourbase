//go:build cell

package celltopology

import "sort"

// upstreamSet accumulates the distinct X-AYB-Upstream values observed across
// the LB-routed proof requests. nginx sets the header to $upstream_addr
// (IP:port, e.g. 172.20.0.3:8090), so distinctness — not literal container
// names — is what proves traffic spanned both AYB upstreams.
type upstreamSet struct {
	seen map[string]struct{}
}

// add records a non-empty upstream address.
func (s *upstreamSet) add(addr string) {
	if addr == "" {
		return
	}
	s.seen[addr] = struct{}{}
}

// distinct returns the number of unique upstream addresses observed.
func (s *upstreamSet) distinct() int {
	return len(s.seen)
}

// list returns the observed upstream addresses sorted, for diagnostics.
func (s *upstreamSet) list() []string {
	addrs := make([]string, 0, len(s.seen))
	for addr := range s.seen {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)
	return addrs
}
