package slim

import (
	"reflect"
	"testing"
)

// TestRubyDumpEscapes exercises every escape branch of rubyDump: the C-style
// escapes, the '#'-interpolation guard, printable passthrough, and \xHH for
// non-printable / high bytes.
func TestRubyDumpEscapes(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"`, `"\""`},
		{`\`, `"\\"`},
		{"\a", `"\a"`},
		{"\b", `"\b"`},
		{"\t", `"\t"`},
		{"\n", `"\n"`},
		{"\v", `"\v"`},
		{"\f", `"\f"`},
		{"\r", `"\r"`},
		{"\x1b", `"\e"`},
		{"#{x}", `"\#{x}"`},
		{"#$g", `"\#$g"`},
		{"#@i", `"\#@i"`},
		{"#a", `"#a"`},     // '#' not followed by {,$,@ stays literal
		{"#", `"#"`},       // trailing '#'
		{"abc", `"abc"`},   // printable passthrough
		{"\x00", `"\x00"`}, // NUL -> \x00
		{"\xff", `"\xFF"`}, // high byte uppercase hex
		{"caf\xc3\xa9", `"caf\xC3\xA9"`},
	}
	for _, tc := range cases {
		if got := rubyDump(tc.in); got != tc.want {
			t.Errorf("rubyDump(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestUnescapeRubyStr covers every branch of unescapeRubyStr: no-backslash
// fast path, \n \t \r, and the default (self) case, plus a trailing backslash.
func TestUnescapeRubyStr(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{`a\nb`, "a\nb"},
		{`a\tb`, "a\tb"},
		{`a\rb`, "a\rb"},
		{`a\"b`, `a"b`},
		{`a\\b`, `a\b`},
		{`end\`, `end\`}, // trailing backslash, nothing to escape
	}
	for _, tc := range cases {
		if got := unescapeRubyStr(tc.in); got != tc.want {
			t.Errorf("unescapeRubyStr(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestStringLiteral covers the non-quoted, too-short, mismatched-quote, and
// interpolated-double-quote rejection branches.
func TestStringLiteral(t *testing.T) {
	if _, ok := stringLiteral("x"); ok {
		t.Error("single char should not be a literal")
	}
	if _, ok := stringLiteral("bare"); ok {
		t.Error("unquoted should not be a literal")
	}
	if _, ok := stringLiteral(`"a'`); ok {
		t.Error("mismatched quotes should not be a literal")
	}
	if _, ok := stringLiteral(`"a#{b}"`); ok {
		t.Error("double-quoted with interpolation is dynamic")
	}
	if v, ok := stringLiteral(`'a#{b}'`); !ok || v != `a#{b}` {
		t.Errorf("single-quoted interpolation stays literal: %q %v", v, ok)
	}
}

// TestStaticStringArray covers non-array input, empty parts skipping, and a
// non-literal element rejecting the whole array.
func TestStaticStringArray(t *testing.T) {
	if _, ok := staticStringArray("notarray"); ok {
		t.Error("non-bracket should fail")
	}
	if arr, ok := staticStringArray(`["a", , "b"]`); !ok || !reflect.DeepEqual(arr, []string{"a", "b"}) {
		t.Errorf("empty parts should be skipped: %v %v", arr, ok)
	}
	if _, ok := staticStringArray(`["a", x]`); ok {
		t.Error("non-literal element should reject")
	}
}

// TestSplitTopLevel covers quote-escaped separators and nested depth.
func TestSplitTopLevel(t *testing.T) {
	got := splitTopLevel(`"a\,b", c`, ',')
	if !reflect.DeepEqual(got, []string{`"a\,b"`, ` c`}) {
		t.Errorf("escaped comma in quote: %v", got)
	}
	got = splitTopLevel(`f(a, b), c`, ',')
	if !reflect.DeepEqual(got, []string{`f(a, b)`, ` c`}) {
		t.Errorf("nested depth: %v", got)
	}
}

