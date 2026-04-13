#!/usr/bin/env bash
# ==============================================================================
# 财务引擎打包脚本 (适配 OpenClaw 插件平台体系)
# 作用: 跨平台编译 Go 后端并打成标准插件压缩包
# ==============================================================================

set -e

# 设置基础目录
PACKAGE_NAME="finance_qa_plugin"
OUTPUT_DIR="dist/$PACKAGE_NAME"

echo "🧹 1. 清除旧版构建残留..."
rm -rf dist/

echo "🔨 2. 跨平台编译 CLI 双架构执行文件..."
mkdir -p "$OUTPUT_DIR/bin"

# 编译 Linux x64 (服务端最常见架构)
# CGO_ENABLED=1 because mattn/go-sqlite3 requires cgo. We'll compile natively or require gcc locally
echo ">> 编译适用你本机的二进制执行档 (默认带着 SQLite cgo 驱动)..."
go build -o "$OUTPUT_DIR/bin/financeqa" ./cmd/financeqa/...

echo "📦 3. 规整并打包知识与附带资产..."
# 将给 AI 读的说明手册放入根目录
cp SKILL.md "$OUTPUT_DIR/"

# 放入测试沙箱使用的 DB 供测试（如果有正式生产环境，这里不打包）
# 如果目前库里没有任何 DB 也是没问题的, sync 指令可以自己通过导出的表格凭空生成
if [ -f "finance.db" ]; then
    cp finance.db "$OUTPUT_DIR/bin/"
    echo ">> (提示: 已将本地 finance.db 包含进测试包内待部署)"
fi

echo "🗜️ 4. 打包最终 OpenClaw 安装包..."
cd dist
tar -czvf "${PACKAGE_NAME}.tar.gz" "$PACKAGE_NAME"

echo "✅ 打包完成！"
echo "👉 安装包路径: ./dist/${PACKAGE_NAME}.tar.gz"
echo "上传或集成进 OpenClaw 时请参考压缩包内解压后的 SKILL.md 进行指令注册及对话对齐操作。"
