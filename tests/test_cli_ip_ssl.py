import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SOURCE = (Path(__file__).resolve().parents[1] / 'x-ui.sh').read_text()


def function(name, end):
    return SOURCE.split(name + '() ', 1)[1].split('\n' + end + '() ', 1)[0]


class IPSSLTests(unittest.TestCase):
    def test_settings_issuance_recovers_without_hidden_menu(self):
        helper = 'issue_ip_ssl_from_settings() ' + function('issue_ip_ssl_from_settings', 'set_port')
        for status in (0, 1):
            for result in (0, 1, 130):
                script = f'''
check_status() {{ return {status}; }}
stop() {{ [[ $# -eq 1 ]] || exit 97; printf 'STOP\n'; }}
start() {{ [[ $# -eq 1 ]] || exit 98; printf 'START\n'; }}
ssl_cert_issue_for_ip() {{ printf 'ISSUE\n'; return {result}; }}
{helper}
issue_ip_ssl_from_settings
'''
                run = subprocess.run(['bash', '-c', script], capture_output=True, text=True, timeout=4)
                self.assertEqual(run.returncode, result, run.stderr)
                self.assertEqual(run.stdout.splitlines(), ['STOP', 'ISSUE', 'START'] if status == 0 else ['ISSUE'])

    def test_current_settings_accepts_yes_and_calls_issuance(self):
        body = 'check_config() ' + function('check_config', 'issue_ip_ssl_from_settings')
        with tempfile.TemporaryDirectory() as directory:
            panel = Path(directory) / 'x-ui'
            panel.write_text('#!/bin/bash\nif [[ "$*" == *getCert* ]]; then printf "cert: \\n"; else printf "port: 2053\\nwebBasePath: /panel/\\n"; fi\n')
            panel.chmod(0o755)
            for answer in ('y', '-y', 'yes', 'Y'):
                script = '''
LOGI() { :; }
LOGE() { :; }
curl() { printf '192.0.2.10'; }
issue_ip_ssl_from_settings() { printf 'ISSUANCE_CALLED\n'; }
''' + body + '\ncheck_config\n'
                run = subprocess.run(['bash', '-c', script], input=answer+'\n', capture_output=True, text=True,
                                     env=dict(os.environ, xui_folder=directory), timeout=4)
                self.assertEqual(run.returncode, 0, run.stderr)
                self.assertIn('ISSUANCE_CALLED', run.stdout)

    def test_busy_port_detection(self):
        body = 'is_port_in_use() ' + function('is_port_in_use', 'is_ipv4')
        script = '''
ss() { printf 'State Recv-Q Send-Q Local Peer\nLISTEN 0 128 0.0.0.0:80 0.0.0.0:*\n'; }
''' + body + '\nis_port_in_use 80 || exit 91\nis_port_in_use 81 && exit 92\nexit 0'
        run = subprocess.run(['bash', '-c', script], capture_output=True, text=True, timeout=4)
        self.assertEqual(run.returncode, 0, run.stderr)
