package htmlmd

import (
	"strings"
	"testing"
)

func TestConvert(t *testing.T) {
	in := `<rendered-html><p>Enable the <code>foo_bar</code> service, see
<a href="https://example.com/docs">the docs</a> and <a href="#opt-x">x</a>.</p>
<p><strong>Never</strong> use <em>snake_case</em> here.</p>
<ul><li><p>one</p></li><li>two</li></ul>
<pre><code>{
  a = 1;
}
</code></pre>
<div class="warning"><p>Careful!</p></div></rendered-html>`
	got := Convert(in)
	for _, want := range []string{
		"Enable the `foo_bar` service, see [the docs](https://example.com/docs) and x.",
		"**Never** use *snake\\_case* here.",
		"• one\n• two",
		"```nix\n{\n  a = 1;\n}\n```",
		"> **Warning:** Careful!",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
