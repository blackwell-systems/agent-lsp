package fixture

fun main() {
    val person = Person("Alice", 30)
    println(person.greet())
    val greeter = Greeter()
    println(greeter.greet(person))
}
