#!/usr/bin/env bash
set -euo pipefail

MODEL_DIR="${MODEL_DIR:-/models/ms-marco}"
MODEL_URL="${MODEL_URL:-https://huggingface.co/cross-encoder/ms-marco-MiniLM-L12-v2/resolve/main/onnx/model_quint8_avx2.onnx}"
TOKENIZER_URL="${TOKENIZER_URL:-https://huggingface.co/cross-encoder/ms-marco-MiniLM-L12-v2/resolve/main/tokenizer.json}"
MODEL_FILE="${MODEL_FILE:-model_quint8_avx2.onnx}"
TOKENIZER_FILE="${TOKENIZER_FILE:-tokenizer.json}"

mkdir -p "${MODEL_DIR}"

echo "Downloading rerank model..."
curl -L --fail --retry 3 "${MODEL_URL}" -o "${MODEL_DIR}/${MODEL_FILE}"

echo "Downloading tokenizer..."
curl -L --fail --retry 3 "${TOKENIZER_URL}" -o "${MODEL_DIR}/${TOKENIZER_FILE}"

echo "Rerank assets ready:"
echo "  model: ${MODEL_DIR}/${MODEL_FILE}"
echo "  tokenizer: ${MODEL_DIR}/${TOKENIZER_FILE}"
