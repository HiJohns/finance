#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

BINARY_NAME="ironcore"
REMOTE_USER="ubuntu"
REMOTE_HOST="opencode.linxdeep.com"
REMOTE_PATH="~/finance"
PLOTTER_NAME="plotter.py"
COLLECTOR_NAME="collector.py"

SMTP_USER="${SMTP_USER:-linwx1978@qq.com}"
SMTP_PASS="${smtpPass:-}"
RECEIVER="${RECEIVER:-linwx1978@gmail.com}"
ADMIN_USER="${adminUser:-admin}"
ADMIN_PASS="${adminPass:-}"
SESSION_SECRET="${sessionSecret:-}"

GIT_VERSION=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LD_FLAGS="-X main.smtpUser=${SMTP_USER} -X main.smtpPass=${SMTP_PASS} -X main.receiver=${RECEIVER} -X main.AdminUser=${ADMIN_USER} -X main.AdminPass=${ADMIN_PASS} -X main.SessionSecret=${SESSION_SECRET} -X main.version=${GIT_VERSION}"

echo "=== IronCore Build Script ==="

if [ "$1" == "release" ]; then
    echo "🚀 开始发布流程：编译 Linux 版本..."

    echo "[1/3] 跨平台编译 (Target: Linux x86_64)..."
    go mod tidy
    GOOS=linux GOARCH=amd64 go build -ldflags "${LD_FLAGS}" -o ${BINARY_NAME}

    echo "[2/3] 正在上传至服务器 $REMOTE_HOST..."
    scp -i "$HOME/zeroSecond/aws/opencode.pem" ./${BINARY_NAME} ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/
    scp -i "$HOME/zeroSecond/aws/opencode.pem" ./${PLOTTER_NAME} ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/
    scp -i "$HOME/zeroSecond/aws/opencode.pem" ./${COLLECTOR_NAME} ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_PATH}/

    echo "[3/3] 安装 Python 依赖 (远程)..."
    ssh -i "$HOME/zeroSecond/aws/opencode.pem" ${REMOTE_USER}@${REMOTE_HOST} "cd ${REMOTE_PATH} && pip3 install efinance yfinance matplotlib --quiet 2>/dev/null || true"

    echo "✅ 发布完成！二进制文件已存放在服务器 ${REMOTE_PATH} 目录下。"
    echo "提示：请记得在服务器上重启服务以应用更新。"

else
    echo "🧪 开始本地测试流程..."

    echo "[1/4] 安装 Go 依赖..."
    go mod tidy

    echo "[2/4] 本地编译 (Mac)..."
    go build -ldflags "${LD_FLAGS}" -o ${BINARY_NAME} .

    echo "[3/4] 检查 Python 依赖..."
    echo "Required: efinance, yfinance, matplotlib"
    if command -v pip3 &> /dev/null; then
        pip3 install efinance yfinance matplotlib --quiet 2>/dev/null || echo "Note: Some packages may need manual install"
    fi

    echo "[4/4] 创建运行时目录..."
    mkdir -p data

    echo ""
    echo "=== Build Complete ==="
    echo ""
    echo "启动命令："
    echo "  # 数据采集 (terminal 1)"
    echo "  python3 collector.py"
    echo ""
    echo "  # 审计引擎 (terminal 2)"
    echo "  ./ironcore -port 9070"
    echo ""
    echo "访问仪表盘: http://localhost:9070"
    echo ""
fi