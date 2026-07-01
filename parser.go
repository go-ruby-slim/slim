package slim

import (
	"fmt"
	"strings"
)

// node is one parsed Slim line together with its nested children. The parser
// turns the indentation-structured template into a tree of these; the compiler
// walks the tree to emit Ruby source.
type node struct {
	kind     nodeKind
	children []*node

	// Element fields (kindElement).
	tag          string
	staticAttr   []staticAttr // .class/#id shorthand + literal attributes
	dynAttr      []dynAttr    // attributes whose value is a Ruby expression
	splat        []string     // "*expr" splat attribute hashes
	leadSpace    bool         // "<" whitespace control: emit a leading space
	trailSpace   bool         // ">" whitespace control: emit a trailing space
	explicitVoid bool         // trailing "/" self-close marker

	// Content carried on the same line as an element, or a standalone
	// text/code line.
	text     string   // literal/interpolated inline or verbatim text
	textKind textKind // how text/codeExpr is emitted
	codeExpr string   // Ruby expression for "=" content
	control  string   // Ruby control statement for "-"

	// Verbatim ("|" / "'") block body lines, dedented relative to the block.
	verbatim    []string
	trailSpaceV bool // "'" verbatim adds a trailing space

	// Embedded-engine (javascript:/css:/ruby:) body lines.
	engine     string
	engineBody []string

	// Comment fields.
	commentText string
	commentCond string // conditional-comment condition, e.g. "if IE", or ""
}

type nodeKind int

const (
	kindElement     nodeKind = iota
	kindVerbatim             // "|" or "'" verbatim text block
	kindCode                 // "-" control line (no output)
	kindExpr                 // "=" / "==" expression line (output)
	kindHTMLComment          // "/!" HTML comment
	kindCondComment          // "/[cond]" conditional comment
	kindSilent               // "/" code comment (discarded)
	kindEngine               // "name:" embedded engine block
	kindDoctype              // "doctype ..." line
)

type textKind int

const (
	textPlain     textKind = iota // literal or interpolated, escaped by default
	textEscaped                   // "=" expression, HTML-escaped
	textUnescaped                 // "==" expression, not escaped
)

// staticAttr is a fully-resolved attribute known at compile time: a literal
// name/value, or a boolean flag.
type staticAttr struct {
	name           string
	value          string // resolved value (raw string, escaped at emit)
	isBool         bool   // true => boolean attribute (true/false literal)
	boolVal        bool
	classShorthand bool // came from ".x" (merged with space)
	idShorthand    bool // came from "#x"
}

// dynAttr is an attribute whose value is a Ruby expression resolved at eval time.
type dynAttr struct {
	name      string
	expr      string
	unescaped bool // "attr==expr" — value not HTML-escaped
}

// parse splits the template into lines and builds the indentation tree.
func parse(template string) []*node {
	lines := splitLines(template)

	type entry struct {
		indent int
		n      *node
	}
	var entries []entry
	i := 0
	for i < len(lines) {
		ln := lines[i]
		if strings.TrimSpace(ln) == "" {
			i++
			continue
		}
		indent := countIndent(ln)
		content := ln[indent:]
		n, consumed := parseLine(content, indent, lines, i)
		if n != nil {
			entries = append(entries, entry{indent, n})
		}
		i += consumed
	}

	// Nest by indentation using a stack.
	var roots []*node
	type stackItem struct {
		indent int
		n      *node
	}
	var stack []stackItem
	for _, e := range entries {
		for len(stack) > 0 && stack[len(stack)-1].indent >= e.indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, e.n)
		} else {
			parent := stack[len(stack)-1].n
			parent.children = append(parent.children, e.n)
		}
		stack = append(stack, stackItem{e.indent, e.n})
	}
	return roots
}

// splitLines splits on "\n", dropping a single trailing empty element so a
// template ending in "\n" does not yield a spurious blank final line.
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// countIndent returns the number of leading space/tab bytes.
func countIndent(s string) int {
	n := 0
	for n < len(s) && (s[n] == ' ' || s[n] == '\t') {
		n++
	}
	return n
}

// parseLine parses one logical line's content (indentation removed) into a node.
// lines/idx let block indicators (|, ', /!, name:) consume their nested body.
func parseLine(content string, indent int, lines []string, idx int) (n *node, consumed int) {
	switch content[0] {
	case '|':
		return parseVerbatim(content, indent, lines, idx, false)
	case '\'':
		return parseVerbatim(content, indent, lines, idx, true)
	case '=':
		return parseExprLine(content)
	case '-':
		ctrl := strings.TrimSpace(content[1:])
		return &node{kind: kindCode, control: ctrl}, 1
	case '/':
		return parseComment(content, indent, lines, idx)
	}
	// "doctype ..." keyword.
	if content == "doctype" || strings.HasPrefix(content, "doctype ") {
		arg := strings.TrimSpace(strings.TrimPrefix(content, "doctype"))
		return &node{kind: kindDoctype, text: arg}, 1
	}
	// Embedded engine "name:" with an indented body (e.g. "javascript:").
	if en, ok := embeddedEngineName(content); ok {
		return parseEngine(en, indent, lines, idx)
	}
	// Everything else is an element (a tag, or .class/#id/* shorthand implying
	// a div).
	return parseElement(content)
}

