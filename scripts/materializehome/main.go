// Command materializehome ensures Atlas Home layout and mirrors bundled assets.
// Used by `make install` so a clean install leaves ATLAS_HOME machine-ready.
//
// Respects ATLAS_HOME when set; otherwise uses ~/.atlas.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func main() {
	result, err := home.EnsureAndMirror(time.Now().UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "materializehome: %v\n", err)
		os.Exit(1)
	}
	action := "refreshed"
	if result.Created {
		action = "created"
	}
	fmt.Printf("atlas home %s at %s (%d assets)\n", action, result.HomePath, len(result.Mirrored))
}
