#ifndef PERSON_H
#define PERSON_H

#include <string>

/**
 * A simple person class with greeting functionality.
 */
class Person {
public:
  std::string name;
  int age;

  Person(const std::string &name, int age);
};

/**
 * Create a person with the given name and age.
 */
Person create_person(const std::string &name, int age);

int add(int x, int y);

#endif /* PERSON_H */
