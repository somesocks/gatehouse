package agent

const agentPrelude = `(import/restrict
  (@native:json/v1
   @native:seq/v1
   @native:crypto/digest/sha256/v1
   @native:crypto/mac/hmac/sha256/v1
   @native:crypto/cipher/aes/128/v1
   @native:crypto/cipher/cbc/v1
   @native:crypto/cipher/ctr/v1
   @native:crypto/padding/pkcs7/v1
   @native:gatehouse/project/v1
   @native:gatehouse/session/v1)
  (import
    (json @native:json/v1)
    (seq @native:seq/v1)
    (crypto/digest/sha256 @native:crypto/digest/sha256/v1)
    (crypto/mac/hmac/sha256 @native:crypto/mac/hmac/sha256/v1)
    (crypto/cipher/aes/128 @native:crypto/cipher/aes/128/v1)
    (crypto/cipher/cbc @native:crypto/cipher/cbc/v1)
    (crypto/cipher/ctr @native:crypto/cipher/ctr/v1)
    (crypto/padding/pkcs7 @native:crypto/padding/pkcs7/v1)
    (project @native:gatehouse/project/v1)
    (session @native:gatehouse/session/v1)
    (let ((import null)
          (import/restrict null)
          (import/search null))
      (eval program))))`
