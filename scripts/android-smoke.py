#!/usr/bin/env python3
"""Exercise the installed Android app, VPN start, both locales and language persistence."""
from pathlib import Path
import re
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
def tap_text(root,phrase,index=0):
    matches=[n for n in root.iter() if n.get('text') == phrase or n.get('content-desc') == phrase]
    assert matches, phrase
    tap_node(matches[index])
def center(n):
    coords=list(map(int,re.findall(r'\d+',n.get('bounds'))))
    return (coords[0]+coords[2])//2,(coords[1]+coords[3])//2
def tap_node(n):
    x,y=center(n)
    adb('shell','input','tap',str(x),str(y))
def scroll_to_top():
    for _ in range(4):
        adb('shell','input','swipe','500','600','500','1800','250')
def open_limits(name):
    scroll_to_top()
    for _ in range(8):
        root=dump()
        labels=[n for n in root.iter() if n.get('text')==name or n.get('content-desc')==name]
        buttons=[n for n in root.iter() if n.get('text')=='حد الاستخدام' or n.get('content-desc')=='حد الاستخدام']
        for label in labels:
            below=[n for n in buttons if center(n)[1]>center(label)[1]]
            if below:
                tap_node(min(below,key=lambda n:center(n)[1]))
                return
        adb('shell','input','swipe','500','1800','500','800','300')
    raise AssertionError(f'No visible limits button for {name}')
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
# Set both visible connections through the native settings UI. The saved state can
# gain a second adapter after the initial snapshot as Android finishes enumeration.
# Existing relayed traffic has already consumed more than a byte on each link.
for name in ('واي فاي', 'البيانات'):
    open_limits(name)
    root = expect('حد السرعة')
    boxes=[n for n in root.iter() if n.get('checkable') == 'true']
    assert len(boxes) == 2, ET.tostring(root,encoding='unicode')
    for box in boxes: tap_node(box)
    fields=[n for n in dump().iter() if n.get('class') == 'android.widget.EditText']
    assert len(fields)==2
    tap_node(fields[0]); adb('shell','input','text','2')
    tap_node(fields[1]); adb('shell','input','text','0.000000001')
    tap_text(dump(),'حفظ')
    time.sleep(2)
state=adb('exec-out','run-as',pkg,'cat','files/quotas.json')
records=json.loads(state)
assert all(v['limit_bytes']==1 and v['used_bytes']>=1 for v in records['adapters'].values()), state
scroll_to_top()
expect('نفدت حصص البيانات'); expect('يعمل'); shot('android-arabic-exhausted')
# A manual reset while paused must preserve the configured allowance and be confirmed.
tap_text(dump(),'إيقاف مؤقت'); expect('متوقف مؤقتًا')
tap_text(dump(),'تصفير استهلاك الحصة')
expect('هل تريد تصفير'); tap_text(dump(),'تصفير')
time.sleep(2)
records=json.loads(adb('exec-out','run-as',pkg,'cat','files/quotas.json'))
assert any(v['limit_bytes']==1 and v['used_bytes']==0 for v in records['adapters'].values()), records
shot('android-arabic-reset')
root=dump(); assert 'Exception' not in text(root), text(root)
log=adb('logcat','-d','-s','AndroidRuntime:E')
assert 'FATAL EXCEPTION' not in log, log
print('Android smoke passed: VPN start, limits, exhaustion, confirmed reset, English/Arabic and language persistence.')
