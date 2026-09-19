package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/ucpr/craft-auto-routing/internal/config"
	"github.com/ucpr/craft-auto-routing/internal/craft"
	"github.com/ucpr/craft-auto-routing/internal/router"
)

func newFoldersCmd() *cobra.Command {
	var (
		showSamples bool
		sampleSize  int
		folderHints []string
	)

	cmd := &cobra.Command{
		Use:   "folders",
		Short: "Show the existing Craft folder structure that documents get routed into, as a tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			craftClient := craft.NewClient(cfg.CraftBaseURL, cfg.CraftAPIToken)

			ctx := cmd.Context()
			tree, err := craftClient.ListFolders(ctx)
			if err != nil {
				return fmt.Errorf("list folders: %w", err)
			}
			tree = router.FilterSpecialFolders(tree)

			hints, err := parseFolderHints(folderHints)
			if err != nil {
				return err
			}
			flat := router.ApplyHints(router.FlattenFolders(tree), hints)
			hintsByID := make(map[string]string, len(flat))
			for _, f := range flat {
				if f.Hint != "" {
					hintsByID[f.ID] = f.Hint
				}
			}

			var samples map[string][]string
			if showSamples {
				catalog, err := router.BuildCatalog(ctx, craftClient, flat, sampleSize)
				if err != nil {
					return fmt.Errorf("build catalog: %w", err)
				}
				samples = make(map[string][]string, len(catalog))
				for _, f := range catalog {
					samples[f.ID] = f.SampleTitles
				}
			}

			printFolderTree(cmd.OutOrStdout(), tree, "", samples, hintsByID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&showSamples, "show-samples", false, "also fetch and show example document titles per folder")
	cmd.Flags().IntVar(&sampleSize, "sample-size", 5, "number of example document titles to show per folder when --show-samples is set")
	cmd.Flags().StringArrayVar(&folderHints, "folder-hint", nil,
		`preview a classification hint for one folder, as "<folder path or id>=<hint text>" (repeatable); `+
			`see "route --help" for details`)

	return cmd
}

// printFolderTree renders folders as a `tree`-style nested listing.
func printFolderTree(w io.Writer, nodes []craft.Folder, prefix string, samples map[string][]string, hints map[string]string) {
	for i, n := range nodes {
		last := i == len(nodes)-1
		connector, childPrefix := "├── ", prefix+"│   "
		if last {
			connector, childPrefix = "└── ", prefix+"    "
		}

		fmt.Fprintf(w, "%s%s%s [%s] (%d doc(s))\n", prefix, connector, n.Name, n.ID, n.DocumentCount)

		if h, ok := hints[n.ID]; ok {
			fmt.Fprintf(w, "%s    hint: %s\n", childPrefix, h)
		}
		for _, t := range samples[n.ID] {
			fmt.Fprintf(w, "%s    - %s\n", childPrefix, t)
		}

		printFolderTree(w, n.Folders, childPrefix, samples, hints)
	}
}
