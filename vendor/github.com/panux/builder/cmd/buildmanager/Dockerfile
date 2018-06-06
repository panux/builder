FROM golang:alpine as builder
WORKDIR /root
RUN apk --no-cache add make git gcc libc-dev
COPY . /go/src/github.com/panux/builder
RUN go build -o buildmanager --ldflags '-linkmode external -extldflags "-static"' github.com/panux/builder/cmd/buildmanager

FROM scratch
COPY --from=builder /root/buildmanager /usr/bin/buildmanager
ENTRYPOINT ["buildmanager"]
