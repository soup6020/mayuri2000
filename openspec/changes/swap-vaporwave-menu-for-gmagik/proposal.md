# Proposal

## Why

`gmagik` has been one of the most popular commands with the trial users, but it's only reachable by typing it. Vaporwave, which has an Apps-menu slot, gets far less use. Discord allows only 5 message context menus and all 5 are taken, so GMagik has to replace one.

## What Changes

- The **Vaporwave** message context menu (right-click a message → Apps) is replaced by a **GMagik** menu that runs `gmagik` on the target message's image:
  - a still image becomes the 30-frame melting GIF;
  - a GIF is magik'd frame by frame;
  - an MP4 gives MP4 back, with its audio.
- `/vw` and `%vw` (vaporwave, vape, vapewave) are unchanged. Only the menu entry goes.
- The message menus become Magik, JPEG, Glitch, GMagik and Invert. User menus are unchanged.
- Discord limits: still 5 message menus. Once the bot syncs commands, the old Vaporwave entry disappears from clients.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `image-commands`: the "Context menus" requirement swaps Vaporwave for GMagik.

## Impact

- `internal/modules/notsobot/notsobot.go` (`Menus`): one entry changes.
- A test that checks the registered message menus.
- README context-menu list.
- No new dependencies. The bot's global command sync handles the change on the next start.
