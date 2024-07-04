package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
)

var methods = []string{"GET", "POST", "PUT", "DELETE"}
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

func (s *Server) serve(transferErrChan chan<- error, doneChan chan<- string) {
	defer s.wg.Done()

	for {
		conn, err := s.l.Accept()
		if err != nil {
			transferErrChan <- err
		} else {
			s.wg.Add(1)
			go s.handleConn(conn, transferErrChan, doneChan)
		}
	}
}

func (s *Server) handleConn(conn net.Conn, transferErrChan chan<- error, doneChan chan<- string) {
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
	requestTarget := strings.Split(statusParts[1], "/")[1]
	log.Println(method, requestTarget)

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

	conn.Write([]byte(Empty200))
}

func main() {
	log.Println("your programm logs will be printed here")

	flag.Parse()

	wg := &sync.WaitGroup{}
	transferErrChan := make(chan error)
	doneChan := make(chan string)
	wg.Add(1)
	go handleStatus(wg, transferErrChan, doneChan)

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt)

	srv := NewServer(
		WithHost("0.0.0.0"),
		WithPort("4221"),
		WithWg(wg),
	)

	wg.Add(1)
	go srv.serve(transferErrChan, doneChan)
	<-stopCh
}

func handleStatus(wg *sync.WaitGroup, transferErrChan <-chan error, doneChan <-chan string) {
	defer wg.Done()
	doneCnt := 0
	for {
		select {
		case err := <-transferErrChan:
			log.Println("an error has occured", err)
		case status := <-doneChan:
			doneCnt++
			msg := fmt.Sprintf("%v|%v\n", status, doneCnt)
			log.Println(msg)
		}
	}
}
