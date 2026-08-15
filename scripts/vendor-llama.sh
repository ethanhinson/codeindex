#!/usr/bin/env bash
# Vendors a pinned llama.cpp revision and builds CPU-only static libraries
# that internal/embed/local links via CGO. Run once per checkout (and after
# bumping LLAMA_PIN). Requires: git, cmake, a C/C++ toolchain.
set -euo pipefail

LLAMA_PIN="${LLAMA_PIN:-b6100}" # pinned upstream tag; bump deliberately
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DST="$ROOT/third_party/llama.cpp"

if [ ! -d "$DST/.git" ]; then
  git clone --depth 1 --branch "$LLAMA_PIN" \
    https://github.com/ggml-org/llama.cpp "$DST" 2>/dev/null ||
    git clone --depth 1 https://github.com/ggml-org/llama.cpp "$DST"
fi
cd "$DST"
git rev-parse HEAD > "$ROOT/third_party/LLAMA_REVISION"

# CPU-only, static, minimal surface: no Metal/CUDA (portability), no tools.
cmake -B build \
  -DCMAKE_BUILD_TYPE=Release \
  -DBUILD_SHARED_LIBS=OFF \
  -DGGML_METAL=OFF \
  -DGGML_CUDA=OFF \
  -DGGML_VULKAN=OFF \
  -DLLAMA_CURL=OFF \
  -DLLAMA_BUILD_TESTS=OFF \
  -DLLAMA_BUILD_EXAMPLES=OFF \
  -DLLAMA_BUILD_TOOLS=OFF \
  -DLLAMA_BUILD_SERVER=OFF
cmake --build build -j "$(getconf _NPROCESSORS_ONLN)" --target llama

echo "static libs:"
find build -name '*.a' -maxdepth 4
