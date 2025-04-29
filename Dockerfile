###########################################################
FROM golang:1.24.2 AS builder

WORKDIR /build
COPY authorization-bearer.go .
# Ollama base image has no libc, build the tool with static linking instead!
RUN go build -tags "netgo" -o authorization-bearer authorization-bearer.go

###########################################################
FROM ollama/ollama:0.6.6

# Create a local non-root user.
RUN useradd -m -s /bin/bash user

WORKDIR /addon
RUN export DEBIAN_FRONTEND=noninteractive \
    && apt-get update -qq \
    && apt-get install -qq --no-install-recommends supervisor nginx \
    && mkdir -p /etc/nginx/ssl \
    && rm /etc/nginx/sites-enabled/default \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /root/.cache
COPY --from=builder /build/authorization-bearer authorization-bearer
COPY --chmod=555 startup.sh .
COPY etc/supervisor/*.supervisord.conf /etc/supervisor/conf.d/
COPY etc/nginx/nginx-log.conf /etc/nginx/conf.d/00-log.conf
COPY etc/nginx/nginx.conf /etc/nginx/conf.d/01-default.conf

ENTRYPOINT ["/addon/startup.sh"]
CMD []
