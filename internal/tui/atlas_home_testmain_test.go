package tui_test

import (
	"os"
	"testing"
)

// Isolate Atlas Home for all TUI package tests so Discover/Status never touch ~/.atlas.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "atlas-home-tui-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("ATLAS_HOME", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
