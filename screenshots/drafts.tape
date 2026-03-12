# Screenshot: Drafts View
# Shows the drafts management screen

Output screenshots/drafts.gif

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

# Navigate to Drafts (3rd menu item when drafts exist)
Down
Sleep 300ms
Down
Sleep 300ms
Enter

Sleep 1.5s

Screenshot screenshots/drafts.png
