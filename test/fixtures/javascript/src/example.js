/**
 * A simple function that adds two numbers
 */
export function add(a, b) {
  return a + b;
}

/**
 * A class representing a person
 */
export class Person {
  constructor(name, age, email) {
    this.name = name;
    this.age = age;
    this.email = email;
  }

  /**
   * Get person's information
   */
  getInfo() {
    return `${this.name} is ${this.age} years old`;
  }
}

/**
 * A simple function that multiplies two numbers
 */
export function multiply(a, b) {
  return a * b;
}
