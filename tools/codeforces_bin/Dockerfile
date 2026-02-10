FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /data

# Keep the binary data in the second-to-last layer.
COPY cmd/cf_case/data/codeforces_problems.bin /data/codeforces_problems.bin

# Final layer: the prebuilt binary.
COPY bin/cf_case /usr/local/bin/cf_case

ENTRYPOINT ["/usr/local/bin/cf_case"]
