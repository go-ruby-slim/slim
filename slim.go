// Package slim is a pure-Go (no cgo) reimplementation of the Ruby Slim template
// engine (the `slim` gem) — the deterministic, interpreter-independent core that
// turns an indentation-structured Slim template into the Ruby source that, when
// evaluated against a set of locals, renders the same HTML the gem produces.
//
// It mirrors the go-ruby-erb / go-ruby-haml design: this package COMPILES a
// template to Ruby source; the final eval that runs any embedded Ruby (`=`
// expressions, `-` control, `#{}` interpolation, dynamic attribute values) needs
// a Ruby interpreter and is deliberately left to the consumer
// (go-embedded-ruby/rbgo).
//
// Everything that is static — element structure, `.class`/`#id` shorthand,
// literal attributes, the doctype, the `/!` HTML comment, and the
// `javascript:`/`css:` embedded engines — is resolved at compile time into
// literal HTML runs, so a template with no embedded Ruby renders with no
// interpreter at all. That deterministic core is what the ruby-free tests cover
// to 100%.
//
// Unlike Haml, Slim emits no incidental whitespace between tags: its Temple
// backend concatenates fragments with no separating newlines, so the compiled
// buffer is a tight HTML string. Void elements render as "<tag ... />" and
// interpolation in plain text is HTML-escaped by default.
package slim

import "strings"

// Options configures Compile.
type Options struct {
	// BufVar names the output-buffer local variable the compiled source appends
	// to. When empty it defaults to "_slimout".
	BufVar string

	// EscapeFn names the Ruby method the compiled source calls to HTML-escape an
	// interpolated `=` expression, an escaped attribute value, or interpolated
	// plain text. When empty it defaults to "::Slim::Helpers.escape_html", the
	// helper the host (rbgo) provides at eval time.
	EscapeFn string
}

// Compile compiles a Slim template into the Ruby source that, when eval'd with
// the template's locals in scope, builds and returns the rendered HTML string —
// matching the `slim` gem's rendered output. err is non-nil only for genuinely
// malformed templates; well-formed templates always compile (the compiled Ruby
// may still raise at eval time, which is the host's concern).
//
// The returned source assigns the buffer to BufVar, appends every fragment, and
// evaluates to the buffer as its final expression, so a host renders with
// `eval(src)` after binding the locals.
func Compile(template string, opts Options) (src string, err error) {
	bufVar := opts.BufVar
	if bufVar == "" {
		bufVar = "_slimout"
	}
	escapeFn := opts.EscapeFn
	if escapeFn == "" {
		escapeFn = "::Slim::Helpers.escape_html"
	}
	roots := parse(template)
	c := &compiler{bufVar: bufVar, escapeFn: escapeFn}
	c.src.WriteString(bufVar + " = ::String.new\n")
	c.compileTree(roots)
	if c.err != nil {
		return "", c.err
	}
	c.src.WriteString(bufVar + "\n")
	return c.src.String(), nil
}

// Render compiles template and renders it via eval, using the supplied
// evaluator. It is the compile+eval seam that mirrors go-ruby-erb.Render and
// go-ruby-haml.Render: the pure-Go side compiles to Ruby source and hands it,
// together with the locals, to eval — a pluggable function a host such as rbgo
// provides. When eval is nil, Render returns the compiled source unchanged as a
// convenience for callers that only need the source.
//
// locals maps a local-variable name to the Ruby literal source for its value
// (e.g. "name" -> `"World"`); the evaluator is expected to bind them before
// eval'ing the compiled source.
func Render(template string, locals map[string]string, eval Evaluator) (string, error) {
	src, err := Compile(template, Options{})
	if err != nil {
		return "", err
	}
	if eval == nil {
		return src, nil
	}
	return eval(src, locals)
}

// Evaluator is the Ruby-eval seam: given compiled Ruby source and the locals to
// bind, it returns the rendered string. go-embedded-ruby/rbgo supplies a real
// implementation; the pure-Go tests use a tiny deterministic shim for the
// interpreter-free cases.
type Evaluator func(rubySource string, locals map[string]string) (string, error)

// PreludeMarker documents the runtime symbols the compiled source expects the
// host to provide: the escape helper (Options.EscapeFn) and, for dynamic
// attributes, ::Slim::Helpers.render_attribute / .render_attributes. It is part
// of the public surface so the runtime contract is greppable from Go.
const PreludeMarker = "::Slim::Helpers.escape_html / .render_attribute / .render_attributes"

// TrimTrailingNewline reports whether s ends in a newline and, if so, strips a
// single one; a small helper for hosts that want to normalise the buffer. Slim
// itself emits no trailing newline, but this keeps the rendering contract
// documented and testable alongside the Haml sibling.
func TrimTrailingNewline(s string) string { return strings.TrimSuffix(s, "\n") }
