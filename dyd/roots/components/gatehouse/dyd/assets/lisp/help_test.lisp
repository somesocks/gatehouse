; Root leading help.
(begin
  (let
    ((leading
       ; Leading first line.
       ; Leading second line.
       1)
     (trailing
       2 ; Trailing first line.
       ; Trailing second line.
       )
     (separated
       ; First block.

       ; Second block.
       3)
     (nested
       (list
         ; First list value.
         4
         5 ; Second list value.
         ))
     (documented
       ; replacement-marker
       +))
    (begin
      (assert (= (help 'leading) "Leading first line.\nLeading second line."))
      (assert (= (help 'trailing) "Trailing first line.\nTrailing second line."))
      (assert (= (help 'separated) "First block.\nSecond block."))
      (assert (= (help (head nested)) "First list value."))
      (assert (= (help (head (tail nested))) "Second list value."))
      (assert (= (help 'documented) "replacement-marker"))
      (assert (= (help/search "replacement-marker") (list 'documented)))))
  null) ; Root trailing help.
