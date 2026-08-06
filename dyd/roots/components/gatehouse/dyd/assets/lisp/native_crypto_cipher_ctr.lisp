(import
  (seq @native:seq/v1)
  (let
    ((crypt-sequence
      (fn (encrypt-block key size counter blocks)
        (if (null? blocks)
            null
            (let
              ((input (seq/head blocks)))
              (if (> (bytes/length input) size)
                  (error/throw "ctr/crypt input block exceeds counter length")
                  (let
                    ((keystream (encrypt-block key counter)))
                    (if (not (= (bytes/length keystream) size))
                        (error/throw "ctr/crypt block function must return a counter-length Bytes value")
                        (pair
                          (bytes/xor input (bytes/slice keystream 0 (bytes/length input)))
                          (fn ()
                            (crypt-sequence encrypt-block key size (bytes/inc/be counter) (seq/tail blocks)))))))))))
     (crypt
      (fn (encrypt-block key counter input)
        (let
          ((size (bytes/length counter)))
          (if (= size 0)
              (error/throw "ctr/crypt requires a non-empty counter")
              (seq/bytes/collect
                (crypt-sequence encrypt-block key size counter (seq/bytes/split input size))))))))
    (list
      ; (ctr/crypt encrypt-block key counter input) -> Bytes
      ; Encrypts or decrypts Bytes with CTR using a block function called as (encrypt-block key block).
      ; Example: (import (aes @native:crypto/cipher/aes/128/v1) (ctr @native:crypto/cipher/ctr/v1) (bytes/hex/encode (ctr/crypt aes/encrypt (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c") (bytes/hex/decode "f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff") (bytes/hex/decode "6bc1bee22e409f96e93d7e117393172a")))) => "874d6191b620e3261bef6864990db6ce".
      (pair 'crypt crypt))))
