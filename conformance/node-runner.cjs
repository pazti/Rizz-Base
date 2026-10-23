const assert = require("node:assert/strict");
const { chmod, mkdir, mkdtemp, readFile, rm, writeFile } = require("node:fs/promises");
const { tmpdir } = require("node:os");
const { join, resolve } = require("node:path");
const { Rizzbase } = require("../sdk/dist/index.js");

const fixturesDirectory = join(__dirname, "fixtures");
const binary = process.env.RIZZBASE_BINARY;
if (!binary) throw new Error("Set RIZZBASE_BINARY to a compatible engine executable.");

function replaceReferences(value, references) {
  if (value && typeof value === "object" && !Array.isArray(value)
    && value.$repeat && typeof value.$repeat.value === "string"
    && Number.isSafeInteger(value.$repeat.count) && value.$repeat.count >= 0) {
    return value.$repeat.value.repeat(value.$repeat.count);
  }
  if (typeof value === "string") {
    if (Object.hasOwn(references, value)) return references[value];
    return value.replace(/\$\{([^}]+)\}/g, (_, name) => {
      const key = Object.hasOwn(references, name) ? name : `$${name}`;
      if (!Object.hasOwn(references, key)) throw new Error(`Unknown fixture reference: ${name}`);
      return references[key];
    });
  }
  if (Array.isArray(value)) return value.map(item => replaceReferences(item, references));
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, replaceReferences(item, references)]));
  }
  return value;
}

function getPath(value, path) {
  return path.split(".").reduce((current, key) => current?.[key], value);
}

function normalize(value, generatedIds, directory, key = "") {
  if (key === "createdAt") return "<timestamp>";
  if (typeof value === "string") {
    if (generatedIds.includes(value)) return "<generated-id>";
    return value.replaceAll(directory, "<temporary-directory>");
  }
  if (Array.isArray(value)) return value.map(item => normalize(item, generatedIds, directory));
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.keys(value).sort().map(itemKey => [
      generatedIds.includes(itemKey) ? "<generated-id>" : itemKey,
      normalize(value[itemKey], generatedIds, directory, itemKey),
    ]));
  }
  return value;
}

