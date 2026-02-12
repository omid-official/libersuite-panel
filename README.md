<div align="center">

# Libersuite Panel

**An SSH & dnstt tunnel management service built with Go**

[![License](https://img.shields.io/github/license/omid-official/libersuite-panel)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/omid-official/libersuite-panel)](go.mod)
[![Issues](https://img.shields.io/github/issues/omid-official/libersuite-panel)](https://github.com/omid-official/libersuite-panel/issues)
[![Stars](https://img.shields.io/github/stars/omid-official/libersuite-panel)](https://github.com/omid-official/libersuite-panel/stargazers)

</div>

---

## About
Libersuite Panel is an SSH and dnstt tunnel management service designed to simplify the administration of SSH/dnstt servers. Built with Go for performance and reliability, it provides an intuitive interface for managing SSH connections, users, and server configurations.

## Installation

### DNS Configuration

Before installing, you need to configure your DNS records. This step is required for **dnstt** to function properly.

**Example DNS Configuration:**

1. Create an A record:
   ```
   A    tns.example.com    1.2.3.4
   ```

2. Create an NS record:
   ```
   NS   t.example.com      tns.example.com
   ```

Replace `example.com` with your actual domain and `1.2.3.4` with your server's IP address.

### Quick Install

Once your DNS is configured, install Libersuite Panel with a single command:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/omid-official/libersuite-panel/master/install.sh)
```

## Usage
### Basic Commands

```bash
# Start the panel
libersuite start

# Stop the panel
libersuite stop

# Restart the panel
libersuite restart

# View logs
libersuite logs
```

### Client Management Commands
Libersuite provides commands to manage SSH VPN clients from your terminal:

```bash
# Add a new client
libersuite client add <username> <password> [--traffic-limit GB] [--expires-in days]

# List all clients
libersuite client list

# Remove a client
libersuite client remove <username>

# Enable a client
libersuite client enable <username>

# Disable a client
libersuite client disable <username>

# Export client connection URLs (SSH & dnstt)
libersuite client export <username> <server_ip>
```

#### Command Descriptions

- **client add**: Adds a new client with optional traffic-limit (in GB) and expiration (in days).
- **client list**: Lists all existing clients with their status, expiry, and usage.
- **client remove**: Removes the specified client.
- **client enable**: Enables a disabled client.
- **client disable**: Disables a client.
- **client export**: Outputs SSH and DNSTT connection URLs for the specified client. You can specify host, port, token, label, domain, and pubkey for advanced configuration.

Example to add a client with a 10GB traffic limit, valid for 30 days:
```bash
libersuite client add someone password123 --traffic-limit 10 --expires-in 30
```
Example to export a client connection:
```bash
libersuite client export someone --host 1.2.3.4 --port 2222 --domain t.example.com --pubkey "YOUR_PUBKEY"
```

## Contributing
Contributions are welcome! Feel free to open an issue or submit a PR.

## License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support
- If you find this project useful, please consider starring the repository ⭐