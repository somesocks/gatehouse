(begin
  (import
    (aes @native:crypto/cipher/aes/128/v1)
    (cbc @native:crypto/cipher/cbc/v1)
    (hmac @native:crypto/mac/hmac/sha256/v1)
    (pkcs7 @native:crypto/padding/pkcs7/v1)
    (begin
      ; NIST SP 800-38A AES-CBC encryption vector.
      (assert
        (= (bytes/hex/encode
             (cbc/encrypt
               aes/encrypt
               (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c")
               (bytes/hex/decode "000102030405060708090a0b0c0d0e0f")
               (bytes/hex/decode "6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411e5fbc1191a0a52eff69f2445df4f9b17ad2b417be66c3710")))
           "7649abac8119b246cee98e9b12e9197d5086cb9b507219ee95db113a917678b273bed6b8e3c1743b7116e69e222295163ff1caa1681fac09120eca307586e1a7"))

      ; NIST SP 800-38A AES-CBC decryption vector.
      (assert
        (= (bytes/hex/encode
             (cbc/decrypt
               aes/decrypt
               (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c")
               (bytes/hex/decode "000102030405060708090a0b0c0d0e0f")
               (bytes/hex/decode "7649abac8119b246cee98e9b12e9197d5086cb9b507219ee95db113a917678b273bed6b8e3c1743b7116e69e222295163ff1caa1681fac09120eca307586e1a7")))
           "6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411e5fbc1191a0a52eff69f2445df4f9b17ad2b417be66c3710"))

      ; AES-CBC composes with PKCS#7 padding for arbitrary-length plaintext.
      (let
        ((key (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c"))
         (iv (bytes/hex/decode "000102030405060708090a0b0c0d0e0f"))
         (plaintext (bytes/utf8/encode "CBC composition")))
        (assert
          (= (bytes/utf8/decode
               (pkcs7/unpad 16
                 (cbc/decrypt aes/decrypt key iv
                   (cbc/encrypt aes/encrypt key iv
                     (pkcs7/pad 16 plaintext)))))
             "CBC composition")))

       ; Encrypt-then-MAC verifies the ciphertext before decryption and unpadding.
       (let
         ((encryption-key (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c"))
          (authentication-key (bytes/utf8/encode "authentication key"))
          (iv (bytes/hex/decode "000102030405060708090a0b0c0d0e0f"))
          (plaintext (bytes/utf8/encode "Authenticated CBC")))
         (let
           ((ciphertext
             (cbc/encrypt aes/encrypt encryption-key iv (pkcs7/pad 16 plaintext))))
           (let
             ((tag (hmac/digest authentication-key ciphertext)))
             (assert
               (and
                 (=
                   (if (hmac/verify authentication-key ciphertext tag)
                       (bytes/utf8/decode
                         (pkcs7/unpad 16
                           (cbc/decrypt aes/decrypt encryption-key iv ciphertext)))
                       "rejected")
                   "Authenticated CBC")
                 (=
                   (if (hmac/verify authentication-key ciphertext (bytes/concat))
                       (bytes/utf8/decode
                         (pkcs7/unpad 16
                           (cbc/decrypt aes/decrypt encryption-key iv ciphertext)))
                       "rejected")
                   "rejected"))))))

       ; CBC preserves secret taint from keys and ciphertext.
      (assert
        (and
          (taint/secret?
            (cbc/encrypt
              aes/encrypt
              (taint/secret/mark (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c"))
              (bytes/hex/decode "000102030405060708090a0b0c0d0e0f")
              (bytes/hex/decode "6bc1bee22e409f96e93d7e117393172a")))
          (taint/secret?
            (cbc/decrypt
              aes/decrypt
              (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c")
              (bytes/hex/decode "000102030405060708090a0b0c0d0e0f")
              (taint/secret/mark (bytes/hex/decode "7649abac8119b246cee98e9b12e9197d"))))))

      ; CBC validates IVs, block alignment, and block function output lengths.
      (assert
        (and
          (string/contains?
            (error/value
              (error/catch
                (cbc/encrypt aes/encrypt (bytes/hex/decode "000102030405060708090a0b0c0d0e0f") (bytes/concat) (bytes/hex/decode "00"))))
            "non-empty IV")
          (string/contains?
            (error/value
              (error/catch
                (cbc/encrypt aes/encrypt (bytes/hex/decode "000102030405060708090a0b0c0d0e0f") (bytes/hex/decode "00010203") (bytes/hex/decode "000102"))))
            "plaintext aligned")
          (string/contains?
            (error/value
              (error/catch
                (cbc/decrypt aes/decrypt (bytes/hex/decode "000102030405060708090a0b0c0d0e0f") (bytes/hex/decode "00010203") (bytes/hex/decode "000102"))))
            "ciphertext aligned")
          (string/contains?
            (error/value
              (error/catch
                (cbc/encrypt (fn (key block) (bytes/concat)) (bytes/hex/decode "00") (bytes/hex/decode "00010203") (bytes/hex/decode "00010203"))))
            "block function must return")))
      null))
  null)
