package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func readAll(t *testing.T, dir string) (current string, all string, files int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		if e.Name() == "app.log" {
			current = string(data)
		}
	}
	return current, b.String(), len(entries)
}

// Many goroutines writing at once, with size rotation kicking in, must not
// lose or interleave a single line.
func TestFileRotateWriter_ConcurrentWritesWithRotation(t *testing.T) {
	dir := t.TempDir()
	w := NewTimeRotationWriter("app.log", dir, time.Hour, 0)
	w.MaxSize = 4 << 10 // rotate every 4 KB

	const goroutines, lines = 20, 200
	line := []byte(strings.Repeat("x", 60) + "\n")
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range lines {
				if _, err := w.Write(line); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	_ = w.Sync()
	_ = w.file.Close()

	_, all, files := readAll(t, dir)
	if got := strings.Count(all, string(line)); got != goroutines*lines {
		t.Fatalf("want %d intact lines, got %d", goroutines*lines, got)
	}
	if files < 2 {
		t.Fatalf("want size rotation to have produced several files, got %d", files)
	}
}

// A restart appends to the existing file instead of rotating it away, and
// counts its size.
func TestFileRotateWriter_AppendsAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	first := NewTimeRotationWriter("app.log", dir, time.Hour, 1)
	_, _ = first.Write([]byte("before restart\n"))
	_ = first.file.Close()

	second := NewTimeRotationWriter("app.log", dir, time.Hour, 1)
	_, _ = second.Write([]byte("after restart\n"))
	_ = second.file.Close()

	current, _, files := readAll(t, dir)
	if files != 1 || !bytes.Contains([]byte(current), []byte("before restart\nafter restart\n")) {
		t.Fatalf("want one file with both lines, got %d files: %q", files, current)
	}
	if second.size != len("before restart\nafter restart\n") {
		t.Fatalf("size must include the existing content, got %d", second.size)
	}
}