// TestScanAttrValueBranches covers the empty, quoted-with-escape, and bare
// token with nested brackets/quotes/depth branches.
func TestScanAttrValueBranches(t *testing.T) {
	if v, nx := scanAttrValue("", 0); v != "" || nx != 0 {
		t.Errorf("empty: %q %d", v, nx)
	}
	// quoted with a backslash escape inside.
	if v, _ := scanAttrValue(`"a\"b"`, 0); v != `"a\"b"` {
		t.Errorf("quoted escape: %q", v)
	}
	// bare token honouring quotes and nested parens, terminating at top-level space.
	if v, nx := scanAttrValue(`url("a b") next`, 0); v != `url("a b")` || nx != 10 {
		t.Errorf("bare nested: %q %d", v, nx)
	}
	// bare token that includes an escaped quote char inside a quote.
	if v, _ := scanAttrValue(`"a\\" x`, 0); v != `"a\\"` {
		t.Errorf("bare escaped quote: %q", v)
	}
}

// TestOpensBlock covers empty, continuation keywords, trailing-do, and leading
// keyword variants.
func TestOpensBlock(t *testing.T) {
	if opensBlock("   ") {
		t.Error("empty should not open a block")
	}
	for _, k := range []string{"else", "elsif x", "when 1", "in Foo", "rescue", "ensure", "end"} {
		if opensBlock(k) {
			t.Errorf("continuation %q should not open", k)
		}
	}
	if !opensBlock("items.each do |i|") {
		t.Error("trailing do should open")
	}
	if !opensBlock("if x") || !opensBlock("case") || !opensBlock("while(y)") {
		t.Error("leading keyword should open")
	}
	if opensBlock("puts x") {
		t.Error("plain statement should not open")
	}
}

// TestIsContinuation covers the true keywords and a non-matching default.
func TestIsContinuation(t *testing.T) {
	for _, k := range []string{"elsif x", "else", "when 1", "in P", "rescue", "ensure"} {
		if !isContinuation(k) {
			t.Errorf("%q should be a continuation", k)
		}
	}
	if isContinuation("if x") {
		t.Error("if is not a continuation")
	}
}

// TestEndsWithDo covers the "do", "do |a|", "do|a|", and non-do branches.
func TestEndsWithDo(t *testing.T) {
	if !endsWithDo("x do") {
		t.Error("trailing do")
	}
	if !endsWithDo("x do |a|") {
		t.Error("do |a|")
	}
	if !endsWithDo("x do|a|") {
		t.Error("do|a|")
	}
	if endsWithDo("undo") {
		t.Error("undo is not a block opener")
	}
	if endsWithDo("x |a|") {
		t.Error("bare |a| without do is not a block opener")
	}
}

// TestHasInterp covers both the interpolation-present and absent branches.
func TestHasInterp(t *testing.T) {
	if !hasInterp("a #{b} c") {
		t.Error("should detect interpolation")
	}
	if hasInterp("plain text") {
		t.Error("no interpolation")
	}
}

// TestCloseOfDefault covers the default branch of closeOf for a non-bracket byte.
func TestCloseOfDefault(t *testing.T) {
	if closeOf('x') != 'x' {
		t.Error("closeOf default should echo the byte")
	}
}

// TestScanBalancedQuoteEscape covers the backslash-escape branch inside a quote.
func TestScanBalancedQuoteEscape(t *testing.T) {
	body, next, ok := scanBalanced(`("a\)b")x`, 0, '(', ')')
	if !ok || body != `"a\)b"` || next != 8 {
		t.Errorf("scanBalanced escape: %q %d %v", body, next, ok)
	}
}

// TestSplitLinesNoTrailingBlank ensures a template without a trailing newline
// keeps its final line (the drop-trailing-empty branch is a no-op).
func TestSplitLinesNoTrailingBlank(t *testing.T) {
	got := splitLines("a\nb")
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("splitLines no trailing blank: %v", got)
	}
}

// TestEmbeddedEngineName covers the no-colon, empty-name, and invalid-char
// rejection branches plus a valid name.
func TestEmbeddedEngineName(t *testing.T) {
	if _, ok := embeddedEngineName("nocolon"); ok {
		t.Error("no colon should fail")
	}
	if _, ok := embeddedEngineName(":"); ok {
		t.Error("empty name should fail")
	}
	if _, ok := embeddedEngineName("has space:"); ok {
		t.Error("invalid char should fail")
	}
	if name, ok := embeddedEngineName("coffee-script:"); !ok || name != "coffee-script" {
		t.Errorf("valid name: %q %v", name, ok)
	}
}
