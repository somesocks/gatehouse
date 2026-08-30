(begin
  (import
    (pkcs7 @native:crypto/padding/pkcs7/v1)
    (begin
      ; Empty input receives a complete block of padding.
      (assert
        (= (bytes/hex/encode (pkcs7/pad 8 (bytes/concat)))
           "0808080808080808"))

      ; Partial input receives the remaining block bytes as padding.
      (assert
        (= (bytes/hex/encode (pkcs7/pad 8 (bytes/utf8/encode "YELLOW")))
           "59454c4c4f570202"))

      ; Aligned input receives a complete additional block.
      (assert
        (= (bytes/hex/encode (pkcs7/pad 8 (bytes/hex/decode "0001020304050607")))
           "00010203040506070808080808080808"))

      ; Padding and unpadding round-trip Bytes.
      (assert
        (= (bytes/hex/encode
             (pkcs7/unpad 8 (pkcs7/pad 8 (bytes/hex/decode "aabbcc"))))
           "aabbcc"))

      ; Padding and unpadding preserve secret taint.
      (assert
        (and
          (taint/secret? (pkcs7/pad 8 (taint/secret/mark (bytes/hex/decode "aa"))))
          (taint/secret?
            (pkcs7/unpad 8
              (taint/secret/mark (bytes/hex/decode "59454c4c4f570202"))))))

      ; Invalid block sizes are rejected.
      (assert
        (and
          (string/contains?
            (error/value (error/catch (pkcs7/pad 0 (bytes/concat))))
            "block size from 1 through 255")
          (string/contains?
            (error/value (error/catch (pkcs7/unpad 256 (bytes/concat))))
            "block size from 1 through 255")))

      ; Unpadding rejects empty, unaligned, oversized, and nonuniform padding.
      (assert
        (and
          (string/contains?
            (error/value (error/catch (pkcs7/unpad 4 (bytes/concat))))
            "valid PKCS#7 padding")
          (string/contains?
            (error/value (error/catch (pkcs7/unpad 4 (bytes/hex/decode "0102"))))
            "valid PKCS#7 padding")
          (string/contains?
            (error/value (error/catch (pkcs7/unpad 4 (bytes/hex/decode "00000005"))))
            "valid PKCS#7 padding")
          (string/contains?
            (error/value (error/catch (pkcs7/unpad 4 (bytes/hex/decode "41020203"))))
            "valid PKCS#7 padding")))
      null))
  null)
