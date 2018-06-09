FROM golang:1.10-alpine
RUN apk --no-cache add make git gcc libc-dev docker
COPY . /go/src/github.com/panux/builder
WORKDIR /go/src/github.com/panux/builder
ENTRYPOINT ["./scripts/build.sh"]
