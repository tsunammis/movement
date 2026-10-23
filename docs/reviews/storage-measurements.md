# Local storage measurements

Measured on macOS 26.7.1, arm64, using the packaged `bin/movement`. Raw observations are in [storage-measurements.json](storage-measurements.json). Reproduce with:

```sh
python3 tests/acceptance/storage_measure.py --binary bin/movement
```

The harness creates all synthetic records through independent CLI processes in temporary local directories. Each task starts with four events. Each size has five samples per operation; each contention round starts three CLI readers and one CLI writer using a thread pool. This measures end-to-end subprocess latency, including launch, locking, recovery checks, scanning and durable writes. Threads do not synchronize at the lock, so the results are not a controlled worst-case lock benchmark.

| Initial tasks / events | List median | Get median | Context median | Update median | Contended read median / max | Contended update median / max |
| --- | --- | --- | --- | --- | --- | --- |
| 10 / 40 | 51 ms | 53 ms | 56 ms | 92 ms | 130 / 293 ms | 217 / 262 ms |
| 50 / 200 | 193 ms | 167 ms | 168 ms | 216 ms | 539 / 796 ms | 551 / 733 ms |

Final verification found 10 tasks / 50 events / 50 receipts and 50 tasks / 210 events / 210 receipts, respectively. All attempted operations succeeded. Temporary stores were removed by the harness.

These small-store observations establish a baseline, not a supported maximum or latency guarantee. Latency grows with store size even for a single-task read; repository-wide validation and scan work remains material. Larger task counts, long histories, sustained contention and slower disks remain unmeasured. Do not extrapolate a capacity envelope from these samples. Preserve correctness and recovery checks when considering future optimizations.
