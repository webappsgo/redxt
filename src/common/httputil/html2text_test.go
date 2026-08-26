package httputil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHTML2TextConverterElements(t *testing.T) {
	const width = 20

	tests := []struct {
		html   string
		want   string
		reason string
	}{
		{
			html:   "<h1>Jokes API</h1>",
			want:   strings.Repeat("═", width) + "\n     JOKES API\n" + strings.Repeat("═", width) + "\n",
			reason: "h1 box drawn, centered and uppercased",
		},
		{
			html:   "<h2>Random Joke</h2>",
			want:   "─── Random Joke ───\n",
			reason: "h2 rule on both sides",
		},
		{
			html:   "<h3>Section</h3>",
			want:   "► Section\n",
			reason: "h3 arrow marker",
		},
		{
			html:   "<p>alpha beta gamma delta</p>",
			want:   "alpha beta gamma\ndelta\n",
			reason: "paragraph wrapped to width",
		},
		{
			html:   "<p>See <a href=\"/x\">Home</a> now</p>",
			want:   "See Home [/x] now\n",
			reason: "link keeps its url",
		},
		{
			html:   "<p><strong>bold</strong> and <b>also</b></p>",
			want:   "*bold* and *also*\n",
			reason: "bold markers",
		},
		{
			html:   "<p><em>soft</em> and <i>slanted</i></p>",
			want:   "_soft_ and _slanted_\n",
			reason: "italic markers",
		},
		{
			html:   "<p><code>go vet</code></p>",
			want:   "`go vet`\n",
			reason: "code backticks",
		},
		{
			html:   "<pre>line one\nline two</pre>",
			want:   "    line one\n    line two\n",
			reason: "pre indented four spaces, verbatim",
		},
		{
			html:   "<hr>",
			want:   strings.Repeat("─", width) + "\n",
			reason: "hr rule spans the width",
		},
		{
			html:   "<blockquote>quoted text</blockquote>",
			want:   "│ quoted text\n",
			reason: "blockquote left border",
		},
		{
			html:   "<p>first<br>second</p>",
			want:   "first\nsecond\n",
			reason: "br breaks the line",
		},
		{
			html:   "<ul><li>One</li><li>Two</li></ul>",
			want:   "  • One\n  • Two\n",
			reason: "unordered list bullets",
		},
		{
			html:   "<ol><li>One</li><li>Two</li></ol>",
			want:   "  1. One\n  2. Two\n",
			reason: "ordered list numbering",
		},
		{
			html:   "<ul><li><a href=\"/\">Home</a></li></ul>",
			want:   "  • Home [/]\n",
			reason: "link inside a list item",
		},
		{
			html:   "<table><tr><th>A</th><th>BB</th></tr><tr><td>1</td><td>2</td></tr></table>",
			want:   " A │ BB\n───┼────\n 1 │ 2\n",
			reason: "ascii table with header rule",
		},
		{
			html:   "<p>a &amp; b</p>",
			want:   "a & b\n",
			reason: "entities decoded by the parser",
		},
		{
			html:   "<html><head><title>Page Title</title></head><body><p>body text</p></body></html>",
			want:   "body text\n",
			reason: "document metadata dropped",
		},
		{
			html:   "<p>one</p><form><input name=\"q\"><button>Go</button></form><script>var x=1;</script><style>b{color:red}</style><p>two</p>",
			want:   "one\n\ntwo\n",
			reason: "form, input, button, script and style skipped entirely",
		},
		{
			html:   "",
			want:   "\n",
			reason: "empty input still ends with one newline",
		},
		{
			html:   "<div><p>alpha</p><p>beta</p></div>",
			want:   "alpha\n\nbeta\n",
			reason: "blank line between paragraphs, never more",
		},
	}

	for _, tc := range tests {
		t.Run(tc.reason, func(t *testing.T) {
			got := HTML2TextConverter(tc.html, width)
			if got != tc.want {
				t.Errorf("HTML2TextConverter(%q, %d) =\n%q\nwant\n%q", tc.html, width, got, tc.want)
			}
		})
	}
}

