#!/usr/bin/env python3
"""Verify a complete platform set, create aliases, and checksum every downloadable file."""
from pathlib import Path
import hashlib
import shutil
import sys
folder, version = Path(sys.argv[1]), sys.argv[2]
required = [f'net-merge-setup-{version}.exe',f'net-merge-{version}-portable.zip',
            f'net-merge-{version}-macos-universal.tar.gz',f'net-merge-{version}-android.apk',
            f'net-merge-{version}-ios-arm64-unsigned.ipa']
architectures = ['x86_64','aarch64','armv7','armv6','i686','riscv64','ppc64le','s390x','loongarch64']
required += [f'net-merge-{version}-linux-{arch}.tar.gz' for arch in architectures]
for name in required:
    assert (folder/name).stat().st_size > 100_000, f'Missing or empty release asset: {name}'
for extension in ('*.deb','*.rpm','*.apk','*.pkg.tar.zst'):
    # .apk includes Android, so count the Alpine architecture packages explicitly below.
    assert len(list(folder.glob(extension))) >= (8 if extension == '*.deb' else 10 if extension == '*.apk' else 9), extension
for name in required:
    alias = name.replace(f'-{version}', '')
    if '-linux-armv7.' in alias: alias = alias.replace('-armv7.','-armv7l.')
    if '-linux-armv6.' in alias: alias = alias.replace('-armv6.','-armv6l.')
    shutil.copyfile(folder/name, folder/alias)
for name in ['LICENSE','THIRD-PARTY-NOTICES.md']:
    shutil.copyfile(name,folder/name)
shutil.copyfile('mobile/android/LICENSE',folder/'LICENSE-Android.txt')
shutil.copyfile('mobile/README.md',folder/'PHONE-BUILDS.md')
with (folder/'SHA256SUMS').open('w') as sums:
    for path in sorted(folder.iterdir()):
        if path.is_file() and path.name != 'SHA256SUMS':
            sums.write(f'{hashlib.file_digest(path.open("rb"),"sha256").hexdigest()}  {path.name}\n')
print(f'Complete release: {len(list(folder.iterdir()))} assets including checksums.')
