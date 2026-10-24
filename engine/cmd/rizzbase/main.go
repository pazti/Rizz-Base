// The CLI accepts one JSON request on stdin and prints one JSON response.
// Passphrases are never accepted as command-line arguments or logged.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"rizzbase/engine/core"
	rizzcrypto "rizzbase/engine/crypto"
	"rizzbase/engine/storage"
)

var version = "dev"

const projectFormatVersion = 1

func init() {
	storage.EngineVersion = version
}

type request struct {
	Operation  string            `json:"operation"`
	Path       string            `json:"path"`
	BackupPath string            `json:"backupPath,omitempty"`
	LobbyID    string            `json:"lobbyId,omitempty"`
	YapID      string            `json:"yapId,omitempty"`
	Name       string            `json:"name,omitempty"`
	Data       json.RawMessage   `json:"data,omitempty"`
	Yaps       []core.YapInput   `json:"yaps,omitempty"`
	Passphrase string            `json:"passphrase"`
	LockWaitMs int               `json:"lockWaitMs,omitempty"`
	Query      core.QueryOptions `json:"query,omitempty"`
}

type response struct {
	OK           bool                 `json:"ok"`
	Project      *core.Project        `json:"project,omitempty"`
	Lobby        *core.Lobby          `json:"lobby,omitempty"`
	Lobbies      []core.Lobby         `json:"lobbies,omitempty"`
	Yap          *core.Yap            `json:"yap,omitempty"`
	Yaps         []core.Yap           `json:"yaps,omitempty"`
	Page         *core.QueryPage      `json:"page,omitempty"`
	Data         json.RawMessage      `json:"data,omitempty"`
	Doctor       *storage.Diagnostics `json:"doctor,omitempty"`
	Error        string               `json:"error,omitempty"`
	ErrorCode    string               `json:"errorCode,omitempty"`
	ErrorContext *storage.LockInfo    `json:"errorContext,omitempty"`
}

