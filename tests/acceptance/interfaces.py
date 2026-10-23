"""Built-binary acceptance: independent CLI, MCP and HTTP processes; no real connectors.

Run: python3 tests/acceptance/interfaces.py /absolute/path/to/movement
Requires permission to bind an ephemeral loopback port. Stores are temporary.
"""
import concurrent.futures
import copy
import datetime as dt
import json
import os
from pathlib import Path
import selectors
import re
import signal
import subprocess
import sys
import tempfile
import threading
import time
import unittest
import urllib.error
import urllib.parse
import urllib.request
import uuid

BINARY = str(Path(sys.argv.pop(1)).resolve()) if len(sys.argv) > 1 else str(Path('bin/movement').resolve())
ACTOR = 'acceptance-agent'


def now():
    return dt.datetime.now(dt.timezone.utc).isoformat().replace('+00:00', 'Z')


def after(seconds):
    return (dt.datetime.now(dt.timezone.utc) + dt.timedelta(seconds=seconds)).isoformat().replace('+00:00', 'Z')


def instant(value):
    # Go RFC3339Nano omits trailing zeros; older Python needs 3/6-digit fractions.
    normalized = re.sub(r"\.(\d+)", lambda m: "." + m.group(1).ljust(6, "0")[:6], value)
    return dt.datetime.fromisoformat(normalized.replace("Z", "+00:00"))


def identity():
    return str(uuid.uuid4())


class Failure(Exception):
    def __init__(self, value):
        self.code = value['code']
        self.value = value
        super().__init__(self.code + ': ' + value['message'])


