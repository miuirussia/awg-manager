#!/usr/bin/env python3
"""Mirror pinned awgm-box bytes and generate an opkg index for GitHub Releases."""

import argparse
import gzip
import hashlib
import os
from pathlib import Path
import re
import tempfile
from urllib.request import urlopen


ARCHES = ("mipsel-3.4", "mips-3.4", "aarch64-3.10")
ROOT = Path(__file__).resolve().parent.parent


def mirror_singbox(arch, release_base, embedded, dist):
    text = embedded.read_text()
    pattern = re.compile(
        rf'("{re.escape(arch)}":\s*\{{Version: RequiredVersion, URL: ")'
        r'([^"\n]+)(", SHA256: "([0-9a-f]{64})", Size: (\d+)\},)'
    )
    match = pattern.search(text)
    if not match:
        raise ValueError(f"Missing pinned binary metadata for {arch}")
    source, expected_hash, expected_size = match[2], match[4], int(match[5])
    name = source.rsplit("/", 1)[-1]
    if not re.fullmatch(r"awgm-box-[A-Za-z0-9.+_-]+", name):
        raise ValueError(f"Unexpected pinned binary name: {name}")
    dist.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=dist, delete=False) as output:
        temporary = Path(output.name)
        try:
            digest = hashlib.sha256()
            size = 0
            with urlopen(source, timeout=120) as response:
                while chunk := response.read(1024 * 1024):
                    size += len(chunk)
                    if size > expected_size:
                        raise ValueError(f"Pinned binary exceeds expected size: {name}")
                    digest.update(chunk)
                    output.write(chunk)
            if size != expected_size or digest.hexdigest() != expected_hash:
                raise ValueError(f"Pinned binary size/SHA256 mismatch: {name}")
            output.close()
            temporary.chmod(0o755)
            temporary.replace(dist / name)
        finally:
            temporary.unlink(missing_ok=True)
    target = release_base.rstrip("/") + "/" + name
    embedded.write_text(text[:match.start(2)] + target + text[match.end(2):])
    print(f"Mirrored {name}, preserved SHA256 {expected_hash}")


def package_index(arch, version, dist):
    if not re.fullmatch(r"[A-Za-z0-9.+_-]+", version):
        raise ValueError("Invalid package version")
    entries = []
    for package in ("awg-manager", "awgmautolist"):
        name = f"{package}_{version}_{arch}-kn.ipk"
        payload = (dist / name).read_bytes()
        entries.append(
            f"Package: {package}\nVersion: {version}\nArchitecture: {arch}\n"
            f"Filename: {name}\nSize: {len(payload)}\n"
            f"SHA256sum: {hashlib.sha256(payload).hexdigest()}\n\n"
        )
    (dist / f"Packages-{arch}.gz").write_bytes(
        gzip.compress("".join(entries).encode(), mtime=0)
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("mirror-singbox", "package-index"))
    parser.add_argument("arch", choices=ARCHES)
    parser.add_argument("--version")
    args = parser.parse_args()
    if args.command == "mirror-singbox":
        mirror_singbox(
            args.arch,
            os.environ.get("SINGBOX_RELEASE_BASE_URL") or os.environ["AWG_RELEASE_BASE_URL"],
            ROOT / "internal/singbox/installer/embedded.go",
            ROOT / "dist",
        )
    else:
        if not args.version:
            parser.error("package-index requires --version")
        package_index(args.arch, args.version, ROOT / "dist")


if __name__ == "__main__":
    main()
