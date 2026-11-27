#!/usr/bin/env bash
# 迁移升级路径验证（需要可连接的 PostgreSQL，本地无 Docker 时也可直接跑）。
#
# 覆盖四件事：
#   1. 存量库升级：用基线 tag 的 backend/migrations 建库 → 插入带分隔符/国家码的手机号
#      → 用工作树的 backend/migrations up 到 HEAD → 断言号码已变成裸 11 位；
#   2. 幂等：force 到 000007 的上一版本后再 up 一次，号码仍为裸号且行数不变；
#   3. 黄金样例：backend/pkg/strutil/testdata/phone_normalization_golden.tsv 的每条样例
#      逐条写库并跑真实 000007，断言清洗结果与 Go 单测共用同一口径；
#   4. 撞号预检：往干净库里插入归一化后相同的两个号码，脚本能查出来，000007 会中止。
#
# 环境变量：
#   PGHOST / PGPORT / PGUSER / PGPASSWORD  数据库连接（默认 localhost:5432 postgres/postgres）
#   UPGRADE_BASELINE_TAG                   升级起点 tag（默认取最早的 migrations-baseline-*）
#   FORCE_VERSION                          幂等复跑时 force 到的版本（默认 6，即 000007 前一版）
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

PGHOST=${PGHOST:-localhost}
PGPORT=${PGPORT:-5432}
PGUSER=${PGUSER:-postgres}
PGPASSWORD=${PGPASSWORD:-postgres}
FORCE_VERSION=${FORCE_VERSION:-6}
export PGPASSWORD

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

command -v migrate >/dev/null || fail "migrate 未安装（go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@v4.17.0）"
command -v psql >/dev/null || fail "psql 未安装"

PSQL=(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -v ON_ERROR_STOP=1 -tAq)

# 升级起点固定用「最早」的基线 tag：基线会随时间推进，但升级路径始终从最早状态开始，
# 才能持续覆盖「老库补跑 000007」这条路径。
UPGRADE_BASELINE_TAG=${UPGRADE_BASELINE_TAG:-}
if [ -z "$UPGRADE_BASELINE_TAG" ]; then
  UPGRADE_BASELINE_TAG=$(git for-each-ref --sort=creatordate --format='%(refname:short)' 'refs/tags/migrations-baseline-*' | head -1)
fi
[ -n "$UPGRADE_BASELINE_TAG" ] || fail "未找到 migrations-baseline-* tag；请先打基线 tag 或设置 UPGRADE_BASELINE_TAG"

HEAD_MIGRATIONS="backend/migrations"
DB_UPGRADE="app_upgrade_test_$$"
DB_COLLISION="app_collision_test_$$"
TMP_BASE=$(mktemp -d)

cleanup() {
  "${PSQL[@]}" -d postgres -c "DROP DATABASE IF EXISTS $DB_UPGRADE" >/dev/null 2>&1 || true
  "${PSQL[@]}" -d postgres -c "DROP DATABASE IF EXISTS $DB_COLLISION" >/dev/null 2>&1 || true
  rm -rf "$TMP_BASE"
}
trap cleanup EXIT

url() {
  echo "postgres://$PGUSER:$PGPASSWORD@$PGHOST:$PGPORT/$1?sslmode=disable"
}

echo "== 1. 准备基线迁移（$UPGRADE_BASELINE_TAG = $(git rev-parse --short "$UPGRADE_BASELINE_TAG")）"
git archive "$UPGRADE_BASELINE_TAG" backend/migrations | tar -x -C "$TMP_BASE"
BASE_MIGRATIONS="$TMP_BASE/backend/migrations"
[ -f "$BASE_MIGRATIONS/000005_identity_canonical.up.sql" ] || fail "基线里缺少 000005"
if grep -q 'regexp_replace(phone' "$BASE_MIGRATIONS/000005_identity_canonical.up.sql"; then
  fail "基线 000005 仍包含手机号清洗，说明 tag 打在修复之后，无法验证升级路径"
fi

"${PSQL[@]}" -d postgres -c "CREATE DATABASE $DB_UPGRADE" >/dev/null
"${PSQL[@]}" -d postgres -c "CREATE DATABASE $DB_COLLISION" >/dev/null

echo "== 2. 用基线迁移建库并写入脏号码"
migrate -path "$BASE_MIGRATIONS" -database "$(url "$DB_UPGRADE")" up >/dev/null

