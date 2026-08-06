(begin
  ; UTF-8 encoding produces a Bytes value.
  (assert
    (bytes? (bytes/utf8/encode "aλ")))

  ; Strings are not Bytes values.
  (assert
    (not (bytes? "aλ")))

  ; UTF-8 byte length differs from Unicode code-point length.
  (assert
    (= (bytes/length (bytes/utf8/encode "aλ")) 3))

  ; Concatenating no values produces empty Bytes.
  (assert
    (= (bytes/length (bytes/concat)) 0))

  ; Concatenating decoded values preserves byte order.
  (assert
    (= (bytes/hex/encode
         (bytes/concat
           (bytes/hex/decode "fF")
           (bytes/hex/decode "00")))
       "ff00"))

  ; Slicing uses an inclusive start and exclusive end index.
  (assert
    (= (bytes/hex/encode
          (bytes/slice (bytes/hex/decode "ff0011") 1 3))
       "0011"))

  ; UTF-8 encoding and decoding round-trip Unicode text.
  (assert
    (= (bytes/utf8/decode (bytes/utf8/encode "aλ")) "aλ"))

  ; Hex decoding accepts upper-case input and encoding normalizes to lower case.
  (assert
    (= (bytes/hex/encode (bytes/hex/decode "Ff00")) "ff00"))

  ; Standard Base64 encoding uses the padded alphabet.
  (assert
    (= (bytes/base64/encode (bytes/hex/decode "ff00")) "/wA="))

  ; Standard Base64 decoding restores the original bytes.
  (assert
    (= (bytes/hex/encode (bytes/base64/decode "/wA=")) "ff00"))

  ; URL-safe Base64 encoding uses the URL-safe alphabet.
  (assert
    (= (bytes/base64url/encode (bytes/hex/decode "ff00")) "_wA="))

  ; URL-safe Base64 decoding restores the original bytes.
  (assert
    (= (bytes/hex/encode (bytes/base64url/decode "_wA=")) "ff00"))

  ; Padding accepts unpadded standard Base64.
  (assert
    (= (bytes/base64/pad "/wA") "/wA="))

  ; Padding accepts unpadded URL-safe Base64.
  (assert
    (= (bytes/base64/pad "_wA") "_wA="))

  ; Padding preserves already padded Base64.
  (assert
    (= (bytes/base64/pad "TQ==") "TQ=="))

  ; Unpadding removes standard Base64 padding.
  (assert
    (= (bytes/base64/unpad "/wA=") "/wA"))

  ; Unpadding removes URL-safe Base64 padding.
  (assert
    (= (bytes/base64/unpad "_wA=") "_wA"))

  ; Unpadding preserves already unpadded Base64.
  (assert
    (= (bytes/base64/unpad "TQ") "TQ"))

  ; Bytes compare structurally by byte content.
  (assert
    (= (bytes/hex/decode "ff") (bytes/hex/decode "ff")))

  ; Distinct byte sequences are not equal.
  (assert
    (not (= (bytes/hex/decode "ff") (bytes/hex/decode "00"))))

  ; Bytewise Boolean operations preserve each bit position.
  (assert
    (and
      (= (bytes/hex/encode (bytes/and (bytes/hex/decode "aaf0") (bytes/hex/decode "0f33"))) "0a30")
      (= (bytes/hex/encode (bytes/or (bytes/hex/decode "aaf0") (bytes/hex/decode "0f33"))) "aff3")
      (= (bytes/hex/encode (bytes/xor (bytes/hex/decode "aaf0") (bytes/hex/decode "0f33"))) "a5c3")
      (= (bytes/hex/encode (bytes/not (bytes/hex/decode "00ff"))) "ff00")
      (= (bytes/length (bytes/xor (bytes/concat) (bytes/concat))) 0)))

  ; Bytewise Boolean operations preserve secret taint.
  (assert
    (and
      (secret? (bytes/and (secret/mark (bytes/hex/decode "aa")) (bytes/hex/decode "0f")))
      (secret? (bytes/or (bytes/hex/decode "aa") (secret/mark (bytes/hex/decode "0f"))))
      (secret? (bytes/xor (secret/mark (bytes/hex/decode "aa")) (bytes/hex/decode "0f")))
      (secret? (bytes/not (secret/mark (bytes/hex/decode "aa"))))))

  ; Big-endian arithmetic carries and borrows from the final byte.
  (assert
    (and
      (= (bytes/hex/encode (bytes/inc/be (bytes/hex/decode "00ff"))) "0100")
      (= (bytes/hex/encode (bytes/dec/be (bytes/hex/decode "0100"))) "00ff")
      (= (bytes/hex/encode (bytes/inc/be (bytes/hex/decode "ffff"))) "0000")
      (= (bytes/hex/encode (bytes/dec/be (bytes/hex/decode "0000"))) "ffff")))

  ; Little-endian arithmetic carries and borrows from the first byte.
  (assert
    (and
      (= (bytes/hex/encode (bytes/inc/le (bytes/hex/decode "ff00"))) "0001")
      (= (bytes/hex/encode (bytes/dec/le (bytes/hex/decode "0001"))) "ff00")
      (= (bytes/hex/encode (bytes/inc/le (bytes/hex/decode "ffff"))) "0000")
      (= (bytes/hex/encode (bytes/dec/le (bytes/hex/decode "0000"))) "ffff")))

  ; Empty Bytes remain empty, and byte arithmetic preserves secret taint.
  (assert
    (and
      (= (bytes/length (bytes/inc/be (bytes/concat))) 0)
      (= (bytes/length (bytes/dec/le (bytes/concat))) 0)
      (secret? (bytes/inc/le (secret/mark (bytes/hex/decode "00"))))
      (secret? (bytes/dec/be (secret/mark (bytes/hex/decode "00"))))))

  ; Start and end padding reach the requested target length.
  (assert
    (and
      (= (bytes/hex/encode (bytes/pad/start (bytes/hex/decode "aabb") 4 0)) "0000aabb")
      (= (bytes/hex/encode (bytes/pad/end (bytes/hex/decode "aabb") 4 255)) "aabbffff")
      (= (bytes/hex/encode (bytes/pad/start (bytes/hex/decode "aabb") 2 0)) "aabb")
      (= (bytes/hex/encode (bytes/pad/end (bytes/hex/decode "aabb") 2 255)) "aabb")))

  ; Padding preserves secret taint from Bytes and padding values.
  (assert
    (and
      (secret? (bytes/pad/start (secret/mark (bytes/hex/decode "aa")) 2 0))
      (secret? (bytes/pad/end (bytes/hex/decode "aa") 2 (secret/mark 0)))))

  ; Signed big-endian codecs preserve one-byte boundaries.
  (assert
    (and
      (= (bytes/hex/encode (bytes/int/be/encode -128 1)) "80")
      (= (bytes/hex/encode (bytes/int/be/encode 127 1)) "7f")
      (= (bytes/int/be/decode (bytes/hex/decode "80")) -128)
      (= (bytes/int/be/decode (bytes/hex/decode "7f")) 127)))

  ; Signed little-endian codecs preserve two-byte boundaries.
  (assert
    (and
      (= (bytes/hex/encode (bytes/int/le/encode -32768 2)) "0080")
      (= (bytes/hex/encode (bytes/int/le/encode 32767 2)) "ff7f")
      (= (bytes/int/le/decode (bytes/hex/decode "0080")) -32768)
      (= (bytes/int/le/decode (bytes/hex/decode "ff7f")) 32767)))

  ; Unsigned codecs retain byte order and the full non-negative range of short widths.
  (assert
    (and
      (= (bytes/hex/encode (bytes/uint/be/encode 258 2)) "0102")
      (= (bytes/hex/encode (bytes/uint/le/encode 258 2)) "0201")
      (= (bytes/uint/be/decode (bytes/hex/decode "ffff")) 65535)
      (= (bytes/uint/le/decode (bytes/hex/decode "ffff")) 65535)))

  ; Integer codecs preserve secret taint.
  (assert
    (and
      (secret? (bytes/int/be/encode (secret/mark -2) 2))
      (secret? (bytes/uint/le/decode (secret/mark (bytes/hex/decode "0201"))))))

  ; Bytes values report their built-in help text.
  (assert
    (= (help (bytes/hex/decode "ff")) "bytes"))

  ; Hex decoding rejects odd-length text.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/hex/decode "f")))
      "requires hexadecimal text"))

  ; UTF-8 decoding rejects invalid byte sequences.
  (assert
    (string/contains?
      (error/value
         (error/catch
           (bytes/utf8/decode (bytes/hex/decode "ff"))))
       "requires valid UTF-8 Bytes"))

  ; Bytes length rejects non-Bytes input.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/length "text")))
      "expected Bytes"))

  ; Slicing rejects an end index before its start index.
  (assert
    (string/contains?
      (error/value
         (error/catch
           (bytes/slice (bytes/hex/decode "ff") 1 0)))
       "indices are out of range"))

  ; Binary Boolean operations require equal-length Bytes values.
  (assert
    (and
      (string/contains?
        (error/value (error/catch (bytes/and (bytes/hex/decode "aa") (bytes/hex/decode "aabb"))))
        "equal-length Bytes")
      (string/contains?
        (error/value (error/catch (bytes/or "aa" (bytes/hex/decode "aa"))))
        "expected Bytes")
      (string/contains?
        (error/value (error/catch (bytes/xor (bytes/hex/decode "aa"))))
        "requires two Bytes")))

  ; Complement requires one Bytes value.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/not "aa")))
      "expected Bytes"))

  ; Byte arithmetic requires exactly one Bytes value.
  (assert
    (and
      (string/contains?
        (error/value (error/catch (bytes/inc/be "00")))
        "expected Bytes")
      (string/contains?
        (error/value (error/catch (bytes/dec/le (bytes/hex/decode "00") (bytes/hex/decode "00"))))
        "requires one Bytes")))

  ; Padding rejects shortening and invalid padding bytes.
  (assert
    (and
      (string/contains?
        (error/value (error/catch (bytes/pad/start (bytes/hex/decode "aabb") 1 0)))
        "shorter than Bytes")
      (string/contains?
        (error/value (error/catch (bytes/pad/end (bytes/hex/decode "aa") 2 -1)))
        "byte from 0 through 255")
      (string/contains?
        (error/value (error/catch (bytes/pad/start (bytes/hex/decode "aa") 2 256)))
        "byte from 0 through 255")))

  ; Signed codecs reject values that do not fit their fixed width.
  (assert
    (and
      (string/contains?
        (error/value (error/catch (bytes/int/be/encode 128 1)))
        "does not fit length")
      (string/contains?
        (error/value (error/catch (bytes/int/le/encode -129 1)))
        "does not fit length")))

  ; Unsigned codecs reject negative and overflowing values.
  (assert
    (and
      (string/contains?
        (error/value (error/catch (bytes/uint/be/encode -1 1)))
        "does not fit length")
      (string/contains?
        (error/value (error/catch (bytes/uint/le/encode 256 1)))
        "does not fit length")))

  ; Codecs require widths and encoded values from one through eight bytes.
  (assert
    (and
      (string/contains?
        (error/value (error/catch (bytes/int/be/encode 0 0)))
        "length from 1 through 8")
      (string/contains?
        (error/value (error/catch (bytes/uint/le/encode 0 9)))
        "length from 1 through 8")
      (string/contains?
        (error/value (error/catch (bytes/int/be/decode (bytes/concat))))
        "from 1 through 8 bytes")
      (string/contains?
        (error/value (error/catch (bytes/uint/le/decode (bytes/hex/decode "000000000000000000"))))
        "from 1 through 8 bytes")))

  ; Unsigned decoding rejects values that exceed the Integer range.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/uint/be/decode (bytes/hex/decode "ffffffffffffffff"))))
      "exceeds Integer range"))

  ; Standard Base64 decoding requires padding.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/decode "/wA")))
      "requires standard padded Base64 text"))

  ; Standard Base64 decoding rejects the URL-safe alphabet.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/decode "_wA=")))
      "requires standard padded Base64 text"))

  ; URL-safe Base64 decoding rejects the standard alphabet.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64url/decode "/wA=")))
      "requires URL-safe padded Base64 text"))

  ; Padding rejects Base64 text with an impossible length.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/pad "A")))
      "requires valid Base64 text"))

  ; Padding rejects incomplete padding.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/pad "TQ=")))
      "requires valid Base64 text"))

  ; Padding rejects padding in the middle of Base64 text.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/pad "T=Q=")))
      "requires valid Base64 text"))

  ; Padding rejects Base64 text that mixes alphabets.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/pad "+_==")))
      "requires valid Base64 text"))

  ; UTF-8 encoding requires a string input.
  (assert
    (string/contains?
      (error/value (error/catch (bytes/utf8/encode 1)))
      "expected a string"))
  null)
