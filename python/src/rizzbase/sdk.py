import json
import math
import os
import pathlib
import random
import re
import shutil
import subprocess
import tempfile
import threading
import time
from copy import deepcopy

from .errors import ErrorCode, RizzbaseError

MIN_ENGINE_VERSION = (1, 1, 0)
MAX_ENGINE_MAJOR = 2
PROJECT_FORMAT_VERSION = 1
MAX_REQUEST_SIZE = 64 << 10
MAX_RESPONSE_SIZE = 64 << 20
MAX_BACKUP_SIZE = 64 << 20

_VERSION_LOCK = threading.Lock()
_CHECKED_BINARIES: set[str] = set()
_ENGINE_ERROR_CODES = {
    ErrorCode.INVALID_ARGUMENT,
    ErrorCode.AUTHENTICATION_FAILED,
    ErrorCode.INVALID_FORMAT,
    ErrorCode.PROJECT_BUSY,
    ErrorCode.LOBBY_NOT_FOUND,
    ErrorCode.YAP_NOT_FOUND,
    ErrorCode.YAP_EXISTS,
    ErrorCode.FILE_NOT_FOUND,
    ErrorCode.FILE_EXISTS,
    ErrorCode.PERMISSION_DENIED,
    ErrorCode.INVALID_BACKUP,
    ErrorCode.ENGINE_ERROR,
}
_READ_OPERATIONS = {"open", "lobby.list", "yap.get", "yap.list", "yap.query"}
_WRITE_OPERATIONS = {"lobby.create", "lobby.delete", "yap.create", "yap.bulkCreate", "yap.update", "yap.delete"}


def _invalid(message: str):
    raise RizzbaseError(message, ErrorCode.INVALID_ARGUMENT)


def _validate_json(value, ancestors=None):
    if value is None or isinstance(value, (str, bool)):
        return
    if isinstance(value, int) and not isinstance(value, bool):
        return
    if isinstance(value, float):
        if not math.isfinite(value):
            _invalid("Yaps and filter values must contain only finite JSON numbers.")
        return
    if not isinstance(value, (list, dict)):
        _invalid("Yaps and filter values must contain only JSON values.")
    ancestors = ancestors if ancestors is not None else set()
    identity = id(value)
    if identity in ancestors:
        _invalid("Circular JSON values are not supported.")
    ancestors.add(identity)
    if isinstance(value, list):
        for item in value:
            _validate_json(item, ancestors)
    else:
        for key, item in value.items():
            if not isinstance(key, str):
                _invalid("JSON object keys must be strings.")
            _validate_json(item, ancestors)
    ancestors.remove(identity)


def _resolve_binary(binary_path):
    if binary_path is not None:
        candidate = pathlib.Path(binary_path).expanduser().resolve()
        if _is_executable(candidate):
            return str(candidate)
        raise _binary_unavailable(f"binary_path: {candidate}")

    configured = os.environ.get("RIZZBASE_BINARY")
    if configured:
        candidate = pathlib.Path(configured).expanduser().resolve()
        if _is_executable(candidate):
            return str(candidate)
        raise _binary_unavailable(f"RIZZBASE_BINARY: {candidate}")

    executable = "rizzbase.exe" if os.name == "nt" else "rizzbase"
    search_path = os.environ.get("PATH", "")
    candidates = [str((pathlib.Path(entry or ".") / executable).resolve()) for entry in search_path.split(os.pathsep)]
    for candidate in candidates:
        if _is_executable(pathlib.Path(candidate)):
            return candidate
    raise _binary_unavailable(f"PATH: {', '.join(candidates) if candidates else '(empty)'}")


def _is_executable(path):
    try:
        if not path.is_file():
            return False
        return os.name == "nt" or os.access(path, os.X_OK)
    except (OSError, ValueError):
        return False


def _binary_unavailable(searched):
    return RizzbaseError(
        f"Rizzbase engine not found or not executable. Searched {searched}. "
        "Install the Rizzbase engine and set RIZZBASE_BINARY or add it to PATH.",
        ErrorCode.BINARY_UNAVAILABLE,
    )


