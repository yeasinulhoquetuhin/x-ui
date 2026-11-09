import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class InstallerVersionTests(unittest.TestCase):
    def run_selection(self, requested='', answers='', latest='v2.9.3-patched.2', missing_assets=False):
        published = [dict(tag_name=f'v2.9.3-patched.{number}', draft=False,
                          assets=[dict(name='x-ui-linux-amd64.tar.gz')]) for number in range(3)]
        if missing_assets:
            published[1]['assets'] = []
        with tempfile.TemporaryDirectory() as directory:
            catalog = Path(directory) / 'releases.json'
            catalog.write_text(json.dumps(published))
            script = r'''
source "$1"
arch() { printf amd64; }
curl() {
    local url
    for url in "$@"; do
        if [[ "$url" == https://api.github.com/* ]]; then
            python3 -c 'import json,sys
rows=json.load(open(sys.argv[1])); url=sys.argv[2]
if url.endswith("/latest"): data=next(row for row in rows if row["tag_name"]==sys.argv[3])
elif "/tags/" in url:
 data=next((row for row in rows if row["tag_name"]==url.rsplit("/",1)[1]),None)
 if data is None: sys.exit(22)
else: data=rows
print(json.dumps(data))' "$TEST_CATALOG" "$url" "$TEST_LATEST"
            return $?
        fi
    done
    return 22
}
if xui_select_version "$2"; then printf 'SELECTED=%s\n' "$tag_version"; else exit 10; fi
'''
            return subprocess.run(['bash', '-c', script, 'version-test', str(ROOT / 'install.sh'), requested],
                                  input=answers, text=True, capture_output=True, timeout=20,
                                  env=dict(os.environ, TEST_CATALOG=str(catalog), TEST_LATEST=latest,
                                           XUI_ENV_FILE=str(Path(directory) / 'no-env')))

    def test_default_installs_latest(self):
        result = self.run_selection()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('SELECTED=v2.9.3-patched.2', result.stdout)

    def test_shortcuts_and_full_tag(self):
        for version in ('v0', 'v1', 'v2', 'v2.9.3-patched.2'):
            result = self.run_selection(version)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            expected = version if 'patched' in version else 'v2.9.3-patched.' + version[1:]
            self.assertIn('SELECTED=' + expected, result.stdout)

    def test_missing_version_can_install_latest(self):
        result = self.run_selection('v5', 'y\n')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('not available', result.stdout)
        self.assertIn('v2  -  v2.9.3-patched.2', result.stdout)
        self.assertIn('SELECTED=v2.9.3-patched.2', result.stdout)

    def test_missing_version_can_select_another(self):
        result = self.run_selection('v9', 'v8\nv2\n')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('Version v8 is unavailable', result.stdout)
        self.assertIn('SELECTED=v2.9.3-patched.2', result.stdout)

    def test_cancel_or_eof_does_not_choose_a_version(self):
        for answer in ('n\n', ''):
            result = self.run_selection('v9', answer)
            self.assertNotEqual(result.returncode, 0)
            self.assertNotIn('SELECTED=', result.stdout)

    def test_release_without_architecture_asset_is_unavailable(self):
        result = self.run_selection('v1', 'v2\n', missing_assets=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('SELECTED=v2.9.3-patched.2', result.stdout)

    def test_missing_version_can_select_zero(self):
        result = self.run_selection('v9', 'v0\n')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('SELECTED=v2.9.3-patched.0', result.stdout)
