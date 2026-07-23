resource "aws_vpc" "elitegate" {
  cidr_block = "10.0.0.0/16"

  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name        = "elitegate-vpc"
    Project     = "elitegate"
    Environment = "production"
    ManagedBy   = "Terraform"
  }
}
