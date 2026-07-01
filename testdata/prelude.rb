# prelude.rb — the minimal runtime the compiled Slim source expects a host to
# provide. go-embedded-ruby/rbgo ships the production versions; this file is the
# reference used by the differential oracle test to eval our compiled source and
# compare its rendered HTML against the `slim` gem.
module Slim
  module Helpers
    # Safe wraps a value the compiled source marked HTML-safe (an "attr==expr"
    # unescaped attribute) so render_attributes skips escaping it.
    class Safe
      attr_reader :value
      def initialize(v) = (@value = v)
    end

    def self.safe(v) = Safe.new(v)

    # escape_html mirrors Temple::Utils.escape_html, the escaper Slim uses for
    # "=" output and interpolated text: the five-character HTML entity table
    # (' becomes &#39;, '/' is left untouched).
    def self.escape_html(s)
      s.to_s.gsub(/[&<>"']/, '&' => '&amp;', '<' => '&lt;', '>' => '&gt;',
                  '"' => '&quot;', "'" => '&#39;')
    end

    # render_attributes renders a merged attribute hash the way Slim does:
    # class values merged with spaces, a single id, boolean attributes (a true
    # value) emitted as name="", nil/false values omitted, every pair sorted
    # alphabetically, string values HTML-escaped (Safe-wrapped values are left
    # as-is). Trailing hashes are splat sources merged on top of the base hash.
    def self.render_attributes(base, *splats)
      merged = {}
      add = lambda do |k, v|
        k = k.to_s
        if k == 'class'
          existing = merged['class']
          val = v.is_a?(Array) ? v.join(' ') : v
          merged['class'] = [existing, val].compact.reject { |x| x == '' }.join(' ')
        elsif k == 'id'
          merged['id'] = v
        else
          merged[k] = v
        end
      end
      base.each { |k, v| add.call(k, v) }
      splats.each { |h| (h || {}).each { |k, v| add.call(k, v) } }

      out = +''
      merged.keys.sort.each do |k|
        v = merged[k]
        next if v.nil? || v == false
        if v == true
          out << %( #{k}="")
        elsif v.is_a?(Safe)
          out << %( #{k}="#{v.value}")
        else
          out << %( #{k}="#{escape_html(v)}")
        end
      end
      out
    end
  end
end
