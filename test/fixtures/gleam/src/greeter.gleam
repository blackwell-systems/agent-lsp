import fixture/person

pub fn main() -> String {
  let p = person.new("Alice", 30)
  let result = person.validate(p)
  case result {
    Ok(validated) -> person.greet(validated)
    Error(msg) -> msg
  }
}
