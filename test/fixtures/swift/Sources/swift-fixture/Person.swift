/// A person with a name and age.
public struct Person {
    public let name: String
    public let age: Int

    public init(name: String, age: Int) {
        self.name = name
        self.age = age
    }

    public func greet() -> String {
        return "Hello, \(name)"
    }
}
