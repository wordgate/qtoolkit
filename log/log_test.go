package log

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/natefinch/lumberjack.v2"
)

func TestCreateLogWriter_EmptyPathUsesStdout(t *testing.T) {
	if w := createLogWriter(""); w != os.Stdout {
		t.Fatalf("expected os.Stdout, got %T", w)
	}
}

func TestCreateLogWriter_PathWritesFileOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "app.log")

	w := createLogWriter(p)
	lj, ok := w.(*lumberjack.Logger)
	if !ok {
		t.Fatalf("expected *lumberjack.Logger only (no stdout tee), got %T", w)
	}
	defer lj.Close()

	if _, err := w.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("unexpected file content %q", data)
	}
}
