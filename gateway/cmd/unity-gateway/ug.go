package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unity-gateway/gateway/internal/ugruntime"
)

func cmdUG(args []string) int {
	if len(args) != 1 || (args[0] != "install" && args[0] != "status") {
		fmt.Fprintln(os.Stderr, "usage: unity-gateway ug install|status")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if args[0] == "install" {
		if err := ugruntime.Install(ctx); err != nil {
			return desktopError(err)
		}
	}
	result, err := ugruntime.Render(ctx, ugruntime.Request{Client: "check"})
	if err != nil {
		return desktopError(err)
	}
	return desktopError(json.NewEncoder(os.Stdout).Encode(map[string]any{"ready": true, "revision": result.Revision, "directory": ugruntime.Root()}))
}
