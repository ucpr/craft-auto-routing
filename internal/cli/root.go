// Package cli wires up the craft-auto-routing command-line interface.
package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd builds the root cobra command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "craft-auto-routing",
		Short: "Automatically sort unfiled Craft docs into existing folders using jev",
		Long: "craft-auto-routing reads your Craft docs space's existing folder structure, " +
			"then uses TypeSafe's jev model to classify unsorted documents and file them " +
			"into the best-matching folder.",
	}

	root.AddCommand(newFoldersCmd())
	root.AddCommand(newRouteCmd())

	return root
}
