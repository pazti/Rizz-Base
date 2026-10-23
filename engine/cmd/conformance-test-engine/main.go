package main

import (
	"encoding/json"
	"fmt"
	"os"
)

var mode = "protocol"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		_ = json.NewEncoder(os.Stdout).Encode(struct {
			Version              string `json:"version"`
			ProjectFormatVersion int    `json:"projectFormatVersion"`
		}{Version: "1.1.0", ProjectFormatVersion: 1})
		return
	}
	if mode == "nonzero" {
		fmt.Fprintln(os.Stdout, `{"ok":true}`)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "invalid engine response")
}
