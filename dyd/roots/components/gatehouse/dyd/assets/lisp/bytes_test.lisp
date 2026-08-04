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
