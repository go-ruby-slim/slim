package slim

import (
	"strconv"
	"strings"
)

// parseAttrGroup parses a wrapped attribute group body ("(...)", "[...]" or
// "{...}"). Slim treats all three wrappers identically: space-separated
// name=value pairs where value is a quoted string, a bare literal, or a Ruby
// expression. It delegates to the shared name=value scanner.
func parseAttrGroup(n *node, body string) {
	scanAttrs(n, body)
}

// scanBareAttrs consumes leading, space-separated "name=value" attributes from
// content starting at i (bare attributes written directly on the tag line, not
// wrapped in a group). It stops at the first token that does not look like an
// attribute (no "name=" head, or a whitespace-control / self-close marker) and
// returns the index where inline content begins. A leading "*expr" splat is
// also consumed.
func scanBareAttrs(n *node, content string, i int) int {
	for {
		j := i
		for j < len(content) && (content[j] == ' ' || content[j] == '\t') {
			j++
		}
		if j >= len(content) {
			return j
		}
		if content[j] == '*' {
			// Splat: "*expr" (bare) or "*{..}"/"*(..)"/"*[..]".
			k := j + 1
			if k < len(content) && (content[k] == '{' || content[k] == '(' || content[k] == '[') {
				if _, nx, ok := scanBalanced(content, k, content[k], closeOf(content[k])); ok {
					n.splat = append(n.splat, content[k:nx])
					i = nx
					continue
				}
				return i
			}
			for k < len(content) && content[k] != ' ' && content[k] != '\t' {
				k++
			}
			n.splat = append(n.splat, content[j+1:k])
			i = k
			continue
		}
		// Read a candidate attribute name.
		start := j
		for j < len(content) && isAttrNameChar(content[j]) {
			j++
		}
		name := content[start:j]
		if name == "" || j >= len(content) || content[j] != '=' {
			return i // not an attribute; inline content begins at i
		}
		j++ // '='
		unescaped := false
		if j < len(content) && content[j] == '=' {
			unescaped = true
			j++
		}
		for j < len(content) && (content[j] == ' ' || content[j] == '\t') {
			j++
		}
		val, next := scanAttrValue(content, j)
		addAttr(n, name, val, unescaped)
		i = next
	}
}

// isAttrNameChar reports whether c is valid in a bare attribute name (allowing
// data-* / aria-* / namespaced names).
func isAttrNameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '-' || c == '_' || c == ':'
}

// scanAttrs scans space-separated Slim attributes ("name=value", "name==value"
// for an unescaped value, or a bare "name" boolean) out of body and records
// each on n as a static or dynamic attribute.
func scanAttrs(n *node, body string) {
	i := 0
	for i < len(body) {
		for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
			i++
		}
		if i >= len(body) {
			break
		}
		// Splat inside a group: "*expr" or "*{..}"/"*(..)"/"*[..]".
		if body[i] == '*' {
			i++
			if i < len(body) && (body[i] == '{' || body[i] == '(' || body[i] == '[') {
				if _, nx, ok := scanBalanced(body, i, body[i], closeOf(body[i])); ok {
					n.splat = append(n.splat, body[i:nx])
					i = nx
					continue
				}
			}
			start := i
			for i < len(body) && body[i] != ' ' && body[i] != '\t' {
				i++
			}
			n.splat = append(n.splat, strings.TrimSpace(body[start:i]))
			continue
		}
		start := i
		for i < len(body) && body[i] != '=' && body[i] != ' ' && body[i] != '\t' {
			i++
		}
		name := body[start:i]
		if name == "" {
			i++
			continue
		}
		for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
			i++
		}
		if i >= len(body) || body[i] != '=' {
			// Bare attribute name => boolean-true flag.
			n.staticAttr = append(n.staticAttr, staticAttr{name: name, isBool: true, boolVal: true})
			continue
		}
		i++ // skip first '='
		unescaped := false
		if i < len(body) && body[i] == '=' {
			unescaped = true
			i++
		}
		for i < len(body) && (body[i] == ' ' || body[i] == '\t') {
			i++
		}
		val, next := scanAttrValue(body, i)
		i = next
		addAttr(n, name, val, unescaped)
	}
}

