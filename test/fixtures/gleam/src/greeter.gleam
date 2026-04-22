import fixture/person

pub fn main() -> String {
  let p = person.new("Alice", 30)
  person.greet(p)
}

pub fn get_name() -> String {
  let p = person.new("Bob", 25)
  person.name(p)
}
