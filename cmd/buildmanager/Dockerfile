FROM golang:alpine as builder
WORKDIR /root
RUN apk --no-cache add make git gcc libc-dev
COPY cmd/buildmanager/main.go /go/src/github.com/panux/builder/cmd/buildmanager/main.go
COPY pkgen /go/src/github.com/panux/builder/pkgen
COPY internal /go/src/github.com/panux/builder/internal
RUN (cd /go/src/github.com/panux/builder/cmd/buildmanager && go get ./...) && \
    go build -o buildmanager --ldflags '-linkmode external -extldflags "-static"' github.com/panux/builder/cmd/buildmanager

FROM scratch
COPY --from=builder /root/buildmanager /usr/bin/buildmanager
ENTRYPOINT ["buildmanager"]
