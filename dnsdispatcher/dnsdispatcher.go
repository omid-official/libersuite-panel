package dnsdispatcher

import (
	"context"
	"log"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const (
	ListenAddr = "0.0.0.0:53"
)

type DnsDispatcher struct {
	domain    string
	dnsttAddr string
}

func NewDnsDispatcher(domain, dnsttAddr string) *DnsDispatcher {
	return &DnsDispatcher{
		domain:    domain + ".",
		dnsttAddr: dnsttAddr,
	}
}

func (d *DnsDispatcher) Start(ctx context.Context) error {
	server := &dns.Server{Addr: ListenAddr, Net: "udp"}

	dnsttUDP, err := net.ResolveUDPAddr("udp", d.dnsttAddr)
	if err != nil {
		log.Fatal(err)
	}

	server.Handler = dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		if len(r.Question) == 0 {
			return
		}

		qName := strings.ToLower(r.Question[0].Name)

		if strings.HasSuffix(qName, d.domain) {
			forwardDNS(w, r, dnsttUDP)
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

func forwardDNS(w dns.ResponseWriter, r *dns.Msg, target *net.UDPAddr) {
	c := dns.Client{}
	c.Timeout = 2 * time.Second

	resp, _, err := c.Exchange(r, target.String())
	if err != nil {
		return
	}

	w.WriteMsg(resp)
}
