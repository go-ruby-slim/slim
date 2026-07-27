package slim

import (
	"os/exec"
	"strings"
	"testing"
)

// oracleCase is a template plus the locals it needs, expressed as name -> Ruby
// literal source. Each is rendered both by the `slim` gem and by eval'ing our
// compiled source under the reference prelude; the two HTML strings must match.
type oracleCase struct {
	tpl    string
	locals map[string]string
}

var oracleCorpus = []oracleCase{
	// Elements and shorthand.
	{"p hello", nil},
	{"p", nil},
	{".foo", nil},
	{"#bar", nil},
	{".foo.bar#baz", nil},
	{"section#main.wide", nil},
	{"ul\n  li a\n  li b", nil},
	{"div.container\n  h1 Hi\n  p Body text", nil},
	{"p\n  span a\n  span b", nil},
	{"h1 Title\np Body", nil},
	{"tag-name foo", nil},
	{"li.first.active", nil},
	// Attributes (static), all wrapper forms.
	{`a href="x" link`, nil},
	{`a(href="x") link`, nil},
	{`a[href="x"] link`, nil},
	{`a{href="x"} link`, nil},
	{`a href="x" title="y" data-z="w"`, nil},
	{`input(type="text" name="q")`, nil},
	{`p.foo.bar#x hi`, nil},
	{`a.c1 class="c2"`, nil},
	{`a.c1.c2 class="c3 c4"`, nil},
	{`p class="a" class="b"`, nil},
	{`a class=["x","y"]`, nil},
	{`p title='single'`, nil},
	{`a href="http://x?a=b&c=d"`, nil},
	{`p disabled="disabled"`, nil},
	{`p attr=1 body`, nil},
	{`p a="1" b="2"`, nil},
	// Boolean attributes.
	{`input type="checkbox" checked=true`, nil},
	{`input type="checkbox" checked=false`, nil},
	{`input disabled=true`, nil},
	{`input(disabled type="text")`, nil},
	{`input[checked]`, nil},
	{`input required=true type="text"`, nil},
	{`input checked=true disabled=false`, nil},
	{`p attr=nil`, nil},
	// Void tags.
	{"br", nil}, {"img src=\"a.png\"", nil}, {"hr", nil},
	{"input", nil}, {`meta charset="utf-8"`, nil},
	{`input#x type="text"/`, nil},
	// Doctype.
	{"doctype html", nil}, {"doctype 5", nil}, {"doctype xml", nil},
	{"doctype transitional", nil}, {"doctype strict", nil},
	// Comments.
	{"/ code comment", nil},
	{"/ code comment\np after", nil},
	{"/! html comment", nil},
	{"/[if IE]\n  p x", nil},
	// Output expressions and escaping.
	{"p= 1+2", nil},
	{"= '<b>&'", nil},
	{"p== '<b>'", nil},
	{"p == '<b>'", nil},
	{"p= '<b>&'", nil},
	{"p\n  = 'inner'", nil},
	{"p\n  == '<script>'", nil},
	{"- x = 5\np= x", nil},
	{"p= nil", nil},
	{"p= [1,2,3].map{|i| i*2}.join(',')", nil},
	// Verbatim text.
	{"| verbatim text", nil},
	{"p\n  | line1\n  | line2", nil},
	{"p\n  ' trailing space", nil},
	{"p\n  |\n    multiline\n    text", nil},
	// Interpolation (escaped by default).
	{"p Hello #{name}", map[string]string{"name": `"Bob"`}},
	{"p Hello, #{name}!", map[string]string{"name": `"<b>"`}},
	{"| plain #{1+1}", nil},
	// Embedded engines.
	{"javascript:\n  var x = 1;", nil},
	{"css:\n  .a { color: red; }", nil},
	{"p\n  javascript:\n    var y=2;", nil},
	{"ruby:\n  a = 1", nil},
	// Whitespace control.
	{"p>", nil},
	{"p<\n  | x", nil},
	{"span> spaced", nil},
	// Control flow.
	{"- if flag\n  p yes\n- else\n  p no", map[string]string{"flag": "true"}},
	{"- if flag\n  p yes\n- else\n  p no", map[string]string{"flag": "false"}},
	{"ul\n  - items.each do |i|\n    li= i", map[string]string{"items": `["a","b"]`}},
	{"- arr=[1,2,3]\n- arr.each do |x|\n  p= x", nil},
	// Dynamic attributes and splat.
	{"p= name", map[string]string{"name": `"World"`}},
	{"a href=url link", map[string]string{"url": `"http://x"`}},
	{"p id=who", map[string]string{"who": `"bar"`}},
	{"a href==dyn", map[string]string{"dyn": `"<b>"`}},
	{"p*attrs", map[string]string{"attrs": `{"data-x"=>"1"}`}},
	{"a *{href: 'x', title: 'y'}", nil},
	// Inline HTML (literal line + rendered children after, no auto-close).
	{"<div>\n  p hi", nil},
	{"<p>Hello #{name}</p>", map[string]string{"name": `"<b>"`}},
	{"<br>\n<hr>", nil},
	{"<section>\n  h1 Title\n  p Body", nil},
	// Text-block continuation: inline text tag consumes deeper lines as text.
	{"p foo\n  bar\n    baz", nil},
	{"p text\n  more text", nil},
	// Multi-line verbatim blocks and offsets.
	{"p\n  | a\n     b\n    c", nil},
	{"p\n  |\n    multi\n      indented\n    line", nil},
	// Verbatim whitespace-control markers.
	{"p\n  |< x", nil},
	{"p\n  |> x", nil},
	{"p\n  |<> x", nil},
	// Multi-line "/!" HTML comment text block.
	{"/! comment\n  more\n  lines", nil},
	// Block expansion (single, chained, with children).
	{"ul: li Item", nil},
	{`ul: li: a href="x" Link`, nil},
	{"ul: li\n  span deep", nil},
	{"table: tr: td cell", nil},
	// Output whitespace control (standalone and on tags).
	{"p=> 1+1", nil},
	{"p=< 5", nil},
	{"p=<> 3", nil},
	{"=> 'x'", nil},
	{"=< 'y'", nil},
	{`p==> "<b>"`, nil},
	// Broken-line continuation.
	{"= [1,\n   2,\n   3].sum", nil},
	{"= {a: 1,\n   b: 2}.size", nil},
	{"- x = [1,\n  2].sum\np= x", nil},
	{"p= [10,\n  20].max", nil},
	// Multi-line attribute groups.
	{"a(href=\"x\"\n  title=\"y\") link", nil},
	{"a[href=\"x\"\n  data-y=\"z\"]", nil},
	{"form(action=\"/x\"\n  method=\"post\")\n  input", nil},
	// Static class shorthand merged with a dynamic class attribute.
	{"a.static class=dyn", map[string]string{"dyn": `"d"`}},
	{"a.x.y class=dyn", map[string]string{"dyn": `"z"`}},
}

