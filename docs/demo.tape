# clau README demo — render with: vhs docs/demo.tape (from repo root)
Output docs/demo.gif
Set Shell "fish"
Set FontSize 20
Set Width 1200
Set Height 700
Set Framerate 25
Set TypingSpeed 75ms

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
Sleep 1.5s

# The freeze-frame: breadth
Type "clau list"
Sleep 500ms
Enter
Sleep 4s
