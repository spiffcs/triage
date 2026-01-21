# VHS Demo for Priority CLI
# Run with: vhs .github/demo.tape

Output .github/demo.gif
Output .github/demo.webm

# Optimized settings for GitHub README
Set Width 1800
Set Height 1200
Set Framerate 24
Set TypingSpeed 50ms
Set CursorBlink false
Set LoopOffset 50%

# Start with a clean terminal
Type "# Priority - GitHub Notification Prioritizer"
Enter
Sleep 500ms

Type "# Let's see what needs our attention today"
Enter
Sleep 1s

# Run the main list command with GitHub token
Type "GITHUB_TOKEN=$(gh auth token) priority list --since 1w"
Enter
Sleep 10s

# Show the output
Sleep 5s

# Now show filtering by urgent
Type "# Filter to just urgent items"
Enter
Sleep 500ms

Type "GITHUB_TOKEN=$(gh auth token) priority list -c urgent --since 1w"
Enter
Sleep 10s

# Show quick wins
Type "# What are our quick wins?"
Enter
Sleep 500ms

Type "GITHUB_TOKEN=$(gh auth token) priority list -c low-hanging --since 1w"
Enter
Sleep 10s

# Show summary view
Type "# Get a summary overview"
Enter
Sleep 500ms

Type "GITHUB_TOKEN=$(gh auth token) priority summary --since 1w"
Enter
Sleep 10s

# End
Sleep 2s