class MCP:
    def __init__(self, command):
        self.process = subprocess.Popen(command + ['mcp', '--stdio'], stdin=subprocess.PIPE,
                                        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.sequence = 0
        initialized = self.call('initialize', {'protocolVersion': '2025-11-25', 'capabilities': {},
                                 'clientInfo': {'name': 'movement-acceptance', 'version': '1'}})
        assert initialized['serverInfo']['name'] == 'movement'
        self.send({'jsonrpc': '2.0', 'method': 'notifications/initialized'})
        assert len(self.call('tools/list', {})['tools']) == 12

    def send(self, message):
        self.process.stdin.write(json.dumps(message) + '\n')
        self.process.stdin.flush()

    def call(self, method, params):
        self.sequence += 1
        self.send({'jsonrpc': '2.0', 'id': self.sequence, 'method': method, 'params': params})
        while True:
            with selectors.DefaultSelector() as selector:
                selector.register(self.process.stdout, selectors.EVENT_READ)
                if not selector.select(15):
                    raise RuntimeError('MCP response timeout')
            line = self.process.stdout.readline()
            if not line:
                raise RuntimeError('MCP exited: ' + self.process.stderr.read())
            message = json.loads(line)  # Any non-protocol stdout fails acceptance.
            if 'id' not in message:
                continue
            assert message['id'] == self.sequence, message
            if 'error' in message:
                raise RuntimeError(message['error'])
            return message['result']

    def tool(self, name, arguments):
        result = self.call('tools/call', {'name': name, 'arguments': arguments})
        value = result['structuredContent']
        text = json.loads(result['content'][0]['text'])
        assert text == value
        if result.get('isError'):
            raise Failure(value['error'])
        return value['result']

    def close(self):
        if self.process.poll() is None:
            self.process.stdin.close()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
        stderr = self.process.stderr.read()
        self.process.stdout.close()
        self.process.stderr.close()
        assert self.process.returncode == 0, stderr
        assert not stderr, stderr


class Store:
    def __init__(self):
        self.temp = tempfile.TemporaryDirectory(prefix='movement-interfaces-')
        self.command = [BINARY, '--data-dir', self.temp.name]
        subprocess.run(self.command + ['init', '--timezone', 'Europe/Paris'], check=True, capture_output=True)
        self.token = subprocess.check_output(self.command + ['auth', 'token'], text=True).strip()
        self.start()
        self.adapters = [Adapter(self, kind) for kind in ['cli', 'mcp', 'http']]

    def start(self):
        self.server = subprocess.Popen(self.command + ['serve', '--listen', '127.0.0.1:0'],
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        with selectors.DefaultSelector() as selector:
            selector.register(self.server.stderr, selectors.EVENT_READ)
            assert selector.select(10), 'HTTP startup timeout'
        line = self.server.stderr.readline().strip()
        assert line.startswith('Movement API: http://127.0.0.1:'), line
        self.url = line.removeprefix('Movement API: ') + '/api/v1'
        self.mcp = MCP(self.command)

    def stop(self):
        self.mcp.close()
        self.server.send_signal(signal.SIGTERM)
        try:
            self.server.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.server.kill()
            self.server.wait()
        assert self.server.returncode == 0
        assert not self.server.stdout.read()
        assert not self.server.stderr.read()
        self.server.stdout.close()
        self.server.stderr.close()

    def restart(self):
        self.stop()
        self.start()

    def close(self):
        try:
            self.stop()
        finally:
            self.temp.cleanup()

    def cli(self, arguments, value=None):
        result = subprocess.run(self.command + arguments, input=json.dumps(value) if value is not None else None,
                                capture_output=True, text=True, timeout=15)
        if result.returncode:
            error = json.loads(result.stderr)['error']
            expected = {'invalid_input': 2, 'not_found': 3, 'revision_conflict': 4,
                        'claim_conflict': 4, 'operation_conflict': 4, 'assessment_conflict': 4,
                        'context_conflict': 4, 'invalid_transition': 4, 'store_busy': 5}.get(error['code'], 1)
            assert result.returncode == expected
            raise Failure(error)
        assert not result.stderr, result.stderr
        return json.loads(result.stdout)

    def http(self, method, path, value=None):
        data = json.dumps(value).encode() if value is not None else None
        request = urllib.request.Request(self.url + path, data=data, method=method,
                  headers={'Authorization': 'Bearer ' + self.token, 'Content-Type': 'application/json'})
        try:
            with urllib.request.urlopen(request, timeout=15) as response:
                return json.load(response)
        except urllib.error.HTTPError as response:
            raise Failure(json.load(response)['error']) from None


class Adapter:
    def __init__(self, store, kind):
        self.store, self.kind = store, kind

    def mutate(self, kind, data=None, task=None, token=None, op=None, override=False):
        payload = {'operation_id': op or identity(), 'actor': ACTOR, 'data': data or {}}
        if task:
            payload['expected_revision'] = task['revision']
        if token:
            payload['claim_token'] = token
        if override:
            payload['override'] = True
        if self.kind == 'mcp':
            if task:
                payload['task_id'] = task['id']
            return self.store.mcp.tool('task_' + ('check_report' if kind == 'check' else kind), payload)
        if self.kind == 'http':
            suffix = {'claim': '/claim', 'renew': '/claim/renew', 'release': '/claim/release',
                      'report': '/reports', 'check': '/checks', 'observe': '/observations'}.get(kind, '')
            path = '/tasks' + ('/' + task['id'] if task else '') + suffix
            return self.store.http('PATCH' if kind == 'update' else 'POST', path, payload)
        args = ['task', 'check-report' if kind == 'check' else kind]
        if task:
            args += [task['id'], '--revision', str(task['revision'])]
        args += ['--op', payload['operation_id'], '--actor', ACTOR, '--input', '-']
        if token:
            args += ['--claim-token', token]
        if override:
            args += ['--override']
        return self.store.cli(args, payload['data'])

    def read(self, kind, task_id=None, query=None):
        query = query or {}
        if self.kind == 'mcp':
            return self.store.mcp.tool('task_' + kind, query if kind == 'list' else {'task_id': task_id, **query})
        if self.kind == 'http':
            path = '/tasks' + ('/' + task_id if task_id else '')
            if kind not in ['list', 'get']:
                path += '/' + kind
            if query:
                path += '?' + urllib.parse.urlencode(query, doseq=True)
            return self.store.http('GET', path)
        if kind == 'list':
            return self.store.cli(['task', 'list', '--json', '--query', '-'], query)
        args = ['task', kind, task_id, '--json']
        for key, value in query.items():
            args += ['--' + ('revision' if key == 'expected_revision' else key), str(value)]
        return self.store.cli(args)

    def task(self, task_id):
        return self.read('get', task_id)


def definition(title='Write an email to Kevin'):
    return {'title': title, 'status': 'ready', 'context': {'objective': 'Create an unsent project update draft',
            'constraints': ['Do not send. Resolve the recipient and project before acting.'], 'notes': 'Synthetic acceptance task'},
            'completion': {'version': 1, 'criteria': [{'id': 'draft', 'description': 'Draft exists and remains unsent'}]},
            'execution_guidance': {'version': 1, 'prompt': 'Read Jira status, deadline and comments, then save a Gmail draft.',
                'steps': ['Read project status', 'Inspect deadline', 'Summarize comments', 'Create a draft'],
                'input_refs': [{'name': 'project', 'reference': 'fake://jira/project'}, {'name': 'recipient', 'reference': 'fake://contacts/kevin'}],
                'required_capabilities': ['jira.read', 'gmail.drafts.create'],
                'expected_outputs': [{'kind': 'email_draft', 'description': 'An unsent draft'}]},
            'completion_check': {'version': 1, 'prompt': 'Inspect the saved artifact without sending it',
                                 'criterion_ids': ['draft'], 'source_refs': ['fake://gmail/drafts']}}


def report(task, outcome='met', results=None, outputs=None):
    observed = now()
    value = {'context_version': task['context_version'], 'completion_version': task['completion']['version'],
             'task_revision_used': task['revision'], 'observed_at': observed, 'outcome': outcome,
             'summary': 'Synthetic assessment: ' + outcome, 'evidence': [], 'outputs': outputs or []}
    if task.get('completion_check'):
        value['check_version'] = task['completion_check']['version']
    if task.get('execution_guidance'):
        value['guidance_version'] = task['execution_guidance']['version']
    if task.get('claim'):
        value['attempt_id'] = task['claim']['attempt_id']
    value['criterion_results'] = results if results is not None else [
        {'criterion_id': c['id'], 'result': 'met', 'evidence': [{'reference': 'fake://evidence/' + c['id'],
          'observed_at': observed, 'description': 'Synthetic verification'}]} for c in task['completion']['criteria']]
    return value


class FakeConnectors:
    """External simulator. Movement never calls this; the test acts as the assistant."""
    def __init__(self):
        self.drafts = {}
        self.creates = 0
        self.sends = 0
        self.access = True
        self.uncertain = False

    def project(self):
        if not self.access:
            raise PermissionError('Jira unavailable')
        return {'status': 'on track', 'deadline': '2026-10-30', 'comments': ['Design approved', 'Tests pending']}

    def draft(self, task_id, recipient, project):
        if not recipient or not project:
            raise ValueError('Resolve recipient and project')
        # Reconcile the external system before retrying an uncertain create.
        if task_id in self.drafts:
            return self.drafts[task_id]
        self.creates += 1
        saved = {'kind': 'email_draft', 'provider': 'gmail', 'reference': 'fake://gmail/drafts/' + task_id,
                 'observed_at': now(), 'description': f'Unsent to {recipient}: {project}'}
        self.drafts[task_id] = saved
        if self.uncertain:
            raise TimeoutError('Draft saved; response lost')
        return saved

    def send(self):
        self.sends += 1
        raise AssertionError('Sending is outside the task scope')


class Acceptance(unittest.TestCase):
    def setUp(self):
        self.store = Store()
        self.adapters = self.store.adapters

    def tearDown(self):
        self.store.close()

    def fails(self, expected, function):
        with self.assertRaises(Failure) as caught:
            function()
        self.assertEqual(caught.exception.code, expected)

    def create(self, adapter, data=None):
        receipt = adapter.mutate('create', data or definition())
        return adapter.task(receipt['task_id'])

    def test_restart_reads_and_receipt_replay(self):
        cli, mcp, http = self.adapters
        payload = definition()
        operation = identity()
        receipt = mcp.mutate('create', payload, op=operation)
        task_id = receipt['task_id']
        self.store.restart()
        for adapter in self.adapters:
            self.assertEqual(adapter.task(task_id)['title'], payload['title'])
            self.assertEqual(adapter.read('context', task_id)['task']['execution_guidance'], payload['execution_guidance'])
            self.assertEqual(adapter.mutate('create', payload, op=operation), receipt)
            self.fails('operation_conflict', lambda: adapter.mutate('create', {**payload, 'title': 'Different'}, op=operation))
        self.assertEqual(len(cli.read('history', task_id)), 1)

    def test_utc_storage_and_offset_equivalent_retries(self):
        timestamp_keys = {'created_at', 'updated_at', 'recorded_at', 'captured_at',
                          'delegated_at', 'observed_at', 'expires_at', 'scheduled_for',
                          'due_at', 'follow_up_at', 'next_follow_up_at'}

        def change_offsets(value, zone):
            if isinstance(value, list):
                return [change_offsets(item, zone) for item in value]
            if isinstance(value, dict):
                return {key: (instant(item).astimezone(zone).isoformat()
                              if key in timestamp_keys and isinstance(item, str)
                              else change_offsets(item, zone)) for key, item in value.items()}
            return value

        def assert_utc(value):
            if isinstance(value, list):
                for item in value:
                    assert_utc(item)
            if isinstance(value, dict):
                for key, item in value.items():
                    if key in timestamp_keys and isinstance(item, str):
                        self.assertTrue(item.endswith('Z'), (key, item))
                    else:
                        assert_utc(item)

        zone = dt.timezone(dt.timedelta(hours=5, minutes=30))
        opposite = dt.timezone(dt.timedelta(hours=-7))
        prose = 'Keep 2026-10-22T14:00:00+02:00 unchanged in prose'
        replays = []
        tasks = []
        for writer in self.adapters:
            data = definition('UTC through ' + writer.kind)
            data.update(scheduled_for=after(60), due_at=after(3600), follow_up_at=after(1800),
                        sources=[{'provider': 'fake', 'reference': 'fake://source',
                                  'captured_at': now(), 'summary': prose}],
                        delegation={'assignee': {'display_name': 'Nathan'},
                                    'delegated_at': now(), 'expected_outcome': prose})
            data = change_offsets(data, zone)
            op = identity()
            receipt = writer.mutate('create', data, op=op)
            task = writer.task(receipt['task_id'])
            replays.append(('create', data, None, None, op, receipt))
            fields = {key: data[key] for key in ['scheduled_for', 'due_at', 'follow_up_at', 'sources', 'delegation']}
            op = identity()
            update = {'fields': fields}
            receipt = writer.mutate('update', update, task, op=op)
            replays.append(('update', update, task, None, op, receipt))
            task = writer.task(task['id'])
            self.assertEqual(task['context_version'], 1)
            claim = writer.mutate('claim', {}, task)
            task = writer.task(task['id'])
            original = report(task, 'progress', [])
            original['evidence'] = [{'reference': 'fake://evidence', 'observed_at': now(), 'description': prose}]
            original['outputs'] = [{'kind': 'draft', 'provider': 'fake', 'reference': 'fake://draft',
                                    'observed_at': now(), 'description': prose}]
            work = change_offsets({'report': original}, zone)
            op = identity()
            receipt = writer.mutate('report', work, task, claim['claim_token'], op=op)
            replays.append(('report', work, task, claim['claim_token'], op, receipt))
            task = writer.task(task['id'])
            assessment = report(task, outputs=original['outputs'])
            assessment['evidence'] = original['evidence']
            check = change_offsets({'report': assessment, 'next_follow_up_at': after(7200)}, zone)
            op = identity()
            receipt = writer.mutate('check', check, task, claim['claim_token'], op=op)
            replays.append(('check', check, task, claim['claim_token'], op, receipt))
            task = writer.task(task['id'])
            observation = change_offsets({'report': original}, zone)
            op = identity()
            receipt = writer.mutate('observe', observation, task, op=op)
            replays.append(('observe', observation, task, None, op, receipt))
            tasks.append(task['id'])

        self.store.restart()
        for reader in self.adapters:
            for kind, data, old_task, token, op, receipt in replays:
                self.assertEqual(reader.mutate(kind, change_offsets(data, opposite), old_task, token, op=op), receipt)
            for task_id in tasks:
                task = reader.task(task_id)
                assert_utc(task)
                self.assertEqual(task['sources'][0]['summary'], prose)
                history = reader.read('history', task_id)
                self.assertEqual(len(history), 6)
                assert_utc(history)
                self.assertEqual(task['status'], 'seems_completed')

        for task_id in tasks:
            directory = Path(self.store.temp.name) / 'tasks' / task_id
            assert_utc(json.loads((directory / 'task.json').read_text()))
            events = list((directory / 'history').glob('*.md'))
            self.assertEqual(len(events), 6)
            values = []
            for event in events:
                front = event.read_text().split('---', 2)[1]
                for key, value in re.findall(r'^\s*"([a-z_]+)": "([^"\n]+)"$', front, re.MULTILINE):
                    if key in timestamp_keys:
                        self.assertTrue(value.endswith('Z'), (event.name, key, value))
                        values.append(key)
            for key in ['recorded_at', 'captured_at', 'delegated_at', 'observed_at', 'due_at', 'follow_up_at']:
                self.assertIn(key, values)

    def test_races_between_independent_adapters(self):
        for operation in ['update', 'claim']:
            task = self.create(self.adapters[0])
            barrier = threading.Barrier(3)
            def compete(adapter):
                barrier.wait()
                try:
                    return adapter.mutate(operation, {'fields': {'title': adapter.kind}} if operation == 'update' else {}, task)
                except Failure as error:
                    return error.code
            with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
                results = list(pool.map(compete, self.adapters))
            self.assertEqual(sum(isinstance(r, dict) for r in results), 1, results)
            self.assertEqual(results.count('revision_conflict'), 2, results)
            current = self.adapters[0].task(task['id'])
            self.assertEqual(current['revision'], 2)
            self.assertEqual(len(self.adapters[2].read('history', task['id'])), 2)
            if operation == 'claim':
                winner = next(r for r in results if isinstance(r, dict))
                token = winner['claim_token']
                self.assertEqual(self.adapters[0].mutate('claim', {}, task, op=winner['operation_id']), winner)
                renew_op = identity()
                renewed = self.adapters[1].mutate('renew', {'seconds': 60}, current, token, renew_op)
                self.assertEqual(self.adapters[2].mutate('renew', {'seconds': 60}, current, token, renew_op), renewed)
                self.assertEqual(renewed['revision'], 3)
                current = self.adapters[2].task(task['id'])
                release_op = identity()
                released = self.adapters[2].mutate('release', {}, current, token, release_op)
                self.assertEqual(self.adapters[0].mutate('release', {}, current, token, release_op), released)
                self.assertEqual(self.adapters[0].task(task['id'])['status'], 'ready')

    def test_ingestion_preserves_decisions_and_occurrences(self):
        for adapter in self.adapters:
            payload = {'title': 'Fortnightly', 'ingestion': {'producer': adapter.kind,
                       'external_key': json.dumps(['document', 'fake://fortnightly', 'fill', 'period-1'], separators=(',', ':'))}}
            task = self.create(adapter, payload)
            adapter.mutate('update', {'action': 'accept', 'fields': {'priority': 'high', 'title': 'My wording'}}, task)
            task = adapter.task(task['id'])
            payload['title'] = 'Changed discovery title'
            payload['due_at'] = after(600)
            self.assertTrue(adapter.mutate('create', payload)['existing'])
            self.assertEqual(adapter.task(task['id'])['priority'], 'high')
            self.assertEqual(adapter.task(task['id'])['title'], 'My wording')
            adapter.mutate('update', {'action': 'complete'}, task)
            self.assertTrue(adapter.mutate('create', payload)['existing'])
            self.assertEqual(adapter.task(task['id'])['status'], 'done')
            payload['ingestion']['external_key'] = payload['ingestion']['external_key'].replace('period-1', 'period-2')
            new = self.create(adapter, payload)
            self.assertNotEqual(new['id'], task['id'])
            adapter.mutate('update', {'action': 'dismiss'}, new)
            self.assertEqual(adapter.mutate('create', payload)['task_id'], new['id'])
            self.assertEqual(adapter.task(new['id'])['status'], 'dismissed')

    def test_delegation_checks_and_atomic_followups(self):
        for adapter in self.adapters:
            data = definition('Fill the fortnightly')
            data.update(flow='waiting_for', due_at=after(3600), follow_up_at=after(-60),
                        delegation={'assignee': {'display_name': 'Nathan Delange'}, 'delegated_at': now(),
                                    'expected_outcome': 'Sections A, B, C and D for this period'})
            data['completion']['criteria'] = [{'id': c, 'description': 'Section ' + c + ' is present'} for c in 'ABCD']
            data['completion_check']['criterion_ids'] = list('ABCD')
            task = self.create(adapter, data)
            self.fails('claim_conflict', lambda: adapter.mutate('claim', {}, task))
            selected = adapter.read('list', query={'flow': 'waiting_for', 'assignee': 'Nathan Delange', 'time_field': 'follow_up'})
            self.assertIn(task['id'], [x['task']['id'] for x in selected['items']])
            executable = adapter.read('list', query={'view': 'execution', 'flow': 'all'})
            self.assertNotIn(task['id'], [x['task']['id'] for x in executable['items']])
            inaccessible = report(task, 'inconclusive', [])
            inaccessible['summary'] = 'Source access unavailable; no negative conclusion'
            adapter.mutate('check', {'report': inaccessible}, task)
            current = adapter.task(task['id'])
            self.assertEqual(current['status'], 'ready')
            self.assertEqual(current['follow_up_at'], task['follow_up_at'])
            packet = adapter.read('context', task['id'])
            self.assertFalse(packet['follow_up']['pending'])
            negative = report(current, 'not_met')
            negative['criterion_results'][-1]['result'] = 'not_met'
            future = after(7200)
            op = identity()
            receipt = adapter.mutate('check', {'report': negative, 'next_follow_up_at': future}, current, op=op)
            self.assertEqual(adapter.mutate('check', {'report': negative, 'next_follow_up_at': future}, current, op=op), receipt)
            current = adapter.task(task['id'])
            self.assertEqual(current['status'], 'ready')
            self.assertEqual(instant(current['follow_up_at']), instant(future))
            self.assertTrue(adapter.read('context', task['id'])['follow_up']['pending'])
            self.fails('revision_conflict', lambda: adapter.mutate('check', {'report': negative, 'next_follow_up_at': None}, task))
            positive = report(current)
            adapter.mutate('check', {'report': positive, 'next_follow_up_at': None}, current)
            current = adapter.task(task['id'])
            self.assertEqual(current['status'], 'seems_completed')
            self.assertIsNone(current['follow_up_at'])
            adapter.mutate('check', {'report': report(current, 'inconclusive', [])}, current)
            current = adapter.task(task['id'])
            self.assertEqual(current['status'], 'seems_completed')
            recheck = report(current, 'not_met')
            recheck['criterion_results'][-1]['result'] = 'not_met'
            adapter.mutate('check', {'report': recheck}, current)
            current = adapter.task(task['id'])
            self.assertEqual(current['status'], 'ready')
            adapter.mutate('check', {'report': report(current)}, current)
            current = adapter.task(task['id'])
            adapter.mutate('update', {'action': 'complete'}, current)
            current = adapter.task(task['id'])
            self.assertEqual(current['status'], 'done')
            self.assertEqual(current['delegation']['assignee']['display_name'], 'Nathan Delange')
            self.assertEqual(current['flow'], 'waiting_for')
            self.assertEqual(instant(current['due_at']), instant(data['due_at']))
            self.assertEqual(len(adapter.read('history', task['id'])), 8)

    def test_personal_guidance_blockers_draft_and_uncertain_retry(self):
        for adapter in self.adapters:
            fake = FakeConnectors()
            task = self.create(adapter)
            context = adapter.read('context', task['id'])
            self.assertIn('Do not send', context['task']['context']['constraints'][0])
            self.assertEqual(len(context['task']['execution_guidance']['steps']), 4)
            claim_op = identity()
            claim = adapter.mutate('claim', {}, task, op=claim_op)
            self.assertEqual(adapter.mutate('claim', {}, task, op=claim_op), claim)
            current = adapter.task(task['id'])
            fake.access = False
            with self.assertRaises(PermissionError):
                fake.project()
            for recipient, project in [('', {'status': 'known'}), ('kevin@example.invalid', None)]:
                with self.assertRaises(ValueError):
                    fake.draft(task['id'], recipient, project)
            blocked = report(current, 'blocked', [])
            blocked['summary'] = 'Missing Jira access and unresolved recipient/project'
            op = identity()
            receipt = adapter.mutate('report', {'report': blocked}, current, claim['claim_token'], op)
            self.assertEqual(adapter.mutate('report', {'report': blocked}, current, claim['claim_token'], op), receipt)
            current = adapter.task(task['id'])
            self.assertEqual(current['status'], 'blocked')
            self.assertNotIn('claim', current)
            adapter.mutate('update', {'action': 'resume'}, current)
            current = adapter.task(task['id'])
            claim = adapter.mutate('claim', {}, current)
            current = adapter.task(task['id'])
            progress = report(current, 'progress', [])
            adapter.mutate('report', {'report': progress}, current, claim['claim_token'])
            current = adapter.task(task['id'])
            fake.access = True
            project = fake.project()
            fake.uncertain = True
            with self.assertRaises(TimeoutError):
                fake.draft(task['id'], 'kevin@example.invalid', project)
            output = fake.draft(task['id'], 'kevin@example.invalid', project)
            proposed = report(current, 'completion_proposed', outputs=[output])
            op = identity()
            receipt = adapter.mutate('report', {'report': proposed}, current, claim['claim_token'], op)
            self.assertEqual(adapter.mutate('report', {'report': proposed}, current, claim['claim_token'], op), receipt)
            self.assertEqual(fake.creates, 1)
            self.assertEqual(fake.sends, 0)
            self.store.restart()
            for reader in self.adapters:
                restored = reader.read('context', task['id'])
                self.assertEqual(restored['task']['status'], 'seems_completed')
                history = reader.read('history', task['id'])
                self.assertEqual(history[-1]['report']['outputs'][0]['reference'], output['reference'])
                self.assertTrue(all(e['actor'] == ACTOR for e in history))
            current = adapter.task(task['id'])
            adapter.mutate('update', {'action': 'complete'}, current)
            self.assertEqual(adapter.task(task['id'])['status'], 'done')

    def test_criteria_and_guidance_staleness(self):
        for adapter in self.adapters:
            data = definition()
            data['completion']['criteria'].append({'id': 'content', 'description': 'Current status included'})
            data['completion_check']['criterion_ids'].append('content')
            task = self.create(adapter, data)
            old = report(task)
            changed = copy.deepcopy(task['completion'])
            changed['criteria'][0]['description'] = 'Draft includes the current deadline'
            adapter.mutate('update', {'fields': {'completion': changed}}, task)
            current = adapter.task(task['id'])
            self.fails('context_conflict', lambda: adapter.mutate('check', {'report': old}, current))
            for variant in ['empty', 'partial', 'unknown', 'duplicate', 'aggregate']:
                invalid = report(current)
                if variant == 'empty':
                    invalid['criterion_results'] = []
                elif variant == 'partial':
                    invalid['criterion_results'] = invalid['criterion_results'][:1]
                elif variant == 'unknown':
                    invalid['criterion_results'][0]['criterion_id'] = 'unknown'
                elif variant == 'duplicate':
                    invalid['criterion_results'] *= 2
                else:
                    invalid['outcome'] = 'not_met'
                self.fails('invalid_input', lambda: adapter.mutate('check', {'report': invalid}, current))
            claim = adapter.mutate('claim', {}, current)
            current = adapter.task(task['id'])
            old_work = report(current, 'completion_proposed')
            guidance = copy.deepcopy(current['execution_guidance'])
            guidance['prompt'] += ' Include the revised risks.'
            adapter.mutate('update', {'fields': {'execution_guidance': guidance}}, current, claim['claim_token'])
            current = adapter.task(task['id'])
            self.fails('context_conflict', lambda: adapter.mutate('report', {'report': old_work}, current, claim['claim_token']))
            for results in [[], report(current)['criterion_results'][:1]]:
                partial = report(current, 'completion_proposed', results)
                self.fails('invalid_input', lambda: adapter.mutate('report', {'report': partial}, current, claim['claim_token']))
            self.assertEqual(adapter.task(task['id'])['status'], 'in_progress')

    def test_planning_selection_and_pagination_parity(self):
        cli = self.adapters[0]
        values = [('overdue', '2026-10-20T12:00:00Z', 'high'),
                  ('carryover', '2026-10-24T12:00:00Z', 'normal'),
                  ('window', '2026-10-26T12:00:00Z', 'normal'), ('undated', None, 'low'),
                  ('lower-bound', '2026-10-26T00:00:00Z', 'normal'),
                  ('upper-bound', '2026-11-02T00:00:00Z', 'normal'),
                  ('both', '2026-10-26T08:00:00Z', 'normal'), ('past-schedule', None, 'normal')]
        tasks = {}
        for name, due, priority in values:
            data = definition(name)
            data.update(due_at=due, priority=priority)
            if name == 'both':
                data['scheduled_for'] = '2026-10-26T06:00:00Z'
            if name == 'past-schedule':
                data['scheduled_for'] = '2026-10-20T12:00:00Z'
            tasks[name] = self.create(cli, data)
        claim = cli.mutate('claim', {}, tasks['overdue'])
        current = cli.task(tasks['overdue']['id'])
        cli.mutate('report', {'report': report(current, 'blocked', [])}, current, claim['claim_token'])
        current = tasks['window']
        cli.mutate('check', {'report': report(current)}, current)
        query = {'from': '2026-10-26T00:00:00Z', 'to': '2026-11-02T00:00:00Z', 'as_of': '2026-10-22T12:00:00Z'}
        baseline = cli.read('list', query=query)
        expected = [tasks[name]['id'] for name in ['overdue', 'carryover', 'lower-bound', 'both', 'window']]
        self.assertEqual([x['task']['id'] for x in baseline['items']], expected)
        self.assertTrue(baseline['items'][0]['is_overdue'])
        self.assertFalse(baseline['items'][1]['is_overdue'])
        self.assertEqual(baseline['items'][1]['reasons'], ['deadline_carryover'])
        for adapter in self.adapters:
            self.assertEqual(adapter.read('list', query=query), baseline)
            self.assertEqual([x['task']['id'] for x in adapter.read('list', query={**query, 'view': 'execution'})['items']], [tasks[name]['id'] for name in ['carryover', 'lower-bound', 'both']])
            page = adapter.read('list', query={**query, 'limit': 2})
            paginated = page['items'][:]
            while page.get('next_cursor'):
                page = adapter.read('list', query={**query, 'limit': 2, 'cursor': page['next_cursor']})
                paginated.extend(page['items'])
            self.assertEqual([x['task']['id'] for x in paginated], expected)
            self.assertEqual(baseline['items'][3]['reasons'], ['scheduled_in_window', 'due_in_window'])
            without_carryover = adapter.read('list', query={**query, 'include_overdue': False, 'include_carryover': False})
            self.assertEqual([x['task']['id'] for x in without_carryover['items']], expected[2:])
            with_undated = adapter.read('list', query={**query, 'include_unscheduled': True})
            self.assertEqual(len(with_undated['items']), 6)

    def test_real_lease_expiry_and_late_draft_reconciliation(self):
        cli, mcp, http = self.adapters
        fake = FakeConnectors()
        task = self.create(cli)
        claim = cli.mutate('claim', {'seconds': 30}, task)
        first = cli.task(task['id'])
        output = fake.draft(task['id'], 'kevin@example.invalid', fake.project())
        late = report(first, 'completion_proposed', outputs=[output])
        expiry = instant(first['claim']['expires_at'])
        delay = max(0, (expiry - dt.datetime.now(dt.timezone.utc)).total_seconds()) + 0.1
        print(f'  Waiting {delay:.1f}s for an actual 30-second lease to expire', flush=True)
        time.sleep(delay)
        second = mcp.mutate('claim', {}, http.task(task['id']))
        current = http.task(task['id'])
        replacement = copy.deepcopy(current['claim'])
        self.fails('claim_conflict', lambda: cli.mutate('report', {'report': late}, current, claim['claim_token']))
        op = identity()
        observed = http.mutate('observe', {'report': late}, current, op=op)
        self.assertEqual(cli.mutate('observe', {'report': late}, current, op=op), observed)
        current = mcp.task(task['id'])
        self.assertEqual(current['claim'], replacement)
        self.assertEqual(current['status'], 'in_progress')
        self.assertNotEqual(current['claim']['attempt_id'], first['claim']['attempt_id'])
        self.assertEqual(fake.draft(task['id'], 'kevin@example.invalid', fake.project())['reference'], output['reference'])
        fresh = report(current, 'completion_proposed', outputs=[output])
        fresh['reconciles'] = [observed['event_id']]
        mcp.mutate('report', {'report': fresh}, current, second['claim_token'])
        packet = http.read('context', task['id'])
        self.assertEqual(packet['task']['status'], 'seems_completed')
        self.assertEqual(packet['unreconciled_count'], 0)
        self.assertEqual(fake.creates, 1)
        self.assertEqual(fake.sends, 0)


if __name__ == '__main__':
    if len(sys.argv) == 3 and sys.argv[1] == '--seed-mcp':
        client = MCP([BINARY, '--data-dir', sys.argv[2]])
        try:
            data = definition()
            data['description'] = '<img src=x onerror="window.movementInjected=true">'
            data['sources'] = [{'provider': 'fake', 'reference': 'javascript:alert(1)',
                                'captured_at': now(), 'summary': '<script>window.movementInjected=true</script>'}]
            print(json.dumps(client.tool('task_create', {'operation_id': identity(), 'actor': ACTOR, 'data': data})))
        finally:
            client.close()
    else:
        unittest.main(verbosity=2)
