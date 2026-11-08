#!/bin/sh
set -eu
case $1 in
    amd64)
        ARCH="64"
        FNAME="amd64"
        ;;
    i386)
        ARCH="32"
        FNAME="i386"
        ;;
    armv8 | arm64 | aarch64)
        ARCH="arm64-v8a"
        FNAME="arm64"
        ;;
    armv7 | arm | arm32)
        ARCH="arm32-v7a"
        FNAME="arm32"
        ;;
    armv6)
        ARCH="arm32-v6"
        FNAME="armv6"
        ;;
    *)
        ARCH="64"
        FNAME="amd64"
        ;;
esac
mkdir -p build/bin
bash build_xray.sh "build/bin/xray-linux-${FNAME}"
cd build/bin
for asset in geoip.dat geosite.dat geoip_IR.dat geosite_IR.dat geoip_RU.dat geosite_RU.dat; do
    curl -sfLRO "https://github.com/yeasinulhoquetuhin/x-ui/releases/latest/download/$asset"
done
cd ../../
