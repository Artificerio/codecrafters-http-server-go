package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
)

var directory = flag.String("directory", "/tmp/", "directory to search files in")

const (
	network = "tcp"
)

const (
	Empty200 = "HTTP/1.1 200 OK\r\n\r\n"
	Empty201 = "HTTP/1.1 201 Created\r\n\r\n"
	Empty400 = "HTTP/1.1 400 Bad Request\r\n\r\n"
	Empty404 = "HTTP/1.1 404 Not Found\r\n\r\n"
	Empty405 = "HTTP/1.1 405 Method Not Allowed\r\n\r\n"
	Empty500 = "HTTP/1.1 405 Internal Server Error\r\n\r\n"
)

var codeToStatus = map[int]string{
	200: "OK",
	201: "Created",
	400: "Bad Request",
	404: "Not Found",
	405: "Method Not Allowed",
}

type Server struct {
	l    net.Listener
	wg   *sync.WaitGroup
	info ServerInfo
}

type ServerInfo struct {
	host string
	port string
}

type Option func(*Server)

func WithHost(host string) Option {
	return func(s *Server) {
		s.info.host = host
	}
}

func WithPort(port string) Option {
	return func(s *Server) {
		s.info.port = port
	}
}

func WithWg(wg *sync.WaitGroup) Option {
	return func(s *Server) {
		s.wg = wg
	}
}

func NewServer(options ...Option) *Server {
	s := &Server{}
	for _, opt := range options {
		opt(s)
	}

	l, err := net.Listen(network, net.JoinHostPort(s.info.host, s.info.port))
	if err != nil {
		log.Println(err)
		return nil
	}
	s.l = l

	return s
}

func (s *Server) serve(transferErrChan chan<- error) {
	defer s.wg.Done()

	for {
		conn, err := s.l.Accept()
		if err != nil {
			transferErrChan <- err
		} else {
			s.wg.Add(1)
			go s.handleConn(conn, transferErrChan)
		}
	}
}

func (s *Server) handleConn(conn net.Conn, transferErrChan chan<- error) {
	defer s.wg.Done()
	defer func() {
		err := conn.Close()
		if err != nil {
			transferErrChan <- err
		}
	}()

	r := bufio.NewReader(conn)
	str, err := r.ReadString('\n')
	if err != nil {
		_, err := conn.Write([]byte(Empty400))
		if err != nil {
			transferErrChan <- err
			return
		}
		transferErrChan <- err
	}

	statusParts := strings.Fields(str)
	method := statusParts[0]

	urlParts := strings.Split(statusParts[1], "/")

	var path, target string
	path = urlParts[1]
	if len(urlParts) > 2 {
		target = urlParts[2]
	}

	headers := make(map[string]string)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			_, err := conn.Write([]byte(Empty400))
			if err != nil {
				transferErrChan <- err
				return
			}
			return
		}
		if line == "\r\n" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	n, _ := strconv.Atoi(headers["Content-Length"])
	body := make([]byte, n)
	_, err = io.ReadFull(r, body)
	if err != nil {
		transferErrChan <- err
	}

	log.Println("here:", method, path, target) // GET, files, foo

	switch path {
	case "":
		r := NewResponse(
			WithStatus(http.StatusOK),
			WithBody(nil),
			WithContentType("text/plain"),
		)
		err = r.writeResponse(conn)
		if err != nil {
			transferErrChan <- err
		}

	case "echo":
		r := handleEcho(headers, []byte(target))
		err = r.writeResponse(conn)
		if err != nil {
			transferErrChan <- err
		}
	case "user-agent":
		r := NewResponse(
			WithStatus(http.StatusOK),
			WithContentType("text/plain"),
			WithBody([]byte(headers["User-Agent"])),
		)
		err = r.writeResponse(conn)
		if err != nil {
			transferErrChan <- err
		}
	case "files":
		switch method {
		case "GET":
			path := *directory + target
			file, err := os.Open(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					conn.Write([]byte(Empty400))
				}
				transferErrChan <- err
				return
			}
			fileContents, err := io.ReadAll(file)
			if err != nil {
				transferErrChan <- err
			}
			r := NewResponse(
				WithStatus(http.StatusOK),
				WithBody(fileContents),
				WithContentType("application/octet-stream"),
			)
			err = r.writeResponse(conn)
			if err != nil {
				transferErrChan <- err
			}
		}
	default:
		r := NewResponse(
			WithStatus(http.StatusNotFound),
			WithBody(nil),
			WithContentType("text/plain"),
		)
		err = r.writeResponse(conn)
		if err != nil {
			transferErrChan <- err
		}
	}
}

func main() {
	log.Println("your programm logs will be printed here")

	flag.Parse()

	wg := &sync.WaitGroup{}
	transferErrChan := make(chan error)

	wg.Add(1)
	go handleStatus(wg, transferErrChan)

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt)

	srv := NewServer(
		WithHost("0.0.0.0"),
		WithPort("4221"),
		WithWg(wg),
	)

	wg.Add(1)
	go srv.serve(transferErrChan)
	<-stopCh
}

func handleStatus(wg *sync.WaitGroup, transferErrChan <-chan error) {
	defer wg.Done()
	for err := range transferErrChan {
		log.Println(err)
	}
}

type Response struct {
	status      int
	body        []byte
	compressed  bool
	encoding    string
	contentType string
}

type ResponseOption func(r *Response)

func WithStatus(status int) ResponseOption {
	return func(r *Response) {
		r.status = status
	}
}

func WithBody(body []byte) ResponseOption {
	return func(r *Response) {
		r.body = body
	}
}

func WithCompressed(compressed bool) ResponseOption {
	return func(r *Response) {
		r.compressed = compressed
	}
}

func WithEncoding(encoding string) ResponseOption {
	return func(r *Response) {
		r.encoding = encoding
	}
}

func WithContentType(contentType string) ResponseOption {
	return func(r *Response) {
		r.contentType = contentType
	}
}

func NewResponse(options ...ResponseOption) *Response {
	r := &Response{}
	for _, opt := range options {
		opt(r)
	}

	return r
}

func (r *Response) writeResponse(w io.Writer) error {
	var sb strings.Builder

	status := fmt.Sprintf("HTTP/1.1 %d %s\r\n", r.status, codeToStatus[r.status])
	sb.WriteString(status)

	if r.contentType != "" {
		contentType := fmt.Sprintf("Content-Type: %s\r\n", r.contentType)
		sb.WriteString(contentType)
	}

	if r.body != nil {
		contentLength := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(r.body))
		sb.WriteString(contentLength)
		sb.WriteString(string(r.body))
	} else {
		sb.WriteString("\r\n")
	}

	_, err := w.Write([]byte(sb.String()))
	if err != nil {
		return err
	}

	return nil
}

func handleEcho(headers map[string]string, body []byte) *Response {
	r := NewResponse(
		WithStatus(http.StatusOK),
		WithContentType("text/plain"),
		WithBody(body),
	)
	return r
}
