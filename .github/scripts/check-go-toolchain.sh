#!/usr/bin/env bash
# 校验并守住 CI 里实际执行的 Go 工具链版本。
#
# 为什么需要：actions/setup-go 从 v6 起会在 Setup Go 步骤里无条件
# `core.exportVariable('GOTOOLCHAIN', 'local')`（见其 src/main.ts 的 setGoToolchain），
# 而本仓库的安全基线靠 `GOTOOLCHAIN=goX.Y.Z+auto`——+auto 保证任何 go 命令实际跑的
# 版本不低于修了标准库漏洞的那个补丁版；local 则把这层保证换成「runner 装的那份」，
# 一旦 setup-go 的 '1.26.x' 解析命中 runner 缓存的旧补丁，补丁就悄悄没了。
# 更阴的是 `GOTOOLCHAIN=local go env GOVERSION` 即使低于 go.mod 的要求也只回当前版本、
# 退出码 0，不会自己报错，所以必须显式比一次。
#
# 实测（2026-11-13，run 43376824593）：GITHUB_ENV 里导出的变量连 `${{ env.* }}` 上下文
# 都会被改写——Setup Go 之后 `${{ env.GOTOOLCHAIN }}` 取到的是 'local'。所以 ci.yml
# 不再声明同名 env（两处声明必然漂移，且 workflow 级那份根本不生效），
# 钉扎只从 backend/go.mod 的 toolchain 行推导（本地/镜像/CI 共用同一份声明）。
#
# 做四件事：
#   1. 拒绝任何 workflow 重新声明 GOTOOLCHAIN（并检查用 setup-go 的 workflow 都跑了本脚本）
#      ——单一来源被破坏时直接失败，而不是静默漂移；
#   2. 把推导出的钉扎写进 $GITHUB_ENV（同名变量后写覆盖先写），抵消 setup-go 的 local，
#      恢复升级 action 之前的语义；
#   3. 按钉扎量一次 `go env GOVERSION`——+auto 需要时会切换/下载，
#      所以这一步同时验证「钉扎版本可达」，量出来的就是后续步骤真正用的版本；
#   4. 量到的版本低于钉扎就失败并写 annotation（工具链被静默降级必须变红，
#      而不是等 govulncheck 兜底）。
#
# 用法：bash "$GITHUB_WORKSPACE/.github/scripts/check-go-toolchain.sh" [pin]
# 不传 pin 时从 go.mod 推导（CI 里就这么用）；pin 形如 go1.26.11+auto，仅本地排障时覆盖。
set -uo pipefail

# ── 定位 go.mod：CI 里 cwd 可能是仓库根（lint job）或 backend/（其余 job）──────────────
find_gomod() {
  local c
  for c in "${GOMOD_FILE:-}" "${GITHUB_WORKSPACE:-}/backend/go.mod" "backend/go.mod" "go.mod" "../go.mod"; do
    [ -n "$c" ] && [ -f "$c" ] && { echo "$c"; return 0; }
  done
  return 1
}

gomod="$(find_gomod || true)"
if [ -z "$gomod" ]; then
  echo "::error title=go-toolchain-pin::找不到 go.mod（GITHUB_WORKSPACE=${GITHUB_WORKSPACE:-<未设>}, cwd=$(pwd)），无法推导钉扎版本"
  exit 2
fi

