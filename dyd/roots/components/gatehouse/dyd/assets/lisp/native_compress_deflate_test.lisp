(begin
  (import
    (deflate @native:compress/deflate/v1)
    (seq @native:seq/v1)
    (begin
      (let ((decode
             (fn pages
               (seq/bytes/collect (deflate/decode (fn/apply seq/from pages))))))
        (begin
        ; RFC 1951 section 3.2.4: a final stored block containing "abc".
        (assert
          (= (bytes/utf8/decode (decode (bytes/hex/decode "010300fcff616263"))) "abc")
          null
          "stored block")

        ; Google Wuffs artificial-deflate/backref-crosses-blocks.deflate.
        ; MIT OR Apache-2.0; expected plaintext: "abcdcdc".
        (assert
          (= (bytes/utf8/decode
               (decode
                 (bytes/hex/decode "000400")
                 (bytes/hex/decode "fbff6162")
                 (bytes/hex/decode "6364034200")))
             "abcdcdc")
          null
          "fixed Huffman back-reference across blocks")

        ; Google Wuffs artificial-deflate/degenerate-huffman.deflate.
        ; MIT OR Apache-2.0; this exercises a dynamic code-length tree.
        (assert
          (= (bytes/utf8/decode
               (decode
                 (bytes/base64/decode "BMAhAQAAAICgt1b+N4kbhmMTAAAAhkG3te3/NyWD6S4A")))
             "fooABCDABC")
          null
          "dynamic Huffman")

        ; Reserved BTYPE=11 is rejected when the sequence is forced.
        (assert
          (error? (error/catch (decode (bytes/hex/decode "07"))))
          null
          "reserved block type")
        null))))
  null)
