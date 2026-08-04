(begin
  (assert
    (bytes? (bytes/utf8/encode "aλ"))
    null)
  (assert
    (not (bytes? "aλ"))
    null)
  (assert
    (= (bytes/length (bytes/utf8/encode "aλ")) 3)
    null)
  (assert
    (= (bytes/length (bytes/concat)) 0)
    null)
  (assert
    (= (bytes/hex/encode
         (bytes/concat
           (bytes/hex/decode "fF")
           (bytes/hex/decode "00")))
       "ff00")
    null)
  (assert
    (= (bytes/hex/encode
         (bytes/slice (bytes/hex/decode "ff0011") 1 3))
       "0011")
    null)
  (assert
    (= (bytes/utf8/decode (bytes/utf8/encode "aλ")) "aλ")
    null)
  (assert
    (= (bytes/hex/encode (bytes/hex/decode "Ff00")) "ff00")
    null)
  (assert
    (= (bytes/base64/encode (bytes/hex/decode "ff00")) "/wA=")
    null)
  (assert
    (= (bytes/hex/encode (bytes/base64/decode "/wA=")) "ff00")
    null)
  (assert
    (= (bytes/base64url/encode (bytes/hex/decode "ff00")) "_wA=")
    null)
  (assert
    (= (bytes/hex/encode (bytes/base64url/decode "_wA=")) "ff00")
    null)
  (assert
    (= (bytes/base64/pad "/wA") "/wA=")
    null)
  (assert
    (= (bytes/base64/pad "_wA") "_wA=")
    null)
  (assert
    (= (bytes/base64/pad "TQ==") "TQ==")
    null)
  (assert
    (= (bytes/base64/unpad "/wA=") "/wA")
    null)
  (assert
    (= (bytes/base64/unpad "_wA=") "_wA")
    null)
  (assert
    (= (bytes/base64/unpad "TQ") "TQ")
    null)
  (assert
    (= (bytes/hex/decode "ff") (bytes/hex/decode "ff"))
    null)
  (assert
    (not (= (bytes/hex/decode "ff") (bytes/hex/decode "00")))
    null)
  (assert
    (= (help (bytes/hex/decode "ff")) "bytes")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (bytes/hex/decode "f")))
      "requires hexadecimal text")
    null)
  (assert
    (string/contains?
      (error/value
        (error/catch
          (bytes/utf8/decode (bytes/hex/decode "ff"))))
      "requires valid UTF-8 Bytes")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (bytes/length "text")))
      "expected Bytes")
    null)
  (assert
    (string/contains?
      (error/value
        (error/catch
          (bytes/slice (bytes/hex/decode "ff") 1 0)))
      "indices are out of range")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/decode "/wA")))
      "requires standard padded Base64 text")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/decode "_wA=")))
      "requires standard padded Base64 text")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64url/decode "/wA=")))
      "requires URL-safe padded Base64 text")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/pad "A")))
      "requires valid Base64 text")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/pad "TQ=")))
      "requires valid Base64 text")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/pad "T=Q=")))
      "requires valid Base64 text")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (bytes/base64/pad "+_==")))
      "requires valid Base64 text")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (bytes/utf8/encode 1)))
      "expected a string")
    null)
  null)
