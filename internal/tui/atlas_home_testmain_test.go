package tui_test

import (
	"os"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

// Isolate Atlas Home for all TUI package tests so Discover/Status never touch ~/.atlas.
// Materialize a healthy Home so Init Apply tests match the Slice 1 install contract.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "atlas-home-tui-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("ATLAS_HOME", dir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
