package downloader

import (
	"bytes"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/pooyaht/http_downloader/writer"
)

const (
	DOWNLOAD_CHUNK_SIZE = 1024 * 1024 * 16
	MAX_WORKERS         = 8
	DOWNLOAD_PATH       = "data"
)

type HttpDownloader struct {
	server_addr string
	server_port int
	writer      *writer.FileWriter
	logger      *slog.Logger
}

func NewHttpDownloader(server_addr string, port int, logger *slog.Logger) *HttpDownloader {
	writer := writer.NewFileWriter(DOWNLOAD_PATH, logger)
	return &HttpDownloader{
		server_addr: server_addr,
		server_port: port,
		writer:      writer,
		logger:      logger,
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
		h.logger.Info(fmt.Sprintf("Parallel download using %d workers", num_workers))
		return h.parallelDownload(filename, content_length, num_workers)
	} else {
		h.logger.Info("Single download")
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

	wg.Add(max_workers)
	for i := 0; i < max_workers; i++ {
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
				h.logger.Error(fmt.Sprintf("Error downloading chunk %d: %s", chunk_no, err))
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
		h.writer.Write([]byte(resp.body), resp.offset, filename)
	}
	h.writer.Close()

	return nil
}

func (h *HttpDownloader) singleDownload(filename string) error {
	request := h.createRequest("GET", filename, nil)
	response, err := h.sendRequest(request)
	if err != nil {
		return err
	}
	body := parseHTTPResponse(string(response)).Body
	h.writer.Write([]byte(body), 0, filename)
	h.writer.Close()
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

	headers["Host"] = fmt.Sprintf("%s:%d", h.server_addr, h.server_port)
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
	h.logger.Info(fmt.Sprintf("Wrote to socket %d request: %s", socket_fd, request))
	response, err := h.readResponse(socket_fd)
	if err != nil {
		h.logger.Error(fmt.Sprintf("Error reading response corresponding to request: %s , error: %s", request, err))
	}
	return response, nil
}

func (h *HttpDownloader) readResponse(socket_fd int) ([]byte, error) {
	buf := make([]byte, 4096)
	var response bytes.Buffer
	h.logger.Info(fmt.Sprintf("Reading from socket %d", socket_fd))

	timeout := syscall.Timeval{Sec: 10, Usec: 0}
	err := syscall.SetsockoptTimeval(socket_fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &timeout)
	if err != nil {
		return nil, os.NewSyscallError("SetsockoptTimeval", err)
	}

	var last_num_bytes_read int
	for {
		n, err := syscall.Read(socket_fd, buf)
		//dirty hack to prevent keep-alive connections from waiting
		if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
			if last_num_bytes_read < len(buf) {
				h.logger.Info(fmt.Sprintf("Socket: [ %d ] read Completed", socket_fd))
				break
			}
			h.logger.Info(fmt.Sprintf("Socket: [ %d ] Timeout", socket_fd))
			break
		}
		if err != nil {
			return nil, os.NewSyscallError("Read", err)
		}
		if n == 0 {
			break
		}
		last_num_bytes_read = n
		response.Write(buf[:n])
	}
	return response.Bytes(), nil
}

func (h *HttpDownloader) socketContext(callback func(fd int) error) error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		return os.NewSyscallError("Socket", err)
	}
	defer syscall.Close(fd)

	server_ip := net.ParseIP(h.server_addr)
	server_addr := &syscall.SockaddrInet4{
		Port: h.server_port,
		Addr: [4]byte(server_ip.To4()),
	}
	err = syscall.Connect(fd, server_addr)
	if err != nil {
		return os.NewSyscallError("Connect", err)
	}

	err = callback(fd)
	if err != nil {
		return err
	}

	return nil
}
