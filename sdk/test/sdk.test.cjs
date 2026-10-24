const { execFileSync } = require("node:child_process");
const { chmod, mkdtemp, readFile, rm, stat, writeFile } = require("node:fs/promises");
const { hostname, tmpdir } = require("node:os");
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

async function createFakeEngine(name, onRequest) {
  const path = join(directory, name);
  const source = `#!/usr/bin/env node
const fs = require("node:fs");
if (process.argv.includes("--version")) {
  process.stdout.write(JSON.stringify({version:"1.1.0",projectFormatVersion:1}));
} else {
  let input = "";
  process.stdin.setEncoding("utf8");
  process.stdin.on("data", chunk => input += chunk);
  process.stdin.on("end", () => (${onRequest})(input));
}
`;
  await writeFile(path, source);
  await chmod(path, 0o755);
  return path;
}

test("SDK retries a timed-out read but never replays a timed-out write", { skip: process.platform === "win32" }, async () => {
  const readCount = join(directory, "read-count");
  const readEngine = await createFakeEngine("retry-read", `input => {
    JSON.parse(input);
    const count = Number(fs.existsSync(${JSON.stringify(readCount)}) ? fs.readFileSync(${JSON.stringify(readCount)}, "utf8") : "0") + 1;
    fs.writeFileSync(${JSON.stringify(readCount)}, String(count));
    if (count === 1) return setTimeout(() => process.exit(0), 2000);
    process.stdout.write(JSON.stringify({ok:true,project:{version:1,name:"retried"}}));
  }`);
  const options = { binaryPath: readEngine, timeoutMs: 500, retry: { maxAttempts: 2, baseDelayMs: 0, maxDelayMs: 0, totalBudgetMs: 250 } };
  const db = new Rizzbase(join(directory, "retry.rizz"), "passphrase", options);
  assert.equal((await db.open()).name, "retried");
  assert.equal(Number(await readFile(readCount, "utf8")), 2);

  const writeCount = join(directory, "write-count");
  const writeEngine = await createFakeEngine("retry-write", `input => {
    const count = Number(fs.existsSync(${JSON.stringify(writeCount)}) ? fs.readFileSync(${JSON.stringify(writeCount)}, "utf8") : "0") + 1;
    fs.writeFileSync(${JSON.stringify(writeCount)}, String(count));
    setTimeout(() => process.exit(0), 2000);
  }`);
  const writer = new Rizzbase(join(directory, "retry-write.rizz"), "passphrase", {
    ...options,
    binaryPath: writeEngine,
  });
  await assert.rejects(writer.createLobby("no replay"), error => error.code === "ENGINE_TIMEOUT");
  assert.equal(Number(await readFile(writeCount, "utf8")), 1);

  const authCount = join(directory, "auth-count");
  const authEngine = await createFakeEngine("retry-auth", `input => {
    const count = Number(fs.existsSync(${JSON.stringify(authCount)}) ? fs.readFileSync(${JSON.stringify(authCount)}, "utf8") : "0") + 1;
    fs.writeFileSync(${JSON.stringify(authCount)}, String(count));
    process.stdout.write(JSON.stringify({ok:false,error:"No",errorCode:"AUTHENTICATION_FAILED"}));
  }`);
  const authReader = new Rizzbase(join(directory, "retry-auth.rizz"), "passphrase", { ...options, binaryPath: authEngine });
  await assert.rejects(authReader.open(), error => error.code === "AUTHENTICATION_FAILED");
  assert.equal(Number(await readFile(authCount, "utf8")), 1);
});

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
  assert.deepEqual((await db.createYaps(lobby.id, [
    { id: "four", data: { score: 10 } },
    { id: "five", data: { score: 12 } },
  ])).map(yap => yap.id), ["four", "five"]);
  await assert.rejects(db.createYaps(lobby.id, [
    { id: "six", data: true },
    { id: "one", data: false },
  ]), error => error.code === "YAP_EXISTS");
  await assert.rejects(db.createYaps(lobby.id, [
    { id: "duplicate", data: true },
    { id: "duplicate", data: false },
  ]), error => error.code === "INVALID_ARGUMENT");
  await assert.rejects(db.getYap(lobby.id, "six"), error => error.code === "YAP_NOT_FOUND");
  assert.deepEqual(await db.getYap(lobby.id, "one"), { score: 2, user: { active: true } });
  assert.deepEqual((await db.listYaps(lobby.id)).map(y => y.id), ["five", "four", "one", "three", "two"]);
  assert.deepEqual((await db.query(lobby.id).where("score", ">=", 2).where("user.active", "==", true).limit(1).exec()).map(y => y.id), ["one"]);
  assert.deepEqual((await db.queryYaps(lobby.id, { where: [{ field: "score", operator: ">", value: 5 }] })).map(y => y.id), ["five", "four", "three"]);
  assert.deepEqual((await db.query(lobby.id).orderBy("score").exec()).map(y => y.id), ["one", "two", "three", "four", "five"]);
  assert.deepEqual((await db.query(lobby.id)
    .where("score", ">=", 2)
    .orderBy("score", "desc")
    .limit(2)
    .exec()).map(y => y.id), ["five", "four"]);
  assert.deepEqual((await db.queryYaps(lobby.id, {
    orderBy: { field: "score", direction: "desc" },
    limit: 1,
  })).map(y => y.id), ["five"]);
  const firstPage = await db.query(lobby.id).limit(2).execPage();
  assert.deepEqual(firstPage.yaps.map(y => y.id), ["five", "four"]);
  assert.notEqual(firstPage.nextCursor, "");
  const secondPage = await db.query(lobby.id).limit(2).after(firstPage.nextCursor).execPage();
  assert.deepEqual(secondPage.yaps.map(y => y.id), ["one", "three"]);
  await assert.rejects(db.queryYapPage(lobby.id, {
    where: [{ field: "score", operator: ">", value: 0 }],
    limit: 2,
    cursor: firstPage.nextCursor,
  }), error => error.code === "INVALID_ARGUMENT");
  await db.updateYap(lobby.id, "one", { score: 3 });
  assert.deepEqual(await db.getYap(lobby.id, "one"), { score: 3 });
  assert.deepEqual((await db.listLobbies()).map(l => l.id), [lobby.id]);
  const project = await db.open();
  assert.equal(project.name, "SDK test");
  const diagnostics = await db.doctor();
  assert.equal(diagnostics.exists, true);
  assert.equal(diagnostics.formatCompatible, true);
  assert.equal(diagnostics.projectValid, true);
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
  assert.throws(() => new Rizzbase(path, "secret", { retry: { maxAttempts: 0 } }), e => e.code === "INVALID_ARGUMENT");
  assert.throws(() => new Rizzbase(path, "secret", { retry: { stale: true } }), e => e.code === "INVALID_ARGUMENT");
  assert.throws(() => db.query("lobby").after("x".repeat(4097)), e => e.code === "INVALID_ARGUMENT");
  assert.throws(() => db.query("lobby").limit(-1), e => e.code === "INVALID_ARGUMENT");
  assert.throws(() => db.query("lobby").orderBy("score", "sideways"), e => e.code === "INVALID_ARGUMENT");
  await assert.rejects(db.queryYaps("lobby", { where: [{ field: "x", operator: "between", value: 1 }] }), e => e.code === "INVALID_ARGUMENT" && e.message.includes("Invalid query operator"));
  const circular = {}; circular.self = circular;
  await assert.rejects(db.createYap("lobby", "circular", circular), e => e.code === "INVALID_ARGUMENT");
  assert.equal((await (await import("node:fs/promises")).readFile(path, "utf8")).includes("never-echo-this"), false);
});

test("PROJECT_BUSY includes structured owner context and preserves the lock", async () => {
  const path = join(directory, "busy.rizz");
  const db = new Rizzbase(path, "passphrase", { binaryPath: binary, retry: { lockWaitMs: 0 } });
  await db.create("Busy");
  await (await import("node:fs/promises")).mkdir(`${path}.lock`);
  await writeFile(join(`${path}.lock`, "lock.json"), JSON.stringify({
    pid: process.pid,
    hostname: hostname(),
    startedAt: new Date().toISOString(),
    engineVersion: "1.1.0",
    token: "0123456789abcdef0123456789abcdef",
  }));
  await assert.rejects(db.createLobby("busy"), error =>
    error.code === "PROJECT_BUSY" &&
    error.context?.state === "active" &&
    error.context?.metadata?.pid === process.pid &&
    typeof error.context?.ageMs === "number");
  assert.equal((await readFile(join(`${path}.lock`, "lock.json"), "utf8")).includes("0123456789abcdef0123456789abcdef"), true);
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
