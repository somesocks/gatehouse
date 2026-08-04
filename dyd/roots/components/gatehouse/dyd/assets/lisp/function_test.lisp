(begin
  ; Closures retain bindings from their lexical definition environment.
  (assert
    (= (let ((make-adder (fn (x) (fn (y) (+ x y)))))
         ((make-adder 2) 3))
       5))

  ; Later let bindings can use earlier initialized bindings.
  (assert (= (let ((x 1) (y (+ x 1))) y) 2))

  ; Mutually recursive bindings share their lexical environment.
  (assert
    (let
      ((even? (fn (n) (if (= n 0) #t (odd? (- n 1)))))
       (odd? (fn (n) (if (= n 0) #f (even? (- n 1))))))
      (even? 100)))

  ; Reading an uninitialized recursive binding fails.
  (assert
    (string/contains?
      (error/value (error/catch (let ((first second) (second 2)) first)))
      "uninitialized"))

  ; Begin evaluates earlier expressions and returns the final value.
  (assert (= (begin (+ 1 2) (+ 3 4)) 7))

  ; Let accepts an empty binding list.
  (assert (= (let () 1) 1))

  ; Inner lexical bindings shadow outer bindings.
  (assert (= (let ((x 1)) (let ((x 2)) x)) 2))

  ; Tail calls do not consume the Go call stack.
  (assert
    (= (let ((count (fn (n total)
                       (if (= n 0) total (count (- n 1) (+ total 1))))))
         (count 100000 0))
       100000))
  null)
