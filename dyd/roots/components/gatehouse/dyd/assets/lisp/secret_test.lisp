(begin
  ; Marked values and direct-flow transformations remain secret.
  (assert
    (and
      (secret? (secret/mark "secret"))
      (secret? (string/upper (secret/mark "secret")))
      (secret? (head (list (secret/mark "secret"))))
      (secret? ((fn (value) value) (secret/mark "secret")))
      (secret? (let ((value (secret/mark "secret"))) ((fn () value))))
      (secret? (list/map string/upper (list (secret/mark "secret"))))
      (secret? (secret/mark (list 1 2)))))

  ; Length operations return public values derived from secret input.
  (assert
    (and
      (not (secret? (string/length (string/upper (secret/mark "secret")))))
      (= (string/length (string/upper (secret/mark "secret"))) 6)
      (not (secret? (string/length (secret/mark "secret"))))
      (= (string/length (secret/mark "secret")) 6)
      (not (secret? (list/length (list (secret/mark "secret")))))
      (= (list/length (list (secret/mark "secret"))) 1)
      (not (secret? (bytes/length (secret/mark (bytes/hex/decode "ff00")))))
      (= (bytes/length (secret/mark (bytes/hex/decode "ff00"))) 2)))

  ; A public predicate can select a public branch from secret input.
  (assert
    (let ((result (if (string/contains? (secret/mark "secret") "e") "yes" "no")))
      (and (not (secret? result)) (= result "yes"))))

  ; Secret predicate recognizes marked strings and builtins.
  (assert (and (secret? (secret/mark "secret")) (secret? (secret/mark +))))

  ; Help operations keep public metadata public.
  (assert
    (and
      (= (help (secret/mark string/slice)) "Returns a Unicode code-point substring slice between an inclusive start and exclusive end.")
      (secret? (help/document (secret/mark "secret") "Public help text."))
      (= (help/search (secret/mark "secret")) (list 'secret/mark 'secret?))
      (= (help/env (secret/mark "secret")) (list 'secret/mark 'secret?))))

  ; Leaky Bytes encoders preserve secret taint.
  (assert
    (and
      (secret? (bytes/hex/encode (secret/mark (bytes/hex/decode "ff"))))
      (secret? (bytes/base64/encode (secret/mark (bytes/hex/decode "ff"))))
      (secret? (bytes/base64url/encode (secret/mark (bytes/hex/decode "ff"))))
      (secret? (bytes/base64/pad (secret/mark "/wA")))))

  ; Predicate and search operations return public Boolean results.
  (assert
    (and
      (not (secret? (bool? (secret/mark "secret"))))
      (not (secret? (int? (secret/mark "secret"))))
      (not (secret? (symbol? (secret/mark "secret"))))
      (not (secret? (pair? (secret/mark "secret"))))
      (not (secret? (null? (secret/mark "secret"))))
      (not (secret? (list? (secret/mark "secret"))))
      (not (secret? (string? (secret/mark "secret"))))
      (not (secret? (string/contains? (secret/mark "secret") "a")))
      (not (secret? (string/prefix? (secret/mark "secret") "a")))
      (not (secret? (string/suffix? (secret/mark "secret") "t")))
      (not (secret? (bytes? (secret/mark (bytes/hex/decode "ff")))))))

  ; Caught diagnostics redact the secret source value.
  (assert
    (let ((diagnostic (error/value (error/catch (int/div (secret/mark "secret") 1)))))
      (and
        (string/contains? diagnostic "#<secret>")
        (not (string/contains? diagnostic "\"secret\"")))))
  null)
