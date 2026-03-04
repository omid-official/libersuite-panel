#!/usr/bin/env bash
set -e

BASE_DIR="$HOME/libersuite"
DNSTT_DIR="$BASE_DIR/dnstt"
SLIPSTREAM_DIR="$BASE_DIR/slipstream"
LIBER_DIR="$BASE_DIR/libersuite"
CONF_FILE="$BASE_DIR/config.env"
BIN_TARGET="/usr/local/bin/libersuite"
DNSTT_RUNNER="$DNSTT_DIR/dnstt-runner.sh"
SLIPSTREAM_RUNNER="$SLIPSTREAM_DIR/slipstream-runner.sh"

DNSTT_SERVICE="/etc/systemd/system/dnstt.service"
SLIPSTREAM_SERVICE="/etc/systemd/system/slipstream.service"
SLIPSTREAM_WATCHDOG_SERVICE="/etc/systemd/system/slipstream-watchdog.service"
SLIPSTREAM_WATCHDOG_TIMER="/etc/systemd/system/slipstream-watchdog.timer"
LIBER_SERVICE="/etc/systemd/system/libersuite.service"

DNSTT_BIN="$DNSTT_DIR/dnstt-server-linux-amd64"
SLIPSTREAM_BIN="$SLIPSTREAM_DIR/slipstream-server"
LIBER_BIN="$LIBER_DIR/panel"

INSTALL_URL="https://raw.githubusercontent.com/omid-official/libersuite-panel/main/install.sh"

STATIC_TOKEN="LCHepgjuVVy6UQRcXWdT8MFUMaAm31Xu8huIC93UZkorBtwkUw3IBDG1ZtfWMqAIOpOkjz5ulpXMq5exBiJl38b2AHQQokvpAFeOfXwqboY="

info()  { echo -e "\033[0;34m[INFO]\033[0m $1"; }
ok()    { echo -e "\033[0;32m[OK]\033[0m $1"; }
warn()  { echo -e "\033[1;33m[WARN]\033[0m $1"; }
err()   { echo -e "\033[0;31m[ERR]\033[0m $1"; exit 1; }

detect_public_ip() {
  local detected_ip=""
  local source_url
  local response=""

  for source_url in "https://api.ipify.org" "https://ifconfig.me/ip" "https://icanhazip.com"; do
    if command -v curl >/dev/null 2>&1; then
      response="$(curl -4fsSL --max-time 5 "$source_url" 2>/dev/null || true)"
    elif command -v wget >/dev/null 2>&1; then
      response="$(wget -4qO- --timeout=5 "$source_url" 2>/dev/null || true)"
    else
      break
    fi

    response="$(echo "$response" | tr -d '[:space:]')"
    if [[ -n "$response" ]]; then
      detected_ip="$response"
      break
    fi
  done

  echo "$detected_ip"
}

need_root() {
  if [[ $EUID -ne 0 ]]; then
    err "This command requires sudo"
  fi
}

load_conf() {
  if [[ -f "$CONF_FILE" ]]; then
    source "$CONF_FILE"
    # Default TUNNEL_MODE for configs created before slipstream support
    TUNNEL_MODE="${TUNNEL_MODE:-1}"
  else
    err "Config not found. Run: libersuite install"
  fi
}

use_dnstt() {
  [[ "$TUNNEL_MODE" == "1" || "$TUNNEL_MODE" == "3" ]]
}

use_slipstream() {
  [[ "$TUNNEL_MODE" == "2" || "$TUNNEL_MODE" == "3" ]]
}

save_conf() {
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
}

parse_domains() {
  DOMAINS=()
  while IFS=',' read -ra RAW_DOMAINS; do
    for raw_domain in "${RAW_DOMAINS[@]}"; do
      trimmed="$(echo "$raw_domain" | xargs)"
      if [[ -n "$trimmed" ]]; then
        DOMAINS+=("$trimmed")
      fi
    done
  done <<< "$DOMAIN"
}

