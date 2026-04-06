use crate::Person;

pub struct Greeter {
    person: Person,
}

impl Greeter {
    pub fn new(person: Person) -> Self {
        Greeter { person }
    }

    pub fn say_hello(&self) -> String {
        format!("Greeter says: {}", self.person.greet())
    }
}
