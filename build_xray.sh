#!/bin/bash
# Reproducible core build: use the same pinned source on every architecture.
set -euo pipefail
root=$(realpath "$(dirname "${BASH_SOURCE[0]}")")
destination=$(realpath -m "${1:?Usage: bash build_xray.sh OUTPUT}")
source_dir="${TDZ_XRAY_SOURCE:-${root}/.cache/xray-core}"
revision=b4f08981becb71eaa995fa98ed2098ade92566bb
source_sha256=a2ed4c9b7022c9dd5eed6863d360c70495d3ca5174a497d99d7e082b087c16fd
if [[ ! -d "$source_dir" ]]; then
    mkdir -p "$(dirname "$source_dir")"
    if [[ -f "$root/third_party/xray-core-v26.4.25-source.tar.gz" ]]; then
        [[ $(sha256sum "$root/third_party/xray-core-v26.4.25-source.tar.gz" | cut -d ' ' -f1) == "$source_sha256" ]] || { printf '%s\n' 'Xray source checksum mismatch' >&2; exit 1; }
        mkdir -p "$source_dir"
        tar -xzf "$root/third_party/xray-core-v26.4.25-source.tar.gz" --strip-components=1 -C "$source_dir"
    else
        archive=$(mktemp)
        curl -fLsS --retry 3 https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/xray-core-v26.4.25-source.tar.gz -o "$archive"
        [[ $(sha256sum "$archive" | cut -d ' ' -f1) == "$source_sha256" ]] || { rm -f "$archive"; printf '%s\n' 'Xray source checksum mismatch' >&2; exit 1; }
        mkdir -p "$source_dir"
        tar -xzf "$archive" --strip-components=1 -C "$source_dir"
        rm -f "$archive"
    fi
fi
if [[ -d "$source_dir/.git" ]]; then
    [[ $(git -C "$source_dir" rev-parse HEAD) == "$revision" ]] || { printf '%s\n' 'Unexpected Xray source revision' >&2; exit 1; }
else
    [[ $(<"$source_dir/TDZ-UPSTREAM-REVISION") == "$revision" ]] || { printf '%s\n' 'Unexpected Xray source revision' >&2; exit 1; }
fi
if [[ ! -f "$source_dir/app/dispatcher/tdz_ip_limit.go" ]]; then
    python3 "$root/xray-patches/apply.py" "$source_dir"
fi
cp "$root/xray-patches/testdata/tdz_ip_limit.go" "$source_dir/app/dispatcher/tdz_ip_limit.go"
cp "$root/xray-patches/testdata/tdz_ip_limit_test.go" "$source_dir/app/dispatcher/tdz_ip_limit_test.go"
cp "$root/xray-patches/testdata/tdz_session.go" "$source_dir/common/session/tdz_session.go"
mkdir -p "$(dirname "$destination")"
export CGO_ENABLED=0
(
    cd "$source_dir"
    go build -trimpath -ldflags '-s -w -X github.com/xtls/xray-core/core.build=TDZ-IPLimit/1' -o "$destination" ./main
)
