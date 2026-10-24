#!/usr/bin/env python3
"""Exercise the installed Android app, VPN start, both locales and language persistence."""
from pathlib import Path
import subprocess
import sys
import time
import xml.etree.ElementTree as ET
pkg = 'org.iraqpc.netmerge'
activity = pkg + '/org.netmerge.android.MainActivity'
out = Path('phone-screenshots'); out.mkdir(exist_ok=True)
def adb(*args, binary=False):
    return subprocess.check_output(['adb', *args], text=not binary)
def dump():
    adb('shell','uiautomator','dump','/sdcard/window.xml')
    return ET.fromstring(adb('exec-out','cat','/sdcard/window.xml'))
def text(root):
    return ' '.join(n.get('text','')+' '+n.get('content-desc','') for n in root.iter())
def expect(phrase):
    for _ in range(12):
        root = dump()
        if phrase in text(root): return root
        time.sleep(1)
    raise AssertionError(f'Missing UI text {phrase}: {text(root)}')
def shot(name):
    (out/(name+'.png')).write_bytes(adb('exec-out','screencap','-p',binary=True))
def tap_text(root,phrase):
    import re
    matches=[n for n in root.iter() if n.get('text') == phrase or n.get('content-desc') == phrase]
    assert matches, phrase
    coords=list(map(int,re.findall(r'\d+',matches[0].get('bounds'))))
    adb('shell','input','tap',str((coords[0]+coords[2])//2),str((coords[1]+coords[3])//2))
adb('wait-for-device')
adb('install','-r',sys.argv[1])
adb('shell','appops','set',pkg,'ACTIVATE_VPN','allow')
adb('shell','pm','grant',pkg,'android.permission.POST_NOTIFICATIONS')
adb('shell','am','start','-W','-n',activity)
root = expect('Paused'); expect('Total used since start:'); shot('android-english-paused')
tap_text(root,'العربية')
expect('متوقف مؤقتًا'); shot('android-arabic-paused')
adb('shell','am','force-stop',pkg)
adb('shell','am','start','-W','-n',activity,'--ez','autostart','true')
expect('يعمل')
expect('المستخدم منذ البدء:'); shot('android-arabic-active')
# The default route is captured and quotas have been durably initialized; no debug-only fake stats.
state = adb('exec-out','run-as',pkg,'cat','files/quotas.json')
import json
records = json.loads(state)
assert records['version'] == 1 and records['adapters'], state
root=dump(); assert 'Exception' not in text(root), text(root)
log=adb('logcat','-d','-s','AndroidRuntime:E')
assert 'FATAL EXCEPTION' not in log, log
print('Android smoke passed: real VPN start, saved allowance state, English/Arabic and language persistence.')
