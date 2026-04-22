pub type Person {
  Person(name: String, age: Int)
}

pub fn new(name: String, age: Int) -> Person {
  Person(name:, age:)
}

pub fn greet(person: Person) -> String {
  "Hello, " <> person.name
}

pub fn validate(person: Person) -> Result(Person, String) {
  case person.age >= 0 {
    True -> Ok(person)
    False -> Error("Invalid age")
  }
}
