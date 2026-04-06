<?php

require_once 'Person.php';

class Greeter {
    private Person $person;

    public function __construct(Person $person) {
        $this->person = $person;
    }

    public function sayHello(): string {
        return "Greeter says: " . $this->person->greet();
    }
}