// scanAttrValue reads one attribute value starting at body[i]: a quoted string,
// a bracketed/braced/parenthesised expression, or a bare token up to the next
// space. It returns the raw value text and the index just past it.
func scanAttrValue(body string, i int) (val string, next int) {
	if i >= len(body) {
		return "", i
	}
	switch body[i] {
	case '\'', '"':
		q := body[i]
		i++
		start := i
		for i < len(body) && body[i] != q {
			if body[i] == '\\' && i+1 < len(body) {
				i++
			}
			i++
		}
		v := body[start:i]
		if i < len(body) {
			i++
		}
		return string(q) + v + string(q), i
	case '[', '{', '(':
		open := body[i]
		if b, nx, ok := scanBalanced(body, i, open, closeOf(open)); ok {
			return string(open) + b + string(closeOf(open)), nx
		}
	}
	// Bare token, honouring nested brackets and quotes so "url(1, 2)" or
	// "a.b(c)" style expressions are captured whole.
	start := i
	depth := 0
	var quote byte
	for i < len(body) {
		c := body[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(body) {
				i += 2
				continue
			}
			if c == quote {
				quote = 0
			}
			i++
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ' ', '\t':
			if depth == 0 {
				return body[start:i], i
			}
		}
		i++
	}
	return body[start:i], i
}

// addAttr resolves a raw Slim attribute value onto the node: a compile-time
// literal becomes a staticAttr; anything else becomes a dynAttr evaluated at
// render time.
func addAttr(n *node, name, raw string, unescaped bool) {
	val := strings.TrimSpace(raw)
	// Array literal of static strings for a class attribute: ["a","b"].
	switch val {
	case "true":
		n.staticAttr = append(n.staticAttr, staticAttr{name: name, isBool: true, boolVal: true})
		return
	case "false":
		n.staticAttr = append(n.staticAttr, staticAttr{name: name, isBool: true, boolVal: false})
		return
	case "nil":
		n.staticAttr = append(n.staticAttr, staticAttr{name: name, isBool: true, boolVal: false})
		return
	}
	if lit, ok := stringLiteral(val); ok {
		n.staticAttr = append(n.staticAttr, staticAttr{name: name, value: lit})
		return
	}
	if _, err := strconv.Atoi(val); err == nil {
		n.staticAttr = append(n.staticAttr, staticAttr{name: name, value: val})
		return
	}
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		n.staticAttr = append(n.staticAttr, staticAttr{name: name, value: val})
		return
	}
	if name == "class" || name == "id" {
		if arr, ok := staticStringArray(val); ok {
			sep := " "
			if name == "id" {
				sep = "_"
			}
			n.staticAttr = append(n.staticAttr, staticAttr{name: name, value: strings.Join(arr, sep), classShorthand: name == "class"})
			return
		}
	}
	n.dynAttr = append(n.dynAttr, dynAttr{name: name, expr: val, unescaped: unescaped})
}

// stringLiteral resolves a single- or double-quoted literal with no live "#{}"
// interpolation to its unescaped string value.
func stringLiteral(val string) (string, bool) {
	if len(val) < 2 {
		return "", false
	}
	q := val[0]
	if (q != '\'' && q != '"') || val[len(val)-1] != q {
		return "", false
	}
	inner := val[1 : len(val)-1]
	if q == '"' && hasInterp(inner) {
		return "", false
	}
	return unescapeRubyStr(inner), true
}

// staticStringArray resolves a `["a", "b"]` literal of static strings.
func staticStringArray(val string) ([]string, bool) {
	if len(val) < 2 || val[0] != '[' || val[len(val)-1] != ']' {
		return nil, false
	}
	parts, err := splitTopLevel(val[1:len(val)-1], ',')
	if err != nil {
		return nil, false
	}
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		s, ok := stringLiteral(p)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// unescapeRubyStr resolves the common backslash escapes inside a quoted
// attribute string (\\ \' \" \n \t \r).
func unescapeRubyStr(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// splitTopLevel splits s on sep, ignoring separators inside quotes or nested
// brackets/braces/parens.
func splitTopLevel(s string, sep byte) ([]string, error) {
	var parts []string
	depth := 0
	var quote byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' {
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
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts, nil
}
