# Screenshot: Inbox View
# Shows a populated inbox with emails from various senders and dates

Output screenshots/inbox_view.gif

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
Type "go run ./screenshots/cmd/inbox_view"
Enter
Show

Sleep 2.5s

Screenshot screenshots/inbox_view.png
