# Focused tour: relationship explorer → insert-related → ERD path → JOIN.
# Larger canvas + GitHub Light (matches the developer's usual creel look).
# Regenerate: ./scripts/record-graph-tour.sh

Output docs/images/demo-graph.gif

Require creel

Set Shell "bash"
Set Width 1400
Set Height 880
Set FontSize 13
Set Padding 0
Set Framerate 18
Set TypingSpeed 45ms
# Light terminal so transparent_background + git-hub-light-default read correctly.
Set Theme { "name": "github-light", "background": "#ffffff", "foreground": "#1f2328", "cursor": "#0969da", "selection": "#ddeeff", "black": "#24292f", "red": "#cf222e", "green": "#116329", "yellow": "#4d2d00", "blue": "#0969da", "magenta": "#8250df", "cyan": "#1b7c83", "white": "#6e7781", "brightBlack": "#57606a", "brightRed": "#a40e26", "brightGreen": "#1a7f37", "brightYellow": "#633c01", "brightBlue": "#218bff", "brightMagenta": "#a475f9", "brightCyan": "#3192aa", "brightWhite": "#1f2328" }

# Clean config is prepared by scripts/record-graph-tour.sh
Hide
Type "export XDG_CONFIG_HOME=/tmp/creel-vhs-config"
Enter
Sleep 200ms
Type "unset NO_COLOR"
Enter
Sleep 100ms
Type "creel -database demo/creel-demo.db"
Enter
Sleep 2.5s
Show

Type ":goto users"
Enter
Sleep 2s

Type "l"
Sleep 500ms

Type "gr"
Sleep 2.5s

Down@300ms 2
Sleep 500ms
Type "A"
Sleep 2s
Escape
Sleep 800ms

Type ":explore"
Enter
Sleep 800ms

Type ":erd"
Enter
Sleep 2.5s

Type "p"
Sleep 800ms
Down@200ms 3
Sleep 500ms
Type "p"
Sleep 1.2s
Type "i"
Sleep 2.5s
Escape
Sleep 1.2s
