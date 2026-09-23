//+------------------------------------------------------------------+
//| MQL fixture entry point                                          |
//+------------------------------------------------------------------+
#include "person.mqh"

int OnInit(void)
  {
   Person person("Alice", 30);
   string message   = person.Greet();
   int    age       = person.GetAge();
   Print(message, " age=", age);
   return(INIT_SUCCEEDED);
  }
