package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstall_SuccessPublishesAfterHome(t *testing.T) {
	prefix := t.TempDir()
	binSrc := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(binSrc, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var materialized bool
	err := Install(binSrc, prefix, func() error {
		materialized = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !materialized {
		t.Fatal("expected materialize before publish")
	}
	final := filepath.Join(prefix, "bin", "atlas")
	data, err := os.ReadFile(final)
	if err != nil || !strings.Contains(string(data), "echo ok") {
		t.Fatalf("final binary = %q err=%v", data, err)
	}
	assertNoStageLeft(t, filepath.Join(prefix, "bin"))
}

func TestInstall_HomeFailurePreservesExistingBinary(t *testing.T) {
	prefix := t.TempDir()
	destDir := filepath.Join(prefix, "bin")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(destDir, "atlas")
	old := []byte("#!/bin/sh\necho old\n")
	if err := os.WriteFile(final, old, 0o755); err != nil {
		t.Fatal(err)
	}

	binSrc := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(binSrc, []byte("#!/bin/sh\necho new\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := Install(binSrc, prefix, func() error {
		return errors.New("home boom")
	})
	if err == nil || !strings.Contains(err.Error(), "home boom") {
		t.Fatalf("expected home failure, got %v", err)
	}
	got, err := os.ReadFile(final)
	if err != nil || string(got) != string(old) {
		t.Fatalf("final binary mutated: %q err=%v", got, err)
	}
	assertNoStageLeft(t, destDir)
}

func TestInstall_SecondInstallReplacesAfterHealthyHome(t *testing.T) {
	prefix := t.TempDir()
	writeBin := func(name, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := Install(writeBin("a1", "v1"), prefix, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := Install(writeBin("a2", "v2"), prefix, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(got) != "v2" {
		t.Fatalf("got %q err=%v", got, err)
	}
	assertNoStageLeft(t, filepath.Join(prefix, "bin"))
}

func assertNoStageLeft(t *testing.T, destDir string) {
	t.Helper()
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".atlas.new.") {
			t.Fatalf("leftover staged binary: %s", e.Name())
		}
	}
}
