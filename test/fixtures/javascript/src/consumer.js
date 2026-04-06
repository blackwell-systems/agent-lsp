import { add, Person } from './example.js';

// Use add()
const sum = add(1, 2);

// Use Person
const alice = new Person('Alice', 30, 'alice@example.com');

console.log(alice.getInfo());
console.log(`Sum: ${sum}`);
