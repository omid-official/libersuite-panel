package panel

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/libersuite-org/panel/database"
	"github.com/spf13/cobra"
)

var (
	dbPath    string
	configDir string
	rootCmd   *cobra.Command
)

func init() {
	// Get user home directory
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	configDir = filepath.Join(home, ".libersuite-panel")

	rootCmd = &cobra.Command{
		Use:   "panel",
		Short: "LiberSuite Panel - SSH VPN Management",
		Long:  `A CLI panel for managing SSH-based VPN services with client limits and traffic monitoring.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Create config directory if it doesn't exist
			if err := os.MkdirAll(configDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create config directory: %v\n", err)
				os.Exit(1)
			}

			// Initialize database
			if err := database.Initialize(dbPath); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
				os.Exit(1)
			}
		},
	}

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", filepath.Join(configDir, "panel.db"), "Database file path")

	// Add subcommands
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(clientCmd)
	rootCmd.AddCommand(keysCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
