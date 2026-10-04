// Command caveman-blocks is the Caveman Blocks CLI and hook binary.
// The design lives in docs/; subcommands land per docs/ROADMAP.md phase 1.
package main

import (
	"fmt"
	"os"
)

var version = "0.0.0-dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println("caveman-blocks", version)
		return
	}
	fmt.Fprintln(os.Stderr, "caveman-blocks: foundation only; see docs/ROADMAP.md")
	os.Exit(2)
}
