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
   @native:gatehouse/session/v1
   @native:gatehouse/policy/v1)
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
	(policy @native:gatehouse/policy/v1)
	(let ((raw-session-notes-create session/notes/create)
	      (raw-project-notes-create project/notes/create)
	      (raw-project-info-set project/info/set))
	  (let ((session/notes/create (policy/require-approval "Create a session note" raw-session-notes-create))
	        (project/notes/create (policy/require-approval "Create a project note" raw-project-notes-create))
	        (project/info/set (policy/require-approval "Update project details" raw-project-info-set)))
	    (let ((import null)
	          (import/restrict null)
	          (import/search null)
	          (policy/await-approval null)
	          (policy/require-approval null)
	          (raw-session-notes-create null)
	          (raw-project-notes-create null)
	          (raw-project-info-set null))
	      (eval program))))))`
