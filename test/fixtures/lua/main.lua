local Person = require("person")
local Greeter = require("greeter")

local person = Person.new("Alice", 30)
print(person:greet())

local greeter = Greeter
print(greeter:greet(person))
