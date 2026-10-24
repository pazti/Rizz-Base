export type ErrorCode =
  | "INVALID_ARGUMENT" | "BINARY_UNAVAILABLE" | "ENGINE_INCOMPATIBLE" | "ENGINE_TIMEOUT"
  | "ENGINE_PROTOCOL_ERROR" | "ENGINE_ERROR" | "AUTHENTICATION_FAILED"
  | "INVALID_FORMAT" | "PROJECT_BUSY" | "LOBBY_NOT_FOUND" | "YAP_NOT_FOUND"
  | "YAP_EXISTS" | "FILE_NOT_FOUND" | "FILE_EXISTS" | "PERMISSION_DENIED"
  | "INVALID_BACKUP" | "EXPORT_FAILED";

export class RizzbaseError extends Error {
  constructor(
    message: string,
    public readonly code: ErrorCode,
    options?: ErrorOptions,
    public readonly context?: Record<string, unknown>,
  ) {
    super(message, options);
    this.name = "RizzbaseError";
  }
}
