(ns fixture.core)

(defn greet [name]
  (str "Hello, " name))

(defn main []
  (println (greet "Alice")))