// embeddedEngineName reports whether content is a bare "name:" embedded-engine
// header (the whole line is an identifier followed by a trailing colon).
func embeddedEngineName(content string) (string, bool) {
	if !strings.HasSuffix(content, ":") {
		return "", false
	}
	name := content[:len(content)-1]
	if name == "" {
		return "", false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return "", false
		}
	}
	return name, true
}

// parseExprLine parses a "= expr" or "== expr" output line.
func parseExprLine(content string) (*node, int) {
	rest := content
	unescaped := false
	if strings.HasPrefix(rest, "==") {
		rest = rest[2:]
		unescaped = true
	} else {
		rest = rest[1:]
	}
	// Slim's "=<" / "=>" whitespace control on output lines is accepted and,
	// for a lone expression, has no visible effect on the concatenated buffer;
	// strip the markers so the expression parses.
	rest = strings.TrimLeft(rest, "<>")
	expr := strings.TrimSpace(rest)
	tk := textEscaped
	if unescaped {
		tk = textUnescaped
	}
	return &node{kind: kindExpr, codeExpr: expr, textKind: tk}, 1
}

// parseVerbatim parses a "|" (plain) or "'" (plain + trailing space) verbatim
// text block: the inline remainder plus any more-indented following lines.
func parseVerbatim(content string, indent int, lines []string, idx int, trailing bool) (*node, int) {
	n := &node{kind: kindVerbatim, trailSpaceV: trailing}
	inline := content[1:]
	inline = strings.TrimPrefix(inline, " ")
	consumed := 1
	var body []string
	if inline != "" {
		body = append(body, inline)
	}
	j := idx + 1
	childIndent := -1
	for j < len(lines) {
		ln := lines[j]
		if strings.TrimSpace(ln) == "" {
			body = append(body, "")
			consumed++
			j++
			continue
		}
		ci := countIndent(ln)
		if ci <= indent {
			break
		}
		if childIndent == -1 {
			childIndent = ci
		}
		strip := childIndent
		if ci < strip {
			strip = ci
		}
		body = append(body, ln[strip:])
		consumed++
		j++
	}
	for len(body) > 0 && body[len(body)-1] == "" {
		body = body[:len(body)-1]
		consumed--
	}
	n.verbatim = body
	return n, consumed
}

// parseEngine parses a "name:" embedded engine (javascript:/css:/ruby:) and its
// indented body block.
func parseEngine(name string, indent int, lines []string, idx int) (*node, int) {
	n := &node{kind: kindEngine, engine: name}
	consumed := 1
	j := idx + 1
	childIndent := -1
	var body []string
	for j < len(lines) {
		ln := lines[j]
		if strings.TrimSpace(ln) == "" {
			body = append(body, "")
			consumed++
			j++
			continue
		}
		ci := countIndent(ln)
		if ci <= indent {
			break
		}
		if childIndent == -1 {
			childIndent = ci
		}
		strip := childIndent
		if ci < strip {
			strip = ci
		}
		body = append(body, ln[strip:])
		consumed++
		j++
	}
	for len(body) > 0 && body[len(body)-1] == "" {
		body = body[:len(body)-1]
		consumed--
	}
	n.engineBody = body
	return n, consumed
}

// parseComment parses a "/" line: "/!"=HTML comment, "/[cond]"=conditional
// comment, or a bare "/"=silent code comment (discarded with its subtree).
func parseComment(content string, indent int, lines []string, idx int) (*node, int) {
	if strings.HasPrefix(content, "/!") {
		n := &node{kind: kindHTMLComment}
		n.commentText = strings.TrimSpace(content[2:])
		return n, 1
	}
	if strings.HasPrefix(content, "/[") {
		end := strings.Index(content, "]")
		if end >= 0 {
			n := &node{kind: kindCondComment}
			n.commentCond = content[2:end]
			n.commentText = strings.TrimSpace(content[end+1:])
			return n, 1
		}
	}
	// Bare "/" code comment: discard this line and any more-indented body.
	consumed := 1
	j := idx + 1
	for j < len(lines) {
		ln := lines[j]
		if strings.TrimSpace(ln) == "" {
			consumed++
			j++
			continue
		}
		if countIndent(ln) <= indent {
			break
		}
		consumed++
		j++
	}
	return &node{kind: kindSilent}, consumed
}

// fmtErr wraps a parse error with context.
func fmtErr(format string, a ...any) error { return fmt.Errorf(format, a...) }