func slimAvailable() bool {
	if _, err := exec.LookPath("ruby"); err != nil {
		return false
	}
	return exec.Command("ruby", "-e", "require 'slim'").Run() == nil
}

func setupLocals(locals map[string]string) string {
	var b strings.Builder
	for k, v := range locals {
		b.WriteString(k + " = " + v + "; ")
	}
	return b.String()
}

// gemRender renders tpl with the `slim` gem, binding locals.
func gemRender(t *testing.T, tpl string, locals map[string]string) string {
	t.Helper()
	script := `$stdout.binmode; require 'slim'; ` + setupLocals(locals) +
		`tpl = $stdin.read; ` +
		`print Slim::Template.new { tpl }.render(Object.new, ` +
		`binding.local_variables.reject{|n| n==:tpl}.map { |n| [n, binding.local_variable_get(n)] }.to_h)`
	cmd := exec.Command("ruby", "-e", script)
	cmd.Stdin = strings.NewReader(tpl)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gem render %q: %v\n%s", tpl, err, out)
	}
	return string(out)
}

// ourRender eval's our compiled source under the reference prelude, binding
// locals, proving the emitted Ruby renders identically to the gem.
func ourRender(t *testing.T, tpl string, locals map[string]string) string {
	t.Helper()
	src, err := Compile(tpl, Options{})
	if err != nil {
		t.Fatalf("Compile %q: %v", tpl, err)
	}
	script := `$stdout.binmode; require_relative 'testdata/prelude'; ` +
		setupLocals(locals) + `print eval($stdin.read)`
	cmd := exec.Command("ruby", "-e", script)
	cmd.Stdin = strings.NewReader(src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("eval our src %q: %v\nsrc=%s\n%s", tpl, err, src, out)
	}
	return string(out)
}

func TestDifferentialRenderAgainstGem(t *testing.T) {
	if !slimAvailable() {
		t.Skip("ruby with the slim gem not available; skipping differential oracle")
	}
	for _, tc := range oracleCorpus {
		want := gemRender(t, tc.tpl, tc.locals)
		got := ourRender(t, tc.tpl, tc.locals)
		if got != want {
			t.Errorf("render mismatch tpl=%q\n gem=%q\n our=%q", tc.tpl, want, got)
		}
	}
}
