package panel

import (
	"fmt"
	"path/filepath"

	"github.com/libersuite-org/panel/crypto"
	"github.com/spf13/cobra"
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage RSA keys",
	Long:  `Generate and manage RSA keys for the SSH server.`,
}

var generateKeyCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a new RSA key pair",
	Long:  `Generate a new RSA key pair for the SSH server.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		keyPath, _ := cmd.Flags().GetString("output")
		keySize, _ := cmd.Flags().GetInt("size")
		force, _ := cmd.Flags().GetBool("force")

		if keyPath == "" {
			keyPath = filepath.Join(configDir, "id_rsa")
		}

		// Check if key already exists
		if crypto.KeyExists(keyPath) && !force {
			return fmt.Errorf("key already exists at %s. Use --force to overwrite", keyPath)
		}

		if force && crypto.KeyExists(keyPath) {
			fmt.Printf("Regenerating RSA key pair at %s...\n", keyPath)
			if err := crypto.RegenerateRSAKeyPair(keyPath, keySize); err != nil {
				return fmt.Errorf("failed to regenerate key: %w", err)
			}
		} else {
			fmt.Printf("Generating RSA key pair at %s...\n", keyPath)
			if err := crypto.GenerateRSAKeyPair(keyPath, keySize); err != nil {
				return fmt.Errorf("failed to generate key: %w", err)
			}
		}

		fmt.Printf("✓ Private key: %s\n", keyPath)
		fmt.Printf("✓ Public key: %s.pub\n", keyPath)
		fmt.Printf("✓ Key size: %d bits\n", keySize)
		return nil
	},
}

var regenerateKeyCmd = &cobra.Command{
	Use:   "regenerate",
	Short: "Regenerate an existing RSA key pair",
	Long:  `Regenerate (replace) an existing RSA key pair.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		keyPath, _ := cmd.Flags().GetString("output")
		keySize, _ := cmd.Flags().GetInt("size")

		if keyPath == "" {
			keyPath = filepath.Join(configDir, "id_rsa")
		}

		fmt.Printf("Regenerating RSA key pair at %s...\n", keyPath)
		if err := crypto.RegenerateRSAKeyPair(keyPath, keySize); err != nil {
			return fmt.Errorf("failed to regenerate key: %w", err)
		}

		fmt.Printf("✓ Private key: %s\n", keyPath)
		fmt.Printf("✓ Public key: %s.pub\n", keyPath)
		fmt.Printf("✓ Key size: %d bits\n", keySize)
		fmt.Println("\nNote: You will need to restart the server to use the new key.")
		return nil
	},
}

var checkKeyCmd = &cobra.Command{
	Use:   "check",
	Short: "Check if RSA key exists",
	Long:  `Check if an RSA key pair exists at the specified path.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		keyPath, _ := cmd.Flags().GetString("path")

		if keyPath == "" {
			keyPath = filepath.Join(configDir, "id_rsa")
		}

		if crypto.KeyExists(keyPath) {
			fmt.Printf("✓ RSA key exists at %s\n", keyPath)
		} else {
			fmt.Printf("✗ RSA key does not exist at %s\n", keyPath)
		}

		return nil
	},
}

func init() {
	// Generate command flags
	generateKeyCmd.Flags().String("output", "", "Output path for the key file")
	generateKeyCmd.Flags().Int("size", 2048, "RSA key size in bits")
	generateKeyCmd.Flags().Bool("force", false, "Force overwrite if key already exists")

	// Regenerate command flags
	regenerateKeyCmd.Flags().String("output", "", "Output path for the key file")
	regenerateKeyCmd.Flags().Int("size", 2048, "RSA key size in bits")

	// Check command flags
	checkKeyCmd.Flags().String("path", "", "Path to the key file")

	// Add subcommands to keys command
	keysCmd.AddCommand(generateKeyCmd)
	keysCmd.AddCommand(regenerateKeyCmd)
	keysCmd.AddCommand(checkKeyCmd)
}
