def add(x, y)
  x + y
end

class Person
  attr_reader :name, :age

  def initialize(name, age)
    @name = name
    @age = age
  end

  def greet
    "Hello, #{@name}"
  end
end

if __FILE__ == $0
  p = Person.new("Alice", 30)
  puts p.greet
  result = add(1, 2)
  puts result
end
