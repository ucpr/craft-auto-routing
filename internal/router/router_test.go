package router

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ucpr/craft-auto-routing/internal/craft"
	"github.com/ucpr/craft-auto-routing/internal/typesafe"
)

// fakeCraftAPI is a CraftAPI test double whose GetBlocks/MoveDocuments
// behavior is set per test, and which records how MoveDocuments was called
// so tests can assert a move did (or did not) happen.
type fakeCraftAPI struct {
	block        *craft.Block
	getBlocksErr error
	moveErr      error

	moveCalled  bool
	movedIDs    []string
	movedFolder string
}

func (f *fakeCraftAPI) GetBlocks(ctx context.Context, id string, maxDepth int) (*craft.Block, error) {
	if f.getBlocksErr != nil {
		return nil, f.getBlocksErr
	}
	if f.block != nil {
		return f.block, nil
	}
	return &craft.Block{}, nil
}

func (f *fakeCraftAPI) MoveDocuments(ctx context.Context, documentIDs []string, folderID string) error {
	f.moveCalled = true
	f.movedIDs = documentIDs
	f.movedFolder = folderID
	return f.moveErr
}

// fakeClassifier is a Classifier test double that returns a fixed result (or
// error) and records the criteria map it was called with, so tests can
// inspect exactly what was sent to the classifier.
type fakeClassifier struct {
	result typesafe.ClassifyResult
	err    error

	called           bool
	capturedCriteria map[string]string
}

func (f *fakeClassifier) Classify(ctx context.Context, questionKey, instructions, state string, options map[string]string) (typesafe.ClassifyResult, error) {
	f.called = true
	f.capturedCriteria = options
	if f.err != nil {
		return typesafe.ClassifyResult{}, f.err
	}
	return f.result, nil
}

func testCatalog() []FolderOption {
	return []FolderOption{
		{ID: "folder-a", Path: "Projects/A", DocumentCount: 2},
		{ID: "folder-b", Path: "Projects/B", DocumentCount: 1},
	}
}

func TestRouteDocument_MovesWhenConfidenceMeetsThreshold(t *testing.T) {
	craftAPI := &fakeCraftAPI{}
	classifier := &fakeClassifier{result: typesafe.ClassifyResult{Choice: "folder-a", Confidence: 0.9}}
	doc := craft.Document{ID: "doc-1", Title: "Some note"}

	decision := RouteDocument(context.Background(), craftAPI, classifier, doc, testCatalog(), Options{MinConfidence: 0.6})

	if decision.Err != nil {
		t.Fatalf("unexpected error: %v", decision.Err)
	}
	if !decision.Moved {
		t.Errorf("Moved = false, want true")
	}
	if decision.Skipped {
		t.Errorf("Skipped = true, want false")
	}
	if !craftAPI.moveCalled {
		t.Fatalf("MoveDocuments was not called")
	}
	if got, want := craftAPI.movedFolder, "folder-a"; got != want {
		t.Errorf("moved to folder %q, want %q", got, want)
	}
	if got, want := craftAPI.movedIDs, []string{"doc-1"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("moved document ids = %v, want %v", got, want)
	}
}

func TestRouteDocument_SkipsBelowThresholdWithoutMoving(t *testing.T) {
	craftAPI := &fakeCraftAPI{}
	classifier := &fakeClassifier{result: typesafe.ClassifyResult{Choice: "folder-a", Confidence: 0.3}}
	doc := craft.Document{ID: "doc-1", Title: "Ambiguous note"}

	decision := RouteDocument(context.Background(), craftAPI, classifier, doc, testCatalog(), Options{MinConfidence: 0.6})

	if !decision.Skipped {
		t.Errorf("Skipped = false, want true")
	}
	if decision.Moved {
		t.Errorf("Moved = true, want false")
	}
	if craftAPI.moveCalled {
		t.Errorf("MoveDocuments was called, want no call for a below-threshold, non-forced decision")
	}
	if decision.SkipReason == "" {
		t.Errorf("SkipReason is empty, want an explanation")
	}
}

