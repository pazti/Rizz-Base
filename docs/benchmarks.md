# Engine performance benchmarks

The engine reads and rewrites a complete encrypted Project for each operation.
Query currently decrypts the full file, scans the Lobby, and sorts matching
Yaps when ordering is requested. The benchmarks below measure those costs as
the Lobby grows.

## Reference measurements

Run with Go 1.27.1 on Linux/amd64, 2 vCPUs (Intel Xeon Platinum 8573C).
Results through 5,000 Yaps are the mean of 10 iterations per case; 10,000 and
20,000-Yap results use 3 iterations. These are reference points, not an SLA.
Each Yap contains a roughly 128-byte string payload plus `group` and `score`
fields. The project file size is the encrypted on-disk representation. Query
filters all Yaps, sorts by `score` descending, then returns at most 100. Update
replaces one existing Yap with a similarly sized value.

| Yaps in Lobby | Encrypted file | Open | Ordered query page | Update one Yap |
| ---: | ---: | ---: | ---: | ---: |
| 100 | 219,670 B | 121 ms | 115 ms | 231 ms |
| 1,000 | 2,175,386 B | 135 ms | 137 ms | 282 ms |
| 5,000 | 10,915,165 B | 208 ms | 228 ms | 464 ms |
| 10,000 | 21,842,235 B | 289 ms | 329 ms | 690 ms |
| 20,000 | 43,805,820 B | 446 ms | 570 ms | 1,175 ms |

Allocations per operation at those sizes were approximately
1.4/13.7/67.7/138.4/284.3 MiB for open,
1.6/16.1/79.9/163.2/334.3 MiB for query, and
2.5/24.9/124.5/252.2/512.1 MiB for update. These are Go heap allocations,
not retained memory.

As a planning guide on a comparable host, keep latency-sensitive Lobbies near
or below 1,000 small Yaps: all measured operations were below 300 ms and
roughly 25 MiB allocated per update at that size. 5,000 Yaps is a tested
larger-workload tier, not a default comfort target: its single-Yap update
averaged 464 ms and allocated about 125 MiB. Larger records, slower storage,
and concurrent workloads can reach those costs sooner.

At 20,000 small Yaps, the project is about 44 MB on disk and a single-Yap
update averages 1.18 s with roughly 512 MiB of cumulative allocations. This
supports treating larger single-Lobby workloads as a performance concern,
even before reaching the 64 MiB file cap. It does not by itself select a
daemon or index design: query still decrypts the whole-file envelope, so an
index cannot avoid the current full-file decryption cost.

This is not a fixed Yap-count limit. The hard encoded project limit is 64 MiB,
but record sizes and metadata vary, so the file-size limit may be reached at
very different Yap counts. Benchmark representative data on the target host
before choosing a production capacity. SDK calls also include process startup
and JSON protocol overhead, which these core API benchmarks do not measure.

## Reproduce

From the repository root:

```sh
cd engine
go test ./core -run '^$' \
  -bench 'Benchmark(OpenProject|QueryYapPage|UpdateYap)$' \
  -benchtime=10x -benchmem
```

The fixtures are created outside the timed section. They use bulk creation to
seed 100, 1,000, 5,000, 10,000, and 20,000 Yaps, then separately time project
open, filtered and ordered cursor-page query, and replacement of one Yap. Use
`-benchtime=10x` for routine comparisons; the larger reference points used
`-benchtime=3x` to keep the run bounded. The benchmark source is
[benchmark_test.go](../engine/core/benchmark_test.go).

## SDK end-to-end benchmarks

The SDK runners in [`benchmarks/`](../benchmarks/) measure one `open`, one
filtered and ordered query page, and one single-Yap update through the
production Node and Python APIs. Each timed call includes SDK request
preparation, spawning a fresh engine process, stdin/stdout protocol handling,
whole-file decryption, and (for update) encryption and durable file publish.
The one-time engine compatibility check and project setup/import are outside
the timed calls. These measurements are the relevant basis for deciding
whether keeping a process alive could help; compare them with the engine-only
results above to isolate language/process overhead.

