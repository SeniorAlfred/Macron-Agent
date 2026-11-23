package diskfree

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSpaceOnARealFolder(t *testing.T) {
	total, free, ok := Space(t.TempDir())
	if !ok {
		t.Fatal("a temporary folder should report its free space")
	}
	if total == 0 || free > total {
		t.Fatalf("unexpected numbers: total=%d free=%d", total, free)
	}
}

func TestWritable(t *testing.T) {
	dir := t.TempDir()
	if !Writable(dir) {
		t.Fatal("a temporary folder is writable")
	}
	if Writable(filepath.Join(dir, "does-not-exist")) {
		t.Fatal("a missing folder is not writable")
	}
	if runtime.GOOS == "windows" {
		return
	}
	readOnly := t.TempDir()
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatalf("cannot make it read-only: %v", err)
	}
	defer os.Chmod(readOnly, 0o700)
	if os.Geteuid() != 0 && Writable(readOnly) {
		t.Fatal("a read-only folder must not be writable")
	}
}

func TestSameDevice(t *testing.T) {
	dir := t.TempDir()
	if !SameDevice(dir, dir) {
		t.Fatal("a folder is on the same device as itself")
	}
	other := t.TempDir()
	if !SameDevice(dir, other) {
		t.Fatal("two temporary folders live on the same device")
	}
	if SameDevice(dir, "") {
		t.Fatal("an empty path cannot be on the same device")
	}
}
