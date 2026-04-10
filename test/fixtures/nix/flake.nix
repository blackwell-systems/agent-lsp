{
  description = "fixture";

  outputs = { self }: {
    helper = name: "Hello, ${name}";

    greeting = self.helper "Alice";
  };
}
