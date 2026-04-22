import person

pub fn create_and_greet() -> String {
  let p = person.new("Alice", 30)
  person.greet(p)
}

pub fn create_validated(name: String, age: Int) -> Result(String, String) {
  case person.validate(name, age) {
    Ok(p) -> Ok(person.greet(p))
    Error(msg) -> Error(msg)
  }
}
