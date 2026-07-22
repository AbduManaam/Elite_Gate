resource "aws_subnet" "public_a" {
  vpc_id                  = aws_vpc.elitegate.id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = "ap-south-1a"
  map_public_ip_on_launch = true

  tags = merge(local.common_tags, {
    Name = "elitegate-public-subnet-a"
    Type = "public"
  })
}

resource "aws_subnet" "public_b" {
  vpc_id                  = aws_vpc.elitegate.id
  cidr_block              = "10.0.2.0/24"
  availability_zone       = "ap-south-1b"
  map_public_ip_on_launch = true

  tags = merge(local.common_tags, {
    Name = "elitegate-public-subnet-b"
    Type = "public"
  })
}

# Private Application Subnets
resource "aws_subnet" "private_app_a" {
  vpc_id                  = aws_vpc.elitegate.id
  cidr_block              = "10.0.11.0/24"
  availability_zone       = "ap-south-1a"
  map_public_ip_on_launch = false

  tags = merge(local.common_tags, {
    Name = "elitegate-private-app-subnet-a"
    Type = "private"
    Tier = "App"
  })
}

resource "aws_subnet" "private_app_b" {
  vpc_id                  = aws_vpc.elitegate.id
  cidr_block              = "10.0.12.0/24"
  availability_zone       = "ap-south-1b"
  map_public_ip_on_launch = false

  tags = merge(local.common_tags, {
    Name = "elitegate-private-app-subnet-b"
    Type = "private"
    Tier = "App"
  })
}

# Private Database Subnets
resource "aws_subnet" "private_db_a" {
  vpc_id                  = aws_vpc.elitegate.id
  cidr_block              = "10.0.21.0/24"
  availability_zone       = "ap-south-1a"
  map_public_ip_on_launch = false

  tags = merge(local.common_tags, {
    Name = "elitegate-private-db-subnet-a"
    Type = "private"
    Tier = "Database"
  })
}

resource "aws_subnet" "private_db_b" {
  vpc_id                  = aws_vpc.elitegate.id
  cidr_block              = "10.0.22.0/24"
  availability_zone       = "ap-south-1b"
  map_public_ip_on_launch = false

  tags = merge(local.common_tags, {
    Name = "elitegate-private-db-subnet-b"
    Type = "private"
    Tier = "Database"
  })
}
