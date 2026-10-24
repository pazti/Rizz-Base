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
index cannot avoid the current full-file decryption cost. Before choosing a
Phase C architecture, profile the timed query path separately and collect
representative application workload and latency targets. Repeated SDK calls
may benefit from a persistent process, but that tradeoff also changes the
process and secret-handling boundary.

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
