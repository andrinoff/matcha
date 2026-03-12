# Screenshot: Email View with Inline Images
# Shows an HTML email with formatted content, inline image placeholders,
# attachments list, and hyperlinks

Output screenshots/email_view.gif

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
Type "go run ./screenshots/cmd/email_view"
Enter
Show

Sleep 2.5s

Screenshot screenshots/email_view.png
