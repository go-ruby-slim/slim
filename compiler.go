package slim

import (
	"sort"
	"strings"
)

// compiler walks the node tree and accumulates Ruby source. Static output is
// coalesced into literal-string appends; dynamic output ("=", interpolation,
// "-", dynamic attributes) becomes Ruby the eval seam runs. Unlike Haml, Slim
// concatenates fragments with no separating whitespace.
type compiler struct {
	bufVar   string
	escapeFn string
	src      strings.Builder
	pending  strings.Builder // coalesced static literal text awaiting flush
	err      error           // first compile error, if any
}

// compileTree emits the full Ruby program for the parsed roots.
func (c *compiler) compileTree(roots []*node) {
	c.emitNodes(roots)
	c.flush()
}

// emitNodes emits a sibling list with control-flow awareness: an if/unless/
// case/begin control node and its elsif/else/when/in/rescue/ensure continuation
// siblings share a single trailing "end", exactly as Slim nests them.
func (c *compiler) emitNodes(nodes []*node) {
	i := 0
	for i < len(nodes) {
		n := nodes[i]
		if n.kind == kindCode && opensBlock(n.control) {
			c.emitRuby(n.control)
			c.emitNodes(n.children)
			i++
			for i < len(nodes) && nodes[i].kind == kindCode && isContinuation(nodes[i].control) {
				c.emitRuby(nodes[i].control)
				c.emitNodes(nodes[i].children)
				i++
			}
			c.emitRuby("end")
			continue
		}
		c.emit(n)
		i++
	}
}

// pushStatic appends literal HTML to the pending static run.
func (c *compiler) pushStatic(s string) { c.pending.WriteString(s) }

// flush writes any pending static run as a single buffer append.
func (c *compiler) flush() {
	if c.pending.Len() == 0 {
		return
	}
	c.src.WriteString(c.bufVar + " << " + rubyDump(c.pending.String()) + "\n")
	c.pending.Reset()
}

// emitRuby writes a raw Ruby statement line, flushing pending static output
// first so ordering is preserved.
func (c *compiler) emitRuby(stmt string) {
	c.flush()
	c.src.WriteString(stmt + "\n")
}

// emit dispatches on node kind.
func (c *compiler) emit(n *node) {
	switch n.kind {
	case kindElement:
		c.emitElement(n)
	case kindVerbatim:
		c.emitVerbatim(n)
	case kindExpr:
		c.emitExpr(n.codeExpr, n.textKind, false)
	case kindCode:
		c.emitControl(n)
	case kindHTMLComment:
		c.emitHTMLComment(n)
	case kindCondComment:
		c.emitCondComment(n)
	case kindSilent:
		// discarded
	case kindEngine:
		c.emitEngine(n)
	case kindDoctype:
		c.pushStatic(doctypeFor(n.text))
	}
}

// emitExpr emits an "=" / "==" expression. Slim escapes "=" output and leaves
// "==" unescaped; a nil value renders as the empty string via Ruby's to_s.
func (c *compiler) emitExpr(expr string, tk textKind, _ bool) {
	var appended string
	switch tk {
	case textUnescaped:
		appended = "(" + expr + ").to_s"
	default:
		appended = c.escapeFn + "((" + expr + ").to_s)"
	}
	c.emitRuby(c.bufVar + " << " + appended)
}

// emitControl emits a non-block "-" control line. Block-opening controls are
// intercepted by emitNodes, which manages the shared trailing "end".
func (c *compiler) emitControl(n *node) {
	c.emitRuby(n.control)
	c.emitNodes(n.children)
}

// emitVerbatim emits a "|" / "'" verbatim text block: literal lines joined by
// newlines, with "#{}" interpolation resolved (escaped by default). A "'" block
// appends a single trailing space.
func (c *compiler) emitVerbatim(n *node) {
	text := strings.Join(n.verbatim, "\n")
	c.emitInterpText(text)
	if n.trailSpaceV {
		c.pushStatic(" ")
	}
}

