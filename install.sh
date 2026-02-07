#!/bin/bash

set -e

DNSTT_URL="https://dnstt.network/dnstt-server-linux-amd64"
LIBERSUITE_URL="https://github.com/omid-official/libersuite-panel/releases/download/v0.1.0/libersuite-panel-linux-amd64.tar.gz"
LIBERSUITE_SH_URL="https://raw.githubusercontent.com/omid-official/libersuite-panel/main/libersuite.sh"

BASE_DIR="$HOME/libersuite"
DNSTT_DIR="$BASE_DIR/dnstt"
LIBER_DIR="$BASE_DIR/libersuite"
BASHRC="$HOME/.bashrc"
CONF_FILE="$BASE_DIR/config.env"
RUN_USER="$(whoami)"

read -rp "Enter your domain (example: t.example.com): " DOMAIN
read -rp "Enter DNSTT listen port (default: 5300): " DNSTT_PORT
read -rp "Enter Libersuite listen port (default: 2222): " LIBERSUITE_PORT

DNSTT_PORT="${DNSTT_PORT:-5300}"
LIBERSUITE_PORT="${LIBERSUITE_PORT:-2222}"

if [[ -z "$DOMAIN" ]]; then
  echo "Domain cannot be empty"
  exit 1
fi

echo "[+] Creating folders..."
mkdir -p "$DNSTT_DIR" "$LIBER_DIR"

echo "[+] Downloading dnstt..."
cd "$DNSTT_DIR"
curl -L "$DNSTT_URL" -o dnstt-server-linux-amd64
chmod +x dnstt-server-linux-amd64

echo "[+] Generating a dsntt key pair"
cd "$DNSTT_DIR"
./dnstt-server-linux-amd64 -gen-key -privkey-file server.key -pubkey-file server.pub

echo "[+] Installing dnstt service..."

sudo tee /etc/systemd/system/dnstt.service > /dev/null <<EOF
[Unit]
Description=DNSTT Service
After=network.target

[Service]
ExecStart=$DNSTT_DIR/dnstt-server-linux-amd64 -udp 127.0.0.1:$DNSTT_PORT -privkey-file $DNSTT_DIR/server.key $DOMAIN 127.0.0.1:$LIBERSUITE_PORT
Restart=always
User=$RUN_USER
WorkingDirectory=$DNSTT_DIR

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable dnstt
sudo systemctl start dnstt

echo "[+] Downloading libersuite..."
cd "$LIBER_DIR"
curl -L "$LIBERSUITE_URL" -o panel
chmod +x panel

echo "[+] Installing libersuite panel service..."
sudo tee /etc/systemd/system/libersuite.service > /dev/null <<EOF
[Unit]
Description=Libersuite Panel Service
After=network.target

[Service]
Type=simple
ExecStart=$LIBER_DIR/panel server --port $LIBERSUITE_PORT
Restart=always
RestartSec=3
User=$RUN_USER
WorkingDirectory=$LIBER_DIR

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable libersuite
sudo systemctl restart libersuite

echo "[+] Downloading libersuite.sh..."
curl -L "$LIBERSUITE_SH_URL" -o "$LIBER_DIR/libersuite"

chmod +x "$LIBER_DIR/libersuite"

if ! grep -q 'export PATH="$HOME/libersuite/libersuite:$PATH"' "$BASHRC"; then
  echo "[+] Adding libersuite to PATH in ~/.bashrc"
  {
    echo ""
    echo "# Added by libersuite installer"
    echo 'export PATH="$HOME/libersuite/libersuite:$PATH"'
  } >> "$BASHRC"
fi

echo "libersuite.sh installed"

echo "[+] Saving config..."
cat > "$CONF_FILE" <<EOF
DOMAIN="$DOMAIN"
DNSTT_PORT="$DNSTT_PORT"
LIBERSUITE_PORT="$LIBERSUITE_PORT"
EOF


echo "[+] Done."