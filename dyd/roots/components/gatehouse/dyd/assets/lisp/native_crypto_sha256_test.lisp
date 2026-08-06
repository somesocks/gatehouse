(begin
  (import
    (sha256 @native:crypto/sha256/v1)
    (begin
      ; Digest returns the known SHA-256 value for abc.
      (assert
        (= (bytes/hex/encode (sha256/digest (bytes/utf8/encode "abc")))
           "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"))

      ; Digest reports its signature and runnable example.
      (assert
        (and
          (string/contains? (help sha256/digest) "(sha256/digest bytes) -> Bytes")
          (string/contains? (help sha256/digest) "Example: (import (sha256 @native:crypto/sha256/v1)")))

      ; Digest preserves secret taint.
      (assert (secret? (sha256/digest (secret/mark (bytes/utf8/encode "abc")))))

      ; Digest requires exactly one Bytes argument.
      (assert
        (and
          (string/contains?
            (error/value (error/catch (sha256/digest)))
            "requires one Bytes value")
          (string/contains?
            (error/value (error/catch (sha256/digest (bytes/utf8/encode "a") (bytes/utf8/encode "b"))))
            "requires one Bytes value")))

      ; Digest rejects non-Bytes input.
      (assert
        (string/contains?
          (error/value (error/catch (sha256/digest "abc")))
          "expected Bytes"))

      ; Unknown native modules return a public import error.
      (assert
        (string/contains?
          (error/value (error/catch (import (unknown @native:crypto/unknown/v1) null)))
          "unknown module"))
      null))
  null)
