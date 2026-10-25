# clau README demo — render with: vhs docs/demo.tape (from repo root)
Output docs/demo.gif
Set Shell "fish"
Set FontSize 20
Set Width 1200
Set Height 1260
Set Framerate 25
Set TypingSpeed 75ms

# Env must come after every Set — a leading Env silently voids later Sets.
# Keep the personal .clau.toml project layer out of the public demo:
Env CLAU_NO_PROJECT "1"
# clau show prints env values (secrets masked, hosts not) — record with a
# config whose ANTHROPIC_BASE_URL you are happy to publish.
# Skip promo/announcement banners and update checks while recording:
Env CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC "1"

# The pitch: the long way, wiped away
Type "claude --model opus --effort max"
Sleep 1.5s
Ctrl+U
Sleep 500ms

# The short way
Type "co5"
Sleep 500ms
Enter
Sleep 6s
Ctrl+C
Sleep 300ms
Ctrl+C
# Hide before claude's exit reaches the shell: the terminal's focus-in
# report gets echoed as a stray ^[[I there.
Hide
Sleep 1.5s

# The freeze-frame: breadth, grouped (fresh screen so the whole list fits)
Type "clear"
Enter
Show
Type "clau list"
Sleep 500ms
Enter
Sleep 4s

# The close-up: one token, everything (note the masked auth token)
Type "clau show qw35f"
Sleep 500ms
Enter
Sleep 5s
