pub type Person {
  Person(name: String, age: Int)
}

pub fn new(name: String, age: Int) -> Person {
  Person(name, age)
}

pub fn greet(person: Person) -> String {
  "Hello, " <> person.name <> "!"
}

pub fn validate(name: String, age: Int) -> Result(Person, String) {
  case age >= 0 {
    True -> Ok(Person(name, age))
    False -> Error("Age must be non-negative")
  }
}
