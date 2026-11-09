import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class MenuUpdateTests(unittest.TestCase):
    def test_update_menu_only_replaces_menu_scripts(self):
        source = (ROOT / 'x-ui.sh').read_text()
        function = source.split('update_menu() {', 1)[1].split('\nlegacy_version()', 1)[0]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            app = root / 'app'
            app.mkdir()
            menu = root / 'downloaded-menu.sh'
            menu.write_text('#!/bin/bash\nprintf "new menu"\n')
            panel = app / 'x-ui'
            panel.write_text('existing panel binary')
            script = '''
confirm() { return 0; }
LOGE() { printf '%s\\n' "$*" >&2; }
LOGI() { printf '%s\\n' "$*"; }
# The real CLI has an install() function. Menu updates must not call it.
install() { printf 'panel installer invoked' >&2; exit 97; }
curl() { cp "$TEST_MENU" "$4"; }
systemctl() { printf 'service changed' >&2; exit 98; }
update_menu() {''' + function + '\nupdate_menu\n'
            result = subprocess.run(['bash', '-c', script], text=True, capture_output=True,
                                    env=dict(os.environ, xui_folder=str(app), XUI_CLI_PATH=str(root / 'cli'), TEST_MENU=str(menu)), timeout=10)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual((root / 'cli').read_bytes(), menu.read_bytes())
            self.assertEqual((app / 'x-ui.sh').read_bytes(), menu.read_bytes())
            self.assertEqual(panel.read_text(), 'existing panel binary')

    def test_payload_manager_replaces_legacy_entry(self):
        source = (ROOT / 'x-ui.sh').read_text()
        menu = source.split('show_menu() {', 1)[1]
        self.assertIn('${green}4.${plain} Payload / Proxy', menu)
        self.assertNotIn('${green}27.', menu)
        self.assertNotIn('${green}4.${plain} Legacy Version', menu)
