(let
  ((from
    (fn (start)
      (pair start (fn () (from (+ start 1))))))
   (range
    (fn (start end)
      (if (>= start end)
          null
          (pair start (fn () (range (+ start 1) end))))))
   (sequence-head
    (fn (sequence) (head sequence)))
   (sequence-tail
    (fn (sequence) ((tail sequence))))
   (take
    (fn (count sequence)
      (if (< count 0)
          (error/throw "seq/take requires a non-negative count")
          (if (= count 0)
              null
              (if (null? sequence)
                  null
                  (pair (head sequence) (take (- count 1) (sequence-tail sequence))))))))
   (map
    (fn (function sequence)
      (if (null? sequence)
          null
          (pair (function (head sequence))
                (fn () (map function (sequence-tail sequence)))))))
   (filter
    (fn (predicate sequence)
      (if (null? sequence)
          null
          (if (predicate (head sequence))
              (pair (head sequence)
                    (fn () (filter predicate (sequence-tail sequence))))
              (filter predicate (sequence-tail sequence)))))))
  (list
    ; (seq/from start) -> Sequence
    ; Returns integers from start, with each successor deferred.
    ; Example: (import (seq @native:seq/v1) (seq/take 4 (seq/from 2))) => (2 3 4 5).
    (pair 'from from)
    ; (seq/range start end) -> Sequence
    ; Returns integers from inclusive start to exclusive end, with each successor deferred.
    ; Example: (import (seq @native:seq/v1) (seq/take 10 (seq/range 2 6))) => (2 3 4 5).
    (pair 'range range)
    ; (seq/head sequence) -> Value
    ; Returns the value at a nonempty sequence node.
    ; Example: (import (seq @native:seq/v1) (seq/head (seq/from 2))) => 2.
    (pair 'head sequence-head)
    ; (seq/tail sequence) -> Sequence
    ; Returns the next sequence by invoking a nonempty node's tail thunk.
    ; Example: (import (seq @native:seq/v1) (seq/head (seq/tail (seq/from 2)))) => 3.
    (pair 'tail sequence-tail)
    ; (seq/take count sequence) -> List
    ; Returns up to count sequence values as a proper list.
    ; Example: (import (seq @native:seq/v1) (seq/take 3 (seq/from 10))) => (10 11 12).
    (pair 'take take)
    ; (seq/map function sequence) -> Sequence
    ; Returns a sequence that applies function to each source value as nodes are produced.
    ; Example: (import (seq @native:seq/v1) (seq/take 3 (seq/map (fn (n) (* n n)) (seq/from 2)))) => (4 9 16).
    (pair 'map map)
    ; (seq/filter predicate sequence) -> Sequence
    ; Returns a sequence containing source values for which predicate returns true.
    ; Example: (import (seq @native:seq/v1) (seq/take 3 (seq/filter (fn (n) (= (int/rem n 2) 1)) (seq/from 1)))) => (1 3 5).
    (pair 'filter filter)
    ; (seq/bytes/split bytes block-size) -> Sequence
    ; Returns Bytes blocks of block-size bytes, with only the final block possibly shorter.
    ; Example: (import (seq @native:seq/v1) (list/map bytes/hex/encode (seq/take 10 (seq/bytes/split (bytes/hex/decode "aabbccddeeff") 2)))) => ("aabb" "ccdd" "eeff").
    (pair 'bytes/split null)
    ; (seq/bytes/collect sequence) -> Bytes
    ; Concatenates the Bytes values in a finite sequence.
    ; Example: (import (seq @native:seq/v1) (bytes/hex/encode (seq/bytes/collect (seq/bytes/split (bytes/hex/decode "aabb") 1)))) => "aabb".
    (pair 'bytes/collect null)))
