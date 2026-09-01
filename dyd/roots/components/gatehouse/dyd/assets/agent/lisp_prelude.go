package agent

const agentPrelude = `(let
  (
    ; Import declarations remain raw forms until fn/apply invokes import.
    (prelude/imports
      '(
        ; Standard library modules.
        (json @native:json/v1)
        (seq @native:seq/v1)
        (vector @native:vector/v1)
        (archive/zip @native:archive/zip/v1)
        (compress/deflate @native:compress/deflate/v1)
        (crypto/digest/sha256 @native:crypto/digest/sha256/v1)
        (crypto/mac/hmac/sha256 @native:crypto/mac/hmac/sha256/v1)
        (crypto/cipher/aes/128 @native:crypto/cipher/aes/128/v1)
        (crypto/cipher/cbc @native:crypto/cipher/cbc/v1)
        (crypto/cipher/ctr @native:crypto/cipher/ctr/v1)
        (crypto/padding/pkcs7 @native:crypto/padding/pkcs7/v1)

        ; Workspace capability modules.
        (project @native:gatehouse/project/v1)
        (session @native:gatehouse/session/v1)
        (policy @native:gatehouse/policy/v1)
        (web @native:gatehouse/web/v1)

        (let
          (
            ; Mutating operations require user approval before invocation.
            (prelude/operations
              (list
                (list 'session/notes/create "Create a session note" session/notes/create)
                (list 'project/notes/create "Create a project note" project/notes/create)
				(list 'session/notes/update "Update a session note" session/notes/update)
				(list 'project/notes/update "Update a project note" project/notes/update)
                (list 'session/notes/remove "Remove a session note" session/notes/remove)
                (list 'project/notes/remove "Remove a project note" project/notes/remove)
                (list 'project/info/set "Update project details" project/info/set)))
            (prelude/operation-binding
              (fn (operation)
                (list
                  (head operation)
                  (policy/require-approval
                    (head (tail operation))
                    (head (tail (tail operation)))))))
          )
          (fn/apply let
            (list
              (list/map prelude/operation-binding prelude/operations)
              '(let
                 ((import null)
                  (import/restrict null)
                  (import/search null)
                  (policy/await-approval null)
                  (policy/require-approval null)
                  (prelude/imports null)
                  (prelude/operations null)
                  (prelude/operation-binding null))
                 (eval program)))))
      )
    )
  )
  (fn/apply import prelude/imports))`
