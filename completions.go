package main

import "fmt"

const fishCompletions = `# clau completions for fish. Load now: clau completions fish | source
# Persist: clau completions fish > ~/.config/fish/conf.d/clau.fish
# (conf.d, not completions/: this file registers completions for both
# clau and c, and the completions/ autoloader only loads a file for the
# command matching its filename.)
complete -c clau -f -n 'test (count (commandline -opc)) -eq 1' -a 'link unlink list run init trust untrust doctor completions version help'
complete -c clau -f -n 'test (count (commandline -opc)) -eq 1' -a '(clau list --tokens 2>/dev/null)'
complete -c clau -f -n 'contains -- (commandline -opc)[2] run; and test (count (commandline -opc)) -eq 2' -a '(clau list --tokens 2>/dev/null)'
complete -c clau -f -n 'contains -- (commandline -opc)[2] completions; and test (count (commandline -opc)) -eq 2' -a 'fish zsh bash'
complete -c c -f -n 'test (count (commandline -opc)) -eq 1' -a '(clau list --tokens 2>/dev/null)'
`

const bashCompletions = `# clau completions for bash. Load: eval "$(clau completions bash)"
_clau() {
  if [ "$COMP_CWORD" -eq 1 ]; then
    COMPREPLY=($(compgen -W "link unlink list run init trust untrust doctor completions version help $(clau list --tokens 2>/dev/null)" -- "${COMP_WORDS[1]}"))
  elif [ "${COMP_WORDS[1]}" = run ] && [ "$COMP_CWORD" -eq 2 ]; then
    COMPREPLY=($(compgen -W "$(clau list --tokens 2>/dev/null)" -- "${COMP_WORDS[2]}"))
  fi
}
complete -o default -F _clau clau
_c_launcher() {
  if [ "$COMP_CWORD" -eq 1 ]; then
    COMPREPLY=($(compgen -W "$(clau list --tokens 2>/dev/null)" -- "${COMP_WORDS[1]}"))
  fi
}
complete -o default -F _c_launcher c
`

const zshCompletions = `# clau completions for zsh. Load: eval "$(clau completions zsh)"
_clau() {
  if (( CURRENT == 2 )); then
    local -a items
    items=(link unlink list run init trust untrust doctor completions version help)
    items+=(${(f)"$(clau list --tokens 2>/dev/null)"})
    _describe 'clau' items
  elif [[ ${words[2]} == run ]] && (( CURRENT == 3 )); then
    local -a tokens
    tokens=(${(f)"$(clau list --tokens 2>/dev/null)"})
    _describe 'token' tokens
  else
    _files
  fi
}
compdef _clau clau
_c_launcher() {
  if (( CURRENT == 2 )); then
    local -a tokens
    tokens=(${(f)"$(clau list --tokens 2>/dev/null)"})
    _describe 'token' tokens
  else
    _files
  fi
}
compdef _c_launcher c
`

func completionScript(shell string) (string, error) {
	switch shell {
	case "fish":
		return fishCompletions, nil
	case "bash":
		return bashCompletions, nil
	case "zsh":
		return zshCompletions, nil
	default:
		return "", fmt.Errorf("unsupported shell %q (want fish, zsh, or bash)", shell)
	}
}
