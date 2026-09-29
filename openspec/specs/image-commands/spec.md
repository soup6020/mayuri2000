# image-commands Specification

## Purpose
Image manipulation commands ported from the NotSoBot Red cog, plus effects reimplemented from upstream NotSoBot (whose processing runs on a closed-source API). Covers how input images are found, limits, output rules and each effect's contract.

## Requirements

### Requirement: Finding the input image
Image commands SHALL use the first image found in this order: the `image` attachment option; the `url` option or leftover text arguments (image links, Tenor links, custom emoji, user mentions or IDs as avatars); the `user` option's avatar; other attachments on the invoking message; the replied-to or context-menu target message; then the last 10 messages in the channel. If none is found the bot MUST say how to provide one.

#### Scenario: Image from history
- **WHEN** a user runs `/invert` with no arguments right after someone posted a picture
- **THEN** the bot inverts that picture

#### Scenario: Avatar by mention
- **WHEN** a user sends `%triggered @friend`
- **THEN** the bot uses @friend's avatar (animated avatars as GIF)

#### Scenario: Tenor link
- **WHEN** a user sends `%magik https://tenor.com/view/...`
- **THEN** the bot resolves the Tenor page to its GIF (via the Tenor API when a key is configured, otherwise by scraping)

### Requirement: Input limits and formats
Inputs SHALL be PNG, JPEG, GIF or WebP identified by content, at most 25 MiB and 40 megapixels per frame; animated inputs SHALL keep at most 200 frames. Unsupported or oversized inputs MUST produce a clear user-facing error.

#### Scenario: Not an image
- **WHEN** the URL points at an HTML page
- **THEN** the bot replies that it doesn't look like a supported image

### Requirement: Animated images
Effects SHALL process every frame of an animated GIF and return a GIF, unless the effect's contract says otherwise. Random effects SHALL use the same randomness on every frame so animations don't flicker.

#### Scenario: GIF in, GIF out
- **WHEN** a user runs `/invert` on an animated GIF
- **THEN** the result is an animated GIF with every frame inverted

### Requirement: Output upload limits
Results larger than the channel's upload limit (the interaction's limit, else 10/50/100 MiB by server boost tier) MUST NOT be uploaded; the user SHALL be told the result was too large.

#### Scenario: Oversized result
- **WHEN** an effect produces a 30 MiB GIF in an unboosted server
- **THEN** the bot says the result is too large to upload

### Requirement: ImageMagick-backed effects
`magik`, `gmagik`, `caption`, `watermark`, `wall` and `watercolor` SHALL use the ImageMagick CLI; `glitch` SHALL use it to render corrupted JPEGs when available. When ImageMagick (or liblqr for magik/gmagik) is unavailable those commands MUST report that instead of failing obscurely, and all other commands SHALL keep working.

#### Scenario: No liquid rescale support
- **WHEN** ImageMagick lacks liblqr and a user runs `/magik`
- **THEN** the bot says magik needs ImageMagick with liquid rescale support

### Requirement: NotSoBot cog commands
The bot SHALL provide these commands with the original Red aliases: magik (imagemagic, imagemagick, magic, magick, cas, liquid; `scale` 1-10), gmagik, petimg (pet, petpet), caption (text, color, size, x/y as % from left/top), triggered, merge (vertical default true, 2+ images), jpeg (needsmorejpeg, jpegify, magik2; quality 1-10), vw (vaporwave, vape, vapewave; text), watermark (wm; mark, x, y, transparency), glitch (jpglitch; iterations, amount, seed).

#### Scenario: Magik on a still
- **WHEN** a user runs `/magik scale:5` on a photo
- **THEN** the bot returns a liquid-rescaled PNG

#### Scenario: gmagik on a still
- **WHEN** a user runs `/gmagik` on a still image
- **THEN** the bot returns a 30 frame progressively melting GIF

### Requirement: Further NotSoBot cog commands
The bot SHALL also provide: pixelate (pixel; pixels 2-100), waaw (magik3, mirror), haah (magik4, mirror2), woow (magik5, mirror3), hooh (magik6, mirror4), flipimg (flip), flop, invert (inverse, negate), rotate (degrees -360..360, given before the image in text form), iascii (gascii; columns 10-200), ascii (expand; starwars figlet text), rip, minecraftachievement (achievement).

#### Scenario: Mirror left onto right
- **WHEN** a user runs `/haah`
- **THEN** the left half of the image is mirrored onto the right half at the original size

### Requirement: Upstream NotSoBot effects
The bot SHALL provide effects matching upstream NotSoBot's names, options and defaults: deepfry (fry, df; scale 1-20 default 10, notransparency), solarize, shake (horizontal/vertical as % of size default 4, randomize), swapcolors, swappixels, swaprgba (channels), wall, watercolor (watercolour; contrast 0, edge 2, mixing 33, smoothing 0).

#### Scenario: Shake a still
- **WHEN** a user runs `/shake` on a still image
- **THEN** the bot returns a 12 frame GIF of the image jittering, with no empty borders

#### Scenario: Deepfry keeps transparency
- **WHEN** a user deep fries a PNG with transparent areas without `notransparency`
- **THEN** the transparent areas stay transparent

### Requirement: swaprgba channel specification
`swaprgba` SHALL accept 3 or 4 characters, each `r`, `g`, `b`, `a` (copy that channel), `0` (empty) or `1` (full), case-insensitive; a 3 character spec keeps alpha. Anything else MUST be rejected with an explanation.

#### Scenario: Swap red and blue
- **WHEN** a user runs `%swaprgba bgr` on an image
- **THEN** red and blue are swapped and alpha is unchanged

### Requirement: swapcolors keeps transparency
`swapcolors` SHALL shuffle only the red, green and blue values within each pixel and MUST leave alpha unchanged, even though upstream describes it as RGBA, so opaque images stay opaque.

#### Scenario: Opaque image
- **WHEN** a user runs `/swapcolors` on an opaque photo
- **THEN** every output pixel is fully opaque and holds a permutation of its original RGB values

### Requirement: Text effects
`aesthetics` (aes) SHALL convert ASCII to full-width characters (input at most 1000 characters) and `emojify` SHALL replace letters, digits and some symbols with emoji (input at most 200 characters); emojify output over 2000 characters MUST be refused.

#### Scenario: Emojify
- **WHEN** a user runs `/emojify text:Ab1`
- **THEN** the reply is regional indicator A, B and keycap 1, with adjacent letters kept from forming flags

### Requirement: Context menus
The bot SHALL offer message context menus Magik, JPEG, Glitch, Vaporwave and Invert (applied to the target message's image) and user context menus Triggered and Pet (applied to the user's avatar).

#### Scenario: Right-click a message
- **WHEN** a user picks Apps → Magik on a message with an image
- **THEN** the bot replies with that image magik'd
