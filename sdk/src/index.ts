import { link, mkdtemp, rm, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { ensureCompatibleEngine, resolveBinary } from "./binary.js";
import { RizzbaseError } from "./errors.js";
import { callEngine } from "./transport.js";
import type { Filter, JsonValue, Lobby, Operator, OrderDirection, Project, QueryOptions, RizzbaseOptions, Yap } from "./types.js";
export { RizzbaseError } from "./errors.js";
export type { ErrorCode } from "./errors.js";
export type { Filter, JsonValue, JsonPrimitive, Lobby, Operator, OrderBy, OrderDirection, Project, QueryOptions, RizzbaseOptions, Yap } from "./types.js";

const MAX_BACKUP_SIZE = 64 << 20;

function invalid(message: string): never {
  throw new RizzbaseError(message, "INVALID_ARGUMENT");
}

function validateJson(value: unknown, ancestors = new Set<object>()): void {
  if (value === null || typeof value === "string" || typeof value === "boolean") return;
  if (typeof value === "number" && Number.isFinite(value)) return;
  if (typeof value !== "object" || value === null) invalid("Yaps and filter values must contain only JSON values.");
  if (ancestors.has(value)) invalid("Circular JSON values are not supported.");
  if (!Array.isArray(value) && Object.getPrototypeOf(value) !== Object.prototype && Object.getPrototypeOf(value) !== null) {
    invalid("Use plain JSON objects, not class instances.");
  }
  ancestors.add(value);
  if (Array.isArray(value)) {
    for (let i = 0; i < value.length; i++) validateJson(value[i], ancestors);
  } else {
    if (Object.getOwnPropertySymbols(value).length) invalid("JSON objects cannot have symbol keys.");
    for (const item of Object.values(value)) validateJson(item, ancestors);
  }
  ancestors.delete(value);
}

function validateQuery(options: QueryOptions): void {
  if (options.limit !== undefined && (!Number.isSafeInteger(options.limit) || options.limit < 0)) invalid("limit must be a non-negative safe integer.");
  if (options.where !== undefined && !Array.isArray(options.where)) invalid("where must be an array of filters.");
  for (const filter of options.where ?? []) {
    if (!filter || typeof filter.field !== "string" || filter.field.split(".").some((segment: string) => !segment.trim())) invalid("Query fields must have non-empty path segments.");
    if (!["==", "!=", ">", ">=", "<", "<="].includes(filter.operator)) invalid(`Invalid query operator: ${String(filter.operator)}.`);
    validateJson(filter.value);
  }
  if (options.orderBy !== undefined) {
    if (!options.orderBy || typeof options.orderBy.field !== "string" || options.orderBy.field.split(".").some(segment => !segment.trim())) {
      invalid("orderBy field must have non-empty path segments.");
    }
    if (options.orderBy.direction !== "asc" && options.orderBy.direction !== "desc") {
      invalid("orderBy direction must be asc or desc.");
    }
  }
}

function exportError(error: unknown): RizzbaseError {
  if (error instanceof RizzbaseError) return error;
  const code = error instanceof Error && "code" in error && typeof error.code === "string" ? error.code : "";
  if (code === "EEXIST") return new RizzbaseError("Backup destination already exists; choose a new file path.", "FILE_EXISTS", { cause: error });
  if (code === "ENOENT") return new RizzbaseError("Backup destination directory does not exist.", "FILE_NOT_FOUND", { cause: error });
  if (code === "EACCES" || code === "EPERM") return new RizzbaseError("Permission denied while writing the backup.", "PERMISSION_DENIED", { cause: error });
  return new RizzbaseError("Could not export the Project to a JSON backup.", "EXPORT_FAILED", { cause: error });
}

export class Rizzbase {
  readonly path: string;
  #passphrase: string;
  #options: RizzbaseOptions;
  // Serialize this instance's operations. A failed write doesn't poison the queue.
  #queue: Promise<unknown> = Promise.resolve();

  constructor(path: string, passphrase: string, options: RizzbaseOptions = {}) {
    if (typeof path !== "string" || !path.endsWith(".rizz")) invalid("Invalid project path: expected a .rizz file.");
    if (typeof passphrase !== "string" || !passphrase.length) invalid("Passphrase must not be empty.");
    if (options.timeoutMs !== undefined && (!Number.isSafeInteger(options.timeoutMs) || options.timeoutMs <= 0 || options.timeoutMs > 2_147_483_647)) invalid("timeoutMs must be a positive 32-bit integer.");
    this.path = resolve(path);
    this.#passphrase = passphrase;
    this.#options = { ...options };
  }

  private request(operation: string, fields: object = {}): Promise<Record<string, unknown>> {
    const job = this.#queue.then(async () => {
      const binary = await resolveBinary(this.#options);
      await ensureCompatibleEngine(binary);
      return callEngine(binary, { operation, path: this.path, passphrase: this.#passphrase, ...fields }, this.#options.timeoutMs ?? 30_000);
    });
    this.#queue = job.catch(() => {});
    return job;
  }

  async create(name: string): Promise<Project> { return (await this.request("create", { name })).project as Project; }
  async open(): Promise<Project> { return (await this.request("open")).project as Project; }
  async export(backupPath: string): Promise<void> {
    if (typeof backupPath !== "string" || !backupPath.endsWith(".json")) invalid("Invalid backup path: expected a .json file.");
    const destination = resolve(backupPath);
    if (destination === this.path) invalid("Backup path must differ from the Project path.");
    const project = await this.open();
    const data = Buffer.from(`${JSON.stringify(project, null, 2)}\n`);
    if (data.length > MAX_BACKUP_SIZE) {
      data.fill(0);
      throw new RizzbaseError("Project JSON backup exceeds the 64 MiB limit.", "EXPORT_FAILED");
    }
    let temporaryDirectory: string | undefined;
    try {
      temporaryDirectory = await mkdtemp(join(dirname(destination), ".rizzbase-backup-"));
      const temporary = join(temporaryDirectory, "backup.json");
      await writeFile(temporary, data, { mode: 0o600, flag: "wx" });
      await link(temporary, destination);
    } catch (error) {
      throw exportError(error);
    } finally {
      try {
        if (temporaryDirectory) await rm(temporaryDirectory, { recursive: true, force: true });
      } finally {
        data.fill(0);
      }
    }
  }
  async importBackup(backupPath: string): Promise<Project> {
    if (typeof backupPath !== "string" || !backupPath.endsWith(".json")) invalid("Invalid backup path: expected a .json file.");
    if (resolve(backupPath) === this.path) invalid("Backup path must differ from the Project path.");
    return (await this.request("project.import", { backupPath: resolve(backupPath) })).project as Project;
  }
  async createLobby(name: string): Promise<Lobby> { return (await this.request("lobby.create", { name })).lobby as Lobby; }
  async listLobbies(): Promise<Lobby[]> { return (await this.request("lobby.list")).lobbies as Lobby[] ?? []; }
  async deleteLobby(lobbyId: string): Promise<void> { await this.request("lobby.delete", { lobbyId }); }
  async createYap<T extends JsonValue>(lobbyId: string, yapId: string, data: T): Promise<Yap<T>> {
    validateJson(data);
    return (await this.request("yap.create", { lobbyId, yapId, data })).yap as Yap<T>;
  }
  async getYap<T = JsonValue>(lobbyId: string, yapId: string): Promise<T> {
    return (await this.request("yap.get", { lobbyId, yapId })).data as T;
  }
  async listYaps<T = JsonValue>(lobbyId: string): Promise<Yap<T>[]> {
    return (await this.request("yap.list", { lobbyId })).yaps as Yap<T>[] ?? [];
  }
  async updateYap<T extends JsonValue>(lobbyId: string, yapId: string, data: T): Promise<Yap<T>> {
    validateJson(data);
    return (await this.request("yap.update", { lobbyId, yapId, data })).yap as Yap<T>;
  }
  async deleteYap(lobbyId: string, yapId: string): Promise<void> { await this.request("yap.delete", { lobbyId, yapId }); }
  async queryYaps<T = JsonValue>(lobbyId: string, options: QueryOptions = {}): Promise<Yap<T>[]> {
    validateQuery(options);
    return (await this.request("yap.query", { lobbyId, query: options })).yaps as Yap<T>[] ?? [];
  }
  query<T = JsonValue>(lobbyId: string): YapQuery<T> { return new YapQuery<T>(this, lobbyId); }
}

/** Immutable builder: each where/limit returns a new query. */
export class YapQuery<T = JsonValue> {
  constructor(private readonly db: Rizzbase, private readonly lobbyId: string, private readonly options: QueryOptions = {}) {}
  where(field: string, operator: Operator, value: JsonValue): YapQuery<T> {
    const filter: Filter = { field, operator, value };
    const options = { ...this.options, where: [...(this.options.where ?? []), filter] };
    validateQuery(options);
    return new YapQuery<T>(this.db, this.lobbyId, options);
  }
  limit(n: number): YapQuery<T> {
    const options = { ...this.options, limit: n };
    validateQuery(options);
    return new YapQuery<T>(this.db, this.lobbyId, options);
  }
  orderBy(field: string, direction: OrderDirection = "asc"): YapQuery<T> {
    const options = { ...this.options, orderBy: { field, direction } };
    validateQuery(options);
    return new YapQuery<T>(this.db, this.lobbyId, options);
  }
  async exec(): Promise<Yap<T>[]> { return this.db.queryYaps<T>(this.lobbyId, this.options); }
}
