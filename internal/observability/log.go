package observability

import (
	"os"
	"path/filepath"
	"sync"
)

const logMaxBytes = 10 << 20

// RotatingLog retains one active file and one backup, each at most 10 MiB.
type RotatingLog struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

func OpenLog(path string) (*RotatingLog, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &RotatingLog{path: path, file: f, size: info.Size()}, nil
}
func (l *RotatingLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return 0, err
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return 0, err
		}
		l.file = f
		l.size = info.Size()
	}
	original := len(p)
	if len(p) > logMaxBytes {
		p = p[:logMaxBytes]
	}
	if l.size+int64(len(p)) > logMaxBytes {
		if err := os.Remove(l.path + ".1"); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		closeErr := l.file.Close()
		l.file = nil
		if closeErr != nil {
			return 0, closeErr
		}
		if err := os.Rename(l.path, l.path+".1"); err != nil {
			return 0, err
		}
		f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return 0, err
		}
		l.file = f
		l.size = 0
	}
	n, err := l.file.Write(p)
	l.size += int64(n)
	if err != nil {
		return n, err
	}
	return original, nil
}
func (l *RotatingLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
