(begin
  (import
    (aes @native:crypto/cipher/aes/128/v1)
    (cbc @native:crypto/cipher/cbc/v1)
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

      ; CBC preserves secret taint from keys and ciphertext.
      (assert
        (and
          (secret?
            (cbc/encrypt
              aes/encrypt
              (secret/mark (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c"))
              (bytes/hex/decode "000102030405060708090a0b0c0d0e0f")
              (bytes/hex/decode "6bc1bee22e409f96e93d7e117393172a")))
          (secret?
            (cbc/decrypt
              aes/decrypt
              (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c")
              (bytes/hex/decode "000102030405060708090a0b0c0d0e0f")
              (secret/mark (bytes/hex/decode "7649abac8119b246cee98e9b12e9197d"))))))

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