func TestHTML2TextConverterTrailingNewline(t *testing.T) {
	inputs := []string{
		"",
		"<p>text</p>",
		"<p>text</p>\n\n\n",
		"<h1>Title</h1><p>body</p>",
		"<pre>code\n\n\n</pre>",
		"<ul><li>a</li></ul>",
		"plain text with no tags",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			got := HTML2TextConverter(in, 80)
			if !strings.HasSuffix(got, "\n") {
				t.Errorf("output for %q does not end with a newline: %q", in, got)
			}
			if strings.HasSuffix(got, "\n\n") {
				t.Errorf("output for %q ends with more than one newline: %q", in, got)
			}
		})
	}
}

func TestHTML2TextConverterWidth(t *testing.T) {
	// An unusable width falls back to the documented 80 columns.
	for _, width := range []int{0, -5, 3} {
		got := HTML2TextConverter("<hr>", width)
		rule := strings.TrimSuffix(got, "\n")
		if utf8.RuneCountInString(rule) != defaultRenderWidth {
			t.Errorf("width %d: rule is %d columns, want %d", width, utf8.RuneCountInString(rule), defaultRenderWidth)
		}
	}

	// A usable width is honored exactly.
	got := HTML2TextConverter("<hr>", 40)
	rule := strings.TrimSuffix(got, "\n")
	if utf8.RuneCountInString(rule) != 40 {
		t.Errorf("rule is %d columns, want 40", utf8.RuneCountInString(rule))
	}
}

func TestWordWrap(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		want   string
		reason string
	}{
		{text: "one two three", width: 20, want: "one two three", reason: "short line untouched"},
		{text: "alpha beta gamma delta", width: 20, want: "alpha beta gamma\ndelta", reason: "wraps on word boundary"},
		{text: "first\nsecond", width: 20, want: "first\nsecond", reason: "explicit breaks preserved"},
		{text: "", width: 20, want: "", reason: "empty text"},
		{text: "supercalifragilisticexpialidocious word", width: 20, want: "supercalifragilisticexpialidocious\nword", reason: "an over-long word is never split"},
	}

	for _, tc := range tests {
		t.Run(tc.reason, func(t *testing.T) {
			if got := wordWrap(tc.text, tc.width); got != tc.want {
				t.Errorf("wordWrap(%q, %d) = %q, want %q", tc.text, tc.width, got, tc.want)
			}
		})
	}
}

func TestCenterText(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		want   string
		reason string
	}{
		{text: "HI", width: 10, want: "    HI", reason: "even padding"},
		{text: "ODD", width: 10, want: "   ODD", reason: "odd padding rounds down"},
		{text: "EXACTLY", width: 7, want: "EXACTLY", reason: "text as wide as the field"},
		{text: "TOO LONG FOR FIELD", width: 5, want: "TOO LONG FOR FIELD", reason: "text wider than the field"},
	}

	for _, tc := range tests {
		t.Run(tc.reason, func(t *testing.T) {
			if got := centerText(tc.text, tc.width); got != tc.want {
				t.Errorf("centerText(%q, %d) = %q, want %q", tc.text, tc.width, got, tc.want)
			}
		})
	}
}

func TestCollapseSpaces(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		reason string
	}{
		{in: "  a   b  ", want: "a b", reason: "runs collapsed and trimmed"},
		{in: "a \n b", want: "a\nb", reason: "spaces around a break dropped"},
		{in: "\n\na", want: "a", reason: "leading breaks dropped"},
		{in: "   ", want: "", reason: "whitespace only"},
		{in: "a\tb", want: "a b", reason: "tabs become spaces"},
	}

	for _, tc := range tests {
		t.Run(tc.reason, func(t *testing.T) {
			if got := collapseSpaces(tc.in); got != tc.want {
				t.Errorf("collapseSpaces(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripTags(t *testing.T) {
	got := stripTags("<p>hello <b>world</b></p>")
	if got != "hello world" {
		t.Errorf("stripTags = %q, want %q", got, "hello world")
	}
}