U1='11111111-1111-1111-1111-111111111111'
U2='22222222-2222-2222-2222-222222222222'
U3='33333333-3333-3333-3333-333333333333'
U4='44444444-4444-4444-4444-444444444444'
"${PSQL[@]}" -d "$DB_UPGRADE" -c "
INSERT INTO users (id, phone) VALUES
  ('$U1', '+86 138-0013-8000'),
  ('$U2', '0086(139)0013-9000'),
  ('$U3', '86 137 0013 7000'),
  ('$U4', '13600136000');
" >/dev/null

COUNT_BEFORE=$("${PSQL[@]}" -d "$DB_UPGRADE" -c "SELECT count(*) FROM users")
DIRTY_BEFORE=$("${PSQL[@]}" -d "$DB_UPGRADE" -c "SELECT count(*) FROM users WHERE phone ~ '[^0-9]'")
[ "$DIRTY_BEFORE" = "3" ] || fail "种子数据异常：期望 3 行带非数字字符，实际 $DIRTY_BEFORE"

expect_phone() {
  local got
  got=$("${PSQL[@]}" -d "$DB_UPGRADE" -c "SELECT phone FROM users WHERE id = '$1'")
  [ "$got" = "$2" ] || fail "用户 $1 的 phone = '$got'，期望 '$2'"
}

echo "== 3. 升级到 HEAD 并断言清洗结果"
migrate -path "$HEAD_MIGRATIONS" -database "$(url "$DB_UPGRADE")" up >/dev/null

expect_phone "$U1" '13800138000'
expect_phone "$U2" '13900139000'
expect_phone "$U3" '13700137000'
expect_phone "$U4" '13600136000'

COUNT_AFTER=$("${PSQL[@]}" -d "$DB_UPGRADE" -c "SELECT count(*) FROM users")
[ "$COUNT_AFTER" = "$COUNT_BEFORE" ] || fail "升级后行数变化：$COUNT_BEFORE -> $COUNT_AFTER"
DIRTY_AFTER=$("${PSQL[@]}" -d "$DB_UPGRADE" -c "SELECT count(*) FROM users WHERE phone ~ '[^0-9]'")
[ "$DIRTY_AFTER" = "0" ] || fail "升级后仍有 $DIRTY_AFTER 行带非数字字符"

echo "== 4. 幂等：force $FORCE_VERSION 后重跑 up"
migrate -path "$HEAD_MIGRATIONS" -database "$(url "$DB_UPGRADE")" force "$FORCE_VERSION" >/dev/null
migrate -path "$HEAD_MIGRATIONS" -database "$(url "$DB_UPGRADE")" up >/dev/null

expect_phone "$U1" '13800138000'
expect_phone "$U2" '13900139000'
expect_phone "$U3" '13700137000'
expect_phone "$U4" '13600136000'
[ "$("${PSQL[@]}" -d "$DB_UPGRADE" -c "SELECT count(*) FROM users")" = "$COUNT_BEFORE" ] \
  || fail "重复执行后行数变化"

echo "== 5. 黄金样例：逐条跑真实 000007，校验与 Go 单测同口径"
GOLDEN_TABLE="backend/pkg/strutil/testdata/phone_normalization_golden.tsv"
[ -f "$GOLDEN_TABLE" ] || fail "缺少黄金样例表 $GOLDEN_TABLE"
if [ -n "$(tail -c 1 "$GOLDEN_TABLE")" ]; then
  fail "黄金样例表末行缺少换行：bash 的 read 会静默跳过最后一条样例"
fi

