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
}

func newFileWriter(store_path string) *FileWriter {
	return &FileWriter{store_path: store_path, mu: sync.Mutex{}}
}

func (w *FileWriter) write(data []byte, offset int64, filename string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	dir, _ := path.Split(filename)

	err := os.MkdirAll(w.store_path+"/"+dir, 0755)
	if err != nil {
		fmt.Println("Error creating directory: ", err)
		return
	}

	f, err := os.OpenFile(w.store_path+"/"+filename, os.O_WRONLY|os.O_CREATE, 0664)
	if err != nil {
		fmt.Println("Error opening file: ", err)
		return
	}
	defer f.Close()

	_, err = f.Seek(offset, 0)
	if err != nil {
		fmt.Println("Error seeking to offset: ", err)
		return
	}

	_, err = f.Write(data)
	if err != nil {
		fmt.Println("Error writing to file: ", err)
		return
	}

	err = f.Sync()
	if err != nil {
		fmt.Println("Error syncing file: ", err)
	}
	fmt.Println("Write Completed")
}
