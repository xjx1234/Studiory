#!/usr/bin/env bash
# 迁移不可变闸门：以最新的 migrations-baseline-* tag 为基线，对比 HEAD。
#
# 规则：backend/migrations/** 只允许新增文件（A）。修改（M）、删除（D）、重命名（R）
# 一律失败——已发布的迁移文件必须保持字节级不变，修正历史数据/schema 请新增版本号。
#
# 基线用 tag 而非硬编码 SHA：每次发布前对「当前状态」打一个新 tag，闸门自动跟随。
# 找不到可达 tag 时只告警不拦截（首次接入时避免把仓库挡死）。
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

baseline=""
while IFS= read -r tag; do
  [ -z "$tag" ] && continue
  if git merge-base --is-ancestor "$tag" HEAD 2>/dev/null; then
    baseline="$tag"
    break
  fi
done < <(git for-each-ref --sort=-creatordate --format='%(refname:short)' 'refs/tags/migrations-baseline-*')

if [ -z "$baseline" ]; then
  echo "::warning::未找到可达的 migrations-baseline-* tag，跳过迁移不可变检查。"
  echo "接入方式：对已发布的迁移状态打 tag，例如："
  echo "  git tag -a migrations-baseline-\$(date +%Y%m%d) -m '迁移基线'"
  echo "并确保 CI 的 checkout 使用 fetch-depth: 0（否则看不到 tag）。"
  exit 0
fi

echo "基线: $baseline ($(git rev-parse --short "$baseline"))"

violations=""
# --no-renames：把重命名拆成 D + A，避免漏判「挪走/改名」已发布文件。
while IFS=$'\t' read -r status path _; do
  [ -z "${status:-}" ] && continue
  case "$status" in
    A) ;;
    *) violations+="${status}: ${path}"$'\n' ;;
  esac
done < <(git diff --no-renames --name-status "$baseline" HEAD -- backend/migrations)

if [ -n "$violations" ]; then
  echo "以下已发布的迁移文件被修改/删除/重命名："
  printf '%s' "$violations"
  cat <<'EOF'

迁移一旦发布就不可修改：
  - 修正历史数据或 schema，请新增下一个版本号（例如 000008_xxx.up.sql）；
  - 迁移执行到一半失败（dirty）时，先按 docs/deploy.md「迁移失败处置」恢复，
    再新增补偿迁移，不要回头改已发布的文件。
EOF
  exit 1
fi

echo "OK: 基线之后 backend/migrations 只有新增文件。"
