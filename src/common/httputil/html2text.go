package httputil

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// defaultRenderWidth is the terminal width used when the caller passes a
// width that is not usable, matching the 80 columns AI.md PART 14 uses in
// its handler example.
const defaultRenderWidth = 80

// minRenderWidth is the narrowest width the renderer will honor. Below
// this, box rules and word wrapping stop being readable.
const minRenderWidth = 20

// skippedElements lists the elements dropped entirely by the converter,
// per the AI.md PART 14 HTML2Text conversion table. Forms and their
// controls are non-interactive in a terminal dump, and script/style carry
// no readable content. The document head is skipped for the same reason:
// its title, meta and link children are page metadata, not page content.
var skippedElements = map[string]bool{
	"head":   true,
	"form":   true,
	"input":  true,
	"button": true,
	"script": true,
	"style":  true,
}

// blockContainers lists the generic grouping elements that carry no
// formatting of their own but must not let their contents run together on
// one line.
var blockContainers = map[string]bool{
	"div":     true,
	"section": true,
	"article": true,
	"header":  true,
	"footer":  true,
	"main":    true,
	"nav":     true,
	"aside":   true,
}

// textBuffer accumulates rendered output and remembers the last byte
// written, so a block element can start on a fresh line without rescanning
// everything written so far.
type textBuffer struct {
	builder strings.Builder
	last    byte
}

// WriteString appends s to the buffer.
func (t *textBuffer) WriteString(s string) {
	if s == "" {
		return
	}
	t.builder.WriteString(s)
	t.last = s[len(s)-1]
}

// String returns everything written so far.
func (t *textBuffer) String() string {
	return t.builder.String()
}

// endLine terminates the current line unless the buffer is empty or
// already ends with a newline.
func (t *textBuffer) endLine() {
	if t.last != 0 && t.last != '\n' {
		t.WriteString("\n")
	}
}

// HTML2TextConverter converts rendered HTML into beautifully formatted
// terminal text, per AI.md PART 14 "HTML2TextConverter Function". It is a
// custom renderer, not a library wrapper: headings become box-drawn rules,
// links keep their URL, tables become ASCII tables, and non-interactive
// elements such as forms and scripts are dropped.
//
// width is the target terminal column count; a width below minRenderWidth
// is raised to defaultRenderWidth. The returned text always ends with
// exactly one newline, so converting empty or content-free HTML yields a
// single newline.
func HTML2TextConverter(htmlSrc string, width int) string {
	if width < minRenderWidth {
		width = defaultRenderWidth
	}

	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		// Parsing a string never normally fails; fall back to a plain tag
		// strip rather than losing the page entirely.
		return normalizeOutput(stripTags(htmlSrc))
	}

	buf := &textBuffer{}
	convertNode(buf, doc, width, 0)
	return normalizeOutput(buf.String())
}

// convertNode renders one node and its children into buf. indent is the
// number of columns already consumed by an enclosing structure.
func convertNode(buf *textBuffer, n *html.Node, width, indent int) {
	switch n.Type {
	case html.ElementNode:
		convertElement(buf, n, width, indent)
	case html.TextNode:
		text := collapseSpaces(flattenWhitespace(n.Data))
		if text != "" {
			buf.WriteString(text)
		}
	default:
		convertChildren(buf, n, width, indent)
	}
}

// convertChildren renders every child of n in document order.
func convertChildren(buf *textBuffer, n *html.Node, width, indent int) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		convertNode(buf, c, width, indent)
	}
}

// convertElement renders a single element according to the AI.md PART 14
// conversion table.
func convertElement(buf *textBuffer, n *html.Node, width, indent int) {
	if skippedElements[n.Data] {
		return
	}

	switch n.Data {
	case "h1":
		text := inlineText(n)
		rule := strings.Repeat("═", width)
		buf.endLine()
		buf.WriteString(rule + "\n")
		buf.WriteString(centerText(strings.ToUpper(text), width) + "\n")
		buf.WriteString(rule + "\n\n")
	case "h2":
		buf.endLine()
		buf.WriteString("─── " + inlineText(n) + " ───\n\n")
	case "h3", "h4", "h5", "h6":
		buf.endLine()
		buf.WriteString("► " + inlineText(n) + "\n\n")
	case "p":
		text := inlineText(n)
		if text == "" {
			return
		}
		buf.endLine()
		buf.WriteString(wordWrap(text, width-indent) + "\n\n")
	case "ul":
		convertList(buf, n, width, indent, false)
	case "ol":
		convertList(buf, n, width, indent, true)
	case "a", "strong", "b", "em", "i", "code":
		// Inline elements standing on their own outside a block render
		// exactly as they would inside one.
		buf.WriteString(inlineElement(n))
	case "pre":
		convertPre(buf, n)
	case "table":
		convertTable(buf, n)
	case "hr":
		buf.endLine()
		buf.WriteString(strings.Repeat("─", width) + "\n\n")
	case "blockquote":
		convertBlockquote(buf, n, width, indent)
	case "br":
		buf.WriteString("\n")
	default:
		convertChildren(buf, n, width, indent)
		if blockContainers[n.Data] {
			buf.endLine()
		}
	}
}

