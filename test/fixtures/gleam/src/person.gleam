pub type Person {
  Person(name: String, age: Int)
}

pub fn new(name: String, age: Int) -> Person {
  Person(name:, age:)
}

pub fn greet(person: Person) -> String {
  "Hello, " <> person.name
}

pub fn name(person: Person) -> String {
  person.name
}
