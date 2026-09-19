// Command craft-auto-routing sorts unfiled Craft docs into existing folders
// using TypeSafe's jev model, per README.md.
package main

import (
	"fmt"
	"os"

	"github.com/ucpr/craft-auto-routing/internal/cli"
)

func main() {
	root := cli.NewRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
