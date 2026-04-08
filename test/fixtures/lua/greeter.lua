local Person = require("person")

---@class Greeter
local Greeter = {}
Greeter.__index = Greeter

---@param person Person
---@return string
function Greeter:greet(person)
    return person:greet()
end

return Greeter