def _parse_version(text):
    try:
        value = json.loads(text)
    except (json.JSONDecodeError, TypeError):
        value = None
    if not isinstance(value, dict):
        raise _incompatible()
    version = value.get("version")
    if not isinstance(version, str):
        raise _incompatible()
    if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", version):
        raise _incompatible()
    parts = version.split(".")
    numbers = tuple(int(part) for part in parts)
    format_version = value.get("projectFormatVersion")
    if (
        isinstance(format_version, bool)
        or not isinstance(format_version, int)
        or format_version != PROJECT_FORMAT_VERSION
        or not (numbers >= MIN_ENGINE_VERSION and numbers[0] < MAX_ENGINE_MAJOR)
    ):
        raise _incompatible()


def _incompatible():
    return RizzbaseError(
        "Could not verify engine compatibility. Use a Rizzbase engine version in "
        "the supported range and check its --version output.",
        ErrorCode.ENGINE_INCOMPATIBLE,
    )


def _normalize_retry_options(options, timeout_s):
    if options is None:
        options = {}
    if not isinstance(options, dict):
        _invalid("retry must be a mapping.")
    allowed = {"max_attempts", "base_delay_ms", "max_delay_ms", "total_budget_ms", "lock_wait_ms"}
    unknown = set(options) - allowed
    if unknown:
        _invalid(f"Unknown retry option: {sorted(unknown)[0]}.")
    timeout_ms = timeout_s * 1000
    result = {
        "max_attempts": options.get("max_attempts", 3),
        "base_delay_ms": options.get("base_delay_ms", 75),
        "max_delay_ms": options.get("max_delay_ms", 1000),
        "total_budget_ms": options.get("total_budget_ms", min(2000, max(0, math.ceil(timeout_ms) - 1))),
        "lock_wait_ms": options.get("lock_wait_ms", min(1000, max(0, math.ceil(timeout_ms) - 1))),
    }
    attempts = result["max_attempts"]
    if isinstance(attempts, bool) or not isinstance(attempts, int) or not 1 <= attempts <= 10:
        _invalid("retry.max_attempts must be an integer from 1 to 10.")
    for name in ("base_delay_ms", "max_delay_ms", "total_budget_ms", "lock_wait_ms"):
        value = result[name]
        if isinstance(value, bool) or not isinstance(value, int) or value < 0:
            _invalid(f"retry.{name} must be a non-negative integer.")
    if result["max_delay_ms"] < result["base_delay_ms"]:
        _invalid("retry.max_delay_ms must be at least retry.base_delay_ms.")
    if result["total_budget_ms"] >= timeout_ms:
        _invalid("retry.total_budget_ms must be less than timeout_s.")
    if result["lock_wait_ms"] >= timeout_ms:
        _invalid("retry.lock_wait_ms must be less than timeout_s.")
    return result


class _OutputLimitExceeded(Exception):
    pass


def _run_bounded(command, input_data, timeout_s, max_output):
    process = subprocess.Popen(
        command,
        stdin=subprocess.PIPE if input_data is not None else subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
    )
    chunks = []
    output_size = 0
    output_exceeded = threading.Event()

    def collect_output():
        nonlocal output_size
        while True:
            chunk = process.stdout.read1(8192)
            if not chunk:
                return
            output_size += len(chunk)
            if output_size > max_output:
                output_exceeded.set()
                try:
                    process.kill()
                except OSError:
                    pass
            elif not output_exceeded.is_set():
                chunks.append(chunk)

    reader = threading.Thread(target=collect_output, daemon=True)
    reader.start()
    writer = None
    if input_data is not None:
        def write_input():
            try:
                process.stdin.write(input_data)
                process.stdin.flush()
            except (BrokenPipeError, OSError, ValueError):
                pass
            finally:
                try:
                    process.stdin.close()
                except (BrokenPipeError, OSError, ValueError):
                    pass

        writer = threading.Thread(target=write_input, daemon=True)
        writer.start()

    try:
        return_code = process.wait(timeout=timeout_s)
    except subprocess.TimeoutExpired as error:
        try:
            process.kill()
        except OSError:
            pass
        process.wait()
        if writer:
            writer.join(timeout=1)
        reader.join(timeout=1)
        process.stdout.close()
        raise error
    if writer:
        writer.join(timeout=1)
    reader.join(timeout=1)
    process.stdout.close()
    if output_exceeded.is_set():
        raise _OutputLimitExceeded()
    return return_code, b"".join(chunks)


