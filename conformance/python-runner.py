#!/usr/bin/env python3
import json
import os
import pathlib
import sys
import tempfile


ROOT = pathlib.Path(__file__).resolve().parent
FIXTURES = ROOT / "fixtures"
PYTHON_PACKAGE = ROOT.parent / "python" / "src"
sys.path.insert(0, str(PYTHON_PACKAGE))
from rizzbase import Rizzbase


if not os.environ.get("RIZZBASE_BINARY"):
    raise SystemExit("Set RIZZBASE_BINARY to a compatible engine executable.")


def replace_references(value, references):
    if (
        isinstance(value, dict)
        and isinstance(value.get("$repeat"), dict)
        and isinstance(value["$repeat"].get("value"), str)
        and isinstance(value["$repeat"].get("count"), int)
        and value["$repeat"]["count"] >= 0
    ):
        return value["$repeat"]["value"] * value["$repeat"]["count"]
    if isinstance(value, str):
        if value in references:
            return references[value]

        def replace(match):
            key = match.group(1)
            if key not in references and f"${key}" in references:
                key = f"${key}"
            if key not in references:
                raise ValueError(f"Unknown fixture reference: {key}")
            return str(references[key])

        import re
        return re.sub(r"\$\{([^}]+)\}", replace, value)
    if isinstance(value, list):
        return [replace_references(item, references) for item in value]
    if isinstance(value, dict):
        return {key: replace_references(item, references) for key, item in value.items()}
    return value


def get_path(value, path):
    for key in path.split("."):
        if isinstance(value, dict):
            value = value.get(key, [] if key in ("lobbies", "yaps") else None)
        elif isinstance(value, list):
            if key == "length":
                value = len(value)
            else:
                value = value[int(key)]
        else:
            return None
    return value


def normalize(value, generated_ids, directory, key=""):
    if key == "createdAt":
        return "<timestamp>"
    if isinstance(value, str):
        if value in generated_ids:
            return "<generated-id>"
        return value.replace(str(directory), "<temporary-directory>")
    if isinstance(value, list):
        return [normalize(item, generated_ids, directory) for item in value]
    if isinstance(value, dict):
        return {
            ("<generated-id>" if item_key in generated_ids else item_key):
            normalize(item, generated_ids, directory, item_key)
            for item_key, item in sorted(value.items())
        }
    return value