build_dnstt_addrs() {
  DNSTT_ADDRS=""
  for ((i = 0; i < ${#DOMAINS[@]}; i++)); do
    dnstt_instance_port=$((DNSTT_PORT + i))
    if [[ -z "$DNSTT_ADDRS" ]]; then
      DNSTT_ADDRS="127.0.0.1:$dnstt_instance_port"
    else
      DNSTT_ADDRS="$DNSTT_ADDRS,127.0.0.1:$dnstt_instance_port"
    fi
  done
}

parse_slipstream_domains() {
  SLIPSTREAM_DOMAINS=()
  while IFS=',' read -ra RAW_DOMAINS; do
    for raw_domain in "${RAW_DOMAINS[@]}"; do
      trimmed="$(echo "$raw_domain" | xargs)"
      if [[ -n "$trimmed" ]]; then
        SLIPSTREAM_DOMAINS+=("$trimmed")
      fi
    done
  done <<< "$SLIPSTREAM_DOMAIN"
}

build_slipstream_addrs() {
  SLIPSTREAM_ADDRS=""
  for ((i = 0; i < ${#SLIPSTREAM_DOMAINS[@]}; i++)); do
    slip_instance_port=$((SLIPSTREAM_PORT + i))
    if [[ -z "$SLIPSTREAM_ADDRS" ]]; then
      SLIPSTREAM_ADDRS="127.0.0.1:$slip_instance_port"
    else
      SLIPSTREAM_ADDRS="$SLIPSTREAM_ADDRS,127.0.0.1:$slip_instance_port"
    fi
  done
}

write_dnstt_runner() {
  cat > "$DNSTT_RUNNER" <<EOF
#!/usr/bin/env bash
set -e

trap 'kill 0' INT TERM EXIT
EOF

  for ((i = 0; i < ${#DOMAINS[@]}; i++)); do
    dnstt_instance_port=$((DNSTT_PORT + i))
    domain_name="${DOMAINS[$i]}"
    echo "$DNSTT_BIN -udp 127.0.0.1:$dnstt_instance_port -privkey-file $DNSTT_DIR/server.key $domain_name 127.0.0.1:$LIBERSUITE_PORT &" >> "$DNSTT_RUNNER"
  done

  cat >> "$DNSTT_RUNNER" <<EOF
wait -n
EOF

  chmod +x "$DNSTT_RUNNER"
}

write_slipstream_runner() {
  mkdir -p "$SLIPSTREAM_DIR"

  cat > "$SLIPSTREAM_RUNNER" <<EOF
#!/usr/bin/env bash
set -e

trap 'kill 0' INT TERM EXIT
EOF

  for ((i = 0; i < ${#SLIPSTREAM_DOMAINS[@]}; i++)); do
    slip_instance_port=$((SLIPSTREAM_PORT + i))
    slip_domain="${SLIPSTREAM_DOMAINS[$i]}"
    echo "$SLIPSTREAM_BIN --dns-listen-host 127.0.0.1 --dns-listen-port $slip_instance_port --domain $slip_domain --cert $SLIPSTREAM_DIR/cert.pem --key $SLIPSTREAM_DIR/key.pem --target-address 127.0.0.1:$LIBERSUITE_PORT &" >> "$SLIPSTREAM_RUNNER"
  done

  cat >> "$SLIPSTREAM_RUNNER" <<EOF
wait -n
EOF

  chmod +x "$SLIPSTREAM_RUNNER"
}

write_slipstream_watchdog() {
  mkdir -p "$SLIPSTREAM_DIR"

  cat > "$SLIPSTREAM_DIR/watchdog.sh" <<'WATCHDOG_EOF'
#!/usr/bin/env bash
# Slipstream watchdog: probes each instance port with a DNS query.
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

  # Send a DNS A query. Any response (even SERVFAIL/REFUSED) means alive.
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
}

rewrite_services() {
  need_root
  load_conf
  parse_domains
  parse_slipstream_domains

  if ! use_dnstt && ! use_slipstream; then
    err "No tunnel mode configured. At least one of DNSTT or Slipstream is required."
  fi

  if use_dnstt && [[ ${#DOMAINS[@]} -eq 0 ]]; then
    err "At least one DNSTT domain is required"
  fi

  if use_slipstream && [[ ${#SLIPSTREAM_DOMAINS[@]} -eq 0 ]]; then
    err "At least one Slipstream domain is required"
  fi

  if use_dnstt; then
    DOMAIN="$(IFS=,; echo "${DOMAINS[*]}")"
  fi
  if use_slipstream; then
    SLIPSTREAM_DOMAIN="$(IFS=,; echo "${SLIPSTREAM_DOMAINS[*]}")"
  fi

  info "Rewriting systemd services..."

  if [[ "$LIBERSUITE_PORT" == "$SSH_PORT" || "$LIBERSUITE_PORT" == "$SOCKS_PORT" || "$SSH_PORT" == "$SOCKS_PORT" ]]; then
    err "Ports must be unique: libersuite, ssh, and socks cannot be the same"
  fi

  if use_dnstt; then
    for ((i = 0; i < ${#DOMAINS[@]}; i++)); do
      dnstt_instance_port=$((DNSTT_PORT + i))
      if [[ "$dnstt_instance_port" == "$LIBERSUITE_PORT" || "$dnstt_instance_port" == "$SSH_PORT" || "$dnstt_instance_port" == "$SOCKS_PORT" ]]; then
        err "DNSTT port $dnstt_instance_port conflicts with libersuite/ssh/socks"
      fi
    done
    build_dnstt_addrs
    write_dnstt_runner

    tee "$DNSTT_SERVICE" > /dev/null <<EOF
[Unit]
Description=DNSTT Service (multi-domain)
After=network.target

[Service]
ExecStart=$DNSTT_RUNNER
Restart=always
User=$(whoami)
WorkingDirectory=$DNSTT_DIR

[Install]
WantedBy=multi-user.target
EOF
  fi

  if use_slipstream; then
    for ((i = 0; i < ${#SLIPSTREAM_DOMAINS[@]}; i++)); do
      slip_instance_port=$((SLIPSTREAM_PORT + i))
      if [[ "$slip_instance_port" == "$LIBERSUITE_PORT" || "$slip_instance_port" == "$SSH_PORT" || "$slip_instance_port" == "$SOCKS_PORT" ]]; then
        err "Slipstream port $slip_instance_port conflicts with libersuite/ssh/socks"
      fi
    done
    build_slipstream_addrs
    write_slipstream_runner

    tee "$SLIPSTREAM_SERVICE" > /dev/null <<EOF
[Unit]
Description=Slipstream Service (multi-domain)
After=network.target

[Service]
ExecStart=$SLIPSTREAM_RUNNER
Restart=always
User=$(whoami)
WorkingDirectory=$SLIPSTREAM_DIR

[Install]
WantedBy=multi-user.target
EOF

    write_slipstream_watchdog

    tee "$SLIPSTREAM_WATCHDOG_SERVICE" > /dev/null <<EOF
[Unit]
Description=Slipstream Watchdog Health Check

[Service]
Type=oneshot
User=root
ExecStart=$SLIPSTREAM_DIR/watchdog.sh
EOF

    tee "$SLIPSTREAM_WATCHDOG_TIMER" > /dev/null <<EOF
[Unit]
Description=Run Slipstream Watchdog every 2 minutes

[Timer]
OnBootSec=60
OnUnitActiveSec=120

[Install]
WantedBy=timers.target
EOF
  fi

  # Build panel server flags
  PANEL_FLAGS="--port $LIBERSUITE_PORT --ssh-port $SSH_PORT --socks-port $SOCKS_PORT"
  if use_dnstt; then
    PANEL_FLAGS="$PANEL_FLAGS --dns-domain $DOMAIN --dnstt-addr $DNSTT_ADDRS"
  fi
  if use_slipstream; then
    PANEL_FLAGS="$PANEL_FLAGS --slipstream-domain $SLIPSTREAM_DOMAIN --slipstream-addr $SLIPSTREAM_ADDRS"
  fi

  tee "$LIBER_SERVICE" > /dev/null <<EOF
[Unit]
Description=Libersuite Panel
After=network.target

[Service]
ExecStart=$LIBER_BIN server $PANEL_FLAGS
Restart=always
User=$(whoami)
WorkingDirectory=$LIBER_DIR

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  if use_slipstream; then
    systemctl enable slipstream-watchdog.timer 2>/dev/null || true
    systemctl start slipstream-watchdog.timer 2>/dev/null || true
  fi
  ok "Services updated"
}

# ===== Helper: list active tunnel services =====
tunnel_services() {
  local svcs="libersuite"
  load_conf 2>/dev/null || true
  use_dnstt && svcs="$svcs dnstt"
  use_slipstream && svcs="$svcs slipstream"
  echo "$svcs"
}

# ===== Commands =====
install() {
  info "Running remote installer..."
  bash <(curl -fsSL "$INSTALL_URL")
}

update() {
  need_root
  info "Updating via remote installer..."
  bash <(curl -fsSL "$INSTALL_URL") update
}

uninstall() {
  need_root
  systemctl stop slipstream-watchdog.timer dnstt slipstream libersuite 2>/dev/null || true
  systemctl disable slipstream-watchdog.timer dnstt slipstream libersuite 2>/dev/null || true
  rm -f "$DNSTT_SERVICE" "$SLIPSTREAM_SERVICE" "$SLIPSTREAM_WATCHDOG_SERVICE" "$SLIPSTREAM_WATCHDOG_TIMER" "$LIBER_SERVICE"
  rm -f "$BIN_TARGET"
  systemctl daemon-reload
  rm -rf "$BASE_DIR"
  ok "Uninstalled successfully"
}

set_domain() {
  load_conf
  local target="$1"
  local value="$2"

  case "$target" in
    dnstt)
      [[ -z "$value" ]] && err "Usage: libersuite domain dnstt <t.example.com[,t2.example.com]>"
      DOMAIN="$value"
      parse_domains
      DOMAIN="$(IFS=,; echo "${DOMAINS[*]}")"
      ;;
    slipstream)
      [[ -z "$value" ]] && err "Usage: libersuite domain slipstream <s.example.com[,s2.example.com]>"
      SLIPSTREAM_DOMAIN="$value"
      parse_slipstream_domains
      SLIPSTREAM_DOMAIN="$(IFS=,; echo "${SLIPSTREAM_DOMAINS[*]}")"
      ;;
    *)
      # Legacy: treat as dnstt domain for backward compatibility
      [[ -z "$target" ]] && err "Usage: libersuite domain <dnstt|slipstream> <domain(s)>"
      DOMAIN="$target"
      parse_domains
      DOMAIN="$(IFS=,; echo "${DOMAINS[*]}")"
      ;;
  esac

  save_conf
  rewrite_services
  for svc in $(tunnel_services); do
    systemctl restart "$svc" 2>/dev/null || true
  done
  ok "Domain updated"
}

set_ports() {
  load_conf
  DNSTT_PORT="$1"
  LIBERSUITE_PORT="$2"
  SSH_PORT="${3:-$SSH_PORT}"
  SOCKS_PORT="${4:-$SOCKS_PORT}"
  [[ -z "$DNSTT_PORT" || -z "$LIBERSUITE_PORT" ]] && err "Usage: libersuite ports <dnstt_port> <libersuite_port> [ssh_port] [socks_port]"
  if [[ "$LIBERSUITE_PORT" == "$SSH_PORT" || "$LIBERSUITE_PORT" == "$SOCKS_PORT" || "$SSH_PORT" == "$SOCKS_PORT" ]]; then
    err "Ports must be unique: libersuite, ssh, and socks cannot be the same"
  fi
  save_conf
  rewrite_services
  for svc in $(tunnel_services); do
    systemctl restart "$svc" 2>/dev/null || true
  done
  ok "Ports updated"
}

start()   { need_root; for svc in $(tunnel_services); do systemctl start "$svc"; done; ok "Started"; }
stop()    { need_root; for svc in $(tunnel_services); do systemctl stop "$svc"; done; ok "Stopped"; }
restart() { need_root; for svc in $(tunnel_services); do systemctl restart "$svc"; done; ok "Restarted"; }
enable()  { need_root; for svc in $(tunnel_services); do systemctl enable "$svc"; done; ok "Enabled at boot"; }
disable() { need_root; for svc in $(tunnel_services); do systemctl disable "$svc"; done; ok "Disabled at boot"; }

logs() {
  local journal_args="-u libersuite"
  load_conf 2>/dev/null || true
  use_dnstt && journal_args="$journal_args -u dnstt"
  use_slipstream && journal_args="$journal_args -u slipstream"
  journalctl $journal_args -f
}

add_client() {
  load_conf
  USERNAME="$1"
  PASSWORD="$2"
  TRAFFIC_LIMIT="$3"
  EXPIRES_IN="$4"

  [[ -z "$USERNAME" || -z "$PASSWORD" ]] && err "Usage: libersuite client add <username> <password> [traffic_limit_gb] [expires_in_days]"

  ARGS=("client" "add" "$USERNAME" "$PASSWORD")

  if [[ -n "$TRAFFIC_LIMIT" ]]; then
    ARGS+=("--traffic-limit" "$TRAFFIC_LIMIT")
  fi

  if [[ -n "$EXPIRES_IN" ]]; then
    ARGS+=("--expires-in" "$EXPIRES_IN")
  fi

  "$LIBER_BIN" "${ARGS[@]}"
  ok "Client '$USERNAME' added"
}

list_clients() {
  load_conf
  "$LIBER_BIN" client list
}

remove_client() {
  load_conf
  USERNAME="$1"
  [[ -z "$USERNAME" ]] && err "Usage: libersuite client remove <username>"
  "$LIBER_BIN" client remove "$USERNAME"
  ok "Client '$USERNAME' removed"
}

enable_client() {
  load_conf
  USERNAME="$1"
  [[ -z "$USERNAME" ]] && err "Usage: libersuite client enable <username>"
  "$LIBER_BIN" client enable "$USERNAME"
  ok "Client '$USERNAME' enabled"
}

disable_client() {
  load_conf
  USERNAME="$1"
  [[ -z "$USERNAME" ]] && err "Usage: libersuite client disable <username>"
  "$LIBER_BIN" client disable "$USERNAME"
  ok "Client '$USERNAME' disabled"
}

export_profile() {
  load_conf
  parse_domains
  parse_slipstream_domains
  USERNAME="$1"
  IP="$2"

  [[ -z "$USERNAME" ]] && err "Usage: libersuite client export <username> [server_ip]"

  if [[ -z "$IP" ]]; then
    info "No server IP provided. Detecting public IP..."
    IP="$(detect_public_ip)"
    [[ -z "$IP" ]] && err "Could not auto-detect public IP. Usage: libersuite client export <username> <server_ip>"
    ok "Detected server IP: $IP"
  fi

  ARGS=(
    "client" "export" "$USERNAME"
    "--host" "$IP"
    "--port" "$LIBERSUITE_PORT"
    "--token" "$STATIC_TOKEN"
    "--label" "$USERNAME"
  )

  if use_dnstt && [[ ${#DOMAINS[@]} -gt 0 ]]; then
    if [[ -f "$DNSTT_DIR/server.pub" ]]; then
      PUBKEY="$(cat "$DNSTT_DIR/server.pub")"
      ARGS+=("--domain" "${DOMAINS[0]}" "--pubkey" "$PUBKEY")
    else
      warn "DNSTT public key not found: $DNSTT_DIR/server.pub"
    fi
  fi

  if use_slipstream && [[ ${#SLIPSTREAM_DOMAINS[@]} -gt 0 ]]; then
    ARGS+=("--slipstream-domain" "${SLIPSTREAM_DOMAINS[0]}")
    if [[ -f "$SLIPSTREAM_DIR/cert.pem" ]]; then
      ARGS+=("--slipstream-cert" "$SLIPSTREAM_DIR/cert.pem")
    fi
  fi

  "$LIBER_BIN" "${ARGS[@]}"
}

client_command() {
  SUBCOMMAND="$1"
  shift

  case "$SUBCOMMAND" in
    add) add_client "$@" ;;
    list) list_clients ;;
    remove) remove_client "$@" ;;
    enable) enable_client "$@" ;;
    disable) disable_client "$@" ;;
    export) export_profile "$@" ;;
    *)
      cat <<EOF

Client Management:
  libersuite client add <username> <password> [traffic_limit_gb] [expires_in_days]
  libersuite client list
  libersuite client remove <username>
  libersuite client enable <username>
  libersuite client disable <username>
  libersuite client export <username> [server_ip]

Examples:
  libersuite client add omid 1234
  libersuite client add mahan 5678 100 30    # 100GB limit, expires in 30 days
  libersuite client list
  libersuite client remove omid
  libersuite client enable mahan
  libersuite client disable omid
  libersuite client export mahan
  libersuite client export mahan 1.2.3.4

EOF
      ;;
  esac
}

help() {
  cat <<EOF

LiberSuite Manager

Commands:
  install                              Run installer
  update                               Update binaries
  uninstall                            Uninstall libersuite
  domain dnstt <name[,name2,...]>      Change DNSTT domain(s)
  domain slipstream <name[,...]>       Change Slipstream domain(s)
  ports <dnstt> <liber> [ssh] [socks]  Change ports
  start | stop | restart               Control services
  enable | disable                     Enable/disable auto-start
  logs                                 Follow logs

Client Management:
  client add <user> <pass> [traffic_gb] [expires_days]
  client list                   List all clients
  client remove <username>      Remove a client
  client enable <username>      Enable a client
  client disable <username>     Disable a client
  client export <user> [ip]     Export connection info (DNSTT + Slipstream)

For client command help: libersuite client

EOF
}

case "$1" in
  install) install ;;
  update) update ;;
  uninstall) uninstall ;;
  domain) set_domain "$2" "$3" ;;
  ports) set_ports "$2" "$3" "$4" "$5" ;;
  start) start ;;
  stop) stop ;;
  restart) restart ;;
  enable) enable ;;
  disable) disable ;;
  logs) logs ;;
  client) shift; client_command "$@" ;;
  help|"") help ;;
  *) err "Unknown command: $1" ;;
esac