def _ensure_compatible(binary):
    with _VERSION_LOCK:
        if binary in _CHECKED_BINARIES:
            return
    try:
        return_code, output = _run_bounded(
            [binary, "--version"],
            input_data=None,
            timeout_s=5,
            max_output=4096,
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        if isinstance(error, subprocess.TimeoutExpired):
            raise _incompatible() from error
        raise _binary_unavailable(f"binary_path: {binary}") from error
    except _OutputLimitExceeded as error:
        raise _incompatible() from error
    if return_code != 0:
        raise _incompatible()
    _parse_version(output.decode("utf-8", errors="replace"))
    with _VERSION_LOCK:
        _CHECKED_BINARIES.add(binary)


def _validate_query(options):
    if not isinstance(options, dict):
        _invalid("query options must be a mapping.")
    if "limit" in options:
        limit = options["limit"]
        if isinstance(limit, bool) or not isinstance(limit, int) or limit < 0:
            _invalid("limit must be a non-negative integer.")
    if "cursor" in options and (not isinstance(options["cursor"], str) or len(options["cursor"]) > 4096):
        _invalid("cursor must be a string no longer than 4096 characters.")
    where = options.get("where", [])
    if not isinstance(where, list):
        _invalid("where must be a list of filters.")
    for item in where:
        if not isinstance(item, dict):
            _invalid("Query filters must be mappings.")
        field = item.get("field")
        if not isinstance(field, str) or any(not part.strip() for part in field.split(".")):
            _invalid("Query fields must have non-empty path segments.")
        if item.get("operator") not in ("==", "!=", ">", ">=", "<", "<="):
            _invalid(f"Invalid query operator: {item.get('operator')}.")
        if "value" not in item:
            _invalid("Query filters require a JSON value.")
        _validate_json(item["value"])
    if "orderBy" in options:
        order_by = options["orderBy"]
        if not isinstance(order_by, dict):
            _invalid("orderBy must be a mapping.")
        field = order_by.get("field")
        if not isinstance(field, str) or any(not part.strip() for part in field.split(".")):
            _invalid("orderBy field must have non-empty path segments.")
        if order_by.get("direction") not in ("asc", "desc"):
            _invalid("orderBy direction must be asc or desc.")


class Rizzbase:
    def __init__(self, path, passphrase, *, binary_path=None, timeout_s=30, retry=None):
        if not isinstance(path, str) or not path.endswith(".rizz"):
            _invalid("Invalid project path: expected a .rizz file.")
        if not isinstance(passphrase, str) or not passphrase:
            _invalid("Passphrase must not be empty.")
        if (isinstance(timeout_s, bool) or not isinstance(timeout_s, (int, float))
                or not math.isfinite(timeout_s) or timeout_s <= 0 or timeout_s > 2_147_483_647):
            _invalid("timeout_s must be a positive 32-bit timeout.")
        self.path = str(pathlib.Path(path).expanduser().resolve())
        self._passphrase = passphrase
        self._binary_path = binary_path
        self._timeout_s = timeout_s
        self._retry = _normalize_retry_options(retry, timeout_s)
        self._operation_lock = threading.RLock()

    def _request(self, operation, **fields):
        body = {
            "operation": operation,
            "path": self.path,
            "passphrase": self._passphrase,
            **fields,
        }
        if operation in _WRITE_OPERATIONS:
            body["lockWaitMs"] = self._retry["lock_wait_ms"]
        try:
            encoded = json.dumps(body, ensure_ascii=False, allow_nan=False, separators=(",", ":")).encode("utf-8")
        except (TypeError, ValueError) as error:
            _invalid(f"Request contains values that are not valid JSON: {error}")
        if len(encoded) > MAX_REQUEST_SIZE:
            _invalid("Engine requests cannot exceed 64 KiB.")

        with self._operation_lock:
            binary = _resolve_binary(self._binary_path)
            _ensure_compatible(binary)
            retry_deadline = None
            for attempt in range(1, self._retry["max_attempts"] + 1):
                attempt_timeout = self._timeout_s if attempt == 1 else min(
                    self._timeout_s,
                    max(0, retry_deadline - time.monotonic()),
                )
                if attempt_timeout <= 0:
                    raise RizzbaseError("Safe retry budget expired.", ErrorCode.ENGINE_TIMEOUT)
                try:
                    return_code, output = _run_bounded(
                        [binary],
                        input_data=encoded,
                        timeout_s=attempt_timeout,
                        max_output=MAX_RESPONSE_SIZE,
                    )
                    break
                except subprocess.TimeoutExpired as error:
                    timeout_error = RizzbaseError(
                        "Engine request timed out; writes are not automatically retried because their outcome may be ambiguous.",
                        ErrorCode.ENGINE_TIMEOUT,
                    )
                    can_retry = operation in _READ_OPERATIONS and attempt < self._retry["max_attempts"]
                    if not can_retry:
                        raise timeout_error from error
                    if retry_deadline is None:
                        retry_deadline = time.monotonic() + self._retry["total_budget_ms"] / 1000
                    remaining_ms = (retry_deadline - time.monotonic()) * 1000
                    if remaining_ms <= 0:
                        raise timeout_error from error
                    ceiling = min(self._retry["max_delay_ms"], self._retry["base_delay_ms"] * (2 ** (attempt - 1)))
                    delay_ms = random.uniform(0, min(ceiling, remaining_ms))
                    time.sleep(delay_ms / 1000)
                except OSError as error:
                    raise _binary_unavailable(f"binary_path: {binary}") from error
                except _OutputLimitExceeded as error:
                    raise RizzbaseError("Engine response exceeded its size limit.", ErrorCode.ENGINE_PROTOCOL_ERROR) from error

        if len(output) > MAX_RESPONSE_SIZE:
            raise RizzbaseError("Engine response exceeded its size limit.", ErrorCode.ENGINE_PROTOCOL_ERROR)
        try:
            response = json.loads(output)
        except (json.JSONDecodeError, UnicodeDecodeError) as error:
            raise RizzbaseError("Engine returned an invalid response.", ErrorCode.ENGINE_PROTOCOL_ERROR) from error
        if not isinstance(response, dict) or not isinstance(response.get("ok"), bool):
            raise RizzbaseError("Engine returned an invalid response.", ErrorCode.ENGINE_PROTOCOL_ERROR)
        if response["ok"] is False:
            code = response.get("errorCode")
            if code not in _ENGINE_ERROR_CODES:
                code = ErrorCode.ENGINE_ERROR
            message = response.get("error")
            context = response.get("errorContext")
            raise RizzbaseError(
                message if isinstance(message, str) else "Engine operation failed.",
                code,
                context if isinstance(context, dict) else None,
            )
        if return_code != 0:
            raise RizzbaseError("Engine exited unsuccessfully.", ErrorCode.ENGINE_ERROR)
        return response

    def create(self, name):
        return self._request("create", name=name)["project"]

    def open(self):
        return self._request("open")["project"]

    def doctor(self):
        """Return non-destructive project diagnostics; supplying a passphrase enables full validation."""
        return self._request("doctor")["doctor"]

    def export(self, backup_path):
        if not isinstance(backup_path, str) or not backup_path.endswith(".json"):
            _invalid("Invalid backup path: expected a .json file.")
        destination = pathlib.Path(backup_path).expanduser().resolve()
        if str(destination) == self.path:
            _invalid("Backup path must differ from the Project path.")
        project = self.open()
        data = (json.dumps(project, indent=2, ensure_ascii=False, allow_nan=False) + "\n").encode("utf-8")
        if len(data) > MAX_BACKUP_SIZE:
            raise RizzbaseError("Project JSON backup exceeds the 64 MiB limit.", ErrorCode.EXPORT_FAILED)
        temporary_directory = None
        try:
            temporary_directory = tempfile.mkdtemp(prefix=".rizzbase-backup-", dir=destination.parent)
            temporary = pathlib.Path(temporary_directory) / "backup.json"
            descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(descriptor, "wb") as backup:
                backup.write(data)
            os.link(temporary, destination)
        except FileExistsError as error:
            raise RizzbaseError("Backup destination already exists; choose a new file path.", ErrorCode.FILE_EXISTS) from error
        except FileNotFoundError as error:
            raise RizzbaseError("Backup destination directory does not exist.", ErrorCode.FILE_NOT_FOUND) from error
        except PermissionError as error:
            raise RizzbaseError("Permission denied while writing the backup.", ErrorCode.PERMISSION_DENIED) from error
        except OSError as error:
            raise RizzbaseError("Could not export the Project to a JSON backup.", ErrorCode.EXPORT_FAILED) from error
        finally:
            if temporary_directory:
                shutil.rmtree(temporary_directory, ignore_errors=True)

    def import_backup(self, backup_path):
        if not isinstance(backup_path, str) or not backup_path.endswith(".json"):
            _invalid("Invalid backup path: expected a .json file.")
        source = pathlib.Path(backup_path).expanduser().resolve()
        if str(source) == self.path:
            _invalid("Backup path must differ from the Project path.")
        return self._request("project.import", backupPath=str(source))["project"]

    def create_lobby(self, name):
        return self._request("lobby.create", name=name)["lobby"]

    def list_lobbies(self):
        return self._request("lobby.list").get("lobbies", [])

    def delete_lobby(self, lobby_id):
        self._request("lobby.delete", lobbyId=lobby_id)

    def create_yap(self, lobby_id, yap_id, data):
        _validate_json(data)
        return self._request("yap.create", lobbyId=lobby_id, yapId=yap_id, data=data)["yap"]

    def create_yaps(self, lobby_id, yaps):
        if not isinstance(yaps, list) or not 1 <= len(yaps) <= 1000:
            _invalid("yaps must be a non-empty list of at most 1000 items.")
        ids = set()
        for yap in yaps:
            if not isinstance(yap, dict) or set(yap) != {"id", "data"}:
                _invalid("Each batch Yap must contain exactly id and data.")
            identifier = yap["id"]
            if not isinstance(identifier, str) or not identifier.strip() or len(identifier.encode("utf-8")) > 128:
                _invalid("Each Yap requires an ID between 1 and 128 bytes.")
            if identifier in ids:
                _invalid(f"Duplicate Yap ID in batch: {identifier}.")
            ids.add(identifier)
            _validate_json(yap["data"])
        return self._request("yap.bulkCreate", lobbyId=lobby_id, yaps=yaps).get("yaps", [])

    def get_yap(self, lobby_id, yap_id):
        return self._request("yap.get", lobbyId=lobby_id, yapId=yap_id)["data"]

    def list_yaps(self, lobby_id):
        return self._request("yap.list", lobbyId=lobby_id).get("yaps", [])

    def update_yap(self, lobby_id, yap_id, data):
        _validate_json(data)
        return self._request("yap.update", lobbyId=lobby_id, yapId=yap_id, data=data)["yap"]

    def delete_yap(self, lobby_id, yap_id):
        self._request("yap.delete", lobbyId=lobby_id, yapId=yap_id)

    def query_yaps(self, lobby_id, options=None):
        options = deepcopy(options or {})
        _validate_query(options)
        return self._request("yap.query", lobbyId=lobby_id, query=options).get("yaps", [])

    def query_yap_page(self, lobby_id, options=None):
        options = deepcopy(options or {})
        _validate_query(options)
        return self._request("yap.query", lobbyId=lobby_id, query=options)["page"]

    def query(self, lobby_id):
        return YapQuery(self, lobby_id)


class YapQuery:
    def __init__(self, database, lobby_id, options=None):
        self._database = database
        self._lobby_id = lobby_id
        self._options = options or {}

    def where(self, field, operator, value):
        options = deepcopy(self._options)
        options.setdefault("where", []).append({"field": field, "operator": operator, "value": value})
        _validate_query(options)
        return YapQuery(self._database, self._lobby_id, options)

    def order_by(self, field, direction="asc"):
        options = deepcopy(self._options)
        options["orderBy"] = {"field": field, "direction": direction}
        _validate_query(options)
        return YapQuery(self._database, self._lobby_id, options)

    def limit(self, count):
        options = deepcopy(self._options)
        options["limit"] = count
        _validate_query(options)
        return YapQuery(self._database, self._lobby_id, options)

    def after(self, cursor):
        options = deepcopy(self._options)
        options["cursor"] = cursor
        _validate_query(options)
        return YapQuery(self._database, self._lobby_id, options)

    def execute(self):
        return self._database.query_yaps(self._lobby_id, self._options)

    def execute_page(self):
        return self._database.query_yap_page(self._lobby_id, self._options)
