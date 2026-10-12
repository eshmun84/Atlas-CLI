package main

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
)

func TestSecurityAssertionHelpers_RequireCEFailedAndMemoryCanary(t *testing.T) {
	ok := composeapp.Result{
		Response: contextcompose.Response{
			SchemaVersion: 1,
			Status:        contextcompose.StatusPartial,
			Count:         1,
			Sources: []contextcompose.SourceStatus{
				{Source: contextcompose.SourceMemory, Status: contextcompose.SourceStatusSupplied},
				{Source: contextcompose.SourceContextEconomy, Status: contextcompose.SourceStatusFailed},
			},
			Items: []contextcompose.Item{
				{Source: contextcompose.SourceMemory, Title: "x", Excerpt: canaryMemA},
			},
		},
	}
	if sourceOf(ok, contextcompose.SourceContextEconomy) != contextcompose.SourceStatusFailed {
		t.Fatal("ce")
	}
	if !hasSource(ok, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		t.Fatal("memory")
	}
	if !containsCanary(ok, canaryMemA) {
		t.Fatal("canary")
	}

	weak := ok
	weak.Sources = []contextcompose.SourceStatus{
		{Source: contextcompose.SourceMemory, Status: contextcompose.SourceStatusUnavailable},
		{Source: contextcompose.SourceContextEconomy, Status: contextcompose.SourceStatusFailed},
	}
	if hasSource(weak, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		t.Fatal("weak memory must not pass supplied assertion")
	}
}
