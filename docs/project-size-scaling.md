# Project size scaling proposal

Status: proposal only. The current 64 MiB encoded-project and backup limit is
unchanged.

## Candidate

Consider a single higher encoded-project limit, initially **256 MiB**, only
after benchmark gates are approved. The request limit should remain separate:
64 KiB for an individual engine JSON request is a protocol and resource bound,
not evidence that the Project limit should stay at 64 MiB.

Do not use a Yap count as a capacity limit. Decoded record size, metadata,
object structure, and encrypted encoding affect file size differently.

## Requirements before implementation

1. Define a target workload, including representative Yap size, Lobby count,
   query patterns, update frequency, and maximum expected file size.
2. Set a latency SLO for open, query, update, export, and import on a
   maintained reference host. Include median and p95/p99 targets.
3. Set peak memory and allocation budgets for both the engine and SDK
   processes. Tests should cover at least 64, 128, 256, and 512 MiB of fixture
   data without treating benchmark duration alone as an approval signal.
4. Decide whether 256 MiB is an upper bound or an intermediate tier leading
   to another limit. Preserve an explicit constant documented across the
   engine, Node SDK, Python SDK, backup APIs, CLI, and docs.
5. Confirm that exporter/importer behavior remains bounded and rejects
   oversized files before replacing any existing Project or backup.

## Benchmark matrix

Run controlled benchmarks at:

- 20,000 small Yaps, the existing near-capacity baseline.
- 50,000 small Yaps.
- The largest representative workload at the candidate 256 MiB boundary.
- Mixed small, medium, and large Yap payloads.
- Cold and warm sequential operations.
- Concurrent writers, including lock timeout and crash recovery cases.

Each result should report exact encoded size, runtime distribution, peak
process memory, heap allocations, host CPU/memory/storage, and SDK process
startup overhead. Compare Node and Python against the engine-only baseline.

## Implementation constraints

- Reads and writes currently decrypt or re-encrypt the whole Project. Raising
  the limit increases worst-case memory and latency even when individual
  records are small.
- A 64 KiB request cap limits operation payloads independently of file size.
  Large state changes need chunked or batched protocol support, not a larger
  request silently accepted without protocol and compatibility tests.
- Export/restore must retain secure temporary-file behavior and never
  overwrite an existing destination.
- Compatibility changes must update docs, conformance fixtures, SDK constants,
  CLI errors, and release notes in one change set.

## Decision gates

Approve an implementation only if the proposed workload and SLO are met,
resource bounds are documented, and cross-SDK behavior is identical. If those
conditions are not met, keep 64 MiB as an intentional V1 boundary and revisit
only with measured representative workloads.
