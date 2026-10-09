package mutatelock_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
)

func TestAcceptSelfOwned(t *testing.T) {
	if !mutatelock.AcceptSelfOwned(501, 501) {
		t.Fatal("same uid must accept")
	}
	if mutatelock.AcceptSelfOwned(0, 501) {
		t.Fatal("foreign uid must reject")
	}
	if mutatelock.AcceptSelfOwned(501, 0) {
		t.Fatal("foreign euid must reject")
	}
}

func TestAcceptCacheBaseMode(t *testing.T) {
	if !mutatelock.AcceptCacheBaseMode(0o755) {
		t.Fatal("0755 must accept (no group/other write)")
	}
	if !mutatelock.AcceptCacheBaseMode(0o700) {
		t.Fatal("0700 must accept")
	}
	if mutatelock.AcceptCacheBaseMode(0o775) {
		t.Fatal("0775 must reject (group write)")
	}
	if mutatelock.AcceptCacheBaseMode(0o757) {
		t.Fatal("0757 must reject (other write)")
	}
	if mutatelock.AcceptCacheBaseMode(0o777) {
		t.Fatal("0777 must reject")
	}
}
