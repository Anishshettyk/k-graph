package cmd

import "testing"

func TestInterpret(t *testing.T) {
	tests := []struct {
		name     string
		question string
		wantVerb string
		wantKind string
		wantName string
	}{
		{"forward slash", "what does deployment/frontend depend on", "deps", "Deployment", "frontend"},
		{"reverse slash", "what depends on service/frontend", "impact", "Service", "frontend"},
		{"breaks if", "what breaks if I change pod/frontend-abc", "impact", "Pod", "frontend-abc"},
		{"why unhealthy", "why is deployment/frontend unhealthy", "why", "Deployment", "frontend"},
		{"kind name form", "what depends on service frontend", "impact", "Service", "frontend"},
		{"uses form", "what does deploy frontend use", "deps", "Deployment", "frontend"},
		{"impact keyword", "show the impact of svc/frontend", "impact", "Service", "frontend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := interpret(tt.question)
			if err != nil {
				t.Fatalf("interpret(%q) error: %v", tt.question, err)
			}
			if got.Verb != tt.wantVerb {
				t.Errorf("verb = %q, want %q", got.Verb, tt.wantVerb)
			}
			if got.Ref.Kind != tt.wantKind || got.Ref.Name != tt.wantName {
				t.Errorf("ref = %s/%s, want %s/%s", got.Ref.Kind, got.Ref.Name, tt.wantKind, tt.wantName)
			}
		})
	}
}

func TestInterpretErrors(t *testing.T) {
	cases := []string{
		"",                              // empty
		"what depends on",               // no resource
		"tell me about the weather pod", // "pod" alone at end, no name after
	}
	for _, q := range cases {
		if _, err := interpret(q); err == nil {
			t.Errorf("interpret(%q) expected error, got nil", q)
		}
	}
}
