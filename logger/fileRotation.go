package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileRotateWriter writes log lines to one file and rotates it by size and
// by age. It is safe for concurrent use: zap calls Write from every
// goroutine that logs, without any locking of its own.
//
// Rotation renames the current file to "<timestamp>_<name>" and starts a
// new one. If a rotation fails (e.g. another process holds the file open on
// Windows), logging carries on in the current file and the rotation is
// retried after rotateRetryDelay - a log line is never dropped because of
// rotation.
type FileRotateWriter struct {
	CompleteFileName string
	fileName         string
	filePath         string
	Interval         time.Duration
	MaxSize          int

	mu          sync.Mutex
	file        *os.File
	size        int
	lastRotated time.Time
	retryAfter  time.Time
}

const rotateRetryDelay = time.Minute

func NewTimeRotationWriter(filename, filePath string, interval time.Duration, maxSize int) *FileRotateWriter {
	return &FileRotateWriter{
		fileName:         filename,
		filePath:         filePath,
		lastRotated:      time.Now(),
		CompleteFileName: filepath.Join(filePath, filename),
		Interval:         interval,
		MaxSize:          maxSize * MB,
	}
}

func (w *FileRotateWriter) Write(output []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}

	now := time.Now()
	sizeRotationRequired := w.MaxSize > 0 && w.size+len(output) > w.MaxSize
	timeRotationRequired := w.Interval > 0 && now.Sub(w.lastRotated) > w.Interval
	if (sizeRotationRequired || timeRotationRequired) && !now.Before(w.retryAfter) {
		if err := w.rotate(); err != nil {
			w.retryAfter = now.Add(rotateRetryDelay)
			fmt.Fprintf(os.Stderr, "log rotation failed, continuing in %s: %v\n", w.CompleteFileName, err)
		}
	}

	n, err := w.file.Write(output)
	w.size += n
	return n, err
}

// Sync flushes the file (zap calls it on Logger.Sync).
func (w *FileRotateWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

// open appends to the existing log file (a restart must not rotate it).
func (w *FileRotateWriter) open() error {
	if err := os.MkdirAll(filepath.Dir(w.CompleteFileName), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(w.CompleteFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file, w.size = f, int(info.Size())
	return nil
}

// rotate closes the current file, renames it with a timestamp and opens a
// fresh one. On failure the current file stays open for writing.
func (w *FileRotateWriter) rotate() error {
	backup := w.backupName(time.Now())

	if err := w.file.Close(); err != nil {
		return err
	}
	renameErr := os.Rename(w.CompleteFileName, backup)
	if err := w.open(); err != nil {
		return err
	}
	if renameErr != nil {
		return renameErr
	}
	w.lastRotated = time.Now()
	return nil
}

// backupName is "<timestamp>_<name>", with a counter added when a backup
// with that name already exists (several rotations in one millisecond
// would otherwise overwrite each other's file).
func (w *FileRotateWriter) backupName(now time.Time) string {
	dir, base := filepath.Dir(w.CompleteFileName), filepath.Base(w.fileName)
	stamp := now.Format("2006-01-02T15-04-05.000")
	name := filepath.Join(dir, fmt.Sprintf("%s_%s", stamp, base))
	for i := 1; ; i++ {
		if _, err := os.Stat(name); os.IsNotExist(err) {
			return name
		}
		name = filepath.Join(dir, fmt.Sprintf("%s-%d_%s", stamp, i, base))
	}
}
