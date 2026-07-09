# frozen_string_literal: true

# Slim template engine: compile an indentation-structured template to HTML.
require "slim"

# A one-line template rendered from the block source form (the gem's
# Slim::Template.new { source }); tag shorthand becomes an element.
puts Slim::Template.new { "h1 Hello" }.render        # => <h1>Hello</h1>
puts Slim::Template.new("p body").render             # => <p>body</p>

# An embedded `=` expression reads a bound local and is HTML-escaped by default.
puts Slim::Template.new { "p\n  = name" }.render(nil, name: "World")
puts Slim::Template.new { "p = '<b>'" }.render       # => <p>&lt;b&gt;</p>

# Nesting, `.class`/`#id` shorthand and attributes, with a dynamic `= title`.
menu = <<~SLIM
  ul#list.menu
    li.active
      a href="/" Home
    li
      a href="/about" = title
SLIM
puts Slim::Template.new { menu }.render(nil, title: "About")

# Control flow: `- if/else` emits only the taken branch (no output for `-`).
ctrl = "- if ok\n  p Yes\n- else\n  p No"
puts Slim::Template.new { ctrl }.render(nil, ok: true)   # => <p>Yes</p>

# The runtime helper the compiled source uses to escape text.
puts Slim::Helpers.escape_html(%q{<a>&"'})            # => &lt;a&gt;&amp;&quot;&#39;
