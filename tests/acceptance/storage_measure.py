#!/usr/bin/env python3
"""Measure real CLI processes against synthetic stores; no performance guarantee.

All records are created through Movement. Output is JSON; progress goes to stderr.
"""
import argparse
import concurrent.futures
import json
from pathlib import Path
import platform
import statistics
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, default=Path('bin/movement'))
    parser.add_argument('--sizes', default='10,50')
    parser.add_argument('--events-per-task', type=int, default=4)
    parser.add_argument('--samples', type=int, default=5)
    args = parser.parse_args()
    sizes = [int(value) for value in args.sizes.split(',')]
    if any(n < 1 for n in sizes) or args.events_per_task < 1 or args.samples < 1:
        parser.error('sizes, events-per-task and samples must be positive')
    binary = args.binary.resolve(strict=True)
    results = []
    for size in sizes:
        with tempfile.TemporaryDirectory(prefix='movement-measure-') as temporary:
            root = Path(temporary)
            command = [str(binary), '--data-dir', str(root)]

            def run(arguments, data=None):
                start = time.perf_counter()
                completed = subprocess.run(command + arguments, input=None if data is None else json.dumps(data),
                                           text=True, capture_output=True, timeout=60)
                if completed.returncode:
                    raise RuntimeError(completed.stderr)
                return (time.perf_counter() - start) * 1000, json.loads(completed.stdout)

            run(['init'])
            seeded = time.perf_counter()
            task_id = None
            for index in range(size):
                _, receipt = run(['task', 'create', '--op', f'create-{index}', '--actor', 'measurement', '--input', '-'],
                                 {'title': f'Synthetic task {index}', 'status': 'ready'})
                task_id = receipt['task_id']
                for revision in range(1, args.events_per_task):
                    run(['task', 'update', task_id, '--revision', str(revision), '--op', f'edit-{index}-{revision}',
                         '--actor', 'measurement', '--input', '-'], {'fields': {'description': f'Synthetic revision {revision}'}})
            seed_seconds = time.perf_counter() - seeded
            print(f'Seeded {size} tasks / {size * args.events_per_task} events', file=__import__('sys').stderr, flush=True)
            samples = {name: [] for name in ['list_ms', 'get_ms', 'context_ms', 'update_ms', 'contended_read_ms', 'contended_update_ms']}
            revision = args.events_per_task
            for sample in range(args.samples):
                for name, command_args in [('list_ms', ['task', 'list', '--json']),
                                           ('get_ms', ['task', 'get', task_id, '--json']),
                                           ('context_ms', ['task', 'context', task_id, '--json'])]:
                    elapsed, _ = run(command_args)
                    samples[name].append(elapsed)
                # Use an editable field; each operation is a genuine durable commit.
                def write(op):
                    return run(['task', 'update', task_id, '--revision', str(revision), '--op', op,
                                '--actor', 'measurement', '--input', '-'], {'fields': {'description': op}})
                elapsed, receipt = write(f'measured-{sample}')
                revision = receipt['revision']
                samples['update_ms'].append(elapsed)
                with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                    readers = [pool.submit(run, ['task', 'list', '--json']) for _ in range(3)]
                    writer = pool.submit(write, f'contended-{sample}')
                    for future in readers:
                        elapsed, _ = future.result()
                        samples['contended_read_ms'].append(elapsed)
                    elapsed, receipt = writer.result()
                    revision = receipt['revision']
                    samples['contended_update_ms'].append(elapsed)
            _, verified = run(['verify'])
            assert verified['tasks'] == size
            assert verified['events'] == size * args.events_per_task + 2 * args.samples
            results.append({'tasks': size, 'initial_events': size * args.events_per_task,
                            'verified': verified, 'seed_seconds': round(seed_seconds, 3),
                            'store_bytes': sum(p.stat().st_size for p in root.rglob('*') if p.is_file()),
                            'timings': {name: {'samples': len(values), 'min_ms': round(min(values), 3),
                                             'median_ms': round(statistics.median(values), 3),
                                             'max_ms': round(max(values), 3)} for name, values in samples.items()}})
    print(json.dumps({'environment': {'system': platform.system(), 'macos': platform.mac_ver()[0],
                                      'machine': platform.machine()}, 'results': results}, indent=2))


if __name__ == '__main__':
    main()
