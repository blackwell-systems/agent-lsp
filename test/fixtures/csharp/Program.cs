using CSharpFixture;

var person = new Person("Alice", 30);
Console.WriteLine(person.Greet());

var greeter = new Greeter();
Console.WriteLine(greeter.Greet(person));
