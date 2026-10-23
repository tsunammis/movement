#!/usr/bin/env python3
"""Retained A20 release check. Builds only a disposable source copy.

Requires Go, npm/Node, installed Chrome and the workspace Playwright dependency,
plus cached dependencies or network access for npm ci.
Run from any directory: python3 tests/acceptance/packaging.py
All runtime checks use an isolated binary with PATH pointing at an empty directory.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import selectors
import shutil
import subprocess
import tempfile
import urllib.error
import urllib.request

SOURCE = Path(__file__).resolve().parents[2]


def run(args, *, cwd, env, succeeds=True):
    result = subprocess.run(args, cwd=cwd, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=180)
    if succeeds and result.returncode:
        raise AssertionError(f'{args} failed ({result.returncode}):\n{result.stdout}')
    return result


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def fetch(url, expected=200, token=None):
    request = urllib.request.Request(url, headers={'Authorization': 'Bearer '+token} if token else {})
    try:
        response = urllib.request.urlopen(request, timeout=10)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        assert response.status == expected, (url, response.status, response.read())
        return response.read(), response.headers


def main():
    with tempfile.TemporaryDirectory(prefix='movement-packaging-') as temporary:
        root = Path(temporary)
        checkout = root/'source'
        # Deliberately do not rely on git tracking: the working tree is authoritative.
        shutil.copytree(SOURCE, checkout, ignore=shutil.ignore_patterns(
            '.git', '.agents', '.codex', '.aws', '.env', '.env.*', '.movement',
            'node_modules', 'dist', 'bin', 'test-results', 'playwright-report', '__pycache__'))
        assert not (checkout/'web/node_modules').exists()
        assert not (checkout/'internal/web/dist').exists()
        env = os.environ.copy()
        env['GOWORK'] = 'off'
        run(['/bin/sh', 'scripts/build.sh'], cwd=checkout, env=env)
        binary = checkout/'bin/movement'
        assert binary.is_file()
        original = sha(binary)
        print('PASS clean source build without dependencies or generated assets', flush=True)

        # A frontend error must not call Go or replace a prior valid binary.
        wrapper = root/'tool-wrapper'
        wrapper.mkdir()
        marker = root/'unexpected-go-invocation'
        shim = wrapper/'go'
        shim.write_text('#!/bin/sh\nprintf invoked > "$MOVEMENT_GO_MARKER"\nexit 98\n')
        shim.chmod(0o700)
        broken = checkout/'web/src/packaging-typecheck.ts'
        broken.write_text('export const packagingMustTypecheck: number = "invalid";\n')
        failure_env = {**env, 'PATH': str(wrapper)+os.pathsep+env['PATH'],
                       'MOVEMENT_GO_MARKER': str(marker)}
        result = run(['/bin/sh', 'scripts/build.sh'], cwd=checkout, env=failure_env, succeeds=False)
        assert result.returncode != 0 and 'packaging-typecheck.ts' in result.stdout, result.stdout
        assert not marker.exists(), 'Go ran after frontend failed'
        assert sha(binary) == original, 'frontend failure replaced the prior binary'
        print('PASS frontend type error stops before Go and preserves prior executable', flush=True)

        broken.unlink()
        stale = checkout/'internal/web/dist/stale-asset.js'
        stale.write_text('This must not enter the next release.')
        run(['/bin/sh', 'scripts/build.sh'], cwd=checkout, env=env)
        assert not stale.exists(), 'production build reused a stale asset'
        print('PASS clean asset rebuild excludes stale output', flush=True)

        runtime = root/'runtime'
        runtime.mkdir()
        isolated = runtime/'movement'
        shutil.copy2(binary, isolated)
        shutil.rmtree(checkout)  # no source, node_modules or external UI files remain
        empty_path = runtime/'empty-path'
        empty_path.mkdir()
        runtime_env = {**env, 'PATH': str(empty_path)}
        # No Node discovery through PATH; launch the absolute binary directly.
        assert shutil.which('node', path=runtime_env['PATH']) is None
        directory = runtime/'data'
        cli = [str(isolated), '--data-dir', str(directory)]
        run(cli+['init', '--timezone', 'Europe/Paris'], cwd=runtime, env=runtime_env)
        token = run(cli+['auth', 'token'], cwd=runtime, env=runtime_env).stdout.strip()
        payload = runtime/'task-input.json'
        payload.write_text(json.dumps({'title':'Standalone packaged task', 'status':'ready'}))
        receipt = json.loads(run(cli+['task', 'create', '--op', 'packaging-create', '--input', str(payload)], cwd=runtime, env=runtime_env).stdout)
        process = subprocess.Popen(cli+['serve', '--listen', '127.0.0.1:0'], cwd=runtime,
                                   env=runtime_env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            with selectors.DefaultSelector() as selector:
                selector.register(process.stderr, selectors.EVENT_READ)
                assert selector.select(15), 'standalone server did not start'
                line = process.stderr.readline()
            match = re.search(r'Movement API: (http://\S+)', line)
            assert match, line
            origin = match[1]
            html, headers = fetch(origin+'/')
            assert 'text/html' in headers['Content-Type']
            assert b'id="root"' in html
            paths = re.findall(rb'(?:src|href)="([^"]+)"', html)
            assert paths and all(p.startswith(b'/assets/') for p in paths), paths
            for path in paths:
                content, asset_headers = fetch(origin+path.decode())
                assert content, path
                assert 'text/html' not in asset_headers['Content-Type'], path
            # Hash routes request the same embedded entry point on browser refresh.
            refreshed, _ = fetch(origin+'/#task/'+receipt['task_id'])
            assert refreshed == html
            task, _ = fetch(origin+'/api/v1/tasks/'+receipt['task_id'], token=token)
            assert json.loads(task)['title'] == 'Standalone packaged task'
            fetch(origin+'/api/v1/tasks', expected=401)
            fetch(origin+'/api/v1/not-a-route', expected=404, token=token)
            fetch(origin+'/assets/not-a-file.js', expected=404)
            fetch(origin+'/assets/', expected=404)
            fetch(origin+'/stale-asset.js', expected=404)
            print('PASS isolated binary: CLI/API/embedded assets, hash refresh and correct errors without Node or UI files', flush=True)
            # Browser tools run outside the isolated server environment. No UI
            # source/dependencies remain in the server's copied build directory.
            node = shutil.which('node')
            assert node, 'Node is required by the browser test harness, not the server'
            browser = subprocess.run([node, str(SOURCE/'tests/acceptance/packaging-browser.mjs')],
                                     input=json.dumps({'origin':origin,'token':token,'task_id':receipt['task_id']}),
                                     cwd=SOURCE, env=env, text=True, stdout=subprocess.PIPE,
                                     stderr=subprocess.STDOUT, timeout=60)
            assert browser.returncode == 0, browser.stdout
            print(browser.stdout.strip(), flush=True)
            cli_task = json.loads(run(cli+['task','get',receipt['task_id'],'--json'], cwd=runtime, env=runtime_env).stdout)
            assert cli_task['title'] == 'Edited in isolated packaged UI'

        finally:
            process.terminate()
            try:
                stdout, stderr = process.communicate(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                stdout, stderr = process.communicate()
                raise AssertionError('standalone server failed graceful shutdown')
            assert stdout == '', 'HTTP server wrote unexpected stdout'
            assert process.returncode == 0, stderr


if __name__ == '__main__':
    main()
