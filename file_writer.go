package main

import (
	"fmt"
	"os"
	"path"
	"sync"
)

type FileWriter struct {
	store_path string
	mu         sync.Mutex
	file       *os.File
}

func newFileWriter(store_path string) *FileWriter {
	return &FileWriter{store_path: store_path, mu: sync.Mutex{}, file: nil}
}

func (w *FileWriter) createFile(filename string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	dir, _ := path.Split(filename)
	err := os.MkdirAll(w.store_path+"/"+dir, 0755)
	if err != nil {
		fmt.Println("Error creating directory: ", err)
		return
	}

	f, err := os.Create(w.store_path + "/" + filename)
	if err != nil {
		fmt.Println("Error creating file: ", err)
		return
	}

	w.file = f
}

func (w *FileWriter) Write(data []byte, offset int64, filename string) {
	if w.file == nil {
		w.createFile(filename)
	}

	_, err := w.file.Seek(offset, 0)
	if err != nil {
		fmt.Println("Error seeking to offset: ", err)
		return
	}

	_, err = w.file.Write(data)
	if err != nil {
		fmt.Println("Error writing to file: ", err)
		return
	}
}

func (w *FileWriter) Close() {
	err := w.file.Sync()
	if err != nil {
		fmt.Println("Error syncing file: ", err)
	}

	fmt.Println("Write Completed")
	w.file.Close()
}
