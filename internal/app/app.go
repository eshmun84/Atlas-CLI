package app

import (
	"io"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
)

// App is the Atlas CLI application entrypoint.
type App struct {
	Stdout io.Writer
	Stderr io.Writer
}

// New creates an App that writes to the process standard streams.
func New() *App {
	return &App{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// Run executes the CLI with the given args (without the program name).
func (a *App) Run(args []string) error {
	stdout := a.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := a.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	return cli.Execute(stdout, stderr, args)
}
