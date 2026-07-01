package slim

import "strings"

// htmlEscapeReplacer are the characters Slim's Temple::Utils.escape_html
// replaces when escaping interpolated content (the `=` output, escaped attribute
// values, and the default `#{}` interpolation path). It maps &, <, >, " and ' to
// their entity references, matching Temple's table exactly (' becomes &#39; and
// '/' is left untouched).
var htmlEscapeReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#39;",
)

// HTMLEscape replaces the five HTML-significant characters with their entity
// references, matching Temple::Utils.escape_html exactly (note "'" becomes
// "&#39;"). It is exposed so a host embedding the compiled source can provide
// the runtime escape helper the emitted Ruby calls.
func HTMLEscape(s string) string {
	if !strings.ContainsAny(s, "&<>\"'") {
		return s
	}
	return htmlEscapeReplacer.Replace(s)
}

// attrEscape escapes a static attribute value the way Slim renders it. Slim
// escapes attribute values with the same five-character table as content, so a
// double-quoted attribute value never terminates early and entities render
// identically to the gem.
func attrEscape(s string) string {
	return HTMLEscape(s)
}
