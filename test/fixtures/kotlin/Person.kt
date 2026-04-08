package fixture

/**
 * A person with a name and age.
 */
data class Person(val name: String, val age: Int) {
    fun greet(): String = "Hello, $name"
}
