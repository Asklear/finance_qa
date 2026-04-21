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
echo ">> 编译适用你本机的二进制执行档..."
go build -o "$OUTPUT_DIR/bin/financeqa" ./cmd/financeqa/...

echo "📦 3. 规整并打包知识与附带资产..."
# 将给 AI 读的说明手册放入根目录，并保留附录的相对路径
cp SKILL.md "$OUTPUT_DIR/"
mkdir -p "$OUTPUT_DIR/docs"
cp docs/SKILL_APPENDIX_FULL.md "$OUTPUT_DIR/docs/"

echo "🗜️ 4. 打包最终 OpenClaw 安装包..."
cd dist
tar -czvf "${PACKAGE_NAME}.tar.gz" "$PACKAGE_NAME"

echo "✅ 打包完成！"
echo "👉 安装包路径: ./dist/${PACKAGE_NAME}.tar.gz"
echo "上传或集成进 OpenClaw 时请参考压缩包内解压后的 SKILL.md 进行指令注册及对话对齐操作。"
