#!/usr/bin/env bash
# 迁移不可变闸门：以「最早可达」的 migrations-baseline-* tag 为基线，对比 HEAD。
#
# 规则：backend/migrations/** 只允许新增文件（A）。修改（M）、删除（D）、重命名（R）
# 一律失败——已发布的迁移文件必须保持字节级不变，修正历史数据/schema 请新增版本号。
#
# 基线与 migration_upgrade_test.sh 口径一致：先按 merge-base --is-ancestor 过滤出
# 可达 tag，再取 creatordate 最早的那个。这样：
#   - 在篡改提交上自打一个「更新」的 tag 不会被选中（老基线仍然生效）；
#   - 分支落后/未 rebase 时会明确失败，而不是悄悄跳过。
#
# 首次接入（仓库还没有任何基线 tag）用显式放行：
#   MIGRATIONS_BASELINE_BOOTSTRAP=1 bash scripts/check_migrations_immutable.sh
# 打完基线 tag 后请立刻删除该环境变量，不要留在 CI 配置里。
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

tags=$(git for-each-ref --format='%(refname:short) %(creatordate:unix)' 'refs/tags/migrations-baseline-*' || true)
tag_count=$(printf '%s\n' "$tags" | grep -c . || true)

if [ "$tag_count" -eq 0 ]; then
  if [ "${MIGRATIONS_BASELINE_BOOTSTRAP:-0}" = "1" ]; then
    echo "=============================================================================="
    echo "::warning::MIGRATIONS_BASELINE_BOOTSTRAP=1：迁移不可变检查被显式跳过。"
    echo "     这只允许用于首次接入！请立即："
    echo "       1) 确认 backend/migrations 处于已发布状态；"
    echo "       2) git tag -a migrations-baseline-\$(date +%Y%m%d) -m '迁移基线'"
    echo "          并推到远端（CI 需要 fetch-depth: 0 才能读到 tag）；"
    echo "       3) 删除 MIGRATIONS_BASELINE_BOOTSTRAP，让闸门恢复正常。"
    echo "=============================================================================="
    exit 0
  fi
  fail "仓库里没有任何 migrations-baseline-* tag，无法确定迁移基线。
首次接入请显式设置 MIGRATIONS_BASELINE_BOOTSTRAP=1 跑一次，然后：
  git tag -a migrations-baseline-\$(date +%Y%m%d) -m '迁移基线'
  git push origin migrations-baseline-\$(date +%Y%m%d)
详见 docs/deploy.md「迁移不可变闸门」。"
fi

# 先过滤可达 tag（只要提交在当前历史里），再取 creatordate 最早的那个。
reachable=""
while read -r tag ts; do
  [ -z "${tag:-}" ] && continue
  if git merge-base --is-ancestor "$tag" HEAD 2>/dev/null; then
    reachable+="${ts} ${tag}"$'\n'
  fi
done <<< "$tags"

if [ -z "$reachable" ]; then
  fail "找到 ${tag_count} 个 migrations-baseline-* tag，但没有一个在当前提交历史中。
这不是脚本故障：本分支从打基线 tag 之前的位置分叉（未 rebase / 基于旧 release 分支）。
请先把分支 rebase（或 merge）到包含基线 tag 的提交（通常是 main）后重试；
若确认基线 tag 被误删，请从远端恢复：git fetch --tags --force。
详见 docs/deploy.md「迁移不可变闸门」。"
fi

baseline=$(printf '%s' "$reachable" | sort -n | head -1 | cut -d' ' -f2-)

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
  echo "以下已发布的迁移文件被修改/删除/重命名：" >&2
  printf '%s' "$violations" >&2
  cat >&2 <<'EOF'

迁移一旦发布就不可修改：
  - 修正历史数据或 schema，请新增下一个版本号（例如 000008_xxx.up.sql）；
  - 迁移执行到一半失败（dirty）时，先按 docs/deploy.md「迁移失败处置」恢复，
    再新增补偿迁移，不要回头改已发布的文件。
EOF
  exit 1
fi

echo "OK: 基线之后 backend/migrations 只有新增文件。"
