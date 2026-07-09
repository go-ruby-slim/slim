# Examples

Runnable pure-Ruby usage of the `slim` template engine, verified under the
[rbgo](https://github.com/go-embedded-ruby/ruby) interpreter.

```sh
rbgo examples/slim_usage.rb
```

| File             | Shows                                                                       |
| ---------------- | --------------------------------------------------------------------------- |
| `slim_usage.rb`  | Compiling and rendering Slim templates: block and string source forms, `= expr` with HTML-escaping, `.class`/`#id` shorthand and attributes, bound locals, `- if/else` control flow, and `Slim::Helpers.escape_html`. |
