(import
  (crc32 @native:crypto/digest/crc32/v1)
  (deflate @native:compress/deflate/v1)
  (seq @native:seq/v1)
  (let
    ((second (fn (value) (head (tail value))))
     (third (fn (value) (head (tail (tail value)))))
     (fourth (fn (value) (head (tail (tail (tail value))))))
     (fifth (fn (value) (head (tail (tail (tail (tail value)))))))
     (reverse
      (fn (values)
        (let ((loop (fn (remaining result)
                     (if (null? remaining)
                         result
                         (loop (tail remaining) (pair (head remaining) result))))))
          (loop values null))))
     (u16 (fn (value offset) (bytes/uint/le/decode (bytes/slice value offset (+ offset 2)))))
     (u32 (fn (value offset) (bytes/uint/le/decode (bytes/slice value offset (+ offset 4)))))
     ; ZIP keeps its source cursor private until another format proves the abstraction reusable.
     (cursor/make (fn (size source offset page page-start) (list size source offset page page-start)))
     (cursor/size (fn (cursor) (head cursor)))
     (cursor/source (fn (cursor) (second cursor)))
     (cursor/offset (fn (cursor) (third cursor)))
     (cursor/page (fn (cursor) (fourth cursor)))
     (cursor/page-start (fn (cursor) (fifth cursor)))
     (cursor/seek
      (fn (cursor offset)
        (if (or (< offset 0) (> offset (cursor/size cursor)))
            (error/throw "ZIP cursor offset is out of range")
            (cursor/make (cursor/size cursor) (cursor/source cursor) offset (cursor/page cursor) (cursor/page-start cursor)))))
     (cursor/page-at
      (fn (cursor offset)
        (let ((page (cursor/page cursor)) (page-start (cursor/page-start cursor)))
          (if (and (< 0 (bytes/length page)) (<= page-start offset) (< offset (+ page-start (bytes/length page))))
              (pair page page-start)
              (if (= offset (cursor/size cursor))
                  (pair (bytes/concat) offset)
                  (let ((length (int/min 65536 (- (cursor/size cursor) offset))))
                    (let ((next ((cursor/source cursor) offset length)))
                      (if (not (= (bytes/length next) length))
                          (error/throw "ZIP source returned a short read")
                          (pair next offset)))))))))
     (cursor/read-pieces
      (fn (cursor offset remaining pieces page page-start)
        (if (= remaining 0)
            (pair
              (fn/apply bytes/concat (reverse pieces))
              (cursor/make (cursor/size cursor) (cursor/source cursor) offset page page-start))
            (let ((loaded (cursor/page-at cursor offset)))
              (let ((next-page (head loaded)) (next-start (tail loaded)))
                (let ((index (- offset next-start)))
                  (let ((length (int/min remaining (- (bytes/length next-page) index))))
                    (if (= length 0)
                        (error/throw "ZIP source returned an empty read")
                        (cursor/read-pieces
                          cursor
                          (+ offset length)
                          (- remaining length)
                          (pair (bytes/slice next-page index (+ index length)) pieces)
                          next-page
                          next-start)))))))))
     (cursor/read
      (fn (cursor length)
        (if (not (int? length))
            (error/throw "ZIP cursor length must be an integer")
            (if (or (< length 0) (> length (- (cursor/size cursor) (cursor/offset cursor))))
                (error/throw "ZIP cursor read is out of range")
                (cursor/read-pieces cursor (cursor/offset cursor) length null (cursor/page cursor) (cursor/page-start cursor))))))
     (cursor/sequence
      (fn (cursor length)
        (if (not (int? length))
            (error/throw "ZIP cursor length must be an integer")
            (if (or (< length 0) (> length (- (cursor/size cursor) (cursor/offset cursor))))
                (error/throw "ZIP cursor sequence is out of range")
                (if (= length 0)
                    null
                    (let ((next (cursor/read cursor (int/min 65536 length))))
                      (pair
                        (head next)
                        (fn () (cursor/sequence (tail next) (- length (bytes/length (head next))))))))))))
     (local-signature (bytes/hex/decode "504b0304"))
     (central-signature (bytes/hex/decode "504b0102"))
     (end-signature (bytes/hex/decode "504b0506"))
     (signature?
      (fn (value offset signature)
        (and
          (<= (+ offset (bytes/length signature)) (bytes/length value))
          (= (bytes/slice value offset (+ offset (bytes/length signature))) signature))))
     (entry/make (fn (name source) (list 'zip/entry name source)))
     (entry/parts
      (fn (value)
        (if (not (list? value))
            (error/throw "expected a ZIP entry")
            (if (not (= (list/length value) 3))
                (error/throw "expected a ZIP entry")
                (if (not (= (head value) 'zip/entry))
                    (error/throw "expected a ZIP entry")
                    (if (not (string? (second value)))
                        (error/throw "ZIP entry name must be a string")
                        value))))))
     (entry/name (fn (value) (second (entry/parts value))))
     (entry/source (fn (value) (third (entry/parts value))))
     (entry
      (fn (name data)
        (if (not (string? name))
            (error/throw "zip/entry requires a string name")
            (entry/make name data))))
     (entry?
      (fn (value)
        (not (error? (error/catch (entry/parts value))))))
     (entry/data
      (fn (value)
        ((entry/source value))))
     (eocd/find
      (fn (value index)
        (if (< index 0)
            (error/throw "ZIP end of central directory record is missing")
            (if (and
                  (signature? value index end-signature)
                  (= (+ index 22 (u16 value (+ index 20))) (bytes/length value)))
                index
                (eocd/find value (- index 1))))))
     (unsupported?
      (fn (flags method compressed-size uncompressed-size local-offset disk)
        (or
          (not (= (int/and flags 8257) 0))
          (not (= (int/and flags 64) 0))
          (and (not (= method 0)) (not (= method 8)))
          (= compressed-size 4294967295)
          (= uncompressed-size 4294967295)
          (= local-offset 4294967295)
          (= disk 65535))))
     (validated-data
      (fn (pages crc size expected-crc expected-size)
        (if (null? pages)
            (if (and (= crc expected-crc) (= size expected-size))
                null
                (error/throw "ZIP entry data does not match its central directory"))
            (let ((page (seq/head pages)))
              (pair
                page
                (fn ()
                  (validated-data
                    (seq/tail pages)
                    (crc32/update crc page)
                    (+ size (bytes/length page))
                    expected-crc
                    expected-size)))))))
     (entry/data-sequence
      (fn (cursor local-offset flags method expected-crc compressed-size expected-size)
        (let ((fixed-read (cursor/read (cursor/seek cursor local-offset) 30)))
          (let ((fixed (head fixed-read)))
            (if (not (signature? fixed 0 local-signature))
                (error/throw "ZIP local file header is missing")
                (let ((local-flags (u16 fixed 6)) (local-method (u16 fixed 8)) (local-crc (u32 fixed 14)) (local-compressed-size (u32 fixed 18)) (local-size (u32 fixed 22)) (name-length (u16 fixed 26)) (extra-length (u16 fixed 28)))
                  (if (or (not (= local-flags flags)) (not (= local-method method)))
                      (error/throw "ZIP local file header does not match its central directory")
                      (if (and (= (int/and flags 8) 0) (or (not (= local-crc expected-crc)) (not (= local-compressed-size compressed-size)) (not (= local-size expected-size))))
                          (error/throw "ZIP local file header sizes do not match its central directory")
                          (let ((data-offset (+ local-offset 30 name-length extra-length)))
                            (if (> (+ data-offset compressed-size) (cursor/size cursor))
                                (error/throw "ZIP entry data exceeds the source size")
                                (let ((compressed (cursor/sequence (cursor/seek cursor data-offset) compressed-size)))
                                  (validated-data
                                    (if (= method 0) compressed (deflate/decode compressed))
                                    0
                                    0
                                    expected-crc
                                    expected-size))))))))))))
     (entries
      (fn (cursor offset end remaining)
        (if (= remaining 0)
            (if (= offset end)
                null
                (error/throw "ZIP central directory size does not match its entries"))
            (if (> (+ offset 46) end)
                (error/throw "ZIP central directory header is truncated")
                (let ((fixed-read (cursor/read (cursor/seek cursor offset) 46)))
                  (let ((fixed (head fixed-read)))
                    (if (not (signature? fixed 0 central-signature))
                        (error/throw "ZIP central directory header is missing")
                        (let ((flags (u16 fixed 8)) (method (u16 fixed 10)) (crc (u32 fixed 16)) (compressed-size (u32 fixed 20)) (size (u32 fixed 24)) (name-length (u16 fixed 28)) (extra-length (u16 fixed 30)) (comment-length (u16 fixed 32)) (disk (u16 fixed 34)) (local-offset (u32 fixed 42)))
                          (let ((variable-length (+ name-length extra-length comment-length)) (next-offset (+ offset 46 name-length extra-length comment-length)))
                            (if (> next-offset end)
                                (error/throw "ZIP central directory entry is truncated")
                                (if (unsupported? flags method compressed-size size local-offset disk)
                                    (error/throw "ZIP entry uses an unsupported feature")
                                    (let ((variable-read (cursor/read (tail fixed-read) variable-length)))
                                      (let ((name (bytes/utf8/decode (bytes/slice (head variable-read) 0 name-length))))
                                        (pair
                                          (entry
                                            name
                                            (fn () (entry/data-sequence cursor local-offset flags method crc compressed-size size)))
                                          (fn () (entries cursor next-offset end (- remaining 1)))))))))))))))))
     (reader
      (fn (size read)
        (if (not (int? size))
            (error/throw "zip/reader requires an integer size")
            (if (< size 22)
                (error/throw "ZIP source is too short")
                (let ((cursor (cursor/make size read 0 (bytes/concat) 0)) (tail-length (int/min size 65557)))
                  (let ((tail-read (cursor/read (cursor/seek cursor (- size tail-length)) tail-length)))
                    (let ((tail (head tail-read)) (tail-offset (- size tail-length)))
                      (let ((index (eocd/find tail (- (bytes/length tail) 22))))
                        (let ((disk (u16 tail (+ index 4)))
                              (central-disk (u16 tail (+ index 6)))
                              (disk-entries (u16 tail (+ index 8)))
                              (entry-count (u16 tail (+ index 10)))
                              (central-size (u32 tail (+ index 12)))
                              (central-offset (u32 tail (+ index 16))))
                          (if (or (not (= disk 0)) (not (= central-disk 0)) (not (= disk-entries entry-count)) (= entry-count 65535) (= central-size 4294967295) (= central-offset 4294967295))
                              (error/throw "ZIP archive uses an unsupported multi-disk or ZIP64 layout")
                              (if (> (+ central-offset central-size) (+ tail-offset index))
                                  (error/throw "ZIP central directory exceeds its end record")
                                  (entries cursor central-offset (+ central-offset central-size) entry-count))))))))))))
     )
    (list
      ; (zip/reader size read) -> Sequence[ZipEntry]
      ; Parses a classic single-disk ZIP source into lazy ZIP entries. read receives offset and length and must return exactly that many Bytes. Entry names must be valid UTF-8.
      ; Example: (import (zip @native:archive/zip/v1) (null? (zip/reader 22 (fn (offset length) (bytes/hex/decode "504b0506000000000000000000000000000000000000"))))) => #t.
      (pair 'reader reader)
      ; (zip/entry name data) -> ZipEntry
      ; Creates a ZIP entry from a string name and a zero-argument data producer returning Sequence[Bytes].
      ; Example: (import (zip @native:archive/zip/v1) (zip/entry? (zip/entry "empty" (fn () null)))) => #t.
      (pair 'entry entry)
      ; (zip/entry? value) -> Boolean
      ; Returns whether value is a validated ZIP entry.
      ; Example: (import (zip @native:archive/zip/v1) (zip/entry? null)) => #f.
      (pair 'entry? entry?)
      ; (zip/entry/name entry) -> String
      ; Returns a ZIP entry name.
      ; Example: (import (zip @native:archive/zip/v1) (zip/entry/name (zip/entry "empty" (fn () null)))) => "empty".
      (pair 'entry/name entry/name)
      ; (zip/entry/data entry) -> Sequence[Bytes]
      ; Invokes the entry data producer and returns its lazy byte sequence.
      ; Example: (import (zip @native:archive/zip/v1) (zip/entry/data (zip/entry "empty" (fn () null)))) => null.
      (pair 'entry/data entry/data))))
