package dnsdispatcher

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const (
	ListenAddr = "0.0.0.0:53"
)

type DnsDispatcher struct {
	routes []domainRoute
}

type domainRoute struct {
	domain   string
	dnsttUDP *net.UDPAddr
}

func NewDnsDispatcher(domains []string, dnsttAddrs []string) (*DnsDispatcher, error) {
	normalizedDomains := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.TrimSpace(strings.ToLower(domain))
		if domain == "" {
			continue
		}
		if !strings.HasSuffix(domain, ".") {
			domain += "."
		}
		normalizedDomains = append(normalizedDomains, domain)
	}

	if len(normalizedDomains) == 0 {
		return nil, fmt.Errorf("at least one domain is required")
	}

	normalizedAddrs := make([]string, 0, len(dnsttAddrs))
	for _, addr := range dnsttAddrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		normalizedAddrs = append(normalizedAddrs, addr)
	}

	if len(normalizedAddrs) == 0 {
		return nil, fmt.Errorf("at least one dnstt address is required")
	}

	if len(normalizedAddrs) != 1 && len(normalizedAddrs) != len(normalizedDomains) {
		return nil, &net.AddrError{Err: "dnstt addr count must be 1 or match dns-domain count"}
	}

	routes := make([]domainRoute, 0, len(normalizedDomains))
	for i, domain := range normalizedDomains {
		addr := normalizedAddrs[0]
		if len(normalizedAddrs) == len(normalizedDomains) {
			addr = normalizedAddrs[i]
		}

		dnsttUDP, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			return nil, err
		}

		routes = append(routes, domainRoute{domain: domain, dnsttUDP: dnsttUDP})
	}

	return &DnsDispatcher{routes: routes}, nil
}

func (d *DnsDispatcher) Start(ctx context.Context) error {
	server := &dns.Server{Addr: ListenAddr, Net: "udp"}

	server.Handler = dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		if len(r.Question) == 0 {
			return
		}

		qName := strings.ToLower(r.Question[0].Name)
		target := d.matchTarget(qName)
		if target != nil {
			forwardDNS(w, r, target)
		}
	})

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return server.Shutdown()
	case err := <-errChan:
		return err
	}
}

func (d *DnsDispatcher) matchTarget(qName string) *net.UDPAddr {
	for _, route := range d.routes {
		if strings.HasSuffix(qName, route.domain) {
			return route.dnsttUDP
		}
	}
	return nil
}

func forwardDNS(w dns.ResponseWriter, r *dns.Msg, target *net.UDPAddr) {
	c := dns.Client{}
	c.Timeout = 2 * time.Second

	resp, _, err := c.Exchange(r, target.String())
	if err != nil {
		return
	}

	w.WriteMsg(resp)
}
