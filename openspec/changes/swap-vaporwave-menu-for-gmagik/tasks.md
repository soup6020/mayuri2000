# Tasks

## 1. Menu

- [x] 1.1 In `Menus` (`internal/modules/notsobot/notsobot.go`) replace `menu("Vaporwave", "vw", …)` with `menu("GMagik", "gmagik", cmd.MenuMessage)`, keeping the order Magik, JPEG, Glitch, GMagik, Invert; verify `TestCommandsValid` passes (≤5 message menus)
- [x] 1.2 Add a test asserting the message menu names are exactly Magik, JPEG, Glitch, GMagik and Invert, that the user menus are Triggered and Pet, and that `vw` is still a registered command; verify `go test ./...` passes

## 2. Docs and checks

- [x] 2.1 Update the README context-menu list to *Apps → Magik / JPEG / Glitch / GMagik / Invert*; verify by reading the section
- [x] 2.2 Run `golangci-lint run ./...` (0 issues) and `nix flake check`; verify both pass
- [ ] 2.3 Manually: after restarting the bot, right-click a message with a still image and with a GIF → Apps → GMagik; verify the replies are a melting GIF and a magik'd GIF, and that Vaporwave is gone from the menu (Discord may need a client refresh, Ctrl+R)