def run_fixture(fixture_path):
    fixture = json.loads(fixture_path.read_text(encoding="utf-8"))
    with tempfile.TemporaryDirectory(prefix="rizzbase-conformance-") as temporary:
        directory = pathlib.Path(temporary)
        references = {
            "$project": str(directory / "project.rizz"),
            "$missingProject": str(directory / "missing.rizz"),
            "$invalidProject": str(directory / "invalid.rizz"),
            "$restoredProject": str(directory / "restored.rizz"),
            "$exportedBackup": str(directory / "exported.json"),
            "$missingBinary": str(directory / "missing-engine"),
            "$incompatibleBinary": os.environ.get("RIZZBASE_INCOMPATIBLE_BINARY"),
            "$protocolErrorBinary": os.environ.get("RIZZBASE_PROTOCOL_ERROR_BINARY"),
            "$nonzeroBinary": os.environ.get("RIZZBASE_NONZERO_BINARY"),
            "$missingExportDirectory": str(directory / "missing-directory"),
        }
        for key, contents in fixture.get("setupFiles", {}).items():
            extension = ".rizz" if key.endswith("Project") else ".json"
            file_path = directory / f"{key[1:]}{extension}"
            references[key] = str(file_path)
            if isinstance(contents, str):
                file_path.write_text(contents, encoding="utf-8")
            elif isinstance(contents, dict) and isinstance(contents.get("brainrotBytes"), int):
                file_path.write_text(" ".join(["rizz rizz"] * contents["brainrotBytes"]), encoding="ascii")
            else:
                file_path.write_text(json.dumps(contents) + "\n", encoding="utf-8")
        for key, definition in fixture.get("setupDirectories", {}).items():
            directory_path = directory / definition["path"]
            directory_path.mkdir(mode=definition["mode"], parents=True)
            os.chmod(directory_path, definition["mode"])
            references[key] = str(directory_path)

        generated_ids = []
        normalized_steps = []
        for step in fixture["steps"]:
            if step.get("skipIfRoot") and hasattr(os, "geteuid") and os.geteuid() == 0:
                normalized_steps.append({
                    "step": step["id"],
                    "result": {"skipped": "requires a non-root user"},
                })
                continue
            args = replace_references(step.get("args", {}), references)
            project_path = args.pop("path", references["$project"])
            passphrase = args.pop("passphrase", "conformance-passphrase")
            database = Rizzbase(
                project_path,
                passphrase,
                binary_path=args.get("binaryPath", os.environ["RIZZBASE_BINARY"]),
                timeout_s=args.get("timeout_s", 30),
            )
            error = None
            result = None
            try:
                operation = step["operation"]
                if operation == "create":
                    result = {"project": database.create(args["name"])}
                elif operation == "open":
                    result = {"project": database.open()}
                elif operation == "project.import":
                    result = {"project": database.import_backup(args["backupPath"])}
                elif operation == "project.export":
                    database.export(args["backupPath"])
                    result = {}
                elif operation == "sdk.export":
                    database.export(args["backupPath"])
                    result = {}
                elif operation == "lobby.create":
                    result = {"lobby": database.create_lobby(args["name"])}
                elif operation == "lobby.list":
                    result = {"lobbies": database.list_lobbies()}
                elif operation == "lobby.delete":
                    database.delete_lobby(args["lobbyId"])
                    result = {}
                elif operation == "yap.create":
                    result = {"yap": database.create_yap(args["lobbyId"], args["yapId"], args["data"])}
                elif operation == "yap.get":
                    result = {"data": database.get_yap(args["lobbyId"], args["yapId"])}
                elif operation == "yap.list":
                    result = {"yaps": database.list_yaps(args["lobbyId"])}
                elif operation == "yap.query":
                    result = {"yaps": database.query_yaps(args["lobbyId"], args.get("query"))}
                elif operation == "yap.update":
                    result = {"yap": database.update_yap(args["lobbyId"], args["yapId"], args["data"])}
                elif operation == "yap.delete":
                    database.delete_yap(args["lobbyId"], args["yapId"])
                    result = {}
                else:
                    raise AssertionError(f"Unsupported fixture operation: {operation}")
            except Exception as caught:
                error = caught
            expected_error = step.get("expect", {}).get("errorCode")
            if expected_error:
                assert getattr(error, "code", None) == expected_error, (
                    f"{fixture['name']}/{step['id']}: expected {expected_error}, got {error}"
                )
            else:
                assert error is None, f"{fixture['name']}/{step['id']} failed: {error}"
                for path, expected in step.get("expect", {}).get("fields", {}).items():
                    assert get_path(result, path) == expected, (
                        f"{fixture['name']}/{step['id']} field {path}: "
                        f"expected {expected!r}, got {get_path(result, path)!r}"
                    )

            for name, path in step.get("capture", {}).items():
                captured = get_path(result, path)
                assert captured is not None, f"{fixture['name']}/{step['id']} did not provide {path}"
                references[name] = captured
                if isinstance(captured, str):
                    generated_ids.append(captured)

            normalized_result = {"ok": False, "errorCode": error.code} if error else {"ok": True, **result}
            if not expected_error and step["operation"] == "lobby.list":
                normalized_result.setdefault("lobbies", [])
            if not expected_error and step["operation"] in ("yap.list", "yap.query"):
                normalized_result.setdefault("yaps", [])
            normalized_steps.append({
                "step": step["id"],
                "result": normalize(normalized_result, generated_ids, directory),
            })

    return {"fixture": fixture["name"], "steps": len(fixture["steps"]), "results": normalized_steps}


def main():
    files = [pathlib.Path(item).resolve() for item in sys.argv[1:]]
    if not files:
        files = sorted(FIXTURES.glob("*.json"))
    print(json.dumps([run_fixture(path) for path in files], separators=(",", ":")))


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(error, file=sys.stderr)
        raise SystemExit(1)