// convertPre renders a preformatted block verbatim, indented four spaces.
func convertPre(buf *textBuffer, n *html.Node) {
	text := strings.Trim(rawText(n), "\n")
	if text == "" {
		return
	}
	buf.endLine()
	for _, line := range strings.Split(text, "\n") {
		buf.WriteString("    " + line + "\n")
	}
	buf.WriteString("\n")
}

// convertBlockquote renders a quote with a left border, wrapped to width.
func convertBlockquote(buf *textBuffer, n *html.Node, width, indent int) {
	text := inlineText(n)
	if text == "" {
		return
	}
	buf.endLine()
	for _, line := range strings.Split(wordWrap(text, width-indent-2), "\n") {
		buf.WriteString(strings.TrimRight("│ "+line, " ") + "\n")
	}
	buf.WriteString("\n")
}

// convertList renders a ul or ol, bulleting or numbering each li and
// wrapping long items under a hanging indent.
func convertList(buf *textBuffer, n *html.Node, width, indent int, ordered bool) {
	buf.endLine()
	number := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "li" {
			continue
		}
		text := inlineText(c)
		if text == "" {
			continue
		}
		marker := "• "
		if ordered {
			marker = strconv.Itoa(number) + ". "
			number++
		}
		prefix := strings.Repeat(" ", indent+2) + marker
		hanging := strings.Repeat(" ", runeLen(prefix))
		for i, line := range strings.Split(wordWrap(text, width-runeLen(prefix)), "\n") {
			if i == 0 {
				buf.WriteString(prefix + line + "\n")
				continue
			}
			buf.WriteString(hanging + line + "\n")
		}
	}
	buf.WriteString("\n")
}

// convertTable renders a table as an ASCII table using the box-drawing
// characters required by AI.md PART 14. The first row is treated as the
// header and is followed by a rule.
func convertTable(buf *textBuffer, n *html.Node) {
	rows := collectRows(n)
	if len(rows) == 0 {
		return
	}

	columns := 0
	for _, row := range rows {
		if len(row) > columns {
			columns = len(row)
		}
	}

	widths := make([]int, columns)
	for _, row := range rows {
		for i, cell := range row {
			if length := runeLen(cell); length > widths[i] {
				widths[i] = length
			}
		}
	}

	buf.endLine()
	for index, row := range rows {
		cells := make([]string, columns)
		for i := range cells {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			cells[i] = " " + cell + strings.Repeat(" ", widths[i]-runeLen(cell)) + " "
		}
		buf.WriteString(strings.TrimRight(strings.Join(cells, "│"), " ") + "\n")

		if index != 0 {
			continue
		}
		rules := make([]string, columns)
		for i := range rules {
			rules[i] = strings.Repeat("─", widths[i]+2)
		}
		buf.WriteString(strings.Join(rules, "┼") + "\n")
	}
	buf.WriteString("\n")
}

// collectRows gathers the cell text of every tr in the table, in document
// order. Nested tables are not descended into, so their rows never leak
// into the enclosing table.
func collectRows(n *html.Node) [][]string {
	var rows [][]string

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || skippedElements[c.Data] || c.Data == "table" {
				continue
			}
			if c.Data == "tr" {
				rows = append(rows, collectCells(c))
				continue
			}
			walk(c)
		}
	}
	walk(n)

	return rows
}

// collectCells returns the rendered text of every th or td in a row.
func collectCells(row *html.Node) []string {
	var cells []string
	for c := row.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		if c.Data != "th" && c.Data != "td" {
			continue
		}
		cells = append(cells, inlineText(c))
	}
	return cells
}

// inlineText renders the children of n as a single inline string, applying
// the link, bold, italic and code markers and collapsing whitespace.
func inlineText(n *html.Node) string {
	var b strings.Builder
	writeInline(&b, n)
	return collapseSpaces(b.String())
}

