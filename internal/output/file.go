package output

import (
	"fmt"
	"os"
)

type FileWriter struct {
	path       string
	maxRecords int
	file       *os.File
	count      int
}

func NewFileWriter(path string, maxRecords int) *FileWriter {
	return &FileWriter{
		path:       path,
		maxRecords: maxRecords,
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
	backup := w.path + ".1"
	os.Remove(backup)
	os.Rename(w.path, backup)
	w.open()
}
