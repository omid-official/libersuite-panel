#!/bin/bash

set -e

DNSTT_URL="https://dnstt.network/dnstt-server-linux-amd64"
SLIPSTREAM_URL="https://github.com/omid-official/slipstream-rust/releases/download/v0.1.0/slipstream-server-linux-amd64"
LIBERSUITE_URL=$(curl -s https://api.github.com/repos/omid-official/libersuite-panel/releases/latest \
  | grep browser_download_url \
  | grep libersuite-panel-linux-amd64 \
  | cut -d '"' -f 4)
LIBERSUITE_SH_URL="https://raw.githubusercontent.com/omid-official/libersuite-panel/main/libersuite.sh"

BASE_DIR="$HOME/libersuite"
DNSTT_DIR="$BASE_DIR/dnstt"
SLIPSTREAM_DIR="$BASE_DIR/slipstream"
LIBER_DIR="$BASE_DIR/libersuite"
CONF_FILE="$BASE_DIR/config.env"
DNSTT_RUNNER="$DNSTT_DIR/dnstt-runner.sh"
SLIPSTREAM_RUNNER="$SLIPSTREAM_DIR/slipstream-runner.sh"
RUN_USER="$(whoami)"
BIN_TARGET="/usr/local/bin/libersuite"

# ─── Tunnel mode selection ────────────────────────────────────────────────────
echo ""
echo "Select tunnel mode:"
echo "  1) DNSTT only"
echo "  2) Slipstream only"
echo "  3) Both DNSTT and Slipstream"
read -rp "Choice [1/2/3] (default: 1): " TUNNEL_MODE
TUNNEL_MODE="${TUNNEL_MODE:-1}"

case "$TUNNEL_MODE" in
  1) USE_DNSTT=true;  USE_SLIPSTREAM=false ;;
  2) USE_DNSTT=false; USE_SLIPSTREAM=true  ;;
  3) USE_DNSTT=true;  USE_SLIPSTREAM=true  ;;
  *) echo "Invalid choice"; exit 1 ;;
esac

# ─── DNSTT domain(s) ─────────────────────────────────────────────────────────
DOMAIN=""
DOMAINS=()
DNSTT_PORT="5300"
DNSTT_ADDRS=""

if $USE_DNSTT; then
  read -rp "Enter DNSTT domain(s), comma-separated (e.g., t.example.com): " DOMAIN
  read -rp "Enter DNSTT listen port (default: 5300): " DNSTT_PORT
  DNSTT_PORT="${DNSTT_PORT:-5300}"

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
    echo "At least one DNSTT domain is required"
    exit 1
  fi
fi

# ─── Slipstream domain(s) ────────────────────────────────────────────────────
SLIPSTREAM_DOMAIN=""
SLIPSTREAM_DOMAINS=()
SLIPSTREAM_PORT="5400"
SLIPSTREAM_ADDRS=""