async function runFixture(fixturePath) {
  const fixture = JSON.parse(await readFile(fixturePath, "utf8"));
  const directory = await mkdtemp(join(tmpdir(), "rizzbase-conformance-"));
  const references = {
    "$project": join(directory, "project.rizz"),
    "$missingProject": join(directory, "missing.rizz"),
    "$invalidProject": join(directory, "invalid.rizz"),
    "$restoredProject": join(directory, "restored.rizz"),
    "$exportedBackup": join(directory, "exported.json"),
    "$missingBinary": join(directory, "missing-engine"),
    "$incompatibleBinary": process.env.RIZZBASE_INCOMPATIBLE_BINARY,
    "$protocolErrorBinary": process.env.RIZZBASE_PROTOCOL_ERROR_BINARY,
    "$nonzeroBinary": process.env.RIZZBASE_NONZERO_BINARY,
    "$missingExportDirectory": join(directory, "missing-directory"),
  };
  const generatedIds = [];
  const normalizedSteps = [];
  try {
    for (const [key, contents] of Object.entries(fixture.setupFiles ?? {})) {
      const path = join(directory, `${key.slice(1)}${key.endsWith("Project") ? ".rizz" : ".json"}`);
      references[key] = path;
      const data = typeof contents === "string"
        ? contents
        : Number.isSafeInteger(contents.brainrotBytes)
          ? Array(contents.brainrotBytes).fill("rizz rizz").join(" ")
          : `${JSON.stringify(contents)}\n`;
      await writeFile(path, data);
    }
    for (const [key, definition] of Object.entries(fixture.setupDirectories ?? {})) {
      const path = join(directory, definition.path);
      references[key] = path;
      await mkdir(path, { recursive: true, mode: definition.mode });
      await chmod(path, definition.mode);
    }

    for (const step of fixture.steps) {
      if (step.skipIfRoot && process.getuid?.() === 0) {
        normalizedSteps.push({ step: step.id, result: { skipped: "requires a non-root user" } });
        continue;
      }
      const args = replaceReferences(step.args ?? {}, references);
      const projectPath = args.path ?? references.$project;
      const passphrase = args.passphrase ?? "conformance-passphrase";
      const db = new Rizzbase(projectPath, passphrase, {
        binaryPath: args.binaryPath ?? binary,
        ...(args.timeoutMs !== undefined ? { timeoutMs: args.timeoutMs } : {}),
      });
      let result;
      let error;
      try {
        switch (step.operation) {
          case "create":
            result = { project: await db.create(args.name) };
            break;
          case "open":
            result = { project: await db.open() };
            break;
          case "project.import":
            result = { project: await db.importBackup(args.backupPath) };
            break;
          case "project.export":
            await db.export(args.backupPath);
            result = {};
            break;
          case "sdk.export":
            await db.export(args.backupPath);
            result = {};
            break;
          case "lobby.create":
            result = { lobby: await db.createLobby(args.name) };
            break;
          case "lobby.list":
            result = { lobbies: await db.listLobbies() };
            break;
          case "lobby.delete":
            await db.deleteLobby(args.lobbyId);
            result = {};
            break;
          case "yap.create":
            result = { yap: await db.createYap(args.lobbyId, args.yapId, args.data) };
            break;
          case "yap.get":
            result = { data: await db.getYap(args.lobbyId, args.yapId) };
            break;
          case "yap.list":
            result = { yaps: await db.listYaps(args.lobbyId) };
            break;
          case "yap.query":
            result = { yaps: await db.queryYaps(args.lobbyId, args.query) };
            break;
          case "yap.update":
            result = { yap: await db.updateYap(args.lobbyId, args.yapId, args.data) };
            break;
          case "yap.delete":
            await db.deleteYap(args.lobbyId, args.yapId);
            result = {};
            break;
          default:
            throw new Error(`Unsupported fixture operation: ${step.operation}`);
        }
      } catch (caught) {
        error = caught;
      }

      if (step.expect?.errorCode) {
        assert.equal(error?.code, step.expect.errorCode, `${fixture.name}/${step.id}`);
        assert.equal(result, undefined, `${fixture.name}/${step.id} unexpectedly succeeded`);
      } else {
        assert.equal(error, undefined, `${fixture.name}/${step.id}: ${error?.message}`);
        for (const [path, expected] of Object.entries(step.expect?.fields ?? {})) {
          assert.deepEqual(getPath(result, path), expected, `${fixture.name}/${step.id} field ${path}`);
        }
      }
      for (const [name, path] of Object.entries(step.capture ?? {})) {
        const captured = getPath(result, path);
        assert.notEqual(captured, undefined, `${fixture.name}/${step.id} did not provide ${path}`);
        references[name] = captured;
        if (typeof captured === "string") generatedIds.push(captured);
      }
      const normalizedResult = error
        ? { ok: false, errorCode: error.code }
        : { ok: true, ...result };
      if (!error && step.operation === "lobby.list" && !("lobbies" in normalizedResult)) normalizedResult.lobbies = [];
      if (!error && ["yap.list", "yap.query"].includes(step.operation) && !("yaps" in normalizedResult)) normalizedResult.yaps = [];
      normalizedSteps.push({
        step: step.id,
        result: normalize(normalizedResult, generatedIds, directory),
      });
    }
    return { fixture: fixture.name, steps: fixture.steps.length, results: normalizedSteps };
  } finally {
    for (const definition of Object.values(fixture.setupDirectories ?? {})) {
      try {
        await chmod(join(directory, definition.path), 0o700);
      } catch (error) {
        if (error.code !== "ENOENT") throw error;
      }
    }
    await rm(directory, { recursive: true, force: true });
  }
}

async function main() {
  const files = process.argv.slice(2);
  const fixtures = files.length
    ? files.map(resolve)
    : (await (await import("node:fs/promises")).readdir(fixturesDirectory))
      .filter(name => name.endsWith(".json"))
      .sort()
      .map(name => join(fixturesDirectory, name));
  console.log(JSON.stringify(await Promise.all(fixtures.map(runFixture))));
}

main().catch(error => {
  console.error(error);
  process.exitCode = 1;
});
