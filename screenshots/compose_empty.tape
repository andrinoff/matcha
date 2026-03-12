# Screenshot: Empty Compose Screen
# Shows the clean compose interface before typing

Output screenshots/compose_empty.gif

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

Screenshot screenshots/compose_empty.png
