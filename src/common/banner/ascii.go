package banner

import (
	"strings"
	"unicode"
)

// glyphRows is the height of every block-font glyph.
const glyphRows = 5

// glyphWidth is the width of every block-font glyph, excluding the
// single-column gap rendered between letters.
const glyphWidth = 5

// maxArtColumns is the widest banner the standard 80-column terminal can
// hold. Names that would exceed it fall back to plain text.
const maxArtColumns = 78

// blockFont maps a lowercase rune onto its 5x5 block glyph. Only the
// characters that can legally appear in an application name are
// present; anything else forces the plain-text fallback.
var blockFont = map[rune][glyphRows]string{
	'a': {" ### ", "#   #", "#####", "#   #", "#   #"},
	'b': {"#### ", "#   #", "#### ", "#   #", "#### "},
	'c': {" ####", "#    ", "#    ", "#    ", " ####"},
	'd': {"#### ", "#   #", "#   #", "#   #", "#### "},
	'e': {"#####", "#    ", "#### ", "#    ", "#####"},
	'f': {"#####", "#    ", "#### ", "#    ", "#    "},
	'g': {" ####", "#    ", "#  ##", "#   #", " ### "},
	'h': {"#   #", "#   #", "#####", "#   #", "#   #"},
	'i': {"#####", "  #  ", "  #  ", "  #  ", "#####"},
	'j': {"    #", "    #", "    #", "#   #", " ### "},
	'k': {"#   #", "#  # ", "###  ", "#  # ", "#   #"},
	'l': {"#    ", "#    ", "#    ", "#    ", "#####"},
	'm': {"#   #", "## ##", "# # #", "#   #", "#   #"},
	'n': {"#   #", "##  #", "# # #", "#  ##", "#   #"},
	'o': {" ### ", "#   #", "#   #", "#   #", " ### "},
	'p': {"#### ", "#   #", "#### ", "#    ", "#    "},
	'q': {" ### ", "#   #", "# # #", "#  # ", " ## #"},
	'r': {"#### ", "#   #", "#### ", "#  # ", "#   #"},
	's': {" ####", "#    ", " ### ", "    #", "#### "},
	't': {"#####", "  #  ", "  #  ", "  #  ", "  #  "},
	'u': {"#   #", "#   #", "#   #", "#   #", " ### "},
	'v': {"#   #", "#   #", "#   #", " # # ", "  #  "},
	'w': {"#   #", "#   #", "# # #", "## ##", "#   #"},
	'x': {"#   #", " # # ", "  #  ", " # # ", "#   #"},
	'y': {"#   #", " # # ", "  #  ", "  #  ", "  #  "},
	'z': {"#####", "   # ", "  #  ", " #   ", "#####"},
	'0': {" ### ", "#  ##", "# # #", "##  #", " ### "},
	'1': {"  #  ", " ##  ", "  #  ", "  #  ", "#####"},
	'2': {" ### ", "#   #", "   # ", "  #  ", "#####"},
	'3': {"#### ", "    #", " ### ", "    #", "#### "},
	'4': {"#   #", "#   #", "#####", "    #", "    #"},
	'5': {"#####", "#    ", "#### ", "    #", "#### "},
	'6': {" ### ", "#    ", "#### ", "#   #", " ### "},
	'7': {"#####", "    #", "   # ", "  #  ", "  #  "},
	'8': {" ### ", "#   #", " ### ", "#   #", " ### "},
	'9': {" ### ", "#   #", " ####", "    #", " ### "},
	'-': {"     ", "     ", "#####", "     ", "     "},
	'_': {"     ", "     ", "     ", "     ", "#####"},
	'.': {"     ", "     ", "     ", "     ", "  #  "},
	' ': {"     ", "     ", "     ", "     ", "     "},
}

// ASCIIArt renders an application name as block letters. White-labeled
// names are arbitrary user input, so any name that is too wide or that
// contains a character the block font does not cover degrades to the
// plain uppercase name rather than producing broken art.
func ASCIIArt(appName string) string {
	name := strings.TrimSpace(appName)
	if name == "" {
		return ""
	}
	runes := []rune(strings.ToLower(name))
	if len(runes)*(glyphWidth+1) > maxArtColumns {
		return plainArt(name)
	}
	glyphs := make([][glyphRows]string, 0, len(runes))
	for _, r := range runes {
		g, ok := blockFont[r]
		if !ok {
			return plainArt(name)
		}
		glyphs = append(glyphs, g)
	}
	rows := make([]string, glyphRows)
	for i := range rows {
		var b strings.Builder
		for j, g := range glyphs {
			if j > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(g[i])
		}
		rows[i] = strings.TrimRight(b.String(), " ")
	}
	return strings.Join(rows, "\n")
}

// plainArt is the fallback rendering: the name in uppercase with any
// control characters removed so a white-label value cannot inject
// escape sequences into the console.
func plainArt(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
