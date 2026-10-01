package agent

const agentPreludeStart = `(let
  (
    ; Import declarations remain raw forms until fn/apply invokes import.
    (prelude/imports
      '(
        ; Standard library modules.
        (json @native:json/v1)
        (xml @native:xml/v1)
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

        ; Workspace resource modules.
`

const agentPreludeProjectImports = `
        (project/info @native:gatehouse/project/info/v1)
        (project/files @native:gatehouse/project/files/v1)
        (project/notes @native:gatehouse/project/notes/v1)
        (project/tasks @native:gatehouse/project/tasks/v1)
        (project/secrets @native:gatehouse/project/secrets/v1)
        (project/records @native:gatehouse/project/records/v1)
`

const agentPreludeEnd = `
        (session/files @native:gatehouse/session/files/v1)
        (session/notes @native:gatehouse/session/notes/v1)
        (session/tasks @native:gatehouse/session/tasks/v1)
        (session/secrets @native:gatehouse/session/secrets/v1)
        (session/events @native:gatehouse/session/events/v1)
        (input @native:gatehouse/input/v1)
        (policy @native:gatehouse/policy/v1)
        (web @native:gatehouse/web/v1)

        (let
          (
            ; Session and project secret mutations opt into approval when exposed.
            (prelude/operations
              (list))
            (prelude/operation-binding
              (fn (operation)
                (list
                  (head operation)
                  (policy/require-approval
                    (head (tail operation))
                    (head (tail (tail operation)))))))
            (prelude/input-ask
              (fn (form) (json/decode (input/ask-json form))))
            (prelude/session-file-picker
              (help/document
                (fn (id label inputs)
                  (input/form/custom id label "/app/tools/session-file-picker/" inputs
                    (input/form/custom/capabilities
                      (list "session.file.list" "session.file.read" "session.file.upload"))))
                "(session-file-picker id label inputs) -> Input Field. Select completed session files or upload new ones. Returns a JSON object with file_ids. Inputs is a JSON object; optional media_types is an array of MIME types or wildcards, and max_files is a positive integer. Example: (session-file-picker \"files\" \"Attachments\" (json/object))."))
          )
          (fn/apply let
            (list
              (list/concat
                (list (list 'input/ask prelude/input-ask) (list 'session-file-picker prelude/session-file-picker))
                (list/map prelude/operation-binding prelude/operations))
              '(let
                  ((import null)
                   (import/restrict null)
                   (import/search null)
                   (input/ask-json null)
                   (prelude/input-ask null)
                   (prelude/session-file-picker null)
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

const agentPrelude = agentPreludeStart + agentPreludeProjectImports + agentPreludeEnd
const agentSessionPrelude = agentPreludeStart + agentPreludeEnd
