package main

import (
	"bytes"
	"crypto/tls"
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
	listen         = flag.String("l", ":8080", "port to accept requests")
	tlsKey         = flag.String("key.file", "", "path to the TLS private key file")
	tlsCertificate = flag.String("cert.file", "", "path to the TLS certificate file")

	targetHost          = flag.String("a", "localhost:8080", "where target traffic goes. http://localhost:8080/")
	targetHostRewrite   = flag.Bool("a.rewrite", false, "rewrite the host header when proxying target traffic")
	targetTimeout       = flag.Int("a.timeout", 3, "timeout in seconds for target traffic")
	isTargetTLS         = flag.Bool("a.tls", false, "proxies to target over TLS")
	isTargetTLSInsecure = flag.Bool("a.tls.insecure", false, "ignores certificate checking on target")

	alternateHost          = flag.String("b", "localhost:8081", "where alternate traffic goes, response is ignored. http://localhost:8081/")
	alternateHostRewrite   = flag.Bool("b.rewrite", false, "rewrite the host header when proxying alternate site traffic")
	alternateTimeout       = flag.Int("b.timeout", 3, "timeout in seconds for alternate site traffic")
	isAlternateTLS         = flag.Bool("b.tls", false, "proxies to alternate over TLS")
	isAlternateTLSInsecure = flag.Bool("b.tls.insecure", false, "ignores certificate checking on alternate")
)

func main() {
	flag.Parse()
	l, err := listener()
	if err != nil {
		fmt.Printf("Failed to listen on %s, %s\n", *listen, err)
		return
	}
	fmt.Printf("Listening on %s and proxying to %s / %s\n", *listen, *targetHost, *alternateHost)
	http.Serve(l, http.HandlerFunc(handler))
}

// listener returns either an HTTP or HTTPS listener.
func listener() (net.Listener, error) {
	if *tlsKey == "" {
		return net.Listen("tcp", *listen)
	}

	cert, err := tls.LoadX509KeyPair(*tlsCertificate, *tlsKey)
	if err != nil {
		return nil, err
	}
	config := &tls.Config{Certificates: []tls.Certificate{cert}}
	return tls.Listen("tcp", *listen, config)
}

// recovery is pretty much copied from golang:net/http/server.go
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

// handler duplicates the incoming request across the target and alternate, discarding the alternates response.
func handler(w http.ResponseWriter, r *http.Request) {
	defer recovery(r)
	targetRequest, alternateRequest, err := proxiedRequests(r)

	// alternate request
	go func() {
		defer recovery(r)
		_, err := request(*alternateHost, *isAlternateTLS, *isAlternateTLSInsecure, alternateRequest)
		if err != nil {
			log.Printf("Failed to receive from alternate %s, %v\n", *alternateHost, err)
		}
	}()

	// target request
	resp, err := request(*targetHost, *isTargetTLS, *isTargetTLSInsecure, targetRequest)
	if err != nil {
		log.Printf("Failed to receive from target %s, %v\n", *targetHost, err)
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

// copyRequest copyies the given request using the given body, re-writing the host when rewriteHost is true.
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

// proxiedRequests creates the `target` and `alternate` requests from the given request.
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
func request(host string, useTLS bool, insecureSkip bool, r *http.Request) (*http.Response, error) {
	tcpConn, err := net.DialTimeout("tcp", host, time.Duration(time.Duration(*alternateTimeout)*time.Second))
	if err != nil {
		return nil, err
	}

	if useTLS {
		//var config tls.Config
		config := &tls.Config{
			InsecureSkipVerify: true,
		}
		tcpConn = tls.Client(tcpConn, config)
		err = tcpConn.(*tls.Conn).Handshake()
	}

	var resp *http.Response
	conn := httputil.NewClientConn(tcpConn, nil)
	err = conn.Write(r)
	if err == nil {
		resp, err = conn.Read(r)
	}
	conn.Close()
	if err == httputil.ErrPersistEOF {
		err = nil
	}
	return resp, err
}
