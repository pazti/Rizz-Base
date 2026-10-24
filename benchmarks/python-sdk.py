import json
import os
import pathlib
import platform
import statistics
import tempfile
import time

from rizzbase import Rizzbase


binary_path = os.environ.get("RIZZBASE_BINARY")
if not binary_path:
    raise RuntimeError("Set RIZZBASE_BINARY to the engine executable.")

passphrase = "rizzbase-sdk-benchmark-only"
counts = [int(value) for value in os.environ.get(
    "RIZZBASE_BENCH_COUNTS", "100,1000,5000,10000,20000"
).split(",")]
repetitions = int(os.environ.get("RIZZBASE_BENCH_REPETITIONS", "5"))
if any(count <= 0 for count in counts) or repetitions <= 0:
    raise ValueError("Benchmark counts and repetitions must be positive integers.")


def project_backup(count):
    yaps = {}
    for index in range(count):
        yaps[f"yap-{index:08d}"] = {
            "group": f"group-{index % 20:02d}",
            "score": index,
            "payload": "x" * 128,
        }
    return {
        "version": 1,
        "name": "SDK end-to-end benchmark",
        "createdAt": "2025-01-01T00:00:00Z",
        "lobbies": {
            "benchmark-lobby": {
                "id": "benchmark-lobby",
                "name": "Benchmark lobby",
                "createdAt": "2025-01-01T00:00:00Z",
                "yaps": yaps,
            }
        },
    }


def measure(operation, action):
    samples = []
    for index in range(repetitions):
        started = time.perf_counter()
        action(index)
        samples.append((time.perf_counter() - started) * 1000)
    sorted_samples = sorted(samples)
    return {
        "operation": operation,
        "samplesMs": [round(sample, 3) for sample in sorted_samples],
        "medianMs": round(statistics.median(sorted_samples), 3),
        "meanMs": round(statistics.mean(sorted_samples), 3),
    }


results = []
for count in counts:
    with tempfile.TemporaryDirectory(prefix="rizzbase-python-bench-") as directory:
        root = pathlib.Path(directory)
        path = root / f"project-{count}.rizz"
        backup_path = root / "seed.json"
        backup_path.write_text(json.dumps(project_backup(count), separators=(",", ":")) + "\n")
        backup_path.chmod(0o600)

        db = Rizzbase(str(path), passphrase, binary_path=str(pathlib.Path(binary_path).resolve()))
        db.import_backup(str(backup_path))
        project_bytes = path.stat().st_size
        db.open()

        operations = [
            measure("open", lambda _index: db.open()),
            measure(
                "ordered_query_page",
                lambda _index: db.query_yap_page(
                    "benchmark-lobby",
                    {
                        "where": [{"field": "score", "operator": ">=", "value": 0}],
                        "orderBy": {"field": "score", "direction": "desc"},
                        "limit": 100,
                    },
                ),
            ),
            measure(
                "update_one_yap",
                lambda index: db.update_yap(
                    "benchmark-lobby",
                    "yap-00000000",
                    {"group": "group-00", "score": index, "payload": "x" * 128},
                ),
            ),
        ]
        results.append({
            "sdk": "python",
            "yaps": count,
            "projectBytes": project_bytes,
            "repetitions": repetitions,
            "operations": operations,
        })

print(json.dumps({
    "runtime": platform.python_version(),
    "platform": platform.system().lower(),
    "architecture": platform.machine(),
    "binaryPath": str(pathlib.Path(binary_path).resolve()),
    "results": results,
}, indent=2))
