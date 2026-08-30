#!/usr/bin/env bash
# ============================================================
# daohang Linux 启动脚本
# 用法：在发布目录执行  ./start.sh
# 会读取同目录下的 .env 文件（若存在）作为环境变量后启动
# ============================================================
set -e

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

# 1. 读取 .env（格式：KEY=VALUE，# 开头为注释）
if [ -f "$ROOT/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$ROOT/.env"
  set +a
else
  echo "WARN: .env not found, using built-in defaults."
fi

# 2. 定位可执行文件（优先 linux 版本）
EXE=$(find "$ROOT" -maxdepth 1 -type f -name 'daohang-linux-*' | sort | head -n 1)
if [ -z "$EXE" ]; then
  echo "ERROR: daohang-linux-* binary not found in $ROOT. Run build.ps1 first or copy the binary here."
  exit 1
fi

chmod +x "$EXE"

echo "Starting: $(basename "$EXE")"
exec "$EXE"
