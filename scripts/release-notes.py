#!/usr/bin/env python3
from pathlib import Path
import re
import sys
version = sys.argv[1]
text = Path('CHANGELOG.md').read_text()
match = re.search(r'^## \['+re.escape(version)+r'\][^\n]*\n(.*?)(?=^## \[|\Z)',text,re.M|re.S)
if not match: raise SystemExit('Missing release changelog')
print(match.group(1).strip().replace('(mobile/README.md)','(https://github.com/DodoBird-debian/net-merge/blob/v'+version+'/mobile/README.md)'))
print('\nVerify downloads with the attached `SHA256SUMS`. Android phones use the `android.apk`; other `.apk` files are Alpine Linux packages. The unsigned iOS IPA requires your own signing and provisioning before use.')