func TestRouteDocument_ForceOverridesLowConfidence(t *testing.T) {
	craftAPI := &fakeCraftAPI{}
	classifier := &fakeClassifier{result: typesafe.ClassifyResult{Choice: "folder-a", Confidence: 0.3}}
	doc := craft.Document{ID: "doc-1", Title: "Ambiguous note"}

	decision := RouteDocument(context.Background(), craftAPI, classifier, doc, testCatalog(), Options{MinConfidence: 0.6, Force: true})

	if decision.Skipped {
		t.Errorf("Skipped = true, want false when Force is set")
	}
	if !decision.Forced {
		t.Errorf("Forced = false, want true")
	}
	if !decision.Moved || !craftAPI.moveCalled {
		t.Errorf("expected a forced move to happen despite low confidence")
	}
}

func TestRouteDocument_DryRunDoesNotMove(t *testing.T) {
	craftAPI := &fakeCraftAPI{}
	classifier := &fakeClassifier{result: typesafe.ClassifyResult{Choice: "folder-a", Confidence: 0.9}}
	doc := craft.Document{ID: "doc-1", Title: "Some note"}

	decision := RouteDocument(context.Background(), craftAPI, classifier, doc, testCatalog(), Options{MinConfidence: 0.6, DryRun: true})

	if decision.Err != nil {
		t.Fatalf("unexpected error: %v", decision.Err)
	}
	if craftAPI.moveCalled {
		t.Errorf("MoveDocuments was called during a dry run")
	}
	if decision.Folder.ID != "folder-a" {
		t.Errorf("Folder = %+v, want the classified folder to still be reported for the plan", decision.Folder)
	}
}

func TestRouteDocument_GetBlocksErrorSkipsClassifyAndMove(t *testing.T) {
	craftAPI := &fakeCraftAPI{getBlocksErr: errors.New("network down")}
	classifier := &fakeClassifier{result: typesafe.ClassifyResult{Choice: "folder-a", Confidence: 0.9}}
	doc := craft.Document{ID: "doc-1", Title: "Some note"}

	decision := RouteDocument(context.Background(), craftAPI, classifier, doc, testCatalog(), Options{MinConfidence: 0.6})

	if decision.Err == nil {
		t.Fatalf("expected an error, got nil")
	}
	if !strings.Contains(decision.Err.Error(), "network down") {
		t.Errorf("error %q does not wrap the underlying cause", decision.Err)
	}
	if classifier.called {
		t.Errorf("classifier was called despite a content-fetch failure")
	}
	if craftAPI.moveCalled {
		t.Errorf("MoveDocuments was called despite a content-fetch failure")
	}
}

func TestRouteDocument_ClassifyErrorDoesNotMove(t *testing.T) {
	craftAPI := &fakeCraftAPI{}
	classifier := &fakeClassifier{err: errors.New("classifier unavailable")}
	doc := craft.Document{ID: "doc-1", Title: "Some note"}

	decision := RouteDocument(context.Background(), craftAPI, classifier, doc, testCatalog(), Options{MinConfidence: 0.6})

	if decision.Err == nil {
		t.Fatalf("expected an error, got nil")
	}
	if craftAPI.moveCalled {
		t.Errorf("MoveDocuments was called despite a classification failure")
	}
}

func TestRouteDocument_UnknownFolderIDIsAnError(t *testing.T) {
	craftAPI := &fakeCraftAPI{}
	classifier := &fakeClassifier{result: typesafe.ClassifyResult{Choice: "does-not-exist", Confidence: 0.9}}
	doc := craft.Document{ID: "doc-1", Title: "Some note"}

	decision := RouteDocument(context.Background(), craftAPI, classifier, doc, testCatalog(), Options{MinConfidence: 0.6})

	if decision.Err == nil {
		t.Fatalf("expected an error for an unrecognized classifier choice, got nil")
	}
	if craftAPI.moveCalled {
		t.Errorf("MoveDocuments was called despite an unresolvable folder id")
	}
}

