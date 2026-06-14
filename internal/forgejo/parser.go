package forgejo

import "strings"

// ParseCommitType mirrors github.ParseCommitType so the syncer's
// commit-type classification is identical across providers. The set
// of recognized prefixes is intentionally kept in sync with
// internal/github/client.go.
func ParseCommitType(msg string) string {
	msg = strings.TrimSpace(msg)
	before, _, ok := strings.Cut(msg, ":")
	if !ok {
		return "other"
	}
	prefix := before
	if parenIdx := strings.Index(prefix, "("); parenIdx != -1 {
		prefix = prefix[:parenIdx]
	}
	switch strings.ToLower(strings.TrimSpace(prefix)) {
	case "feat", "feature":
		return "feat"
	case "fix", "bugfix", "bug":
		return "fix"
	case "docs", "documentation":
		return "docs"
	case "chore", "refactor", "test", "style", "perf", "ci", "build", "revert":
		return "chore"
	default:
		return "other"
	}
}
