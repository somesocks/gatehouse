(let
  ((require-block-size
    (fn (block-size)
      (if (or (< block-size 1) (> block-size 255))
          (error/throw "pkcs7 requires a block size from 1 through 255")
          block-size)))
   (pad
    (fn (block-size bytes)
      (let
        ((size (require-block-size block-size))
         (length (bytes/length bytes)))
        (let
          ((remainder (int/rem length size)))
          (let
            ((padding (if (= remainder 0) size (- size remainder))))
            (bytes/pad/end bytes (+ length padding) padding))))))
   (unpad
    (fn (block-size bytes)
      (let
        ((size (require-block-size block-size))
         (length (bytes/length bytes)))
        (if (or (= length 0) (not (= (int/rem length size) 0)))
            (error/throw "pkcs7/unpad requires valid PKCS#7 padding")
            (let
              ((padding
                (bytes/uint/be/decode
                  (bytes/slice bytes (- length 1) length))))
              (if (or (< padding 1) (> padding size) (> padding length))
                  (error/throw "pkcs7/unpad requires valid PKCS#7 padding")
                  (let
                    ((content-length (- length padding)))
                    (if (= (bytes/slice bytes content-length length)
                           (bytes/pad/end (bytes/concat) padding padding))
                        (bytes/slice bytes 0 content-length)
                        (error/throw "pkcs7/unpad requires valid PKCS#7 padding"))))))))))
  (list
    ; (pkcs7/pad block-size bytes) -> Bytes
    ; Appends PKCS#7 padding for a block size from 1 through 255.
    ; Example: (import (pkcs7 @native:crypto/padding/pkcs7/v1) (bytes/hex/encode (pkcs7/pad 8 (bytes/utf8/encode "YELLOW")))) => "59454c4c4f570202".
    (pair 'pad pad)
    ; (pkcs7/unpad block-size bytes) -> Bytes
    ; Removes and validates PKCS#7 padding for a block size from 1 through 255.
    ; Example: (import (pkcs7 @native:crypto/padding/pkcs7/v1) (bytes/utf8/decode (pkcs7/unpad 8 (bytes/hex/decode "59454c4c4f570202")))) => "YELLOW".
    (pair 'unpad unpad)))