func run(input io.Reader, output io.Writer) error {
	// Read the entire bounded request before executing, so trailing data cannot
	// produce an unintended create operation.
	data, err := io.ReadAll(io.LimitReader(input, (64<<10)+1))
	if err != nil {
		return err
	}
	defer clear(data)
	if len(data) > 64<<10 {
		return errors.New("request exceeds 64 KiB")
	}
	var req request
	// Unmarshal rejects trailing JSON. Reject unknown properties separately.
	if err := json.Unmarshal(data, &req); err != nil {
		return errors.New("invalid JSON request")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return errors.New("invalid JSON request")
	}
	for field := range fields {
		switch field {
		case "operation", "path", "backupPath", "lobbyId", "yapId", "name", "data", "yaps", "passphrase", "lockWaitMs", "query":
		default:
			return errors.New("unknown request field")
		}
	}
	if req.LockWaitMs < 0 || req.LockWaitMs > 60_000 {
		return errors.New("lockWaitMs must be between 0 and 60000")
	}
	res := response{OK: true}
	switch req.Operation {
	case "create":
		res.Project, err = core.CreateProject(req.Path, req.Name, req.Passphrase)
	case "open":
		res.Project, err = core.OpenProject(req.Path, req.Passphrase)
	case "project.import":
		res.Project, err = core.ImportProject(req.Path, req.Passphrase, req.BackupPath)
	case "doctor":
		diagnostics := core.Diagnose(req.Path, req.Passphrase)
		res.Doctor = &diagnostics
	case "lobby.create":
		var lobby core.Lobby
		lobby, err = core.CreateLobby(req.Path, req.Passphrase, req.Name, time.Duration(req.LockWaitMs)*time.Millisecond)
		res.Lobby = &lobby
	case "lobby.list":
		res.Lobbies, err = core.ListLobbies(req.Path, req.Passphrase)
	case "lobby.delete":
		err = core.DeleteLobby(req.Path, req.Passphrase, req.LobbyID, time.Duration(req.LockWaitMs)*time.Millisecond)
	case "yap.create":
		if len(req.Data) == 0 || !json.Valid(req.Data) {
			return errors.New("yap.create requires valid JSON data")
		}
		err = core.CreateYap(req.Path, req.Passphrase, req.LobbyID, req.YapID, req.Data, time.Duration(req.LockWaitMs)*time.Millisecond)
		if err == nil {
			res.Yap = &core.Yap{ID: req.YapID, Data: req.Data}
		}
	case "yap.bulkCreate":
		res.Yaps, err = core.CreateYaps(req.Path, req.Passphrase, req.LobbyID, req.Yaps, time.Duration(req.LockWaitMs)*time.Millisecond)
	case "yap.get":
		res.Data, err = core.GetYap(req.Path, req.Passphrase, req.LobbyID, req.YapID)
	case "yap.list":
		res.Yaps, err = core.ListYaps(req.Path, req.Passphrase, req.LobbyID)
	case "yap.query":
		var page core.QueryPage
		page, err = core.QueryYapPage(req.Path, req.Passphrase, req.LobbyID, req.Query)
		if err == nil {
			res.Yaps = page.Yaps
			res.Page = &page
		}
	case "yap.update":
		if len(req.Data) == 0 || !json.Valid(req.Data) {
			return errors.New("yap.update requires valid JSON data")
		}
		err = core.UpdateYap(req.Path, req.Passphrase, req.LobbyID, req.YapID, req.Data, time.Duration(req.LockWaitMs)*time.Millisecond)
		if err == nil {
			res.Yap = &core.Yap{ID: req.YapID, Data: req.Data}
		}
	case "yap.delete":
		err = core.DeleteYap(req.Path, req.Passphrase, req.LobbyID, req.YapID, time.Duration(req.LockWaitMs)*time.Millisecond)
	default:
		return errors.New("unsupported engine operation")
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(res)
}

func main() {
	os.Exit(runCommand(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func runCommand(args []string, input io.Reader, output, errorOutput io.Writer) int {
	if len(args) == 1 && args[0] == "--version" {
		if err := json.NewEncoder(output).Encode(struct {
			Version              string `json:"version"`
			ProjectFormatVersion int    `json:"projectFormatVersion"`
		}{Version: version, ProjectFormatVersion: projectFormatVersion}); err != nil {
			fmt.Fprintln(errorOutput, "Could not write version output.")
			return 1
		}
		return 0
	}
	if len(args) == 1 && args[0] == "--help" {
		fmt.Fprintln(output, "Usage: rizzbase < request.json")
		fmt.Fprintln(output, "       rizzbase --version")
		return 0
	}
	if len(args) != 0 {
		fmt.Fprintln(errorOutput, "Usage: rizzbase < request.json (or rizzbase --version)")
		return 2
	}
	if err := run(input, output); err != nil {
		res := response{OK: false, Error: err.Error(), ErrorCode: errorCode(err)}
		var busy *storage.BusyError
		if errors.As(err, &busy) {
			res.ErrorContext = &busy.Lock
		}
		_ = json.NewEncoder(output).Encode(res)
		return 1
	}
	return 0
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, rizzcrypto.ErrAuthentication):
		return "AUTHENTICATION_FAILED"
	case errors.Is(err, rizzcrypto.ErrFormat):
		return "INVALID_FORMAT"
	case errors.Is(err, storage.ErrBusy):
		return "PROJECT_BUSY"
	case errors.Is(err, core.ErrInvalidCursor):
		return "INVALID_ARGUMENT"
	case errors.Is(err, core.ErrLobbyNotFound):
		return "LOBBY_NOT_FOUND"
	case errors.Is(err, core.ErrYapNotFound):
		return "YAP_NOT_FOUND"
	case errors.Is(err, core.ErrInvalidBackup):
		return "INVALID_BACKUP"
	case errors.Is(err, core.ErrYapExists):
		return "YAP_EXISTS"
	case errors.Is(err, os.ErrNotExist):
		return "FILE_NOT_FOUND"
	case errors.Is(err, os.ErrExist):
		return "FILE_EXISTS"
	case errors.Is(err, os.ErrPermission):
		return "PERMISSION_DENIED"
	default:
		return "ENGINE_ERROR"
	}
}
