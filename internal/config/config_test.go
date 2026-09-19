package config

import (
	"strings"
	"testing"
)

// TestLoad is a regression suite for a real incident: CRAFT_BASE_URL used to
// default to a value baked into the binary that embedded one user's private
// Craft Connect link id (see git history). Load must now refuse to start
// without it, exactly like the other required credentials, and must never
// reintroduce a default for it.
func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
		want    Config
	}{
		{
			name:    "missing craft base url",
			env:     map[string]string{"CRAFT_API_TOKEN": "tok", "TYPESAFE_API_KEY": "key"},
			wantErr: "CRAFT_BASE_URL",
		},
		{
			name:    "missing craft api token",
			env:     map[string]string{"CRAFT_BASE_URL": "https://example.com/api", "TYPESAFE_API_KEY": "key"},
			wantErr: "CRAFT_API_TOKEN",
		},
		{
			name:    "missing typesafe api key",
			env:     map[string]string{"CRAFT_BASE_URL": "https://example.com/api", "CRAFT_API_TOKEN": "tok"},
			wantErr: "TYPESAFE_API_KEY",
		},
		{
			name: "all required vars set defaults the model",
			env: map[string]string{
				"CRAFT_BASE_URL":   "https://example.com/api",
				"CRAFT_API_TOKEN":  "tok",
				"TYPESAFE_API_KEY": "key",
			},
			want: Config{
				CraftBaseURL:   "https://example.com/api",
				CraftAPIToken:  "tok",
				TypesafeAPIKey: "key",
				TypesafeModel:  "jev-latest",
			},
		},
		{
			name: "explicit model overrides the default",
			env: map[string]string{
				"CRAFT_BASE_URL":   "https://example.com/api",
				"CRAFT_API_TOKEN":  "tok",
				"TYPESAFE_API_KEY": "key",
				"TYPESAFE_MODEL":   "custom-model",
			},
			want: Config{
				CraftBaseURL:   "https://example.com/api",
				CraftAPIToken:  "tok",
				TypesafeAPIKey: "key",
				TypesafeModel:  "custom-model",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setenv every relevant key on each subtest, including to "" for
			// keys the case doesn't set, so a variable exported in the
			// developer's own shell (this tool talks to a real Craft/TypeSafe
			// account) can never leak into the test and mask a missing-var
			// case.
			for _, key := range []string{"CRAFT_BASE_URL", "CRAFT_API_TOKEN", "TYPESAFE_API_KEY", "TYPESAFE_MODEL"} {
				t.Setenv(key, tt.env[key])
			}

			got, err := Load()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Load() = %+v, nil; want an error mentioning %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() error = %q, want it to mention %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
