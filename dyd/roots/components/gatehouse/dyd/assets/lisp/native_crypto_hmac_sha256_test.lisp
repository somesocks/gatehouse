(begin
  (import
    (hmac @native:crypto/hmac/sha256/v1)
    (begin
      ; Digest matches RFC 4231 test case 1.
      (assert
        (= (bytes/hex/encode
             (hmac/digest
               (bytes/hex/decode "0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b")
               (bytes/utf8/encode "Hi There")))
           "b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7"))

      ; Digest reports its public help text.
      (assert (= (help hmac/digest) "Returns the HMAC-SHA256 digest of key and message Bytes."))

      ; Digest preserves taint from either key or message.
      (assert
        (and
          (secret? (hmac/digest (secret/mark (bytes/utf8/encode "key")) (bytes/utf8/encode "message")))
          (secret? (hmac/digest (bytes/utf8/encode "key") (secret/mark (bytes/utf8/encode "message"))))))

      ; Digest requires exactly two Bytes arguments.
      (assert
        (and
          (string/contains?
            (error/value (error/catch (hmac/digest)))
            "requires key and message Bytes values")
          (string/contains?
            (error/value (error/catch (hmac/digest (bytes/utf8/encode "key"))))
            "requires key and message Bytes values")
          (string/contains?
            (error/value (error/catch (hmac/digest (bytes/utf8/encode "key") (bytes/utf8/encode "message") (bytes/utf8/encode "extra"))))
            "requires key and message Bytes values")))

      ; Digest rejects non-Bytes keys and messages.
      (assert
        (and
          (string/contains?
            (error/value (error/catch (hmac/digest "key" (bytes/utf8/encode "message"))))
            "expected Bytes")
          (string/contains?
            (error/value (error/catch (hmac/digest (bytes/utf8/encode "key") "message")))
            "expected Bytes")))
      null))
  null)
