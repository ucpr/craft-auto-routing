package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/ucpr/craft-auto-routing/internal/craft"
	"github.com/ucpr/craft-auto-routing/internal/typesafe"
)

const classifyQuestionKey = "target_folder"

const classifyInstructions = "This is an unfiled document from a note-taking space. " +
	"Based on the document's title and content, and on the existing folder " +
	"structure (including examples of documents already filed in each " +
	"folder), choose the single folder it best belongs in."

// Decision is the outcome of classifying one document.
type Decision struct {
	Document      craft.Document
	Folder        FolderOption
	Confidence    float64
	Probabilities map[string]float64
	Moved         bool
	// Forced indicates the move happened despite Confidence being below
	// MinConfidence, because Options.Force was set.
	Forced     bool
	Skipped    bool
	SkipReason string
	// State is the exact text sent to the classifier as the document body,
	// kept for debugging low-confidence or surprising decisions.
	State string
	Err   error
}

// Options controls a routing run.
type Options struct {
	DryRun bool
	// Force bypasses MinConfidence and always routes to the classifier's
	// top-probability folder, even when it isn't confident.
	Force          bool
	MinConfidence  float64
	MaxContentRune int
	BlocksMaxDepth int
}

// CraftAPI is the subset of the Craft client the router needs.
type CraftAPI interface {
	GetBlocks(ctx context.Context, id string, maxDepth int) (*craft.Block, error)
	MoveDocuments(ctx context.Context, documentIDs []string, folderID string) error
}

// Classifier is the subset of the TypeSafe client the router needs.
type Classifier interface {
	Classify(ctx context.Context, questionKey, instructions, state string, options map[string]string) (typesafe.ClassifyResult, error)
}

// RouteDocument classifies a single unfiled document against the catalog and,
// unless DryRun is set or confidence is below MinConfidence, moves it into
// the chosen folder.
func RouteDocument(ctx context.Context, craftClient CraftAPI, tsClient Classifier, doc craft.Document, catalog []FolderOption, opts Options) Decision {
	block, err := craftClient.GetBlocks(ctx, doc.ID, opts.BlocksMaxDepth)
	if err != nil {
		return Decision{Document: doc, Err: fmt.Errorf("fetch content: %w", err)}
	}

	state := buildState(doc.Title, block, opts.MaxContentRune)
	criteria := ToCriteria(catalog)

	result, err := tsClient.Classify(ctx, classifyQuestionKey, classifyInstructions, state, criteria)
	if err != nil {
		return Decision{Document: doc, Err: fmt.Errorf("classify: %w", err)}
	}

	folder, ok := FindByID(catalog, result.Choice)
	if !ok {
		return Decision{
			Document:      doc,
			Confidence:    result.Confidence,
			Probabilities: result.Probabilities,
			State:         state,
			Err:           fmt.Errorf("classifier returned unknown folder id %q", result.Choice),
		}
	}

	decision := Decision{
		Document:      doc,
		Folder:        folder,
		Confidence:    result.Confidence,
		Probabilities: result.Probabilities,
		State:         state,
	}

	belowThreshold := result.Confidence < opts.MinConfidence
	if belowThreshold && !opts.Force {
		decision.Skipped = true
		decision.SkipReason = fmt.Sprintf("confidence %.2f below threshold %.2f", result.Confidence, opts.MinConfidence)
		return decision
	}
	decision.Forced = belowThreshold && opts.Force

	if opts.DryRun {
		return decision
	}

	if err := craftClient.MoveDocuments(ctx, []string{doc.ID}, folder.ID); err != nil {
		decision.Err = fmt.Errorf("move: %w", err)
		return decision
	}
	decision.Moved = true
	return decision
}

// buildState concatenates the document title and body markdown into a single
// text blob for the classifier, truncated to maxRunes.
func buildState(title string, block *craft.Block, maxRunes int) string {
	var sb strings.Builder
	sb.WriteString("Title: ")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	var walk func(b *craft.Block)
	walk = func(b *craft.Block) {
		if b == nil {
			return
		}
		if b.Markdown != "" {
			sb.WriteString(b.Markdown)
			sb.WriteString("\n")
		}
		for i := range b.Content {
			walk(&b.Content[i])
		}
	}
	walk(block)

	text := sb.String()
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}
