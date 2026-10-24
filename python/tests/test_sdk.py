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
    Rizzbase,
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
        with self.assertRaises(RizzbaseError) as caught:
            _validate_query({"cursor": "x" * 4097})
        self.assertEqual(caught.exception.code, ErrorCode.INVALID_ARGUMENT)

    def test_bulk_yap_input_validation_precedes_engine_spawn(self):
        database = Rizzbase("bulk.rizz", "passphrase", binary_path="missing-engine")
        for yaps in (
            [],
            [{"id": "a", "data": True}, {"id": "a", "data": False}],
            [{"id": "bad" * 50, "data": True}],
            [{"id": "a", "data": float("nan")}],
        ):
            with self.subTest(yaps=yaps):
                with self.assertRaises(RizzbaseError) as caught:
                    database.create_yaps("lobby", yaps)
                self.assertEqual(caught.exception.code, ErrorCode.INVALID_ARGUMENT)


class SubprocessOutputTests(unittest.TestCase):
    @staticmethod
    def _fake_engine(directory, filename, counter, *, error_code=None, error_context=None):
        executable = pathlib.Path(directory) / filename
        error_response = (
            f'print(json.dumps({{"ok": False, "error": "No", "errorCode": {error_code!r}, "errorContext": {error_context!r}}}))'
            if error_code
            else 'print(json.dumps({"ok": True, "project": {"version": 1, "name": "retried"}}))'
        )
        source = f"""#!{sys.executable}
import json
import pathlib
import sys
import time

counter = pathlib.Path({str(counter)!r})
if "--version" in sys.argv:
    print(json.dumps({{"version": "1.1.0", "projectFormatVersion": 1}}))
else:
    request = json.loads(sys.stdin.read())
    count = int(counter.read_text() if counter.exists() else "0") + 1
    counter.write_text(str(count))
    if count == 1 and {error_code is None}:
        time.sleep(1.5)
    {error_response}
"""
        executable.write_text(source)
        executable.chmod(0o755)
        return executable

    def test_only_safe_reads_are_retried_after_engine_timeout(self):
        with tempfile.TemporaryDirectory() as directory:
            read_counter = pathlib.Path(directory) / "read-count"
            read_binary = self._fake_engine(directory, "read-engine", read_counter)
            retry = {
                "max_attempts": 2,
                "base_delay_ms": 0,
                "max_delay_ms": 0,
                "total_budget_ms": 250,
            }
            reader = Rizzbase(
                str(pathlib.Path(directory) / "read.rizz"),
                "passphrase",
                binary_path=str(read_binary),
                timeout_s=0.5,
                retry=retry,
            )
            self.assertEqual(reader.open()["name"], "retried")
            self.assertEqual(read_counter.read_text(), "2")

            write_counter = pathlib.Path(directory) / "write-count"
            write_binary = self._fake_engine(directory, "write-engine", write_counter)
            writer = Rizzbase(
                str(pathlib.Path(directory) / "write.rizz"),
                "passphrase",
                binary_path=str(write_binary),
                timeout_s=0.5,
                retry=retry,
            )
            with self.assertRaises(RizzbaseError) as caught:
                writer.create_lobby("no replay")
            self.assertEqual(caught.exception.code, ErrorCode.ENGINE_TIMEOUT)
            self.assertEqual(write_counter.read_text(), "1")

            auth_counter = pathlib.Path(directory) / "auth-count"
            auth_binary = self._fake_engine(directory, "auth-engine", auth_counter, error_code=ErrorCode.AUTHENTICATION_FAILED)
            auth_reader = Rizzbase(
                str(pathlib.Path(directory) / "auth.rizz"),
                "passphrase",
                binary_path=str(auth_binary),
                timeout_s=0.5,
                retry=retry,
            )
            with self.assertRaises(RizzbaseError) as auth_error:
                auth_reader.open()
            self.assertEqual(auth_error.exception.code, ErrorCode.AUTHENTICATION_FAILED)
            self.assertEqual(auth_counter.read_text(), "1")

            busy_context = {
                "present": True,
                "state": "active",
                "ageMs": 25,
                "metadata": {"pid": 123, "hostname": "local", "startedAt": "2026-10-24T00:00:00Z"},
            }
            busy_counter = pathlib.Path(directory) / "busy-count"
            busy_binary = self._fake_engine(
                directory,
                "busy-engine",
                busy_counter,
                error_code=ErrorCode.PROJECT_BUSY,
                error_context=busy_context,
            )
            busy_reader = Rizzbase(
                str(pathlib.Path(directory) / "busy.rizz"),
                "passphrase",
                binary_path=str(busy_binary),
                timeout_s=0.5,
                retry=retry,
            )
            with self.assertRaises(RizzbaseError) as busy_error:
                busy_reader.open()
            self.assertEqual(busy_error.exception.code, ErrorCode.PROJECT_BUSY)
            self.assertEqual(busy_error.exception.context, busy_context)
            self.assertEqual(busy_counter.read_text(), "1")

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

    def test_retry_policy_validation(self):
        for retry in (
            {"max_attempts": 0},
            {"base_delay_ms": -1},
            {"max_delay_ms": 1, "base_delay_ms": 2},
            {"unknown": 1},
        ):
            with self.subTest(retry=retry):
                with self.assertRaises(RizzbaseError) as caught:
                    Rizzbase("test.rizz", "passphrase", retry=retry)
                self.assertEqual(caught.exception.code, ErrorCode.INVALID_ARGUMENT)
        Rizzbase("test.rizz", "passphrase", timeout_s=0.0005)


if __name__ == "__main__":
    unittest.main()
