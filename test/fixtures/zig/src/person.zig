const std = @import("std");

/// A person with a name and age.
pub const Person = struct {
    name: []const u8,
    age: u32,

    pub fn init(name: []const u8, age: u32) Person {
        return Person{ .name = name, .age = age };
    }

    pub fn greet(self: Person, allocator: std.mem.Allocator) ![]u8 {
        return std.fmt.allocPrint(allocator, "Hello, {s}", .{self.name});
    }
};
