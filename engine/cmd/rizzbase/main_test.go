package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.rizz")
	for _, operation := range []string{"create", "open"} {
		data, _ := json.Marshal(request{Operation: operation, Path: path, Name: "cli-demo", Passphrase: "test-only"})
		var out bytes.Buffer
		if err := run(bytes.NewReader(data), &out); err != nil {
			t.Fatal(err)
		}
		var res response
		if err := json.Unmarshal(out.Bytes(), &res); err != nil || !res.OK || res.Project.Name != "cli-demo" {
			t.Fatalf("bad response: %s", out.String())
		}
		if strings.Contains(out.String(), "test-only") {
			t.Fatal("passphrase leaked into response")
		}
	}
}

func TestDoctorOperation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doctor.rizz")
	createRequest, err := json.Marshal(request{Operation: "create", Path: path, Passphrase: "test-only", Name: "doctor"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(bytes.NewReader(createRequest), &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	structuralRequest, err := json.Marshal(request{Operation: "doctor", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := run(bytes.NewReader(structuralRequest), &out); err != nil {
		t.Fatal(err)
	}
	var res response
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Doctor == nil || !res.Doctor.Exists || !res.Doctor.FormatCompatible || res.Doctor.ProjectValid != nil {
		t.Fatalf("unexpected structural doctor response: %s", out.String())
	}
	out.Reset()
	fullRequest, err := json.Marshal(request{Operation: "doctor", Path: path, Passphrase: "test-only"})
	if err != nil {
		t.Fatal(err)
	}
	if err := run(bytes.NewReader(fullRequest), &out); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Doctor.ProjectValid == nil || !*res.Doctor.ProjectValid {
		t.Fatalf("expected full project validation: %s", out.String())
	}
}

func TestVersionCommand(t *testing.T) {
	previousVersion := version
	version = "1.2.3"
	t.Cleanup(func() { version = previousVersion })

	var output, errorOutput bytes.Buffer
	status := runCommand([]string{"--version"}, strings.NewReader("not JSON"), &output, &errorOutput)
	if status != 0 || errorOutput.Len() != 0 {
		t.Fatalf("version command status=%d stderr=%q", status, errorOutput.String())
	}
	var info struct {
		Version              string `json:"version"`
		ProjectFormatVersion int    `json:"projectFormatVersion"`
	}
	if err := json.Unmarshal(output.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Version != "1.2.3" || info.ProjectFormatVersion != projectFormatVersion {
		t.Fatalf("unexpected version output: %s", output.String())
	}
}

func TestCommandArguments(t *testing.T) {
	var output, errorOutput bytes.Buffer
	if status := runCommand([]string{"extra-arg"}, strings.NewReader(""), &output, &errorOutput); status != 2 {
		t.Fatalf("unexpected status for extra argument: %d", status)
	}
	if output.Len() != 0 || !strings.Contains(errorOutput.String(), "Usage:") {
		t.Fatalf("unexpected output: stdout=%q stderr=%q", output.String(), errorOutput.String())
	}
}

func TestBadRequests(t *testing.T) {
	for _, input := range []string{
		`{`, `{}`, `null`, `{"operation":"delete"}`, `{"unknown":1}`,
		`{"operation":"doctor","path":"test.rizz","lockWaitMs":-1}`,
		`{"operation":"create"} {}`, strings.Repeat("x", (64<<10)+1),
	} {
		var out bytes.Buffer
		if err := run(strings.NewReader(input), &out); err == nil {
			t.Fatalf("accepted bad request: %.50s", input)
		}
		if out.Len() != 0 {
			t.Fatal("success output on failure")
		}
	}
}

func TestAllOperationDispatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-created.rizz")
	requests := []request{
		{Operation: "lobby.create", Path: path, Name: "general", Passphrase: "test-only"},
		{Operation: "lobby.list", Path: path, Passphrase: "test-only"},
		{Operation: "lobby.delete", Path: path, LobbyID: "lobby", Passphrase: "test-only"},
		{Operation: "yap.create", Path: path, LobbyID: "lobby", YapID: "yap", Data: json.RawMessage(`true`), Passphrase: "test-only"},
		{Operation: "yap.get", Path: path, LobbyID: "lobby", YapID: "yap", Passphrase: "test-only"},
		{Operation: "yap.list", Path: path, LobbyID: "lobby", Passphrase: "test-only"},
		{Operation: "yap.update", Path: path, LobbyID: "lobby", YapID: "yap", Data: json.RawMessage(`true`), Passphrase: "test-only"},
		{Operation: "yap.delete", Path: path, LobbyID: "lobby", YapID: "yap", Passphrase: "test-only"},
	}
	for _, req := range requests {
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err = run(bytes.NewReader(data), &out)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s didn't reach storage: %v", req.Operation, err)
		}
	}
}
