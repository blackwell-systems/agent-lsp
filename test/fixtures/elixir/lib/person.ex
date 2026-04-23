defmodule Person do
  defstruct [:name, :age]  

  def new(name, age) do
    %Person{name: name, age: age}
  end

  def greet(%Person{name: name}) do
    "Hello, #{name}"
  end
end
