import { add, Person, Greeter } from './example';

// Use add()
const sum = add(1, 2);

// Use Person as a type
const alice: Person = { name: 'Alice', age: 30 };

// Use Greeter
const greeter = new Greeter('Hello');
console.log(greeter.greet(alice));
console.log(sum);
