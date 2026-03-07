package panel

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/libersuite-org/panel/crypto"
	"github.com/libersuite-org/panel/dnsdispatcher"
	"github.com/libersuite-org/panel/mixedserver"
	"github.com/libersuite-org/panel/runner"
	"github.com/libersuite-org/panel/socksserver"
	"github.com/libersuite-org/panel/sshserver"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the SSH VPN server",
	Long:  "Start the SSH server to accept client connections.",
	RunE: func(cmd *cobra.Command, args []string) error {
		host, err := cmd.Flags().GetString("host")
		if err != nil {
			return err
		}
		port, err := cmd.Flags().GetInt("port")
		if err != nil {
			return err
		}
		sshPort, err := cmd.Flags().GetInt("ssh-port")
		if err != nil {
			return err
		}
		socksPort, err := cmd.Flags().GetInt("socks-port")
		if err != nil {
			return err
		}
		hostKey, err := cmd.Flags().GetString("host-key")
		if err != nil {
			return err
		}
		regenerateKey, err := cmd.Flags().GetBool("regenerate-key")
		if err != nil {
			return err
		}
		keySize, err := cmd.Flags().GetInt("key-size")
		if err != nil {
			return err
		}
		dnsDomain, err := cmd.Flags().GetString("dns-domain")
		if err != nil {
			return err
		}
		dnsttPort, err := cmd.Flags().GetInt("dnstt-port")
		if err != nil {
			return err
		}
		dnsttBin, err := cmd.Flags().GetString("dnstt-bin")
		if err != nil {
			return err
		}
		dnsttKey, err := cmd.Flags().GetString("dnstt-key")
		if err != nil {
			return err
		}
		slipstreamDomain, err := cmd.Flags().GetString("slipstream-domain")
		if err != nil {
			return err
		}
		slipstreamPort, err := cmd.Flags().GetInt("slipstream-port")
		if err != nil {
			return err
		}
		slipstreamBin, err := cmd.Flags().GetString("slipstream-bin")
		if err != nil {
			return err
		}
		slipstreamCert, err := cmd.Flags().GetString("slipstream-cert")
		if err != nil {
			return err
		}
		slipstreamKeyPath, err := cmd.Flags().GetString("slipstream-key")
		if err != nil {
			return err
		}

		dnsDomains := parseDomains(dnsDomain)
		slipstreamDomains := parseDomains(slipstreamDomain)

		if len(dnsDomains) == 0 && len(slipstreamDomains) == 0 {
			return fmt.Errorf("at least one dns-domain or slipstream-domain is required")
		}

		if len(dnsDomains) > 0 && (dnsttBin == "" || dnsttKey == "") {
			return fmt.Errorf("--dnstt-bin and --dnstt-key are required when --dns-domain is set")
		}

		if len(slipstreamDomains) > 0 && (slipstreamBin == "" || slipstreamCert == "" || slipstreamKeyPath == "") {
			return fmt.Errorf("--slipstream-bin, --slipstream-cert, and --slipstream-key are required when --slipstream-domain is set")
		}

		// Compute backend addresses for the DNS dispatcher
		var dnsttAddrs, slipstreamAddrs []string
		for i := range dnsDomains {
			dnsttAddrs = append(dnsttAddrs, fmt.Sprintf("127.0.0.1:%d", dnsttPort+i))
		}
		for i := range slipstreamDomains {
			slipstreamAddrs = append(slipstreamAddrs, fmt.Sprintf("127.0.0.1:%d", slipstreamPort+i))
		}

		// Merge all domains and backend addresses for the DNS dispatcher
		allDomains := append(dnsDomains, slipstreamDomains...)
		allAddrs := append(dnsttAddrs, slipstreamAddrs...)

		if port == sshPort || port == socksPort || sshPort == socksPort {
			return fmt.Errorf("port, ssh-port, and socks-port must be different values")
		}

		if hostKey == "" {
			hostKey = filepath.Join(configDir, "id_rsa")
		}

		if regenerateKey {
			log.Printf("Regenerating RSA host key at %s...", hostKey)
			if err := crypto.RegenerateRSAKeyPair(hostKey, keySize); err != nil {
				return fmt.Errorf("failed to regenerate host key: %w", err)
			}
			log.Println("Host key regenerated")
		} else if !crypto.KeyExists(hostKey) {
			log.Printf("Generating RSA host key at %s...", hostKey)
			if err := crypto.GenerateRSAKeyPair(hostKey, keySize); err != nil {
				return fmt.Errorf("failed to generate host key: %w", err)
			}
			log.Println("Host key generated")
		} else {
			log.Printf("Using existing host key at %s", hostKey)
		}

		cfg := sshserver.Config{
			Host:    host,
			Port:    sshPort,
			HostKey: hostKey,
		}

		sshServer := sshserver.New(&cfg)
		socksServer := socksserver.New(&socksserver.Config{Host: host, Port: socksPort})
		mixedServer := mixedserver.New(&mixedserver.Config{
			Host:        host,
			Port:        port,
			BackendHost: "127.0.0.1",
			SSHPort:     sshPort,
			SOCKSPort:   socksPort,
		})
		dnsDispatcher, err := dnsdispatcher.NewDnsDispatcher(allDomains, allAddrs)
		if err != nil {
			return fmt.Errorf("failed to initialize DNS dispatcher: %w", err)
		}

		log.Printf("Starting mixed SSH/SOCKS entrypoint on %s:%d", host, port)
		log.Printf("Starting internal SSH server on %s:%d", host, sshPort)
		log.Printf("Starting internal SOCKS5 server on %s:%d", host, socksPort)
		if len(dnsDomains) > 0 {
			for i, domain := range dnsDomains {
				log.Printf("Will start dnstt for domain %s on 127.0.0.1:%d", domain, dnsttPort+i)
			}
		}
		if len(slipstreamDomains) > 0 {
			for i, domain := range slipstreamDomains {
				log.Printf("Will start slipstream for domain %s on 127.0.0.1:%d", domain, slipstreamPort+i)
			}
		}
		log.Printf("Database: %s", dbPath)
		log.Printf("Host key: %s", hostKey)
		log.Println("Press Ctrl+C to stop the server")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errChan := make(chan error, 4)
		go func() {
			if err := sshServer.Start(ctx); err != nil {
				errChan <- fmt.Errorf("SSH server error: %w", err)
			}
		}()

		go func() {
			if err := socksServer.Start(ctx); err != nil {
				errChan <- fmt.Errorf("SOCKS server error: %w", err)
			}
		}()

		go func() {
			if err := mixedServer.Start(ctx); err != nil {
				errChan <- fmt.Errorf("mixed server error: %w", err)
			}
		}()

		go func() {
			if err := dnsDispatcher.Start(ctx); err != nil {
				errChan <- fmt.Errorf("DNS dispatcher error: %w", err)
			}
		}()

		// Launch dnstt subprocesses — they auto-restart on crash.
		for i, domain := range dnsDomains {
			proc := &runner.Process{
				Name: fmt.Sprintf("dnstt[%s]", domain),
				Bin:  dnsttBin,
				Args: []string{
					"-udp", fmt.Sprintf("127.0.0.1:%d", dnsttPort+i),
					"-privkey-file", dnsttKey,
					domain,
					fmt.Sprintf("127.0.0.1:%d", port),
				},
			}
			go proc.Run(ctx)
		}

		// Launch slipstream subprocesses — they auto-restart on crash.
		for i, domain := range slipstreamDomains {
			proc := &runner.Process{
				Name: fmt.Sprintf("slipstream[%s]", domain),
				Bin:  slipstreamBin,
				Args: []string{
					"--dns-listen-host", "127.0.0.1",
					"--dns-listen-port", fmt.Sprintf("%d", slipstreamPort+i),
					"--domain", domain,
					"--cert", slipstreamCert,
					"--key", slipstreamKeyPath,
					"--target-address", fmt.Sprintf("127.0.0.1:%d", port),
				},
			}
			go proc.Run(ctx)
		}

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		select {
		case sig := <-sigChan:
			log.Printf("Received signal %v, shutting down...", sig)
		case err := <-errChan:
			return fmt.Errorf("server crashed: %w", err)
		}

		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := sshServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
		if err := socksServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("SOCKS shutdown error: %v", err)
		}
		if err := mixedServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Mixed server shutdown error: %v", err)
		}

		log.Println("Server stopped cleanly")
		return nil
	},
}

