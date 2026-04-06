package com.example;

/**
 * A person with a name and age.
 */
public class Person {
    private String name;
    private int age;

    public Person(String name, int age) {
        this.name = name;
        this.age = age;
    }

    /** Returns a greeting string. */
    public String greet() {
        return "Hello, " + this.name;
    }

    public static int add(int x, int y) {
        return x + y;
    }

    public static void main(String[] args) {
        Person p = new Person("Alice", 30);
        System.out.println(p.greet());
        System.out.println(add(1, 2));
    }
}
