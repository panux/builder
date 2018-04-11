FROM golang:alpine as builder
WORKDIR /root
RUN apk --no-cache add make git gcc libc-dev && \
    go get github.com/blang/semver github.com/panux/go-makefile golang.org/x/tools/godoc/vfs gopkg.in/yaml.v2 golang.org/x/crypto/ssh golang.org/x/net/websocket k8s.io/client-go/...
COPY pkgen /go/src/github.com/panux/builder/pkgen
COPY bmapi /go/src/github.com/panux/builder/bmapi
COPY cmd/buildmanager/main.go main.go
RUN go build -o buildmanager --ldflags '-linkmode external -extldflags "-static"'

FROM scratch
COPY --from=builder /root/buildmanager /usr/bin/buildmanager
ENTRYPOINT ["buildmanager"]
