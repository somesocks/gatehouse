(begin
  (import
    (vector @native:vector/v1)
    (begin
      ; Constructors preserve ordering and validate their transparent form.
      (let ((values (vector/from 1 2 3)))
        (assert
          (and
            (vector/vector? values)
            (= (vector/length values) 3)
            (= (vector/head values) 1)
            (= (vector/tail values) 3)
            (= (vector/get values 1) 2)
            (= (vector/instance 0 null) (vector/from)))
          null
          "constructors"))

      ; End operations retain order while returning persistent vectors.
      (let
        ((head-values (vector/from 2 3))
         (tail-values (vector/from 1 2))
         (head-result (vector/head/pop (vector/head/push head-values 1)))
         (tail-result (vector/tail/pop (vector/tail/push tail-values 3))))
        (assert
          (and
            (= (head head-result) 1)
            (= (tail head-result) head-values)
            (= (head tail-result) 3)
            (= (tail tail-result) tail-values))
          null
          "end operations"))

      ; Indexed updates share the original vector's value order except at the replacement.
      (let ((values (vector/from 1 2 3)))
        (assert
          (and
            (= (vector/set values 1 9) (vector/from 1 9 3))
            (= values (vector/from 1 2 3)))
          null
          "indexed update"))

      ; Vectors preserve secrecy through ordinary pair structure.
      (assert (taint/secret? (vector/from (taint/secret/mark 1))) null "secrecy")

      ; Invalid vector operations report errors.
      (assert
        (and
          (error? (error/catch (vector/head (vector/from))))
          (error? (error/catch (vector/tail/pop (vector/from))))
          (error? (error/catch (vector/get (vector/from 1) 1)))
          (error? (error/catch (vector/instance 1 null))))
        null
        "invalid operations")
      null))
  null)