From the repository root, build the SDK and compatible engine, install the
Python package into the environment, then run both runners against the same
engine version and host:

```sh
cd sdk
npm ci
npm run build
cd ../engine
go build -ldflags="-X main.version=1.2.0" -o /tmp/rizzbase ./cmd/rizzbase
cd ..
python -m pip install ./python
export RIZZBASE_BINARY=/tmp/rizzbase
node benchmarks/node-sdk.mjs > /tmp/node-sdk-bench.json
python benchmarks/python-sdk.py > /tmp/python-sdk-bench.json
```

By default they seed 100, 1,000, 5,000, 10,000, and 20,000 small Yaps via an
SDK backup import outside the timed section and take five samples per
operation. Set `RIZZBASE_BENCH_COUNTS=100,1000` and
`RIZZBASE_BENCH_REPETITIONS=3` to run a shorter sample. Each runner emits JSON
with runtime, platform, exact encrypted file size, per-sample latency, mean,
and median. Do not compare runs across different hosts, engine builds, runtime
versions, or storage conditions as if they were controlled results.

The SDK subprocess boundary adds some fixed overhead, but most large-project
latency is expected to remain in PBKDF2, whole-file decoding, and JSON parsing.
A daemon would reduce process startup while retaining those cryptographic
costs; an index would still require decrypting the current whole-file envelope.
The measured end-to-end curves below 20,000 Yaps do not justify introducing
either architectural change without a concrete application workload,
acceptable latency target, and a design that addresses whole-file
decryption.

### SDK reference measurements

Measured sequentially on Linux/amd64 with 2 vCPUs (Intel Xeon Platinum 8573C),
Go engine 1.2.0, Node.js 24.21.0, Python 3.14.2, and three timed calls per
operation. Each value is the median in milliseconds. The project was imported
from a deterministic JSON fixture outside the timed section; encryption salt
and nonce make the exact file size vary slightly between runs. Each timed
operation starts a new engine process.

| Yaps | Approx. file size | Node open | Node query page | Node update | Python open | Python query page | Python update |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 100 | 0.22 MB | 137 | 127 | 242 | 165 | 165 | 265 |
| 1,000 | 2.18 MB | 148 | 150 | 283 | 166 | 165 | 317 |
| 5,000 | 10.91 MB | 258 | 313 | 538 | 275 | 267 | 522 |
| 10,000 | 21.84 MB | 343 | 372 | 774 | 377 | 417 | 767 |
| 20,000 | 43.80 MB | 572 | 696 | 1,396 | 591 | 767 | 1,320 |

Three samples are enough to expose the growth curve but not to characterize
tail latency; one 5,000-Yap Node query sample was substantially slower than
the other two. Treat the table as an indicative local baseline, not a
cross-platform guarantee. Python and Node results are close at larger sizes,
and the SDK-level timings are generally within a few hundred milliseconds of
the direct core measurements above. That does not establish process startup
as the dominant cost. A persistent daemon is not justified by these results
alone; a larger sample, target application SLO, and explicit security/process
model are needed first.

Run both SDK measurements sequentially on an otherwise idle machine to avoid
CPU contention:

```sh
RIZZBASE_BINARY=/tmp/rizzbase RIZZBASE_BENCH_REPETITIONS=3 \
  node benchmarks/node-sdk.mjs > /tmp/node-sdk-bench.json
RIZZBASE_BINARY=/tmp/rizzbase RIZZBASE_BENCH_REPETITIONS=3 \
  PYTHONPATH=python/src python3 benchmarks/python-sdk.py > /tmp/python-sdk-bench.json
```

The SDK runner source is
[node-sdk.mjs](../benchmarks/node-sdk.mjs) and
[python-sdk.py](../benchmarks/python-sdk.py).
