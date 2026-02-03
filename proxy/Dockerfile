FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata curl bash &&\
    update-ca-certificates
COPY bin/proxy /usr/local/bin/proxy
