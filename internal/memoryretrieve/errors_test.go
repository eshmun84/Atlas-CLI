package memoryretrieve

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestClassify_UsesSentinels(t *testing.T) {
	cases := []struct {
		err    error
		status string
	}{
		{memory.ErrDatabaseNotFound, StatusMissingMemoryDB},
		{fmt.Errorf("wrap: %w", memory.ErrInvalidSearchQuery), StatusInvalidQuery},
		{memory.ErrSearchQueryRequired, StatusInvalidQuery},
		{fmt.Errorf("%w: newer", memory.ErrSchemaIncompatible), StatusIncompatibleSchema},
		{memory.ErrSymlinkDatabase, StatusUnsafeFilesystem},
		{memory.ErrNotRegularDatabase, StatusUnsafeFilesystem},
		{fmt.Errorf("%w: join", memory.ErrUnsafePath), StatusUnsafeFilesystem},
		{ErrUnsafeFilesystem, StatusUnsafeFilesystem},
		{fmt.Errorf("%w: %w", ErrUnsafeFilesystem, fsafety.ErrSymlink), StatusUnsafeFilesystem},
		{fsafety.ErrSymlink, StatusUnsafeFilesystem},
		{fsafety.ErrPathTraversal, StatusUnsafeFilesystem},
		{fmt.Errorf("%w: %q", memory.ErrObservationNotFound, "x"), StatusNotFound},
		{ErrNotInitialized, StatusNotInitialized},
		{fmt.Errorf("%w: %w", ErrNotInitialized, os.ErrNotExist), StatusNotInitialized},
		{fmt.Errorf("load config: %w", os.ErrNotExist), StatusNotInitialized},
		{errors.New("parse config: yaml boom"), StatusInternalError},
	}
	for _, tc := range cases {
		status, _ := classify(tc.err)
		if status != tc.status {
			t.Fatalf("err=%v status=%s want %s", tc.err, status, tc.status)
		}
	}
}

func TestMapConfigLoadError(t *testing.T) {
	cases := []struct {
		name   string
		in     error
		wantIs error
	}{
		{"missing", fmt.Errorf("load config: %w", os.ErrNotExist), ErrNotInitialized},
		{"symlink", fmt.Errorf("load config: %w", fsafety.ErrSymlink), ErrUnsafeFilesystem},
		{"traversal", fmt.Errorf("load config: %w", fsafety.ErrPathTraversal), ErrUnsafeFilesystem},
		{"not regular", fmt.Errorf("load config: %w", fsafety.ErrNotRegularFile), ErrUnsafeFilesystem},
		{"corrupt", fmt.Errorf("parse config: yaml: line 1"), nil},
	}
	for _, tc := range cases {
		got := mapConfigLoadError(tc.in)
		if tc.wantIs == nil {
			if errors.Is(got, ErrNotInitialized) || errors.Is(got, ErrUnsafeFilesystem) {
				t.Fatalf("%s: unexpected sentinel wrap: %v", tc.name, got)
			}
			if !errors.Is(got, tc.in) && got.Error() != tc.in.Error() {
				t.Fatalf("%s: got %v want original", tc.name, got)
			}
			continue
		}
		if !errors.Is(got, tc.wantIs) {
			t.Fatalf("%s: got %v want Is(%v)", tc.name, got, tc.wantIs)
		}
	}
}
