#!/bin/bash
# Use the same settings-preserving installer for upgrades and reinstalls.
set -e
installer=$(mktemp)
trap 'rm -f -- "$installer"' EXIT
curl -fLsS --retry 3 \
    https://raw.githubusercontent.com/yeasinulhoquetuhin/x-ui/master/install.sh \
    -o "$installer"
bash "$installer" "$@"
