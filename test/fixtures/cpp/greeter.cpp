#include "person.h"
#include <iostream>

void greet_person(const Person& p) {
    std::cout << "Greeter says: Hello, " << p.name << std::endl;
}
