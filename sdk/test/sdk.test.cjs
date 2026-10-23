const { execFileSync } = require("node:child_process");
const { mkdtemp, readFile, rm, stat, writeFile } = require("node:fs/promises");
const { tmpdir } = require("node:os");
const { join } = require("node:path");
const { after, before, test } = require("node:test");
const assert = require("node:assert/strict");
const { Rizzbase, RizzbaseError } = require("../dist/index.js");
const { pathToFileURL } = require("node:url");

let directory;
let binary;
before(async () => {
  directory = await mkdtemp(join(tmpdir(), "rizzbase-sdk-"));
  binary = join(directory, "rizzbase");
  execFileSync("go", ["build", "-ldflags=-X main.version=1.1.0", "-o", binary, "./cmd/rizzbase"], { cwd: join(__dirname, "../../engine") });
});
after(async () => rm(directory, { recursive: true, force: true }));

test("SDK covers project, Lobby, Yap and query operations", async () => {
  const path = join(directory, "demo.rizz");
  const db = new Rizzbase(path, "SDK test passphrase", { binaryPath: binary });
  assert.equal((await db.create("SDK test")).name, "SDK test");
  const lobby = await db.createLobby("general");
  await Promise.all([
    db.createYap(lobby.id, "one", { score: 2, user: { active: true } }),
    db.createYap(lobby.id, "two", { score: 5, user: { active: false } }),
    db.createYap(lobby.id, "three", { score: 8, user: { active: true } }),
  ]);
  assert.deepEqual(await db.getYap(lobby.id, "one"), { score: 2, user: { active: true } });
  assert.deepEqual((await db.listYaps(lobby.id)).map(y => y.id), ["one", "three", "two"]);
  assert.deepEqual((await db.query(lobby.id).where("score", ">=", 2).where("user.active", "==", true).limit(1).exec()).map(y => y.id), ["one"]);
  assert.deepEqual((await db.queryYaps(lobby.id, { where: [{ field: "score", operator: ">", value: 5 }] })).map(y => y.id), ["three"]);
  assert.deepEqual((await db.query(lobby.id).orderBy("score").exec()).map(y => y.id), ["one", "two", "three"]);
  assert.deepEqual((await db.query(lobby.id)
    .where("score", ">=", 2)
    .orderBy("score", "desc")
    .limit(2)
    .exec()).map(y => y.id), ["three", "two"]);
  assert.deepEqual((await db.queryYaps(lobby.id, {
    orderBy: { field: "score", direction: "desc" },
    limit: 1,
  })).map(y => y.id), ["three"]);
  await db.updateYap(lobby.id, "one", { score: 3 });
  assert.deepEqual(await db.getYap(lobby.id, "one"), { score: 3 });
  assert.deepEqual((await db.listLobbies()).map(l => l.id), [lobby.id]);
  const project = await db.open();
  assert.equal(project.name, "SDK test");
  const backupPath = join(directory, "backup.json");
  await db.export(backupPath);
  const backupContents = await readFile(backupPath, "utf8");
  assert.deepEqual(JSON.parse(backupContents), project);
  assert.equal(backupContents.includes("SDK test passphrase"), false);
  if (process.platform !== "win32") assert.equal((await stat(backupPath)).mode & 0o777, 0o600);
  await assert.rejects(db.export(backupPath), e => e.code === "FILE_EXISTS");
  const restoredPath = join(directory, "restored.rizz");
  const restored = new Rizzbase(restoredPath, "restored passphrase", { binaryPath: binary });
  assert.deepEqual(await restored.importBackup(backupPath), project);
  assert.deepEqual(await restored.open(), project);
  await assert.rejects(restored.importBackup(backupPath), e => e.code === "FILE_EXISTS");
  const invalidBackup = join(directory, "invalid.json");
  await writeFile(invalidBackup, `{"version":99}`);
  await assert.rejects(
    new Rizzbase(join(directory, "invalid.rizz"), "passphrase", { binaryPath: binary }).importBackup(invalidBackup),
    e => e.code === "INVALID_BACKUP",
  );
  await db.deleteYap(lobby.id, "one");
  await db.deleteLobby(lobby.id);
  assert.deepEqual(await db.listLobbies(), []);
});

test("SDK reports typed engine and input errors without exposing passphrase", async () => {
  const path = join(directory, "errors.rizz");
  const db = new Rizzbase(path, "never-echo-this", { binaryPath: binary });
  await db.create("Errors");
  await assert.rejects(new Rizzbase(path, "wrong passphrase", { binaryPath: binary }).open(),
    e => e instanceof RizzbaseError && e.code === "AUTHENTICATION_FAILED" && !e.message.includes("wrong passphrase"));
  await assert.rejects(db.getYap("missing", "y"), e => e.code === "LOBBY_NOT_FOUND" && e.message === "Lobby not found.");
  assert.throws(() => new Rizzbase("missing.txt", "secret"), e => e.code === "INVALID_ARGUMENT" && e.message.includes("Invalid project path"));
  assert.throws(() => db.query("lobby").limit(-1), e => e.code === "INVALID_ARGUMENT");
  assert.throws(() => db.query("lobby").orderBy("score", "sideways"), e => e.code === "INVALID_ARGUMENT");
  await assert.rejects(db.queryYaps("lobby", { where: [{ field: "x", operator: "between", value: 1 }] }), e => e.code === "INVALID_ARGUMENT" && e.message.includes("Invalid query operator"));
  const circular = {}; circular.self = circular;
  await assert.rejects(db.createYap("lobby", "circular", circular), e => e.code === "INVALID_ARGUMENT");
  assert.equal((await (await import("node:fs/promises")).readFile(path, "utf8")).includes("never-echo-this"), false);
});

test("package supports named ES module imports", async () => {
  const imported = await import(pathToFileURL(join(__dirname, "../dist/index.js")));
  assert.equal(typeof imported.Rizzbase, "function");
  assert.equal(typeof imported.RizzbaseError, "function");
});

test("failed operations do not poison the instance queue", async () => {
  const db = new Rizzbase(join(directory, "queue.rizz"), "passphrase", { binaryPath: binary });
  await db.create("Queue");
  await assert.rejects(db.deleteLobby("does-not-exist"), e => e.code === "LOBBY_NOT_FOUND");
  assert.deepEqual(await db.listLobbies(), []);
});
