#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== IronCore Build Script ==="

echo "[1/4] Installing Go dependencies..."
go mod tidy

echo "[2/4] Building Go binary..."
SMTP_USER="${SMTP_USER:-}"
SMTP_PASS="${SMTP_PASS:-}"
RECEIVER="${RECEIVER:-}"

LDFLAGS="-X main.smtpUser=${SMTP_USER} -X main.smtpPass=${SMTP_PASS} -X main.receiver=${RECEIVER}"
go build -ldflags "${LDFLAGS}" -o ironcore .

echo "[3/4] Checking Python dependencies..."
echo "Required Python packages:"
echo "  - efinance (for A-share data)"
echo "  - yfinance (for US stock/macro data)"
echo "  - matplotlib (for chart generation)"

if command -v pip3 &> /dev/null; then
    echo "Installing Python dependencies..."
    pip3 install efinance yfinance matplotlib --quiet 2>/dev/null || echo "Note: Some packages may require manual installation"
fi

echo "[4/4] Creating runtime directories..."
mkdir -p data

echo ""
echo "=== Build Complete ==="
echo ""
echo "To run the service:"
echo "  # Start data collector (terminal 1)"
echo "  python3 collector.py"
echo ""
echo "  # Start audit engine (terminal 2)"  
echo "  ./ironcore -port 8080"
echo ""
echo "Access dashboard: http://localhost:8080"
echo ""