#!/usr/bin/env python3
"""Add a root-private socket/IP side channel to the TDZP 1.2 payload bridge.

Payload handshakes and relayed bytes are unchanged. The matching TDZ Xray core
reads these per-socket records to identify the clients behind the local proxy.
"""
import argparse
import ast
from datetime import datetime, timezone
from pathlib import Path
import subprocess

MARKER = '# TDZP_REAL_CLIENT_IP_V2'


def patch_source(source):
    if MARKER in source:
        return source
    replacements = [
        ('import contextlib\nimport signal\n',
         'import contextlib\nimport json\nimport os\nfrom pathlib import Path\nimport signal\n'),
        ('async def client_handler(client_reader, client_writer):\n    backend_writer = None\n',
         'async def client_handler(client_reader, client_writer):\n    backend_writer = None\n    ip_record = None\n'),
        ('        tune(backend_writer)\n        client_writer.write(RESPONSE)\n',
         '''        tune(backend_writer)
        # TDZP_REAL_CLIENT_IP_V2
        # No protocol bytes are added or rewritten. The core checks the live
        # backend socket inode before using this private client-IP record.
        peer = client_writer.get_extra_info("peername")
        local = backend_writer.get_extra_info("sockname")
        sock = backend_writer.get_extra_info("socket")
        directory = Path(os.environ.get("TDZ_XRAY_PROXY_IP_DIR", "/run/tdzp-client-ips"))
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
'''),
        ('    finally:\n        await close(backend_writer)\n',
         '    finally:\n        if ip_record is not None:\n            with contextlib.suppress(OSError):\n                ip_record.unlink()\n        await close(backend_writer)\n'),
    ]
    for before, after in replacements:
        if source.count(before) != 1:
            raise ValueError('Unrecognized TDZP bridge version; original script was preserved')
        source = source.replace(before, after)
    python = source.split("<<'PYTHON_PROXY'\n", 1)[1].split('\nPYTHON_PROXY', 1)[0]
    ast.parse(python)
    return source


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('path', nargs='?', default='/usr/local/bin/tdzp')
    parser.add_argument('--restart', action='store_true')
    args = parser.parse_args()
    path = Path(args.path)
    if not path.is_file():
        return
    original = path.read_text()
    updated = patch_source(original)
    if updated == original:
        print('TDZP real-client-IP forwarding is already enabled.')
        return
    stamp = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')
    backup = path.with_name(path.name + '.backup-' + stamp)
    backup.write_text(original)
    backup.chmod(path.stat().st_mode)
    temp = path.with_name('.' + path.name + '.client-ip-' + stamp)
    temp.write_text(updated)
    temp.chmod(path.stat().st_mode)
    checked = subprocess.run(['bash', '-n', str(temp)], check=False)
    if checked.returncode:
        temp.unlink()
        raise ValueError('TDZP syntax check failed; original script was preserved')
    temp.replace(path)
    print(f'TDZP real-client-IP forwarding enabled. Backup: {backup}')
    if args.restart:
        units = subprocess.check_output([
            'systemctl', 'list-units', '--state=active', '--type=service',
            '--no-legend', '--plain', 'tdzp@*.service',
        ], text=True)
        names = [line.split()[0] for line in units.splitlines() if line.split()]
        if names:
            subprocess.run(['systemctl', 'restart', *names], check=True)
            print('Restarted ' + ', '.join(names))


if __name__ == '__main__':
    main()
