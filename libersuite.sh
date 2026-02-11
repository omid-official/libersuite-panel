#!/usr/bin/env bash
set -e

BASE_DIR="$HOME/libersuite"
DNSTT_DIR="$BASE_DIR/dnstt"
LIBER_DIR="$BASE_DIR/libersuite"
BASHRC="$HOME/.bashrc"
CONF_FILE="$BASE_DIR/config.env"

DNSTT_SERVICE="/etc/systemd/system/dnstt.service"
LIBER_SERVICE="/etc/systemd/system/libersuite.service"

DNSTT_BIN="$DNSTT_DIR/dnstt-server-linux-amd64"
LIBER_BIN="$LIBER_DIR/panel"

INSTALL_URL="https://raw.githubusercontent.com/omid-official/libersuite-panel/main/install.sh"

STATIC_TOKEN="LCHepgjuVVy6UQRcXWdT8MFUMaAm31Xu8huIC93UZkorBtwkUw3IBDG1ZtfWMqAIOpOkjz5ulpXMq5exBiJl38b2AHQQokvpAFeOfXwqboY="

info()  { echo -e "\033[0;34m[INFO]\033[0m $1"; }
ok()    { echo -e "\033[0;32m[OK]\033[0m $1"; }
warn()  { echo -e "\033[1;33m[WARN]\033[0m $1"; }
err()   { echo -e "\033[0;31m[ERR]\033[0m $1"; exit 1; }

need_root() {
  if [[ $EUID -ne 0 ]]; then
    err "This command requires sudo"
  fi
}

load_conf() {
  if [[ -f "$CONF_FILE" ]]; then
    source "$CONF_FILE"
  else
    err "Config not found. Run: libersuite install"
  fi
}

save_conf() {
  cat > "$CONF_FILE" <<EOF
DOMAIN="$DOMAIN"
DNSTT_PORT="$DNSTT_PORT"
LIBERSUITE_PORT="$LIBERSUITE_PORT"
EOF
}

rewrite_services() {
  need_root
  load_conf

  info "Rewriting systemd services..."

  tee "$DNSTT_SERVICE" > /dev/null <<EOF
[Unit]
Description=DNSTT Service
After=network.target

[Service]
ExecStart=$DNSTT_BIN -udp 127.0.0.1:$DNSTT_PORT -privkey-file $DNSTT_DIR/server.key $DOMAIN 127.0.0.1:$LIBERSUITE_PORT
Restart=always
User=$(whoami)
WorkingDirectory=$DNSTT_DIR

[Install]
WantedBy=multi-user.target
EOF

  tee "$LIBER_SERVICE" > /dev/null <<EOF
[Unit]
Description=Libersuite Panel
After=network.target

[Service]
ExecStart=$LIBER_BIN server --port $LIBERSUITE_PORT --dns-domain $DOMAIN --dnstt-addr 127.0.0.1:$DNSTT_PORT
Restart=always
User=$(whoami)
WorkingDirectory=$LIBER_DIR

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  ok "Services updated"
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
  systemctl stop dnstt libersuite 2>/dev/null || true
  systemctl disable dnstt libersuite 2>/dev/null || true
  rm -f "$DNSTT_SERVICE" "$LIBER_SERVICE"
  systemctl daemon-reload
  rm -r "$BASE_DIR"
  ok "Uninstalled successfully"
}

set_domain() {
  load_conf
  DOMAIN="$1"
  [[ -z "$DOMAIN" ]] && err "Usage: libersuite domain <t.example.com>"
  save_conf
  rewrite_services
  systemctl restart dnstt libersuite
  ok "Domain updated"
}

set_ports() {
  load_conf
  DNSTT_PORT="$1"
  LIBERSUITE_PORT="$2"
  [[ -z "$DNSTT_PORT" || -z "$LIBERSUITE_PORT" ]] && err "Usage: libersuite ports <dnstt_port> <libersuite_port>"
  save_conf
  rewrite_services
  systemctl restart dnstt libersuite
  ok "Ports updated"
}

start()   { need_root; systemctl start dnstt libersuite; ok "Started"; }
stop()    { need_root; systemctl stop dnstt libersuite; ok "Stopped"; }
restart() { need_root; systemctl restart dnstt libersuite; ok "Restarted"; }
enable()  { need_root; systemctl enable dnstt libersuite; ok "Enabled at boot"; }
disable() { need_root; systemctl disable dnstt libersuite; ok "Disabled at boot"; }

logs() {
  journalctl -u libersuite -u dnstt -f
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
  USERNAME="$1"
  IP="$2"

  [[ -z "$USERNAME" || -z "$IP" ]] && err "Usage: libersuite client export <username> <server_ip>"

  if [[ ! -f "$DNSTT_DIR/server.pub" ]]; then
      err "Public key not found: $DNSTT_DIR/server.pub"
  fi

  PUBKEY="$(cat "$DNSTT_DIR/server.pub")"

  ARGS=(
    "client" "export" "$USERNAME"
    "--host" "$IP"
    "--port" "$LIBERSUITE_PORT"
    "--token" "$STATIC_TOKEN"
    "--label" "$USERNAME"
    "--domain" "$DOMAIN"
    "--pubkey" "$PUBKEY"
  )

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
  libersuite client export <username> <server_ip>

Examples:
  libersuite client add omid 1234
  libersuite client add mahan 5678 100 30    # 100GB limit, expires in 30 days
  libersuite client list
  libersuite client remove omid
  libersuite client enable mahan
  libersuite client disable omid
  libersuite client export mahan 1.2.3.4 t.example.com pubkey

EOF
      ;;
  esac
}

help() {
  cat <<EOF

LiberSuite Manager

Commands:
  install                       Run installer
  update                        Update binaries
  uninstall                     Uninstall libersuite
  domain <name>                 Change domain
  ports <dnstt> <liber>         Change ports
  start | stop | restart        Control services
  enable | disable              Enable/disable auto-start
  logs                          Follow logs

Client Management:
  client add <user> <pass> [traffic_gb] [expires_days]
  client list                   List all clients
  client remove <username>      Remove a client
  client enable <username>      Enable a client
  client disable <username>     Disable a client
  client export <user> <ip>

For client command help: libersuite client

EOF
}

case "$1" in
  install) install ;;
  update) update ;;
  uninstall) uninstall ;;
  domain) set_domain "$2" ;;
  ports) set_ports "$2" "$3" ;;
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
