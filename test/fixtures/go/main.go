package main

import "fmt"
   
// Person represents a person with a name and age.
type Person struct {
	Name string
	Age  int
}

// Greet returns a greeting string for the person.
func (p Person) Greet() string {
	return fmt.Sprintf("Hello, %s", p.Name)
}

func add(x, y int) int {
	return x + y
}

func main() {
	p := Person{Name: "Alice", Age: 30}
	fmt.Println(p.Greet())
	fmt.Println(add(1, 2))
}
