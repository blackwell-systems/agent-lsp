--!strict
-- A greeter that greets people.

local Greeter = {}
Greeter.__index = Greeter

-- Greet the given person.
function Greeter.greet(self, person): string
	return person:greet()
end

return Greeter
