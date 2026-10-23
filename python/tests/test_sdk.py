import json
import os
import pathlib
import sys
import tempfile
import unittest
from unittest.mock import patch

from rizzbase.errors import ErrorCode, RizzbaseError
from rizzbase.sdk import (
    _OutputLimitExceeded,
    _parse_version,
    _resolve_binary,
    _run_bounded,
    _validate_json,
    _validate_query,
)


class BinaryResolutionTests(unittest.TestCase):
    def test_explicit_binary_has_priority(self):
        with tempfile.TemporaryDirectory() as directory:
            executable = pathlib.Path(directory) / "explicit"
            executable.write_text("binary")
            executable.chmod(0o755)
            with patch.dict(os.environ, {"RIZZBASE_BINARY": str(pathlib.Path(directory) / "missing"), "PATH": ""}):
                self.assertEqual(_resolve_binary(str(executable)), str(executable.resolve()))

    def test_environment_binary_has_priority_over_path(self):
        with tempfile.TemporaryDirectory() as directory:
            configured = pathlib.Path(directory) / "configured"
            configured.write_text("binary")
            configured.chmod(0o755)
            with patch.dict(os.environ, {"RIZZBASE_BINARY": str(configured), "PATH": ""}):
                self.assertEqual(_resolve_binary(None), str(configured.resolve()))

    def test_path_resolution_and_missing_binary_guidance(self):
        with tempfile.TemporaryDirectory() as directory:
            executable = pathlib.Path(directory) / ("rizzbase.exe" if os.name == "nt" else "rizzbase")
            executable.write_text("binary")
            executable.chmod(0o755)
            with patch.dict(os.environ, {"PATH": directory}, clear=False):
                self.assertEqual(_resolve_binary(None), str(executable.resolve()))
            with patch.dict(os.environ, {"PATH": directory}, clear=False):
                executable.unlink()
                with self.assertRaises(RizzbaseError) as caught:
                    _resolve_binary(None)
            self.assertEqual(caught.exception.code, ErrorCode.BINARY_UNAVAILABLE)
            self.assertIn(directory, str(caught.exception))
            self.assertIn("RIZZBASE_BINARY", str(caught.exception))


class ValidationTests(unittest.TestCase):
    def test_engine_compatibility(self):
        for version in ("1.1.0", "1.9.99"):
            _parse_version(json.dumps({"version": version, "projectFormatVersion": 1}))
        for version, format_version in (
            ("1.0.9", 1),
            ("2.0.0", 1),
            ("dev", 1),
            ("01.1.0", 1),
            ("1.1.0", 2),
            ("1.1.0", True),
        ):
            with self.subTest(version=version, format_version=format_version):
                with self.assertRaises(RizzbaseError) as caught:
                    _parse_version(json.dumps({"version": version, "projectFormatVersion": format_version}))
                self.assertEqual(caught.exception.code, ErrorCode.ENGINE_INCOMPATIBLE)

    def test_json_validation_rejects_non_json_and_cycles(self):
        for value in (float("nan"), float("inf"), {1: "non-string key"}, object()):
            with self.subTest(value=value):
                with self.assertRaises(RizzbaseError) as caught:
                    _validate_json(value)
                self.assertEqual(caught.exception.code, ErrorCode.INVALID_ARGUMENT)
        cycle = []
        cycle.append(cycle)
        with self.assertRaises(RizzbaseError) as caught:
            _validate_json(cycle)
        self.assertEqual(caught.exception.code, ErrorCode.INVALID_ARGUMENT)

    def test_query_validation(self):
        _validate_query({
            "where": [{"field": "user.active", "operator": "==", "value": True}],
            "orderBy": {"field": "score", "direction": "desc"},
            "limit": 2,
        })
        for options in (
            {"limit": True},
            {"limit": None},
            {"limit": -1},
            {"where": [{"field": "x..y", "operator": "==", "value": 1}]},
            {"where": [{"field": "x", "operator": "contains", "value": 1}]},
            {"orderBy": None},
            {"orderBy": {"field": "score", "direction": "sideways"}},
        ):
            with self.subTest(options=options):
                with self.assertRaises(RizzbaseError) as caught:
                    _validate_query(options)
                self.assertEqual(caught.exception.code, ErrorCode.INVALID_ARGUMENT)


class SubprocessOutputTests(unittest.TestCase):
    def test_output_limit_terminates_oversized_process(self):
        with self.assertRaises(_OutputLimitExceeded):
            _run_bounded(
                [
                    sys.executable,
                    "-c",
                    "import sys\n"
                    "while True:\n"
                    "    sys.stdout.buffer.write(b'x' * 8192)\n"
                    "    sys.stdout.flush()\n",
                ],
                input_data=None,
                timeout_s=5,
                max_output=1024,
            )


if __name__ == "__main__":
    unittest.main()
