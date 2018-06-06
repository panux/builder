FROM golang:alpine as builder
WORKDIR /root
RUN apk --no-cache add make git gcc libc-dev && \
    go get github.com/blang/semver github.com/panux/go-makefile golang.org/x/tools/godoc/vfs gopkg.in/yaml.v2
COPY pkgen /go/src/github.com/panux/builder/pkgen
COPY cmd/dlserver/main.go main.go
RUN go build -o dlserver --ldflags '-linkmode external -extldflags "-static"'

FROM scratch
COPY --from=builder /root/dlserver /usr/bin/dlserver
ENTRYPOINT ["dlserver"]
