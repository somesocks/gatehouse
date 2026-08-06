(import
  (seq @native:seq/v1)
  (let
    ((encrypt-sequence
      (fn (encrypt-block key size previous blocks)
        (if (null? blocks)
            null
            (let
              ((plaintext (seq/head blocks)))
              (if (not (= (bytes/length plaintext) size))
                  (error/throw "cbc/encrypt requires plaintext aligned to IV length")
                  (let
                    ((ciphertext (encrypt-block key (bytes/xor previous plaintext))))
                    (if (not (= (bytes/length ciphertext) size))
                        (error/throw "cbc/encrypt block function must return an IV-length Bytes value")
                        (pair ciphertext
                              (fn ()
                                (encrypt-sequence encrypt-block key size ciphertext (seq/tail blocks)))))))))))
     (decrypt-sequence
      (fn (decrypt-block key size previous blocks)
        (if (null? blocks)
            null
            (let
              ((ciphertext (seq/head blocks)))
              (if (not (= (bytes/length ciphertext) size))
                  (error/throw "cbc/decrypt requires ciphertext aligned to IV length")
                  (let
                    ((decrypted (decrypt-block key ciphertext)))
                    (if (not (= (bytes/length decrypted) size))
                        (error/throw "cbc/decrypt block function must return an IV-length Bytes value")
                        (pair (bytes/xor previous decrypted)
                              (fn ()
                                (decrypt-sequence decrypt-block key size ciphertext (seq/tail blocks)))))))))))
     (encrypt
      (fn (encrypt-block key iv plaintext)
        (let
          ((size (bytes/length iv)))
          (if (= size 0)
              (error/throw "cbc/encrypt requires a non-empty IV")
              (seq/bytes/collect
                (encrypt-sequence encrypt-block key size iv (seq/bytes/split plaintext size)))))))
     (decrypt
      (fn (decrypt-block key iv ciphertext)
        (let
          ((size (bytes/length iv)))
          (if (= size 0)
              (error/throw "cbc/decrypt requires a non-empty IV")
              (seq/bytes/collect
                (decrypt-sequence decrypt-block key size iv (seq/bytes/split ciphertext size))))))))
    (list
      ; (cbc/encrypt encrypt-block key iv plaintext) -> Bytes
      ; Encrypts IV-aligned Bytes with CBC using a block function called as (encrypt-block key block).
      ; Example: (import (aes @native:crypto/cipher/aes/128/v1) (cbc @native:crypto/cipher/cbc/v1) (bytes/hex/encode (cbc/encrypt aes/encrypt (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c") (bytes/hex/decode "000102030405060708090a0b0c0d0e0f") (bytes/hex/decode "6bc1bee22e409f96e93d7e117393172a")))) => "7649abac8119b246cee98e9b12e9197d".
      (pair 'encrypt encrypt)
      ; (cbc/decrypt decrypt-block key iv ciphertext) -> Bytes
      ; Decrypts IV-aligned CBC Bytes with a block function called as (decrypt-block key block).
      ; Example: (import (aes @native:crypto/cipher/aes/128/v1) (cbc @native:crypto/cipher/cbc/v1) (bytes/hex/encode (cbc/decrypt aes/decrypt (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c") (bytes/hex/decode "000102030405060708090a0b0c0d0e0f") (bytes/hex/decode "7649abac8119b246cee98e9b12e9197d")))) => "6bc1bee22e409f96e93d7e117393172a".
      (pair 'decrypt decrypt))))
