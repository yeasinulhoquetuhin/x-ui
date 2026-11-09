import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('tdzp_client_ip', ROOT / 'resources/tdzp-client-ip.py')
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


class TDZPClientIPTests(unittest.TestCase):
    def test_patch_preserves_payload_relay_and_is_idempotent(self):
        source = (ROOT / 'tests/testdata/tdzp-1.2.sh').read_text()
        updated = helper.patch_source(source)
        self.assertEqual(helper.patch_source(updated), updated)
        before = source.split("<<'PYTHON_PROXY'\n", 1)[1].split('\nPYTHON_PROXY', 1)[0]
        after = updated.split("<<'PYTHON_PROXY'\n", 1)[1].split('\nPYTHON_PROXY', 1)[0]
        # The existing payload parser, relay and listener are byte-for-byte kept.
        for start, end in [('async def read_header', 'async def client_handler'), ('async def main', 'asyncio.run(main())')]:
            self.assertEqual(before.split(start)[1].split(end)[0], after.split(start)[1].split(end)[0])

    def test_unknown_version_is_not_overwritten(self):
        with self.assertRaises(ValueError):
            helper.patch_source('#!/bin/bash\ncustom-proxy\n')
