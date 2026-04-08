const std = @import("std");
const Person = @import("person.zig").Person;

pub const Greeter = struct {
    pub fn greet(person: Person, allocator: std.mem.Allocator) ![]u8 {
        return person.greet(allocator);
    }
};
