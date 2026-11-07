import { mkdtemp, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { performance } from "node:perf_hooks";
import { Rizzbase } from "../sdk/dist/index.js";

const binaryPath = process.env.RIZZBASE_BINARY;
if (!binaryPath) throw new Error("Set RIZZBASE_BINARY to the engine executable.");

const passphrase = "rizzbase-sdk-benchmark-only";
const counts = (process.env.RIZZBASE_BENCH_COUNTS ?? "100,1000,5000,10000,20000")
  .split(",")
  .map(value => Number(value));
const repetitions = Number(process.env.RIZZBASE_BENCH_REPETITIONS ?? 5);
if (counts.some(count => !Number.isSafeInteger(count) || count <= 0) ||
    !Number.isSafeInteger(repetitions) || repetitions <= 0) {
  throw new Error("Benchmark counts and repetitions must be positive integers.");
}

function projectBackup(count) {
  const yaps = {};
  for (let index = 0; index < count; index++) {
    yaps[`yap-${String(index).padStart(8, "0")}`] = {
      group: `group-${String(index % 20).padStart(2, "0")}`,
      score: index,
      payload: "x".repeat(128),
    };
  }
  return {
    version: 1,
    name: "SDK end-to-end benchmark",
    createdAt: "2025-01-01T00:00:00Z",
    lobbies: {
      "benchmark-lobby": {
        id: "benchmark-lobby",
        name: "Benchmark lobby",
        createdAt: "2025-01-01T00:00:00Z",
        yaps,
      },
    },
  };
}

async function measure(operation, action) {
  const samples = [];
  for (let index = 0; index < repetitions; index++) {
    const started = performance.now();
    await action(index);
    samples.push(performance.now() - started);
  }
  samples.sort((a, b) => a - b);
  return {
    operation,
    samplesMs: samples.map(sample => Number(sample.toFixed(3))),
    medianMs: Number(samples[Math.floor(samples.length / 2)].toFixed(3)),
    meanMs: Number((samples.reduce((sum, sample) => sum + sample, 0) / samples.length).toFixed(3)),
  };
}

const results = [];
for (const count of counts) {
  const directory = await mkdtemp(join(tmpdir(), "rizzbase-node-bench-"));
  const path = join(directory, `project-${count}.rizz`);
  const backupPath = join(directory, "seed.json");
  try {
    await writeFile(backupPath, `${JSON.stringify(projectBackup(count))}\n`, { mode: 0o600 });
    const db = new Rizzbase(path, passphrase, { binaryPath: resolve(binaryPath) });
    await db.importBackup(backupPath);
    const projectBytes = (await stat(path)).size;
    await db.open();

    const operations = [
      await measure("open", () => db.open()),
      await measure("ordered_query_page", () => db.queryYapPage("benchmark-lobby", {
        where: [{ field: "score", operator: ">=", value: 0 }],
        orderBy: { field: "score", direction: "desc" },
        limit: 100,
      })),
      await measure("update_one_yap", index => db.updateYap("benchmark-lobby", "yap-00000000", {
        group: "group-00",
        score: index,
        payload: "x".repeat(128),
      })),
    ];
    results.push({ sdk: "node", yaps: count, projectBytes, repetitions, operations });
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
}

console.log(JSON.stringify({
  runtime: process.version,
  platform: process.platform,
  architecture: process.arch,
  binaryPath: resolve(binaryPath),
  results,
}, null, 2));
