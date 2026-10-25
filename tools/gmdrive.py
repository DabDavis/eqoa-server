#!/usr/bin/env python3
"""gmdrive -- drive the GM panel (internal/gmweb) in a headless Chromium over
the DevTools protocol: sign in, visit pages, screenshot them, and print the
page's console errors, for testing the panel without a person at it.

    tools/gmdrive.py http://127.0.0.1:18081/ GMNAME PASSWORD steps.json outdir

steps.json is a list of steps, each optional field done in this order:
    {"hash": "#/npcs/500", "file": ["input[type=file]", "/path/to/disc.iso"],
     "mouse": [["click", 120, 80], ["down", 10, 10], ["move", 40, 10], ["up", 40, 10]], "at": "canvas",
     "key": "Delete", "wait": 3, "js": "document.title", "label": "what js prints", "shot": "npc.png"}
"file" puts a file into a file input (the game disc on #/disc). "mouse" is the
left button as a person moves it (real input, so pointer capture works), at
CSS pixels from the top left of the element "at" names (the page's without);
"key" presses a key ("Delete", "Escape", "Enter", or a character). WebGL runs on
the GPU (Vulkan ANGLE). DARK=1 emulates a dark colour scheme; MOBILE=1 with
SIZE=390,844 a phone. Needs the `websockets` package. The browser's profile
is kept in outdir/profile (a disc's kept files land there: delete it after)."""
import asyncio, base64, json, os, shutil, subprocess, sys, time, urllib.request
from pathlib import Path
import websockets

BASE, USER, PASS, STEPS, OUT = sys.argv[1:6]
PORT = 9333
SIZE = os.environ.get('SIZE', '1440,900')


