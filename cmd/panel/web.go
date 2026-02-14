package panel

import (
	"fmt"

	"github.com/libersuite-org/panel/web"
	"github.com/spf13/cobra"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start the Web UI panel",
	RunE: func(cmd *cobra.Command, args []string) error {

		port, err := cmd.Flags().GetInt("port")
		if err != nil {
			return fmt.Errorf("failed to read port flag: %w", err)
		}

		username, err := cmd.Flags().GetString("user")
		if err != nil {
			return fmt.Errorf("failed to read user flag: %w", err)
		}

		password, err := cmd.Flags().GetString("pass")
		if err != nil {
			return fmt.Errorf("failed to read pass flag: %w", err)
		}

		if password == "" {
			return fmt.Errorf("admin password is required (--pass)")
		}

		if username == "" {
			return fmt.Errorf("admin username cannot be empty")
		}

		if port <= 0 || port > 65535 {
			return fmt.Errorf("invalid port number")
		}

		return web.StartServer(port, username, password)
	},
}

func init() {
	webCmd.Flags().Int("port", 8080, "Port to run the Web UI on")
	webCmd.Flags().String("user", "admin", "Admin username")
	webCmd.Flags().String("pass", "", "Admin password")

	// Register web command
	rootCmd.AddCommand(webCmd)
}
