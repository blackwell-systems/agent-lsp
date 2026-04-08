/// A person with a name and age.
pub struct Person {
    name: String,
    age: u32,
}
   
impl Person {
    pub fn new(name: &str, age: u32) -> Person {
        Person {
            name: name.to_string(),
            age,
        }
    }

    /// Returns a greeting string.
    pub fn greet(&self) -> String {
        format!("Hello, {}", self.name)
    }
}

mod greeter;

fn add(x: i32, y: i32) -> i32 {
    x + y
}

fn main() {
    let p = Person::new("Alice", 30);
    println!("{}", p.greet());
    println!("{}", add(1, 2));
}
