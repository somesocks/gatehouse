(begin
  (assert (= '(one 2) (list 'one 2)))
  (assert (= (pair 'one (pair 2 null)) (list 'one 2)))
  (assert (= (head (pair 1 2)) 1))
  (assert (= (tail (pair 1 2)) 2))
  (assert (= (tail (list 1 2 3)) (list 2 3)))
  (assert (pair? (pair 1 2)))
  (assert (pair? (list 1 2)))
  (assert (not (pair? null)))
  (assert (not (and (pair? null) (head null))))
  (assert (list? null))
  (assert (list? (list 1 2)))
  (assert (not (list? (pair 1 2))))
  (assert (= (list/length null) 0))
  (assert (= (list/length (list 1 2 3)) 3))
  (assert (= (list/concat) null))
  (assert (= (list/concat (list 1 2) null (list 3)) (list 1 2 3)))
  (assert (= (list/fold + 0 (list 1 2 3)) 6))
  (assert
    (= (list/fold
         (fn (values value) (pair value values))
         null
         (list 1 2 3))
       (list 3 2 1)))
  (assert (= (list/map (fn (x) (* x x)) (list 1 2 3)) (list 1 4 9)))
  (assert (= (list/filter (fn (x) (< x 3)) (list 1 2 3)) (list 1 2)))
  (assert (= (list/filter (fn (x) #f) (list 1 2 3)) null))
  (assert (= 1 1))
  (assert (not (= 1 "1")))
  (assert (= 'name 'name))
  (assert (not (= 'name "name")))
  (assert (= null null))
  (assert (= '(1 (2 3)) (list 1 (list 2 3))))
  (assert (not (= '(1 2) '(1 3))))
  (assert
    (string/contains?
      (error/value (error/catch (list/length (pair 1 2))))
      "expected a proper list"))
  (assert
    (string/contains?
      (error/value (error/catch (list/concat (pair 1 2))))
      "expected a proper list"))
  (assert
    (string/contains?
      (error/value (error/catch (list/fold + 0 (pair 1 2))))
      "expected a proper list"))
  (assert
    (string/contains?
      (error/value (error/catch (list/fold + 0)))
      "requires a function, initial value, and list"))
  (assert
    (string/contains?
      (error/value (error/catch (list/filter (fn (x) x) (list 1))))
      "expected a Boolean"))
  (assert
    (string/contains?
      (error/value (error/catch (map (fn (x) x) null)))
      "unknown binding"))
  (assert
    (string/contains?
      (error/value (error/catch (filter (fn (x) #t) null)))
      "unknown binding"))
  null)
