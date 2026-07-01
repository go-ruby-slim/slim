package slim

import "strings"

// parseElement parses an element line: an optional tag name, then any mix of
// ".class"/"#id" shorthand and "*splat"/attribute groups, whitespace-control
// markers, an optional "/" self-close, and inline content.
func parseElement(content string) (*node, int, error) {
	n := &node{kind: kindElement, tag: "div"}
	i := 0

	// Tag name.
	if isTagStart(content[0]) {
		start := i
		for i < len(content) && isTagChar(content[i]) {
			i++
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
					return n, 1, nil
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
			open := content[i]
			body, next, ok := scanBalanced(content, i, open, closeOf(open))
			if !ok {
				goto inline
			}
			if err := parseAttrGroup(n, body); err != nil {
				return nil, 0, err
			}
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
	// Inline content after the tag.
	rest := content[i:]
	if strings.HasPrefix(rest, " ") || rest == "" {
		rest = strings.TrimPrefix(rest, " ")
	}
	if rest != "" {
		switch {
		case strings.HasPrefix(rest, "=="):
			n.codeExpr = strings.TrimSpace(rest[2:])
			n.textKind = textUnescaped
			n.text = "\x00expr"
		case strings.HasPrefix(rest, "="):
			r := strings.TrimLeft(rest[1:], "<>")
			n.codeExpr = strings.TrimSpace(r)
			n.textKind = textEscaped
			n.text = "\x00expr"
		default:
			n.text = strings.TrimRight(rest, " ")
			n.textKind = textPlain
		}
	}
	return n, 1, nil
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
