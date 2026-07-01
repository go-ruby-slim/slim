package slim

import "strings"

// voidTags is the HTML5 set of void elements: Slim renders them with a trailing
// " />" and never gives them a closing tag or content.
var voidTags = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
	"basefont": true, "frame": true, "keygen": true, "menuitem": true,
}

// isVoidTag reports whether tag is an HTML5 void element.
func isVoidTag(tag string) bool { return voidTags[tag] }

// blockOpeners are the Ruby keywords whose "-" line opens a block that Slim
// closes with a matching "end" after the nested children.
var blockOpeners = []string{
	"if", "unless", "while", "until", "for", "case", "begin",
	"def", "class", "module", "loop",
}

// opensBlock reports whether a "-" control statement opens a Ruby block that
// needs a trailing "end". It recognises leading block keywords and trailing
// "do"/"do |args|" forms, but not modifier "if"/"unless"/"while"/"until"
// (a statement with the keyword mid-line), nor "else"/"elsif"/"when"/"in"/
// "rescue"/"ensure" continuations (which belong to an already-open block).
func opensBlock(stmt string) bool {
	s := strings.TrimSpace(stmt)
	if s == "" {
		return false
	}
	// Continuation keywords do not open a new block.
	for _, k := range []string{"else", "elsif", "when", "in", "rescue", "ensure", "end"} {
		if s == k || strings.HasPrefix(s, k+" ") {
			return false
		}
	}
	// Trailing "do" / "do |..|" opens a block.
	if endsWithDo(s) {
		return true
	}
	// Leading block keyword, used as a statement opener.
	for _, k := range blockOpeners {
		if s == k || strings.HasPrefix(s, k+" ") || strings.HasPrefix(s, k+"(") {
			return true
		}
	}
	return false
}

// isContinuation reports whether a "-" control statement continues an already
// open block rather than opening a fresh one: the "elsif"/"else"/"when"/"in"/
// "rescue"/"ensure" branch keywords. These share the enclosing block's "end".
func isContinuation(stmt string) bool {
	s := strings.TrimSpace(stmt)
	for _, k := range []string{"elsif", "else", "when", "in", "rescue", "ensure"} {
		if s == k || strings.HasPrefix(s, k+" ") {
			return true
		}
	}
	return false
}

// endsWithDo reports whether a statement ends with a "do" or "do |block args|"
// block-opener.
func endsWithDo(s string) bool {
	s = strings.TrimRight(s, " \t")
	if strings.HasSuffix(s, "do") {
		if len(s) == 2 || s[len(s)-3] == ' ' || s[len(s)-3] == '\t' {
			return true
		}
	}
	if strings.HasSuffix(s, "|") {
		if strings.Contains(s, "do |") || strings.Contains(s, "do|") {
			return true
		}
	}
	return false
}

// rubyStrLit renders s as a Ruby double-quoted string literal.
func rubyStrLit(s string) string { return rubyDump(s) }

// interpChunk is one segment of interpolated text: a literal run or a "#{}"
// interpolation expression (escaped or not).
type interpChunk struct {
	literal   string // set for a literal run
	expr      string // Ruby expression for an interpolation (literal == "")
	isInterp  bool
	unescaped bool // "#{{ ... }}" — the doubled-brace unescaped form
}

// splitInterp splits Slim plain text into literal and interpolation chunks. Slim
// interpolates "#{expr}" (HTML-escaped), "#{{expr}}" (not escaped), and treats a
// backslash-escaped "\#{...}" as a literal "#{...}".
func splitInterp(s string) []interpChunk {
	var chunks []interpChunk
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			chunks = append(chunks, interpChunk{literal: lit.String()})
			lit.Reset()
		}
	}
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '#' && i+2 < len(s) && s[i+2] == '{' {
			// Escaped interpolation: emit a literal "#{".
			lit.WriteString("#{")
			i += 3
			continue
		}
		if s[i] == '#' && i+1 < len(s) && s[i+1] == '{' {
			unescaped := false
			exprStart := i + 2
			if exprStart < len(s) && s[exprStart] == '{' {
				unescaped = true
			}
			// Scan the balanced "#{ ... }" (or "#{{ ... }}") span.
			depth := 0
			j := i + 1 // at '{'
			for j < len(s) {
				if s[j] == '{' {
					depth++
				} else if s[j] == '}' {
					depth--
					if depth == 0 {
						j++
						break
					}
				}
				j++
			}
			// Extract inner expression (strip the outer #{ } and, for unescaped,
			// the inner doubled braces).
			inner := s[i+2 : j-1]
			if unescaped {
				inner = strings.TrimPrefix(inner, "{")
				inner = strings.TrimSuffix(inner, "}")
			}
			flush()
			chunks = append(chunks, interpChunk{expr: inner, isInterp: true, unescaped: unescaped})
			i = j
			continue
		}
		lit.WriteByte(s[i])
		i++
	}
	flush()
	return chunks
}

// hasInterp reports whether s contains a live "#{" interpolation (i.e. not one
// that is backslash-escaped away).
func hasInterp(s string) bool {
	for _, ch := range splitInterp(s) {
		if ch.isInterp {
			return true
		}
	}
	return false
}
