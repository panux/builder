FROM alpine:3.7 as bbuilder
RUN apk --no-cache add make gcc binutils curl libc-dev linux-headers
WORKDIR /root
RUN curl https://busybox.net/downloads/busybox-1.28.2.tar.bz2 | tar -xjf - && mv busybox-1.28.2 busybox
RUN make -C busybox defconfig && \
    sed -i -e 's/# CONFIG_FEATURE_SH_STANDALONE is not set/CONFIG_FEATURE_SH_STANDALONE=y/g' busybox/.config
RUN make -C busybox -j10 LDFLAGS="-static -s"

FROM scratch
COPY --from=bbuilder /root/busybox/busybox /usr/bin/busybox
COPY worker /usr/bin/worker
COPY cmd/worker/bootstrap.sh /root/bootstrap.sh
RUN ["busybox", "mkdir", "/tmp"]
RUN ["busybox", "ln", "-s", "/usr/bin", "/bin"]
RUN ["busybox", "ln", "-s", "/usr/bin/busybox", "/usr/bin/sh"]
ENV PATH="/usr/lib/ccache/bin:${PATH}"
ENV CCACHE_DIR=/ccache
ENTRYPOINT ["worker"]
