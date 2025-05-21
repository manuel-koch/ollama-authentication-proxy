ARG GOLANG_VERSION
ARG OLLAMA_VERSION

###########################################################
FROM golang:$GOLANG_VERSION AS builder

WORKDIR /build
COPY *.go go.mod go.sum ./
# Ollama base image has no libc, build the tool with static linking instead!
RUN go build -tags "netgo" -o ollama-authentication-proxy .

###########################################################
FROM ollama/ollama:$OLLAMA_VERSION

# Create a local non-root user.
RUN useradd -m -s /bin/bash user

WORKDIR /addon
RUN export DEBIAN_FRONTEND=noninteractive \
    && apt-get update -qq \
    && apt-get install -qq --no-install-recommends supervisor \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /root/.cache
COPY --from=builder /build/ollama-authentication-proxy ollama-authentication-proxy
COPY --chmod=555 startup.sh .
COPY etc/supervisor/*.supervisord.conf /etc/supervisor/conf.d/

ENTRYPOINT ["/addon/startup.sh"]
CMD []
