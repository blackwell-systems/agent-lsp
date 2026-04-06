require_relative 'person'

class Greeter
  attr_reader :person

  def initialize(person)
    @person = person
  end

  def say_hello
    "Greeter says: #{@person.greet}"
  end
end
