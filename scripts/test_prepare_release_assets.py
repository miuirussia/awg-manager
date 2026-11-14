import gzip
import hashlib
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location(
    "release_assets", Path(__file__).with_name("prepare-release-assets.py")
)
assets = importlib.util.module_from_spec(spec)
spec.loader.exec_module(assets)


class ReleaseAssetsTest(unittest.TestCase):
    def test_mirror_preserves_pin_and_other_architectures(self):
        payload = b"pinned awgm-box"
        digest = hashlib.sha256(payload).hexdigest()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            embedded = root / "embedded.go"
            text = 'const RequiredVersion = "41"\nvar RequiredTags = []string{"with_awg"}\n'
            for arch in assets.ARCHES:
                text += (
                    f'"{arch}": {{Version: RequiredVersion, '
                    f'URL: "https://source.test/awgm-box-41-{arch}", '
                    f'SHA256: "{digest}", Size: {len(payload)}}},\n'
                )
            embedded.write_text(text)
            for arch in assets.ARCHES:
                with self.subTest(arch=arch):
                    embedded.write_text(text)
                    with patch.object(assets, "urlopen", return_value=io.BytesIO(payload)):
                        assets.mirror_singbox(arch, "https://mirror.test/", embedded, root / "dist")
                    self.assertEqual(
                        embedded.read_text(),
                        text.replace(
                            f"https://source.test/awgm-box-41-{arch}",
                            f"https://mirror.test/awgm-box-41-{arch}",
                        ),
                    )
                    self.assertEqual((root / "dist" / f"awgm-box-41-{arch}").read_bytes(), payload)

    def test_bad_download_does_not_replace_metadata_or_publish_asset(self):
        for payload in (b"bad", b"too long"):
            with self.subTest(payload=payload), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                embedded = root / "embedded.go"
                text = (
                    '"mipsel-3.4": {Version: RequiredVersion, '
                    'URL: "https://source.test/awgm-box-41-mipsel-3.4", '
                    f'SHA256: "{hashlib.sha256(b"pin").hexdigest()}", Size: 3}},\n'
                )
                embedded.write_text(text)
                with patch.object(assets, "urlopen", return_value=io.BytesIO(payload)):
                    with self.assertRaises(ValueError):
                        assets.mirror_singbox("mipsel-3.4", "https://mirror.test", embedded, root / "dist")
                self.assertEqual(embedded.read_text(), text)
                self.assertEqual(list((root / "dist").iterdir()), [])

    def test_package_index_contains_actual_package_hashes(self):
        with tempfile.TemporaryDirectory() as temporary:
            dist = Path(temporary)
            for package in ("awg-manager", "awgmautolist"):
                (dist / f"{package}_2.20.3+r5_mipsel-3.4-kn.ipk").write_bytes(package.encode())
            assets.package_index("mipsel-3.4", "2.20.3+r5", dist)
            index = gzip.decompress((dist / "Packages-mipsel-3.4.gz").read_bytes()).decode()
            entries = [dict(line.split(": ", 1) for line in block.splitlines())
                       for block in index.strip().split("\n\n")]
            self.assertEqual(len(entries), 2)
            for entry in entries:
                payload = (dist / entry["Filename"]).read_bytes()
                self.assertEqual(entry["SHA256sum"], hashlib.sha256(payload).hexdigest())
                self.assertEqual(int(entry["Size"]), len(payload))
                self.assertEqual(entry["Version"], "2.20.3+r5")


if __name__ == "__main__":
    unittest.main()
