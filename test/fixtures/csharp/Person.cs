namespace CSharpFixture;

/// <summary>A person with a name and age.</summary>
public class Person
{
    public string Name { get; set; }
    public int Age { get; set; }

    public Person(string name, int age)
    {
        Name = name;
        Age = age;
    }

    public string Greet()
    {
        return $"Hello, {Name}";
    }
}
