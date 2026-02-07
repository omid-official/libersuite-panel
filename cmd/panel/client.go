package panel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"text/tabwriter"
	"time"

	"github.com/libersuite-org/panel/database"
	"github.com/libersuite-org/panel/database/models"
	"github.com/spf13/cobra"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Manage SSH VPN clients",
	Long:  `Add, remove, list, and manage SSH VPN clients.`,
}

var clientAddCmd = &cobra.Command{
	Use:   "add [username] [password]",
	Short: "Add a new client",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		username := args[0]
		password := args[1]

		trafficLimit, _ := cmd.Flags().GetInt64("traffic-limit")
		expiresIn, _ := cmd.Flags().GetInt("expires-in")

		client := &models.Client{
			Username:     username,
			Password:     password,
			TrafficLimit: trafficLimit * 1024 * 1024 * 1024, // Convert GB to bytes
			Enabled:      true,
		}

		if expiresIn > 0 {
			client.ExpiresAt = time.Now().AddDate(0, 0, expiresIn)
		}

		if err := database.DB.Create(client).Error; err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		fmt.Printf("Client '%s' created successfully (ID: %d)\n", username, client.ID)
		return nil
	},
}

var clientListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all clients",
	RunE: func(cmd *cobra.Command, args []string) error {
		var clients []models.Client
		if err := database.DB.Find(&clients).Error; err != nil {
			return fmt.Errorf("failed to retrieve clients: %w", err)
		}

		if len(clients) == 0 {
			fmt.Println("No clients found")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tUSERNAME\tSTATUS\tTRAFFIC USED\tTRAFFIC LIMIT\tEXPIRES AT")
		fmt.Fprintln(w, "--\t--------\t------\t------------\t-------------\t----------")

		for _, client := range clients {
			status := "Active"
			if !client.Enabled {
				status = "Disabled"
			} else if client.IsExpired() {
				status = "Expired"
			} else if !client.HasTrafficRemaining() {
				status = "No Traffic"
			}

			trafficUsed := formatBytes(client.TrafficUsed)
			trafficLimit := "Unlimited"
			if client.TrafficLimit > 0 {
				trafficLimit = formatBytes(client.TrafficLimit)
			}

			expiresAt := "Never"
			if !client.ExpiresAt.IsZero() {
				expiresAt = client.ExpiresAt.Format("2006-01-02")
			}

			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
				client.ID, client.Username, status, trafficUsed, trafficLimit, expiresAt)
		}

		w.Flush()
		return nil
	},
}

var clientRemoveCmd = &cobra.Command{
	Use:   "remove [username]",
	Short: "Remove a client",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		username := args[0]

		result := database.DB.Where("username = ?", username).Delete(&models.Client{})
		if result.Error != nil {
			return fmt.Errorf("failed to remove client: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			return fmt.Errorf("client '%s' not found", username)
		}

		fmt.Printf("Client '%s' removed successfully\n", username)
		return nil
	},
}

var clientEnableCmd = &cobra.Command{
	Use:   "enable [username]",
	Short: "Enable a client",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		username := args[0]

		result := database.DB.Model(&models.Client{}).Where("username = ?", username).Update("enabled", true)
		if result.Error != nil {
			return fmt.Errorf("failed to enable client: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			return fmt.Errorf("client '%s' not found", username)
		}

		fmt.Printf("Client '%s' enabled successfully\n", username)
		return nil
	},
}

var clientDisableCmd = &cobra.Command{
	Use:   "disable [username]",
	Short: "Disable a client",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		username := args[0]

		result := database.DB.Model(&models.Client{}).Where("username = ?", username).Update("enabled", false)
		if result.Error != nil {
			return fmt.Errorf("failed to disable client: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			return fmt.Errorf("client '%s' not found", username)
		}

		fmt.Printf("Client '%s' disabled successfully\n", username)
		return nil
	},
}

var clientExportCmd = &cobra.Command{
	Use:   "export [username]",
	Short: "Export client connection URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		username := args[0]

		var client models.Client
		if err := database.DB.Where("username = ?", username).First(&client).Error; err != nil {
			return fmt.Errorf("client '%s' not found", username)
		}

		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetInt("port")
		token, _ := cmd.Flags().GetString("token")
		label, _ := cmd.Flags().GetString("label")
		domain, _ := cmd.Flags().GetString("domain")
		pubkey, _ := cmd.Flags().GetString("pubkey")

		if label == "" {
			label = fmt.Sprintf("SSH %s", username)
		}

		sshConnectionURL := generateSSHURL(username, client.Password, host, port, token, label)
		dnsttConnectionURL := generateDNSTTURL(label, domain, pubkey, username, client.Password)
		fmt.Println(sshConnectionURL)
		fmt.Println(dnsttConnectionURL)
		return nil
	},
}

func init() {
	// Add flags
	clientAddCmd.Flags().Int64("traffic-limit", 0, "Traffic limit in GB (0 for unlimited)")
	clientAddCmd.Flags().Int("expires-in", 0, "Expiration in days from now (0 for never)")

	clientExportCmd.Flags().String("host", "localhost", "SSH server host")
	clientExportCmd.Flags().Int("port", 2222, "SSH server port")
	clientExportCmd.Flags().String("token", "", "Connection token/key")
	clientExportCmd.Flags().String("label", "", "Connection label")
	clientExportCmd.Flags().String("domain", "", "Dnstt domain")
	clientExportCmd.Flags().String("pubkey", "", "Public key")

	// Add subcommands
	clientCmd.AddCommand(clientAddCmd)
	clientCmd.AddCommand(clientListCmd)
	clientCmd.AddCommand(clientRemoveCmd)
	clientCmd.AddCommand(clientEnableCmd)
	clientCmd.AddCommand(clientDisableCmd)
	clientCmd.AddCommand(clientExportCmd)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func generateSSHURL(username, password, host string, port int, token, label string) string {
	// Format: ssh://username:password@host:port?token#label
	u := &url.URL{
		Scheme: "ssh",
		User:   url.UserPassword(username, password),
		Host:   fmt.Sprintf("%s:%d", host, port),
	}

	if token != "" {
		u.RawQuery = token
	}

	if label != "" {
		u.Fragment = "SSH " + label
	}

	return u.String()
}

func generateDNSTTURL(label, domain, pubkey, username, password string) string {
	// Format: {"ps":"Dnstt","addr":"8.8.8.8","ns":"domain","pubkey":"pubkey","user":"username","pass":"password"}
	data, err := json.Marshal(struct {
		Ps       string `json:"ps"`
		Addr     string `json:"addr"`
		Ns       string `json:"ns"`
		Pubkey   string `json:"pubkey"`
		Username string `json:"user"`
		Password string `json:"pass"`
	}{
		Ps:       "Dnstt " + label,
		Addr:     "8.8.8.8",
		Ns:       domain,
		Pubkey:   pubkey,
		Username: username,
		Password: password,
	})
	if err != nil {
		fmt.Println(err)
		return ""
	}

	return "dns://" + base64.StdEncoding.EncodeToString(data)
}
