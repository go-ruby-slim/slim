<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-slim/brand/main/social/go-ruby-slim-slim.png" alt="go-ruby-slim/slim" width="720"></p>

# slim — go-ruby-slim

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-slim.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the Ruby [Slim](https://slim-template.github.io)
template engine** (the `slim` gem) — the deterministic, interpreter-independent
core that turns an indentation-structured Slim template into the **Ruby source
that renders it**, producing the same HTML the gem produces for the same locals.

It is the Slim backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module with no dependency on the Ruby runtime.

> **What it is — and isn't.** Compiling a template to Ruby source (indentation
> nesting, `tag`/`.class`/`#id` shorthand, attributes, embedded engines,
> comments, the doctype) is fully deterministic and needs **no interpreter**, so
> it lives here as pure Go. The final `eval(compiled_src)` that runs any embedded
> Ruby (`=` expressions, `-` control, `#{}` interpolation, dynamic attribute
> values) **does** need a Ruby interpreter and stays in the consumer (e.g. rbgo)
> — this library **compiles**, the host **evaluates**. This mirrors the sibling
> [go-ruby-erb](https://github.com/go-ruby-erb/erb) and
> [go-ruby-haml](https://github.com/go-ruby-haml/haml) designs exactly.

Unlike Haml, Slim emits **no incidental whitespace between tags**: its
Temple-based backend concatenates fragments tightly, void elements render as
`<tag ... />`, and `#{}` interpolation in plain text is **HTML-escaped by
default**. Everything static — element structure, literal attributes,
`.class`/`#id` shorthand, the doctype, `/!` comments, and the `javascript:`/
`css:` embedded engines — is resolved **at compile time** into literal HTML
runs, so a template with no embedded Ruby renders with **no interpreter at all**.

## Features

Validated against the `slim` gem (5.x) on every supported platform, comparing
rendered HTML **byte-for-byte**:

- **Elements & shorthand** — `tag`, `.class`/`#id` (div default),
  `tag.c1.c2#id`, class-merge (space) between shorthand and attributes; a single
  `id` (duplicates are a compile error, matching the gem).
- **Attributes** — bare `a href="x"`, and every wrapper form `a(href="x")` /
  `a[href="x"]` / `a{href="x"}`; string/number/`true`/`false`/`nil` literals,
  boolean attributes (`checked=true` → `checked=""`, falsy omitted), bare
  booleans inside wrappers (`input(disabled)`), class-array `["a","b"]`,
  alphabetical ordering, escaped values. Dynamic values `href=url`, unescaped
  `attr==expr`, and `*splat` hashes are rendered at eval time via
  `::Slim::Helpers.render_attributes`.
- **Output** — inline text, `=` (HTML-escaped Ruby), `==` (unescaped), `-`
  (control, no output), `|` verbatim text block, `'` verbatim + trailing space,
  text interpolation `#{...}` (escaped) / `#{{...}}` (unescaped) / `\#{...}`
  (literal).
- **Control flow** — `- if/elsif/else`, `- case/when`, `- begin/rescue/ensure`
  and `- … do |x|` blocks nest correctly and share a single emitted `end`.
- **Embedded engines** — `javascript:` → `<script>…</script>`, `css:` →
  `<style>…</style>`, `ruby:` runs the body as code.
- **Comments & doctype** — code comment `/` (discarded with its subtree), HTML
  comment `/!`, conditional comment `/[if IE]`, and `doctype html/5/xml/
  transitional/strict/frameset/1.1/basic/mobile`.
- **Whitespace control** — `<` (leading space) and `>` (trailing space).
- **Void / self-closing** — the HTML5 void set (`br`, `img`, `input`, …) and the
  explicit `tag/` marker render as `<tag ... />`.

CGO-free, dependency-free, **100% test coverage**, `gofmt` + `go vet` clean, and
green across the six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le,
s390x).

### Deferred, honestly

Text-processing filters that need an external engine — `markdown:`,
`scss:`/`sass:`, `coffee:`, `less:` — are **not** compiled; the header parses and
the raw body is emitted verbatim (best-effort) rather than run through the engine.
Everything else in the feature list above matches the gem's rendered HTML
byte-for-byte in the test corpus.

## Install

```sh
go get github.com/go-ruby-slim/slim
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-slim/slim"
)

func main() {
	src, err := slim.Compile("p= name\n", slim.Options{})
	if err != nil {
		panic(err)
	}
	fmt.Println(src)
	// _slimout = ::String.new
	// _slimout << "<p>"
	// _slimout << ::Slim::Helpers.escape_html((name).to_s)
	// _slimout << "</p>"
	// _slimout
	//
	// Hand it to a Ruby interpreter with `name` in scope:
	//   name = "World"; eval(src)  ->  "<p>World</p>"

	// A fully-static template compiles to a single literal append — no interpreter needed:
	src, _ = slim.Compile(".card\n  h1 Title\n  p Body", slim.Options{})
	// _slimout << "<div class=\"card\"><h1>Title</h1><p>Body</p></div>"
}
```

Render through a pluggable evaluator (the rbgo seam):

```go
out, err := slim.Render("p= greeting",
	map[string]string{"greeting": `"hi"`},
	func(rubySrc string, locals map[string]string) (string, error) {
		// go-embedded-ruby/rbgo binds the locals and eval's rubySrc here.
		return rbgo.Eval(rubySrc, locals)
	})
```

## API

```go
type Options struct {
	BufVar   string // output-buffer var name; default "_slimout"
	EscapeFn string // Ruby escape helper for "="; default "::Slim::Helpers.escape_html"
}

// Compile returns the Ruby source that, when eval'd with the template's locals
// in scope, builds and returns the rendered HTML string, matching the `slim` gem.
func Compile(template string, opts Options) (src string, err error)

// Render = Compile + a pluggable Ruby-eval seam (nil eval returns the source).
func Render(template string, locals map[string]string, eval Evaluator) (string, error)
type Evaluator func(rubySource string, locals map[string]string) (string, error)

func HTMLEscape(s string) string // Temple::Utils.escape_html
```

### What the host (rbgo) provides at eval time

The compiled source references the runtime symbols the host supplies:

- `::Slim::Helpers.escape_html(s)` — the five-character HTML escape (overridable
  via `Options.EscapeFn`), also used for interpolated text;
- `::Slim::Helpers.render_attributes(hash, *splats)` — renders **dynamic** /
  **splat** attributes (class merge, single id, boolean handling, alphabetical
  order, escaping);
- `::Slim::Helpers.safe(v)` — marks an `attr==expr` value HTML-safe so it is not
  re-escaped.

The reference implementations used by the differential oracle live in
[`testdata/prelude.rb`](testdata/prelude.rb).

## Tests & coverage

The suite includes a **differential oracle**: a wide template corpus (elements,
shorthand, static & dynamic attributes, embedded engines, comments, control
flow, interpolation, verbatim blocks, whitespace control) is rendered both by the
system `slim` gem and by eval'ing our compiled source under the reference
prelude, comparing the HTML **byte-for-byte**. The deterministic, ruby-free
golden-source tests alone hold coverage at **100%**, so the no-ruby lanes still
pass the gate.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

The oracle tests skip themselves where `ruby`/`slim` is not available (e.g. the
qemu arch lanes), so the cross-arch builds still validate the compiler itself.

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-slim/slim authors.
