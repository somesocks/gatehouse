(begin
  (import
    (random @native:random/v1)
    (begin
      ; Bytes returns the requested public length.
      (let ((value (random/bytes 32)))
        (assert (and (= (bytes/length value) 32) (not (secret? value)))))

      ; Int returns a public value below its bound.
      (let ((value (random/int 10)))
        (assert (and (<= 0 value) (< value 10) (not (secret? value)))))

      ; A bound of one has the only possible result.
      (assert (= (random/int 1) 0))

      ; Bool returns a public Boolean.
      (let ((value (random/bool)))
        (assert (and (bool? value) (not (secret? value)))))

      ; Random exports report signatures and runnable examples.
      (assert
        (and
          (string/contains? (help random/bytes) "(random/bytes length) -> Bytes")
          (string/contains? (help random/int) "(random/int bound) -> Integer")
          (string/contains? (help random/bool) "(random/bool) -> Boolean")))

      ; Bytes validates arity, type, and length.
      (assert
        (and
          (string/contains? (error/value (error/catch (random/bytes))) "requires one non-negative integer")
          (string/contains? (error/value (error/catch (random/bytes 1 2))) "requires one non-negative integer")
          (string/contains? (error/value (error/catch (random/bytes -1))) "requires one non-negative integer")
          (string/contains? (error/value (error/catch (random/bytes "1"))) "expected an integer")))

      ; Int validates arity, type, and bound.
      (assert
        (and
          (string/contains? (error/value (error/catch (random/int))) "requires one positive integer")
          (string/contains? (error/value (error/catch (random/int 0))) "requires one positive integer")
          (string/contains? (error/value (error/catch (random/int -1))) "requires one positive integer")
          (string/contains? (error/value (error/catch (random/int "1"))) "expected an integer")))

      ; Bool requires no arguments.
      (assert
        (string/contains?
          (error/value (error/catch (random/bool #t)))
          "requires no arguments"))
      null))
  null)
