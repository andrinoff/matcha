# Screenshot: Email Composition
# Shows the compose screen with Markdown content being written

Output screenshots/compose_email.gif

Set FontSize 14
Set FontFamily "JetBrainsMono Nerd Font"
Set Width 1400
Set Height 800
Set Theme "Catppuccin Mocha"
Set Padding 20
Set WindowBar Colorful
Set WindowBarSize 40
Set BorderRadius 10

Hide
Type "go run ."
Enter
Show

Sleep 2s

# Navigate to Compose Email
Down
Sleep 300ms
Enter
Sleep 1.5s

# Type recipient
Type "alice@example.com"
Sleep 200ms
Tab
Sleep 200ms
Tab
Sleep 100ms
Tab
Sleep 200ms

# Type subject
Type "Weekly Project Update"
Sleep 200ms
Tab
Sleep 300ms

# Type email body with Markdown
Type "# Project Update"
Enter
Enter
Type "Hi team, here's our **weekly update**:"
Enter
Enter
Type "## Completed"
Enter
Type "- Shipped new dashboard redesign"
Enter
Type "- Fixed authentication bug in production"
Enter
Type "- Added unit tests for core modules"
Enter
Enter
Type "## In Progress"
Enter
Type "- API rate limiting implementation"
Enter
Type "- Database migration planning"
Enter
Enter
Type "Best regards"
Sleep 800ms

Screenshot screenshots/compose_email.png
