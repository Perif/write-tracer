package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type FileWriter struct {
	path       string
	maxRecords int
	maxBackups int
	file       *os.File
	count      int
}

func NewFileWriter(path string, maxRecords int, maxBackups int) *FileWriter {
	return &FileWriter{
		path:       path,
		maxRecords: maxRecords,
		maxBackups: maxBackups,
	}
}

func (w *FileWriter) Write(line string) error {
	if w.path == "" {
		return nil
	}

	if w.file == nil {
		if err := w.open(); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w.file, line); err != nil {
		return err
	}

	w.count++
	if w.count >= w.maxRecords {
		w.rotate()
	}

	return nil
}

func (w *FileWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

func (w *FileWriter) open() error {
	// If file exists, rotate it on startup
	if _, err := os.Stat(w.path); err == nil {
		w.shiftBackups()
		os.Rename(w.path, w.path+".1")
	}

	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w.file = f
	w.count = 0
	return nil
}

func (w *FileWriter) rotate() {
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}

	// Shift existing backups: .N --> .N+1, remove oldest if it exceeds maxBackups
	w.shiftBackups()

	// Rename current file to .1
	os.Rename(w.path, w.path+".1")

	// Open new file
	w.open()
}

func (w *FileWriter) shiftBackups() {
	if w.maxBackups <= 0 {
		// No limit (aka very large limit), persist all the backups
		w.maxBackups = 1000
	}

	// Find all the existing backup files
	dir := filepath.Dir(w.path)
	base := filepath.Base(w.path)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Collect backup numbers
	var backupNums []int
	prefix := base + "."
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, prefix) {
			suffix := strings.TrimPrefix(name, prefix)
			if num, err := strconv.Atoi(suffix); err == nil && num > 0 {
				backupNums = append(backupNums, num)
			}
		}
	}

	// Sort in descending order
	sort.Sort(sort.Reverse(sort.IntSlice(backupNums)))

	for _, num := range backupNums {
		oldPath := fmt.Sprintf("%s.%d", w.path, num)
		newNum := num + 1

		if newNum > w.maxBackups {
			// Remove files that exceed the limit
			os.Remove(oldPath)
		} else {
			// Shift file
			newPath := fmt.Sprintf("%s.%d", w.path, newNum)
			os.Rename(oldPath, newPath)
		}
	}
}
