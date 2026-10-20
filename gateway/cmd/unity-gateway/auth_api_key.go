package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"unity-gateway/gateway/internal/auth"
)

func cmdAuthAPIKey(args []string, input io.Reader, out, errOut io.Writer) int {
	if len(args) != 1 || (args[0] != "set" && args[0] != "remove") {
		fmt.Fprintln(errOut, "usage: unity-gateway auth api-key set < key-file | unity-gateway auth api-key remove")
		return 2
	}
	path, _ := resolveConfigPath()
	var mutationErr error
	if args[0] == "set" {
		const maxInput = 4096
		var raw []byte
		var err error
		if file, ok := input.(*os.File); ok && isTerminal(file) {
			raw, err = readAPIKeyTerminal(file, errOut, maxInput)
			if err != nil {
				fmt.Fprintf(errOut, "auth api-key: %v\n", err)
				return 1
			}
		} else {
			raw, err = io.ReadAll(io.LimitReader(input, maxInput+1))
		}
		if err != nil || len(raw) > maxInput {
			fmt.Fprintln(errOut, "auth api-key: could not read API key from stdin (maximum 4096 bytes)")
			return 1
		}
		mutationErr = auth.SaveSavedAPIKey(path, strings.TrimSpace(string(raw)))
		if mutationErr == nil {
			fmt.Fprintln(out, "Saved API key. Switch prioritizes it over Databricks CLI authentication.")
		}
	} else {
		mutationErr = auth.RemoveSavedAPIKey(path)
		if mutationErr == nil {
			fmt.Fprintln(out, "Removed saved API key. Switch uses Databricks CLI authentication when available.")
		}
	}
	if mutationErr != nil {
		fmt.Fprintf(errOut, "auth api-key: %v\n", mutationErr)
	}
	if state, pid := classifyPidfile(gatewayPidfilePath()); state == pidfileAlive {
		if err := signalRouter(pid); err != nil {
			fmt.Fprintln(errOut, "The router could not be reloaded. Send SIGHUP to reconcile its API key setting.")
			return 1
		}
		fmt.Fprintln(out, "Router reloaded (SIGHUP).")
	} else if mutationErr == nil {
		fmt.Fprintln(out, "Router not running; the setting applies on the next start.")
	}
	if mutationErr != nil {
		return 1
	}
	return 0
}
