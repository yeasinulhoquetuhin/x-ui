import io
import os
import pathlib
import subprocess
import tarfile
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.base = pathlib.Path(self.temp.name)
        self.app = self.base / "existing panel"
        self.app.mkdir()
        self.db = self.base / "database"
        self.db.mkdir()
        self.units = self.base / "units"
        self.units.mkdir()
        self.cli = self.base / "x-ui-cli"
        self.events = self.base / "events"
        self.archive = self.base / "release.tar.gz"
        (self.app / "x-ui").write_text("#!/bin/bash\nexit 0\n")
        (self.app / "x-ui").chmod(0o755)
        (self.app / "x-ui.sh").write_text("old-menu")
        (self.app / "custom-cert.pem").write_text("existing-certificate")
        (self.app / ".env").write_text("EXISTING_SETTING=preserved\n")
        (self.db / "x-ui.db").write_bytes(b"existing database/settings")
        (self.units / "x-ui.service").write_text("custom service")
        self.env = dict(os.environ, XUI_MAIN_FOLDER=str(self.app),
                        XUI_DB_FOLDER=str(self.db), XUI_SERVICE=str(self.units),
                        XUI_CLI_PATH=str(self.cli), XUI_LOG_FOLDER=str(self.base / "logs"),
                        XUI_ENV_FILE=str(self.base / "absent.env"),
                        TEST_ARCHIVE=str(self.archive), TEST_EVENTS=str(self.events))

    def package(self, fail=False, invalid=False):
        with tarfile.open(self.archive, "w:gz") as archive:
            files = {"x-ui/x-ui": ("#!/bin/bash\n"
                     'printf "%s\\n" "$*" >> "$TEST_EVENTS"\n'
                     '[[ "$1" == migrate ]] || exit 99\n'
                     + ("exit 1\n" if fail else "exit 0\n")),
                     "x-ui/x-ui.sh": "new-menu", "x-ui/bin/xray-linux-arm64": "new-core"}
            if invalid:
                files.pop("x-ui/x-ui")
            for name, text in files.items():
                info = tarfile.TarInfo(name)
                data = text.encode()
                info.size, info.mode = len(data), 0o755
                archive.addfile(info, io.BytesIO(data))

    def run_installer(self):
        script = r'''
source "$1"
release=debian
arch() { printf '%s' arm64; }
curl() {
    local destination=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
          -fLRo|-4fLRo|-o) destination="$2"; shift 2 ;;
          *) shift ;;
        esac
    done
    if [[ -n "$destination" ]]; then cp "$TEST_ARCHIVE" "$destination"; else printf '%s\n' '{"tag_name":"v2.9.3-patched.1"}'; fi
}
systemctl() { printf 'service %s\n' "$*" >> "$TEST_EVENTS"; }
install_x-ui
'''
        return subprocess.run(["bash", "-c", script, "installer-test", str(ROOT / "install.sh")],
                              env=self.env, capture_output=True, text=True, timeout=30)

    def test_reinstall_preserves_existing_configuration(self):
        self.package()
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual((self.app / "custom-cert.pem").read_text(), "existing-certificate")
        self.assertEqual((self.app / ".env").read_text(), "EXISTING_SETTING=preserved\n")
        self.assertEqual((self.db / "x-ui.db").read_bytes(), b"existing database/settings")
        self.assertEqual((self.units / "x-ui.service").read_text(), "custom service")
        self.assertEqual(self.cli.read_text(), "new-menu")
        events = self.events.read_text()
        self.assertIn("migrate", events)
        self.assertNotIn("setting ", events)
        self.assertNotIn("cert ", events)
        self.assertEqual((self.app / "bin/xray-linux-arm64").read_text(), "new-core")

    def test_invalid_archive_does_not_stop_existing_panel(self):
        self.package(invalid=True)
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.events.exists())
        self.assertEqual((self.app / "x-ui.sh").read_text(), "old-menu")

    def test_failed_migration_restores_previous_application(self):
        self.package(fail=True)
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.app / "x-ui.sh").read_text(), "old-menu")
        self.assertEqual((self.db / "x-ui.db").read_bytes(), b"existing database/settings")
        self.assertIn("service start x-ui", self.events.read_text())


if __name__ == "__main__":
    unittest.main()
