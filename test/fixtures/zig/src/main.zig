const std = @import("std");
const Person = @import("person.zig").Person;
const Greeter = @import("greeter.zig").Greeter;

pub fn main() !void {
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    const allocator = gpa.allocator();
    const person = Person.init("Alice", 30);
    const greeting = try person.greet(allocator);
    defer allocator.free(greeting);
    std.debug.print("{s}\n", .{greeting});
}
