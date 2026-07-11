# Orma bash integration
# eval "$(orma hook bash)"

_ORMA_BIN='__ORMA_BIN__'
_ORMA_LAST_CMD=""

_orma_preexec() {
  _ORMA_LAST_CMD="$BASH_COMMAND"
  if ! "$_ORMA_BIN" daemon status >/dev/null 2>&1; then
    ("$_ORMA_BIN" daemon start >/dev/null 2>&1 &)
  fi
}

_orma_prompt_command() {
  local ec=$?
  if [[ -n "$_ORMA_LAST_CMD" ]]; then
    local cmd="$_ORMA_LAST_CMD"
    _ORMA_LAST_CMD=""
    (
      printf '%s' "$cmd" | "$_ORMA_BIN" hook-capture --shell bash --exit "$ec" --cwd "$PWD" 2>/dev/null || true
    ) >/dev/null 2>&1 &
    ("$_ORMA_BIN" hook-exit --exit "$ec" >/dev/null 2>&1 &)
  fi
  return 0
}

trap '_orma_preexec' DEBUG
if [[ -z "${PROMPT_COMMAND:-}" ]]; then
  PROMPT_COMMAND="_orma_prompt_command"
else
  PROMPT_COMMAND="_orma_prompt_command;${PROMPT_COMMAND}"
fi

bind -x "\"\C-g\":\"orma-recall-insert\"" 2>/dev/null || true
orma-recall-insert() {
  local sel
  sel=$("$_ORMA_BIN" recall --pick 2>/dev/null) || return 0
  READLINE_LINE="${READLINE_LINE}${sel}"
  READLINE_POINT=${#READLINE_LINE}
}
