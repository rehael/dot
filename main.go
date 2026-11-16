package main

import (
	"fmt"
	"os"

	"github.com/rehael/dot/internal/cli"
	"github.com/rehael/dot/internal/core"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "dot: %v\n", err)
		os.Exit(1)
	}
	os.Exit(cli.Run(os.Args[1:], core.Env{
		Getenv: os.Getenv,
		Cwd:    cwd,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}))
}
