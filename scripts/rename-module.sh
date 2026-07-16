#!/usr/bin/env bash
# 一次性把 Go module 名从脚手架默认的 "backend" 改成你的新项目名，
# 并同步替换所有受影响的 import 路径 / -ldflags 包路径。
#
# 用法：
#   scripts/rename-module.sh <new-module-path>
#
# 示例：
#   scripts/rename-module.sh github.com/acme/myapp
#   scripts/rename-module.sh myapp                    # 不带域名也合法
#
# 会修改：
#   - backend/go.mod                 module 声明（用 `go mod edit`）
#   - backend/**/*.go                import 路径中的 backend/internal、
#                                    backend/pkg、backend/locales、backend/cmd
#                                    （backend/internal/repo/sqlc/gen 是生成代码，跳过）
#   - Makefile、backend/Dockerfile   -ldflags 里硬编码的 backend/internal/buildinfo
#
# 不会修改：
#   - backend/ 目录名本身（module 名和目录名是两件事，无需重命名目录）
#   - docs/*.md、README.md 里提到 "backend/..." 的地方（那些是目录路径说明，不是 import 路径）
#
# 执行完脚本会自动跑一次 `go build ./...` 做编译自检；建议随后再跑
# `go test ./...`、`git diff` 确认改动范围后再提交。

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
OLD_MODULE="backend"

if [[ $# -ne 1 ]]; then
  echo "用法: $0 <new-module-path>" >&2
  echo "示例: $0 github.com/acme/myapp" >&2
  exit 1
fi

NEW_MODULE="$1"

if [[ "$NEW_MODULE" == "$OLD_MODULE" ]]; then
  echo "新模块名与当前模块名相同（\"$OLD_MODULE\"），无需改动。"
  exit 0
fi

if ! [[ "$NEW_MODULE" =~ ^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$ ]]; then
  echo "错误：模块名 \"$NEW_MODULE\" 含非法字符（合法示例：github.com/acme/myapp 或 myapp）" >&2
  exit 1
fi

# BSD sed（macOS）与 GNU sed（Linux）的 -i 参数不兼容，做个兼容包装。
sed_inplace() {
  if sed --version >/dev/null 2>&1; then
    sed -i "$@"        # GNU sed
  else
    sed -i '' "$@"      # BSD sed
  fi
}

echo "→ module: \"$OLD_MODULE\" → \"$NEW_MODULE\""

echo "→ 更新 backend/go.mod"
( cd "$BACKEND_DIR" && go mod edit -module "$NEW_MODULE" )

echo "→ 更新 .go 文件里的 import 路径"
while IFS= read -r -d '' file; do
  sed_inplace \
    -e "s#\"${OLD_MODULE}/internal#\"${NEW_MODULE}/internal#g" \
    -e "s#\"${OLD_MODULE}/pkg#\"${NEW_MODULE}/pkg#g" \
    -e "s#\"${OLD_MODULE}/locales#\"${NEW_MODULE}/locales#g" \
    -e "s#\"${OLD_MODULE}/cmd#\"${NEW_MODULE}/cmd#g" \
    "$file"
done < <(find "$BACKEND_DIR" -name '*.go' -not -path '*/internal/repo/sqlc/gen/*' -print0)

echo "→ 更新 Makefile / Dockerfile 里的 -ldflags 包路径"
sed_inplace -e "s#${OLD_MODULE}/internal/buildinfo#${NEW_MODULE}/internal/buildinfo#g" \
  "$ROOT_DIR/Makefile" "$BACKEND_DIR/Dockerfile"

echo "→ 编译自检（go build ./...）"
if ( cd "$BACKEND_DIR" && go build ./... ); then
  echo "✓ 重命名完成，编译通过。"
else
  echo "✗ 编译失败，请检查上面的报错（也可以 git diff 查看改动范围后手动修复）。" >&2
  exit 1
fi

cat <<EOF

接下来建议：
  cd backend && go test ./...   # 跑一遍测试确认没有遗漏
  git diff                       # 检查改动范围
  git add -A && git commit -m "chore: rename module to ${NEW_MODULE}"
EOF