// emitInterpText emits plain text, resolving "#{expr}" (escaped) and
// "#{{expr}}" (unescaped) interpolation. Literal runs are coalesced statically.
func (c *compiler) emitInterpText(text string) {
	for _, ch := range splitInterp(text) {
		if !ch.isInterp {
			c.pushStatic(ch.literal)
			continue
		}
		if ch.unescaped {
			c.emitRuby(c.bufVar + " << (" + ch.expr + ").to_s")
		} else {
			c.emitRuby(c.bufVar + " << " + c.escapeFn + "((" + ch.expr + ").to_s)")
		}
	}
}

// emitHTMLComment emits a "/!" HTML comment "<!--text-->", including any nested
// children between the delimiters.
func (c *compiler) emitHTMLComment(n *node) {
	c.pushStatic("<!--" + n.commentText)
	c.emitNodes(n.children)
	c.pushStatic("-->")
}

// emitCondComment emits a "/[cond]" conditional comment
// "<!--[cond]>...<![endif]-->".
func (c *compiler) emitCondComment(n *node) {
	c.pushStatic("<!--[" + n.commentCond + "]>" + n.commentText)
	c.emitNodes(n.children)
	c.pushStatic("<![endif]-->")
}

// emitEngine emits a "name:" embedded engine block. :javascript and :css wrap
// the literal body in <script>/<style>; :ruby runs the body as code; unknown
// engines emit the raw body (best-effort, documented as deferred).
func (c *compiler) emitEngine(n *node) {
	body := strings.Join(n.engineBody, "\n")
	switch n.engine {
	case "javascript":
		c.pushStatic("<script>")
		c.emitInterpText(body)
		c.pushStatic("</script>")
	case "css":
		c.pushStatic("<style>")
		c.emitInterpText(body)
		c.pushStatic("</style>")
	case "ruby":
		for _, line := range n.engineBody {
			c.emitRuby(line)
		}
	default:
		// Deferred filters (markdown, coffee, scss, ...) — emit the raw body.
		c.pushStatic(body)
	}
}

// emitElement emits an element node and its subtree.
func (c *compiler) emitElement(n *node) {
	if n.leadSpace {
		c.pushStatic(" ")
	}
	void := isVoidTag(n.tag) || n.explicitVoid
	c.emitOpenTag(n, void)
	if void {
		if n.trailSpace {
			c.pushStatic(" ")
		}
		return
	}
	closeTag := "</" + n.tag + ">"

	hasChildren := len(n.children) > 0
	switch {
	case n.text == "\x00expr":
		c.emitExpr(n.codeExpr, n.textKind, false)
		c.pushStatic(closeTag)
	case n.text != "":
		c.emitInterpText(n.text)
		c.pushStatic(closeTag)
	case hasChildren:
		c.emitNodes(n.children)
		c.pushStatic(closeTag)
	default:
		c.pushStatic(closeTag)
	}
	if n.trailSpace {
		c.pushStatic(" ")
	}
}

// emitOpenTag emits "<tag ...attrs...>" (or "<tag ...attrs... />" for a void
// element), rendering static attributes at compile time and dynamic/splat
// attributes via the runtime helper.
func (c *compiler) emitOpenTag(n *node, void bool) {
	c.pushStatic("<" + n.tag)
	c.emitAttrs(n)
	if void {
		c.pushStatic(" />")
	} else {
		c.pushStatic(">")
	}
}

// emitAttrs renders the element's attributes. When every attribute is static it
// emits a literal attribute string; otherwise it merges static and dynamic
// attributes through the runtime helper so ordering/escaping match the gem.
func (c *compiler) emitAttrs(n *node) {
	if len(n.dynAttr) == 0 && len(n.splat) == 0 {
		s, err := c.staticAttrString(n)
		if err != nil {
			c.setErr(err)
			return
		}
		c.pushStatic(s)
		return
	}
	c.emitRuby(c.bufVar + " << " + c.dynAttrCall(n))
}

