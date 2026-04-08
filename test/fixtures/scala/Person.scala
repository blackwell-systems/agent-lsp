package fixture

/** A person with a name and age. */
case class Person(name: String, age: Int):
  def greet(): String = s"Hello, $name"
