package slim

import (
	"reflect"
	"testing"
)

// TestScanBareAttrs covers the splat variants (balanced group, unterminated
// group, bare "*expr"), the "==" unescaped form after whitespace, and the
// end-of-content break.
func TestScanBareAttrs(t *testing.T) {
	// Bare "*expr" splat.
	n := &node{}
	scanBareAttrs(n, "*attrs", 0)
	if !reflect.DeepEqual(n.splat, []string{"attrs"}) {
		t.Errorf("bare splat: %v", n.splat)
	}
	// "*{...}" balanced group splat.
	n = &node{}
	scanBareAttrs(n, "*{a: 1}", 0)
	if !reflect.DeepEqual(n.splat, []string{"{a: 1}"}) {
		t.Errorf("group splat: %v", n.splat)
	}
	// "*{" unterminated group: bails, returns index unchanged (no splat).
	n = &node{}
	if got := scanBareAttrs(n, "*{unterminated", 0); got != 0 {
		t.Errorf("unterminated group should return start index, got %d", got)
	}
	if n.splat != nil {
		t.Errorf("unterminated group should record no splat: %v", n.splat)
	}
	// "==" unescaped attribute after whitespace before the value.
	n = &node{}
	scanBareAttrs(n, `x== foo`, 0)
	if len(n.dynAttr) != 1 || !n.dynAttr[0].unescaped {
		t.Errorf("bare == unescaped: %+v", n.dynAttr)
	}
	// Trailing whitespace only after attrs -> break at end of content.
	n = &node{}
	if got := scanBareAttrs(n, `a=1   `, 0); got != 6 {
		t.Errorf("trailing ws break: got %d", got)
	}
}

// TestScanAttrsBranches covers group parsing: end-of-body after whitespace,
// splat balanced + unbalanced fallback, bare-name splat, empty name, and the
// "==" unescaped form with whitespace after the "==".
func TestScanAttrsBranches(t *testing.T) {
	// Whitespace then end of body.
	n := &node{}
	scanAttrs(n, "   ")
	if len(n.staticAttr)+len(n.dynAttr)+len(n.splat) != 0 {
		t.Error("whitespace-only body should yield nothing")
	}
	// "*{...}" balanced splat inside a group.
	n = &node{}
	scanAttrs(n, "*{a: 1}")
	if !reflect.DeepEqual(n.splat, []string{"{a: 1}"}) {
		t.Errorf("group balanced splat: %v", n.splat)
	}
	// "*expr" bare splat inside a group (no bracket after '*').
	n = &node{}
	scanAttrs(n, "*expr")
	if !reflect.DeepEqual(n.splat, []string{"expr"}) {
		t.Errorf("group bare splat: %v", n.splat)
	}
	// "*{" unbalanced inside a group: falls through to bare-token splat.
	n = &node{}
	scanAttrs(n, "*{oops")
	if !reflect.DeepEqual(n.splat, []string{"{oops"}) {
		t.Errorf("group unbalanced splat: %v", n.splat)
	}
	// Empty name (leading '='): the empty-name char is skipped.
	n = &node{}
	scanAttrs(n, "=")
	if len(n.staticAttr)+len(n.dynAttr)+len(n.splat) != 0 {
		t.Errorf("empty name should be skipped: %+v %+v", n.staticAttr, n.dynAttr)
	}
	// "name ==  value" unescaped with whitespace after ==.
	n = &node{}
	scanAttrs(n, `href==  dyn`)
	if len(n.dynAttr) != 1 || !n.dynAttr[0].unescaped || n.dynAttr[0].expr != "dyn" {
		t.Errorf("group == unescaped: %+v", n.dynAttr)
	}
}

// TestAddAttrIDArray covers the id-shorthand array-join branch (sep "_").
func TestAddAttrIDArray(t *testing.T) {
	n := &node{}
	addAttr(n, "id", `["a","b"]`, false)
	if len(n.staticAttr) != 1 || n.staticAttr[0].value != "a_b" {
		t.Errorf("id array join: %+v", n.staticAttr)
	}
}

// TestScanAttrValueBareEscape covers the bare-token quote-escape branch
// (backslash inside a quoted run of a bare token).
func TestScanAttrValueBareEscape(t *testing.T) {
	// A bare token containing an escaped char inside single quotes, ending at a
	// top-level space.
	v, _ := scanAttrValue(`a'x\'y'b end`, 0)
	if v != `a'x\'y'b` {
		t.Errorf("bare token escape in quote: %q", v)
	}
}
