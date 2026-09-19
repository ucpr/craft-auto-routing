package cli

import (
	"reflect"
	"testing"
)

func TestParseFolderHints(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "no flags means no hints",
			in:   nil,
			want: nil,
		},
		{
			name: "simple pair",
			in:   []string{"FleetingNotes=only as a last resort"},
			want: map[string]string{"FleetingNotes": "only as a last resort"},
		},
		{
			name: "surrounding whitespace is trimmed",
			in:   []string{"  Projects/Work  =  prefer for infra topics  "},
			want: map[string]string{"Projects/Work": "prefer for infra topics"},
		},
		{
			name: "hint text may itself contain an equals sign",
			// strings.Cut splits on the FIRST "=" only, so "a=b" here must
			// stay part of the hint, not get truncated to "a".
			in:   []string{"key=a=b"},
			want: map[string]string{"key": "a=b"},
		},
		{
			name:    "missing equals sign is rejected",
			in:      []string{"no-equals-sign"},
			wantErr: true,
		},
		{
			name:    "empty key is rejected",
			in:      []string{"=hint text"},
			wantErr: true,
		},
		{
			name:    "empty hint text is rejected",
			in:      []string{"key="},
			wantErr: true,
		},
		{
			name: "later duplicate key wins",
			in:   []string{"k=first", "k=second"},
			want: map[string]string{"k": "second"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFolderHints(tt.in)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseFolderHints(%v) = %v, nil; want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFolderHints(%v) unexpected error: %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseFolderHints(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
