(begin
  (import
    (crc32 @native:crypto/digest/crc32/v1)
    (let
      ((check
        (fn (name condition)
          (if condition
              null
              (error/throw (string/concat "CRC-32 " name " failed"))))))
      (begin
      ; Digest uses the standard IEEE CRC-32 test vector.
      (check "standard vector"
        (= (crc32/digest (bytes/utf8/encode "123456789"))
           3421780262))

      ; Update chains finalized CRC values, starting from zero.
      (check "incremental update"
        (= (crc32/update
             (crc32/update 0 (bytes/utf8/encode "1234"))
             (bytes/utf8/encode "56789"))
           (crc32/digest (bytes/utf8/encode "123456789"))))

      ; Empty input preserves the initial CRC.
      (check "empty update" (= (crc32/update 0 (bytes/concat)) 0))

      ; Digest operations preserve secret taint.
      (check "digest sensitivity" (secret? (crc32/digest (secret/mark (bytes/utf8/encode "abc")))))
      (check "update sensitivity" (secret? (crc32/update 0 (secret/mark (bytes/utf8/encode "abc")))))

      ; CRC values must be unsigned 32-bit integers.
      (check "integer range"
        (and
          (string/contains?
            (error/value (error/catch (crc32/update -1 (bytes/concat))))
            "from 0 through 4294967295")
          (string/contains?
            (error/value (error/catch (crc32/update 4294967296 (bytes/concat))))
            "from 0 through 4294967295")))

      ; Public functions require their documented arity and types.
      (check "argument validation"
        (and
          (string/contains?
            (error/value (error/catch (crc32/digest)))
            "requires one Bytes value")
          (string/contains?
            (error/value (error/catch (crc32/digest "abc")))
            "expected Bytes")
          (string/contains?
            (error/value (error/catch (crc32/update 0)))
            "requires an IEEE CRC-32 Integer and one Bytes value")
          (string/contains?
            (error/value (error/catch (crc32/update "zero" (bytes/concat))))
            "expected an integer")
          (string/contains?
            (error/value (error/catch (crc32/update 0 "abc")))
            "expected Bytes")))
      null)))
  null)
