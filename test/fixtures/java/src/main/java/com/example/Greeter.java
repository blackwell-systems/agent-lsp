package com.example;

public class Greeter {
    private Person person;

    public Greeter(Person person) {
        this.person = person;
    }

    public String sayHello() {
        return "Greeter says: " + this.person.greet();
    }
}
