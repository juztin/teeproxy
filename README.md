teeproxy
=========

A reverse HTTP proxy that duplicates requests.

Why you may need this?
----------------------
You may have production servers running, but you need to upgrade to a new system. You want to run A/B test on both old and new systems to confirm the new system can handle the production load, and want to see whether the new system can run in shadow mode continuously without any issue.

How it works?
-------------
teeproxy is a reverse HTTP proxy. For each incoming request, it clones the request into 2 requests, forwards them to 2 servers. The results from server A are returned as usual, but the results from server B are ignored.

teeproxy handles GET, POST, and all other http methods.

Build
-------------
```
go build
```

Install
-------------
```
go install github.com/juztin/teeproxy
```

Usage
-------------
```
 ./teeproxy -l :8888 -a localhost:800 -b localhost:8001
```
 `-l` specifies the listening port. `-a` and `-b` are meant for system A and B. The B system can be taken down or started up without causing any issue to the teeproxy.

#### Configuring timeouts ####
It's also possible to configure the timeout to both systems
*  `-a.timeout int`: timeout in seconds for production traffic (default `3`)
*  `-b.timeout int`: timeout in seconds for alternate site traffic (default `3`)

#### Configuring host header rewrite ####
Optionally rewrite host value in the http request header.
*  `-a.rewrite bool`: rewrite for production traffic (default `false`)
*  `-b.rewrite bool`: rewrite for alternate site traffic (default `false`)
