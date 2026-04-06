package main

import "fmt"

// Greeter wraps a Person and produces greetings.
type Greeter struct {
	person Person
}

func NewGreeter(p Person) Greeter {
	return Greeter{person: p}
}

func (g Greeter) SayHello() string {
	return fmt.Sprintf("Greeter says: %s", g.person.Greet())
}
