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

func main() {
	flag.Parse()

	var err error
	var listener net.Listener

	listener, err = net.Listen("tcp", *listen)
	if err != nil {
		fmt.Printf("Failed to listen on %s, %s\n", *listen, err)
		return
	}
	fmt.Printf("Listening on %s\n", *listen)
	http.Serve(listener, http.HandlerFunc(handler))
}

func recovery(req *http.Request) {
	if r := recover(); r != nil {
		if err := recover(); err != nil && err != http.ErrAbortHandler {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			log.Printf("panic serving %s: %v\n%s\n", req.RemoteAddr, err, buf)
		}
	}
}

func proxyHandler(host string, r *http.Request) (*http.Response, error) {
	resp, err := request(host, r)
	if err != nil && err != httputil.ErrPersistEOF {
		log.Printf("Failed to receive from %s, %v\n", host, err)
	}
	return resp, nil
}

// handler duplicates the incoming request across the target and alternate, discarding the alternates response.
func handler(w http.ResponseWriter, r *http.Request) {
	defer recovery(r)
	targetRequest, alternateRequest, err := proxiedRequests(r)

	// alternate request
	go func() {
		defer recovery(r)
		proxyHandler(*alternateHost, alternateRequest)
	}()

	// target request
	resp, err := proxyHandler(*targetHost, targetRequest)
	if err != nil {
		log.Printf("Failed during request to target proxy: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body from target proxy: %v", err)
		return
	}
	resp.Body.Close()
	_, err = w.Write(body)
	if err != nil {
		log.Printf("Failed to write response body from target proxy: %v", err)
	}
}

//copyRequest copyies the given request using the given body, re-writing the host when rewriteHost is true.
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

//proxiedRequests creates the `target` and `alternate` requests from the given request.
func proxiedRequests(r *http.Request) (*http.Request, *http.Request, error) {
	// Duplicate the request body.
	b1 := new(bytes.Buffer)
	b2 := new(bytes.Buffer)
	w := io.MultiWriter(b1, b2)
	_, err := io.Copy(w, r.Body)
	r.Body.Close()

	// Duplicate the request, using the duplicated body for each.
	r1 := copyRequest(r, ioutil.NopCloser(b1), *targetHostRewrite, *targetHost)
	r2 := copyRequest(r, ioutil.NopCloser(b2), *alternateHostRewrite, *alternateHost)

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
