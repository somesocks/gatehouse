(begin
  ; Quote expands to a proper list value.
  (assert (= '(one 2) (list 'one 2)))

  ; Nested pairs represent the same proper list.
  (assert (= (pair 'one (pair 2 null)) (list 'one 2)))

  ; Head returns the first pair value.
  (assert (= (head (pair 1 2)) 1))

  ; Tail returns the second pair value.
  (assert (= (tail (pair 1 2)) 2))

  ; Tail of a proper list returns its remaining list.
  (assert (= (tail (list 1 2 3)) (list 2 3)))

  ; A constructed pair satisfies the pair predicate.
  (assert (pair? (pair 1 2)))

  ; A proper list is made from pairs.
  (assert (pair? (list 1 2)))

  ; Null is not a pair.
  (assert (not (pair? null)))

  ; Short-circuiting prevents head from receiving null.
  (assert (not (and (pair? null) (head null))))

  ; Null is a proper list.
  (assert (list? null))

  ; A constructed list satisfies the list predicate.
  (assert (list? (list 1 2)))

  ; An improper pair is not a proper list.
  (assert (not (list? (pair 1 2))))

  ; The empty list has length zero.
  (assert (= (list/length null) 0))

  ; List length counts every element.
  (assert (= (list/length (list 1 2 3)) 3))

  ; Concatenating no lists produces null.
  (assert (= (list/concat) null))

  ; Concatenation flattens supplied proper lists in order.
  (assert (= (list/concat (list 1 2) null (list 3)) (list 1 2 3)))

  ; Folding with addition accumulates from left to right.
  (assert (= (list/fold + 0 (list 1 2 3)) 6))

  ; Folding can construct a reversed list.
  (assert
    (= (list/fold
         (fn (values value) (pair value values))
         null
         (list 1 2 3))
       (list 3 2 1)))

  ; Mapping applies a function to every element.
  (assert (= (list/map (fn (x) (* x x)) (list 1 2 3)) (list 1 4 9)))

  ; Filtering retains values accepted by the predicate.
  (assert (= (list/filter (fn (x) (< x 3)) (list 1 2 3)) (list 1 2)))

  ; Filtering can remove every value.
  (assert (= (list/filter (fn (x) #f) (list 1 2 3)) null))

  ; Equal integers compare equal.
  (assert (= 1 1))

  ; Values with different kinds are not equal.
  (assert (not (= 1 "1")))

  ; Equal symbols compare equal.
  (assert (= 'name 'name))

  ; Symbols and strings are not equal.
  (assert (not (= 'name "name")))

  ; Null compares equal to itself.
  (assert (= null null))

  ; Equality traverses nested proper lists.
  (assert (= '(1 (2 3)) (list 1 (list 2 3))))

  ; Equality distinguishes nested list contents.
  (assert (not (= '(1 2) '(1 3))))

  ; List length rejects an improper pair.
  (assert
    (string/contains?
      (error/value (error/catch (list/length (pair 1 2))))
      "expected a proper list"))

  ; List concatenation rejects an improper pair.
  (assert
    (string/contains?
      (error/value (error/catch (list/concat (pair 1 2))))
      "expected a proper list"))

  ; List folding rejects an improper pair.
  (assert
    (string/contains?
      (error/value (error/catch (list/fold + 0 (pair 1 2))))
      "expected a proper list"))

  ; List folding requires a function, initial value, and list.
  (assert
    (string/contains?
      (error/value (error/catch (list/fold + 0)))
      "requires a function, initial value, and list"))

  ; List filtering requires a Boolean predicate result.
  (assert
    (string/contains?
      (error/value (error/catch (list/filter (fn (x) x) (list 1))))
      "expected a Boolean"))

  ; The legacy map name is unavailable.
  (assert
    (string/contains?
      (error/value (error/catch (map (fn (x) x) null)))
      "unknown binding"))

  ; The legacy filter name is unavailable.
  (assert
    (string/contains?
      (error/value (error/catch (filter (fn (x) #t) null)))
      "unknown binding"))
  null)
