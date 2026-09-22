# Focused tour: relationship explorer → insert-related → ERD path → JOIN.
# Regenerate from repo root (creel on PATH, demo DB built):
#   vhs demo/graph-tour.tape

Output docs/images/demo-graph.gif

Require creel

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 640
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set Framerate 18
Set Padding 12

# Clean config so no session restore leaks into the tape.
Hide
Type "rm -rf /tmp/creel-vhs-config && mkdir -p /tmp/creel-vhs-config && export XDG_CONFIG_HOME=/tmp/creel-vhs-config"
Enter
Sleep 300ms
Type "creel -database demo/creel-demo.db"
Enter
Sleep 2.5s
Show

# Editor starts focused (NORMAL). Open users via ex command.
Type ":goto users"
Enter
Sleep 2s

# :goto leaves focus on the sidebar — move to results for g-chords.
Type "l"
Sleep 500ms

# Relationship explorer on the current row
Type "gr"
Sleep 2.5s

# Inbound orders edge → insert-related (cancel after it opens)
Down@300ms 2
Sleep 500ms
Type "A"
Sleep 2s
Escape
Sleep 800ms

# Close explorer via :explore (do not press q — global q quits the app)
Type ":explore"
Enter
Sleep 800ms

# Static ERD overlay
Type ":erd"
Enter
Sleep 2.5s

# Path: anchor on focused card, move to a neighbour, trace, drop JOIN SQL
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
