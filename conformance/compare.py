#!/usr/bin/env python3
import difflib
import json
import pathlib
import sys


def canonical(value):
    if isinstance(value, dict):
        return {key: canonical(value[key]) for key in sorted(value)}
    if isinstance(value, list):
        return [canonical(item) for item in value]
    return value


def main():
    if len(sys.argv) != 4:
        raise SystemExit("usage: compare.py NODE.json PYTHON.json GO.json")
    paths = [pathlib.Path(name) for name in sys.argv[1:]]
    results = [json.loads(path.read_text(encoding="utf-8")) for path in paths]
    baseline = canonical(results[0])
    for path, result in zip(paths[1:], results[1:]):
        candidate = canonical(result)
        if candidate != baseline:
            expected = json.dumps(baseline, indent=2, sort_keys=True).splitlines()
            actual = json.dumps(candidate, indent=2, sort_keys=True).splitlines()
            diff = "\n".join(difflib.unified_diff(
                expected, actual, fromfile=str(paths[0]), tofile=str(path), lineterm=""
            ))
            raise SystemExit(f"Normalized conformance results differ:\n{diff}")
    print("Node, Python, and Go conformance results match.")


if __name__ == "__main__":
    main()
