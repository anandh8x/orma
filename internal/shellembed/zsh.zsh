# Orma zsh integration
# eval "$(orma hook zsh)"

_ORMA_BIN='__ORMA_BIN__'
_ORMA_LAST_CMD=""

_orma_preexec() {
  _ORMA_LAST_CMD="$1"
  if ! "$_ORMA_BIN" daemon status >/dev/null 2>&1; then
    ("$_ORMA_BIN" daemon start >/dev/null 2>&1 &)
  fi
}

_orma_precmd() {
  local ec=$?
  [[ -z "$_ORMA_LAST_CMD" ]] && return 0
  local cmd="$_ORMA_LAST_CMD"
  _ORMA_LAST_CMD=""
  (
    printf '%s' "$cmd" | "$_ORMA_BIN" hook-capture --shell zsh --exit "$ec" --cwd "$PWD" 2>/dev/null || true
  ) >/dev/null 2>&1 &!
  ("$_ORMA_BIN" hook-exit --exit "$ec" >/dev/null 2>&1 &)
  return 0
}

autoload -Uz add-zsh-hook 2>/dev/null
if typeset -f add-zsh-hook >/dev/null; then
  add-zsh-hook preexec _orma_preexec
  add-zsh-hook precmd _orma_precmd
else
  preexec_functions+=(_orma_preexec)
  precmd_functions+=(_orma_precmd)
fi

_orma_widget() {
  local sel
  sel=$("$_ORMA_BIN" recall --pick 2>/dev/null) || return 0
  [[ -n "$sel" ]] && LBUFFER="${LBUFFER}${sel}"
}
zle -N orma-recall-widget _orma_widget
bindkey "${ORMA_KEYBIND:-\C-g}" orma-recall-widget
