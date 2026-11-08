package ai

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncatePromptRuneBoundary(t *testing.T) {
	short := "短字符串"
	if got := truncatePrompt(short); got != short {
		t.Fatalf("truncatePrompt(%q) = %q, want unchanged", short, got)
	}

	// A string whose 4 KiB boundary lands in the middle of a multi-byte rune
	// must be trimmed back to a whole rune, never splitting UTF-8.
	chinese := strings.Repeat("题", 2*1024) // each '题' is 3 bytes -> 6 KiB total
	if len(chinese) <= maxPromptFieldBytes {
		t.Fatalf("test input must exceed maxPromptFieldBytes")
	}
	got := truncatePrompt(chinese)
	if len(got) > maxPromptFieldBytes {
		t.Fatalf("truncatePrompt returned %d bytes, want <= %d", len(got), maxPromptFieldBytes)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncatePrompt returned invalid UTF-8")
	}
}
