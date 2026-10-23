#!/usr/bin/env python3
"""Opt-in actual assistant-client acceptance with synthetic local task data.

Requires an installed, authenticated client and network access to its model provider.
Uses existing client authentication, temporary MCP config/store, and no real connectors.
Usage: python3 tests/acceptance/clients.py --client codex --binary bin/movement
"""
import argparse
from collections import Counter
import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile

TOOLS = {'task_create','task_update','task_claim','task_renew','task_release','task_report',
         'task_check_report','task_observe','task_get','task_context','task_history','task_list'}
EXPECTED = Counter({'task_create':2,'task_get':1,'task_update':1,'task_history':1})
PROMPT = '''Run this synthetic Movement MCP compatibility test using ONLY movement MCP tools.
Do not delegate, spawn agents, use other tools, access files, run shell commands or contact external sources.
1. Call task_create with operation_id="actual-client-create", actor="CLIENT-client-test",
   data={"title":"Synthetic client acceptance","status":"ready"}.
2. Repeat EXACTLY the same task_create input to verify the identical receipt.
3. Call task_get using the returned task_id.
4. Call task_update for that task_id with operation_id="actual-client-conflict", actor="CLIENT-client-test",
   expected_revision=999, data={"fields":{"title":"Must not be applied"}}.
   Expect a structured revision_conflict. Do not retry with another revision.
5. Call task_history. Expect exactly one task_created event.
Return a short summary of the observed receipts and error code. Only perform these five tool calls.'''


def execute(args, cwd, timeout):
    process = subprocess.Popen(args, cwd=cwd, stdin=subprocess.DEVNULL, text=True,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
    try:
        stdout, stderr = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.communicate(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.communicate()
        raise AssertionError('Actual client exceeded the test timeout; no compatibility pass recorded')
    assert process.returncode == 0, f'Client exit {process.returncode}: {stderr[-2000:]}'
    return stdout


def extract_claude(events):
    initialized = [e for e in events if e.get('type')=='system' and e.get('subtype')=='init']
    assert len(initialized)==1
    init = initialized[0]
    assert init['mcp_servers']==[{'name':'movement','status':'connected','source':'dynamic'}], init['mcp_servers']
    assert set(init['tools']) == {'mcp__movement__'+name for name in TOOLS}, init['tools']
    calls, results = {}, {}
    for event in events:
        if event.get('type') not in ['assistant','user']:
            continue
        for content in event.get('message',{}).get('content',[]):
            if content.get('type')=='tool_use':
                assert content['name'].startswith('mcp__movement__'), content['name']
                calls[content['id']] = content['name'].removeprefix('mcp__movement__')
            elif content.get('type')=='tool_result':
                value = content['content']
                if isinstance(value,list):
                    value = ''.join(block.get('text','') for block in value)
                results[content['tool_use_id']] = json.loads(value)
    assert set(calls)==set(results), 'Missing client tool result'
    return [(calls[key],value) for key,value in results.items()]


def extract_codex(events):
    results=[]
    assert any(e.get('type')=='turn.completed' for e in events), 'Client turn did not complete'
    for event in events:
        if event.get('type')!='item.completed':
            continue
        item=event['item']
        if item['type']=='agent_message':
            continue
        assert item['type']=='mcp_tool_call' and item['server']=='movement', item['type']
        assert item['error'] is None, item['error']
        structured=item['result']['structured_content']
        text=json.loads(''.join(c['text'] for c in item['result']['content'] if c['type']=='text'))
        assert structured==text, 'Structured and text MCP responses differ'
        if item['tool']=='task_update':
            assert item['status']=='failed', 'Expected a tool error for stale revision'
        else:
            assert item['status']=='completed'
        results.append((item['tool'],structured))
    return results


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--client',choices=['claude','codex'],required=True)
    parser.add_argument('--binary',type=Path,default=Path('bin/movement'))
    parser.add_argument('--timeout',type=int,default=180)
    options=parser.parse_args()
    binary=options.binary.resolve()
    with tempfile.TemporaryDirectory(prefix='movement-actual-client-') as temporary:
        root=Path(temporary); store=root/'data'
        cli=[str(binary),'--data-dir',str(store)]
        subprocess.run(cli+['init'],check=True,stdout=subprocess.DEVNULL)
        version=subprocess.check_output([options.client,'--version'],text=True,stderr=subprocess.DEVNULL).strip()
        prompt=PROMPT.replace('CLIENT',options.client)
        if options.client=='claude':
            config=root/'mcp.json'
            config.write_text(json.dumps({'mcpServers':{'movement':{'command':str(binary),'args':['--data-dir',str(store),'mcp','--stdio']}}}))
            args=['claude','--restricted','--setting-sources','','--settings','{"disableAllHooks":true}',
                  '--disable-slash-commands','--strict-mcp-config','--mcp-config',str(config),'--tools','',
                  '--allowedTools','mcp__movement__*','--permission-mode','dontAsk','--no-chrome',
                  '--no-session-persistence','--output-format','stream-json','--verbose','-p',prompt]
        else:
            args=['codex','exec','--ignore-user-config','--ephemeral','--skip-git-repo-check','--sandbox','read-only','--json']
            for feature in ['apps','plugins','hooks','shell_tool','computer_use','browser_use','image_generation','multi_agent','memories','skill_search']:
                args+=['--disable',feature]
            args+=['-c','mcp_servers.movement.command='+json.dumps(str(binary)),
                   '-c','mcp_servers.movement.args='+json.dumps(['--data-dir',str(store),'mcp','--stdio']),
                   '-c','mcp_servers.movement.required=true',
                   '-c','mcp_servers.movement.default_tools_approval_mode="approve"',prompt]
        print('Testing '+version+' with synthetic data and temporary MCP configuration',flush=True)
        output=execute(args,root,options.timeout)
        events=[json.loads(line) for line in output.splitlines() if line.strip()]
        results=extract_claude(events) if options.client=='claude' else extract_codex(events)
        assert Counter(name for name,_ in results)==EXPECTED, 'Unexpected tool-call inventory'
        receipts=[value['result'] for name,value in results if name=='task_create']
        assert receipts[0]==receipts[1] and receipts[0]['revision']==1
        failure=next(value['error'] for name,value in results if name=='task_update')
        assert failure['code']=='revision_conflict' and failure['revision']==1
        task=next(value['result'] for name,value in results if name=='task_get')
        history=next(value['result'] for name,value in results if name=='task_history')
        assert task['id']==receipts[0]['task_id'] and task['title']=='Synthetic client acceptance' and task['revision']==1
        assert len(history)==1 and history[0]['type']=='task_created' and history[0]['event_id']==receipts[0]['event_id']
        readback=json.loads(subprocess.check_output(cli+['task','list','--json'],text=True))
        assert len(readback['items'])==1 and readback['items'][0]['task']==task
        persisted=json.loads(subprocess.check_output(cli+['task','history',task['id'],'--json'],text=True))
        assert persisted==history
        print('PASS '+version+': actual MCP calls, identical create retry, structured revision conflict, one unchanged task/event and independent CLI readback',flush=True)


if __name__=='__main__':
    main()
