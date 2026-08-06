(begin
  (import
    (seq @native:seq/v1)
    (begin
      ; Ranges defer successors and stop before their end.
      (assert (= (seq/take 10 (seq/range 2 6)) (list 2 3 4 5)))
      (assert (= (seq/range 2 2) null))

      ; A sequence remains reusable because pulling never mutates it.
      (let ((sequence (seq/range 2 5)))
        (assert
          (and
            (= (seq/take 10 sequence) (list 2 3 4))
            (= (seq/take 10 sequence) (list 2 3 4)))))

      ; From produces an unbounded integer sequence.
      (assert (= (seq/take 4 (seq/from 2)) (list 2 3 4 5)))

      ; Null is a valid sequence value, distinct from an empty sequence.
      (assert (= (seq/take 2 (pair null (fn () null))) (list null)))

      ; Head and tail expose values while keeping the tail deferred.
      (assert
        (and
          (= (seq/head (seq/from 2)) 2)
          (= (seq/head (seq/tail (seq/from 2))) 3)))

      ; Map and filter defer their later nodes.
      (assert
        (and
          (= (seq/take 3 (seq/map (fn (n) (* n n)) (seq/from 2))) (list 4 9 16))
          (= (seq/take 3 (seq/filter (fn (n) (= (int/rem n 2) 1)) (seq/from 1))) (list 1 3 5))))

      ; Sequence exports report signatures and runnable examples.
      (assert
        (and
          (string/contains? (help seq/from) "(seq/from start) -> Sequence")
          (string/contains? (help seq/range) "(seq/range start end) -> Sequence")
          (string/contains? (help seq/head) "(seq/head sequence) -> Value")
          (string/contains? (help seq/tail) "(seq/tail sequence) -> Sequence")
          (string/contains? (help seq/take) "(seq/take count sequence) -> List")
          (string/contains? (help seq/map) "(seq/map function sequence) -> Sequence")
          (string/contains? (help seq/filter) "(seq/filter predicate sequence) -> Sequence")))

      ; Taking validates its count before consuming a sequence.
      (assert
        (and
          (string/contains? (error/value (error/catch (seq/take -1 (seq/from 0)))) "requires a non-negative count")
          (string/contains? (error/value (error/catch (seq/take "1" (seq/from 0)))) "expected an integer")))
      null))
  null)
