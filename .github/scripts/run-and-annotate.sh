#!/usr/bin/env bash
# 在 CI 里跑一条命令：保留真实退出码，失败时把关键输出写成 ::error annotation。
#
# 为什么需要：GitHub 的 job 日志需要登录才能看（API 拿不到），而 annotation
# 可以通过未登录的 REST 接口读到。另外 `go test ... | tee log` 这种管道在没有
# pipefail 时退出码取的是 tee（永远为 0），测试失败也会「空过」——本脚本统一处理。
#
# 用法：bash "$GITHUB_WORKSPACE/.github/scripts/run-and-annotate.sh" <label> <cmd> [args...]
# label 会出现在 annotation 标题里（如 mysql-integration-failure），保持简短唯一。
set -uo pipefail

if [ "$#" -lt 2 ]; then
  echo "usage: $0 <label> <command> [args...]" >&2
  exit 2
fi

label="$1"
shift
log="/tmp/${label}.log"

# pipefail 让管道以左侧命令（go test / govulncheck）的退出码为准；
# 不开 pipefail 时 `cmd | tee` 的退出码永远是 tee 的 0，失败会被吞掉。
# 注意不能写 `if cmd; then exit 0; fi` 后再取 $?：if 语句本身会把 $? 归零。
set -o pipefail
code=0
"$@" 2>&1 | tee "$log" || code=$?
if [ "$code" -eq 0 ]; then
  exit 0
fi

# 失败时先原样回显一份，再挑「能定位问题」的行塞进 annotation（长度有限，按优先级截断）
grep -E "^Vulnerability #|^ +(Found|Fixed) in:|^Your code is affected|^--- FAIL|^=== RUN|^FAIL|^ok |panic:|error:|Error [0-9]+|\.go:[0-9]+:[0-9]+:" "$log" |
  head -n 30 >"/tmp/${label}.key"

MSG=$(
  {
    cat "/tmp/${label}.key"
    echo "--tail--"
    tail -n 15 "$log"
  } | tr '\n' '|' | cut -c1-1400
)

echo "::error title=${label}-failure::${MSG}"
exit "$code"
