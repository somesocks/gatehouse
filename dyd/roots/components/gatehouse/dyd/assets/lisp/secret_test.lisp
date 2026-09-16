(begin
  ; Marked values and direct-flow transformations remain secret.
  (assert
    (and
      (taint/secret? (taint/secret/mark "secret"))
      (taint/secret? (string/upper (taint/secret/mark "secret")))
      (taint/secret? (head (list (taint/secret/mark "secret"))))
      (taint/secret? ((fn (value) value) (taint/secret/mark "secret")))
      (taint/secret? (let ((value (taint/secret/mark "secret"))) ((fn () value))))
      (taint/secret? (list/map string/upper (list (taint/secret/mark "secret"))))
      (taint/secret? (taint/secret/mark (list 1 2)))))

  ; Pairs and enclosing lists inherit taint from either element.
  (assert
    (and
      (taint/secret? (pair (taint/secret/mark 1) 2))
      (taint/secret? (pair 1 (taint/secret/mark 2)))
      (taint/secret? (list 1 (taint/secret/mark 2)))
      (taint/secret? (list (taint/secret/mark 1) 2))))

  ; Length operations return public values derived from secret input.
  (assert
    (and
      (not (taint/secret? (string/length (string/upper (taint/secret/mark "secret")))))
      (= (string/length (string/upper (taint/secret/mark "secret"))) 6)
      (not (taint/secret? (string/length (taint/secret/mark "secret"))))
      (= (string/length (taint/secret/mark "secret")) 6)
      (not (taint/secret? (list/length (list (taint/secret/mark "secret")))))
      (= (list/length (list (taint/secret/mark "secret"))) 1)
      (not (taint/secret? (bytes/length (taint/secret/mark (bytes/hex/decode "ff00")))))
      (= (bytes/length (taint/secret/mark (bytes/hex/decode "ff00"))) 2)))

  ; A content predicate taints its selected branch.
  (assert
    (let ((result (if (string/contains? (taint/secret/mark "secret") "e") "yes" "no")))
      (and (taint/secret? result) (= result "yes"))))

  ; Ordered predicates taint selected branches and no-match diagnostics.
  (assert
    (and
      (let ((result
              (ifs
                ((string/contains? (taint/secret/mark "secret") "x") "yes")
                (#t "no"))))
        (and (taint/secret? result) (= result "no")))
      (taint/secret?
        (error/catch
          (ifs
            ((string/contains? (taint/secret/mark "secret") "x") "yes"))))
      (taint/secret?
        (error/catch
          (ifs
            ((string/contains? (taint/secret/mark "secret") "e")
              (error/throw "selected")))))))

  ; Secret predicate recognizes marked strings and builtins.
  (assert (and (taint/secret? (taint/secret/mark "secret")) (taint/secret? (taint/secret/mark +))))

  ; Help operations that discriminate their input preserve its taint.
  (assert
    (and
       (taint/secret? (help (taint/secret/mark string/slice)))
       (taint/secret? (help/document (taint/secret/mark "secret") "Public help text."))
       (taint/secret? (help (help/document (taint/secret/mark "secret") "Public help text.")))
       (taint/secret? (help/search (taint/secret/mark "secret")))
       (taint/secret? (help/env (taint/secret/mark "secret")))))

  ; Leaky Bytes encoders preserve secret taint.
  (assert
    (and
      (taint/secret? (bytes/hex/encode (taint/secret/mark (bytes/hex/decode "ff"))))
      (taint/secret? (bytes/base64/encode (taint/secret/mark (bytes/hex/decode "ff"))))
      (taint/secret? (bytes/base64url/encode (taint/secret/mark (bytes/hex/decode "ff"))))
      (taint/secret? (bytes/base64/pad (taint/secret/mark "/wA")))))

  ; Type predicates are public while content predicates preserve taint.
  (assert
    (and
      (not (taint/secret? (bool? (taint/secret/mark "secret"))))
      (not (taint/secret? (int? (taint/secret/mark "secret"))))
      (not (taint/secret? (symbol? (taint/secret/mark "secret"))))
      (not (taint/secret? (pair? (taint/secret/mark "secret"))))
      (not (taint/secret? (null? (taint/secret/mark "secret"))))
      (not (taint/secret? (list? (taint/secret/mark "secret"))))
      (not (taint/secret? (string? (taint/secret/mark "secret"))))
      (taint/secret? (string/contains? (taint/secret/mark "secret") "a"))
      (taint/secret? (string/prefix? (taint/secret/mark "secret") "a"))
      (taint/secret? (string/suffix? (taint/secret/mark "secret") "t"))
      (not (taint/secret? (bytes? (taint/secret/mark (bytes/hex/decode "ff")))))))

  ; Caught diagnostics redact the secret source value.
  (assert
    (taint/secret?
      (error/value (error/catch (int/div (taint/secret/mark "secret") 1)))))
  null)
