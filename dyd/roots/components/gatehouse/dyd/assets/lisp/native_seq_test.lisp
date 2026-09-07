(begin
  (import
    (seq @native:seq/v1)
    (begin
      ; Ranges defer successors and stop before their end when supplied.
      (assert (= (seq/take 10 (seq/range 2 6)) (list 2 3 4 5)))
      (assert (= (seq/range 2 2) null))

      ; Omitting the end produces an unbounded range.
      (assert (= (seq/take 4 (seq/range 2)) (list 2 3 4 5)))

      ; A sequence remains reusable because pulling never mutates it.
      (let ((sequence (seq/range 2 5)))
        (assert
          (and
            (= (seq/take 10 sequence) (list 2 3 4))
            (= (seq/take 10 sequence) (list 2 3 4)))))

      ; From constructs a finite sequence from its supplied values.
      (assert (= (seq/take 10 (seq/from 2 3 4 5)) (list 2 3 4 5)))
      (assert (= (seq/from) null))

      ; Null is a valid sequence value, distinct from an empty sequence.
      (assert (= (seq/take 2 (seq/from null)) (list null)))

      ; Head and tail expose values while keeping the tail deferred.
      (assert
        (and
          (= (seq/head (seq/range 2)) 2)
          (= (seq/head (seq/tail (seq/range 2))) 3)))

      ; Map and filter defer their later nodes.
      (assert
        (and
          (= (seq/take 3 (seq/map (fn (n) (* n n)) (seq/range 2))) (list 4 9 16))
          (= (seq/take 3 (seq/filter (fn (n) (= (int/rem n 2) 1)) (seq/range 1))) (list 1 3 5))))

       ; Byte splitting emits fixed-size Bytes blocks and a final short block.
       (assert
         (= (list/map bytes/hex/encode
              (seq/take 10 (seq/bytes/split (bytes/hex/decode "aabbccddee") 2)))
            (list "aabb" "ccdd" "ee")))

       ; Splitting empty Bytes produces an empty sequence.
       (assert
         (= (seq/bytes/split (bytes/concat) 16) null))

       ; Collecting split blocks restores the original Bytes.
       (assert
         (= (bytes/hex/encode
              (seq/bytes/collect
                (seq/bytes/split (bytes/hex/decode "aabbccddee") 2)))
            "aabbccddee"))

        ; Collecting succeeds when the result exactly reaches its maximum.
       (assert
         (= (bytes/hex/encode
              (seq/bytes/collect
                (seq/bytes/split (bytes/hex/decode "aabbccddee") 2)
                5))
            "aabbccddee"))

        ; Collecting stops before appending a block that exceeds its maximum.
        (assert
          (string/contains?
            (error/value
              (error/catch
                (seq/bytes/collect
                  (seq/bytes/split (bytes/hex/decode "aabbccddee") 2)
                  4)))
            "exceeds maximum bytes"))

        ; A rejected block does not force its tail.
        (assert
          (string/contains?
            (error/value
              (error/catch
                (seq/bytes/collect
                  (pair (bytes/hex/decode "aa") (fn () (error/throw "tail was forced")))
                  0)))
            "exceeds maximum bytes"))

        ; Collecting an empty sequence produces empty Bytes.
       (assert
         (= (bytes/length (seq/bytes/collect null 0)) 0))

       ; Splitting preserves secrecy in deferred blocks.
       (let ((sequence (seq/bytes/split (taint/secret/mark (bytes/hex/decode "aabb")) 1)))
         (assert
           (and
             (taint/secret? (seq/head sequence))
             (taint/secret? (seq/head (seq/tail sequence))))))

       ; Collecting preserves secrecy that appears after the first block.
       (assert
          (taint/secret?
           (seq/bytes/collect
             (pair
               (bytes/hex/decode "aa")
               (fn ()
                 (pair (taint/secret/mark (bytes/hex/decode "bb")) (fn () null))))
             2)))

       ; Bootstrapped sequence definitions do not capture caller shadowing.
       (let ((+ (fn (left right) 0)))
         (assert (= (seq/take 3 (seq/range 1)) (list 1 2 3))))

      ; Sequence exports report signatures and runnable examples.
      (assert
        (and
          (string/contains? (help seq/from) "(seq/from value...) -> Sequence")
          (string/contains? (help seq/range) "(seq/range start [end]) -> Sequence")
          (string/contains? (help seq/head) "(seq/head sequence) -> Value")
          (string/contains? (help seq/tail) "(seq/tail sequence) -> Sequence")
          (string/contains? (help seq/take) "(seq/take count sequence) -> List")
          (string/contains? (help seq/map) "(seq/map function sequence) -> Sequence")
          (string/contains? (help seq/filter) "(seq/filter predicate sequence) -> Sequence")
          (string/contains? (help seq/bytes/split) "(seq/bytes/split bytes block-size) -> Sequence")
          (string/contains? (help seq/bytes/collect) "(seq/bytes/collect sequence [maximum-bytes]) -> Bytes")))

      ; Taking validates its count before consuming a sequence.
      (assert
        (and
          (string/contains? (error/value (error/catch (seq/take -1 (seq/from 0)))) "requires a non-negative count")
          (string/contains? (error/value (error/catch (seq/take "1" (seq/from 0)))) "expected an integer")
          (string/contains? (error/value (error/catch (seq/range 0 1 2))) "requires a start and optional end")
          (string/contains? (error/value (error/catch (seq/bytes/split (bytes/concat) 0))) "requires a positive block size")
          (string/contains? (error/value (error/catch (seq/bytes/collect (pair 1 (fn () null))))) "expected Bytes")
          (string/contains? (error/value (error/catch (seq/bytes/collect null -1))) "requires a non-negative maximum bytes")
          (string/contains? (error/value (error/catch (seq/bytes/collect null "1"))) "expected an integer")))
      null))
  null)
