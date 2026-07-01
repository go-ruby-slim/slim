package slim

import (
	"strings"
	"testing"
)

// mustCompile compiles and fails on error.
func mustCompile(t *testing.T, tpl string) string {
	t.Helper()
	src, err := Compile(tpl, Options{})
	if err != nil {
		t.Fatalf("Compile(%q): %v", tpl, err)
	}
	return src
}

// TestElementSplatForms covers the element-level "*{...}" splat (balanced) and
// its unterminated fallback, plus an unterminated attribute group that falls
// through to inline content.
func TestElementSplatForms(t *testing.T) {
	// Balanced "*{...}" splat directly abutting the tag (parseElement's '*'
	// case, no intervening space).
	src := mustCompile(t, "p*{a: 1}")
	if !strings.Contains(src, "render_attributes({}, ({a: 1}))") {
		t.Errorf("balanced element splat: %q", src)
	}
	// Bare "*expr" splat abutting the tag (no bracket after '*').
	src = mustCompile(t, "p*attrs")
	if !strings.Contains(src, "render_attributes({}, (attrs))") {
		t.Errorf("bare element splat: %q", src)
	}
	// Unterminated "*{..." splat abutting the tag: the remainder becomes text.
	src = mustCompile(t, "p*{oops")
	if !strings.Contains(src, `{oops`) {
		t.Errorf("unterminated element splat should become text: %q", src)
	}
	// Unterminated attribute group "(..." falls through to inline content.
	src = mustCompile(t, "p(unterminated")
	if !strings.Contains(src, "(unterminated") {
		t.Errorf("unterminated group should become inline text: %q", src)
	}
}

// TestDuplicateAttrOverwrite covers the seen-map overwrite branches of
// staticAttrString for both a boolean and a value attribute repeated.
func TestDuplicateAttrOverwrite(t *testing.T) {
	// Repeated boolean attribute: later wins (overwrite branch).
	src := mustCompile(t, `input checked=true checked=true`)
	if strings.Count(src, `checked=`) != 1 {
		t.Errorf("duplicate bool should collapse: %q", src)
	}
	// Repeated value attribute: later wins.
	src = mustCompile(t, `p data-x="1" data-x="2"`)
	if !strings.Contains(src, `data-x=\"2\"`) || strings.Contains(src, `data-x=\"1\"`) {
		t.Errorf("duplicate value should keep last: %q", src)
	}
}

// TestSplitLinesTrailingNewline covers the drop-trailing-empty branch: a
// template ending in "\n" must not yield a spurious blank final line.
func TestSplitLinesTrailingNewline(t *testing.T) {
	src := mustCompile(t, "p hi\n")
	if !strings.Contains(src, "<p>hi</p>") {
		t.Errorf("trailing newline template: %q", src)
	}
}

// TestVerbatimBlockBranches covers blank-line preservation inside a verbatim
// block, trailing blank-line stripping, and the re-dedent (ci < childIndent)
// branch.
func TestVerbatimBlockBranches(t *testing.T) {
	// Blank line between two verbatim body lines is preserved (as an empty
	// element that concatenates to "a" + "" + "b" per line joins).
	src := mustCompile(t, "p\n  |\n    a\n\n    b\n")
	if !strings.Contains(src, "a") || !strings.Contains(src, "b") {
		t.Errorf("verbatim blank line: %q", src)
	}
	// A body line indented LESS than the first child line triggers re-dedent.
	src = mustCompile(t, "p\n  |\n      deep\n    shallow\n")
	if !strings.Contains(src, "deep") || !strings.Contains(src, "shallow") {
		t.Errorf("verbatim re-dedent: %q", src)
	}
	// Trailing blank lines are stripped from the verbatim body.
	src = mustCompile(t, "p\n  | text\n\n\np after")
	if !strings.Contains(src, "text") || !strings.Contains(src, "<p>after</p>") {
		t.Errorf("verbatim trailing blank strip: %q", src)
	}
}

// TestEngineBlockBranches covers the same blank-line / re-dedent / trailing
// blank-strip branches for an embedded-engine block.
func TestEngineBlockBranches(t *testing.T) {
	src := mustCompile(t, "javascript:\n  var a=1;\n\n  var b=2;\n")
	if !strings.Contains(src, "var a=1;") || !strings.Contains(src, "var b=2;") {
		t.Errorf("engine blank line: %q", src)
	}
	src = mustCompile(t, "css:\n      .deep{}\n    .shallow{}\n")
	if !strings.Contains(src, ".deep{}") || !strings.Contains(src, ".shallow{}") {
		t.Errorf("engine re-dedent: %q", src)
	}
	src = mustCompile(t, "javascript:\n  code\n\n\np after")
	if !strings.Contains(src, "code") || !strings.Contains(src, "<p>after</p>") {
		t.Errorf("engine trailing blank strip: %q", src)
	}
}

// TestSilentCommentBlankLine covers the blank-line skip inside a "/" silent
// comment's discarded subtree.
func TestSilentCommentBlankLine(t *testing.T) {
	src := mustCompile(t, "/ dead\n\n  child\np ok")
	if !strings.Contains(src, "<p>ok</p>") || strings.Contains(src, "child") {
		t.Errorf("silent comment blank line: %q", src)
	}
}
