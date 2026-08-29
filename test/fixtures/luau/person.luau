--!strict
-- A person with a name and age.

local Human = {}
Human.__index = Human

export type PersonType = {
	name: string,
	age: number,
}

-- Create a new Person.
function Human.new(name: string, age: number)
	local self = setmetatable({}, Human)
	self.name = name
	self.age = age
	return self
end

-- Return a greeting for this person.
function Human.greet(self): string
	return "Hello, " .. self.name
end

return Human
