#!/bin/bash
# ==============================================================================
# FinanceQA (OpenClaw Plugin) 线上部署脚本
# 作用: 编译 FinanceQA 后端并发送至目标服务器进行原生部署
# ==============================================================================

set -euo pipefail

# --- 配置区 ---
: "${SERVER:?Set SERVER to your SSH target, for example deploy@finance-host}"
: "${KEY:?Set KEY to your private key path}"
REMOTE_HOME="$(ssh -i "$KEY" "$SERVER" 'printf %s "$HOME"')"
REMOTE_DIR="${REMOTE_DIR:-$REMOTE_HOME/finance_qa}"
REMOTE_FINANCEQA_BIN="${REMOTE_FINANCEQA_BIN:-$REMOTE_DIR/bin/financeqa}"
PACKAGE_NAME="finance_qa_plugin"
# -------------

echo "🚀 [1/4] 开始 FinanceQA 跨平台编译..."
# 使用 CGO_ENABLED=1 时交叉编译比较麻烦，为了保证在服务端完美运行，如果本地 Mac 编译 Linux 带 cgo 遇到阻碍，
# 我们可以选择两种方式：
# 1. 源码上传，让服务端自己去 go build
# 2. 纯静态构建或确保本地能交叉编译 CGO
# 由于对方是标准的 Linux 服务器，最安全无痛的做法是：打包源码让远端自动拉库编译，反正 Go 编译只需 3 秒钟。

rm -rf /tmp/${PACKAGE_NAME}_deploy.tar.gz
cd "$(dirname "$0")/.."

echo "📦 [2/4] 打包源码与核心资产..."
# 将当前需要的文件打包到 tmp
tar --exclude='.git' \
    --exclude='.tools' \
    --exclude='test_data' \
    --exclude='archive' \
    --exclude='.DS_Store' \
    --exclude='uploads' \
    -czf /tmp/${PACKAGE_NAME}_deploy.tar.gz .

echo "📤 [3/4] 上传包至服务器 ($SERVER)..."
scp -i "$KEY" /tmp/${PACKAGE_NAME}_deploy.tar.gz $SERVER:/tmp/

echo "🛠️ [4/4] 远程服务器接管：解压与环境初始化..."
ssh -i "$KEY" $SERVER << REMOTE_SCRIPT
    echo ">> 正在清理远端目录 $REMOTE_DIR..."
    mkdir -p $REMOTE_DIR
    cd $REMOTE_DIR
    
    # 因为安全原因，不会强行清掉远端可能有用的数据（如果他们自己在远端存了 sqlite 表）
    # 解压缩新版资产覆盖
    tar xzf /tmp/${PACKAGE_NAME}_deploy.tar.gz
    
    echo ">> 检查 Go 环境并开始编译安装..."
    if ! command -v go &> /dev/null; then
        echo "⚠️ 注意：远端服务器找不到 go 命令！这可能需要您手动安装 (apt install golang / wget golang.org)."
        exit 1
    fi

    # 开始针对服务器架构原生编译
    export GOPROXY=https://goproxy.cn,direct
    go mod tidy
    mkdir -p "$(dirname "$REMOTE_FINANCEQA_BIN")"
    go build -o "$REMOTE_FINANCEQA_BIN" ./cmd/financeqa/...
    rm -f "$REMOTE_DIR/financeqa"
    
    echo ">> (可选) 将 SKILL.md 注册进 OpenClaw..."
    echo "============================================="
    echo "✅ 部署大功告成！"
    echo "当前服务端运行库入口已就绪:"
    echo "$REMOTE_FINANCEQA_BIN query '今年人力成本多少'"
    echo "============================================="
REMOTE_SCRIPT

echo "🎉 本地推送脚本全部执行完毕。"
