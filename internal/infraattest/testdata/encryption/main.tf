terraform {
  required_version = "= 1.12.5"

  encryption {
    plan {
      enforced = true
    }
  }
}

variable "private_sentinel" {
  type      = string
  sensitive = true
}

resource "terraform_data" "private" {
  input = var.private_sentinel
}
