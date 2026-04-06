#ifndef PERSON_H
#define PERSON_H

/**
 * A simple person struct with greeting functionality.
 */
typedef struct {
    char *name;
    int age;
} Person;

/**
 * Create a person with the given name and age.
 */
Person create_person(const char *name, int age);

int add(int x, int y);

#endif /* PERSON_H */
