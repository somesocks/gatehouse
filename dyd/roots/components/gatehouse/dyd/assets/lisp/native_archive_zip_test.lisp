(begin
  (import
    (seq @native:seq/v1)
    (zip @native:archive/zip/v1)
    (let
      ((reader
        (fn (encoded)
          (let ((source (bytes/hex/decode encoded)))
            (zip/reader
              (bytes/length source)
              (fn (offset length) (bytes/slice source offset (+ offset length)))))))
       (stored "504b03041400000000002993165d00000000000000000000000005000000656d707479504b010214031400000000002993165d000000000000000000000000050000000000000000000000800100000000656d707479504b0506000000000100010033000000230000000000")
       (deflated "504b03041400000008002993165dc2412435050000000300000005000000612e7478744b4c4a0600504b010214031400000008002993165dc24124350500000003000000050000000000000000000000800100000000612e747874504b0506000000000100010033000000280000000000")
       (descriptor "504b03041400080008000000000000000000000000000000000005000000612e7478744b4c4a0600504b0708c24124350500000003000000504b0102140014000800080000000000c24124350500000003000000050000000000000000000000000000000000612e747874504b0506000000000100010033000000380000000000")
       (encrypted "504b03041400010000000000000000000000000000000000000006000000736563726574504b0102140014000100000000000000000000000000000000000000060000000000000000000000000000000000736563726574504b0506000000000100010034000000240000000000")
       (bad-crc "504b03041400000008000000000000000000080000000300000005000000612e747874010300fcff616263504b0102140014000000080000000000000000000800000003000000050000000000000000000000000000000000612e747874504b05060000000001000100330000002b0000000000"))
      (begin
        ; Stored entries retain their name and validate empty content immediately.
        (let ((entry (seq/head (reader stored))))
          (begin
            (assert (= (zip/entry/name entry) "empty") null "stored name")
            (assert (null? (zip/entry/data entry)) null "stored data")
            (assert (null? (seq/tail (reader stored))) null "stored entry count")))

        ; Raw DEFLATE entry data is decoded lazily through the existing decoder.
        (let ((entry (seq/head (reader deflated))))
          (assert
            (= (bytes/utf8/decode (seq/bytes/collect (zip/entry/data entry))) "abc")
            null
            "deflated data"))

        ; Central-directory metadata bounds data when a local header uses a descriptor.
        (let ((entry (seq/head (reader descriptor))))
          (assert
            (= (bytes/utf8/decode (seq/bytes/collect (zip/entry/data entry))) "abc")
            null
            "data descriptor"))

        ; Reader-produced and caller-created entries share the same public type.
        (assert
          (and
            (zip/entry? (seq/head (reader deflated)))
            (zip/entry? (zip/entry "empty" (fn () null)))
            (not (zip/entry? null)))
          null
          "entry type")

        ; Encryption is rejected when the central directory entry is forced.
        (assert
          (error? (error/catch (seq/head (reader encrypted))))
          null
          "encrypted entry")

        ; A CRC mismatch is reported when the complete data sequence is forced.
        (assert
          (error? (error/catch (seq/bytes/collect (zip/entry/data (seq/head (reader bad-crc))))))
          null
          "CRC mismatch")
        null)))
  null)
