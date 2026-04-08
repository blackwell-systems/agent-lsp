package fixture

@main def run(): Unit =
  val person = Person("Alice", 30)
  println(person.greet())
  val greeter = Greeter()
  println(greeter.greet(person))
