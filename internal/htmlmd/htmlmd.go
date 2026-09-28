// Package htmlmd converts HTML fragments (NixOS option docs, feed item
// summaries) into Discord markdown.
package htmlmd

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var (
	mdEscaper     = strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "~", `\~`, "|", `\|`, "`", "\\`")
	extraNewlines = regexp.MustCompile(`\n{3,}`)
	wsRuns        = regexp.MustCompile(`\s+`)
)

// Convert turns an HTML fragment into Discord markdown.
func Convert(src string) string {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return src
	}
	var b strings.Builder
	var walk func(n *html.Node, inPre bool, listDepth int)
	children := func(n *html.Node, inPre bool, listDepth int) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inPre, listDepth)
		}
	}
	inner := func(n *html.Node, inPre bool) string {
		saved := b.String()
		b.Reset()
		children(n, inPre, 0)
		out := b.String()
		b.Reset()
		b.WriteString(saved)
		return out
	}
	walk = func(n *html.Node, inPre bool, listDepth int) {
		switch n.Type {
		case html.TextNode:
			if inPre {
				b.WriteString(n.Data)
				return
			}
			// Collapse source line wrapping and indentation to single spaces,
			// keeping the spaces that separate text from inline elements.
			text := wsRuns.ReplaceAllString(n.Data, " ")
			if cur := b.String(); cur == "" || strings.HasSuffix(cur, "\n") || strings.HasSuffix(cur, " ") {
				text = strings.TrimLeft(text, " ")
			}
			b.WriteString(mdEscaper.Replace(text))
			return
		case html.ElementNode:
		default:
			children(n, inPre, listDepth)
			return
		}

		switch n.Data {
		case "p":
			children(n, inPre, listDepth)
			b.WriteString("\n\n")
		case "br":
			b.WriteByte('\n')
		case "pre":
			code := strings.TrimRight(textContent(n), "\n")
			b.WriteString("\n```nix\n" + code + "\n```\n")
		case "code":
			if inPre {
				children(n, true, listDepth)
				return
			}
			b.WriteString("`" + strings.ReplaceAll(textContent(n), "`", "'") + "`")
		case "em", "i":
			b.WriteString("*" + strings.TrimSpace(inner(n, inPre)) + "*")
		case "strong", "b":
			b.WriteString("**" + strings.TrimSpace(inner(n, inPre)) + "**")
		case "a":
			text := strings.TrimSpace(inner(n, inPre))
			href := attr(n, "href")
			switch {
			case strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://"):
				b.WriteString("[" + text + "](" + href + ")")
			default:
				b.WriteString(text)
			}
		case "ul", "ol":
			b.WriteByte('\n')
			i := 1
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode || c.Data != "li" {
					continue
				}
				bullet := "•"
				if n.Data == "ol" {
					bullet = strconv.Itoa(i) + "."
					i++
				}
				item := strings.TrimSpace(extraNewlines.ReplaceAllString(inner(c, inPre), "\n"))
				b.WriteString(strings.Repeat("  ", listDepth) + bullet + " " + item + "\n")
			}
			b.WriteByte('\n')
		case "dt":
			b.WriteString("**" + strings.TrimSpace(inner(n, inPre)) + "**\n")
		case "dd":
			b.WriteString(strings.TrimSpace(inner(n, inPre)) + "\n\n")
		case "div":
			class := attr(n, "class")
			for _, kind := range []string{"note", "warning", "important", "tip", "caution"} {
				if strings.Contains(class, kind) {
					body := strings.TrimSpace(inner(n, inPre))
					body = strings.TrimSpace(strings.TrimPrefix(body, "**"+strings.ToUpper(kind[:1])+kind[1:]+"**"))
					label := strings.ToUpper(kind[:1]) + kind[1:]
					b.WriteString("> **" + label + ":** " + strings.ReplaceAll(body, "\n", "\n> ") + "\n\n")
					return
				}
			}
			children(n, inPre, listDepth)
		case "h1", "h2", "h3", "h4", "h5", "h6":
			b.WriteString("**" + strings.TrimSpace(inner(n, inPre)) + "**\n")
		case "script", "style":
		default:
			children(n, inPre, listDepth)
		}
	}
	walk(doc, false, 0)

	// Trim trailing spaces only; leading indentation matters in code blocks.
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	out := extraNewlines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return strings.TrimSpace(out)
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return b.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
