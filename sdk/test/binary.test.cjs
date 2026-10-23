const { chmod, mkdir, mkdtemp, rm, writeFile } = require("node:fs/promises");
const { tmpdir } = require("node:os");
const { delimiter, join } = require("node:path");
const { afterEach, test } = require("node:test");
const assert = require("node:assert/strict");
const { ensureCompatibleEngine, resolveBinary, validateEngineVersion } = require("../dist/binary.js");

const directories = [];
const originalEnv = {
  path: process.env.PATH,
  binary: process.env.RIZZBASE_BINARY,
  fetch: global.fetch,
};
afterEach(async () => {
  if (originalEnv.path === undefined) delete process.env.PATH;
  else process.env.PATH = originalEnv.path;
  if (originalEnv.binary === undefined) delete process.env.RIZZBASE_BINARY;
  else process.env.RIZZBASE_BINARY = originalEnv.binary;
  global.fetch = originalEnv.fetch;
  await Promise.all(directories.splice(0).map(path => rm(path, { recursive: true, force: true })));
});

async function createBinary(directory, name = process.platform === "win32" ? "rizzbase.exe" : "rizzbase") {
  const path = join(directory, name);
  await writeFile(path, "test executable");
  if (process.platform !== "win32") await chmod(path, 0o755);
  return path;
}

test("explicit binaryPath takes precedence over the environment and PATH", async () => {
  const directory = await mkdtemp(join(tmpdir(), "rizz-binary-"));
  directories.push(directory);
  const explicit = await createBinary(directory, "explicit");
  process.env.RIZZBASE_BINARY = join(directory, "missing-env-binary");
  process.env.PATH = "";
  assert.equal(await resolveBinary({ binaryPath: explicit }), explicit);
});

test("RIZZBASE_BINARY takes precedence over PATH", async () => {
  const directory = await mkdtemp(join(tmpdir(), "rizz-binary-"));
  const pathDirectory = join(directory, "path");
  directories.push(directory);
  await mkdir(pathDirectory);
  const configured = await createBinary(directory, "configured");
  const fromPath = await createBinary(pathDirectory);
  process.env.RIZZBASE_BINARY = configured;
  process.env.PATH = pathDirectory;
  assert.equal(await resolveBinary({}), configured);
  assert.notEqual(configured, fromPath);
});

test("resolves the executable from PATH without making network requests", async () => {
  const directory = await mkdtemp(join(tmpdir(), "rizz-binary-"));
  directories.push(directory);
  const binary = await createBinary(directory);
  delete process.env.RIZZBASE_BINARY;
  process.env.PATH = `${directory}${delimiter}${join(directory, "missing")}`;
  global.fetch = () => { throw new Error("binary resolution must not use the network"); };
  assert.equal(await resolveBinary({}), binary);
});

test("missing binaries report searched paths and installation guidance", async () => {
  delete process.env.RIZZBASE_BINARY;
  process.env.PATH = "/definitely/missing/rizzbase-path";
  await assert.rejects(
    resolveBinary({}),
    error => error.code === "BINARY_UNAVAILABLE"
      && error.message.includes("/definitely/missing/rizzbase-path")
      && error.message.includes("RIZZBASE_BINARY"),
  );
});

test("rejects unsupported engine versions and project formats", () => {
  assert.doesNotThrow(() => validateEngineVersion("1.1.0", 1));
  assert.doesNotThrow(() => validateEngineVersion("1.9.99", 1));
  for (const [version, format] of [["1.0.9", 1], ["2.0.0", 1], ["dev", 1], ["01.1.0", 1], ["1.1.0", 2]]) {
    assert.throws(
      () => validateEngineVersion(version, format),
      error => error.code === "ENGINE_INCOMPATIBLE",
    );
  }
});

test("checks each resolved binary once and verifies its version output", async t => {
  if (process.platform === "win32") return t.skip("fixture uses a POSIX executable script");
  const directory = await mkdtemp(join(tmpdir(), "rizz-version-"));
  directories.push(directory);
  const binary = join(directory, "rizzbase");
  const calls = join(directory, "version-calls");
  await writeFile(binary, `#!/bin/sh\necho checked >> '${calls}'\nprintf '%s\\n' '{"version":"1.1.0","projectFormatVersion":1}'\n`);
  await chmod(binary, 0o755);
  await Promise.all([ensureCompatibleEngine(binary), ensureCompatibleEngine(binary)]);
  const { readFile } = require("node:fs/promises");
  assert.equal((await readFile(calls, "utf8")).trim().split("\n").length, 1);
});
