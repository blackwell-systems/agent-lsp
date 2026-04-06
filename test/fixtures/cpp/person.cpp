#include "person.h"

Person::Person(const std::string& name, int age) : name(name), age(age) {}

Person create_person(const std::string& name, int age) {
    return Person(name, age);
}

int add(int x, int y) {
    return x + y;
}

int main() {
    Person p = create_person("Alice", 30);
    return add(1, 2);
}
