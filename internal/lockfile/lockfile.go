package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Lock represents a process lock file
type Lock struct {
	path string
	file *os.File
}

// Acquire attempts to acquire an exclusive lock on the lock file
// Returns an error if another process already holds the lock
func Acquire(lockPath string) (*Lock, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(lockPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Open or create the lock file
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try to acquire exclusive lock (non-blocking)
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		file.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, fmt.Errorf("another instance is already running (lock held at %s)", lockPath)
		}
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	// Write current PID to lock file for debugging
	pid := os.Getpid()
	if err := file.Truncate(0); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to truncate lock file: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to seek lock file: %w", err)
	}
	if _, err := file.WriteString(strconv.Itoa(pid) + "\n"); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to write PID to lock file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to sync lock file: %w", err)
	}

	return &Lock{
		path: lockPath,
		file: file,
	}, nil
}

// Release releases the lock
// Note: Does NOT remove the lock file to avoid race conditions where a second
// process could create a new file (different inode) between LOCK_UN and os.Remove,
// causing both processes to hold locks on different inodes.
func (l *Lock) Release() error {
	if l.file == nil {
		return nil
	}

	// Release the flock
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	// Close the file
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("failed to close lock file: %w", err)
	}
	l.file = nil

	// Note: We intentionally do NOT remove the lock file here.
	// The lock file persists so the next process reuses the same inode,
	// preventing race conditions during lock acquisition.

	return nil
}

// Path returns the path to the lock file
func (l *Lock) Path() string {
	return l.path
}

// ReadPID reads the PID from a lock file
// Returns 0 if the file doesn't exist or can't be read
func ReadPID(lockPath string) (int, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read lock file: %w", err)
	}

	// Handle empty file or whitespace-only content
	content := strings.TrimSpace(string(data))
	if content == "" {
		return 0, fmt.Errorf("lock file is empty")
	}

	pid, err := strconv.Atoi(content)
	if err != nil {
		return 0, fmt.Errorf("failed to parse PID from lock file: %w", err)
	}

	return pid, nil
}

// IsProcessRunning checks if a process with the given PID is running
func IsProcessRunning(pid int) bool {
	// Send signal 0 to check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send a signal
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// GetLockPath returns the default lock file path based on database path
func GetLockPath(dbPath string) string {
	return dbPath + ".lock"
}
