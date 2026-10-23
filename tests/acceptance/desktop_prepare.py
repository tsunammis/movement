#!/usr/bin/env python3
"""Prepare an isolated Claude Desktop MCP acceptance case; never install client settings.

Run: python3 tests/acceptance/desktop_prepare.py --binary bin/movement
The output directory contains only synthetic data, a config fragment and a prompt.
"""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile
from clients import PROMPT


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, default=Path('bin/movement'))
    options = parser.parse_args()
    binary = options.binary.resolve(strict=True)
    root = Path(tempfile.mkdtemp(prefix='movement-desktop-acceptance-'))
    store = root / 'data'
    subprocess.run([str(binary), '--data-dir', str(store), 'init', '--timezone', 'Europe/Paris'],
                   check=True, capture_output=True)
    config = {'mcpServers': {'movement-desktop-acceptance': {
        'command': str(binary), 'args': ['--data-dir', str(store), 'mcp', '--stdio']}}}
    (root / 'claude-config-fragment.json').write_text(json.dumps(config, indent=2) + '\n')
    prompt = PROMPT.replace('CLIENT', 'claude-desktop').replace('ONLY movement MCP tools',
                                                              'ONLY movement-desktop-acceptance MCP tools')
    (root / 'prompt.txt').write_text(prompt + '\n')
    (root / 'README.txt').write_text(
        'Prepared only; no client settings were changed.\n'
        'Merge this single MCP entry into the existing client configuration, preserving all other entries.\n'
        'Use a fresh chat and only the synthetic prompt. Inspect actual tool results.\n'
        'Pass requires two identical create receipts, one revision_conflict, and one creation history event.\n'
        'Read the resulting task and history independently with Movement CLI; do not trust model prose alone.\n'
        'After testing, remove only movement-desktop-acceptance from the client configuration.\n'
        'Do not delete an existing client configuration or interrupt other active client work.\n')
    verified = subprocess.check_output([str(binary), '--data-dir', str(store), 'verify'], text=True)
    print(json.dumps({'directory': str(root), 'store': str(store),
                      'config_fragment': str(root / 'claude-config-fragment.json'),
                      'prompt': str(root / 'prompt.txt'), 'verification': json.loads(verified)}, indent=2))


if __name__ == '__main__':
    main()
