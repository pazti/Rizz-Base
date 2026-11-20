export type JsonPrimitive = string | number | boolean | null;
export type JsonValue = JsonPrimitive | JsonValue[] | { [key: string]: JsonValue };
export type Operator = "==" | "!=" | ">" | ">=" | "<" | "<=";
export type OrderDirection = "asc" | "desc";
export interface Yap<T = JsonValue> { id: string; data: T }
export interface YapInput<T = JsonValue> { id: string; data: T }
export interface QueryPage<T = JsonValue> { yaps: Yap<T>[]; nextCursor: string }
export interface Lobby {
  id: string;
  name: string;
  createdAt: string;
  yaps: Record<string, JsonValue>;
}
export interface Project {
  version: number;
  projectId?: string;
  name: string;
  createdAt: string;
  lobbies: Record<string, Lobby>;
  events?: Event[];
  sync?: ProjectSync;
}
export interface Event<T = JsonValue> {
  id: string;
  kind: string;
  source?: string;
  createdAt: string;
  data: T;
}
export interface ProjectSync {
  ownerPublicKey: number[];
  membershipHistory: unknown[];
  operations: SyncOperation[];
  conflicts?: SyncConflict[];
}
export interface SyncIdentityInfo {
  actorId: string;
  signingPublicKey: number[];
  encryptionPublicKey: number[];
  identityEnvelope: number[];
}
export interface SyncOperation {
  projectId: string;
  membershipEpoch: number;
  id: { actor: string; sequence: number };
  key: string;
  clock: Record<string, number>;
  encryptedValue?: number[];
  deleted?: boolean;
  signature?: number[];
}
export interface SyncConflict {
  key: string;
  operations: SyncOperation[];
}
export interface LockMetadata {
  pid: number;
  hostname: string;
  startedAt: string;
  engineVersion: string;
  token: string;
}
export interface LockDiagnostics {
  present: boolean;
  state: "active" | "stale" | "unknown" | "";
  ageMs?: number;
  metadata?: LockMetadata;
  problem?: string;
}
export interface DoctorReport {
  exists: boolean;
  regular: boolean;
  readable: boolean;
  encodedSize: number;
  formatVersion: number;
  formatValid: boolean;
  permissions?: string;
  lock: LockDiagnostics;
  tempFiles: string[];
  engineVersion: string;
  formatCompatible: boolean;
  projectValid?: boolean;
  projectCheckReason?: string;
  problems: string[];
}
export interface Filter { field: string; operator: Operator; value: JsonValue }
export interface OrderBy { field: string; direction: OrderDirection }
export interface QueryOptions { where?: readonly Filter[]; orderBy?: OrderBy; limit?: number; cursor?: string }
export interface RetryOptions {
  /** Total calls for retry-safe read operations, including the first attempt. Default: 3. */
  maxAttempts?: number;
  /** Full-jitter exponential-backoff base. Default: 75 ms. */
  baseDelayMs?: number;
  /** Maximum backoff delay. Default: 1000 ms. */
  maxDelayMs?: number;
  /** Maximum additional retry time after the first timeout, including calls and backoff. Default: 2000 ms. */
  totalBudgetMs?: number;
  /** Maximum engine-side wait to acquire a write lock. Default: 1000 ms and less than timeoutMs. */
  lockWaitMs?: number;
}
export interface RizzbaseOptions {
  /** Explicit trusted executable; takes precedence over RIZZBASE_BINARY and PATH. */
  binaryPath?: string;
  /** Per-process timeout, default 30 seconds. */
  timeoutMs?: number;
  /** Safe retry and lock-wait policy. Writes are never replayed after timeout. */
  retry?: RetryOptions;
}
