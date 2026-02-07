package main

import (
	"fmt"
	"os"

	"github.com/libersuite-org/panel/cmd/panel"
)

func main() {
	if err := panel.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
