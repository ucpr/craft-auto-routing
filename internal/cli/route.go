package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ucpr/craft-auto-routing/internal/config"
	"github.com/ucpr/craft-auto-routing/internal/craft"
	"github.com/ucpr/craft-auto-routing/internal/router"
)

func newRouteCmd() *cobra.Command {
	var (
		dryRun         bool
		verbose        bool
		force          bool
		location       string
		minConfidence  float64
		limit          int
		sampleSize     int
		maxFolders     int
		maxContentRune int
		blocksMaxDepth int
		folderHints    []string
	)

	cmd := &cobra.Command{
		Use:   "route",
		Short: "Classify unfiled documents and move them into the best-matching folder",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			craftClient := craft.NewClient(cfg.CraftBaseURL, cfg.CraftAPIToken)
			tsClient := typesafeClient(cfg)

			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			fmt.Fprintln(out, "Reading existing folder structure...")
			tree, err := craftClient.ListFolders(ctx)
			if err != nil {
				return fmt.Errorf("list folders: %w", err)
			}
			flat := router.FlattenFolders(tree)
			if len(flat) == 0 {
				return fmt.Errorf("no folders found in this Craft space; nothing to route into")
			}
			flat = router.LimitByDocumentCount(flat, maxFolders)

			catalog, err := router.BuildCatalog(ctx, craftClient, flat, sampleSize)
			if err != nil {
				return fmt.Errorf("build folder catalog: %w", err)
			}

			hints, err := parseFolderHints(folderHints)
			if err != nil {
				return err
			}
			catalog = router.ApplyHints(catalog, hints)

			fmt.Fprintf(out, "Found %d candidate folder(s).\n", len(catalog))

			docs, err := craftClient.ListDocuments(ctx, craft.ListDocumentsParams{Location: location})
			if err != nil {
				return fmt.Errorf("list %s documents: %w", location, err)
			}
			if limit > 0 && len(docs) > limit {
				docs = docs[:limit]
			}
			fmt.Fprintf(out, "Found %d document(s) in %q to route.\n", len(docs), location)

			opts := router.Options{
				DryRun:         dryRun,
				Force:          force,
				MinConfidence:  minConfidence,
				MaxContentRune: maxContentRune,
				BlocksMaxDepth: blocksMaxDepth,
			}

			var moved, skipped, failed int
			for _, doc := range docs {
				decision := router.RouteDocument(ctx, craftClient, tsClient, doc, catalog, opts)

				forcedNote := ""
				if decision.Forced {
					forcedNote = ", forced despite low confidence"
				}

				switch {
				case decision.Err != nil:
					failed++
					fmt.Fprintf(out, "ERROR  %s: %v\n", doc.Title, decision.Err)
				case decision.Skipped:
					skipped++
					fmt.Fprintf(out, "SKIP   %s -> %s (%s)\n", doc.Title, decision.Folder.Path, decision.SkipReason)
					printCandidates(out, catalog, decision.Probabilities)
				case dryRun:
					fmt.Fprintf(out, "PLAN   %s -> %s (confidence %.2f%s)\n", doc.Title, decision.Folder.Path, decision.Confidence, forcedNote)
					moved++
				default:
					moved++
					fmt.Fprintf(out, "MOVED  %s -> %s (confidence %.2f%s)\n", doc.Title, decision.Folder.Path, decision.Confidence, forcedNote)
				}

				if verbose {
					printVerbose(out, catalog, decision)
				}
			}

			verb := "moved"
			if dryRun {
				verb = "planned"
			}
			fmt.Fprintf(out, "\nDone: %d %s, %d skipped, %d failed.\n", moved, verb, skipped, failed)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "classify and print the plan without moving any documents")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "print the exact text sent to the classifier and candidate folder descriptions, for debugging")
	cmd.Flags().BoolVar(&force, "force", false, "always route to the highest-probability folder, ignoring --min-confidence")
	cmd.Flags().StringVar(&location, "location", "unsorted", "Craft location to pull documents from (unsorted, trash, templates, daily_notes)")
	cmd.Flags().Float64Var(&minConfidence, "min-confidence", 0.6, "minimum classifier confidence required to move a document (ignored with --force)")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of documents to process (0 = no limit)")
	cmd.Flags().IntVar(&sampleSize, "sample-size", 5, "number of example document titles to sample per folder for context")
	cmd.Flags().IntVar(&maxFolders, "max-folders", 0, "cap the candidate folder list to the N folders with the most documents (0 = no cap)")
	cmd.Flags().IntVar(&maxContentRune, "max-content-chars", 4000, "maximum characters of document content sent to the classifier")
	cmd.Flags().IntVar(&blocksMaxDepth, "blocks-max-depth", 3, "maximum block nesting depth fetched from a document")
	cmd.Flags().StringArrayVar(&folderHints, "folder-hint", nil,
		`steer classification for one folder, as "<folder path or id>=<hint text>" (repeatable); `+
			`the hint is appended to that folder's description sent to the classifier, e.g. `+
			`--folder-hint "FleetingNotes=Only choose this as a last resort if no other folder fits" `+
			`or --folder-hint "Engineering/Research=Prefer this folder for anything about tools, libraries, or infrastructure"`)

	return cmd
}

// printCandidates prints the top-scoring folders behind a classification, so
// a low-confidence skip can be explained (e.g. several folders scored close
// together, rather than one folder clearly fitting).
func printCandidates(out io.Writer, catalog []router.FolderOption, probabilities map[string]float64) {
	for _, c := range router.TopCandidates(catalog, probabilities, 3) {
		fmt.Fprintf(out, "         %5.1f%%  %s\n", c.Probability*100, c.Folder.Path)
	}
}

// printVerbose dumps the exact document text sent to the classifier plus the
// folder descriptions behind the top candidates, so a surprising decision
// (e.g. the "obviously right" folder losing to something else) can be
// diagnosed without re-running against the live API.
func printVerbose(out io.Writer, catalog []router.FolderOption, decision router.Decision) {
	fmt.Fprintln(out, "       --- state sent to classifier ---")
	fmt.Fprintln(out, indent(decision.State, "       "))
	fmt.Fprintln(out, "       --- candidate folder descriptions ---")
	for _, c := range router.TopCandidates(catalog, decision.Probabilities, 3) {
		fmt.Fprintf(out, "       [%5.1f%%] %s\n", c.Probability*100, c.Folder.Description())
	}
	fmt.Fprintln(out)
}

func indent(text, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
