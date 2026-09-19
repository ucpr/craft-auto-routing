package craft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// These tests pin down the wire contract with the Craft Connect API using a
// local httptest server rather than mocks, so a refactor that silently
// changes a JSON tag, a header, or how a query parameter is built would be
// caught here instead of failing only against the live API.

func TestClient_ListFolders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/folders" {
			t.Errorf("request = %s %s, want GET /folders", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer test-token")
		}
		_ = json.NewEncoder(w).Encode(foldersResponse{
			Items: []Folder{{ID: "f1", Name: "Projects", DocumentCount: 2}},
		})
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "test-token").ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "f1" {
		t.Fatalf("ListFolders() = %+v, want a single folder f1", got)
	}
}

func TestClient_ListDocuments_QueryParams(t *testing.T) {
	tests := []struct {
		name   string
		params ListDocumentsParams
		want   url.Values
	}{
		{
			name:   "location only",
			params: ListDocumentsParams{Location: "unsorted"},
			want:   url.Values{"location": {"unsorted"}},
		},
		{
			name:   "folder id only",
			params: ListDocumentsParams{FolderID: "folder-123"},
			want:   url.Values{"folderId": {"folder-123"}},
		},
		{
			name:   "fetch metadata only set when true",
			params: ListDocumentsParams{Location: "trash", FetchMetadata: true},
			want:   url.Values{"location": {"trash"}, "fetchMetadata": {"true"}},
		},
		{
			name:   "no filters means no query string",
			params: ListDocumentsParams{},
			want:   url.Values{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotQuery url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.Query()
				_ = json.NewEncoder(w).Encode(documentsResponse{})
			}))
			defer srv.Close()

			if _, err := NewClient(srv.URL, "tok").ListDocuments(context.Background(), tt.params); err != nil {
				t.Fatalf("ListDocuments() error: %v", err)
			}
			if len(gotQuery) != len(tt.want) {
				t.Fatalf("query = %v, want %v", gotQuery, tt.want)
			}
			for k, v := range tt.want {
				if gotQuery.Get(k) != v[0] {
					t.Errorf("query[%q] = %q, want %q", k, gotQuery.Get(k), v[0])
				}
			}
		})
	}
}

func TestClient_GetBlocks_OmitsMaxDepthWhenNotPositive(t *testing.T) {
	tests := []struct {
		maxDepth    int
		wantPresent bool
	}{
		{maxDepth: 0, wantPresent: false},
		{maxDepth: -1, wantPresent: false},
		{maxDepth: 3, wantPresent: true},
	}

	for _, tt := range tests {
		var sawMaxDepth bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, sawMaxDepth = r.URL.Query()["maxDepth"]
			_ = json.NewEncoder(w).Encode(Block{ID: "b1"})
		}))

		if _, err := NewClient(srv.URL, "tok").GetBlocks(context.Background(), "b1", tt.maxDepth); err != nil {
			t.Fatalf("GetBlocks(maxDepth=%d) error: %v", tt.maxDepth, err)
		}
		if sawMaxDepth != tt.wantPresent {
			t.Errorf("GetBlocks(maxDepth=%d): maxDepth query param present = %v, want %v", tt.maxDepth, sawMaxDepth, tt.wantPresent)
		}
		srv.Close()
	}
}

func TestClient_MoveDocuments_RequestBodyShape(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/documents/move" {
			t.Errorf("request = %s %s, want PUT /documents/move", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("request body is not valid JSON: %v (%s)", err, body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := NewClient(srv.URL, "tok").MoveDocuments(context.Background(), []string{"doc-1", "doc-2"}, "folder-9")
	if err != nil {
		t.Fatalf("MoveDocuments() error: %v", err)
	}

	// This shape (documentIds / destination.folderId) is the literal Craft
	// API contract; a JSON tag rename anywhere in moveRequest would silently
	// break production while every other test here kept passing.
	ids, _ := gotBody["documentIds"].([]any)
	if len(ids) != 2 || ids[0] != "doc-1" || ids[1] != "doc-2" {
		t.Errorf("documentIds = %v, want [doc-1 doc-2]", gotBody["documentIds"])
	}
	destination, _ := gotBody["destination"].(map[string]any)
	if destination["folderId"] != "folder-9" {
		t.Errorf("destination.folderId = %v, want folder-9", destination["folderId"])
	}
}

func TestClient_WrapsNon2xxResponsesWithStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "tok").ListFolders(context.Background())
	if err == nil {
		t.Fatalf("expected an error for a 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to mention the status code and response body so a CLI user can diagnose it", err.Error())
	}
}
