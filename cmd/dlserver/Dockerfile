FROM golang:alpine as builder
WORKDIR /root
RUN apk --no-cache add make git gcc libc-dev
COPY . /go/src/github.com/panux/builder
RUN go build -o dlserver --ldflags '-linkmode external -extldflags "-static"' github.com/panux/builder/cmd/dlserver

FROM scratch
COPY --from=builder /root/dlserver /usr/bin/dlserver
ENTRYPOINT ["dlserver"]
