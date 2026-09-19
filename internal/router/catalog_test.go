package router

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ucpr/craft-auto-routing/internal/craft"
)

// TestFlattenFolders_BuildsBreadcrumbPathsAndSkipsSpecialFolders guards the
// breadcrumb construction (parent/child joined with "/") against a wrong
// separator or swapped parent/child order, and confirms special
// pseudo-folders (unsorted, trash, ...) never end up as routing candidates.
func TestFlattenFolders_BuildsBreadcrumbPathsAndSkipsSpecialFolders(t *testing.T) {
	tree := []craft.Folder{
		{ID: "unsorted", Name: "Unsorted", DocumentCount: 3},
		{
			ID: "f1", Name: "Projects", DocumentCount: 2,
			Folders: []craft.Folder{
				{ID: "f2", Name: "Work", DocumentCount: 5},
			},
		},
		{ID: "f3", Name: "Personal", DocumentCount: 1},
	}

	got := FlattenFolders(tree)

	want := []FolderOption{
		{ID: "f1", Path: "Projects", DocumentCount: 2},
		{ID: "f2", Path: "Projects/Work", DocumentCount: 5},
		{ID: "f3", Path: "Personal", DocumentCount: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("FlattenFolders() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Path != want[i].Path || got[i].DocumentCount != want[i].DocumentCount {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// fakeTitleFetcher is a DocumentTitleFetcher test double keyed by folder id.
type fakeTitleFetcher struct {
	byFolder map[string][]craft.Document
	calls    map[string]int
}

func newFakeTitleFetcher() *fakeTitleFetcher {
	return &fakeTitleFetcher{byFolder: map[string][]craft.Document{}, calls: map[string]int{}}
}

func (f *fakeTitleFetcher) ListDocuments(ctx context.Context, params craft.ListDocumentsParams) ([]craft.Document, error) {
	f.calls[params.FolderID]++
	if docs, ok := f.byFolder[params.FolderID]; ok {
		return docs, nil
	}
	return nil, errors.New("unexpected folder id: " + params.FolderID)
}

func TestBuildCatalog_SkipsSamplingEmptyFolders(t *testing.T) {
	fetcher := newFakeTitleFetcher()
	folders := []FolderOption{{ID: "empty", Path: "Empty", DocumentCount: 0}}

	got, err := BuildCatalog(context.Background(), fetcher, folders, 5)
	if err != nil {
		t.Fatalf("BuildCatalog() error: %v", err)
	}
	if fetcher.calls["empty"] != 0 {
		t.Errorf("ListDocuments was called for a folder with DocumentCount 0; that's a wasted API call")
	}
	if len(got[0].SampleTitles) != 0 {
		t.Errorf("SampleTitles = %v, want none for an empty folder", got[0].SampleTitles)
	}
}

func TestBuildCatalog_SkipsSamplingWhenSampleSizeIsZero(t *testing.T) {
	fetcher := newFakeTitleFetcher()
	folders := []FolderOption{{ID: "f1", Path: "F1", DocumentCount: 10}}

	_, err := BuildCatalog(context.Background(), fetcher, folders, 0)
	if err != nil {
		t.Fatalf("BuildCatalog() error: %v", err)
	}
	if fetcher.calls["f1"] != 0 {
		t.Errorf("ListDocuments was called even though sampleSize is 0")
	}
}

func TestBuildCatalog_CapsSamplesAtSampleSize(t *testing.T) {
	fetcher := newFakeTitleFetcher()
	fetcher.byFolder["f1"] = []craft.Document{
		{ID: "d1", Title: "One"},
		{ID: "d2", Title: "Two"},
		{ID: "d3", Title: "Three"},
		{ID: "d4", Title: "Four"},
	}
	folders := []FolderOption{{ID: "f1", Path: "F1", DocumentCount: 4}}

	got, err := BuildCatalog(context.Background(), fetcher, folders, 2)
	if err != nil {
		t.Fatalf("BuildCatalog() error: %v", err)
	}
	if len(got[0].SampleTitles) != 2 {
		t.Fatalf("SampleTitles = %v, want exactly 2 (sampleSize cap)", got[0].SampleTitles)
	}
	if len(got[0].SampleDocumentIDs) != len(got[0].SampleTitles) {
		t.Fatalf("SampleDocumentIDs (%v) is not index-aligned with SampleTitles (%v)", got[0].SampleDocumentIDs, got[0].SampleTitles)
	}
	if got[0].SampleDocumentIDs[0] != "d1" || got[0].SampleDocumentIDs[1] != "d2" {
		t.Errorf("SampleDocumentIDs = %v, want [d1 d2] to match the sampled titles", got[0].SampleDocumentIDs)
	}
}

func TestLimitByDocumentCount_KeepsTopNStably(t *testing.T) {
	folders := []FolderOption{
		{ID: "low", DocumentCount: 1},
		{ID: "tie-a", DocumentCount: 5},
		{ID: "tie-b", DocumentCount: 5},
		{ID: "high", DocumentCount: 10},
	}

	got := LimitByDocumentCount(folders, 2)

	if len(got) != 2 {
		t.Fatalf("LimitByDocumentCount() returned %d folders, want 2", len(got))
	}
	if got[0].ID != "high" {
		t.Errorf("got[0] = %q, want the highest document count first", got[0].ID)
	}
	if got[1].ID != "tie-a" {
		// SliceStable must break the tie by preserving original input order,
		// not just any order: otherwise which folder survives the cap
		// changes nondeterministically between runs.
		t.Errorf("got[1] = %q, want %q (stable tie-break on equal document counts)", got[1].ID, "tie-a")
	}
}

func TestLimitByDocumentCount_DisabledByZeroOrNegative(t *testing.T) {
	folders := []FolderOption{{ID: "a"}, {ID: "b"}}
	for _, max := range []int{0, -1} {
		got := LimitByDocumentCount(folders, max)
		if len(got) != len(folders) {
			t.Errorf("LimitByDocumentCount(_, %d) = %v, want all folders unchanged", max, got)
		}
	}
}

func TestExcludeByID(t *testing.T) {
	folders := []FolderOption{{ID: "a"}, {ID: "b"}, {ID: "c"}}

	got := ExcludeByID(folders, "b")

	if len(got) != 2 {
		t.Fatalf("ExcludeByID() = %v, want 2 remaining folders", got)
	}
	for _, f := range got {
		if f.ID == "b" {
			t.Fatalf("ExcludeByID() did not remove folder %q", f.ID)
		}
	}
}

func TestFindByPathOrID(t *testing.T) {
	folders := []FolderOption{
		{ID: "id-1", Path: "Projects/Work"},
		{ID: "id-2", Path: "Personal"},
	}

	if f, ok := FindByPathOrID(folders, "id-2"); !ok || f.Path != "Personal" {
		t.Errorf("lookup by id = (%+v, %v), want id-2/Personal", f, ok)
	}
	if f, ok := FindByPathOrID(folders, "Projects/Work"); !ok || f.ID != "id-1" {
		t.Errorf("lookup by path = (%+v, %v), want id-1", f, ok)
	}
	if _, ok := FindByPathOrID(folders, "does-not-exist"); ok {
		t.Errorf("lookup for a missing key unexpectedly succeeded")
	}
}

// TestDescriptionExcluding is the catalog-level counterpart to the
// RouteDocument regression test in router_test.go: it isolates the
// description-rendering contract itself (excludeDocID removes exactly one
// sample, "" excludes nothing) from the plumbing that calls it.
func TestDescriptionExcluding(t *testing.T) {
	f := FolderOption{
		Path:              "FleetingNotes",
		DocumentCount:     2,
		SampleTitles:      []string{"Keep me", "Drop me"},
		SampleDocumentIDs: []string{"keep-id", "drop-id"},
	}

	withoutExclusion := f.DescriptionExcluding("")
	if !strings.Contains(withoutExclusion, "Keep me") || !strings.Contains(withoutExclusion, "Drop me") {
		t.Errorf("DescriptionExcluding(%q) = %q, want both samples present", "", withoutExclusion)
	}

	excluded := f.DescriptionExcluding("drop-id")
	if !strings.Contains(excluded, "Keep me") {
		t.Errorf("DescriptionExcluding(%q) dropped an unrelated sample: %q", "drop-id", excluded)
	}
	if strings.Contains(excluded, "Drop me") {
		t.Errorf("DescriptionExcluding(%q) did not drop the targeted sample: %q", "drop-id", excluded)
	}
}
