import { spawn } from "node:child_process";
import { RizzbaseError, type ErrorCode } from "./errors.js";

// Largest CLI response: a full decoded project. File envelope is bounded at 64 MiB.
const MAX_OUTPUT = 64 << 20;
export async function callEngine(binary: string, request: object, timeoutMs: number): Promise<Record<string, unknown>> {
  const input = Buffer.from(JSON.stringify(request));
  if (input.length > 64 << 10) {
    throw new RizzbaseError("Engine requests cannot exceed 64 KiB.", "INVALID_ARGUMENT");
  }
  return new Promise((resolve, reject) => {
    // No shell, no passphrase in arguments or environment.
    const child = spawn(binary, [], { stdio: ["pipe", "pipe", "pipe"], windowsHide: true });
    const output: Buffer[] = [];
    let size = 0;
    let failure: RizzbaseError | undefined;
    const timer = setTimeout(() => {
      failure = new RizzbaseError("Engine request timed out; inspect the project lock before retrying writes.", "ENGINE_TIMEOUT");
      child.kill();
    }, timeoutMs);
    child.stdout.on("data", (chunk: Buffer) => {
      size += chunk.length;
      if (size > MAX_OUTPUT) {
        failure = new RizzbaseError("Engine response exceeded its size limit.", "ENGINE_PROTOCOL_ERROR");
        child.kill();
      } else output.push(chunk);
    });
    // Never include stderr or stdin in errors: they may contain private values.
    child.stderr.resume();
    child.stdin.on("error", () => { /* close/error below reports process failure */ });
    child.on("error", () => {
      clearTimeout(timer);
      reject(new RizzbaseError("Cannot start engine; check binaryPath, permissions and platform.", "BINARY_UNAVAILABLE"));
    });
    child.on("close", (status) => {
      clearTimeout(timer);
      input.fill(0);
      if (failure) return reject(failure);
      let response: Record<string, unknown>;
      try {
        const parsed: unknown = JSON.parse(Buffer.concat(output).toString("utf8"));
        if (!parsed || typeof parsed !== "object" || typeof (parsed as Record<string, unknown>).ok !== "boolean") throw new Error();
        response = parsed as Record<string, unknown>;
      } catch {
        return reject(new RizzbaseError("Engine returned an invalid response.", "ENGINE_PROTOCOL_ERROR"));
      }
      if (response.ok === false) {
        const codes: string[] = ["INVALID_ARGUMENT", "AUTHENTICATION_FAILED", "INVALID_FORMAT", "PROJECT_BUSY", "LOBBY_NOT_FOUND", "YAP_NOT_FOUND", "YAP_EXISTS", "FILE_NOT_FOUND", "FILE_EXISTS", "PERMISSION_DENIED", "INVALID_BACKUP", "ENGINE_ERROR"];
        const code = codes.includes(String(response.errorCode)) ? response.errorCode as ErrorCode : "ENGINE_ERROR";
        const context = response.errorContext && typeof response.errorContext === "object" && !Array.isArray(response.errorContext)
          ? response.errorContext as Record<string, unknown>
          : undefined;
        return reject(new RizzbaseError(typeof response.error === "string" ? response.error : "Engine operation failed.", code, undefined, context));
      }
      if (status !== 0) return reject(new RizzbaseError("Engine exited unsuccessfully.", "ENGINE_ERROR"));
      resolve(response);
    });
    child.stdin.end(input);
  });
}
