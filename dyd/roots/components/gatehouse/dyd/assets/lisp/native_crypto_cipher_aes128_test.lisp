(begin
  (import
    (aes @native:crypto/cipher/aes/128/v1)
    (begin
      ; FIPS-197 AES-128 encryption vector.
      (assert
        (= (bytes/hex/encode
             (aes/encrypt
               (bytes/hex/decode "000102030405060708090a0b0c0d0e0f")
               (bytes/hex/decode "00112233445566778899aabbccddeeff")))
           "69c4e0d86a7b0430d8cdb78070b4c55a"))

      ; FIPS-197 AES-128 decryption vector.
      (assert
        (= (bytes/hex/encode
             (aes/decrypt
               (bytes/hex/decode "000102030405060708090a0b0c0d0e0f")
               (bytes/hex/decode "69c4e0d86a7b0430d8cdb78070b4c55a")))
           "00112233445566778899aabbccddeeff"))

      ; Encrypting and decrypting preserve secret taint from either input.
      (assert
        (and
          (secret?
            (aes/encrypt
              (secret/mark (bytes/hex/decode "000102030405060708090a0b0c0d0e0f"))
              (bytes/hex/decode "00112233445566778899aabbccddeeff")))
          (secret?
            (aes/decrypt
              (bytes/hex/decode "000102030405060708090a0b0c0d0e0f")
              (secret/mark (bytes/hex/decode "69c4e0d86a7b0430d8cdb78070b4c55a"))))))

      ; AES-128 validates key and block lengths.
      (assert
        (and
          (string/contains?
            (error/value
              (error/catch
                (aes/encrypt (bytes/hex/decode "000102030405060708090a0b0c0d0e")
                             (bytes/hex/decode "00112233445566778899aabbccddeeff"))))
            "16-byte key")
          (string/contains?
            (error/value
              (error/catch
                (aes/decrypt (bytes/hex/decode "000102030405060708090a0b0c0d0e0f")
                             (bytes/hex/decode "00112233445566778899aabbccddeeff00"))))
            "16-byte block")))
      null))
  null)
