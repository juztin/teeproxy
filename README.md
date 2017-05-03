# teeproxy

A reverse HTTP proxy that duplicates requests.

### Why you may need this?
You may have production servers running, but you need to upgrade to a new system. You want to run A/B test on both old and new systems to confirm the new system can handle the production load, and want to see whether the new system can run in shadow mode continuously without any issue.

### How it works?
teeproxy is a reverse HTTP proxy. For each incoming request, it clones the request into 2 requests, forwards them to 2 servers. The results from server A are returned as usual, but the results from server B are ignored.

teeproxy handles GET, POST, and all other http methods.

## Build
```bash
go build
```

## Install
```bash
go install github.com/juztin/teeproxy
```

## Docker

#### Scratch
_(Using scratch will only work when not proxying to TLS endpoints, as we're relying on the cert store of the OS)_

To be able to use the binary from within a `scratch` Docker image, we need to build a static binary _(no CGO)_.

```bash
% CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o teeproxy .
```

Then build the image like usual

```bash
% docker build -t teeproxy .
```

#### Alpine (Other)

```bash
% docker build -t teeproxy .
```

## Usage

```bash
 ./teeproxy -l :8888 -a localhost:800 -b localhost:8001
```
 `-l` specifies the listening port. `-a` and `-b` are meant for system A and B. The B system can be taken down or started up without causing any issue to the teeproxy.

##### full example

```bash
 ./teeproxy \
	-l :8080 \
	-a 192.168.100.100 \
	-b 192.168.100.200 \
	-b.tls \
	-b.tls.insecure
```

### Docker

```bash
 docker run \
	-it \
	--rm \
	--publish 8080:8080 \
	minty/teeproxy \
		-l :8080 \
		-a 192.168.100.100 \
		-b 192.168.100.200 \
		-b.tls \
		-b.tls.insecure
```


#### Configuring timeouts ####
It's also possible to configure the timeout to both systems
*  `-a.timeout int`: timeout in seconds for production traffic (default `3`)
*  `-b.timeout int`: timeout in seconds for alternate site traffic (default `3`)

#### Configuring host header rewrite ####
Optionally rewrite host value in the http request header.
*  `-a.rewrite bool`: rewrite for production traffic (default `false`)
*  `-b.rewrite bool`: rewrite for alternate site traffic (default `false`)

#### Configuring TLS
If you need teeproxy to listen on a TLS socket.
*  `-cert.file server.crt`: the name of the certificate file (default ` `)
*  `-key.file server.key`: the name of the key file (default ` `)

#### Proxying to TLS
If you need teeproxy to proxy to a TLS host.
*  `-a.tls`: specifies that the proxy should use a TLS connection to `a`

#### Proxying to TLS, while ignoreing certificate checks
If you need teeproxy to proxy to an insecure TLS host _(self signed certificate)_.
*  `-a.tls`: specifies that the proxy should use a TLS connection to `a`
*  `-a.tls.insecure`: ignores the certificate validtion of the `a` host
