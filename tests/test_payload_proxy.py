import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('setup_payload_proxy', ROOT / 'resources/setup-payload-proxy.py')
setup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(setup)


class PayloadProxyTests(unittest.TestCase):
    @unittest.skipUnless(os.geteuid() == 0, 'interactive proxy management requires root')
    def test_empty_menu_and_name(self):
        with tempfile.TemporaryDirectory() as directory:
            result = subprocess.run(['bash', str(ROOT / 'resources/payload-proxy.sh')],
                                    input='0\n', text=True, capture_output=True, timeout=5,
                                    env=dict(os.environ, XUI_PROXY_BASE_DIR=directory))
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn('X-UI Payload / Proxy Management', result.stdout)
            self.assertNotIn('TDZP', result.stdout)
            self.assertIn('Proxies: 0', result.stdout)
            self.assertNotIn('Uninstall', result.stdout)
            self.assertNotIn('[99]', result.stdout)
            self.assertIn('1. Create Proxy', result.stdout)

    def test_migration_preserves_ports_and_is_repeatable(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            old = root / 'etc/tdzp/instances'
            old.mkdir(parents=True)
            (old / '1.conf').write_text('PUBLIC_PORT=280\nBACKEND_PORT=279\n')
            calls = []
            def run(args, **kwargs):
                calls.append(args)
                return subprocess.CompletedProcess(args, 0, stdout='')
            source = ROOT / 'resources/payload-proxy.sh'
            setup.install(source, root, run, migrate=True)
            self.assertEqual((root / 'etc/x-ui/payload-proxy/instances/1.conf').read_text(), (old / '1.conf').read_text())
            self.assertEqual((root / 'usr/local/bin/x-ui-proxy').read_bytes(), source.read_bytes())
            self.assertIn(['systemctl', 'start', 'x-ui-proxy@1.service'], calls)
            calls.clear()
            setup.install(source, root, run, migrate=True)
            self.assertNotIn(['systemctl', 'stop', 'tdzp@1.service'], calls)

    def test_migration_failure_restarts_original_proxy(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            old = root / 'etc/tdzp/instances'
            old.mkdir(parents=True)
            (old / '1.conf').write_text('PUBLIC_PORT=280\nBACKEND_PORT=279\n')
            calls = []
            def run(args, **kwargs):
                calls.append(args)
                if args == ['systemctl', 'start', 'x-ui-proxy@1.service']:
                    raise subprocess.CalledProcessError(1, args)
                return subprocess.CompletedProcess(args, 0, stdout='')
            with self.assertRaises(subprocess.CalledProcessError):
                setup.install(ROOT / 'resources/payload-proxy.sh', root, run,migrate=True)
            self.assertIn(['systemctl', 'start', 'tdzp@1.service'], calls)
            self.assertIn(['systemctl', 'disable', 'x-ui-proxy@1.service'], calls)
            self.assertTrue((old / '1.conf').is_file())

    def test_install_never_creates_or_imports_proxy_instances(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            old=root/'etc/tdzp/instances'
            old.mkdir(parents=True)
            (old/'1.conf').write_text('PUBLIC_PORT=280\nBACKEND_PORT=279\n')
            calls=[]
            def run(args,**kwargs):
                calls.append(args)
                return subprocess.CompletedProcess(args,0,stdout='')
            setup.install(ROOT/'resources/payload-proxy.sh',root,run)
            self.assertEqual(list((root/'etc/x-ui/payload-proxy/instances').glob('*.conf')),[])
            self.assertFalse(any('start' in call or 'enable' in call or 'stop' in call for call in calls))
            self.assertTrue((old/'1.conf').is_file())
