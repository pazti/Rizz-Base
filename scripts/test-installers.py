import hashlib
import http.server
import os
import pathlib
import platform
import socketserver
import subprocess
import tempfile
import threading


ROOT = pathlib.Path(__file__).resolve().parents[1]
INSTALLER = ROOT / "install.sh"


def run_installer(base, install_dir, version, api_base=None):
    def as_env_value(value):
        if isinstance(value, pathlib.Path):
            return value.as_uri()
        return value

    environment = os.environ.copy()
    environment.update(
        RIZZBASE_RELEASE_BASE_URL=as_env_value(base),
        RIZZBASE_INSTALL_DIR=str(install_dir),
    )
    if api_base is not None:
        environment["RIZZBASE_RELEASE_API_URL"] = as_env_value(api_base)
    if version is not None:
        environment["RIZZBASE_VERSION"] = version
    return subprocess.run(
        ["bash", str(INSTALLER)],
        env=environment,
        text=True,
        capture_output=True,
        check=False,
    )


def main():
    system = platform.system()
    if system not in ("Linux", "Darwin"):
        raise SystemExit("install.sh integration tests require Linux or macOS.")
    arch = platform.machine().lower()
    if arch in ("x86_64", "amd64"):
        arch = "amd64"
    elif arch in ("aarch64", "arm64"):
        arch = "arm64"
    else:
        raise SystemExit(f"Unsupported test architecture: {arch}")
    asset = f"rizzbase-{'linux' if system == 'Linux' else 'darwin'}-{arch}"

    with tempfile.TemporaryDirectory(prefix="rizzbase-installer-test-") as temporary:
        root = pathlib.Path(temporary)
        release = root / "v1.2.3"
        release.mkdir()
        binary = b"verified test executable"
        (release / asset).write_bytes(binary)
        checksum = hashlib.sha256(binary).hexdigest()
        (release / "SHA256SUMS").write_text(
            f"{'0' * 64}  another-asset\n{checksum}  {asset}\n",
            encoding="ascii",
        )
        (root / "releases").mkdir()
        (root / "releases" / "latest").write_text('{"tag_name":"v1.2.3"}', encoding="utf-8")

        class Handler(http.server.SimpleHTTPRequestHandler):
            def __init__(self, *args, **kwargs):
                super().__init__(*args, directory=str(root), **kwargs)

        server = socketserver.TCPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        server_base = f"http://127.0.0.1:{server.server_address[1]}"

        install_dir = root / "installed"
        installed = run_installer(server_base, install_dir, None, server_base)
        assert installed.returncode == 0, installed.stderr
        destination = install_dir / "rizzbase"
        assert destination.read_bytes() == binary
        assert destination.stat().st_mode & 0o777 == 0o755
        assert "PATH" in installed.stdout and "--version" in installed.stdout

        (release / asset).write_bytes(b"tampered")
        mismatch = run_installer(server_base, install_dir, "1.2.3", server_base)
        assert mismatch.returncode != 0
        assert "Checksum mismatch" in mismatch.stderr
        assert destination.read_bytes() == binary

        missing = run_installer(server_base, install_dir, "9.9.9", server_base)
        assert missing.returncode != 0
        assert destination.read_bytes() == binary

        server.shutdown()
        server.server_close()

    print("installer integration tests passed")


if __name__ == "__main__":
    main()
