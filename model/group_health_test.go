package model

import (
	"strings"
	"testing"
)

func TestClampErrorMsgKeepsShortMessages(t *testing.T) {
	msg := "bad response status code 403"
	if got := clampErrorMsg(msg); got != msg {
		t.Fatalf("clampErrorMsg(%q) = %q, want unchanged", msg, got)
	}
}

// The column is varchar(512) and MySQL runs in strict mode: a longer value makes
// the whole upsert fail, so the probe row never lands and the group looks
// unmeasured. Clamp in runes so a multi-byte body cannot be cut mid-character.
func TestClampErrorMsgTruncatesToColumnWidth(t *testing.T) {
	long := strings.Repeat("错", modelHealthErrorMsgLimit+50)
	got := clampErrorMsg(long)
	if len([]rune(got)) != modelHealthErrorMsgLimit {
		t.Fatalf("clampErrorMsg length = %d runes, want %d",
			len([]rune(got)), modelHealthErrorMsgLimit)
	}
	if !strings.HasPrefix(long, got) {
		t.Fatal("clamped message is not a prefix of the original")
	}
}

func TestClampErrorMsgBoundary(t *testing.T) {
	exact := strings.Repeat("a", modelHealthErrorMsgLimit)
	if got := clampErrorMsg(exact); got != exact {
		t.Fatal("a message exactly at the limit must be kept as-is")
	}
	over := exact + "b"
	if got := clampErrorMsg(over); got != exact {
		t.Fatalf("over-limit message should lose the last rune, got %d runes",
			len([]rune(got)))
	}
}
