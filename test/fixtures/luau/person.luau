--!strict
-- A person with a name and age.

local Person = {}
Person.__index = Person

export type PersonType = {
	name: string,
	age: number,
}

-- Create a new Person.
function Person.new(name: string, age: number)
	local self = setmetatable({}, Person)
	self.name = name
	self.age = age
	return self
end

-- Return a greeting for this person.
function Person.greet(self): string
	return "Hello, " .. self.name
end

return Person
