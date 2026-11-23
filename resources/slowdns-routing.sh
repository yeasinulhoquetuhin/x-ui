#!/bin/bash
# Domain-scoped DNS forwarding. The early nft NAT hook wins before TDZ SSH's
# ordinary port-53 REDIRECT, without editing its rules, services or key files.
set -euo pipefail
mark_chain=TDZ_XUI_DNS
input_chain=TDZ_XUI_DNS_IN
table=tdz_xui_slowdns
state_dir="${XUI_SLOWDNS_STATE_DIR:-/run/x-ui-slowdns}"
action="${1:-}"

remove_family() {
    local command="$1" family="$2"
    command -v "$command" >/dev/null 2>&1 || return 0
    for hook in PREROUTING OUTPUT; do
        while "$command" -w 5 -t mangle -C "$hook" -p udp --dport 53 -m addrtype --dst-type LOCAL -j "$mark_chain" 2>/dev/null; do
            "$command" -w 5 -t mangle -D "$hook" -p udp --dport 53 -m addrtype --dst-type LOCAL -j "$mark_chain"
        done
    done
    while "$command" -w 5 -C INPUT -j "$input_chain" 2>/dev/null; do
        "$command" -w 5 -D INPUT -j "$input_chain"
    done
    "$command" -w 5 -t mangle -F "$mark_chain" 2>/dev/null || true
    "$command" -w 5 -t mangle -X "$mark_chain" 2>/dev/null || true
    "$command" -w 5 -F "$input_chain" 2>/dev/null || true
    "$command" -w 5 -X "$input_chain" 2>/dev/null || true
    if command -v nft >/dev/null 2>&1; then
        nft delete table "$family" "$table" 2>/dev/null || true
    fi
}

if [[ "$action" == stop ]]; then
    [[ -f "$state_dir/state" ]] || exit 0
    exec 9>"$state_dir/lock"
    flock 9
    remove_family iptables ip
    remove_family ip6tables ip6
    rm -f "$state_dir/state"
    exit 0
fi
[[ "$action" == apply && $# == 4 ]] || { printf '%s\n' 'Usage: slowdns-routing.sh apply DNS_ZONE_HEX UDP_PORT ipv4|dual|ipv6; or stop' >&2; exit 1; }
zone_hex="$2" port="$3" mode="$4"
[[ "$zone_hex" =~ ^[0-9a-f]+00$ && ${#zone_hex} -le 512 && "$port" =~ ^[0-9]+$ ]] || exit 1
(( port > 0 && port < 65536 && port != 53 )) || exit 1
[[ "$mode" == ipv4 || "$mode" == dual || "$mode" == ipv6 ]] || exit 1
command -v nft >/dev/null 2>&1 || { printf '%s\n' 'Install nftables to enable shared-port SlowDNS routing.' >&2; exit 1; }
mkdir -p "$state_dir"
chmod 700 "$state_dir"
exec 9>"$state_dir/lock"
flock 9

apply_family() {
    local command="$1" family="$2" signature="$zone_hex:$port:$mode"
    command -v "$command" >/dev/null 2>&1 || return 1
    if [[ -f "$state_dir/state" && $(<"$state_dir/state") == "$signature" ]] &&
       "$command" -w 5 -t mangle -C "$mark_chain" -m string --hex-string "|$zone_hex|" --algo kmp --icase -j MARK --set-xmark 0x40000000/0x40000000 2>/dev/null &&
       "$command" -w 5 -t mangle -C PREROUTING -p udp --dport 53 -m addrtype --dst-type LOCAL -j "$mark_chain" 2>/dev/null &&
       nft list table "$family" "$table" >/dev/null 2>&1; then
        return 0
    fi
    local replacement=""
    if nft list table "$family" "$table" >/dev/null 2>&1; then replacement="delete table $family $table"; fi
    # One atomic nft transaction. Priority -101 precedes ordinary dstnat (-100).
    # Clear only our reserved mark bit before routing; preserve every other bit.
    nft -f - <<EOF
$replacement
add table $family $table
add chain $family $table prerouting { type nat hook prerouting priority -101; policy accept; }
add chain $family $table output { type nat hook output priority -101; policy accept; }
add rule $family $table prerouting udp dport 53 meta mark & 0x40000000 != 0 meta mark set meta mark & 0xbfffffff redirect to :$port
add rule $family $table output udp dport 53 meta mark & 0x40000000 != 0 meta mark set meta mark & 0xbfffffff redirect to :$port
EOF
    if [[ $? -ne 0 ]]; then return 1; fi
    "$command" -w 5 -t mangle -N "$mark_chain" 2>/dev/null || true
    "$command" -w 5 -t mangle -F "$mark_chain" || return 1
    "$command" -w 5 -t mangle -A "$mark_chain" -m string --hex-string "|$zone_hex|" --algo kmp --icase -j MARK --set-xmark 0x40000000/0x40000000 || return 1
    for hook in PREROUTING OUTPUT; do
        "$command" -w 5 -t mangle -C "$hook" -p udp --dport 53 -m addrtype --dst-type LOCAL -j "$mark_chain" 2>/dev/null ||
            "$command" -w 5 -t mangle -I "$hook" 1 -p udp --dport 53 -m addrtype --dst-type LOCAL -j "$mark_chain" || return 1
    done
    "$command" -w 5 -N "$input_chain" 2>/dev/null || true
    "$command" -w 5 -F "$input_chain" || return 1
    "$command" -w 5 -A "$input_chain" -p udp --dport "$port" -j ACCEPT || return 1
    "$command" -w 5 -C INPUT -j "$input_chain" 2>/dev/null || "$command" -w 5 -I INPUT 1 -j "$input_chain" || return 1
}

# Persist the ownership marker before changes so a failed apply can be removed.
previous=""
[[ ! -f "$state_dir/state" ]] || previous=$(<"$state_dir/state")
printf '%s' "${previous:-pending}" > "$state_dir/state"
failed=false
if [[ "$mode" == ipv4 || "$mode" == dual ]]; then apply_family iptables ip || failed=true; fi
if [[ "$mode" == ipv6 || "$mode" == dual ]]; then apply_family ip6tables ip6 || failed=true; fi
if [[ "$failed" == true ]]; then
    remove_family iptables ip
    remove_family ip6tables ip6
    rm -f "$state_dir/state"
    printf '%s\n' 'SlowDNS routing failed; its partial rules were removed.' >&2
    exit 1
fi
printf '%s' "$zone_hex:$port:$mode" > "$state_dir/state"
chmod 600 "$state_dir/state"
