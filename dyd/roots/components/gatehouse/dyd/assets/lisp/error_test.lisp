(begin
  ; Catch returns a non-error value unchanged.
  (assert (= (error/catch 7) 7))

  ; Catch converts an explicit throw into an Error value.
  (assert (error? (error/catch (error/throw (list 'missing 7)))))

  ; Error values preserve arbitrary thrown values.
  (assert (= (error/value (error/catch (error/throw (list 'missing 7)))) (list 'missing 7)))

  ; Error values preserve opaque Bytes payloads.
  (assert (bytes? (error/value (error/catch (error/throw (bytes/hex/decode "ff"))))))

  ; Catch converts runtime diagnostics into public text payloads.
  (assert
    (string/contains?
      (error/value (error/catch (int/div 1 0)))
      "non-zero divisor"))

  ; Throwing a caught Error preserves its original payload.
  (assert (= (error/value (error/catch (error/throw (error/catch (error/throw 7))))) 7))

  ; Errors can carry secret-tainted values.
  (assert (error? (error/catch (error/throw (secret/mark "secret")))))

  ; Catch preserves a secret-tainted thrown value.
  (assert (secret? (error/catch (error/throw (secret/mark "secret")))))

  ; Error value preserves secret taint.
  (assert (secret? (error/value (error/catch (error/throw (secret/mark "secret"))))))

  ; Error values report their built-in help text.
  (assert (= (help (error/catch (error/throw 7))) "error"))

  ; Catch requires exactly one expression.
  (assert
    (string/contains?
      (error/value (error/catch (error/catch)))
      "requires one expression"))

  ; Catch rejects more than one expression.
  (assert
    (string/contains?
      (error/value (error/catch (error/catch 1 2)))
      "requires one expression"))

  ; Throw requires one value.
  (assert
    (string/contains?
      (error/value (error/catch (error/throw)))
      "requires one value"))

  ; Throw rejects extra values.
  (assert
    (string/contains?
      (error/value (error/catch (error/throw 1 2)))
      "requires one value"))

  ; Error value requires an Error argument.
  (assert
    (string/contains?
      (error/value (error/catch (error/value 1)))
      "expected an Error"))

  ; Assert without an optional value returns null.
  (assert (null? (assert #t)))

  ; Assert returns its optional value when the condition is true.
  (assert (= (assert #t 7) 7))

  ; Assert returns opaque values without changing them.
  (assert (bytes? (assert #t (bytes/hex/decode "ff"))))

  ; Timing assertions return their expression value when within the limit.
  (assert (= (assert/timing 1000 (+ 2 5)) 7))

  ; Timing assertions report their signature and runnable example.
  (assert
    (and
      (string/contains? (help assert/timing) "(assert/timing maximum-milliseconds expression) -> Value")
      (string/contains? (help assert/timing) "Example: (assert/timing 1000 (+ 1 2)) => 3.")))

  ; Timing assertions require an integer, non-negative maximum and one expression.
  (assert
    (and
      (string/contains?
        (error/value (error/catch (assert/timing)))
        "requires maximum milliseconds and one expression")
      (string/contains?
        (error/value (error/catch (assert/timing -1 null)))
        "requires non-negative maximum milliseconds")
      (string/contains?
        (error/value (error/catch (assert/timing "fast" null)))
        "expected an integer")))

  ; A false assertion does not evaluate its success value.
  (assert (= (error/value (error/catch (assert #f missing))) "assertion failed"))

  ; A false assertion evaluates its failure value instead of its success value.
  (assert (= (error/value (error/catch (assert #f missing "required value"))) "required value"))

  ; A true assertion does not evaluate its failure value.
  (assert (= (assert #t 7 missing) 7))

  ; A failure Error value is raised without an extra Error wrapper.
  (assert
    (= (error/value
         (error/catch
           (assert #f null (error/catch (error/throw "required value")))))
       "required value"))

  ; Assert reports its signature and runnable example.
  (assert
    (and
      (string/contains? (help assert) "(assert condition [value [failure]]) -> Null | Value")
      (string/contains? (help assert) "Example: (assert #t 7 \"required value\") => 7.")))

  ; Assert requires a condition and at most two optional values.
  (assert
    (string/contains?
      (error/value (error/catch (assert)))
      "requires a condition and optional value and failure"))

  ; Assert rejects more than two optional values.
  (assert
    (string/contains?
      (error/value (error/catch (assert #t 1 2 3)))
      "requires a condition and optional value and failure"))

  ; Assert requires a Boolean condition.
  (assert
    (string/contains?
      (error/value (error/catch (assert 1 7)))
      "expected a Boolean"))

  ; Quote requires one expression.
  (assert
    (string/contains?
      (error/value (error/catch (quote)))
      "quote requires one expression"))

  ; Quote rejects extra expressions.
  (assert
    (string/contains?
      (error/value (error/catch (quote 1 2)))
      "quote requires one expression"))

  ; Let requires a proper list of bindings.
  (assert
    (string/contains?
      (error/value (error/catch (let (x) x)))
      "expected a proper list"))

  ; Let rejects duplicate bindings.
  (assert
    (string/contains?
      (error/value (error/catch (let ((x 1) (x 2)) x)))
      "duplicated"))

  ; Let requires one body expression.
  (assert
    (string/contains?
      (error/value (error/catch (let () 1 2)))
      "let requires bindings and one body expression"))

  ; Functions require symbol parameters.
  (assert
    (string/contains?
      (error/value (error/catch (fn (1) 1)))
      "fn parameters must be symbols"))

  ; Dotted function tails must be unique symbols.
  (assert
    (string/contains?
      (error/value (error/catch (fn (x . 1) x)))
      "fn parameters must be symbols"))
  (assert
    (string/contains?
      (error/value (error/catch (fn (x . x) x)))
      "duplicated"))

  ; Functions require one body expression.
  (assert
    (string/contains?
      (error/value (error/catch (fn (x) x x)))
      "fn requires parameters and one body expression"))

  ; Begin requires at least one expression.
  (assert
    (string/contains?
      (error/value (error/catch (begin)))
      "begin requires at least one expression"))

  ; Begin evaluates all non-final expressions.
  (assert
    (string/contains?
      (error/value (error/catch (begin missing 1)))
      "unknown binding"))

  ; Function calls enforce their declared arity.
  (assert
    (string/contains?
      (error/value (error/catch (let ((id (fn (x) x))) (id))))
      "function requires 1 arguments, got 0"))

  ; Variadic functions require their fixed arguments.
  (assert
    (string/contains?
      (error/value (error/catch ((fn (x . rest) x))))
      "function requires at least 1 arguments, got 0"))

  ; Referencing an unknown symbol fails.
  (assert
    (string/contains?
      (error/value (error/catch missing))
      "unknown binding"))

  ; Calling a non-callable value fails.
  (assert
    (string/contains?
      (error/value (error/catch (1 2)))
      "not callable"))

  ; Functions reject duplicate parameter names.
  (assert
    (string/contains?
      (error/value (error/catch (fn (x x) x)))
      "duplicated"))

  ; If requires a condition and both branches.
  (assert
    (string/contains?
      (error/value (error/catch (if #t 1)))
      "requires a condition"))

  ; Head rejects null.
  (assert
    (string/contains?
      (error/value (error/catch (head null)))
      "requires a pair"))

  ; The legacy cons name is unavailable.
  (assert
    (string/contains?
      (error/value (error/catch (cons 1 2)))
      "unknown binding"))

  ; The legacy first name is unavailable.
  (assert
    (string/contains?
      (error/value (error/catch (first (list 1))))
      "unknown binding"))

  ; The legacy rest name is unavailable.
  (assert
    (string/contains?
      (error/value (error/catch (rest (list 1))))
      "unknown binding"))
  null)
