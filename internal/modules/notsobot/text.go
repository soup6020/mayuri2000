package notsobot

import (
	"strings"
	"unicode"
)

// Aesthetics converts printable ASCII to full-width characters.
func Aesthetics(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == ' ':
			return '　'
		case r >= 33 && r < 127:
			return r + 65248
		}
		return r
	}, s)
}

var emojiMap = map[rune]string{
	'0': "0️⃣", '1': "1️⃣", '2': "2️⃣", '3': "3️⃣", '4': "4️⃣",
	'5': "5️⃣", '6': "6️⃣", '7': "7️⃣", '8': "8️⃣", '9': "9️⃣",
	'$': "💲", '!': "❗", '?': "❓", ' ': "　",
}

// Emojify replaces letters with regional indicator emoji.
func Emojify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z':
			// Regional indicators; a zero-width space stops adjacent pairs
			// rendering as flags.
			b.WriteRune(0x1F1E6 + (r - 'a'))
			b.WriteRune('​')
		case emojiMap[r] != "":
			b.WriteString(emojiMap[r])
		case !unicode.IsControl(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}
