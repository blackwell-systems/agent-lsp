// Person class shared by MQL4 and MQL5 projects.
class Person
  {
private:
   string            m_name;
   int               m_age;

public:
                     Person(const string name, const int age);
   string            Greet(void) const;
   int               GetAge(void) const;
  };

Person::Person(const string name, const int age)
  {
   m_name            = name;
   m_age             = age;
  }

string Person::Greet(void) const
  {
   return("Hello, " + m_name + "!");
  }

int Person::GetAge(void) const
  {
   return(m_age);
  }
