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

	switch requestTarget {
	}
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

	// 	wg := &sync.WaitGroup{}
	// 	for {
	// 		c, err := l.Accept()
	// 		if err != nil {
	// 			log.Println("closing server")
	// 			break
	// 		}
	// 		wg.Add(1)
	// 		go handleConnection(wg, c)
	// 	}

	// 	wg.Wait()
	// }

	// func handleConnection(wg *sync.WaitGroup, c net.Conn) {
	// 	defer func() {
	// 		err := c.Close()
	// 		if err != nil {
	// 			log.Println(err)
	// 		}
	// 	}()
	// 	defer wg.Done()
	// 	buf := make([]byte, 4096)
	// 	_, err := c.Read(buf)
	// 	if err != nil {
	// 		log.Println("error reading data from connection")
	// 		return
	// 	}
	// 	requestMap := parseRequest(string(buf))

	// 	for _, method := range methods {
	// 		if _, ok := requestMap[method]; ok {
	// 			handleRequest(c, requestMap, method)
	// 		}
	// 	}

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

// func handleRequest(c net.Conn, requestMap map[string][]string, method string) {
// 	requestTarget := requestMap[method][0]
// 	switch {
// 	case strings.HasPrefix(requestTarget, "/files/"):
// 		switch method {
// 		case "GET":
// 			filename := strings.TrimPrefix(requestTarget, "/files/")
// 			file, err := os.Open(*directory + filename)
// 			defer func() {
// 				err := file.Close()
// 				if err != nil {
// 					log.Println(err)
// 					return
// 				}
// 			}()
// 			if err != nil {
// 				if errors.Is(err, os.ErrNotExist) {
// 					c.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
// 					return
// 				}
// 			}
// 			fileData, err := io.ReadAll(file)
// 			if err != nil {
// 				log.Println(err)
// 				return
// 			}
// 			c.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %v\r\n\r\n%v", len(fileData), string(fileData))))
// 		case "POST":
// 			filename := strings.TrimPrefix(requestTarget, "/files/")
// 			file, err := os.Create(*directory + filename)
// 			defer func() {
// 				err := file.Close()
// 				if err != nil {
// 					log.Println(err)
// 					return
// 				}
// 			}()
// 			if err != nil {
// 				if errors.Is(err, os.ErrExist) {
// 					c.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
// 					return
// 				}
// 			}
// 			data := requestMap["Request-Body:"]
// 			_, err = file.WriteString(strings.Join(data, " "))
// 			if err != nil {
// 				log.Println(err)
// 				return
// 			}
// 			c.Write([]byte("HTTP/1.1 201 Created\r\n\r\n"))
// 		}
// 	case strings.HasPrefix(requestTarget, "/echo/"):
// 		resp := strings.TrimPrefix(requestTarget, "/echo/")
// 		c.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %v\r\n\r\n%v", len(resp), resp)))
// 	case strings.HasPrefix(requestTarget, "/user-agent"):
// 		resp := requestMap["User-Agent:"][0]
// 		c.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %v\r\n\r\n%v", len(resp), resp)))
// 	default:
// 		if requestTarget != "/" {
// 			c.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
// 		} else {
// 			c.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
// 		}
// 	}
// }

// func parseRequest(r string) map[string][]string {
// 	requestData := strings.Split(string(r), "\r\n")
// 	requestMap := make(map[string][]string)
// 	for _, field := range requestData {
// 		headerField := ""
// 		for i, str := range strings.Fields(field) {
// 			if i == 0 {
// 				headerField = str
// 				continue
// 			}
// 			requestMap[headerField] = append(requestMap[headerField], str)
// 		}
// 	}
// 	idx := strings.LastIndex(r, "\r\n")
// 	body := ""
// 	if idx+2 < len(r) {
// 		body = r[idx+2:]
// 	}
// 	requestMap["Request-Body:"] = strings.Fields(string(bytes.Trim([]byte(body), "\x00")))
// 	return requestMap
// }
