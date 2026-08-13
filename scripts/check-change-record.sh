#!/usr/bin/env bash
# AGENTS.md 归档铁律的最小机械门禁。
#
# 规则（AGENTS.md §1）：所有改动必须归档，未归档的修改视为未完成，禁止提交。
# 本门禁校验：当一次改动包含非文档代码文件时，同一改动中必须存在
# docs/changes/YYYY-MM-DD-<slug>.md 形式的 change-record；否则以可读错误
# 指向 AGENTS.md 归档规则并以非零码退出。文档-only 改动不触发本门禁。
#
# 用法：
#   scripts/check-change-record.sh --base <ref>   # CI：校验 <ref>..HEAD 的改动
#   scripts/check-change-record.sh --staged       # 提交钩子：校验暂存区改动
#
# 与 .github/workflows/ci.yml 的 changes job 保持一致的文档判定：
# docs/ 下全部文件、README.md、CHANGELOG.md 视为文档。
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/check-change-record.sh --base <ref>   validate changes between <ref> and HEAD (CI)
  scripts/check-change-record.sh --staged       validate staged changes (pre-commit hook)
EOF
}

mode=""
base=""
while [ $# -gt 0 ]; do
  case "$1" in
    --base)
      mode="base"
      if [ $# -lt 2 ] || [ -z "${2:-}" ]; then
        echo "check-change-record: --base requires a ref argument" >&2
        usage >&2
        exit 2
      fi
      base="$2"
      shift 2
      ;;
    --staged)
      mode="staged"
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "check-change-record: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [ -z "$mode" ]; then
  usage >&2
  exit 2
fi

case "$mode" in
  base)
    if ! git cat-file -e "$base^{commit}" 2>/dev/null; then
      echo "check-change-record: base ref '$base' is not a valid commit" >&2
      exit 2
    fi
    changed="$(git diff --name-only "$base" HEAD)"
    ;;
  staged)
    changed="$(git diff --cached --name-only)"
    ;;
esac

# change-record 命名规范：docs/changes/YYYY-MM-DD-<slug>.md（TEMPLATE.md 不符合）。
record_pattern='^docs/changes/[0-9]{4}-[0-9]{2}-[0-9]{2}-[a-z0-9][a-z0-9-]*\.md$'

code_files=""
record_found=""
if [ -n "$changed" ]; then
  while IFS= read -r file; do
    [ -n "$file" ] || continue
    if printf '%s\n' "$file" | grep -Eq "$record_pattern"; then
      record_found="$file"
      continue
    fi
    case "$file" in
      docs/* | README.md | CHANGELOG.md)
        continue
        ;;
    esac
    code_files="${code_files}${file}"$'\n'
  done <<EOF
$changed
EOF
fi

if [ -z "$code_files" ]; then
  echo "check-change-record: 文档-only 改动，无需 change-record，通过。"
  exit 0
fi

if [ -n "$record_found" ]; then
  echo "check-change-record: 检测到 change-record '$record_found'，归档门禁通过。"
  exit 0
fi

{
  echo "check-change-record: 拦截 —— 改动包含非文档代码文件，但同一改动中缺少 change-record。"
  echo ""
  echo "违反 AGENTS.md §1 铁律：「每一次改动都必须落到 docs/changes/YYYY-MM-DD-<slug>.md，"
  echo "未归档的修改视为未完成，禁止提交。」"
  echo ""
  echo "本次改动中的非文档代码文件："
  printf '%s' "$code_files" | sed 's/^/  - /'
  echo ""
  echo "修复方式："
  echo "  1. 复制 docs/changes/TEMPLATE.md 为 docs/changes/YYYY-MM-DD-<slug>.md 并填写；"
  echo "  2. 将该 change-record 与代码改动放入同一次提交 / 同一个 PR；"
  echo "  3. 用户可见改动还需同步更新 CHANGELOG.md（AGENTS.md §1）。"
  echo ""
  echo "归档规范见 AGENTS.md 与 docs/ARCHIVING.md。"
} >&2
exit 1
