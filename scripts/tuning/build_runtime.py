#!/usr/bin/env python3
"""Build-time only: make a pinned, offline CPython/Optuna payload."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request
import zstandard

ROOT = Path(__file__).resolve().parents[2]
LOCK = json.loads((Path(__file__).parent / 'python-lock.json').read_text())

def fetch(url, digest, destination):
    cache = ROOT / 'internal/tuning/runtime/assets/downloads'
    cache.mkdir(exist_ok=True)
    stored = cache / digest
    if not stored.exists() or hashlib.sha256(stored.read_bytes()).hexdigest() != digest:
        urllib.request.urlretrieve(url, destination)
        if hashlib.sha256(destination.read_bytes()).hexdigest() != digest:
            raise RuntimeError('Archive checksum mismatch')
        shutil.copyfile(destination, stored)
    else:
        shutil.copyfile(stored, destination)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--arch', choices=['amd64', 'arm64'], required=True)
    args = parser.parse_args()
    arch = args.arch
    assets = ROOT / 'internal/tuning/runtime/assets'
    assets.mkdir(parents=True, exist_ok=True)
    target = assets / f'linux-{arch}.tar.gz'
    fingerprint = hashlib.sha256((Path(__file__).read_bytes() + (Path(__file__).parent / 'requirements.lock').read_bytes() + json.dumps(LOCK, sort_keys=True).encode())).hexdigest()
    metadata = target.with_suffix('').with_suffix('.json')
    if target.exists() and metadata.exists():
        previous = json.loads(metadata.read_text())
        if previous.get('build_hash') == fingerprint and hashlib.sha256(target.read_bytes()).hexdigest() == previous.get('sha256'):
            print(f'Using verified payload: {target}')
            return
    with tempfile.TemporaryDirectory(prefix='plaza-runtime-') as temporary:
        work = Path(temporary)
        record = LOCK['assets'][arch]
        archive = work / 'python.tar.gz'
        fetch(record['url'], record['sha256'], archive)
        if hashlib.sha256(archive.read_bytes()).hexdigest() != record['sha256']:
            raise RuntimeError('CPython archive checksum mismatch')
        with tarfile.open(archive) as source:
            source.extractall(work, filter='data')
        python = work / 'python'
        full = work / 'full.tar.zst'
        fetch(record['full']['url'], record['full']['sha256'], full)
        if hashlib.sha256(full.read_bytes()).hexdigest() != record['full']['sha256']:
            raise RuntimeError('CPython full archive checksum mismatch')
        # The install-only archive omits dependency license texts and PYTHON.json.
        with full.open('rb') as raw, zstandard.ZstdDecompressor().stream_reader(raw) as stream, tarfile.open(fileobj=stream, mode='r|') as source:
            for member in source:
                if member.isfile() and (member.name.startswith('python/licenses/') or member.name == 'python/PYTHON.json'):
                    relative = Path(member.name).relative_to('python')
                    if '..' in relative.parts or relative.is_absolute():
                        raise RuntimeError('Invalid metadata path')
                    destination = python / relative
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    destination.write_bytes(source.extractfile(member).read())
        # Berkeley DB is not needed by Optuna; omit its copyleft extension entirely.
        for extension in (python / 'lib/python3.12/lib-dynload').glob('_dbm.*'):
            extension.unlink()
        (python / 'PLAZA-CHANGES.txt').write_text('CPython packaging changes: omit _dbm (Berkeley DB), bundled pip and bytecode caches; retain only the versioned Python executable and shared library; add locked Optuna packages. Python source code is unchanged.\n')
        site = python / 'lib/python3.12/site-packages'
        subprocess.run(['uv', 'pip', 'install', '--target', str(site), '--python-version', '3.12',
                        '--python-platform', {'amd64':'x86_64-unknown-linux-gnu', 'arm64':'aarch64-unknown-linux-gnu'}[arch],
                        '--only-binary', ':all:', '--require-hashes', '--no-python-downloads',
                        '-r', str(Path(__file__).parent / 'requirements.lock')], check=True)
        # Preserve upstream licenses, including transitive/native-library notices.
        notices = python / 'PLAZA-NOTICES.txt'
        licenses = sorted(p.relative_to(python).as_posix() for p in python.rglob('*') if p.is_file() and any(x in p.name.lower() for x in ('license', 'copying', 'notice')))
        notices.write_text('AHC Plaza bundled runtime\nCPython source: https://github.com/astral-sh/python-build-standalone/releases/tag/' + LOCK['release'] + '\n\nLicense files retained in this payload:\n' + '\n'.join(licenses) + '\n')
        # Stable ordering/metadata; dereference upstream links so the runtime extractor rejects all links.
        unpacked = 0
        partial = target.with_suffix('.tmp')
        with partial.open('wb') as raw, gzip.GzipFile(fileobj=raw, mode='wb', mtime=0, filename='') as compressed, tarfile.open(fileobj=compressed, mode='w', dereference=True) as output:
            for path in sorted(python.rglob('*')):
                if path.is_dir():
                    continue
                relative = path.relative_to(python).as_posix()
                if (path.is_symlink() and relative != 'bin/python3.12') or '__pycache__' in path.parts or relative.startswith('lib/python3.12/site-packages/pip'):
                    continue
                contents = path.read_bytes()
                info = tarfile.TarInfo(relative)
                info.size = len(contents)
                info.mode = 0o755 if os.access(path, os.X_OK) else 0o644
                output.addfile(info, io.BytesIO(contents))
                unpacked += len(contents)
        digest = hashlib.sha256(partial.read_bytes()).hexdigest()
        partial.replace(target)
        metadata.write_text(json.dumps({'build_hash': fingerprint, 'sha256': digest, 'python': LOCK['version'], 'optuna':'5.0.0', 'compressed_bytes':target.stat().st_size, 'unpacked_bytes':unpacked, 'arch':arch}, indent=2) + '\n')
        print(metadata.read_text())

if __name__ == '__main__':
    main()
