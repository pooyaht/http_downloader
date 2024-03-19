package writer

import (
	"fmt"
	"log/slog"
	"os"
	"path"
	"sync"
)

type FileWriter struct {
	store_path string
	mu         sync.Mutex
	file       *os.File
	logger     *slog.Logger
}

func NewFileWriter(store_path string, logger *slog.Logger) *FileWriter {
	return &FileWriter{store_path: store_path, mu: sync.Mutex{}, file: nil, logger: logger}
}

func (w *FileWriter) createFile(filename string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	dir, _ := path.Split(filename)
	err := os.MkdirAll(w.store_path+"/"+dir, 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(w.store_path + "/" + filename)
	if err != nil {
		return err
	}

	w.file = f
	return nil
}

func (w *FileWriter) Write(data []byte, offset int64, filename string) {
	var errors []error
	if w.file == nil {
		err := w.createFile(filename)
		if err != nil {
			errors = append(errors, err)
		}
	}

	_, err := w.file.Seek(offset, 0)
	if err != nil {
		errors = append(errors, err)
	}

	_, err = w.file.Write(data)
	if err != nil {
		errors = append(errors, err)
	}
	if len(errors) > 0 {
		for _, err := range errors {
			w.logger.Error(fmt.Sprintf("Error writing to file: %s", err))
		}
		w.logger.Error("Write failed; exiting ...")
		os.Exit(1)
	}

}

func (w *FileWriter) Close() {
	err := w.file.Sync()
	if err != nil {
		w.logger.Error(fmt.Sprintf("Error syncing file: %s", err))
		return
	}

	w.logger.Info("Write completed")
	w.file.Close()
}
