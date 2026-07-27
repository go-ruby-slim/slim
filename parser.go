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

	// attachTarget, when non-nil, is where indent-nested children attach for a
	// block-expansion chain ("ul: li"): the outer node links its subtree but the
	// innermost expanded tag receives the indented children, exactly as Slim
	// nests them.
	attachTarget *node

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

	// Embedded-engine (javascript:/css:/ruby:) body lines.
	engine     string
	engineBody []string

	// Comment fields.
	commentText string
	commentCond string // conditional-comment condition, e.g. "if IE", or ""
}

// attach returns the node that indent-nested children of n should be appended
// to: the innermost tag of a block-expansion chain, or n itself.
func (n *node) attach() *node {
	if n.attachTarget != nil {
		return n.attachTarget
	}
	return n
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
	kindInlineHTML           // "<..." inline (literal) HTML line
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

	// Nest by indentation using a stack. Each stack item remembers the node that
	// indent-nested children attach to (attach()), so a block-expansion chain's
	// innermost tag collects the children.
	var roots []*node
	type stackItem struct {
		indent int
		attach *node
	}
	var stack []stackItem
	for _, e := range entries {
		for len(stack) > 0 && stack[len(stack)-1].indent >= e.indent {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, e.n)
		} else {
			parent := stack[len(stack)-1].attach
			parent.children = append(parent.children, e.n)
		}
		stack = append(stack, stackItem{e.indent, e.n.attach()})
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
	case '<':
		return parseInlineHTML(content)
	case '=':
		return parseExprLine(content, lines, idx)
	case '-':
		ctrl, consumed := joinBrokenLine(content[1:], lines, idx)
		return &node{kind: kindCode, control: ctrl}, 1 + consumed
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
	return parseElement(content, indent, lines, idx)
}

// parseInlineHTML parses a line whose first character is "<": Slim treats it as
// literal inline HTML. The whole line is emitted verbatim with "#{}"
// interpolation (escaped by default); indent-nested children render after it,
// unwrapped — Slim does not auto-close inline HTML.
func parseInlineHTML(content string) (*node, int) {
	return &node{kind: kindInlineHTML, text: content}, 1
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

// parseExprLine parses a "= expr" / "== expr" output line, honouring the
// "=<" (leading space) and "=>" (trailing space) whitespace-control markers and
// "," / "\" broken-line continuation onto following lines.
func parseExprLine(content string, lines []string, idx int) (*node, int) {
	rest := content[1:]
	unescaped := false
	if strings.HasPrefix(rest, "=") {
		rest = rest[1:]
		unescaped = true
	}
	lead, trail := false, false
	for len(rest) > 0 && (rest[0] == '<' || rest[0] == '>') {
		if rest[0] == '<' {
			lead = true
		} else {
			trail = true
		}
		rest = rest[1:]
	}
	expr, consumed := joinBrokenLine(rest, lines, idx)
	tk := textEscaped
	if unescaped {
		tk = textUnescaped
	}
	return &node{kind: kindExpr, codeExpr: expr, textKind: tk, leadSpace: lead, trailSpace: trail}, 1 + consumed
}

// parseVerbatim parses a "|" (plain) or "'" (plain + trailing space) verbatim
// text block. The marker may be followed by "<"/">" whitespace-control markers
// (a leading/trailing literal space) before the text; the inline remainder plus
// any more-indented following lines form the interpolated text block.
func parseVerbatim(content string, indent int, lines []string, idx int, apos bool) (*node, int) {
	rest := content[1:]
	lead, trail := false, false
	spaces := 0
	// Match ([<>]{1,2}(?: |\z)| ?): one or two "<"/">" markers followed by a
	// space or end-of-line, otherwise a single optional leading space.
	k := 0
	for k < len(rest) && k < 2 && (rest[k] == '<' || rest[k] == '>') {
		k++
	}
	if k > 0 && (k == len(rest) || rest[k] == ' ') {
		grp := rest[:k]
		lead = strings.Contains(grp, "<")
		trail = strings.Contains(grp, ">")
		rest = rest[k:]
		if len(rest) > 0 && rest[0] == ' ' {
			spaces = 1
			rest = rest[1:]
		}
	} else if len(rest) > 0 && rest[0] == ' ' {
		spaces = 1
		rest = rest[1:]
	}
	if apos {
		trail = true
	}
	textIndent := indent + spaces + 1
	text, consumed := parseTextBlock(rest, textIndent, indent, lines, idx)
	return &node{kind: kindVerbatim, text: text, leadSpace: lead, trailSpace: trail}, 1 + consumed
}

// parseTextBlock joins firstLine (the inline remainder of the owning line) with
// every following line indented deeper than ownerIndent, reproducing Slim's
// Parser#parse_text_block exactly: subsequent lines are separated by newlines
// and keep their indentation relative to the block's base column (textIndent),
// while an empty firstLine defers the base column to the first content line.
// Blank lines inside the block become newlines; blank lines are always consumed.
// It returns the joined (still un-interpolated) text and the number of lines
// consumed after the owning line.
func parseTextBlock(firstLine string, textIndent, ownerIndent int, lines []string, idx int) (string, int) {
	var b strings.Builder
	haveBase := firstLine != ""
	if haveBase {
		b.WriteString(firstLine)
	}
	consumed := 0
	emptyLines := 0
	for j := idx + 1; j < len(lines); j++ {
		ln := lines[j]
		if strings.TrimSpace(ln) == "" {
			consumed++
			if haveBase {
				emptyLines++
			}
			continue
		}
		ci := countIndent(ln)
		if ci <= ownerIndent {
			break
		}
		if emptyLines > 0 {
			b.WriteString(strings.Repeat("\n", emptyLines))
			emptyLines = 0
		}
		consumed++
		body := ln[ci:]
		offset := 0
		if haveBase {
			offset = ci - textIndent
			if offset < 0 {
				textIndent += offset
				offset = 0
			}
			b.WriteByte('\n')
		}
		b.WriteString(strings.Repeat(" ", offset))
		b.WriteString(body)
		if !haveBase {
			textIndent = ci
			haveBase = true
		}
	}
	return b.String(), consumed
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
		// "/!" then an optional space, then an interpolated text block: nested
		// deeper lines are part of the comment text, not child elements.
		rest := content[2:]
		sp := 0
		if strings.HasPrefix(rest, " ") {
			sp = 1
			rest = rest[1:]
		}
		textIndent := indent + sp + 2
		text, consumed := parseTextBlock(rest, textIndent, indent, lines, idx)
		return &node{kind: kindHTMLComment, commentText: text}, 1 + consumed
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

// joinBrokenLine reproduces Slim's Parser#parse_broken_line: a "-" control or
// "=" output whose (stripped) text ends in "," or "\" continues onto the next
// line, which is itself stripped and appended with a newline, repeating until a
// line does not end in a continuation character. It returns the joined code and
// the number of continuation lines consumed.
func joinBrokenLine(first string, lines []string, idx int) (string, int) {
	joined := strings.TrimSpace(first)
	consumed := 0
	for endsWithBreak(joined) && idx+consumed+1 < len(lines) {
		consumed++
		joined += "\n" + strings.TrimSpace(lines[idx+consumed])
	}
	return joined, consumed
}

// endsWithBreak reports whether s ends in a line-continuation character ("," or
// a backslash).
func endsWithBreak(s string) bool {
	if s == "" {
		return false
	}
	c := s[len(s)-1]
	return c == ',' || c == '\\'
}

// fmtErr wraps a parse error with context.
func fmtErr(format string, a ...any) error { return fmt.Errorf(format, a...) }
