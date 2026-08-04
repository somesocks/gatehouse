(begin
  (import
    (time @native:time/v1)
    (begin
      ; Now returns a public positive Unix-millisecond timestamp.
      (let ((value (time/now)))
        (assert (and (int? value) (> value 0) (not (secret? value)))))

      ; Now reports its public help text.
      (assert (= (help time/now) "Returns the current Unix time in milliseconds."))

      ; Now requires no arguments.
      (assert
        (string/contains?
          (error/value (error/catch (time/now 1)))
          "requires no arguments"))
      null))
  null)
