(begin
  ; Booleans satisfy the Boolean predicate.
  (assert (bool? #t))

  ; Non-Boolean values do not satisfy the Boolean predicate.
  (assert (not (bool? 1)))

  ; Negation maps false to true.
  (assert (not #f))

  ; If evaluates its false branch when the condition is false.
  (assert (= (if #f 1 2) 2))

  ; If does not evaluate an unused false branch.
  (assert (= (if #t 1 missing) 1))

  ; If does not evaluate an unused true branch.
  (assert (= (if #f missing 2) 2))

  ; And with no operands is true.
  (assert (and))

  ; Or with no operands is false.
  (assert (not (or)))

  ; And is true when every operand is true.
  (assert (and #t #t))

  ; And is false when any operand is false.
  (assert (not (and #t #f)))

  ; Or is true when any operand is true.
  (assert (or #f #t))

  ; Or is false when every operand is false.
  (assert (not (or #f #f)))

  ; And short-circuits after a false operand.
  (assert (not (and #f missing)))

  ; Or short-circuits after a true operand.
  (assert (or #t missing))

  ; The Boolean predicate requires one argument.
  (assert
    (string/contains?
      (error/value (error/catch (bool?)))
      "bool? requires one argument"))

  ; If requires a Boolean condition.
  (assert
    (string/contains?
      (error/value (error/catch (if null 1 2)))
      "expected a Boolean"))

  ; Negation requires a Boolean value.
  (assert
    (string/contains?
      (error/value (error/catch (not null)))
      "expected a Boolean"))

  ; And requires Boolean operands.
  (assert
    (string/contains?
      (error/value (error/catch (and #t null)))
      "expected a Boolean"))

  ; Or requires Boolean operands.
  (assert
    (string/contains?
      (error/value (error/catch (or #f null)))
      "expected a Boolean"))
  null)
