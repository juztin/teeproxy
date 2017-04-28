package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"runtime"
	"time"
)

var (
	listen               = flag.String("l", ":8080", "port to accept requests")
	targetHost           = flag.String("a", "localhost:8080", "where target traffic goes. http://localhost:8080/")
	alternateHost        = flag.String("b", "localhost:8081", "where alternate traffic goes, response is ignored. http://localhost:8081/")
	targetTimeout        = flag.Int("a.timeout", 3, "timeout in seconds for target traffic")
	alternateTimeout     = flag.Int("b.timeout", 3, "timeout in seconds for alternate site traffic")
	targetHostRewrite    = flag.Bool("a.rewrite", false, "rewrite the host header when proxying target traffic")
	alternateHostRewrite = flag.Bool("b.rewrite", false, "rewrite the host header when proxying alternate site traffic")
)

type noopCloser struct {
	io.Reader
}

func (noopCloser) Close() error {
	return nil
}

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(runtime.NumCPU())

	var err error
	var listener net.Listener

	listener, err = net.Listen("tcp", *listen)
	if err != nil {
		fmt.Printf("Failed to listen to %s: %s\n", *listen, err)
		return
	}
	fmt.Printf("Listening on %s\n", *listen)
	http.Serve(listener, http.HandlerFunc(handler))
}

func recovery() {
	if r := recover(); r != nil {
		log.Println("Unsafe recovery: ", r)
	}
}

// handler duplicates the incoming request (req) and does the request to the Target and the Alternate target discarding the Alternate response
func handler(w http.ResponseWriter, req *http.Request) {
	defer recovery()
	targetRequest, alternateRequest, err := duplicateRequest(req)

	// Alternate request
	go func() {
		defer recovery()
		resp, err := request(*alternateHost, alternateRequest)
		if err != nil && err != httputil.ErrPersistEOF {
			log.Printf("Failed to receive from %s: %v\n", *alternateHost, err)
		}
		resp.Body.Close()
	}()

	// Target request
	resp, err := request(*targetHost, targetRequest)
	if err != nil && err != httputil.ErrPersistEOF {
		log.Printf("Failed to receive from %s: %v\n", *targetHost, err)
		return
	}

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	body, _ := ioutil.ReadAll(resp.Body)
	w.Write(body)
	resp.Body.Close()
}

func copyRequest(request *http.Request, body io.ReadCloser, rewriteHost bool, host string) *http.Request {
	r := http.Request{
		Method:        request.Method,
		URL:           request.URL,
		Proto:         request.Proto,
		ProtoMajor:    request.ProtoMajor,
		ProtoMinor:    request.ProtoMinor,
		Header:        request.Header,
		Body:          body,
		Host:          request.Host,
		ContentLength: request.ContentLength,
		Close:         true,
	}

	if rewriteHost {
		r.Host = host
	}

	return &r
}

func duplicateRequest(request *http.Request) (*http.Request, *http.Request, error) {
	// Duplicate the request body.
	b1 := new(bytes.Buffer)
	b2 := new(bytes.Buffer)
	w := io.MultiWriter(b1, b2)
	_, err := io.Copy(w, request.Body)
	request.Body.Close()

	// Duplicate the request, using the duplicated body for each.
	r1 := copyRequest(request, noopCloser{b1}, *targetHostRewrite, *targetHost)
	r2 := copyRequest(request, noopCloser{b2}, *alternateHostRewrite, *alternateHost)

	return r1, r2, err
}

// request invokes the request upon the host.
func request(host string, r *http.Request) (*http.Response, error) {
	tcpConn, err := net.DialTimeout("tcp", host, time.Duration(time.Duration(*alternateTimeout)*time.Second))
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	conn := httputil.NewClientConn(tcpConn, nil)
	err = conn.Write(r)
	if err == nil {
		resp, err = conn.Read(r)
	}
	conn.Close()
	return resp, err
}
