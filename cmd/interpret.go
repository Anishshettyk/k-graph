package cmd

import (
	"fmt"
	"strings"
)

// intent is the deterministic interpretation of a natural-language question.
type intent struct {
	Verb string // "deps", "impact", or "why"
	Ref  resourceRef
}

// interpret maps a plain-English question to a graph operation using simple,
// deterministic keyword rules — no AI. Recognized shapes include:
//
//	"what does deployment/frontend depend on"   -> deps
//	"what depends on service/frontend"          -> impact
//	"what breaks if I change secret/db"         -> impact
//	"why is deployment/frontend unhealthy"      -> why
func interpret(question string) (intent, error) {
	q := strings.ToLower(strings.TrimSpace(question))
	if q == "" {
		return intent{}, fmt.Errorf("empty question")
	}

	ref, err := extractRef(question)
	if err != nil {
		return intent{}, err
	}

	verb, err := classify(q)
	if err != nil {
		return intent{}, err
	}
	return intent{Verb: verb, Ref: ref}, nil
}

// classify picks the operation from the question's wording. Order matters:
// more specific phrases are checked first.
func classify(q string) (string, error) {
	has := func(sub string) bool { return strings.Contains(q, sub) }

	// Health questions.
	if has("why") || has("unhealthy") || has("broken") || has("root cause") {
		return "why", nil
	}

	// Dependency questions. The presence of "does"/"do" flips the direction:
	//   "what does X depend on"  -> forward (deps)
	//   "what depends on X"      -> reverse (impact)
	mentionsDependency := has("depend") || has("dependencies") || has("rely") ||
		has("relies") || has("needs") || has("need") || has("uses") || has("use")
	if mentionsDependency {
		if has("does ") || has("do ") {
			return "deps", nil
		}
		if has("depends on") || has("depending on") || has("depended on") {
			return "impact", nil
		}
		return "deps", nil
	}

	// Impact / blast-radius questions.
	if has("breaks if") || has("break if") || has("affected") || has("affects") ||
		has("impact") || has("consumers") || has("consume") {
		return "impact", nil
	}

	return "", fmt.Errorf("could not understand the question; try 'what depends on <ref>', 'what does <ref> depend on', or 'why is <ref> unhealthy'")
}

// extractRef finds a resource reference in a free-form question. It accepts the
// "kind/name" form (e.g. deployment/frontend) or a "kind name" pair
// (e.g. "deployment frontend").
func extractRef(question string) (resourceRef, error) {
	fields := strings.Fields(question)

	// Preferred: a token containing a slash that parses as <kind>/<name>.
	for _, f := range fields {
		f = strings.Trim(f, ".,?!\"'`")
		if strings.Contains(f, "/") {
			if ref, err := parseRef(f); err == nil {
				return ref, nil
			}
		}
	}

	// Fallback: a known kind word immediately followed by a name.
	for i := 0; i < len(fields)-1; i++ {
		word := strings.ToLower(strings.Trim(fields[i], ".,?!\"'`"))
		if canonical, ok := kindAliases[word]; ok {
			name := strings.Trim(fields[i+1], ".,?!\"'`")
			if name != "" {
				return resourceRef{Kind: canonical, Name: name}, nil
			}
		}
	}

	return resourceRef{}, fmt.Errorf("could not find a resource in the question; mention one like deployment/frontend")
}
