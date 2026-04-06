from main import Person

class Greeter:
    def __init__(self, person: Person) -> None:
        self.person = person

    def say_hello(self) -> str:
        return f"Greeter says: {self.person.greet()}"
