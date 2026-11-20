package rizzbase

import "fmt"

type ErrorCode string

const (
	CodeInvalidArgument         ErrorCode = "INVALID_ARGUMENT"
	CodeBinaryUnavailable       ErrorCode = "BINARY_UNAVAILABLE"
	CodeEngineIncompatible      ErrorCode = "ENGINE_INCOMPATIBLE"
	CodeEngineTimeout           ErrorCode = "ENGINE_TIMEOUT"
	CodeEngineProtocolError     ErrorCode = "ENGINE_PROTOCOL_ERROR"
	CodeEngineError             ErrorCode = "ENGINE_ERROR"
	CodeAuthenticationFailed    ErrorCode = "AUTHENTICATION_FAILED"
	CodeInvalidFormat           ErrorCode = "INVALID_FORMAT"
	CodeProjectBusy             ErrorCode = "PROJECT_BUSY"
	CodeLobbyNotFound           ErrorCode = "LOBBY_NOT_FOUND"
	CodeYapNotFound             ErrorCode = "YAP_NOT_FOUND"
	CodeYapExists               ErrorCode = "YAP_EXISTS"
	CodeFileNotFound            ErrorCode = "FILE_NOT_FOUND"
	CodeFileExists              ErrorCode = "FILE_EXISTS"
	CodePermissionDenied        ErrorCode = "PERMISSION_DENIED"
	CodeInvalidBackup           ErrorCode = "INVALID_BACKUP"
	CodeInvalidCursor           ErrorCode = "INVALID_CURSOR"
	CodeExportFailed            ErrorCode = "EXPORT_FAILED"
	CodeSyncNotEnabled          ErrorCode = "SYNC_NOT_ENABLED"
	CodeSyncIdentityUnavailable ErrorCode = "SYNC_IDENTITY_UNAVAILABLE"
	CodeSyncAlreadyEnabled      ErrorCode = "SYNC_ALREADY_ENABLED"
	CodeSyncConflict            ErrorCode = "SYNC_CONFLICT"
	CodeSyncConflictNotFound    ErrorCode = "SYNC_CONFLICT_NOT_FOUND"
	CodeSyncInvalidState        ErrorCode = "SYNC_INVALID_STATE"
	CodeSyncHistoryLimit        ErrorCode = "SYNC_HISTORY_LIMIT"
	CodeSyncUnsupportedKey      ErrorCode = "SYNC_UNSUPPORTED_KEY"
	CodeSyncStaleMembership     ErrorCode = "SYNC_STALE_MEMBERSHIP"
	CodeSyncNotMember           ErrorCode = "SYNC_NOT_MEMBER"
	CodeSyncPermissionDenied    ErrorCode = "SYNC_PERMISSION_DENIED"
	CodeSyncInvalidMembership   ErrorCode = "SYNC_INVALID_MEMBERSHIP"
	CodeSyncInvalidSignature    ErrorCode = "SYNC_INVALID_SIGNATURE"
	CodeSyncInvalidOperation    ErrorCode = "SYNC_INVALID_OPERATION"
	CodeSyncOperationCollision  ErrorCode = "SYNC_OPERATION_COLLISION"
	CodeSyncMergeLimit          ErrorCode = "SYNC_MERGE_LIMIT"
	CodeSyncInvalidTransition   ErrorCode = "SYNC_INVALID_TRANSITION"
	CodeSyncInvalidCheckpoint   ErrorCode = "SYNC_INVALID_CHECKPOINT"
	CodeSyncInvalidValue        ErrorCode = "SYNC_INVALID_VALUE"
)

type Error struct {
	Code    ErrorCode
	Message string
	Context map[string]any
	Cause   error
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error { return e.Cause }

func invalid(message string) error {
	return &Error{Code: CodeInvalidArgument, Message: message}
}

func sdkError(code ErrorCode, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func errorf(code ErrorCode, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
