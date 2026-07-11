# Orma fish integration
# orma hook fish | source

set -g _ORMA_BIN '__ORMA_BIN__'
set -g _ORMA_LAST_CMD ''

function _orma_preexec --on-event fish_preexec
  set -g _ORMA_LAST_CMD $argv
  if not $_ORMA_BIN daemon status >/dev/null 2>&1
    $_ORMA_BIN daemon start >/dev/null 2>&1 &
  end
end

function _orma_postexec --on-event fish_postexec
  set -l ec $status
  if test -z "$_ORMA_LAST_CMD"
    return
  end
  set -l cmd $_ORMA_LAST_CMD
  set -g _ORMA_LAST_CMD ''
  begin
    printf '%s' "$cmd" | $_ORMA_BIN hook-capture --shell fish --exit $ec --cwd (pwd) 2>/dev/null
  end >/dev/null 2>&1 &
  $_ORMA_BIN hook-exit --exit $ec >/dev/null 2>&1 &
end
