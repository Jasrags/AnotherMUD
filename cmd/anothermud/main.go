// Command anothermud is the MUD server entrypoint.
//
// Currently a no-op scaffold — real wiring (config load, pack discovery,
// network listeners, tick loop) lands as the substrate specs are implemented.
package main

import (
	"fmt"
	"os"
)

// version is set via -ldflags "-X main.version=..." by the Makefile.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "anothermud: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Printf("anothermud %s: scaffold — nothing to do yet\n", version)
	return nil
}