async def main():
    prof = Path(OUT) / 'profile'
    prof.mkdir(parents=True, exist_ok=True)
    tmp = Path(OUT) / 'tmp'
    tmp.mkdir(exist_ok=True)
    flags = [shutil.which('chromium'), '--headless=new', f'--user-data-dir={prof}', f'--remote-debugging-port={PORT}',
             '--no-first-run', '--no-default-browser-check', f'--window-size={SIZE}',
             '--use-angle=vulkan', '--enable-features=Vulkan', '--ignore-gpu-blocklist', '--enable-gpu']
    env = dict(os.environ, TMPDIR=str(tmp), DBUS_SESSION_BUS_ADDRESS='disabled:')
    proc = subprocess.Popen(flags + ['about:blank'], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        for _ in range(100):
            try:
                urllib.request.urlopen(f'http://127.0.0.1:{PORT}/json/version', timeout=1)
                break
            except OSError:
                await asyncio.sleep(0.1)
        req = urllib.request.Request(f'http://127.0.0.1:{PORT}/json/new?about:blank', method='PUT')
        target = json.load(urllib.request.urlopen(req))
        await drive(target['webSocketDebuggerUrl'])
    finally:
        proc.terminate()
        try:
            proc.wait(5)
        except subprocess.TimeoutExpired:
            proc.kill()


async def drive(ws_url):
    steps = json.loads(Path(STEPS).read_text())
    errors = []
    async with websockets.connect(ws_url, max_size=1 << 28) as ws:
        seq, pending = 0, {}

        async def call(method, **params):
            nonlocal seq
            seq += 1
            fut = asyncio.get_running_loop().create_future()
            pending[seq] = fut
            await ws.send(json.dumps({'id': seq, 'method': method, 'params': params}))
            return await fut

        async def reader():
            async for raw in ws:
                m = json.loads(raw)
                if 'id' in m and m['id'] in pending:
                    pending.pop(m['id']).set_result(m.get('result', m.get('error')))
                elif m.get('method') == 'Runtime.consoleAPICalled':
                    p = m['params']
                    text = ' '.join(str(x.get('value', x.get('description', ''))) for x in p['args'])
                    if p.get('type') in ('error', 'warning'):
                        errors.append(f"console.{p['type']}: {text}")
                    print('console', p.get('type'), text[:300], flush=True)
                elif m.get('method') == 'Runtime.exceptionThrown':
                    d = m['params']['exceptionDetails']
                    msg = 'EXCEPTION ' + d.get('exception', {}).get('description', d.get('text', ''))
                    errors.append(msg)
                    print(msg[:500], flush=True)
                elif m.get('method') == 'Log.entryAdded':
                    e = m['params']['entry']
                    if e.get('level') in ('error', 'warning'):
                        errors.append(f"log {e.get('level')}: {e.get('text')} {e.get('url', '')}")
                        print('log', e.get('level'), e.get('text'), e.get('url', ''), flush=True)

        rt = asyncio.ensure_future(reader())
        await call('Runtime.enable')
        await call('Log.enable')
        await call('Page.enable')
        if os.environ.get('DARK'):
            await call('Emulation.setEmulatedMedia', features=[{'name': 'prefers-color-scheme', 'value': 'dark'}])
        if os.environ.get('MOBILE'):
            w, hgt = map(int, SIZE.split(','))
            await call('Emulation.setDeviceMetricsOverride', width=w, height=hgt, deviceScaleFactor=2, mobile=True)
        await call('Page.navigate', url=BASE)
        await asyncio.sleep(2)

        async def js(expr):
            r = await call('Runtime.evaluate', expression=expr, awaitPromise=True, returnByValue=True)
            if r and r.get('exceptionDetails'):
                print('js error', r['exceptionDetails'].get('exception', {}).get('description'))
            return (r or {}).get('result', {}).get('value')

        # sign in
        await js(f"""(async () => {{
            const u = document.querySelector('input[name=username]'), p = document.querySelector('input[name=password]');
            if (!u) return 'no form';
            u.value = {json.dumps(USER)}; p.value = {json.dumps(PASS)};
            document.querySelector('form').requestSubmit();
            return 'submitted';
        }})()""")
        await asyncio.sleep(2)
        print('signed in as', await js("document.querySelector('.who strong')?.textContent"), flush=True)
        for st in steps:
            if 'hash' in st:
                await js(f"location.hash = {json.dumps(st['hash'])}")
            if 'file' in st:
                doc = await call('DOM.getDocument')
                node = await call('DOM.querySelector', nodeId=doc['root']['nodeId'], selector=st['file'][0])
                await call('DOM.setFileInputFiles', nodeId=node['nodeId'], files=[st['file'][1]])
            if 'mouse' in st:
                ox, oy = 0, 0
                if st.get('at'):
                    r = await js(f"(() => {{ const b = document.querySelector({json.dumps(st['at'])}).getBoundingClientRect(); return [b.left, b.top]; }})()")
                    ox, oy = r
                for act, x, y in st['mouse']:
                    x, y = ox + x, oy + y
                    kinds = {'down': ['mousePressed'], 'up': ['mouseReleased'], 'move': ['mouseMoved'],
                             'click': ['mouseMoved', 'mousePressed', 'mouseReleased']}[act]
                    for k in kinds:
                        held = k == 'mousePressed' or (k == 'mouseMoved' and act == 'move' and st.get('_down'))
                        await call('Input.dispatchMouseEvent', type=k, x=x, y=y, button='left' if k != 'mouseMoved' or held else 'none',
                                   buttons=1 if held else 0, clickCount=1 if k != 'mouseMoved' else 0)
                        await asyncio.sleep(0.05)
                    if act == 'down':
                        st['_down'] = True
                    elif act == 'up':
                        st['_down'] = False
            if 'key' in st:
                k = st['key']
                codes = {'Delete': 46, 'Escape': 27, 'Enter': 13, 'Backspace': 8}
                vk = codes.get(k, ord(k.upper()) if len(k) == 1 else 0)
                for t in ('keyDown', 'keyUp'):
                    await call('Input.dispatchKeyEvent', type=t, key=k, code=k if len(k) > 1 else 'Key' + k.upper(), windowsVirtualKeyCode=vk)
            await asyncio.sleep(st.get('wait', 2))
            if 'js' in st:
                print('js', st.get('label', ''), '=>', json.dumps(await js(st['js']))[:1500], flush=True)
            if 'shot' in st:
                r = await call('Page.captureScreenshot', format='png')
                Path(OUT, st['shot']).write_bytes(base64.b64decode(r['data']))
                print('shot', st['shot'], flush=True)
        rt.cancel()
    print('ERRORS:', len(errors))
    for e in errors:
        print('  ', e[:400])


asyncio.run(main())
