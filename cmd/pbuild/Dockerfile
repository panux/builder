FROM golang:alpine as builder
WORKDIR /root
RUN apk --no-cache add make git gcc libc-dev
COPY . /go/src/github.com/panux/builder
RUN go build --ldflags '-linkmode external -extldflags "-static"' -o pbuild github.com/panux/builder/cmd/pbuild
RUN (cd /go/src/github.com/panux/builder/static && make)

FROM alpine
RUN apk --no-cache add git
COPY --from=builder /root/pbuild /usr/bin/pbuild
COPY --from=builder /go/src/github.com/panux/builder/static /srv/static
COPY cmd/pbuild/config.json /srv/config.json
ENTRYPOINT ["pbuild","-config","/srv/config.json"]
