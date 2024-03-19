package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pooyaht/http_downloader/downloader"
)

func main() {
	args := os.Args[1:]
	if len(args) < 3 {
		fmt.Println("Usage : go run main.go target_server_ip port filename")
		os.Exit(1)
	}

	target_server_ip := args[0]
	port, err := strconv.Atoi(args[1])
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}

	filename := args[2]
	downloader := downloader.NewHttpDownloader(target_server_ip, port)
	err = downloader.Download(filename)
	if err != nil {
		fmt.Println("Error: ", err)
	}
}
