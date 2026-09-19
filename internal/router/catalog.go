package router

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ucpr/craft-auto-routing/internal/craft"
)

// FolderOption is a single flattened, describable folder candidate.
type FolderOption struct {
	ID            string
	Path          string
	DocumentCount int
	SampleTitles  []string
	// Hint is user-supplied text appended to Description() to steer the
	// classifier's own judgment for this folder (e.g. "only choose this as
	// a last resort" or "prefer this for anything infra-related"). See
	// ApplyHints.
	Hint string
}

// Description renders a human-readable description used as the TypeSafe
// choice criteria value, so the model can judge fit from folder structure
// and example documents already filed there.
func (f FolderOption) Description() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Folder path: %s. Contains %d document(s).", f.Path, f.DocumentCount)
	if len(f.SampleTitles) > 0 {
		sb.WriteString(" Example existing documents: ")
		sb.WriteString(strings.Join(f.SampleTitles, "; "))
		sb.WriteString(".")
	}
	if f.Hint != "" {
		sb.WriteString(" Note: ")
		sb.WriteString(f.Hint)
	}
	return sb.String()
}

// ApplyHints returns a copy of folders with Hint set on any entry whose ID
// or Path matches a key in hints, so user-supplied routing guidance can be
// layered onto the catalog read from Craft's existing structure.
func ApplyHints(folders []FolderOption, hints map[string]string) []FolderOption {
	if len(hints) == 0 {
		return folders
	}
	out := make([]FolderOption, len(folders))
	for i, f := range folders {
		if h, ok := hints[f.ID]; ok {
			f.Hint = h
		} else if h, ok := hints[f.Path]; ok {
			f.Hint = h
		}
		out[i] = f
	}
	return out
}

// specialLocationIDs are pseudo-folders GET /folders returns alongside real
// folders (e.g. "Daily Notes" with id "daily_notes"). They aren't addressable
// via folderId and aren't sensible auto-routing targets, so they're excluded
// from the catalog.
var specialLocationIDs = map[string]bool{
	"unsorted":    true,
	"trash":       true,
	"templates":   true,
	"daily_notes": true,
}

// FilterSpecialFolders returns a copy of the folder tree with
// special-location pseudo-folders (see specialLocationIDs) removed at every
// level, preserving the real nesting of the remaining folders. Useful for
// tree-style rendering where the parent/child structure must be kept intact.
func FilterSpecialFolders(folders []craft.Folder) []craft.Folder {
	var out []craft.Folder
	for _, n := range folders {
		if specialLocationIDs[n.ID] {
			continue
		}
		n.Folders = FilterSpecialFolders(n.Folders)
		out = append(out, n)
	}
	return out
}

// FlattenFolders walks the (possibly nested) folder tree and returns every
// real folder with its full breadcrumb path, skipping special-location
// pseudo-folders (see specialLocationIDs).
func FlattenFolders(folders []craft.Folder) []FolderOption {
	var out []FolderOption
	var walk func(nodes []craft.Folder, parentPath string)
	walk = func(nodes []craft.Folder, parentPath string) {
		for _, n := range nodes {
			if specialLocationIDs[n.ID] {
				continue
			}
			path := n.Name
			if parentPath != "" {
				path = parentPath + "/" + n.Name
			}
			out = append(out, FolderOption{
				ID:            n.ID,
				Path:          path,
				DocumentCount: n.DocumentCount,
			})
			if len(n.Folders) > 0 {
				walk(n.Folders, path)
			}
		}
	}
	walk(folders, "")
	return out
}

// DocumentTitleFetcher fetches a sample of document titles for a folder.
type DocumentTitleFetcher interface {
	ListDocuments(ctx context.Context, params craft.ListDocumentsParams) ([]craft.Document, error)
}

// BuildCatalog enriches flattened folders with a sample of existing document
// titles, so the classifier can infer each folder's purpose from what is
// already filed there. Folders are fetched sequentially to stay within the
// Craft API rate limits.
func BuildCatalog(ctx context.Context, client DocumentTitleFetcher, folders []FolderOption, sampleSize int) ([]FolderOption, error) {
	out := make([]FolderOption, 0, len(folders))
	for _, f := range folders {
		if sampleSize > 0 && f.DocumentCount > 0 {
			docs, err := client.ListDocuments(ctx, craft.ListDocumentsParams{FolderID: f.ID})
			if err != nil {
				return nil, fmt.Errorf("sample documents for folder %q: %w", f.Path, err)
			}
			for i, d := range docs {
				if i >= sampleSize {
					break
				}
				if d.Title != "" {
					f.SampleTitles = append(f.SampleTitles, d.Title)
				}
			}
		}
		out = append(out, f)
	}
	return out, nil
}

// LimitByDocumentCount caps the catalog to the N folders with the most
// documents, keeping the classifier's option set (and token usage) bounded
// for spaces with very large folder trees. max <= 0 disables the cap.
func LimitByDocumentCount(folders []FolderOption, max int) []FolderOption {
	if max <= 0 || len(folders) <= max {
		return folders
	}
	sorted := make([]FolderOption, len(folders))
	copy(sorted, folders)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].DocumentCount > sorted[j].DocumentCount
	})
	return sorted[:max]
}

// ToCriteria renders the catalog as a TypeSafe choice criteria map keyed by
// folder id.
func ToCriteria(folders []FolderOption) map[string]string {
	criteria := make(map[string]string, len(folders))
	for _, f := range folders {
		criteria[f.ID] = f.Description()
	}
	return criteria
}

// FindByID returns the folder option with the given id, if present.
func FindByID(folders []FolderOption, id string) (FolderOption, bool) {
	for _, f := range folders {
		if f.ID == id {
			return f, true
		}
	}
	return FolderOption{}, false
}

// Candidate pairs a folder option with the classifier's probability for it,
// used to explain why a classification landed where it did (e.g. a low
// confidence because several folders scored close together).
type Candidate struct {
	Folder      FolderOption
	Probability float64
}

// TopCandidates returns up to n folders from catalog with the highest
// classifier probability, sorted descending. n <= 0 returns all of them.
func TopCandidates(catalog []FolderOption, probabilities map[string]float64, n int) []Candidate {
	candidates := make([]Candidate, 0, len(probabilities))
	for id, p := range probabilities {
		folder, ok := FindByID(catalog, id)
		if !ok {
			continue
		}
		candidates = append(candidates, Candidate{Folder: folder, Probability: p})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Probability > candidates[j].Probability
	})
	if n > 0 && len(candidates) > n {
		candidates = candidates[:n]
	}
	return candidates
}
