(begin
  (import
    (hmac @native:crypto/mac/hmac/sha256/v1)
    (begin
      ; Digest matches RFC 4231 test case 1.
      (assert
        (= (bytes/hex/encode
             (hmac/digest
               (bytes/hex/decode "0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b")
               (bytes/utf8/encode "Hi There")))
           "b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7"))

      ; Digest reports its signature and runnable example.
      (assert
        (and
          (string/contains? (help hmac/digest) "(hmac/digest key message) -> Bytes")
          (string/contains? (help hmac/digest) "Example: (import (hmac @native:crypto/mac/hmac/sha256/v1)")))

      ; Verification accepts a matching tag and rejects a mismatched tag.
      (assert
        (and
          (hmac/verify
            (bytes/utf8/encode "key")
            (bytes/utf8/encode "abc")
            (bytes/hex/decode "9c196e32dc0175f86f4b1cb89289d6619de6bee699e4c378e68309ed97a1a6ab"))
          (not
            (hmac/verify
              (bytes/utf8/encode "key")
              (bytes/utf8/encode "abc")
              (bytes/hex/decode "9d196e32dc0175f86f4b1cb89289d6619de6bee699e4c378e68309ed97a1a6ab")))))

      ; Verification reports its signature and runnable example.
      (assert
        (and
          (string/contains? (help hmac/verify) "(hmac/verify key message tag) -> Boolean")
          (string/contains? (help hmac/verify) "Example: (import (hmac @native:crypto/mac/hmac/sha256/v1)")))

      ; Digest preserves taint from either key or message.
      (assert
        (and
          (secret? (hmac/digest (secret/mark (bytes/utf8/encode "key")) (bytes/utf8/encode "message")))
          (secret? (hmac/digest (bytes/utf8/encode "key") (secret/mark (bytes/utf8/encode "message"))))
          (secret?
            (hmac/verify
              (bytes/utf8/encode "key")
              (bytes/utf8/encode "message")
              (secret/mark (bytes/hex/decode "9c196e32dc0175f86f4b1cb89289d6619de6bee699e4c378e68309ed97a1a6ab"))))))

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
             "requires key and message Bytes values")
           (string/contains?
             (error/value (error/catch (hmac/verify (bytes/utf8/encode "key") (bytes/utf8/encode "message"))))
             "requires key, message, and tag Bytes values")))

      ; Digest rejects non-Bytes keys and messages.
      (assert
        (and
          (string/contains?
            (error/value (error/catch (hmac/digest "key" (bytes/utf8/encode "message"))))
            "expected Bytes")
          (string/contains?
            (error/value (error/catch (hmac/digest (bytes/utf8/encode "key") "message")))
            "expected Bytes")
          (string/contains?
            (error/value (error/catch (hmac/verify (bytes/utf8/encode "key") (bytes/utf8/encode "message") "tag")))
            "expected Bytes")))
      null))
  null)
