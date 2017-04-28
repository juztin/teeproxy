# Need to first build a self contained binary for scratch:
#
#   ```bash
#   % CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o teeproxy
#   ```
#


FROM scratch

COPY teeproxy /

EXPOSE 8080

ENTRYPOINT ["/teeproxy"]
CMD ["--help"]
