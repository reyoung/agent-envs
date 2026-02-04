FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /data

# Keep the DB in the second-to-last layer.
COPY ioi_data.db /data/ioi_data.db

# Final layer: the prebuilt binary.
COPY bin/ioi_db_lookup /usr/local/bin/ioi_db_lookup
