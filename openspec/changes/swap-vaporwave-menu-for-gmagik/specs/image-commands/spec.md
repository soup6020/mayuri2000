## MODIFIED Requirements

### Requirement: Context menus
The bot SHALL offer message context menus Magik, JPEG, Glitch, GMagik and Invert (applied to the target message's image) and user context menus Triggered and Pet (applied to the user's avatar). The GMagik menu SHALL behave like `/gmagik`: a still image becomes a melting GIF, a GIF is magik'd frame by frame, and an MP4 produces an MP4.

#### Scenario: Right-click a message
- **WHEN** a user picks Apps → Magik on a message with an image
- **THEN** the bot replies with that image magik'd

#### Scenario: GMagik from the Apps menu
- **WHEN** a user picks Apps → GMagik on a message with a still image
- **THEN** the bot replies with a 30 frame progressively melting GIF

#### Scenario: Vaporwave stays a command
- **WHEN** a user right-clicks a message and opens Apps
- **THEN** there is no Vaporwave entry, and `/vw` still works
