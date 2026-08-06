package main

import (
	"fmt"
	"os"

	"github.com/tankibaj/server-them/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "serve-them:", err)
		os.Exit(1)
	}
}
