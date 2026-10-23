import { spawn } from "node:child_process";
import { access, stat } from "node:fs/promises";
import { constants } from "node:fs";
import { delimiter, join, resolve } from "node:path";
import { RizzbaseError } from "./errors.js";
import type { RizzbaseOptions } from "./types.js";

const MIN_ENGINE_VERSION = [1, 1, 0] as const;
const MAX_ENGINE_MAJOR = 2;
const PROJECT_FORMAT_VERSION = 1;
const INSTALL_GUIDANCE = "Install the Rizzbase engine and set RIZZBASE_BINARY or add it to PATH.";
const checkedBinaries = new Map<string, Promise<void>>();

export async function resolveBinary(options: RizzbaseOptions): Promise<string> {
  if (options.binaryPath !== undefined) {
    const path = resolve(options.binaryPath);
    if (await isExecutable(path)) return path;
    throw unavailable(`binaryPath: ${path}`);
  }

  const configuredPath = process.env.RIZZBASE_BINARY;
  if (configuredPath) {
    const path = resolve(configuredPath);
    if (await isExecutable(path)) return path;
    throw unavailable(`RIZZBASE_BINARY: ${path}`);
  }

  const name = process.platform === "win32" ? "rizzbase.exe" : "rizzbase";
  const candidates = (process.env.PATH ?? "")
    .split(delimiter)
    .map(directory => resolve(directory || "."))
    .map(directory => join(directory, name));
  for (const candidate of candidates) {
    if (await isExecutable(candidate)) return candidate;
  }
  throw unavailable(`PATH: ${candidates.join(", ") || "(empty)"}`);
}

export async function ensureCompatibleEngine(binary: string): Promise<void> {
  const existing = checkedBinaries.get(binary);
  if (existing) return existing;
  const check = readAndValidateEngineVersion(binary);
  checkedBinaries.set(binary, check);
  try {
    await check;
  } catch (error) {
    if (checkedBinaries.get(binary) === check) checkedBinaries.delete(binary);
    throw error;
  }
}

export function validateEngineVersion(version: unknown, projectFormatVersion: unknown): void {
  const parts = typeof version === "string" && /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)
    ? version.split(".").map(Number)
    : undefined;
  const isSupported = parts !== undefined
    && parts.every(Number.isSafeInteger)
    && (parts[0]! > MIN_ENGINE_VERSION[0]
      || (parts[0] === MIN_ENGINE_VERSION[0]
        && (parts[1]! > MIN_ENGINE_VERSION[1]
          || (parts[1] === MIN_ENGINE_VERSION[1] && parts[2]! >= MIN_ENGINE_VERSION[2]))))
    && parts[0]! < MAX_ENGINE_MAJOR;
  if (!isSupported || projectFormatVersion !== PROJECT_FORMAT_VERSION) {
    throw new RizzbaseError(
      `Engine version or project format is incompatible. This SDK supports engine >=${MIN_ENGINE_VERSION.join(".")} <${MAX_ENGINE_MAJOR}.0.0 and project format ${PROJECT_FORMAT_VERSION}.`,
      "ENGINE_INCOMPATIBLE",
    );
  }
}

async function isExecutable(path: string): Promise<boolean> {
  try {
    if (!(await stat(path)).isFile()) return false;
    if (process.platform !== "win32") await access(path, constants.X_OK);
    return true;
  } catch (error) {
    if (error instanceof Error && "code" in error
      && ["ENOENT", "ENOTDIR", "EACCES", "EPERM"].includes(String(error.code))) return false;
    throw error;
  }
}

function unavailable(searched: string): RizzbaseError {
  return new RizzbaseError(`Rizzbase engine not found or not executable. Searched ${searched}. ${INSTALL_GUIDANCE}`, "BINARY_UNAVAILABLE");
}

async function readAndValidateEngineVersion(binary: string): Promise<void> {
  const output = await new Promise<string>((resolveOutput, reject) => {
    const child = spawn(binary, ["--version"], { stdio: ["ignore", "pipe", "ignore"], windowsHide: true });
    const chunks: Buffer[] = [];
    let size = 0;
    let failure: RizzbaseError | undefined;
    const timer = setTimeout(() => {
      failure = incompatible();
      child.kill();
    }, 5_000);
    child.stdout.on("data", (chunk: Buffer) => {
      size += chunk.length;
      if (size > 4096) {
        failure = incompatible();
        child.kill();
      } else {
        chunks.push(chunk);
      }
    });
    child.once("error", () => {
      clearTimeout(timer);
      reject(unavailable(`binaryPath: ${binary}`));
    });
    child.once("close", status => {
      clearTimeout(timer);
      if (failure) return reject(failure);
      if (status !== 0) return reject(incompatible());
      resolveOutput(Buffer.concat(chunks).toString("utf8"));
    });
  });

  try {
    const info: unknown = JSON.parse(output);
    if (!info || typeof info !== "object") throw incompatible();
    const versionInfo = info as Record<string, unknown>;
    validateEngineVersion(versionInfo.version, versionInfo.projectFormatVersion);
  } catch (error) {
    if (error instanceof RizzbaseError) throw error;
    throw incompatible();
  }
}

function incompatible(): RizzbaseError {
  return new RizzbaseError(
    "Could not verify engine compatibility. Use a Rizzbase engine version in the supported range and check its --version output.",
    "ENGINE_INCOMPATIBLE",
  );
}
