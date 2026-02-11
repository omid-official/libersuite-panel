package panel

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/libersuite-org/panel/crypto"
	"github.com/libersuite-org/panel/dnsdispatcher"
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
		dnsttAddr, err := cmd.Flags().GetString("dnstt-addr")
		if err != nil {
			return err
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
			Port:    port,
			HostKey: hostKey,
		}

		sshServer := sshserver.New(&cfg)
		dnsDispatcher := dnsdispatcher.NewDnsDispatcher(dnsDomain, dnsttAddr)

		log.Printf("Starting SSH VPN server on %s:%d", host, port)
		log.Printf("Starting DNS dispatcher for domain: %s, forwarding to: %s", dnsDomain, dnsttAddr)
		log.Printf("Database: %s", dbPath)
		log.Printf("Host key: %s", hostKey)
		log.Println("Press Ctrl+C to stop the server")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errChan := make(chan error, 2)
		go func() {
			if err := sshServer.Start(ctx); err != nil {
				errChan <- fmt.Errorf("SSH server error: %w", err)
			}
		}()

		go func() {
			if err := dnsDispatcher.Start(ctx); err != nil {
				errChan <- fmt.Errorf("DNS dispatcher error: %w", err)
			}
		}()

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

		log.Println("Server stopped cleanly")
		return nil
	},
}

func init() {
	serverCmd.Flags().String("host", "0.0.0.0", "Host address to bind to")
	serverCmd.Flags().Int("port", 2222, "Port to listen on")
	serverCmd.Flags().String("host-key", "", "Path to SSH host key file (will be generated if not exists)")
	serverCmd.Flags().Bool("regenerate-key", false, "Regenerate the host key even if it already exists")
	serverCmd.Flags().Int("key-size", 2048, "RSA key size in bits")
	serverCmd.Flags().String("dns-domain", "", "Domain to handle DNS queries for (e.g., t.example.com.)")
	serverCmd.Flags().String("dnstt-addr", "127.0.0.1:5300", "Address to forward DNS queries to (dnstt server)")
}
