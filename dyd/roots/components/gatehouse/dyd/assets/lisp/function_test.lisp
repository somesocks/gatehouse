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

  ; Functions can bind all arguments to a rest parameter.
  (assert (= ((fn rest rest)) null))
  (assert (= ((fn rest rest) 1 2 3) (list 1 2 3)))

  ; Functions can combine fixed and rest parameters.
  (assert (= ((fn (first . rest) (list first rest)) 1) (list 1 null)))
  (assert (= ((fn (first . rest) (list first rest)) 1 2 3) (list 1 (list 2 3))))

  ; Rest parameters retain lexical bindings and preserve secret taint.
  (assert (= (let ((prefix 1)) ((fn (first . rest) (list prefix first rest)) 2 3)) (list 1 2 (list 3))))
  (assert (taint/secret? ((fn rest rest) (taint/secret/mark 1))))

  ; Recursive variadic functions work through fn/apply.
  (assert
    (= (let ((collect (fn (first . rest) (if (null? rest) (list first) (pair first (fn/apply collect rest))))))
         (collect 1 2 3))
       (list 1 2 3)))

  ; Tail calls do not consume the Go call stack.
  (assert
    (= (let ((count (fn (n total)
                       (if (= n 0) total (count (- n 1) (+ total 1))))))
         (count 100000 0))
       100000))
  null)
