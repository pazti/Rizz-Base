import { cp, mkdir, rm } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const frontend = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(frontend, "../..");
const studio = path.resolve(frontend, "..");
const windowsBuild = path.join(studio, "build", "windows");
const vite = path.join(root, "node_modules", "vite", "bin", "vite.js");
const result = spawnSync(process.execPath, [vite, "build", "--mode", "studio"], { cwd: root, stdio: "inherit" });

if (result.error) throw result.error;
if (result.status !== 0) process.exit(result.status ?? 1);

const bundledAssets = path.join(frontend, "dist");
await rm(bundledAssets, { recursive: true, force: true });
await cp(path.join(root, "dist"), bundledAssets, { recursive: true });
await mkdir(windowsBuild, { recursive: true });
await cp(path.join(root, "public", "favicon-512.png"), path.join(studio, "build", "appicon.png"));
await cp(path.join(studio, "windows", "info.json"), path.join(windowsBuild, "info.json"));
await rm(path.join(windowsBuild, "icon.ico"), { force: true });
