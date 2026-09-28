package core

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"mayuri2000/internal/cmd"
	"mayuri2000/internal/imaging"
)

const (
	// maxInline is the longest output shown in a message; longer results
	// are attached as a file.
	maxInline = 1900
	// maxBase64Input bounds attachments and decoded data.
	maxBase64Input = 1 << 20
)

func base64Command() *cmd.Command {
	return &cmd.Command{
		Name:        "base64",
		Aliases:     []string{"b64"},
		Category:    "Tools",
		Description: "Encode or decode base64",
		Subcommands: []*cmd.Command{
			{
				Name: "encode", Aliases: []string{"enc", "e"}, Description: "Encode text or a file as base64",
				Options: []cmd.Option{
					{Name: "text", Description: "Text to encode", Type: cmd.String, Rest: true, MaxLength: cmd.Ptr(4000)},
					{Name: "file", Description: "File to encode instead of text (max 1 MiB)", Type: cmd.Attachment},
					{Name: "urlsafe", Description: "Use the URL-safe alphabet without padding", Type: cmd.Bool, SlashOnly: true},
				},
				Run: base64Encode,
			},
			{
				Name: "decode", Aliases: []string{"dec", "d"}, Description: "Decode base64 (standard or URL-safe, padding optional)",
				Options: []cmd.Option{
					{Name: "text", Description: "Base64 to decode", Type: cmd.String, Required: true, Rest: true, MaxLength: cmd.Ptr(6000)},
				},
				Run: base64Decode,
			},
		},
	}
}

func base64Encode(c *cmd.Context) error {
	var input []byte
	name := "encoded.txt"
	if a, ok := c.OptAttachment("file"); ok {
		if a.Size > maxBase64Input {
			return cmd.Errorf("That file is too big to encode (max 1 MiB).")
		}
		data, err := downloadAttachment(c, a.URL)
		if err != nil {
			return cmd.Errorf("I couldn't download that file.")
		}
		input, name = data, a.Filename+".b64.txt"
	} else if text, ok := c.OptString("text"); ok {
		input = []byte(text)
	} else {
		return cmd.Errorf("Give me some text or attach a file to encode.")
	}

	enc := base64.StdEncoding
	if c.Bool("urlsafe") {
		enc = base64.RawURLEncoding
	}
	return replyOutput(c, enc.EncodeToString(input), name)
}

// DecodeBase64 accepts standard or URL-safe base64, with or without padding
// and ignoring whitespace (e.g. line-wrapped input).
func DecodeBase64(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimRight(s, "=")
	if strings.ContainsAny(s, "-_") {
		return base64.RawURLEncoding.DecodeString(s)
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func base64Decode(c *cmd.Context) error {
	data, err := DecodeBase64(c.String("text"))
	if err != nil {
		return cmd.Errorf("That isn't valid base64.")
	}
	if len(data) == 0 {
		return cmd.Errorf("That decodes to nothing.")
	}
	if !IsPrintableText(data) {
		name := "decoded.bin"
		if format := imaging.Sniff(data); format != "" {
			name = "decoded." + format
		}
		return c.ReplyFile(fmt.Sprintf("Decoded %d bytes of binary data.", len(data)), name, data)
	}
	return replyOutput(c, string(data), "decoded.txt")
}

// IsPrintableText reports whether data is UTF-8 text without control
// characters other than common whitespace.
func IsPrintableText(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, r := range string(data) {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

// replyOutput shows text in a code block, or attaches it when it is too long
// or would break out of the code block.
func replyOutput(c *cmd.Context, text, filename string) error {
	if len(text) <= maxInline && !strings.Contains(text, "```") {
		return c.Reply("```\n" + text + "\n```")
	}
	return c.ReplyFile("", filename, []byte(text))
}

func downloadAttachment(c *cmd.Context, url string) ([]byte, error) {
	// Attachment URLs come from Discord's CDN, but go through the safe
	// client anyway since the URL is part of the interaction payload.
	data, err := imaging.Download(c, url)
	if err != nil {
		return nil, err
	}
	if len(data) > maxBase64Input {
		return nil, io.ErrShortBuffer
	}
	return data, nil
}
