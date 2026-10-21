package fs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestVideoDirs(t *testing.T) {
	root := t.TempDir()

	mustWrite := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite(filepath.Join(root, "root.mp4"))
	mustWrite(filepath.Join(root, "sub1", "a.mkv"))
	mustWrite(filepath.Join(root, "sub1", "compressed", "a.mp4"))
	mustWrite(filepath.Join(root, "sub2", "notes.txt"))
	mustWrite(filepath.Join(root, "sub3", "deep", "b.mov"))

	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := New().VideoDirs(root)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		root,
		filepath.Join(root, "sub1"),
		filepath.Join(root, "sub3", "deep"),
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("VideoDirs = %v, want %v", got, want)
	}
}
