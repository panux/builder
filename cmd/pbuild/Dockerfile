FROM alpine
COPY ca-certificates.crt /etc/ssl/certs/
RUN apk --no-cache add git
COPY pbuild /usr/bin/pbuild
COPY static /srv/static
COPY cmd/pbuild/config.json /srv/config.json
ENTRYPOINT ["pbuild","-config","/srv/config.json"]
