package main

import (
	"fmt"
	"os"

	"github.com/tankibaj/surfy/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "surfy:", err)
		os.Exit(1)
	}
}