func init() {
	serverCmd.Flags().String("host", "0.0.0.0", "Host address to bind to")
	serverCmd.Flags().Int("port", 2222, "Mixed SSH/SOCKS entrypoint port")
	serverCmd.Flags().Int("ssh-port", 2223, "Internal SSH port")
	serverCmd.Flags().Int("socks-port", 1080, "SOCKS5 port to listen on")
	serverCmd.Flags().String("host-key", "", "Path to SSH host key file (will be generated if not exists)")
	serverCmd.Flags().Bool("regenerate-key", false, "Regenerate the host key even if it already exists")
	serverCmd.Flags().Int("key-size", 2048, "RSA key size in bits")
	serverCmd.Flags().String("dns-domain", "", "DNSTT domain(s), comma-separated (e.g., t.example.com,t2.example.com)")
	serverCmd.Flags().Int("dnstt-port", 5300, "DNSTT base UDP listen port (increments per domain)")
	serverCmd.Flags().String("dnstt-bin", "", "Path to the dnstt-server binary")
	serverCmd.Flags().String("dnstt-key", "", "Path to the dnstt private key file")
	serverCmd.Flags().String("slipstream-domain", "", "Slipstream domain(s), comma-separated (e.g., s.example.com)")
	serverCmd.Flags().Int("slipstream-port", 5400, "Slipstream base UDP listen port (increments per domain)")
	serverCmd.Flags().String("slipstream-bin", "", "Path to the slipstream-server binary")
	serverCmd.Flags().String("slipstream-cert", "", "Path to the slipstream TLS certificate file")
	serverCmd.Flags().String("slipstream-key", "", "Path to the slipstream TLS private key file")
}

func parseDomains(value string) []string {
	parts := strings.Split(value, ",")
	domains := make([]string, 0, len(parts))

	for _, part := range parts {
		domain := strings.TrimSpace(part)
		if domain == "" {
			continue
		}
		domains = append(domains, domain)
	}

	return domains
}
