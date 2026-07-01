package slim

import (
	"strings"
	"testing"
)

// compileGolden is a deterministic, ruby-free assertion: Compile(tpl) must equal
// the exact Ruby source we expect. These hold coverage at 100% with no
// interpreter, so the no-ruby CI lanes still pass the gate.
type compileGolden struct {
	tpl  string
	want string
}

// wrap builds the full expected source from the buffer body lines.
func wrap(body ...string) string {
	var b strings.Builder
	b.WriteString("_slimout = ::String.new\n")
	for _, l := range body {
		b.WriteString(l + "\n")
	}
	b.WriteString("_slimout\n")
	return b.String()
}

func TestCompileGolden(t *testing.T) {
	cases := []compileGolden{
		// Static elements and shorthand.
		{"p hello", wrap(`_slimout << "<p>hello</p>"`)},
		{"p", wrap(`_slimout << "<p></p>"`)},
		{".foo", wrap(`_slimout << "<div class=\"foo\"></div>"`)},
		{"#bar", wrap(`_slimout << "<div id=\"bar\"></div>"`)},
		{".foo.bar#baz", wrap(`_slimout << "<div class=\"foo bar\" id=\"baz\"></div>"`)},
		{"section#main.wide", wrap(`_slimout << "<section class=\"wide\" id=\"main\"></section>"`)},
		{"ul\n  li a\n  li b", wrap(`_slimout << "<ul><li>a</li><li>b</li></ul>"`)},
		// Static attributes and wrappers.
		{`a href="x" link`, wrap(`_slimout << "<a href=\"x\">link</a>"`)},
		{`a(href="x") link`, wrap(`_slimout << "<a href=\"x\">link</a>"`)},
		{`a[href="x"] link`, wrap(`_slimout << "<a href=\"x\">link</a>"`)},
		{`a{href="x"} link`, wrap(`_slimout << "<a href=\"x\">link</a>"`)},
		{`a href="http://x?a=b&c=d"`, wrap(`_slimout << "<a href=\"http://x?a=b&amp;c=d\"></a>"`)},
		{`a.c1 class="c2"`, wrap(`_slimout << "<a class=\"c1 c2\"></a>"`)},
		{`a class=["x","y"]`, wrap(`_slimout << "<a class=\"x y\"></a>"`)},
		{`p a="1" b="2"`, wrap(`_slimout << "<p a=\"1\" b=\"2\"></p>"`)},
		{`p attr=1 body`, wrap(`_slimout << "<p attr=\"1\">body</p>"`)},
		{`p attr=1.5`, wrap(`_slimout << "<p attr=\"1.5\"></p>"`)},
		{`p title='single'`, wrap(`_slimout << "<p title=\"single\"></p>"`)},
		// Boolean attributes.
		{`input type="checkbox" checked=true`, wrap(`_slimout << "<input checked=\"\" type=\"checkbox\" />"`)},
		{`input checked=false`, wrap(`_slimout << "<input />"`)},
		{`input(disabled type="text")`, wrap(`_slimout << "<input disabled=\"\" type=\"text\" />"`)},
		{`input[checked]`, wrap(`_slimout << "<input checked=\"\" />"`)},
		{`p attr=nil`, wrap(`_slimout << "<p></p>"`)},
		// Void and self-close.
		{"br", wrap(`_slimout << "<br />"`)},
		{`input/`, wrap(`_slimout << "<input />"`)},
		{`p/`, wrap(`_slimout << "<p />"`)},
		// Doctype.
		{"doctype", wrap(`_slimout << "<!DOCTYPE html>"`)},
		{"doctype html", wrap(`_slimout << "<!DOCTYPE html>"`)},
		{"doctype 5", wrap(`_slimout << "<!DOCTYPE html>"`)},
		{"doctype xml", wrap(`_slimout << "<?xml version=\"1.0\" encoding=\"utf-8\" ?>"`)},
		{"doctype transitional", wrap(`_slimout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.0 Transitional//EN\" \"http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd\">"`)},
		{"doctype strict", wrap(`_slimout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.0 Strict//EN\" \"http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd\">"`)},
		{"doctype frameset", wrap(`_slimout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.0 Frameset//EN\" \"http://www.w3.org/TR/xhtml1/DTD/xhtml1-frameset.dtd\">"`)},
		{"doctype 1.1", wrap(`_slimout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML 1.1//EN\" \"http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd\">"`)},
		{"doctype basic", wrap(`_slimout << "<!DOCTYPE html PUBLIC \"-//W3C//DTD XHTML Basic 1.1//EN\" \"http://www.w3.org/TR/xhtml-basic/xhtml-basic11.dtd\">"`)},
		{"doctype mobile", wrap(`_slimout << "<!DOCTYPE html PUBLIC \"-//WAPFORUM//DTD XHTML Mobile 1.2//EN\" \"http://www.openmobilealliance.org/tech/DTD/xhtml-mobile12.dtd\">"`)},
		{"doctype nonsense", wrap(`_slimout << "<!DOCTYPE html>"`)},
		// Comments.
		{"/ dead\np ok", wrap(`_slimout << "<p>ok</p>"`)},
		{"/ dead\n  child\np ok", wrap(`_slimout << "<p>ok</p>"`)},
		{"/! c", wrap(`_slimout << "<!--c-->"`)},
		{"/!\n  p in", wrap(`_slimout << "<!--<p>in</p>-->"`)},
		{"/[if IE]\n  p x", wrap(`_slimout << "<!--[if IE]><p>x</p><![endif]-->"`)},
		{"/[if IE] txt", wrap(`_slimout << "<!--[if IE]>txt<![endif]-->"`)},
		{"/unterminated cond", wrap()}, // "/u..." is a bare code comment (no "[")
		// Whitespace control.
		{"p>", wrap(`_slimout << "<p></p> "`)},
		{"p<\n  | x", wrap(`_slimout << " <p>x</p>"`)},
		{"span> spaced", wrap(`_slimout << "<span>spaced</span> "`)},
		{"br>", wrap(`_slimout << "<br /> "`)},
		{"img< ", wrap(`_slimout << " <img />"`)},
		// Output expressions.
		{"p= x", wrap(`_slimout << "<p>"`, `_slimout << ::Slim::Helpers.escape_html((x).to_s)`, `_slimout << "</p>"`)},
		{"= y", wrap(`_slimout << ::Slim::Helpers.escape_html((y).to_s)`)},
		{"p== z", wrap(`_slimout << "<p>"`, `_slimout << (z).to_s`, `_slimout << "</p>"`)},
		{"== w", wrap(`_slimout << (w).to_s`)},
		{"=< a", wrap(`_slimout << ::Slim::Helpers.escape_html((a).to_s)`)},
		{"p=< a", wrap(`_slimout << "<p>"`, `_slimout << ::Slim::Helpers.escape_html((a).to_s)`, `_slimout << "</p>"`)},
		// Verbatim.
		{"| verbatim", wrap(`_slimout << "verbatim"`)},
		{"p\n  | a\n  | b", wrap(`_slimout << "<p>ab</p>"`)},
		{"p\n  ' t", wrap(`_slimout << "<p>t </p>"`)},
		{"'", wrap(`_slimout << " "`)},
		{"|", wrap()},
		// Interpolation.
		{"p Hello #{n}", wrap(`_slimout << "<p>Hello "`, `_slimout << ::Slim::Helpers.escape_html((n).to_s)`, `_slimout << "</p>"`)},
		{"| a #{{b}} c", wrap(`_slimout << "a "`, `_slimout << (b).to_s`, `_slimout << " c"`)},
		{`| x \#{y} z`, wrap(`_slimout << "x \#{y} z"`)},
		// Embedded engines.
		{"javascript:\n  code", wrap(`_slimout << "<script>code</script>"`)},
		{"css:\n  body{}", wrap(`_slimout << "<style>body{}</style>"`)},
		{"ruby:\n  a = 1\n  b = 2", wrap(`a = 1`, `b = 2`)},
		{"markdown:\n  # H", wrap(`_slimout << "# H"`)},
		{"javascript:", wrap(`_slimout << "<script></script>"`)},
		// Control flow.
		{"- x = 5\np= x", wrap(`x = 5`, `_slimout << "<p>"`, `_slimout << ::Slim::Helpers.escape_html((x).to_s)`, `_slimout << "</p>"`)},
		{"- if f\n  p y\n- else\n  p n", wrap(`if f`, `_slimout << "<p>y</p>"`, `else`, `_slimout << "<p>n</p>"`, `end`)},
		{"- arr.each do |x|\n  p= x", wrap(`arr.each do |x|`, `_slimout << "<p>"`, `_slimout << ::Slim::Helpers.escape_html((x).to_s)`, `_slimout << "</p>"`, `end`)},
		// Dynamic and splat attributes.
		{"a href=url", wrap(`_slimout << "<a"`, `_slimout << ::Slim::Helpers.render_attributes({"href" => (url)})`, `_slimout << "></a>"`)},
		{"a href==dyn", wrap(`_slimout << "<a"`, `_slimout << ::Slim::Helpers.render_attributes({"href" => ::Slim::Helpers.safe((dyn))})`, `_slimout << "></a>"`)},
		{"p*attrs", wrap(`_slimout << "<p"`, `_slimout << ::Slim::Helpers.render_attributes({}, (attrs))`, `_slimout << "></p>"`)},
		{"a *{href: 'x'}", wrap(`_slimout << "<a"`, `_slimout << ::Slim::Helpers.render_attributes({}, ({href: 'x'}))`, `_slimout << "></a>"`)},
		{"a.c href=url", wrap(`_slimout << "<a"`, `_slimout << ::Slim::Helpers.render_attributes({"class" => "c", "href" => (url)})`, `_slimout << "></a>"`)},
		{"a checked=true href=url", wrap(`_slimout << "<a"`, `_slimout << ::Slim::Helpers.render_attributes({"checked" => true, "href" => (url)})`, `_slimout << "></a>"`)},
		{"a checked=false href=url", wrap(`_slimout << "<a"`, `_slimout << ::Slim::Helpers.render_attributes({"checked" => false, "href" => (url)})`, `_slimout << "></a>"`)},
		// Blank / whitespace-only lines are skipped.
		{"p a\n\n  \np b", wrap(`_slimout << "<p>a</p><p>b</p>"`)},
	}
	for _, tc := range cases {
		got, err := Compile(tc.tpl, Options{})
		if err != nil {
			t.Errorf("Compile(%q) error: %v", tc.tpl, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Compile(%q)\n got: %q\nwant: %q", tc.tpl, got, tc.want)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	if _, err := Compile("p#a#b", Options{}); err == nil {
		t.Error("expected error for duplicate id")
	}
	if _, err := Compile("a#x id=\"y\"", Options{}); err == nil {
		t.Error("expected error for shorthand+attr duplicate id")
	}
}

func TestCompileOptions(t *testing.T) {
	got, err := Compile("p= x", Options{BufVar: "buf", EscapeFn: "esc"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "buf = ::String.new") || !strings.Contains(got, "esc((x).to_s)") {
		t.Errorf("options not honoured: %q", got)
	}
}

func TestRender(t *testing.T) {
	// nil evaluator returns the compiled source.
	src, err := Render("p hi", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "<p>hi</p>") {
		t.Errorf("Render(nil eval) = %q", src)
	}
	// A pluggable evaluator receives the source and locals.
	out, err := Render("p= x", map[string]string{"x": `"v"`},
		func(rb string, locals map[string]string) (string, error) {
			if locals["x"] != `"v"` {
				t.Errorf("locals not passed: %v", locals)
			}
			return "RENDERED:" + rb[:0] + "ok", nil
		})
	if err != nil || out != "RENDERED:ok" {
		t.Errorf("Render eval seam = %q, %v", out, err)
	}
	// A compile error short-circuits Render.
	if _, err := Render("p#a#b", nil, nil); err == nil {
		t.Error("expected Render to surface compile error")
	}
}

func TestExportedHelpers(t *testing.T) {
	if got := HTMLEscape(`a<>&"'`); got != "a&lt;&gt;&amp;&quot;&#39;" {
		t.Errorf("HTMLEscape = %q", got)
	}
	if got := HTMLEscape("plain/slash"); got != "plain/slash" {
		t.Errorf("HTMLEscape passthrough = %q", got)
	}
	if got := TrimTrailingNewline("a\n"); got != "a" {
		t.Errorf("TrimTrailingNewline = %q", got)
	}
	if got := TrimTrailingNewline("b"); got != "b" {
		t.Errorf("TrimTrailingNewline noop = %q", got)
	}
	if PreludeMarker == "" {
		t.Error("PreludeMarker empty")
	}
}