func TestRouteDocument_MoveErrorIsReported(t *testing.T) {
	craftAPI := &fakeCraftAPI{moveErr: errors.New("move rejected")}
	classifier := &fakeClassifier{result: typesafe.ClassifyResult{Choice: "folder-a", Confidence: 0.9}}
	doc := craft.Document{ID: "doc-1", Title: "Some note"}

	decision := RouteDocument(context.Background(), craftAPI, classifier, doc, testCatalog(), Options{MinConfidence: 0.6})

	if decision.Err == nil {
		t.Fatalf("expected an error, got nil")
	}
	if decision.Moved {
		t.Errorf("Moved = true after a failed MoveDocuments call")
	}
}

// TestRouteDocument_ExcludesDocumentFromItsOwnFolderCriteria is a regression
// test for a real bug: when --source-folder leaves the folder being emptied
// in the candidate list, BuildCatalog samples that folder's own contents as
// "example existing documents". Without excluding the document currently
// being classified, its own title (and body) would appear verbatim in its
// own folder's description, so the classifier trivially "matches" it back to
// where it already is regardless of actual fit.
func TestRouteDocument_ExcludesDocumentFromItsOwnFolderCriteria(t *testing.T) {
	catalog := []FolderOption{
		{
			ID:                "folder-a",
			Path:              "FleetingNotes",
			DocumentCount:     2,
			SampleTitles:      []string{"The document being routed", "Some other note"},
			SampleDocumentIDs: []string{"doc-1", "doc-2"},
		},
		{ID: "folder-b", Path: "Projects/B", DocumentCount: 0},
	}
	craftAPI := &fakeCraftAPI{}
	classifier := &fakeClassifier{result: typesafe.ClassifyResult{Choice: "folder-b", Confidence: 0.9}}
	doc := craft.Document{ID: "doc-1", Title: "The document being routed"}

	RouteDocument(context.Background(), craftAPI, classifier, doc, catalog, Options{MinConfidence: 0.6})

	if !classifier.called {
		t.Fatalf("classifier was never called")
	}
	folderADescription := classifier.capturedCriteria["folder-a"]
	if strings.Contains(folderADescription, "The document being routed") {
		t.Errorf("folder-a description leaks the document being classified as its own example: %q", folderADescription)
	}
	if !strings.Contains(folderADescription, "Some other note") {
		t.Errorf("folder-a description dropped an unrelated sample it should have kept: %q", folderADescription)
	}
}

func TestBuildState(t *testing.T) {
	block := &craft.Block{
		Markdown: "root",
		Content: []craft.Block{
			{Markdown: "child one"},
			{Content: []craft.Block{{Markdown: "grandchild"}}},
		},
	}

	got := buildState("My Title", block, 0)

	for _, want := range []string{"Title: My Title", "root", "child one", "grandchild"} {
		if !strings.Contains(got, want) {
			t.Errorf("buildState() = %q, missing expected substring %q", got, want)
		}
	}
}

// TestBuildState_TruncatesByRuneNotByte guards against a byte-oriented
// truncation bug: slicing multi-byte UTF-8 text (e.g. Japanese) by byte
// count instead of rune count can cut a character in half and produce
// invalid UTF-8, which would then corrupt whatever gets sent to the
// classifier.
func TestBuildState_TruncatesByRuneNotByte(t *testing.T) {
	block := &craft.Block{Markdown: "こんにちは世界"} // 7 multi-byte runes
	// "Title: " + "\n\n" is 9 runes; cutting 3 runes into the Japanese text
	// lands the truncation boundary in the middle of a multi-byte run.
	const maxRunes = 12
	got := buildState("", block, maxRunes)

	runes := []rune(got)
	if len(runes) != maxRunes {
		t.Fatalf("buildState() returned %d runes, want exactly %d: %q", len(runes), maxRunes, got)
	}
	if !isValidUTF8(got) {
		t.Fatalf("buildState() produced invalid UTF-8 by truncating mid-character: %q", got)
	}
	if !strings.HasSuffix(got, "こんに") {
		t.Fatalf("buildState() = %q, want it to end mid-word with the first 3 Japanese characters intact", got)
	}
}

// isValidUTF8 reports whether s decodes cleanly, with no bytes replaced by
// the Unicode replacement character U+FFFD (what range-over-string produces
// for a truncated/invalid multi-byte sequence).
func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