// inlineElement renders n itself, markers included, as inline text.
func inlineElement(n *html.Node) string {
	var b strings.Builder
	writeInlineElement(&b, n)
	return collapseSpaces(b.String())
}

// writeInline renders the children of n into b.
func writeInline(b *strings.Builder, n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			b.WriteString(flattenWhitespace(c.Data))
		case html.ElementNode:
			writeInlineElement(b, c)
		}
	}
}

// writeInlineElement renders one element and its children into b.
func writeInlineElement(b *strings.Builder, n *html.Node) {
	if skippedElements[n.Data] {
		return
	}

	switch n.Data {
	case "a":
		text := inlineText(n)
		href := getAttr(n, "href")
		switch {
		case href == "":
			b.WriteString(text)
		case text == "":
			b.WriteString("[" + href + "]")
		default:
			b.WriteString(text + " [" + href + "]")
		}
	case "strong", "b":
		writeMarked(b, n, "*", "*")
	case "em", "i":
		writeMarked(b, n, "_", "_")
	case "code":
		writeMarked(b, n, "`", "`")
	case "br":
		b.WriteString("\n")
	default:
		writeInline(b, n)
	}
}

// writeMarked renders n wrapped in the given markers, writing nothing when
// the element has no text so that empty markup leaves no stray markers.
func writeMarked(b *strings.Builder, n *html.Node, opener, closer string) {
	text := inlineText(n)
	if text == "" {
		return
	}
	b.WriteString(opener + text + closer)
}

// rawText returns the concatenated text of n exactly as authored, used for
// preformatted blocks where whitespace is meaningful.
func rawText(n *html.Node) string {
	var b strings.Builder

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				b.WriteString(c.Data)
			case html.ElementNode:
				if skippedElements[c.Data] {
					continue
				}
				if c.Data == "br" {
					b.WriteString("\n")
					continue
				}
				walk(c)
			}
		}
	}
	walk(n)

	return b.String()
}

// getAttr returns the value of the named attribute, or an empty string
// when the element does not carry it.
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

// whitespaceFlattener rewrites the whitespace HTML treats as
// insignificant into plain spaces.
var whitespaceFlattener = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ")

// flattenWhitespace turns the whitespace that HTML treats as insignificant
// into plain spaces, leaving explicit line breaks to be inserted by br.
func flattenWhitespace(s string) string {
	return whitespaceFlattener.Replace(s)
}

// collapseSpaces squeezes runs of spaces into one, drops spaces around
// line breaks, and trims the result.
func collapseSpaces(s string) string {
	var b strings.Builder
	pendingSpace := false
	var last rune

	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\r' {
			pendingSpace = true
			continue
		}
		if r == '\n' {
			pendingSpace = false
			if last == 0 {
				continue
			}
			b.WriteRune('\n')
			last = '\n'
			continue
		}
		if pendingSpace {
			pendingSpace = false
			if last != 0 && last != '\n' {
				b.WriteRune(' ')
			}
		}
		b.WriteRune(r)
		last = r
	}

	return strings.TrimRight(b.String(), " \n")
}

// wordWrap wraps text to width columns, wrapping each explicit line
// separately so that br-inserted breaks survive.
func wordWrap(text string, width int) string {
	if width < minRenderWidth {
		width = minRenderWidth
	}

	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if runeLen(line)+1+runeLen(word) <= width {
				line += " " + word
				continue
			}
			lines = append(lines, line)
			line = word
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// centerText pads text on the left so it sits centered in width columns.
// Text at least as wide as the target is returned unchanged.
func centerText(text string, width int) string {
	length := runeLen(text)
	if length >= width {
		return text
	}
	return strings.Repeat(" ", (width-length)/2) + text
}

// runeLen returns the display column count of s, counting runes rather
// than bytes so that box-drawing characters measure as one column.
func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

// normalizeOutput trims trailing whitespace from every line, collapses
// runs of blank lines to one, removes leading and trailing blank lines,
// and guarantees exactly one trailing newline.
func normalizeOutput(s string) string {
	var lines []string
	blank := 0

	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			blank++
			if blank > 1 || len(lines) == 0 {
				continue
			}
			lines = append(lines, "")
			continue
		}
		blank = 0
		lines = append(lines, line)
	}

	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n") + "\n"
}

// stripTags removes every tag from src, used only as the fallback when the
// HTML parser fails.
func stripTags(src string) string {
	var b strings.Builder
	inTag := false

	for _, r := range src {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}

	return collapseSpaces(flattenWhitespace(b.String()))
}
