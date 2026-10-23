import { access, mkdir } from "node:fs/promises";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { createInterface } from "node:readline/promises";
import { stdin, stdout } from "node:process";
import { Rizzbase, RizzbaseError } from "../sdk/dist/index.js";

export function promptSession() {
  return createInterface({ input: stdin, output: stdout });
}

export async function openExampleProject(fileName, projectName, lobbyName) {
  const passphrase = process.env.RIZZBASE_PASSPHRASE;
  if (!passphrase) {
    throw new Error("Set RIZZBASE_PASSPHRASE through a private environment or secret manager.");
  }

  const directory = resolve(
    process.env.RIZZBASE_EXAMPLES_DIR ?? join(homedir(), ".local", "share", "rizzbase", "examples"),
  );
  await mkdir(directory, { recursive: true, mode: 0o700 });

  const path = join(directory, fileName);
  const db = new Rizzbase(path, passphrase, {
    ...(process.env.RIZZBASE_BINARY ? { binaryPath: process.env.RIZZBASE_BINARY } : {}),
  });
  try {
    await access(path);
    await db.open();
  } catch (error) {
    if (error instanceof RizzbaseError) {
      if (error.code !== "FILE_NOT_FOUND") throw error;
    } else if (!(error instanceof Error && "code" in error && error.code === "ENOENT")) {
      throw error;
    }
    await db.create(projectName);
  }

  const existing = (await db.listLobbies()).find(lobby => lobby.name === lobbyName);
  const lobby = existing ?? await db.createLobby(lobbyName);
  return { db, lobby };
}

export function recordError(error) {
  console.error(error instanceof RizzbaseError ? `${error.code}: ${error.message}` : error.message);
  process.exitCode = 1;
}
