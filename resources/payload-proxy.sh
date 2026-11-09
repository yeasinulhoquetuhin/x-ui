#!/usr/bin/env bash
set -Eeuo pipefail

VERSION="1.3.0"
BASE_DIR="${XUI_PROXY_BASE_DIR:-${TDZP_BASE_DIR:-/etc/x-ui/payload-proxy}}"
INSTANCES_DIR="$BASE_DIR/instances"
FIREWALL_DIR="$BASE_DIR/firewall"
COUNTER_FILE="$BASE_DIR/next-id"
SERVICE_FILE="${XUI_PROXY_SERVICE_FILE:-/etc/systemd/system/x-ui-proxy@.service}"
SYSCTL_FILE="${XUI_PROXY_SYSCTL_FILE:-/etc/sysctl.d/99-x-ui-proxy.conf}"
SELF_BIN="${XUI_PROXY_SELF_BIN:-/usr/local/bin/x-ui-proxy}"
LEGACY_CONFIG="${TDZP_LEGACY_CONFIG:-/etc/tdzp.conf}"
LEGACY_FIREWALL="${TDZP_LEGACY_FIREWALL:-/etc/tdzp.firewall}"
LEGACY_SERVICE="${TDZP_LEGACY_SERVICE:-/etc/systemd/system/tdzp.service}"

if [[ -t 1 ]]; then
    RESET=$'\033[0m'; BOLD=$'\033[1m'; RED=$'\033[38;5;196m'; GREEN=$'\033[38;5;46m'
    YELLOW=$'\033[38;5;226m'; CYAN=$'\033[0;32m'; GRAY=$'\033[38;5;245m'; WHITE=$'\033[38;5;255m'
else
    RESET=''; BOLD=''; RED=''; GREEN=''; YELLOW=''; CYAN=''; GRAY=''; WHITE=''
fi

