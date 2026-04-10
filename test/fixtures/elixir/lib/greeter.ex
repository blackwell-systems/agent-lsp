defmodule Greeter do
  def greet_person(name, age) do
    person = Person.new(name, age)
    Person.greet(person)
  end
end
