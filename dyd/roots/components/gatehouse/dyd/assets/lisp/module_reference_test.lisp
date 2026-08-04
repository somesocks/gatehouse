(begin
  ; Identical fingerprint references compare equal.
  (assert
    (= @#sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
       @#sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef))

  ; Identical file references compare equal.
  (assert (= @file:../shared.lisp @file:../shared.lisp))

  ; Distinct native references do not compare equal.
  (assert (not (= @native:net/v1 @native:time/v1)))

  ; Module references report their help category.
  (assert (= (help @native:net/v1) "module reference"))
  null)
