package conformance

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run renders the suite and compares the bytes with the golden file at
// goldenPath. With update true it writes the file instead. Differences are
// reported through tb.Errorf, so a plugin can wrap Run in its own test.
func Run[T any](tb testing.TB, s Suite[T], goldenPath string, update bool) {
	tb.Helper()
	got := Markdown(s, Results(s))
	if update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			tb.Errorf("conformance: create golden directory: %v", err)
			return
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			tb.Errorf("conformance: write golden file %s: %v", goldenPath, err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if errors.Is(err, fs.ErrNotExist) {
		tb.Errorf("conformance: golden file %s does not exist, rerun with the update flag to create it", goldenPath)
		return
	}
	if err != nil {
		tb.Errorf("conformance: read golden file %s: %v", goldenPath, err)
		return
	}
	if string(want) == got {
		return
	}
	tb.Errorf("conformance: output differs from %s\n%s\nrerun with the update flag to accept the new output", goldenPath, diffLines(string(want), got, 10))
}

const endOfFile = "(end of file)"

func diffLines(want, got string, limit int) string {
	w := strings.Split(want, "\n")
	g := strings.Split(got, "\n")
	var b strings.Builder
	reported := 0
	for i := 0; i < len(w) || i < len(g); i++ {
		wl, gl := endOfFile, endOfFile
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl == gl {
			continue
		}
		fmt.Fprintf(&b, "line %d\n  want: %s\n  got:  %s\n", i+1, wl, gl)
		reported++
		if reported == limit {
			break
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
