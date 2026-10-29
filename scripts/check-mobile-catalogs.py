#!/usr/bin/env python3
"""Fail CI on missing phone translations and mismatched format arguments."""
from pathlib import Path
import json
import re
import xml.etree.ElementTree as ET
root = Path(__file__).resolve().parents[1]
base = root / 'mobile/android/app/src/main/res'
catalogs = [{e.attrib['name']: e.text or '' for e in ET.parse(base / folder / 'strings.xml').getroot()} for folder in ('values', 'values-ar')]
assert catalogs[0].keys() == catalogs[1].keys(), 'Android catalog keys differ'
for key in catalogs[0]:
    assert re.findall(r'%\d+\$\w', catalogs[0][key]) == re.findall(r'%\d+\$\w', catalogs[1][key]), key
source = '\n'.join(p.read_text() for p in (root/'mobile/android/app/src/main/java').rglob('*.kt'))
for key in re.findall(r'R.string.(\w+)', source):
    assert key in catalogs[0], f'missing Android text: {key}'
ios = []
for lang in ('en','ar'):
    text = (root/f'mobile/ios/Shared/{lang}.lproj/Localizable.strings').read_text()
    values = {}
    for line in text.splitlines():
        key, value = line.removesuffix(';').split(' = ',1)
        values[json.loads(key)] = json.loads(value)
    ios.append(values)
assert ios[0].keys() == ios[1].keys(), 'iOS catalog keys differ'
for key in ios[0]:
    assert ios[0][key].count('%@') == ios[1][key].count('%@'), key
source = '\n'.join(p.read_text() for p in (root/'mobile/ios').rglob('*.swift'))
for key in re.findall(r'L\("([^"]+)"\)',source):
    assert key in ios[0], f'missing iOS text: {key}'
print(f'Phone catalogs verified: {len(catalogs[0])} Android and {len(ios[0])} iOS messages in English and Arabic.')
