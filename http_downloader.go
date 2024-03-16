package main

import (
	"bytes"
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const (
	DOWNLOAD_CHUNK_SIZE = 1024 * 1024 * 16
	MAX_WORKERS         = 8
)

type HttpDownloader struct {
	server_addr *syscall.SockaddrInet4
	writer      *FileWriter
}

func NewHttpDownloader(server_ip net.IP, port int) *HttpDownloader {
	server_addr := &syscall.SockaddrInet4{
		Port: port,
		Addr: [4]byte(server_ip.To4()),
	}
	writer := newFileWriter("data")
	return &HttpDownloader{
		server_addr: server_addr,
		writer:      writer,
	}
}

func (h *HttpDownloader) Download(filename string) error {
	head_request := h.createRequest("HEAD", filename, nil)
	response, err := h.sendRequest(head_request)
	if err != nil {
		return err
	}

	response_str := string(response)
	parsed_response := parseHTTPResponse(response_str)
	content_length, _ := strconv.Atoi(parsed_response.Headers["Content-Length"])
	max_chunks := int(content_length / DOWNLOAD_CHUNK_SIZE)
	if max_chunks == 0 {
		max_chunks = 1
	}

	num_workers := int(math.Min(float64(max_chunks), float64(MAX_WORKERS)))
	if parsed_response.Headers["Accept-Ranges"] == "bytes" && num_workers > 1 {
		return h.parallelDownload(filename, content_length, num_workers)
	} else {
		return h.singleDownload(filename)
	}
}

func (h *HttpDownloader) parallelDownload(filename string, content_length int, max_workers int) error {
	type chanResponse struct {
		offset int64
		body   string
	}

	wg := &sync.WaitGroup{}
	responses := make(chan chanResponse, max_workers)
	chunk_size := int(math.Ceil(float64(content_length) / float64(max_workers)))

	for i := 0; i < max_workers; i++ {
		wg.Add(1)
		go func(chunk_no int) {
			defer wg.Done()

			start_byte := int64(chunk_no * chunk_size)
			end_byte := int64((chunk_no+1)*chunk_size - 1)
			if end_byte > int64(content_length) {
				end_byte = int64(content_length)
			}

			request := h.createRequest("GET", filename, map[string]string{
				"Range": fmt.Sprintf("bytes=%d-%d", start_byte, end_byte),
			})
			response, err := h.sendRequest(request)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}

			body := parseHTTPResponse(string(response)).Body
			responses <- chanResponse{body: body, offset: start_byte}
		}(i)
	}

	go func() {
		wg.Wait()
		close(responses)
	}()

	for resp := range responses {
		h.writer.write([]byte(resp.body), resp.offset, filename)
	}

	return nil
}

func (h *HttpDownloader) singleDownload(filename string) error {
	request := h.createRequest("GET", filename, nil)
	response, err := h.sendRequest(request)
	if err != nil {
		return err
	}
	body := parseHTTPResponse(string(response)).Body
	h.writer.write([]byte(body), 0, filename)
	return nil
}

type HTTPResponse struct {
	Headers map[string]string
	Body    string
}

func parseHTTPResponse(response string) HTTPResponse {
	headers := make(map[string]string)
	var body string

	parts := strings.SplitN(response, "\r\n\r\n", 2)
	if len(parts) == 2 {
		body = parts[1]
	}

	lines := strings.Split(parts[0], "\r\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := parts[1]
			headers[key] = value
		}
	}

	return HTTPResponse{Headers: headers, Body: body}
}

func (h *HttpDownloader) createRequest(method string, resource string, headers map[string]string) []byte {
	request := []byte(method + " /" + resource + " HTTP/1.1\r\n")
	if headers == nil {
		headers = make(map[string]string)
	}

	// TODO remove hardcoded address
	headers["Host"] = fmt.Sprintf("%s:%d", "127.0.0.1", h.server_addr.Port)
	for k, v := range headers {
		request = append(request, []byte(k+": "+v+"\r\n")...)
	}
	request = append(request, []byte("\r\n")...)
	return request
}

func (h *HttpDownloader) sendRequest(request []byte) ([]byte, error) {
	var response []byte
	err := h.socketContext(func(fd int) error {
		resp, err := h.sendRequestUtil(fd, request)
		if err == nil {
			response = resp
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}
func (h *HttpDownloader) sendRequestUtil(socket_fd int, request []byte) ([]byte, error) {
	_, err := syscall.Write(socket_fd, request)
	if err != nil {
		return nil, os.NewSyscallError("Write", err)
	}
	response, err := h.readResponse(socket_fd)
	if err != nil {
		fmt.Println("Error", err)
	}
	return response, nil
}

func (h *HttpDownloader) readResponse(socket_fd int) ([]byte, error) {
	buf := make([]byte, 4096)
	var response bytes.Buffer
	fmt.Println("[", socket_fd, "] Downloading")

	timeout := syscall.Timeval{Sec: 10, Usec: 0}
	err := syscall.SetsockoptTimeval(socket_fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &timeout)
	if err != nil {
		return nil, os.NewSyscallError("SetsockoptTimeval", err)
	}

	for {
		n, err := syscall.Read(socket_fd, buf)
		if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
			fmt.Println("[", socket_fd, "] Timeout")
			break
		}
		if err != nil {
			return nil, os.NewSyscallError("Read", err)
		}
		if n == 0 {
			break
		}
		response.Write(buf[:n])
	}
	fmt.Println("[", socket_fd, "] Complete")
	return response.Bytes(), nil
}

func (h *HttpDownloader) socketContext(callback func(fd int) error) error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		return os.NewSyscallError("Socket", err)
	}
	defer syscall.Close(fd)

	err = syscall.Connect(fd, h.server_addr)
	if err != nil {
		return os.NewSyscallError("Connect", err)
	}

	err = callback(fd)
	if err != nil {
		return err
	}

	return nil
}
