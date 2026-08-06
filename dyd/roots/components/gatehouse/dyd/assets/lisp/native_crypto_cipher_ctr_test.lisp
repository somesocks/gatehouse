(begin
  (import
    (aes @native:crypto/cipher/aes/128/v1)
    (ctr @native:crypto/cipher/ctr/v1)
    (begin
      ; NIST SP 800-38A AES-CTR encryption vector.
      (assert
        (= (bytes/hex/encode
             (ctr/crypt
               aes/encrypt
               (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c")
               (bytes/hex/decode "f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff")
               (bytes/hex/decode "6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411e5fbc1191a0a52eff69f2445df4f9b17ad2b417be66c3710")))
           "874d6191b620e3261bef6864990db6ce9806f66b7970fdff8617187bb9fffdff5ae4df3edbd5d35e5b4f09020db03eab1e031dda2fbe03d1792170a0f3009cee"))

      ; CTR decrypts by applying the same block encryption function.
      (assert
        (= (bytes/hex/encode
             (ctr/crypt
               aes/encrypt
               (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c")
               (bytes/hex/decode "f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff")
               (bytes/hex/decode "874d6191b620e3261bef6864990db6ce9806f66b7970fdff8617187bb9fffdff5ae4df3edbd5d35e5b4f09020db03eab1e031dda2fbe03d1792170a0f3009cee")))
           "6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411e5fbc1191a0a52eff69f2445df4f9b17ad2b417be66c3710"))

      ; CTR advances counters and handles a final partial input block.
      (assert
        (= (bytes/hex/encode
             (ctr/crypt
               (fn (key block) block)
               null
               (bytes/hex/decode "00")
               (bytes/hex/decode "ff00")))
           "ff01"))

      ; CTR preserves secret taint from keys, counters, and input.
      (assert
        (and
          (secret?
            (ctr/crypt
              aes/encrypt
              (secret/mark (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c"))
              (bytes/hex/decode "f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff")
              (bytes/hex/decode "6bc1bee22e409f96e93d7e117393172a")))
          (secret?
            (ctr/crypt
              aes/encrypt
              (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c")
              (secret/mark (bytes/hex/decode "f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff"))
              (bytes/hex/decode "6bc1bee22e409f96e93d7e117393172a")))))

      ; CTR validates counters and block function output lengths.
      (assert
        (and
          (string/contains?
            (error/value
              (error/catch
                (ctr/crypt aes/encrypt (bytes/hex/decode "2b7e151628aed2a6abf7158809cf4f3c") (bytes/concat) (bytes/hex/decode "00"))))
            "non-empty counter")
          (string/contains?
            (error/value
              (error/catch
                (ctr/crypt (fn (key block) (bytes/concat)) null (bytes/hex/decode "00") (bytes/hex/decode "00"))))
            "block function must return")))
      null))
  null)
