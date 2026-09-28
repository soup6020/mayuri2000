package cmd

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/disgoorg/snowflake/v2"
)

type token struct {
	Value string
	Start int // byte offset of the token (including any opening quote) in the raw input
}

// tokenize splits s on whitespace, honouring "double" and 'single' quotes.
func tokenize(s string) []token {
	var (
		toks  []token
		cur   strings.Builder
		start = -1
		quote rune
	)
	flush := func() {
		if start >= 0 {
			toks = append(toks, token{Value: cur.String(), Start: start})
		}
		cur.Reset()
		start = -1
	}
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			cur.WriteRune(r)
		case r == '"' || r == '\'' || r == '“' || r == '”':
			if start < 0 {
				start = i
			}
			// Only treat quotes as delimiters at the start of a token so that
			// apostrophes in words like "don't" survive.
			if cur.Len() == 0 {
				quote = r
				if r == '“' {
					quote = '”'
				}
				continue
			}
			cur.WriteRune(r)
		case unicode.IsSpace(r):
			flush()
		default:
			if start < 0 {
				start = i
			}
			cur.WriteRune(r)
		}
	}
	flush()
	return toks
}

var mentionRe = regexp.MustCompile(`^<@!?(\d{15,21})>$`)

// ParseUserID parses a user mention or raw snowflake.
func ParseUserID(s string) (snowflake.ID, bool) {
	if m := mentionRe.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if len(s) < 15 || len(s) > 21 {
		return 0, false
	}
	id, err := snowflake.Parse(s)
	return id, err == nil
}

func parseBool(s string) (bool, bool) {
	switch strings.ToLower(s) {
	case "true", "yes", "y", "on", "1", "enable", "enabled":
		return true, true
	case "false", "no", "n", "off", "0", "disable", "disabled":
		return false, true
	}
	return false, false
}

// rawArg is a prefix argument assigned to an option before type resolution.
type rawArg struct {
	opt   Option
	value string
}

// assign maps prefix tokens onto the command's options in declaration order.
// Returned extras are tokens that could not be assigned to any option.
func assign(opts []Option, raw string) (args []rawArg, missing []Option, extras []string) {
	toks := tokenize(raw)
	i := 0
	for _, o := range opts {
		if o.SlashOnly || o.Type == Attachment {
			continue
		}
		if i >= len(toks) {
			if o.Required {
				missing = append(missing, o)
			}
			continue
		}
		if o.Rest && o.Type == String {
			rest := strings.TrimSpace(raw[toks[i].Start:])
			if len(toks)-i == 1 {
				rest = toks[i].Value // strip quotes from a single quoted token
			}
			args = append(args, rawArg{opt: o, value: rest})
			i = len(toks)
			continue
		}
		tok := toks[i].Value
		if !o.Required && !accepts(o, tok) {
			continue
		}
		args = append(args, rawArg{opt: o, value: tok})
		i++
	}
	for ; i < len(toks); i++ {
		extras = append(extras, toks[i].Value)
	}
	return args, missing, extras
}

func accepts(o Option, tok string) bool {
	if o.Accept != nil {
		return o.Accept(tok)
	}
	switch o.Type {
	case Int:
		_, err := strconv.Atoi(tok)
		return err == nil
	case Float:
		_, err := strconv.ParseFloat(tok, 64)
		return err == nil
	case Bool:
		_, ok := parseBool(tok)
		return ok
	case User:
		_, ok := ParseUserID(tok)
		return ok
	}
	return true
}