// staticAttrString renders fully-static attributes into an attribute string in
// Slim's canonical order: alphabetical by name, class values merged with
// spaces, a single id (duplicates are an error), boolean-true attributes as
// name="" and boolean-false/nil omitted, values HTML-escaped.
func (c *compiler) staticAttrString(n *node) (string, error) {
	var classes []string
	var ids []string
	type kv struct {
		name    string
		val     string
		boolean bool
	}
	var others []kv
	seen := map[string]int{}

	for _, sa := range n.staticAttr {
		switch sa.name {
		case "class":
			classes = append(classes, strings.Fields(sa.value)...)
		case "id":
			ids = append(ids, sa.value)
		default:
			if sa.isBool {
				if sa.boolVal {
					if idx, ok := seen[sa.name]; ok {
						others[idx] = kv{sa.name, "", true}
					} else {
						seen[sa.name] = len(others)
						others = append(others, kv{sa.name, "", true})
					}
				}
				// boolean-false / nil: omit.
				continue
			}
			if idx, ok := seen[sa.name]; ok {
				others[idx] = kv{sa.name, sa.value, false}
			} else {
				seen[sa.name] = len(others)
				others = append(others, kv{sa.name, sa.value, false})
			}
		}
	}
	if len(ids) > 1 {
		return "", fmtErr("slim: multiple id attributes specified")
	}

	type outAttr struct {
		name    string
		val     string
		boolean bool
	}
	var out []outAttr
	if len(classes) > 0 {
		out = append(out, outAttr{"class", strings.Join(classes, " "), false})
	}
	if len(ids) == 1 {
		out = append(out, outAttr{"id", ids[0], false})
	}
	for _, o := range others {
		out = append(out, outAttr{o.name, o.val, o.boolean})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].name < out[j].name })

	var b strings.Builder
	for _, o := range out {
		if o.boolean {
			b.WriteString(" " + o.name + `=""`)
		} else {
			b.WriteString(" " + o.name + `="` + attrEscape(o.val) + `"`)
		}
	}
	return b.String(), nil
}

// dynAttrCall builds the Ruby expression that renders the element's full
// attribute set (static + dynamic + splat) at eval time via the runtime helper
// the host provides. Static attributes are folded into the hash as literals so
// the helper produces the gem-identical ordering and escaping.
func (c *compiler) dynAttrCall(n *node) string {
	var pairs []string
	for _, sa := range n.staticAttr {
		key := rubyStrLit(sa.name)
		var val string
		switch {
		case sa.isBool:
			if sa.boolVal {
				val = "true"
			} else {
				val = "false"
			}
		default:
			val = rubyStrLit(sa.value)
		}
		pairs = append(pairs, key+" => "+val)
	}
	for _, da := range n.dynAttr {
		key := rubyStrLit(da.name)
		v := "(" + da.expr + ")"
		if da.unescaped {
			// "attr==expr": mark the value HTML-safe so the helper skips escaping.
			v = "::Slim::Helpers.safe(" + v + ")"
		}
		pairs = append(pairs, key+" => "+v)
	}
	hash := "{" + strings.Join(pairs, ", ") + "}"
	call := "::Slim::Helpers.render_attributes(" + hash
	for _, sp := range n.splat {
		call += ", (" + sp + ")"
	}
	call += ")"
	return call
}

// doctypeFor returns the HTML string Slim emits for a "doctype <arg>" line.
func doctypeFor(arg string) string {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "", "html", "5":
		return "<!DOCTYPE html>"
	case "xml":
		return `<?xml version="1.0" encoding="utf-8" ?>`
	case "transitional":
		return `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" ` +
			`"http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">`
	case "strict":
		return `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" ` +
			`"http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">`
	case "frameset":
		return `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Frameset//EN" ` +
			`"http://www.w3.org/TR/xhtml1/DTD/xhtml1-frameset.dtd">`
	case "1.1":
		return `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" ` +
			`"http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">`
	case "basic":
		return `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML Basic 1.1//EN" ` +
			`"http://www.w3.org/TR/xhtml-basic/xhtml-basic11.dtd">`
	case "mobile":
		return `<!DOCTYPE html PUBLIC "-//WAPFORUM//DTD XHTML Mobile 1.2//EN" ` +
			`"http://www.openmobilealliance.org/tech/DTD/xhtml-mobile12.dtd">`
	default:
		return "<!DOCTYPE html>"
	}
}

// setErr records the first compile error; Compile surfaces it after the walk.
func (c *compiler) setErr(err error) {
	if c.err == nil {
		c.err = err
	}
}
