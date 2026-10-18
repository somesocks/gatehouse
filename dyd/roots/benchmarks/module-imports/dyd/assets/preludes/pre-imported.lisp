(import/define
  (@native:weather/v1
    (list
      (pair 'forecast
        (help/document
          (fn (city)
            (ifs
              ((= city "Athens") (list (pair 'temperature_c 20) (pair 'rain #t)))
              ((= city "Berlin") (list (pair 'temperature_c 23) (pair 'rain #t)))
              ((= city "Dublin") (list (pair 'temperature_c 17) (pair 'rain #f)))
              ((= city "Helsinki") (list (pair 'temperature_c 16) (pair 'rain #t)))
              ((= city "Lisbon") (list (pair 'temperature_c 22) (pair 'rain #f)))
              ((= city "Madrid") (list (pair 'temperature_c 19) (pair 'rain #f)))
              ((= city "Oslo") (list (pair 'temperature_c 29) (pair 'rain #t)))
              ((= city "Paris") (list (pair 'temperature_c 18) (pair 'rain #t)))
              ((= city "Prague") (list (pair 'temperature_c 21) (pair 'rain #f)))
              ((= city "Rome") (list (pair 'temperature_c 24) (pair 'rain #f)))
              ((= city "Vienna") (list (pair 'temperature_c 26) (pair 'rain #t)))
              ((= city "Warsaw") (list (pair 'temperature_c 25) (pair 'rain #f)))
              (#t (error/throw "weather/forecast: unsupported city"))))
          "(forecast city) -> association list. Returns the forecast for city, which must be exactly Athens, Berlin, Dublin, Helsinki, Lisbon, Madrid, Oslo, Paris, Prague, Rome, Vienna, or Warsaw (String, case-sensitive). Symbol fields: temperature_c (Integer, degrees Celsius) and rain (Boolean, whether rain is expected). Read a field with (list/assoc/require 'temperature_c record). All unsupported inputs raise weather/forecast: unsupported city."))))
  (@native:travel/v1
    (list
      (pair 'fare
        (help/document
          (fn (origin destination)
            (assert (= origin "London") null "travel/fare: unsupported origin")
            (ifs
              ((= destination "Athens") 150)
              ((= destination "Berlin") 140)
              ((= destination "Dublin") 110)
              ((= destination "Helsinki") 55)
              ((= destination "Lisbon") 120)
              ((= destination "Madrid") 185)
              ((= destination "Oslo") 175)
              ((= destination "Paris") 90)
              ((= destination "Prague") 80)
              ((= destination "Rome") 160)
              ((= destination "Vienna") 130)
              ((= destination "Warsaw") 95)
              (#t (error/throw "travel/fare: unsupported destination"))))
          "(fare origin destination) -> Integer. Returns the fare in EUR. origin must be exactly London; destination must be exactly Athens, Berlin, Dublin, Helsinki, Lisbon, Madrid, Oslo, Paris, Prague, Rome, Vienna, or Warsaw (Strings, case-sensitive). Unsupported origins raise travel/fare: unsupported origin; unsupported destinations raise travel/fare: unsupported destination."))))
  (import/restrict
    (@native:weather/v1 @native:travel/v1)
    (let ((import/define null))
      (import
        (weather @native:weather/v1)
        (travel @native:travel/v1)
        (eval program)))))
