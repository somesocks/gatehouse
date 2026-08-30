(begin
  (import
    (time @native:time/v1)
    (begin
      ; Now returns a public positive Unix-millisecond timestamp.
      (let ((value (time/now)))
        (assert (and (int? value) (> value 0) (not (taint/secret? value)))))

      ; Now reports its signature and runnable example.
      (assert
        (and
          (string/contains? (help time/now) "(time/now) -> Integer")
          (string/contains? (help time/now) "Example: (import (time @native:time/v1) (int? (time/now))) => #t.")))

      ; Now requires no arguments.
      (assert
        (string/contains?
          (error/value (error/catch (time/now 1)))
          "requires no arguments"))
      null))
  null)