if $USE_SLIPSTREAM; then
  read -rp "Enter Slipstream domain(s), comma-separated (e.g., s.example.com): " SLIPSTREAM_DOMAIN
  read -rp "Enter Slipstream listen port (default: 5400): " SLIPSTREAM_PORT
  SLIPSTREAM_PORT="${SLIPSTREAM_PORT:-5400}"

  while IFS=',' read -ra RAW_DOMAINS; do
    for raw_domain in "${RAW_DOMAINS[@]}"; do
      trimmed="$(echo "$raw_domain" | xargs)"
      if [[ -n "$trimmed" ]]; then
        SLIPSTREAM_DOMAINS+=("$trimmed")
      fi
    done
  done <<< "$SLIPSTREAM_DOMAIN"
  SLIPSTREAM_DOMAIN="$(IFS=,; echo "${SLIPSTREAM_DOMAINS[*]}")"

  if [[ ${#SLIPSTREAM_DOMAINS[@]} -eq 0 ]]; then
    echo "At least one Slipstream domain is required"
    exit 1
  fi
fi

# ─── Common ports ─────────────────────────────────────────────────────────────
read -rp "Enter Libersuite listen port (default: 2223): " LIBERSUITE_PORT
read -rp "Enter internal SSH listen port (default: 2222): " SSH_PORT
read -rp "Enter SOCKS5 listen port (default: 1080): " SOCKS_PORT

LIBERSUITE_PORT="${LIBERSUITE_PORT:-2223}"
SSH_PORT="${SSH_PORT:-2222}"
SOCKS_PORT="${SOCKS_PORT:-1080}"

if [[ "$LIBERSUITE_PORT" == "$SSH_PORT" || "$LIBERSUITE_PORT" == "$SOCKS_PORT" || "$SSH_PORT" == "$SOCKS_PORT" ]]; then
  echo "Ports must be unique: libersuite, ssh, and socks cannot be the same"
  exit 1
fi

# ─── Build DNSTT addresses ───────────────────────────────────────────────────
if $USE_DNSTT; then
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
fi

# ─── Build Slipstream addresses ──────────────────────────────────────────────
if $USE_SLIPSTREAM; then
  for ((i = 0; i < ${#SLIPSTREAM_DOMAINS[@]}; i++)); do
    slip_instance_port=$((SLIPSTREAM_PORT + i))
    if [[ "$slip_instance_port" == "$LIBERSUITE_PORT" || "$slip_instance_port" == "$SSH_PORT" || "$slip_instance_port" == "$SOCKS_PORT" ]]; then
      echo "Slipstream port $slip_instance_port conflicts with libersuite/ssh/socks"
      exit 1
    fi

    if [[ -z "$SLIPSTREAM_ADDRS" ]]; then
      SLIPSTREAM_ADDRS="127.0.0.1:$slip_instance_port"
    else
      SLIPSTREAM_ADDRS="$SLIPSTREAM_ADDRS,127.0.0.1:$slip_instance_port"
    fi
  done
fi

# ─── Create folders ──────────────────────────────────────────────────────────
echo "[+] Creating folders..."
mkdir -p "$LIBER_DIR"
$USE_DNSTT && mkdir -p "$DNSTT_DIR"
$USE_SLIPSTREAM && mkdir -p "$SLIPSTREAM_DIR"

# ─── Install DNSTT ───────────────────────────────────────────────────────────
if $USE_DNSTT; then
  echo "[+] Downloading dnstt..."
  cd "$DNSTT_DIR"
  curl -L "$DNSTT_URL" -o dnstt-server-linux-amd64
  chmod +x dnstt-server-linux-amd64

  echo "[+] Generating dnstt key pair..."
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
fi

# ─── Install Slipstream ──────────────────────────────────────────────────────
if $USE_SLIPSTREAM; then
  echo "[+] Downloading slipstream-server..."
  cd "$SLIPSTREAM_DIR"
  curl -L "$SLIPSTREAM_URL" -o slipstream-server
  chmod +x slipstream-server

  echo "[+] Generating Slipstream TLS certificate..."
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
    -nodes -keyout "$SLIPSTREAM_DIR/key.pem" -out "$SLIPSTREAM_DIR/cert.pem" \
    -days 3650 -subj "/CN=slipstream" 2>/dev/null
  chmod 600 "$SLIPSTREAM_DIR/key.pem"

  echo "[+] Installing slipstream service..."
  cat > "$SLIPSTREAM_RUNNER" <<EOF
#!/usr/bin/env bash
set -e

trap 'kill 0' INT TERM EXIT
EOF

  for ((i = 0; i < ${#SLIPSTREAM_DOMAINS[@]}; i++)); do
    slip_instance_port=$((SLIPSTREAM_PORT + i))
    slip_domain="${SLIPSTREAM_DOMAINS[$i]}"
    echo "$SLIPSTREAM_DIR/slipstream-server --dns-listen-host 127.0.0.1 --dns-listen-port $slip_instance_port --domain $slip_domain --cert $SLIPSTREAM_DIR/cert.pem --key $SLIPSTREAM_DIR/key.pem --target-address 127.0.0.1:$LIBERSUITE_PORT &" >> "$SLIPSTREAM_RUNNER"
  done

  cat >> "$SLIPSTREAM_RUNNER" <<EOF
wait -n
EOF
  chmod +x "$SLIPSTREAM_RUNNER"

  sudo tee /etc/systemd/system/slipstream.service > /dev/null <<EOF
[Unit]
Description=Slipstream Service (multi-domain)
After=network.target

[Service]
ExecStart=$SLIPSTREAM_RUNNER
Restart=always
User=$RUN_USER
WorkingDirectory=$SLIPSTREAM_DIR

[Install]
WantedBy=multi-user.target
EOF

  sudo systemctl daemon-reload
  sudo systemctl enable slipstream
  sudo systemctl start slipstream

  echo "[+] Installing slipstream watchdog..."
  cat > "$SLIPSTREAM_DIR/watchdog.sh" <<'WATCHDOG_EOF'
#!/usr/bin/env bash
# Slipstream watchdog: checks each instance port with a DNS probe.
# If any port is unresponsive, restart the slipstream service.

CONF_FILE="$HOME/libersuite/config.env"
[[ -f "$CONF_FILE" ]] && source "$CONF_FILE"

SLIPSTREAM_PORT="${SLIPSTREAM_PORT:-5400}"
SLIPSTREAM_DOMAIN="${SLIPSTREAM_DOMAIN:-}"

if [[ -z "$SLIPSTREAM_DOMAIN" ]]; then
  exit 0
fi

IFS=',' read -ra DOMAINS <<< "$SLIPSTREAM_DOMAIN"
FAIL=0

for ((i = 0; i < ${#DOMAINS[@]}; i++)); do
  port=$((SLIPSTREAM_PORT + i))
  domain="${DOMAINS[$i]}"

  # Send a DNS A query to the slipstream port. We expect *some* response
  # (even SERVFAIL or REFUSED means the process is alive).
  if ! dig +short +timeout=5 +tries=1 "@127.0.0.1" -p "$port" "$domain" A >/dev/null 2>&1; then
    echo "[watchdog] slipstream port $port ($domain) unresponsive"
    FAIL=1
  fi
done

if [[ "$FAIL" -eq 1 ]]; then
  echo "[watchdog] restarting slipstream service..."
  systemctl restart slipstream
fi
WATCHDOG_EOF
  chmod +x "$SLIPSTREAM_DIR/watchdog.sh"

  sudo tee /etc/systemd/system/slipstream-watchdog.service > /dev/null <<EOF
[Unit]
Description=Slipstream Watchdog Health Check

[Service]
Type=oneshot
User=root
ExecStart=$SLIPSTREAM_DIR/watchdog.sh
EOF

  sudo tee /etc/systemd/system/slipstream-watchdog.timer > /dev/null <<EOF
[Unit]
Description=Run Slipstream Watchdog every 2 minutes

[Timer]
OnBootSec=60
OnUnitActiveSec=120

[Install]
WantedBy=timers.target
EOF

  sudo systemctl daemon-reload
  sudo systemctl enable slipstream-watchdog.timer
  sudo systemctl start slipstream-watchdog.timer
fi

# ─── Install Libersuite panel ────────────────────────────────────────────────
echo "[+] Downloading libersuite..."
cd "$LIBER_DIR"
curl -L "$LIBERSUITE_URL" -o panel
chmod +x panel

# Build panel server flags
PANEL_FLAGS="--port $LIBERSUITE_PORT --ssh-port $SSH_PORT --socks-port $SOCKS_PORT"
if $USE_DNSTT; then
  PANEL_FLAGS="$PANEL_FLAGS --dns-domain $DOMAIN --dnstt-addr $DNSTT_ADDRS"
fi
if $USE_SLIPSTREAM; then
  PANEL_FLAGS="$PANEL_FLAGS --slipstream-domain $SLIPSTREAM_DOMAIN --slipstream-addr $SLIPSTREAM_ADDRS"
fi

echo "[+] Installing libersuite panel service..."
sudo tee /etc/systemd/system/libersuite.service > /dev/null <<EOF
[Unit]
Description=Libersuite Panel Service
After=network.target

[Service]
Type=simple
ExecStart=$LIBER_DIR/panel server $PANEL_FLAGS
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
TUNNEL_MODE="$TUNNEL_MODE"
DOMAIN="$DOMAIN"
DNSTT_PORT="$DNSTT_PORT"
DNSTT_ADDRS="$DNSTT_ADDRS"
SLIPSTREAM_DOMAIN="$SLIPSTREAM_DOMAIN"
SLIPSTREAM_PORT="$SLIPSTREAM_PORT"
SLIPSTREAM_ADDRS="$SLIPSTREAM_ADDRS"
LIBERSUITE_PORT="$LIBERSUITE_PORT"
SSH_PORT="$SSH_PORT"
SOCKS_PORT="$SOCKS_PORT"
EOF

echo ""
echo "[+] Done."
if $USE_DNSTT; then
  echo "    DNSTT domains: $DOMAIN"
  echo "    DNSTT pubkey:  $(cat "$DNSTT_DIR/server.pub" 2>/dev/null || echo 'N/A')"
fi
if $USE_SLIPSTREAM; then
  echo "    Slipstream domains: $SLIPSTREAM_DOMAIN"
  CERT_FP=$(openssl x509 -noout -fingerprint -sha256 -in "$SLIPSTREAM_DIR/cert.pem" 2>/dev/null | cut -d= -f2)
  echo "    Slipstream cert SHA256: $CERT_FP"
fi
echo "    Panel port: $LIBERSUITE_PORT"