#!/bin/bash

set -e

DNSTT_URL="https://dnstt.network/dnstt-server-linux-amd64"
LIBERSUITE_URL=$(curl -s https://api.github.com/repos/omid-official/libersuite-panel/releases/latest \
  | grep browser_download_url \
  | grep libersuite-panel-linux-amd64 \
  | cut -d '"' -f 4)
LIBERSUITE_SH_URL="https://raw.githubusercontent.com/omid-official/libersuite-panel/main/libersuite.sh"

BASE_DIR="$HOME/libersuite"
DNSTT_DIR="$BASE_DIR/dnstt"
LIBER_DIR="$BASE_DIR/libersuite"
CONF_FILE="$BASE_DIR/config.env"
DNSTT_RUNNER="$DNSTT_DIR/dnstt-runner.sh"
RUN_USER="$(whoami)"
BIN_TARGET="/usr/local/bin/libersuite"

read -rp "Enter domain(s), comma-separated (example: t.example.com,t.example2.com): " DOMAIN
read -rp "Enter DNSTT listen port (default: 5300): " DNSTT_PORT
read -rp "Enter Libersuite listen port (default: 2223): " LIBERSUITE_PORT
read -rp "Enter internal SSH listen port (default: 2222): " SSH_PORT
read -rp "Enter SOCKS5 listen port (default: 1080): " SOCKS_PORT

DNSTT_PORT="${DNSTT_PORT:-5300}"
LIBERSUITE_PORT="${LIBERSUITE_PORT:-2223}"
SSH_PORT="${SSH_PORT:-2222}"
SOCKS_PORT="${SOCKS_PORT:-1080}"

DOMAINS=()
while IFS=',' read -ra RAW_DOMAINS; do
  for raw_domain in "${RAW_DOMAINS[@]}"; do
    trimmed="$(echo "$raw_domain" | xargs)"
    if [[ -n "$trimmed" ]]; then
      DOMAINS+=("$trimmed")
    fi
  done
done <<< "$DOMAIN"

DOMAIN="$(IFS=,; echo "${DOMAINS[*]}")"

if [[ ${#DOMAINS[@]} -eq 0 ]]; then
  echo "At least one domain is required"
  exit 1
fi

if [[ "$LIBERSUITE_PORT" == "$SSH_PORT" || "$LIBERSUITE_PORT" == "$SOCKS_PORT" || "$SSH_PORT" == "$SOCKS_PORT" ]]; then
  echo "Ports must be unique: libersuite, ssh, and socks cannot be the same"
  exit 1
fi

DNSTT_ADDRS=""
for ((i = 0; i < ${#DOMAINS[@]}; i++)); do
  dnstt_instance_port=$((DNSTT_PORT + i))
  if [[ "$dnstt_instance_port" == "$LIBERSUITE_PORT" || "$dnstt_instance_port" == "$SSH_PORT" || "$dnstt_instance_port" == "$SOCKS_PORT" ]]; then
    echo "DNSTT port $dnstt_instance_port conflicts with libersuite/ssh/socks"
    exit 1
  fi

  if [[ -z "$DNSTT_ADDRS" ]]; then
    DNSTT_ADDRS="127.0.0.1:$dnstt_instance_port"
  else
    DNSTT_ADDRS="$DNSTT_ADDRS,127.0.0.1:$dnstt_instance_port"
  fi
done

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

cat > "$DNSTT_RUNNER" <<EOF
#!/usr/bin/env bash
set -e

trap 'kill 0' INT TERM EXIT
EOF

for ((i = 0; i < ${#DOMAINS[@]}; i++)); do
  dnstt_instance_port=$((DNSTT_PORT + i))
  domain_name="${DOMAINS[$i]}"
  echo "$DNSTT_DIR/dnstt-server-linux-amd64 -udp 127.0.0.1:$dnstt_instance_port -privkey-file $DNSTT_DIR/server.key $domain_name 127.0.0.1:$LIBERSUITE_PORT &" >> "$DNSTT_RUNNER"
done

cat >> "$DNSTT_RUNNER" <<EOF
wait -n
EOF

chmod +x "$DNSTT_RUNNER"

sudo tee /etc/systemd/system/dnstt.service > /dev/null <<EOF
[Unit]
Description=DNSTT Service (multi-domain)
After=network.target

[Service]
ExecStart=$DNSTT_RUNNER
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
ExecStart=$LIBER_DIR/panel server --port $LIBERSUITE_PORT --ssh-port $SSH_PORT --socks-port $SOCKS_PORT --dns-domain $DOMAIN --dnstt-addr $DNSTT_ADDRS
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

echo "[+] Installing libersuite command to $BIN_TARGET..."
sudo install -m 755 "$LIBER_DIR/libersuite" "$BIN_TARGET"
echo "libersuite command installed"

echo "[+] Saving config..."
cat > "$CONF_FILE" <<EOF
DOMAIN="$DOMAIN"
DNSTT_PORT="$DNSTT_PORT"
DNSTT_ADDRS="$DNSTT_ADDRS"
LIBERSUITE_PORT="$LIBERSUITE_PORT"
SSH_PORT="$SSH_PORT"
SOCKS_PORT="$SOCKS_PORT"
EOF


echo "[+] Done."