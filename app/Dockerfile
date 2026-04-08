FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/fusion-search .

FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    libgomp1 \
    && rm -rf /var/lib/apt/lists/*

ARG ORT_VERSION=1.22.0
RUN mkdir -p /opt/onnxruntime && \
    curl -L --fail --retry 3 \
    "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-linux-x64-${ORT_VERSION}.tgz" \
    -o /tmp/onnxruntime.tgz && \
    tar -xzf /tmp/onnxruntime.tgz -C /tmp && \
    cp -r "/tmp/onnxruntime-linux-x64-${ORT_VERSION}/lib" /opt/onnxruntime/ && \
    rm -rf /tmp/onnxruntime*

ENV LD_LIBRARY_PATH=/opt/onnxruntime/lib
ENV FUSION_SEARCH_CONFIG=/app/config.yaml

COPY --from=builder /out/fusion-search /app/fusion-search
COPY config.yaml /app/config.yaml
COPY scripts/download_rerank_assets.sh /app/scripts/download_rerank_assets.sh

RUN chmod +x /app/scripts/download_rerank_assets.sh && /app/scripts/download_rerank_assets.sh

EXPOSE 9000

CMD ["/app/fusion-search"]