# 样例表用 \t、\n 字面量表示控制字符，避免控制字符破坏 TSV 结构。
decode_golden() {
  local s=$1
  s=${s//'\t'/$'\t'}
  s=${s//'\n'/$'\n'}
  printf '%s' "$s"
}

GOLDEN_ID='99999999-9999-9999-9999-999999999999'
cat >"$TMP_BASE/insert_golden.sql" <<SQL
INSERT INTO users (id, phone) VALUES ('$GOLDEN_ID', :'in');
SQL

GOLDEN_CASES=0
while IFS=$'\t' read -r raw_in raw_want; do
  case "$raw_in" in
    ''|'#'*) continue ;;
  esac
  [ -n "${raw_want:-}" ] || fail "黄金样例缺少 expected 列：$raw_in"
  GOLDEN_CASES=$((GOLDEN_CASES + 1))

  in=$(decode_golden "$raw_in")
  want=$(decode_golden "$raw_want")
  # 空串在 TSV 里用 (empty) 占位（空行会被忽略），两列规则一致。
  [ "$in" = "(empty)" ] && in=""
  [ "$want" = "(empty)" ] && want=""

  # 每轮先清空、退回 000007 前一版，保证一次只有一条样例，不会触发撞号预检。
  # 插入值经 psql -v 变量传入 :'in'，引号/换行由 psql 负责转义，不拼 SQL 字面量。
  "${PSQL[@]}" -d "$DB_UPGRADE" -c "TRUNCATE users CASCADE" >/dev/null
  migrate -path "$HEAD_MIGRATIONS" -database "$(url "$DB_UPGRADE")" force "$FORCE_VERSION" >/dev/null
  "${PSQL[@]}" -d "$DB_UPGRADE" -v in="$in" -f "$TMP_BASE/insert_golden.sql" >/dev/null
  migrate -path "$HEAD_MIGRATIONS" -database "$(url "$DB_UPGRADE")" up >/dev/null

  got=$("${PSQL[@]}" -d "$DB_UPGRADE" -c "SELECT phone FROM users WHERE id = '$GOLDEN_ID'")
  [ "$got" = "$want" ] \
    || fail "黄金样例 $(printf '%q' "$in") 清洗后 = $(printf '%q' "$got")，期望 $(printf '%q' "$want")"
done <"$GOLDEN_TABLE"
[ "$GOLDEN_CASES" -gt 0 ] || fail "黄金样例表为空：$GOLDEN_TABLE"

echo "== 5b. 撞号预检脚本与黄金表同口径"
# 000007 不可改，它的防撞号逻辑与 scripts/check_phone_collisions.sql 是两份 SQL 拷贝；
# 用同一张黄金表跑一遍脚本，任何一侧单独改动都会在这里变红。
SCRIPT_CASES=0
while IFS=$'\t' read -r raw_in raw_want; do
  case "$raw_in" in
    ''|'#'*) continue ;;
  esac
  [ -n "${raw_want:-}" ] || fail "黄金样例缺少 expected 列：$raw_in"
  SCRIPT_CASES=$((SCRIPT_CASES + 1))

  in=$(decode_golden "$raw_in")
  want=$(decode_golden "$raw_want")
  [ "$in" = "(empty)" ] && in=""
  [ "$want" = "(empty)" ] && want=""

  "${PSQL[@]}" -d "$DB_UPGRADE" -c "TRUNCATE users CASCADE" >/dev/null
  "${PSQL[@]}" -d "$DB_UPGRADE" -v in="$in" -f "$TMP_BASE/insert_golden.sql" >/dev/null
  # 脚本输出三列（normalized|rows|ids），取第一列与黄金表期望比对。
  got=$("${PSQL[@]}" -d "$DB_UPGRADE" -F '|' -f scripts/check_phone_collisions.sql | cut -d'|' -f1)
  [ "$got" = "$want" ] \
    || fail "撞号预检脚本对 $(printf '%q' "$in") 归一化为 $(printf '%q' "$got")，期望 $(printf '%q' "$want")：脚本与黄金表口径已漂移"
done <"$GOLDEN_TABLE"
[ "$SCRIPT_CASES" -gt 0 ] || fail "撞号预检脚本一致性检查没有样例"

echo "== 6. 撞号预检：脚本能报出冲突组，000007 会中止"
migrate -path "$HEAD_MIGRATIONS" -database "$(url "$DB_COLLISION")" up >/dev/null
C1='aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
C2='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
"${PSQL[@]}" -d "$DB_COLLISION" -c "
INSERT INTO users (id, phone) VALUES
  ('$C1', '138-0013-8000'),
  ('$C2', '+86 13800138000');
" >/dev/null

COLLISIONS=$("${PSQL[@]}" -d "$DB_COLLISION" -f scripts/check_phone_collisions.sql)
echo "$COLLISIONS"
echo "$COLLISIONS" | grep -q '13800138000' || fail "check_phone_collisions.sql 没有报出 13800138000"
echo "$COLLISIONS" | grep -q "$C1" || fail "check_phone_collisions.sql 输出缺少冲突 id $C1"
echo "$COLLISIONS" | grep -q "$C2" || fail "check_phone_collisions.sql 输出缺少冲突 id $C2"

migrate -path "$HEAD_MIGRATIONS" -database "$(url "$DB_COLLISION")" force "$FORCE_VERSION" >/dev/null
if MIGRATE_OUT=$(migrate -path "$HEAD_MIGRATIONS" -database "$(url "$DB_COLLISION")" up 2>&1); then
  fail "存在撞号时 000007 仍然执行成功"
fi
echo "$MIGRATE_OUT" | grep -q 'phone collision after normalization' \
  || fail "000007 中止信息不含预期文案：$MIGRATE_OUT"

echo "ALL MIGRATION UPGRADE CHECKS PASSED"
