import fixture/person

pub fn main() -> String {
  let p = person.new("Alice", 30)
  person.greet(p)
}
