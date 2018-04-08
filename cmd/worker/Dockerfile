FROM alpine:3.7 as bbuilder
RUN apk --no-cache add make gcc binutils curl libc-dev linux-headers
WORKDIR /root
RUN curl https://busybox.net/downloads/busybox-1.28.2.tar.bz2 | tar -xjf - && mv busybox-1.28.2 busybox
RUN make -C busybox defconfig && \
    sed -i -e 's/# CONFIG_FEATURE_SH_STANDALONE is not set/CONFIG_FEATURE_SH_STANDALONE=y/g' busybox/.config
RUN make -C busybox -l6 -j10 LDFLAGS="-static -s"

FROM scratch as self-test
COPY --from=bbuilder /root/busybox/busybox /usr/bin/busybox
RUN ["busybox","sh","-c","echo hello"]

FROM golang:alpine as workerbuilder
WORKDIR /root
RUN apk --no-cache add make git gcc libc-dev && go get -v golang.org/x/crypto/ssh
COPY cmd/worker/worker.go worker.go
RUN go build -o worker --ldflags '-linkmode external -extldflags "-static"'

FROM scratch
COPY --from=bbuilder /root/busybox/busybox /usr/bin/busybox
COPY --from=workerbuilder /root/worker /usr/bin/worker
ENTRYPOINT ["worker"]