BOX_WIDTH=58
repeat_char() { local char=$1 count=$2 out; printf -v out '%*s' "$count" ''; printf '%s' "${out// /$char}"; }
center_line() {
    local text=$1 color=${2:-$WHITE} len left right
    len=${#text}
    (( len > BOX_WIDTH )) && text=${text:0:BOX_WIDTH} && len=${#text}
    left=$(( (BOX_WIDTH - len) / 2 ))
    right=$(( BOX_WIDTH - len - left ))
    printf '  %b║%*s%b%s%b%*s%b║%b\n' "$CYAN" "$left" '' "$color" "$text" "$RESET" "$right" '' "$CYAN" "$RESET"
}
banner() {
    [[ -t 1 ]] && clear || true
    echo
    printf '  %b╔%s╗%b\n' "$CYAN" "$(repeat_char '═' "$BOX_WIDTH")" "$RESET"
    center_line 'X-UI PAYLOAD / PROXY' "${BOLD}${CYAN}"
    center_line 'Create • Manage • Real Client IP' "$GREEN"
    printf '  %b╚%s╝%b\n' "$CYAN" "$(repeat_char '═' "$BOX_WIDTH")" "$RESET"
}
line() { printf '  %b%s%b\n' "$CYAN" "$(repeat_char '─' "$BOX_WIDTH")" "$RESET"; }
msg() {
    local kind=${1^^} text=$2 color mark
    case "$kind" in
        OK) color=$GREEN; mark='✓' ;;
        INFO) color=$CYAN; mark='•' ;;
        WARN) color=$YELLOW; mark='!' ;;
        *) color=$RED; mark='✗' ;;
    esac
    printf '  %b%s%b %s\n' "$color" "$mark" "$RESET" "$text"
}
pause_screen() { echo; read -r -p '  Press Enter to continue...' _ || true; }
require_root() { (( EUID == 0 )) || { msg error 'Run this command as root.'; exit 1; }; }
valid_port() { [[ ${1:-} =~ ^[0-9]+$ ]] && (( 10#$1 >= 1 && 10#$1 <= 65535 )); }

ensure_layout() {
    install -d -m 0755 "$BASE_DIR" "$INSTANCES_DIR" "$FIREWALL_DIR"
}
instance_file() { printf '%s/%s.conf' "$INSTANCES_DIR" "$1"; }
firewall_file() { printf '%s/%s.state' "$FIREWALL_DIR" "$1"; }
service_name() { printf 'x-ui-proxy@%s.service' "$1"; }
instance_exists() { [[ -r "$(instance_file "$1")" ]]; }
instance_get() {
    local id=$1 key=$2
    awk -F= -v k="$key" '$1==k {print $2; exit}' "$(instance_file "$id")" 2>/dev/null
}
instance_ids() {
    [[ -d "$INSTANCES_DIR" ]] || return 0
    local file name
    for file in "$INSTANCES_DIR/"*.conf; do
        [[ -f "$file" ]] || continue
        name=${file##*/}; name=${name%.conf}
        [[ "$name" =~ ^[0-9]+$ ]] && printf '%s\n' "$name" || true
    done | sort -n
}
instance_count() { instance_ids | awk 'END {print NR+0}'; }
active_count() {
    local id count=0
    while IFS= read -r id; do
        [[ -n "$id" ]] || continue
        systemctl is-active --quiet "$(service_name "$id")" 2>/dev/null && count=$((count + 1)) || true
    done < <(instance_ids)
    printf '%s\n' "$count"
}
service_active() { systemctl is-active --quiet "$(service_name "$1")" 2>/dev/null; }
port_listening() {
    command -v ss >/dev/null 2>&1 && ss -H -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "(^|:|\\])$1$"
}
public_port_owner() {
    local wanted=$1 exclude=${2:-} id port
    while IFS= read -r id; do
        [[ -n "$id" && "$id" != "$exclude" ]] || continue
        port=$(instance_get "$id" PUBLIC_PORT)
        [[ "$port" == "$wanted" ]] && { printf '%s\n' "$id"; return 0; }
    done < <(instance_ids)
    return 1
}
validate_pair() {
    local public=$1 backend=$2 exclude=${3:-} owner
    valid_port "$public" || { msg error 'Invalid public port. Use 1-65535.'; return 1; }
    valid_port "$backend" || { msg error 'Invalid backend port. Use 1-65535.'; return 1; }
    [[ "$public" != "$backend" ]] || { msg error 'Public and backend ports must be different.'; return 1; }
    owner=$(public_port_owner "$public" "$exclude" 2>/dev/null || true)
    [[ -z "$owner" ]] || { msg error "Public port $public is already used by Proxy #$owner."; return 1; }
    owner=$(public_port_owner "$backend" "$exclude" 2>/dev/null || true)
    [[ -z "$owner" ]] || { msg error "Backend port $backend is a public proxy port on Proxy #$owner."; return 1; }
    return 0
}
write_instance() {
    local id=$1 public=$2 backend=$3 tmp
    ensure_layout
    tmp=$(mktemp "$INSTANCES_DIR/.${id}.XXXXXX")
    printf 'PUBLIC_PORT=%s\nBACKEND_PORT=%s\n' "$public" "$backend" > "$tmp"
    chmod 0600 "$tmp"
    mv -f "$tmp" "$(instance_file "$id")"
}
next_instance_id() {
    local next=1 last
    if [[ -r "$COUNTER_FILE" ]]; then
        read -r next < "$COUNTER_FILE" || next=1
    else
        last=$(instance_ids | tail -n 1)
        [[ -n "$last" ]] && next=$((last + 1))
    fi
    while instance_exists "$next"; do next=$((next + 1)); done
    printf '%s\n' "$((next + 1))" > "$COUNTER_FILE"
    chmod 0600 "$COUNTER_FILE"
    printf '%s\n' "$next"
}
write_service_template() {
    local escaped_self=${SELF_BIN//%/%%}
    cat > "$SERVICE_FILE" <<EOF_SERVICE
[Unit]
Description=X-UI Payload Proxy #%i
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$escaped_self --run-proxy %i
Restart=always
RestartSec=2
LimitNOFILE=1048576
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=full

[Install]
WantedBy=multi-user.target
EOF_SERVICE
    chmod 0644 "$SERVICE_FILE"
}
apply_tuning() {
    local bbr=''
    modprobe tcp_bbr >/dev/null 2>&1 || true
    if sysctl -n net.ipv4.tcp_available_congestion_control 2>/dev/null | grep -qw bbr; then
        bbr='net.ipv4.tcp_congestion_control = bbr'
    fi
    cat > "$SYSCTL_FILE" <<EOF_SYSCTL
# Managed by X-UI Payload Proxy
net.ipv4.ip_local_port_range = 10000 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fastopen = 3
net.core.default_qdisc = fq
net.core.somaxconn = 4096
net.ipv4.tcp_max_syn_backlog = 4096
$bbr
EOF_SYSCTL
    sed -i '/^[[:space:]]*$/d' "$SYSCTL_FILE"
    sysctl -p "$SYSCTL_FILE" >/dev/null 2>&1 || true
}
state_get() { awk -F= -v k="$2" '$1==k {print $2; exit}' "$1" 2>/dev/null; }
firewall_remove_state() {
    local state_file=$1 legacy=${2:-0} method port added comment
    [[ -r "$state_file" ]] || return 0
    method=$(state_get "$state_file" METHOD)
    port=$(state_get "$state_file" PORT)
    added=$(state_get "$state_file" ADDED)
    [[ "$added" == 1 && -n "$port" ]] || { rm -f "$state_file"; return 0; }
    comment=$(state_get "$state_file" COMMENT)
    if [[ -z "$comment" ]]; then comment="XUI-PROXY:$(basename "$state_file" .state)"; fi
    [[ "$legacy" == 1 ]] && comment='TDZP'
    case "$method" in
        ufw) ufw --force delete allow "$port/tcp" >/dev/null 2>&1 || true ;;
        firewalld)
            firewall-cmd --permanent --remove-port="$port/tcp" >/dev/null 2>&1 || true
            firewall-cmd --reload >/dev/null 2>&1 || true
            ;;
        iptables) iptables -D INPUT -p tcp --dport "$port" -m comment --comment "$comment" -j ACCEPT >/dev/null 2>&1 || true ;;
    esac
    rm -f "$state_file"
}
firewall_remove() { firewall_remove_state "$(firewall_file "$1")" 0; }
firewall_open() {
    local id=$1 port=$2 method=none added=0 state_file
    state_file=$(firewall_file "$id")
    ensure_layout
    if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi '^Status: active'; then
        method=ufw
        if ! ufw status 2>/dev/null | grep -Eq "^${port}/tcp[[:space:]]+ALLOW"; then
            ufw allow "$port/tcp" comment "XUI-PROXY:$id" >/dev/null
            added=1
        fi
    elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
        method=firewalld
        if ! firewall-cmd --permanent --query-port="$port/tcp" >/dev/null 2>&1; then
            firewall-cmd --permanent --add-port="$port/tcp" >/dev/null
            firewall-cmd --reload >/dev/null
            added=1
        fi
    elif command -v iptables >/dev/null 2>&1; then
        method=iptables
        if ! iptables -C INPUT -p tcp --dport "$port" -m comment --comment "XUI-PROXY:$id" -j ACCEPT >/dev/null 2>&1; then
            iptables -I INPUT -p tcp --dport "$port" -m comment --comment "XUI-PROXY:$id" -j ACCEPT
            added=1
        fi
    fi
    printf 'METHOD=%s\nPORT=%s\nADDED=%s\nCOMMENT=XUI-PROXY:%s\n' "$method" "$port" "$added" "$id" > "$state_file"
    chmod 0600 "$state_file"
}
prepare_system() {
    require_root
    ensure_layout
    write_service_template
    apply_tuning
    systemctl daemon-reload >/dev/null 2>&1 || true
}
create_instance() {
    require_root
    local public=${1:-} backend=${2:-} id service
    validate_pair "$public" "$backend" || return 1
    if port_listening "$public"; then
        msg error "Public port $public is already listening on this server."
        return 1
    fi
    prepare_system
    id=$(next_instance_id)
    service=$(service_name "$id")
    write_instance "$id" "$public" "$backend"
    firewall_open "$id" "$public"
    if systemctl enable --now "$service" >/dev/null 2>&1 && sleep 1 && service_active "$id"; then
        CREATED_ID=$id
        return 0
    fi
    systemctl disable --now "$service" >/dev/null 2>&1 || true
    firewall_remove "$id"
    rm -f "$(instance_file "$id")"
    msg error "Proxy #$id failed to start."
    journalctl -u "$service" -n 12 --no-pager 2>/dev/null || true
    return 1
}
migrate_legacy() {
    require_root
    ensure_layout
    [[ -r "$LEGACY_CONFIG" ]] || return 0
    if (( $(instance_count) > 0 )); then
        return 0
    fi
    local public backend
    public=$(awk -F= '$1=="PUBLIC_PORT" {print $2; exit}' "$LEGACY_CONFIG" 2>/dev/null)
    backend=$(awk -F= '$1=="BACKEND_PORT" {print $2; exit}' "$LEGACY_CONFIG" 2>/dev/null)
    validate_pair "$public" "$backend" || return 1
    systemctl disable --now tdzp.service >/dev/null 2>&1 || true
    firewall_remove_state "$LEGACY_FIREWALL" 1
    rm -f "$LEGACY_SERVICE" "$LEGACY_CONFIG" "$LEGACY_FIREWALL"
    systemctl daemon-reload >/dev/null 2>&1 || true
    create_instance "$public" "$backend"
}
run_proxy() {
    local id=${1:-} public backend
    instance_exists "$id" || exit 1
    public=$(instance_get "$id" PUBLIC_PORT)
    backend=$(instance_get "$id" BACKEND_PORT)
    valid_port "$public" && valid_port "$backend" || exit 1

    exec python3 - "$public" "$backend" <<'PYTHON_PROXY'
import asyncio
import contextlib
import json
import os
from pathlib import Path
import signal
import socket
import sys

PUBLIC_PORT = int(sys.argv[1])
BACKEND_PORT = int(sys.argv[2])
HEADER_END = b"\r\n\r\n"
RESPONSE = (
    b"HTTP/1.1 101 Switching Protocols\r\n"
    b"Upgrade: websocket\r\n"
    b"Connection: Upgrade\r\n\r\n"
)


def tune(writer):
    sock = writer.get_extra_info("socket")
    if sock is None:
        return
    with contextlib.suppress(OSError):
        sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
    with contextlib.suppress(OSError):
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_KEEPALIVE, 1)


async def close(writer):
    if writer is None:
        return
    writer.close()
    with contextlib.suppress(Exception):
        await writer.wait_closed()


async def read_header(reader):
    data = bytearray()
    while HEADER_END not in data:
        if len(data) >= 65536:
            raise ValueError("header too large")
        chunk = await asyncio.wait_for(reader.read(4096), timeout=8)
        if not chunk:
            raise ConnectionError("incomplete header")
        data.extend(chunk)
    end = data.index(HEADER_END) + 4
    return bytes(data[end:])


async def relay(reader, writer):
    try:
        while True:
            data = await reader.read(65536)
            if not data:
                break
            writer.write(data)
            await writer.drain()
    except (ConnectionError, OSError, asyncio.CancelledError):
        pass


async def client_handler(client_reader, client_writer):
    backend_writer = None
    ip_record = None
    try:
        tune(client_writer)
        pending = await read_header(client_reader)
        backend_reader, backend_writer = await asyncio.open_connection("127.0.0.1", BACKEND_PORT)
        tune(backend_writer)
        # XUI_PROXY_REAL_CLIENT_IP_V2
        # No protocol bytes are added or rewritten. The core checks the live
        # backend socket inode before using this private client-IP record.
        peer = client_writer.get_extra_info("peername")
        local = backend_writer.get_extra_info("sockname")
        sock = backend_writer.get_extra_info("socket")
        # Retain the original private channel for compatibility with patched.3.
        directory = Path(os.environ.get("XUI_PROXY_IP_DIR", os.environ.get("TDZ_XRAY_PROXY_IP_DIR", "/run/tdzp-client-ips")))
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        if directory.stat().st_mode & 0o077:
            raise ValueError("unsafe client-IP directory permissions")
        ip_record = directory / f"{BACKEND_PORT}-{local[1]}.json"
        info = {"ip": peer[0], "pid": os.getpid(), "fd": sock.fileno(),
                "socket": os.readlink(f"/proc/self/fd/{sock.fileno()}")}
        temporary = ip_record.with_suffix(f".{os.getpid()}.tmp")
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as output:
            json.dump(info, output)
        temporary.replace(ip_record)
        client_writer.write(RESPONSE)
        await client_writer.drain()
        if pending:
            backend_writer.write(pending)
            await backend_writer.drain()
        tasks = {
            asyncio.create_task(relay(client_reader, backend_writer)),
            asyncio.create_task(relay(backend_reader, client_writer)),
        }
        done, pending_tasks = await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
        for task in pending_tasks:
            task.cancel()
        await asyncio.gather(*done, *pending_tasks, return_exceptions=True)
    except (asyncio.TimeoutError, ConnectionError, OSError, ValueError):
        pass
    finally:
        if ip_record is not None:
            with contextlib.suppress(OSError):
                ip_record.unlink()
        await close(backend_writer)
        await close(client_writer)


async def main():
    server = await asyncio.start_server(
        client_handler,
        "0.0.0.0",
        PUBLIC_PORT,
        backlog=512,
        reuse_address=True,
        limit=131072,
    )
    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT):
        with contextlib.suppress(NotImplementedError):
            loop.add_signal_handler(sig, stop.set)
    async with server:
        await stop.wait()


asyncio.run(main())
PYTHON_PROXY
}
card_row() {
    local label=$1 value=$2 color=${3:-$WHITE} used pad
    used=$((2 + 18 + 1 + ${#value}))
    pad=$((BOX_WIDTH - used))
    (( pad < 0 )) && pad=0
    printf '  %b│%b  %-18s %b%s%b%*s%b│%b\n' \
        "$CYAN" "$RESET" "$label" "$color" "$value" "$RESET" "$pad" '' "$CYAN" "$RESET"
}
show_proxy_card() {
    local id=$1 public backend status status_color listener listener_color backend_state backend_color title fill
    public=$(instance_get "$id" PUBLIC_PORT)
    backend=$(instance_get "$id" BACKEND_PORT)
    if service_active "$id"; then status='ACTIVE'; status_color=$GREEN; else status='INACTIVE'; status_color=$RED; fi
    if port_listening "$public"; then listener='LISTENING'; listener_color=$GREEN; else listener='NOT LISTENING'; listener_color=$RED; fi
    if port_listening "$backend"; then backend_state='AVAILABLE'; backend_color=$GREEN; else backend_state='NOT DETECTED'; backend_color=$YELLOW; fi
    title=" Proxy #$id "
    fill=$((BOX_WIDTH - ${#title}))
    (( fill < 0 )) && fill=0
    printf '  %b┌%s%s┐%b\n' "$CYAN" "$title" "$(repeat_char '─' "$fill")" "$RESET"
    card_row 'Public Port:' "$public" "$WHITE"
    card_row 'Backend Port:' "$backend" "$WHITE"
    card_row 'Service Status:' "$status" "$status_color"
    card_row 'Public Listener:' "$listener" "$listener_color"
    card_row 'Backend Listener:' "$backend_state" "$backend_color"
    printf '  %b└%s┘%b\n' "$CYAN" "$(repeat_char '─' "$BOX_WIDTH")" "$RESET"
}
prompt_pair() {
    local public backend
    while true; do
        read -r -p '  Choose Public Port: ' public
        valid_port "$public" || { msg error 'Invalid port. Use 1-65535.'; continue; }
        break
    done
    while true; do
        read -r -p '  Choose Backend Port: ' backend
        valid_port "$backend" || { msg error 'Invalid port. Use 1-65535.'; continue; }
        validate_pair "$public" "$backend" || continue
        break
    done
    SELECTED_PUBLIC=$public
    SELECTED_BACKEND=$backend
}
create_proxy_screen() {
    banner
    echo
    msg info 'Create a new public-to-backend proxy.'
    echo
    prompt_pair
    echo
    printf '  Creating proxy... '
    if create_instance "$SELECTED_PUBLIC" "$SELECTED_BACKEND"; then
        printf '%bDONE%b\n' "$GREEN" "$RESET"
        msg ok "Proxy #$CREATED_ID created: $SELECTED_PUBLIC → $SELECTED_BACKEND"
    else
        printf '%bFAILED%b\n' "$RED" "$RESET"
    fi
    pause_screen
}
edit_public_port() {
    local id=$1 old_public backend new_public service
    old_public=$(instance_get "$id" PUBLIC_PORT)
    backend=$(instance_get "$id" BACKEND_PORT)
    service=$(service_name "$id")
    banner; echo; msg info "Current public port: $old_public"
    read -r -p '  Enter new public port: ' new_public
    validate_pair "$new_public" "$backend" "$id" || { pause_screen; return; }
    [[ "$new_public" == "$old_public" ]] && { msg info 'Port is unchanged.'; pause_screen; return; }
    if port_listening "$new_public"; then msg error "Port $new_public is already listening."; pause_screen; return; fi

    systemctl stop "$service" >/dev/null 2>&1 || true
    firewall_remove "$id"
    write_instance "$id" "$new_public" "$backend"
    firewall_open "$id" "$new_public"
    if systemctl restart "$service" >/dev/null 2>&1 && sleep 1 && service_active "$id"; then
        msg ok "Public port changed to $new_public."
    else
        msg error 'Change failed. Restoring previous port.'
        firewall_remove "$id"
        write_instance "$id" "$old_public" "$backend"
        firewall_open "$id" "$old_public"
        systemctl restart "$service" >/dev/null 2>&1 || true
    fi
    pause_screen
}
edit_backend_port() {
    local id=$1 public old_backend new_backend service
    public=$(instance_get "$id" PUBLIC_PORT)
    old_backend=$(instance_get "$id" BACKEND_PORT)
    service=$(service_name "$id")
    banner; echo; msg info "Current backend port: $old_backend"
    read -r -p '  Enter new backend port: ' new_backend
    validate_pair "$public" "$new_backend" "$id" || { pause_screen; return; }
    [[ "$new_backend" == "$old_backend" ]] && { msg info 'Port is unchanged.'; pause_screen; return; }
    write_instance "$id" "$public" "$new_backend"
    if systemctl restart "$service" >/dev/null 2>&1 && sleep 1 && service_active "$id"; then
        msg ok "Backend port changed to $new_backend."
    else
        msg error 'Change failed. Restoring previous port.'
        write_instance "$id" "$public" "$old_backend"
        systemctl restart "$service" >/dev/null 2>&1 || true
    fi
    pause_screen
}
restart_instance() {
    local id=$1
    banner; echo
    printf '  Restarting Proxy #%s... ' "$id"
    if systemctl restart "$(service_name "$id")" >/dev/null 2>&1 && sleep 1 && service_active "$id"; then
        printf '%bDONE%b\n' "$GREEN" "$RESET"
    else
        printf '%bFAILED%b\n' "$RED" "$RESET"
        journalctl -u "$(service_name "$id")" -n 10 --no-pager 2>/dev/null || true
    fi
    pause_screen
}
delete_instance() {
    local id=$1 answer
    banner; echo; show_proxy_card "$id"; echo
    msg warn "Proxy #$id will be removed."
    read -r -p '  Type DELETE to continue: ' answer
    [[ "$answer" == DELETE ]] || { msg info 'Delete cancelled.'; pause_screen; return; }
    systemctl disable --now "$(service_name "$id")" >/dev/null 2>&1 || true
    firewall_remove "$id"
    rm -f "$(instance_file "$id")"
    systemctl daemon-reload >/dev/null 2>&1 || true
    msg ok "Proxy #$id deleted."
    pause_screen
}
manage_instance() {
    local id=$1 choice
    while instance_exists "$id"; do
        banner; echo; show_proxy_card "$id"; echo
        printf '  %b[1]%b Edit Public Port\n' "$CYAN" "$RESET"
        printf '  %b[2]%b Edit Backend Port\n' "$CYAN" "$RESET"
        printf '  %b[3]%b Restart Proxy\n' "$CYAN" "$RESET"
        printf '  %b[4]%b Delete Proxy\n' "$RED" "$RESET"
        printf '  %b[5]%b Start Proxy\n' "$GREEN" "$RESET"
        printf '  %b[6]%b Stop Proxy\n' "$YELLOW" "$RESET"
        printf '  %b[7]%b View Logs\n' "$GREEN" "$RESET"
        printf '  %b[0]%b Back\n' "$GRAY" "$RESET"
        echo
        read -r -p '  Select option: ' choice
        case "$choice" in
            1) edit_public_port "$id" ;;
            2) edit_backend_port "$id" ;;
            3) restart_instance "$id" ;;
            4) delete_instance "$id" ;;
            5) systemctl start "$(service_name "$id")"; pause_screen ;;
            6) systemctl stop "$(service_name "$id")"; pause_screen ;;
            7) journalctl -u "$(service_name "$id")" -n 40 --no-pager; pause_screen ;;
            0) return ;;
            *) msg error 'Invalid option.'; sleep 1 ;;
        esac
    done
}
proxy_list_screen() {
    local choice id found
    while true; do
        banner; echo
        if (( $(instance_count) == 0 )); then
            msg warn 'No proxy has been created yet.'
            pause_screen
            return
        fi
        while IFS= read -r id; do
            [[ -n "$id" ]] || continue
            show_proxy_card "$id"
            echo
        done < <(instance_ids)
        read -r -p '  Select Proxy Number (0 = Back): ' choice
        [[ "$choice" == 0 ]] && return
        if [[ "$choice" =~ ^[0-9]+$ ]] && instance_exists "$choice"; then
            manage_instance "$choice"
        else
            msg error 'Invalid proxy number.'
            sleep 1
        fi
    done
}
spinner_run() {
    local label=$1; shift
    local frames=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏') i=0 pid status
    "$@" >/dev/null 2>&1 & pid=$!
    if [[ -t 1 ]]; then
        while kill -0 "$pid" 2>/dev/null; do
            printf '\r  %b%s%b %-36s' "$CYAN" "${frames[$i]}" "$RESET" "$label"
            i=$(( (i + 1) % ${#frames[@]} ))
            sleep 0.08
        done
    fi
    if wait "$pid"; then status=0; else status=$?; fi
    if [[ -t 1 ]]; then
        printf '\r  %b✓%b %-36s %bDONE%b\n' "$GREEN" "$RESET" "$label" "$GREEN" "$RESET"
    fi
    return "$status"
}
uninstall_stop_all() {
    local id
    while IFS= read -r id; do
        [[ -n "$id" ]] || continue
        systemctl disable --now "$(service_name "$id")" >/dev/null 2>&1 || true
    done < <(instance_ids)
}
uninstall_firewalls() {
    local id
    while IFS= read -r id; do [[ -n "$id" ]] && firewall_remove "$id"; done < <(instance_ids)
}
uninstall_settings() {
    rm -rf "$BASE_DIR"
    rm -f "$SERVICE_FILE" "$SYSCTL_FILE"
    sysctl --system >/dev/null 2>&1 || true
    systemctl daemon-reload >/dev/null 2>&1 || true
    systemctl reset-failed >/dev/null 2>&1 || true
}
uninstall_command() { rm -f "$SELF_BIN"; }
uninstall_proxy() {
    local answer
    banner; echo
    msg warn 'This will remove every X-UI Payload Proxy.'
    read -r -p '  Type YES to continue: ' answer
    [[ "$answer" == YES ]] || { msg info 'Uninstall cancelled.'; pause_screen; return; }
    echo
    spinner_run 'Stopping all proxy services' uninstall_stop_all
    spinner_run 'Removing firewall rules' uninstall_firewalls
    spinner_run 'Removing proxy settings' uninstall_settings
    spinner_run 'Removing proxy command' uninstall_command
    echo
    msg ok 'X-UI Payload Proxy has been uninstalled.'
    exit 0
}
menu() {
    require_root
    ensure_layout
    while true; do
        local total active choice
        total=$(instance_count)
        active=$(active_count)
        banner
        echo
        printf '  Proxies: %b%s%b   Active: %b%s%b\n' "$CYAN" "$total" "$RESET" "$GREEN" "$active" "$RESET"
        line
        printf '  %b[1]%b Create New Proxy\n' "$CYAN" "$RESET"
        printf '  %b[2]%b Proxy List\n' "$CYAN" "$RESET"
        printf '  %b[0]%b Exit\n' "$GRAY" "$RESET"
        line
        printf '  %b[99]%b Uninstall\n' "$RED" "$RESET"
        echo
        read -r -p '  Select option: ' choice
        case "$choice" in
            1) create_proxy_screen ;;
            2) proxy_list_screen ;;
            99) uninstall_proxy ;;
            0) exit 0 ;;
            *) msg error 'Invalid option.'; sleep 1 ;;
        esac
    done
}

case "${1:-}" in
    --run-proxy) shift; run_proxy "${1:-}" ;;
    --prepare) prepare_system ;;
    --migrate) prepare_system; migrate_legacy ;;
    --create) shift; create_instance "${1:-}" "${2:-}"; printf '%s\n' "$CREATED_ID" ;;
    --has-instances) (( $(instance_count) > 0 )) ;;
    --count) instance_count ;;
    --version) printf 'X-UI Payload Proxy %s\n' "$VERSION" ;;
    '') menu ;;
    *) printf 'Usage: x-ui-proxy [--version]\n' >&2; exit 2 ;;
esac
