FROM alpine:3.8

RUN addgroup -g 2000 -S bitworking && \
    adduser -u 2000 -S bitworking -G bitworking && \
    apk update && apk add --no-cache ca-certificates

USER bitworking:bitworking

COPY . /

ENTRYPOINT ["/webmention"]
