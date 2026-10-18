(begin
  (assert (= (help/env "weather/") (list 'weather/forecast)))
  (assert (= (help/env "travel/") (list 'travel/fare)))
  (assert (= (list/assoc/require 'temperature_c (weather/forecast "Rome")) 24))
  (assert (= (travel/fare "London" "Lisbon") 120))
  (assert (string/contains? (help 'weather/forecast) "temperature_c"))
  (assert (string/contains? (help 'travel/fare) "EUR"))
  null)
