(begin
  ; Addition sums every integer argument.
  (assert (= (+ 1 2 3) 6))

  ; Subtraction applies operands from left to right.
  (assert (= (- 10 3 2) 5))

  ; Multiplication multiplies every integer argument.
  (assert (= (* 2 3 4) 24))

  ; Multiplication preserves a negative factor.
  (assert (= (* 1 -1) -1))

  ; Integers satisfy the integer predicate.
  (assert (int? 1))

  ; Non-integer values do not satisfy the integer predicate.
  (assert (not (int? "1")))

  ; Division truncates positive values toward zero.
  (assert (= (int/div 5 2) 2))

  ; Division truncates a negative dividend toward zero.
  (assert (= (int/div -5 2) -2))

  ; Division truncates a negative divisor toward zero.
  (assert (= (int/div 5 -2) -2))

  ; Division of two negative values produces a positive quotient.
  (assert (= (int/div -5 -2) 2))

  ; Remainder preserves the positive dividend sign.
  (assert (= (int/rem 5 2) 1))

  ; Remainder preserves the negative dividend sign.
  (assert (= (int/rem -5 2) -1))

  ; Remainder ignores the divisor sign.
  (assert (= (int/rem 5 -2) 1))

  ; Remainder remains negative for a negative dividend and divisor.
  (assert (= (int/rem -5 -2) -1))

  ; The minimum integer remainder avoids division overflow.
  (assert (= (int/rem -9223372036854775808 -1) 0))

  ; Absolute value returns a positive magnitude.
  (assert (= (int/abs -5) 5))

  ; Absolute value preserves zero.
  (assert (= (int/abs 0) 0))

  ; Minimum selects the smallest supplied value.
  (assert (= (int/min 3 -1 2) -1))

  ; Maximum selects the largest supplied value.
  (assert (= (int/max 3 -1 2) 3))

  ; Minimum accepts a single value.
  (assert (= (int/min 5) 5))

  ; Maximum accepts a single value.
  (assert (= (int/max -5) -5))

  ; Greater-than recognizes strictly decreasing values.
  (assert (> 3 2))

  ; Greater-than rejects increasing values.
  (assert (not (> 2 3)))

  ; Greater-than-or-equal accepts equal values.
  (assert (>= 3 3))

  ; Greater-than-or-equal rejects increasing values.
  (assert (not (>= 2 3)))

  ; Division rejects a zero divisor.
  (assert
    (string/contains?
      (error/value (error/catch (int/div 1 0)))
      "non-zero divisor"))

  ; Remainder rejects a zero divisor.
  (assert
    (string/contains?
      (error/value (error/catch (int/rem 1 0)))
      "non-zero divisor"))

  ; Dividing the minimum integer by negative one overflows.
  (assert
    (string/contains?
      (error/value (error/catch (int/div -9223372036854775808 -1)))
      "integer overflow"))

  ; Division requires exactly two integer arguments.
  (assert
    (string/contains?
      (error/value (error/catch (int/div 1)))
      "requires two integers"))

  ; Remainder rejects non-integer arguments.
  (assert
    (string/contains?
      (error/value (error/catch (int/rem 1 "2")))
      "expected an integer"))

  ; Absolute value rejects the minimum integer overflow.
  (assert
    (string/contains?
      (error/value (error/catch (int/abs -9223372036854775808)))
      "integer overflow"))

  ; Absolute value requires one integer argument.
  (assert
    (string/contains?
      (error/value (error/catch (int/abs)))
      "requires one integer"))

  ; Minimum requires at least one integer.
  (assert
    (string/contains?
      (error/value (error/catch (int/min)))
      "requires at least one integer"))

  ; Maximum rejects non-integer arguments.
  (assert
    (string/contains?
      (error/value (error/catch (int/max 1 "2")))
      "expected an integer"))

  ; Comparisons require at least two integer arguments.
  (assert
    (string/contains?
      (error/value (error/catch (> 1)))
      "requires at least two arguments"))
  null)
