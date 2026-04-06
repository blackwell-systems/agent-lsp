<?php

/**
 * A person with a name and age.
 */
class Person {
    private string $name;
    private int $age;

    public function __construct(string $name, int $age) {
        $this->name = $name;
        $this->age = $age;
    }

    /** Returns a greeting string. */
    public function greet(): string {
        return "Hello, " . $this->name;
    }

    public static function add(int $x, int $y): int {
        return $x + $y;
    }
}

$p = new Person("Alice", 30);
echo $p->greet();
echo Person::add(1, 2);
