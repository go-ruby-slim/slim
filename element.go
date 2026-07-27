package slim

import "strings"

// parseElement parses an element line: an optional tag name, then any mix of
// ".class"/"#id" shorthand and "*splat"/attribute groups, whitespace-control
// markers, an optional "/" self-close, block-expansion (":"), and inline
// content (which, when plain text, consumes a nested text block). lines/idx give
// the parser context needed to consume that text block; consumed counts the
// owning line plus any lines the text block or an expanded child pulls in.
func parseElement(content string, indent int, lines []string, idx int) (*node, int) {
	n := &node{kind: kindElement, tag: "div"}
	i := 0
	lineOff := 0 // physical lines an unclosed attribute group pulled in

	// Tag name. A trailing ":" is never part of the tag (Slim tags end in a word
	// character); it is left for block-expansion detection below.
	if isTagStart(content[0]) {
		start := i
		for i < len(content) && isTagChar(content[i]) {
			i++
		}
		for i > start && content[i-1] == ':' {
			i--
		}
		n.tag = content[start:i]
	}

	// Shorthand ".class" / "#id" and "*splat" and attribute groups, repeated.
	for i < len(content) {
		switch content[i] {
		case '.', '#':
			marker := content[i]
			i++
			start := i
			for i < len(content) && isNameChar(content[i]) {
				i++
			}
			name := content[start:i]
			if marker == '.' {
				n.staticAttr = append(n.staticAttr, staticAttr{name: "class", value: name, classShorthand: true})
			} else {
				n.staticAttr = append(n.staticAttr, staticAttr{name: "id", value: name, idShorthand: true})
			}
		case '*':
			// Splat: "*expr" or "*{ hash }". Consume up to a whitespace or an
			// attribute-group boundary.
			i++
			if i < len(content) && (content[i] == '{' || content[i] == '(' || content[i] == '[') {
				open := content[i]
				_, next, ok := scanBalanced(content, i, open, closeOf(open))
				if !ok {
					n.text = strings.TrimLeft(content[i:], " ")
					return n, 1 + lineOff
				}
				n.splat = append(n.splat, content[i:next])
				i = next
			} else {
				start := i
				for i < len(content) && content[i] != ' ' && content[i] != '\t' {
					i++
				}
				n.splat = append(n.splat, content[start:i])
			}
		case '(', '[', '{':
			// Attribute group. Slim lets a group span multiple lines: if the
			// delimiter is not closed on this line, pull in following lines until
			// it balances (mirroring Parser#parse_attributes calling next_line).
			open := content[i]
			body, next, ok := scanBalanced(content, i, open, closeOf(open))
			if !ok {
				orig, off := content, lineOff
				for !ok && idx+lineOff+1 < len(lines) {
					lineOff++
					content += "\n" + lines[idx+lineOff]
					body, next, ok = scanBalanced(content, i, open, closeOf(open))
				}
				if !ok {
					// Never balanced: not an attribute group. Give back the borrowed
					// lines and treat the delimiter as the start of inline text.
					content, lineOff = orig, off
					goto inline
				}
			}
			parseAttrGroup(n, body)
			i = next
		default:
			goto bareattrs
		}
	}

bareattrs:
	// Bare, space-separated "name=value" attributes on the tag line, e.g.
	// `a href="x" title="y" body`. Scanning stops at the first token that is not
	// a name=value attribute; the remainder is inline content.
	i = scanBareAttrs(n, content, i)

	// Whitespace-control markers "<" / ">" and self-close "/", in any order,
	// immediately following the tag/shorthand/attribute groups.
	for i < len(content) && (content[i] == '<' || content[i] == '>' || content[i] == '/') {
		switch content[i] {
		case '<':
			n.leadSpace = true
		case '>':
			n.trailSpace = true
		case '/':
			n.explicitVoid = true
		}
		i++
	}

inline:
	rest := content[i:]

	// Block expansion: "tag: child" nests child inside tag on one line. The
	// remainder after the ":" is parsed as a fresh element line whose subtree
	// hangs off this tag; the innermost expanded tag receives indent-children.
	if t := strings.TrimLeft(rest, " \t"); strings.HasPrefix(t, ":") {
		sub := strings.TrimLeft(t[1:], " \t")
		if sub != "" {
			child, consumed := parseElement(sub, indent, lines, idx+lineOff)
			n.children = append(n.children, child)
			n.attachTarget = child.attach()
			return n, lineOff + consumed
		}
	}

	// Inline content after the tag.
	if strings.HasPrefix(rest, " ") || rest == "" {
		rest = strings.TrimPrefix(rest, " ")
	}
	if rest == "" {
		return n, 1 + lineOff
	}
	switch {
	case strings.HasPrefix(rest, "=="):
		expr, l, tr := parseInlineOutput(rest[2:], n.leadSpace, n.trailSpace)
		expr, consumed := joinBrokenLine(expr, lines, idx+lineOff)
		n.codeExpr, n.leadSpace, n.trailSpace = expr, l, tr
		n.textKind = textUnescaped
		n.text = "\x00expr"
		return n, 1 + lineOff + consumed
	case strings.HasPrefix(rest, "="):
		expr, l, tr := parseInlineOutput(rest[1:], n.leadSpace, n.trailSpace)
		expr, consumed := joinBrokenLine(expr, lines, idx+lineOff)
		n.codeExpr, n.leadSpace, n.trailSpace = expr, l, tr
		n.textKind = textEscaped
		n.text = "\x00expr"
		return n, 1 + lineOff + consumed
	default:
		// Inline plain text starts a text block that consumes any deeper-indented
		// following lines (they are text, not child elements). The base column is
		// where the text begins on the original line: indent + (i+1) accounts for
		// the consumed prefix and the single space before the text.
		textIndent := indent + i + 1
		text, consumed := parseTextBlock(rest, textIndent, indent, lines, idx+lineOff)
		n.text = text
		n.textKind = textPlain
		return n, 1 + lineOff + consumed
	}
}

// parseInlineOutput strips the "=<"/"=>" whitespace-control markers from an
// inline "= expr" / "== expr" tag output and returns the expression plus the
// (possibly newly set) leading/trailing-space flags.
func parseInlineOutput(rest string, lead, trail bool) (expr string, l, t bool) {
	for len(rest) > 0 && (rest[0] == '<' || rest[0] == '>') {
		if rest[0] == '<' {
			lead = true
		} else {
			trail = true
		}
		rest = rest[1:]
	}
	return strings.TrimSpace(rest), lead, trail
}

// closeOf returns the closing delimiter for an opening bracket.
func closeOf(open byte) byte {
	switch open {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	}
	return open
}

// scanBalanced returns the body between a balanced open/close pair starting at
// content[start] (which must equal open), respecting single/double-quoted Ruby
// strings. next is the index just past the closing delimiter.
func scanBalanced(content string, start int, open, close byte) (body string, next int, ok bool) {
	depth := 0
	var quote byte
	for i := start; i < len(content); i++ {
		c := content[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(content) {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return content[start+1 : i], i + 1, true
			}
		}
	}
	return "", 0, false
}

// isTagStart reports whether c can begin an explicit tag name.
func isTagStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

// isTagChar reports whether c is valid inside a tag name.
func isTagChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == ':' || c == '_'
}

// isNameChar reports whether c is valid in a .class / #id shorthand name.
func isNameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}
