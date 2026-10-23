export type JsonPrimitive = string | number | boolean | null;
export type JsonValue = JsonPrimitive | JsonValue[] | { [key: string]: JsonValue };
export type Operator = "==" | "!=" | ">" | ">=" | "<" | "<=";
export type OrderDirection = "asc" | "desc";
export interface Yap<T = JsonValue> { id: string; data: T }
export interface Lobby {
  id: string;
  name: string;
  createdAt: string;
  yaps: Record<string, JsonValue>;
}
export interface Project {
  version: number;
  name: string;
  createdAt: string;
  lobbies: Record<string, Lobby>;
}
export interface Filter { field: string; operator: Operator; value: JsonValue }
export interface OrderBy { field: string; direction: OrderDirection }
export interface QueryOptions { where?: readonly Filter[]; orderBy?: OrderBy; limit?: number }
export interface RizzbaseOptions {
  /** Explicit trusted executable; takes precedence over RIZZBASE_BINARY and PATH. */
  binaryPath?: string;
  /** Per-process timeout, default 30 seconds. */
  timeoutMs?: number;
}
