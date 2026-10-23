// Package testharness installs disposable doubles for the standard test suite.
//
// The OpenSpec program it places on PATH is a contract double. It answers the
// argv Atlas already probes and writes only the layout ClassifyProject and
// MissingToolProjections require. A passing standard suite does not claim that
// the installed OpenSpec CLI was exercised. That path is the openspec_live
// build tag.
package testharness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ContractOpenSpecVersion is the fixed version reported by the contract double.
// It is inside the Atlas support window and is not read from the operator's CLI.
const ContractOpenSpecVersion = "1.14.2"

// Options selects extra LookPath stubs. They are not executed by Atlas discovery.
type Options struct {
	// CursorStub adds a no-op "cursor" executable so TUI discovery does not
	// require the operator's Cursor installation. It does not add opencode:
	// several tests assert that an unselected OpenCode projection stays absent.
	CursorStub bool
}

// Installed is the disposable bin directory prepended to PATH.
type Installed struct {
	Dir     string
	Cleanup func()
}

// Install creates a private bin directory and prepends it to PATH.
// The returned cleanup removes that directory. TestMain calls it after m.Run.
func Install(opts Options) Installed {
	dir, err := os.MkdirTemp("", "atlas-contract-bin-")
	if err != nil {
		panic(err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := writeExecutable(filepath.Join(dir, "openspec"), openSpecScript()); err != nil {
		cleanup()
		panic(err)
	}
	if opts.CursorStub {
		if err := writeExecutable(filepath.Join(dir, "cursor"), "#!/bin/sh\nexit 0\n"); err != nil {
			cleanup()
			panic(err)
		}
	}
	path := os.Getenv("PATH")
	if path == "" {
		path = dir
	} else {
		path = dir + string(os.PathListSeparator) + path
	}
	if err := os.Setenv("PATH", path); err != nil {
		cleanup()
		panic(err)
	}
	return Installed{Dir: dir, Cleanup: cleanup}
}

func writeExecutable(path, body string) error {
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

func openSpecScript() string {
	return fmt.Sprintf(`#!/bin/sh
set -eu
cmd=${1:-}
if [ "$#" -gt 0 ]; then
  shift
fi

has_help=0
for arg in "$@"; do
  if [ "$arg" = "--help" ] || [ "$arg" = "-h" ]; then
    has_help=1
  fi
done

case "$cmd" in
version)
  printf '%%s\n' '{"version":"%s"}'
  ;;
list)
  printf '%%s\n' '{"root":"."}'
  ;;
init)
  if [ "$has_help" -eq 1 ]; then
    printf '%%s\n' 'openspec init --tools <tools> --no-animation'
    exit 0
  fi
  tools=none
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --tools)
        shift
        tools=$1
        shift
        ;;
      --tools=*)
        tools=${1#--tools=}
        shift
        ;;
      *)
        shift
        ;;
    esac
  done
  if [ -L openspec ] || [ -L .cursor ] || [ -L .opencode ]; then
    printf '%%s\n' 'contract openspec: refusing symlink provider root' >&2
    exit 1
  fi
  mkdir -p openspec/specs openspec/changes/archive
  printf '%%s\n' 'schema: spec-driven' > openspec/config.yaml
  old_ifs=$IFS
  IFS=,
  for tool in $tools; do
    case "$tool" in
      cursor)
        mkdir -p .cursor/commands .cursor/skills/openspec-propose
        printf '%%s\n' 'contract-openspec-marker' > .cursor/commands/opsx-propose.md
        printf '%%s\n' 'contract-openspec-marker' > .cursor/skills/openspec-propose/SKILL.md
        ;;
      opencode)
        mkdir -p .opencode/commands .opencode/skills/openspec-propose
        printf '%%s\n' 'contract-openspec-marker' > .opencode/commands/opsx-propose.md
        printf '%%s\n' 'contract-openspec-marker' > .opencode/skills/openspec-propose/SKILL.md
        ;;
    esac
  done
  IFS=$old_ifs
  ;;
update)
  printf '%%s\n' 'openspec update --json'
  ;;
status|instructions|validate|archive)
  printf '%%s\n' "openspec $cmd --json"
  ;;
*)
  printf '%%s\n' '--tools --json'
  ;;
esac
`, ContractOpenSpecVersion)
}

// OnPath reports whether dir is one of the PATH entries.
func OnPath(dir string) bool {
	for _, entry := range strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)) {
		if entry == dir {
			return true
		}
	}
	return false
}
