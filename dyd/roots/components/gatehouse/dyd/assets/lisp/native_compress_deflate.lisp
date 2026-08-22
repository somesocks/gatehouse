(import
  (seq @native:seq/v1)
  (vector @native:vector/v1)
  (let
    ((second (fn (value) (head (tail value))))
     (third (fn (value) (head (tail (tail value)))))
     (fourth (fn (value) (head (tail (tail (tail value))))))
     (fifth (fn (value) (head (tail (tail (tail (tail value)))))))
     (sixth (fn (value) (head (tail (tail (tail (tail (tail value))))))))
     (seventh (fn (value) (head (tail (tail (tail (tail (tail (tail value)))))))))
     (eighth (fn (value) (head (tail (tail (tail (tail (tail (tail (tail value))))))))))
     (reverse
      (fn (values)
        (let ((loop (fn (remaining result)
                     (if (null? remaining)
                         result
                         (loop (tail remaining) (pair (head remaining) result))))))
          (loop values null))))
     (repeat
      (fn (value count values)
        (if (= count 0)
            values
            (repeat value (- count 1) (pair value values)))))
     ; A reader retains the current immutable page and a bounded LSB-first bit buffer.
     (reader
      (fn (input page index bits count)
        (list input page index bits count)))
     (r/input (fn (value) (head value)))
     (r/page (fn (value) (second value)))
     (r/index (fn (value) (third value)))
     (r/buffer (fn (value) (fourth value)))
     (r/count (fn (value) (fifth value)))
     (r/byte
      (fn (value)
        (if (< (r/index value) (bytes/length (r/page value)))
            (pair
              (bytes/uint/le/decode (bytes/slice (r/page value) (r/index value) (+ (r/index value) 1)))
              (reader (r/input value) (r/page value) (+ (r/index value) 1) (r/buffer value) (r/count value)))
            (if (null? (r/input value))
                (error/throw "unexpected end of DEFLATE input")
                (r/byte (reader (seq/tail (r/input value)) (seq/head (r/input value)) 0 (r/buffer value) (r/count value)))))))
     (r/bits
      (fn (value width)
        (if (>= (r/count value) width)
            (pair
              (int/and (r/buffer value) (- (int/shl 1 width) 1))
              (reader
                (r/input value)
                (r/page value)
                (r/index value)
                (int/shr (r/buffer value) width)
                (- (r/count value) width)))
            (let ((next (r/byte value)))
              (r/bits
                (reader
                  (r/input (tail next))
                  (r/page (tail next))
                  (r/index (tail next))
                  (int/or (r/buffer (tail next)) (int/shl (head next) (r/count (tail next))))
                  (+ (r/count (tail next)) 8))
                width)))))
     (r/align
      (fn (value)
        (reader (r/input value) (r/page value) (r/index value) 0 0)))
     (r/u16
      (fn (value)
        (let ((low (r/byte value)))
          (let ((high (r/byte (tail low))))
            (pair (+ (head low) (* 256 (head high))) (tail high))))))
      ; Canonical Huffman codes are compiled into immutable bit-prefix tries.
      (h/node (fn (symbol left right) (list symbol left right)))
      (h/symbol (fn (node) (head node)))
      (h/left (fn (node) (second node)))
      (h/right (fn (node) (third node)))
     (h/count
      (fn (lengths index counts)
        (if (= index (vector/length lengths))
            counts
            (let ((width (vector/get lengths index)))
              (if (= width 0)
                  (h/count lengths (+ index 1) counts)
                  (h/count
                    lengths
                    (+ index 1)
                    (vector/set counts width (+ (vector/get counts width) 1))))))))
     (h/next
      (fn (counts width code result)
        (if (> width 15)
            result
            (let ((next (* 2 (+ code (vector/get counts (- width 1))))))
              (h/next counts (+ width 1) next (vector/set result width next))))))
      (h/assign
       (fn (lengths index next table)
         (if (= index (vector/length lengths))
             table
             (let ((width (vector/get lengths index)))
               (if (= width 0)
                   (h/assign lengths (+ index 1) next table)
                   (if (> width 15)
                       (error/throw "DEFLATE Huffman code length exceeds 15 bits")
                       (let ((code (vector/get next width)))
                         (h/assign
                           lengths
                           (+ index 1)
                           (vector/set next width (+ code 1))
                           (h/insert table code width index 0)))))))))
      (h/insert
       (fn (tree code width symbol depth)
         (if (= depth width)
             (if (null? tree)
                 (h/node symbol null null)
                 (error/throw "DEFLATE Huffman tree has overlapping codes"))
             (let ((node (if (null? tree) (h/node null null null) tree)))
               (if (not (null? (h/symbol node)))
                   (error/throw "DEFLATE Huffman tree has a prefix code")
                   (let ((bit (int/and (int/shr code (- width depth 1)) 1)))
                     (if (= bit 0)
                         (h/node
                           null
                           (h/insert (h/left node) code width symbol (+ depth 1))
                           (h/right node))
                         (h/node
                           null
                           (h/left node)
                           (h/insert (h/right node) code width symbol (+ depth 1))))))))))
     (h/build
      (fn (lengths)
        (let ((zeroes (vector/from 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0)))
          (let ((counts (h/count lengths 0 zeroes)))
            (h/assign lengths 0 (h/next counts 1 0 zeroes) null)))))
      (h/decode-at
       (fn (tree source depth)
         (if (null? tree)
             (error/throw "invalid DEFLATE Huffman code")
             (if (not (null? (h/symbol tree)))
                 (pair (h/symbol tree) source)
                 (if (= depth 15)
                     (error/throw "invalid DEFLATE Huffman code")
                     (let ((bit (r/bits source 1)))
                       (h/decode-at
                         (if (= (head bit) 0) (h/left tree) (h/right tree))
                         (tail bit)
                         (+ depth 1))))))))
      (h/decode (fn (tree source) (h/decode-at tree source 0)))
     (list/vector (fn (values) (fn/apply vector/from values)))
     (list/take
      (fn (count values)
        (if (= count 0)
            null
            (pair (head values) (list/take (- count 1) (tail values))))))
     (list/drop
      (fn (count values)
        (if (= count 0)
            values
            (list/drop (- count 1) (tail values)))))
     (fixed-lengths
      (fn (index values)
        (if (= index 288)
            (reverse values)
            (fixed-lengths
              (+ index 1)
              (pair
                (if (< index 144) 8 (if (< index 256) 9 (if (< index 280) 7 8)))
                values)))))
     (fixed-literal (h/build (list/vector (fixed-lengths 0 null))))
     (fixed-distance (h/build (list/vector (reverse (repeat 5 32 null)))))
     (length-base (vector/from 3 4 5 6 7 8 9 10 11 13 15 17 19 23 27 31 35 43 51 59 67 83 99 115 131 163 195 227 258))
     (length-extra (vector/from 0 0 0 0 0 0 0 0 1 1 1 1 2 2 2 2 3 3 3 3 4 4 4 4 5 5 5 5 0))
     (distance-base (vector/from 1 2 3 4 5 7 9 13 17 25 33 49 65 97 129 193 257 385 513 769 1025 1537 2049 3073 4097 6145 8193 12289 16385 24577))
     (distance-extra (vector/from 0 0 0 0 1 1 2 2 3 3 4 4 5 5 6 6 7 7 8 8 9 9 10 10 11 11 12 12 13 13))
     (read-extra
      (fn (source base width)
        (if (= width 0)
            (pair base source)
            (let ((extra (r/bits source width)))
              (pair (+ base (head extra)) (tail extra))))))
     (dynamic/lengths
      (fn (source tree remaining previous values)
        (if (= remaining 0)
            (pair source (reverse values))
            (let ((decoded (h/decode tree source)))
              (let ((code (head decoded)) (next (tail decoded)))
                (if (< code 16)
                    (dynamic/lengths next tree (- remaining 1) code (pair code values))
                    (let ((repeat-bits (if (= code 16) 2 (if (= code 17) 3 7)))
                          (repeat-base (if (= code 16) 3 (if (= code 17) 3 11)))
                          (repeat-value (if (= code 16) previous 0)))
                      (if (and (= code 16) (= previous -1))
                          (error/throw "DEFLATE code-length repeat has no previous length")
                          (let ((amount (r/bits next repeat-bits)))
                            (let ((count (+ repeat-base (head amount))))
                              (if (> count remaining)
                                  (error/throw "DEFLATE code-length repeat exceeds table")
                                  (dynamic/lengths
                                    (tail amount)
                                    tree
                                    (- remaining count)
                                    repeat-value
                                    (repeat repeat-value count values)))))))))))))
     (dynamic/read-clen
      (fn (source count order index lengths)
        (if (= index count)
            (pair source lengths)
            (let ((value (r/bits source 3)))
              (dynamic/read-clen
                (tail value)
                count
                order
                (+ index 1)
                (vector/set lengths (vector/get order index) (head value)))))))
     (dynamic/trees
      (fn (source)
        (let ((hlit (r/bits source 5)))
          (let ((hdist (r/bits (tail hlit) 5)))
            (let ((hclen (r/bits (tail hdist) 4)))
              (let ((order (vector/from 16 17 18 0 8 7 9 6 10 5 11 4 12 3 13 2 14 1 15))
                    (zeroes (vector/from 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0)))
                (let ((clens (dynamic/read-clen (tail hclen) (+ (head hclen) 4) order 0 zeroes)))
                  (let ((expanded (dynamic/lengths (head clens) (h/build (tail clens)) (+ 258 (head hlit) (head hdist)) -1 null)))
                    (let ((literal-lengths (list/vector (list/take (+ 257 (head hlit)) (tail expanded))))
                          (distance-lengths (list/vector (list/drop (+ 257 (head hlit)) (tail expanded)))))
                      (pair (head expanded) (pair (h/build literal-lengths) (h/build distance-lengths))))))))))))
      ; Decoder state: reader, history pages plus active page, write index, history length, final flag, trees, mode, mode data.
      (decoder (fn (source history write length final trees mode data) (list source history write length final trees mode data)))
      (d/source (fn (value) (head value)))
      (d/history (fn (value) (second value)))
     (d/write (fn (value) (third value)))
     (d/length (fn (value) (fourth value)))
     (d/final (fn (value) (fifth value)))
     (d/trees (fn (value) (sixth value)))
     (d/mode (fn (value) (seventh value)))
      (d/data (fn (value) (eighth value)))
      (d/with
       (fn (value source history write length final trees mode data)
         (decoder source history write length final trees mode data)))
      (history (fn (pages page) (pair pages page)))
      (history/pages (fn (value) (head value)))
      (history/page (fn (value) (tail value)))
      (d/emit
       (fn (value byte)
         (let ((length (d/length value)) (write (d/write value)))
           (let ((previous (d/history value)))
             (let ((page (vector/tail/push (history/page previous) byte)))
               (let ((complete (= (vector/length page) 256)))
                 (let ((pages (history/pages previous)))
                   (pair
                     byte
                     (d/with
                       value
                       (d/source value)
                       (history
                         (if complete
                             (if (< (vector/length pages) 128)
                                 (vector/tail/push pages page)
                                 (vector/set pages (int/div write 256) page))
                             pages)
                         (if complete (vector/from) page))
                       (int/rem (+ write 1) 32768)
                       (if (< length 32768) (+ length 1) 32768)
                       (d/final value)
                       (d/trees value)
                       (d/mode value)
                       (d/data value))))))))))
     (d/with-source
      (fn (value source)
        (d/with value source (d/history value) (d/write value) (d/length value) (d/final value) (d/trees value) (d/mode value) (d/data value))))
     (d/with-mode
      (fn (value final trees mode data)
        (d/with value (d/source value) (d/history value) (d/write value) (d/length value) final trees mode data)))
      (d/history-byte
       (fn (value distance)
         (if (or (< distance 1) (> distance (d/length value)))
             (error/throw "DEFLATE distance exceeds output history")
             (let ((index (if (< (d/length value) 32768)
                              (- (d/write value) distance)
                              (int/rem (+ (d/write value) 32768 (- distance)) 32768))))
               (let ((page-index (int/div index 256)) (page-offset (int/rem index 256)))
                 (let ((history (d/history value)))
                   (let ((page (history/page history)))
                     (vector/get
                       (if (and
                             (= page-index (int/div (d/write value) 256))
                             (< page-offset (vector/length page)))
                           page
                           (vector/get (history/pages history) page-index))
                        page-offset))))))))
     (d/block
      (fn (value)
        (let ((final (r/bits (d/source value) 1)))
          (let ((kind (r/bits (tail final) 2)))
            (if (= (head kind) 0)
                (let ((aligned (r/align (tail kind))))
                  (let ((length (r/u16 aligned)))
                    (let ((inverse (r/u16 (tail length))))
                      (if (not (= (+ (head length) (head inverse)) 65535))
                          (error/throw "DEFLATE stored block length check failed")
                          (d/with-mode (d/with-source value (tail inverse)) (head final) null 'stored (head length))))))
                (if (= (head kind) 1)
                    (d/with-mode (d/with-source value (tail kind)) (head final) (pair fixed-literal fixed-distance) 'huffman null)
                    (if (= (head kind) 2)
                        (let ((trees (dynamic/trees (tail kind))))
                          (d/with-mode (d/with-source value (head trees)) (head final) (pair (head (tail trees)) (tail (tail trees))) 'huffman null))
                        (error/throw "DEFLATE block type is reserved"))))))))
     (d/next
      (fn (value)
        (if (= (d/mode value) 'stored)
            (if (= (d/data value) 0)
                (if (= (d/final value) 1)
                    null
                    (d/next (d/with-mode value 0 null 'block null)))
                (let ((byte (r/byte (d/source value))))
                  (d/emit
                    (d/with-mode (d/with-source value (tail byte)) (d/final value) null 'stored (- (d/data value) 1))
                    (head byte))))
            (if (= (d/mode value) 'match)
                (if (= (head (d/data value)) 0)
                    (d/next (d/with-mode value (d/final value) (d/trees value) 'huffman null))
                    (let ((byte (d/history-byte value (tail (d/data value)))))
                      (d/emit
                        (d/with-mode value (d/final value) (d/trees value) 'match (pair (- (head (d/data value)) 1) (tail (d/data value))))
                        byte)))
                (if (= (d/mode value) 'huffman)
                    (let ((decoded (h/decode (head (d/trees value)) (d/source value))))
                      (let ((symbol (head decoded)) (source (tail decoded)))
                        (if (< symbol 256)
                            (d/emit (d/with-source value source) symbol)
                            (if (= symbol 256)
                                (if (= (d/final value) 1)
                                    null
                                    (d/next (d/with-mode (d/with-source value source) 0 null 'block null)))
                                (if (> symbol 285)
                                    (error/throw "invalid DEFLATE length code")
                                    (let ((length (read-extra source (vector/get length-base (- symbol 257)) (vector/get length-extra (- symbol 257)))))
                                      (let ((distance-code (h/decode (tail (d/trees value)) (tail length))))
                                        (if (> (head distance-code) 29)
                                            (error/throw "invalid DEFLATE distance code")
                                            (let ((distance (read-extra (tail distance-code) (vector/get distance-base (head distance-code)) (vector/get distance-extra (head distance-code)))))
                                              (d/next
                                                (d/with-mode
                                                  (d/with-source value (tail distance))
                                                  (d/final value)
                                                  (d/trees value)
                                                  'match
                                                  (pair (head length) (head distance)))))))))))))
                    (d/next (d/block value)))))))
      (output-page
       (fn (value count word width pieces)
         (if (= count 1024)
             (pair
               value
               (reverse
                 (if (= width 0)
                     pieces
                     (pair (bytes/uint/le/encode word width) pieces))))
             (let ((next (d/next value)))
               (if (null? next)
                   (if (and (= width 0) (null? pieces))
                       null
                       (pair
                         value
                         (reverse
                           (if (= width 0)
                               pieces
                               (pair (bytes/uint/le/encode word width) pieces)))))
                   (let ((next-word (int/or word (int/shl (head next) (* width 8)))))
                     (if (= width 6)
                         (output-page
                           (tail next)
                           (+ count 1)
                           0
                           0
                           (pair (bytes/uint/le/encode next-word 7) pieces))
                         (output-page
                           (tail next)
                           (+ count 1)
                           next-word
                           (+ width 1)
                            pieces))))))))
     (sequence
      (fn (value)
         (let ((output (output-page value 0 0 0 null)))
          (if (null? output)
              null
              (pair
                (fn/apply bytes/concat (tail output))
                (fn () (sequence (head output))))))))
      (decode
       (fn (input)
         (sequence (decoder (reader input (bytes/concat) 0 0 0) (history (vector/from) (vector/from)) 0 0 0 null 'block null))))
     )
    (list
      ; (deflate/decode compressed-pages) -> Sequence[Bytes]
      ; Lazily decodes a finite raw RFC 1951 DEFLATE byte sequence into up to 1024-byte pages.
      ; Example: (import (seq @native:seq/v1) (deflate @native:compress/deflate/v1) (bytes/utf8/decode (seq/bytes/collect (deflate/decode (seq/from (bytes/hex/decode "010300fcff616263")))))) => "abc".
      (pair 'decode decode))))
