--- A person with a name and age.
---@class Person
---@field name string
---@field age integer
local Person = {}
Person.__index = Person

---@param name string
---@param age integer
---@return Person
function Person.new(name, age)
    local self = setmetatable({}, Person)
    self.name = name
    self.age = age
    return self
end

---@return string
function Person:greet()
    return "Hello, " .. self.name
end

  
return Person
