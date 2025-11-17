FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

ARG APT_PROXY

RUN set -eux; \
    if [ -n "$APT_PROXY" ]; then \
        echo "Using proxy: $APT_PROXY"; \
        export http_proxy="$APT_PROXY"; \
        export https_proxy="$APT_PROXY"; \
        env | grep -i proxy || true; \
    fi; \
    apt-get update; \
    apt-get install -y --no-install-recommends \
        ffmpeg \
        ca-certificates; \
    rm -rf /var/lib/apt/lists/*


WORKDIR /app

COPY ./bin/uploader2 /usr/local/bin/uploader2

RUN chmod +x /usr/local/bin/uploader2

ENTRYPOINT ["/usr/local/bin/uploader2"]
