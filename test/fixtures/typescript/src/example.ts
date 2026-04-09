/**
 * A simple function that adds two numbers
 */
export function add(a: number, b: number): number {
  return a + b;
}

/**
 * An interface representing a person
 */
export interface Person {
  name: string;
  age: number;
  email?: string;
}

/**
 * A class representing a greeter
 */
export class Greeter {
  private greeting: string;

  constructor(greeting: string) {
    this.greeting = greeting;
  }

  /**
   * Greets a person
   */
  greet(person: Person): string {
    return `${this.greeting}, ${person.name}!`;
  }
}

// This will generate a diagnostic error - missing return type
export function multiply(a: number, b: number) {
  return a * b;
}

// This will generate a diagnostic error - unused variable
const unused = "This variable is not used";

// This will generate a diagnostic error - undefined variable
const result = undefinedVariable + 10;
