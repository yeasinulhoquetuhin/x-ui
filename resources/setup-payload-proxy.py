#!/usr/bin/env python3
"""Install X-UI Payload Proxy and migrate existing proxy instances safely."""
import argparse
from datetime import datetime, timezone
from pathlib import Path
import shutil
import subprocess


def read_settings(path):
    values = {}
    for line in path.read_text().splitlines():
        if '=' in line:
            key, value = line.split('=', 1)
            values[key] = value
    public = int(values['PUBLIC_PORT'])
    backend = int(values['BACKEND_PORT'])
    if not (1 <= public <= 65535 and 1 <= backend <= 65535) or public == backend:
        raise ValueError('Invalid proxy port configuration: ' + str(path))
    return public, backend


def install(source, root=Path('/'), run=subprocess.run):
    base = root / 'etc/x-ui/payload-proxy'
    binary = root / 'usr/local/bin/x-ui-proxy'
    units = root / 'etc/systemd/system'
    old = root / 'etc/tdzp'
    single = root / 'etc/tdzp.conf'
    source = Path(source)
    subprocess.run(['bash', '-n', str(source)], check=True)
    stamp = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')
    legacy = []
    if old.is_dir():
        for file in sorted((old / 'instances').glob('*.conf')):
            if file.stem.isdigit():
                legacy.append((file.stem, file, f'tdzp@{file.stem}.service'))
    if not legacy and single.is_file():
        legacy.append(('1', single, 'tdzp.service'))
    # Validate and plan every port before touching services.
    planned = []
    for identity, path, unit in legacy:
        if (base / ('migrated-' + identity)).is_file() or (path.parent / ('.x-ui-migrated-' + identity)).is_file():
            continue
        public, backend = read_settings(path)
        target = base / 'instances' / (identity + '.conf')
        if target.exists() and read_settings(target) != (public, backend):
            raise ValueError('Proxy instance conflict; previous setup was preserved: ' + identity)
        planned.append((identity, path, unit, public, backend))
    binary.parent.mkdir(parents=True, exist_ok=True)
    units.mkdir(parents=True, exist_ok=True)
    for name in ('instances', 'firewall'):
        (base / name).mkdir(parents=True, exist_ok=True)
    changed = not binary.is_file() or binary.read_bytes() != source.read_bytes()
    if binary.is_file() and changed:
        shutil.copy2(binary, binary.with_name(binary.name + '.backup-' + stamp))
    temporary = binary.with_name('.x-ui-proxy-' + stamp)
    shutil.copyfile(source, temporary)
    temporary.chmod(0o755)
    temporary.replace(binary)
    template = '''[Unit]
Description=X-UI Payload Proxy #%i
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/x-ui-proxy --run-proxy %i
Restart=always
RestartSec=2
LimitNOFILE=1048576
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=full

[Install]
WantedBy=multi-user.target
'''
    (units / 'x-ui-proxy@.service').write_text(template)
    (units / 'x-ui-proxy@.service').chmod(0o644)
    run(['systemctl', 'daemon-reload'], check=True)
    for identity, path, unit, public, backend in planned:
        active = run(['systemctl', 'is-active', '--quiet', unit], check=False).returncode == 0
        enabled = run(['systemctl', 'is-enabled', '--quiet', unit], check=False).returncode == 0
        destination = base / 'instances' / (identity + '.conf')
        if not destination.exists():
            shutil.copy2(path, destination)
            destination.chmod(0o600)
        # Preserve existing firewall rules and their original deletion identity.
        firewall = old / 'firewall' / (identity + '.state')
        if not firewall.is_file() and unit == 'tdzp.service':
            firewall = root / 'etc/tdzp.firewall'
        if firewall.is_file() and not (base / 'firewall' / (identity + '.state')).exists():
            content = firewall.read_text()
            content += '\nCOMMENT=' + ('TDZP' if unit == 'tdzp.service' else 'TDZP:' + identity) + '\n'
            target = base / 'firewall' / (identity + '.state')
            target.write_text(content)
            target.chmod(0o600)
        replacement = f'x-ui-proxy@{identity}.service'
        run(['systemctl', 'stop', unit], check=True)
        try:
            if enabled:
                run(['systemctl', 'enable', replacement], check=True)
            if active:
                run(['systemctl', 'start', replacement], check=True)
                run(['systemctl', 'is-active', '--quiet', replacement], check=True)
            run(['systemctl', 'disable', unit], check=True)
        except subprocess.CalledProcessError:
            run(['systemctl', 'stop', replacement], check=False)
            run(['systemctl', 'disable', replacement], check=False)
            if enabled:
                run(['systemctl', 'enable', unit], check=False)
            if active:
                run(['systemctl', 'start', unit], check=False)
            raise
        (base / ('migrated-' + identity)).write_text('Previous config preserved at ' + str(path) + '\n')
        (path.parent / ('.x-ui-migrated-' + identity)).write_text('Migrated to ' + str(destination) + '\n')
        print(f'Proxy #{identity}: {public} -> {backend}; migrated to X-UI Payload Proxy.')
    if changed:
        result = run(['systemctl', 'list-units', '--state=active', '--type=service', '--no-legend', '--plain', 'x-ui-proxy@*.service'], check=True, capture_output=True, text=True)
        names = [line.split()[0] for line in result.stdout.splitlines() if line.split()]
        if names:
            run(['systemctl', 'restart', *names], check=True)
    print('X-UI Payload Proxy is ready in x-ui menu 4.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', nargs='?', default=str(Path(__file__).with_name('payload-proxy.sh')))
    args = parser.parse_args()
    install(args.source)
