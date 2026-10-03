package main

import (
	"fmt"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/app"
)

func main() {
	application := app.New()
	if err := application.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