# ── 单一来源守卫：任何 workflow 都不允许再声明 GOTOOLCHAIN ───────────────────────
# 同名时 GITHUB_ENV 的写入会覆盖 workflow 级 env，两处声明只会漂移且很难察觉。
# 只匹配 2-10 空格缩进的 YAML 键，覆盖 workflow(2) / job(6) / step(10) 三级 env；
# 注释以 # 开头不会命中；run: | 的内容通常更深，且只有整行恰好形如 `GOTOOLCHAIN: 值`
# 才会误报——真误报也是显式失败，改一下注释措辞即可，不会静默放过。
#
# 同时要求：用了 actions/setup-go 的 workflow 必须执行本脚本。setup-go 默认导出的
# GOTOOLCHAIN=local 会绕过 backend/go.mod 的补丁版钉扎，不校验就等于没有钉扎——
# 新加的 workflow（如 migrations.yml）漏掉这一步，在这里直接变红。
if [ -n "${GITHUB_WORKSPACE:-}" ] && [ -d "$GITHUB_WORKSPACE/.github/workflows" ]; then
  for wf in "$GITHUB_WORKSPACE"/.github/workflows/*.yml; do
    [ -f "$wf" ] || continue
    name="$(basename "$wf")"
    if grep -Eq '^ {2,10}GOTOOLCHAIN:[[:space:]]*[^[:space:]]' "$wf"; then
      echo "::error title=go-toolchain-pin::${name} 又声明了 GOTOOLCHAIN；工具链版本只允许在 backend/go.mod 的 toolchain 行声明，避免 GITHUB_ENV 与 workflow env 两处漂移"
      exit 2
    fi
    if grep -q 'uses: actions/setup-go' "$wf" && ! grep -q 'check-go-toolchain.sh' "$wf"; then
      echo "::error title=go-toolchain-pin::${name} 使用了 actions/setup-go 却没有执行 .github/scripts/check-go-toolchain.sh；setup-go 会导出 GOTOOLCHAIN=local，不校验则 backend/go.mod 的补丁版钉扎形同虚设"
      exit 2
    fi
  done
fi

# ── 钉扎：argv 优先，其次 go.mod 的 toolchain 行，最后退化为 go 行 ────────────────────
pin="${1:-}"
source_of="参数"
if [ -z "$pin" ]; then
  toolchain_line="$(awk '/^toolchain go/{print $2; exit}' "$gomod")"
  go_line="$(awk '/^go [0-9]/{print $2; exit}' "$gomod")"
  if [ -n "$toolchain_line" ]; then
    pin="${toolchain_line}+auto"
    source_of="$gomod 的 toolchain 行"
  elif [ -n "$go_line" ]; then
    # 没有 toolchain 行时只能拿 go 行当下限：它给出的是语言/依赖下限，不含补丁版
    pin="go${go_line}+auto"
    source_of="$gomod 的 go 行（无 toolchain 行，下限不含补丁版）"
  else
    echo "::error title=go-toolchain-pin::$gomod 里既没有 toolchain 行也没有 go 行"
    exit 2
  fi
fi

# go1.26.11+auto -> 1.26.11；没有补丁号（go1.26）时补成 1.26.0 方便逐段比较
want="${pin#go}"
want="${want%%+*}"
case "$want" in
  *.*.*) ;;
  *.*) want="${want}.0" ;;
  *)
    echo "::error title=go-toolchain-pin::无法解析钉扎版本：'${pin}'（来源：${source_of}，期望形如 go1.26.11+auto）"
    exit 2
    ;;
esac
echo "pin               : ${pin}（来源：${source_of}）"

if [ -n "${GITHUB_ENV:-}" ]; then
  echo "GOTOOLCHAIN=${pin}" >>"$GITHUB_ENV"
  echo "pin re-exported to \$GITHUB_ENV: GOTOOLCHAIN=${pin}"
else
  echo "GITHUB_ENV 未设置（本地跑），跳过回写"
fi

# 本步骤实际拿到的环境：mode=local 就说明 setup-go 的覆写生效了（workflow env 优先级更低）
mode="$(go env GOTOOLCHAIN 2>/dev/null || echo '?')"
observed="$(go env GOVERSION 2>/dev/null || echo '')"
echo "step GOTOOLCHAIN  : ${mode:-<unknown>}"
echo "step GOVERSION    : ${observed:-<unknown>}"

# 按钉扎再量一次：这才是后续步骤会用的版本
err_file="$(mktemp)"
if ! effective="$(GOTOOLCHAIN="$pin" go env GOVERSION 2>"$err_file")"; then
  echo "::error title=go-toolchain-pin::GOTOOLCHAIN=${pin} 下 go 命令失败（钉扎版本不可达？检查 GOPROXY 与 toolchain 模块校验）：$(tr '\n' '|' <"$err_file" | cut -c1-600)"
  rm -f "$err_file"
  exit 1
fi
rm -f "$err_file"
effective="${effective#go}"

echo "pinned GOVERSION  : go${effective}（下限 go${want}）"

if [[ ! "$effective" =~ ^[0-9.]+$ ]]; then
  echo "::error title=go-toolchain-pin::go env GOVERSION 输出 'go${effective}' 不是可比较的版本号（期望形如 go1.26.11）"
  exit 1
fi

# 逐段数字比较：effective >= want ?
IFS='.' read -r e_maj e_min e_pat <<<"$effective"
IFS='.' read -r w_maj w_min w_pat <<<"$want"
for pair in "${e_maj:-0}:${w_maj:-0}" "${e_min:-0}:${w_min:-0}" "${e_pat:-0}:${w_pat:-0}"; do
  a="${pair%%:*}"
  b="${pair##*:}"
  if [ "$a" -gt "$b" ]; then
    echo "Go toolchain ${effective} > pinned ${want} ✔"
    exit 0
  fi
  if [ "$a" -lt "$b" ]; then
    echo "::error title=go-toolchain-pin::GOTOOLCHAIN=${pin} 下实际执行的是 go${effective}，低于钉扎的 go${want}（本步骤环境为 ${mode}，钉扎来源 ${source_of}）——${want} 之后的标准库安全补丁不会生效。检查 backend/go.mod 的 toolchain 行、ci.yml 里 setup-go 的 go-version，以及 GOPROXY 是否能取到 golang.org/toolchain 模块。"
    exit 1
  fi
done
echo "Go toolchain ${effective} == pinned ${want} ✔"
