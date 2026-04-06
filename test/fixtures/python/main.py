def add(x: int, y: int) -> int:
    return x + y

class Person:
    def __init__(self, name: str, age: int) -> None:
        self.name = name
        self.age = age

    def greet(self) -> str:
        return f"Hello, {self.name}"

def main() -> None:
    p = Person("Alice", 30)
    print(p.greet())
    result = add(1, 2)
    print(result)

if __name__ == "__main__":
    main()
