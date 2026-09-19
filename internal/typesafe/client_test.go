package typesafe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_Classify_RequestBodyShape(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("request = %s %s, want POST /v1/systemone", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization header = %q, want Bearer test-key", got)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("request body is not valid JSON: %v (%s)", err, body)
		}
		_ = json.NewEncoder(w).Encode(systemOneResponse{
			Answers: map[string]choiceAnswer{
				"target_folder": {Choice: "folder-a", Confidence: 0.8, Probabilities: map[string]float64{"folder-a": 0.8}},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("test-key", "jev-latest", WithBaseURL(srv.URL))
	result, err := c.Classify(context.Background(), "target_folder", "pick one", "the document text", map[string]string{
		"folder-a": "description of folder a",
	})
	if err != nil {
		t.Fatalf("Classify() error: %v", err)
	}

	// This shape (model / state / questions[key].{type,instructions,criteria})
	// is the literal SystemOne API contract; a rename anywhere in
	// systemOneRequest would silently stop sending real classification input.
	if gotBody["model"] != "jev-latest" {
		t.Errorf("model = %v, want jev-latest", gotBody["model"])
	}
	if gotBody["state"] != "the document text" {
		t.Errorf("state = %v, want the document text", gotBody["state"])
	}
	questions, _ := gotBody["questions"].(map[string]any)
	q, _ := questions["target_folder"].(map[string]any)
	if q["type"] != "choice" {
		t.Errorf("questions.target_folder.type = %v, want choice", q["type"])
	}
	if q["instructions"] != "pick one" {
		t.Errorf("questions.target_folder.instructions = %v, want %q", q["instructions"], "pick one")
	}
	criteria, _ := q["criteria"].(map[string]any)
	if criteria["folder-a"] != "description of folder a" {
		t.Errorf("questions.target_folder.criteria[folder-a] = %v, want %q", criteria["folder-a"], "description of folder a")
	}

	if result.Choice != "folder-a" || result.Confidence != 0.8 {
		t.Errorf("Classify() = %+v, want Choice=folder-a Confidence=0.8", result)
	}
}

func TestClient_Classify_NoOptionsErrorsWithoutCallingTheAPI(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	c := NewClient("key", "jev-latest", WithBaseURL(srv.URL))
	_, err := c.Classify(context.Background(), "q", "instructions", "state", map[string]string{})

	if err == nil {
		t.Fatalf("expected an error for zero options, got nil")
	}
	if called {
		t.Errorf("Classify() made an HTTP request despite having no options to classify against")
	}
}

func TestClient_Classify_MissingAnswerIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Responds without the "target_folder" key the caller asked about.
		_ = json.NewEncoder(w).Encode(systemOneResponse{Answers: map[string]choiceAnswer{}})
	}))
	defer srv.Close()

	c := NewClient("key", "jev-latest", WithBaseURL(srv.URL))
	_, err := c.Classify(context.Background(), "target_folder", "instructions", "state", map[string]string{"a": "b"})

	if err == nil {
		t.Fatalf("expected an error when the response has no answer for the asked question, got nil")
	}
}

func TestClient_Classify_WrapsNon2xxResponsesWithStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad state"}`))
	}))
	defer srv.Close()

	c := NewClient("key", "jev-latest", WithBaseURL(srv.URL))
	_, err := c.Classify(context.Background(), "q", "instructions", "state", map[string]string{"a": "b"})

	if err == nil {
		t.Fatalf("expected an error for a 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "bad state") {
		t.Errorf("error = %q, want it to mention the status code and response body", err.Error())
	}
}
