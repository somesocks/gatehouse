(let
  ((make
    (fn (length root)
      (list 'vector/instance length root)))
   (node
    (fn (value left right)
      (pair value (pair left right))))
   (node?
    (fn (tree)
      (and (pair? tree) (pair? (tail tree)))))
   (node/value
    (fn (tree)
      (if (node? tree)
          (head tree)
          (error/throw "vector tree contains an invalid node"))))
   (node/left
    (fn (tree)
      (if (node? tree)
          (head (tail tree))
          (error/throw "vector tree contains an invalid node"))))
   (node/right
    (fn (tree)
      (if (node? tree)
          (tail (tail tree))
          (error/throw "vector tree contains an invalid node"))))
   (tree/size
    (fn (tree)
      (if (null? tree)
          0
          (let
            ((left-size (tree/size (node/left tree)))
             (right-size (tree/size (node/right tree))))
            (if (and (<= right-size left-size) (<= left-size (+ right-size 1)))
                (+ 1 left-size right-size)
                (error/throw "vector tree is not Braun balanced"))))))
   (tree/cons
    (fn (value tree)
      (if (null? tree)
          (node value null null)
          (node value
                (tree/cons (node/value tree) (node/right tree))
                (node/left tree)))))
   (tree/uncons
    (fn (tree)
      (if (null? tree)
          (error/throw "cannot pop an empty vector")
          (if (null? (node/left tree))
              (pair (node/value tree) null)
              (let ((result (tree/uncons (node/left tree))))
                (pair
                  (node/value tree)
                  (node (head result) (node/right tree) (tail result))))))))
   (tree/get
    (fn (tree index)
      (if (= index 0)
          (node/value tree)
          (if (= (int/rem index 2) 1)
              (tree/get (node/left tree) (int/div (- index 1) 2))
              (tree/get (node/right tree) (int/div (- index 2) 2))))))
   (tree/set
    (fn (tree index value)
      (if (= index 0)
          (node value (node/left tree) (node/right tree))
          (if (= (int/rem index 2) 1)
              (node (node/value tree)
                    (tree/set (node/left tree) (int/div (- index 1) 2) value)
                    (node/right tree))
              (node (node/value tree)
                    (node/left tree)
                    (tree/set (node/right tree) (int/div (- index 2) 2) value))))))
   (tree/append
    (fn (tree index value)
      (if (= index 0)
          (if (null? tree)
              (node value null null)
              (error/throw "vector tree append target is occupied"))
          (if (null? tree)
              (error/throw "vector tree append target is unavailable")
              (if (= (int/rem index 2) 1)
                  (node (node/value tree)
                        (tree/append (node/left tree) (int/div (- index 1) 2) value)
                        (node/right tree))
                  (node (node/value tree)
                        (node/left tree)
                        (tree/append (node/right tree) (int/div (- index 2) 2) value)))))))
   (tree/remove
    (fn (tree index)
      (if (= index 0)
          null
          (if (= (int/rem index 2) 1)
              (node (node/value tree)
                    (tree/remove (node/left tree) (int/div (- index 1) 2))
                    (node/right tree))
               (node (node/value tree)
                     (node/left tree)
                     (tree/remove (node/right tree) (int/div (- index 2) 2)))))))
   (parts
    (fn (value)
      (if (not (list? value))
          (error/throw "expected a vector")
          (if (not (= (list/length value) 3))
              (error/throw "expected a vector")
              (if (not (= (head value) 'vector/instance))
                  (error/throw "expected a vector")
                  (let ((length (head (tail value))))
                    (if (not (int? length))
                        (error/throw "vector length must be an integer")
                        (if (< length 0)
                            (error/throw "vector length must be non-negative")
                            value))))))))
   (length-of
    (fn (vector)
      (head (tail (parts vector)))))
   (root-of
    (fn (vector)
      (head (tail (tail (parts vector))))) )
   (require-index
    (fn (length index)
      (if (not (int? index))
          (error/throw "vector index must be an integer")
          (if (or (< index 0) (>= index length))
              (error/throw "vector index is out of range")
              index))))
   (instance
    (fn (length root)
      (if (not (int? length))
          (error/throw "vector/instance requires an integer length")
          (if (< length 0)
              (error/throw "vector/instance requires a non-negative length")
              (if (= (tree/size root) length)
                  (make length root)
                  (error/throw "vector/instance length does not match root"))))))
   (from
    (fn values
      (list/fold tail-push (make 0 null) values)))
   (is-vector
    (fn (value)
      (not
        (error?
          (error/catch
            (let ((vector (parts value)))
              (instance (length-of vector) (root-of vector))))))))
   (length
    (fn (vector)
      (length-of vector)))
   (head-value
    (fn (vector)
      (let ((length (length-of vector)))
        (if (= length 0)
            (error/throw "cannot read an empty vector")
            (node/value (root-of vector))))))
   (tail-value
    (fn (vector)
      (let ((length (length-of vector)))
        (if (= length 0)
            (error/throw "cannot read an empty vector")
            (tree/get (root-of vector) (- length 1))))))
   (head-push
    (fn (vector value)
      (let ((length (length-of vector)))
        (make (+ length 1) (tree/cons value (root-of vector))))))
   (head-pop
    (fn (vector)
      (let ((length (length-of vector)))
        (if (= length 0)
            (error/throw "cannot pop an empty vector")
            (let ((result (tree/uncons (root-of vector))))
              (pair (head result) (make (- length 1) (tail result))))))))
   (tail-push
    (fn (vector value)
      (let ((length (length-of vector)))
        (make (+ length 1) (tree/append (root-of vector) length value)))))
   (tail-pop
    (fn (vector)
      (let ((length (length-of vector)))
        (if (= length 0)
            (error/throw "cannot pop an empty vector")
            (let ((index (- length 1)))
              (pair
                (tree/get (root-of vector) index)
                (make index (tree/remove (root-of vector) index))))))))
   (get
    (fn (vector index)
      (let ((length (length-of vector)))
        (tree/get (root-of vector) (require-index length index)))) )
   (set
    (fn (vector index value)
      (let ((length (length-of vector)))
        (make length (tree/set (root-of vector) (require-index length index) value)))))
   )
  (list
    ; (vector/instance length root) -> Vector
    ; Creates a vector from a validated Braun tree root.
    ; Example: (import (vector @native:vector/v1) (vector/vector? (vector/instance 0 null))) => #t.
    (pair 'instance instance)
    ; (vector/from value...) -> Vector
    ; Creates a vector from supplied values in order.
    ; Example: (import (vector @native:vector/v1) (vector/length (vector/from 1 2 3))) => 3.
    (pair 'from from)
    ; (vector? value) -> Boolean
    ; Returns whether value is a validated vector.
    ; Example: (import (vector @native:vector/v1) (vector/vector? (vector/from))) => #t.
    (pair 'vector? is-vector)
    ; (vector/length vector) -> Integer
    ; Returns the number of values in vector.
    ; Example: (import (vector @native:vector/v1) (vector/length (vector/from 1 2))) => 2.
    (pair 'length length)
    ; (vector/head vector) -> Value
    ; Returns the first vector value.
    ; Example: (import (vector @native:vector/v1) (vector/head (vector/from 1 2))) => 1.
    (pair 'head head-value)
    ; (vector/tail vector) -> Value
    ; Returns the final vector value.
    ; Example: (import (vector @native:vector/v1) (vector/tail (vector/from 1 2))) => 2.
    (pair 'tail tail-value)
    ; (vector/head/push vector value) -> Vector
    ; Returns vector with value inserted at its front.
    ; Example: (import (vector @native:vector/v1) (vector/head (vector/head/push (vector/from 2) 1))) => 1.
    (pair 'head/push head-push)
    ; (vector/head/pop vector) -> Pair
    ; Returns the first value paired with the remaining vector.
    ; Example: (import (vector @native:vector/v1) (head (vector/head/pop (vector/from 1 2)))) => 1.
    (pair 'head/pop head-pop)
    ; (vector/tail/push vector value) -> Vector
    ; Returns vector with value appended at its end.
    ; Example: (import (vector @native:vector/v1) (vector/tail (vector/tail/push (vector/from 1) 2))) => 2.
    (pair 'tail/push tail-push)
    ; (vector/tail/pop vector) -> Pair
    ; Returns the final value paired with the remaining vector.
    ; Example: (import (vector @native:vector/v1) (head (vector/tail/pop (vector/from 1 2)))) => 2.
    (pair 'tail/pop tail-pop)
    ; (vector/get vector index) -> Value
    ; Returns the value at zero-based index.
    ; Example: (import (vector @native:vector/v1) (vector/get (vector/from 1 2 3) 1)) => 2.
    (pair 'get get)
    ; (vector/set vector index value) -> Vector
    ; Returns vector with value replacing the zero-based index.
    ; Example: (import (vector @native:vector/v1) (vector/get (vector/set (vector/from 1 2 3) 1 9) 1)) => 9.
    (pair 'set set)))
