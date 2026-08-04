(begin
  (assert (= '(one 2) (list 'one 2)) null)
  (assert (= (pair 'one (pair 2 null)) (list 'one 2)) null)
  (assert (= (head (pair 1 2)) 1) null)
  (assert (= (tail (pair 1 2)) 2) null)
  (assert (= (tail (list 1 2 3)) (list 2 3)) null)
  (assert (pair? (pair 1 2)) null)
  (assert (pair? (list 1 2)) null)
  (assert (not (pair? null)) null)
  (assert (not (and (pair? null) (head null))) null)
  (assert (list? null) null)
  (assert (list? (list 1 2)) null)
  (assert (not (list? (pair 1 2))) null)
  (assert (= (list/length null) 0) null)
  (assert (= (list/length (list 1 2 3)) 3) null)
  (assert (= (list/concat) null) null)
  (assert (= (list/concat (list 1 2) null (list 3)) (list 1 2 3)) null)
  (assert (= (list/fold + 0 (list 1 2 3)) 6) null)
  (assert
    (= (list/fold
         (fn (values value) (pair value values))
         null
         (list 1 2 3))
       (list 3 2 1))
    null)
  (assert (= (list/map (fn (x) (* x x)) (list 1 2 3)) (list 1 4 9)) null)
  (assert (= (list/filter (fn (x) (< x 3)) (list 1 2 3)) (list 1 2)) null)
  (assert (= (list/filter (fn (x) #f) (list 1 2 3)) null) null)
  (assert (= 1 1) null)
  (assert (not (= 1 "1")) null)
  (assert (= 'name 'name) null)
  (assert (not (= 'name "name")) null)
  (assert (= null null) null)
  (assert (= '(1 (2 3)) (list 1 (list 2 3))) null)
  (assert (not (= '(1 2) '(1 3))) null)
  (assert
    (string/contains?
      (error/value (error/catch (list/length (pair 1 2))))
      "expected a proper list")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (list/concat (pair 1 2))))
      "expected a proper list")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (list/fold + 0 (pair 1 2))))
      "expected a proper list")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (list/fold + 0)))
      "requires a function, initial value, and list")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (list/filter (fn (x) x) (list 1))))
      "expected a Boolean")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (map (fn (x) x) null)))
      "unknown binding")
    null)
  (assert
    (string/contains?
      (error/value (error/catch (filter (fn (x) #t) null)))
      "unknown binding")
    null)
  null)
