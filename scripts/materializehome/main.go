// Command materializehome mirrors bundled assets into Atlas Home.
// It is not the installer: it does not publish a binary and it does not roll back.
// Local installation is `make install` (scripts/installlocal).
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
