FROM golang:alpine as builder
WORKDIR /root
COPY . /go/src/github.com/panux/builder
RUN apk --no-cache add make git gcc libc-dev && \
    (cd /go/src/github.com/panux/builder && go get ./...)
RUN go build github.com/panux/builder/cmd/pbuild -o pbuild --ldflags '-linkmode external -extldflags "-static"'

FROM alpine
RUN apk --no-cache git
COPY --from=builder /root/pbuild /usr/bin/pbuild
COPY cmd/pbuild/config.json /srv/config.json
ENTRYPOINT ["pbuild","-config","/srv/config.json"]
