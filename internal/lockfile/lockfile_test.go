package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireAndRelease(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Acquire lock
	lock, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Verify lock file exists
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("Lock file was not created")
	}

	// Verify PID was written
	pid, err := ReadPID(lockPath)
	if err != nil {
		t.Fatalf("Failed to read PID: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("Expected PID %d, got %d", os.Getpid(), pid)
	}

	// Release lock
	if err := lock.Release(); err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Verify lock file still exists (not removed to avoid race conditions)
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("Lock file should persist after release")
	}
}

func TestAcquireTwice_Fails(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Acquire first lock
	lock1, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}
	defer lock1.Release()

	// Try to acquire second lock (should fail)
	lock2, err := Acquire(lockPath)
	if err == nil {
		lock2.Release()
		t.Fatal("Expected second lock acquisition to fail, but it succeeded")
	}

	if lock2 != nil {
		t.Error("Expected lock2 to be nil when acquisition fails")
	}
}

func TestAcquireAfterRelease(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Acquire and release first lock
	lock1, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}

	// Get inode of first lock file
	info1, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("Failed to stat lock file: %v", err)
	}

	if err := lock1.Release(); err != nil {
		t.Fatalf("Failed to release first lock: %v", err)
	}

	// Acquire second lock (should succeed and reuse same file)
	lock2, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Failed to acquire second lock after release: %v", err)
	}
	defer lock2.Release()

	// Verify same inode is reused (prevents race condition)
	info2, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("Failed to stat lock file after reacquire: %v", err)
	}

	if !os.SameFile(info1, info2) {
		t.Error("Lock file inode changed after release/reacquire - race condition possible")
	}
}

func TestReadPID_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "nonexistent.lock")

	pid, err := ReadPID(lockPath)
	if err != nil {
		t.Fatalf("ReadPID should not error on non-existent file: %v", err)
	}
	if pid != 0 {
		t.Errorf("Expected PID 0 for non-existent file, got %d", pid)
	}
}

func TestReadPID_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "empty.lock")

	// Create empty lock file
	if err := os.WriteFile(lockPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create empty lock file: %v", err)
	}

	pid, err := ReadPID(lockPath)
	if err == nil {
		t.Error("Expected error for empty lock file, got nil")
	}
	if pid != 0 {
		t.Errorf("Expected PID 0 for empty file, got %d", pid)
	}
}

func TestReadPID_WhitespaceOnly(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "whitespace.lock")

	// Create lock file with only whitespace
	if err := os.WriteFile(lockPath, []byte("  \n\t  \n"), 0644); err != nil {
		t.Fatalf("Failed to create whitespace lock file: %v", err)
	}

	pid, err := ReadPID(lockPath)
	if err == nil {
		t.Error("Expected error for whitespace-only lock file, got nil")
	}
	if pid != 0 {
		t.Errorf("Expected PID 0 for whitespace file, got %d", pid)
	}
}

func TestReadPID_NoNewline(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "nonewline.lock")

	// Create lock file without trailing newline
	if err := os.WriteFile(lockPath, []byte("12345"), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	pid, err := ReadPID(lockPath)
	if err != nil {
		t.Fatalf("ReadPID should handle missing newline: %v", err)
	}
	if pid != 12345 {
		t.Errorf("Expected PID 12345, got %d", pid)
	}
}

func TestIsProcessRunning(t *testing.T) {
	// Current process should be running
	if !IsProcessRunning(os.Getpid()) {
		t.Error("Expected current process to be running")
	}

	// Very high PID should not exist
	if IsProcessRunning(999999) {
		t.Error("Expected PID 999999 to not be running")
	}
}

func TestGetLockPath(t *testing.T) {
	dbPath := "/var/lib/metrics/metrics.db"
	expected := "/var/lib/metrics/metrics.db.lock"

	lockPath := GetLockPath(dbPath)
	if lockPath != expected {
		t.Errorf("Expected lock path %q, got %q", expected, lockPath)
	}
}

func TestLockPath(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	defer lock.Release()

	if lock.Path() != lockPath {
		t.Errorf("Expected lock path %q, got %q", lockPath, lock.Path())
	}
}

func TestRelease_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// First release
	if err := lock.Release(); err != nil {
		t.Fatalf("First release failed: %v", err)
	}

	// Second release should not error
	if err := lock.Release(); err != nil {
		t.Errorf("Second release failed: %v", err)
	}
}

func TestAcquire_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "subdir", "test.lock")

	lock, err := Acquire(lockPath)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	defer lock.Release()

	// Verify directory was created
	dir := filepath.Dir(lockPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Lock directory was not created")
	}
}
