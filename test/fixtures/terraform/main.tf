terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

variable "person_name" {
  description = "Name of the person"
  type        = string
  default     = "Alice"
}

variable "person_age" {
  description = "Age of the person"
  type        = number
  default     = 30
}

locals {
  greeting = "Hello, ${var.person_name}"
}

output "greeting" {
  value = local.greeting
}
