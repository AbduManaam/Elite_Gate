resource "aws_internet_gateway" "elitegate" {
  vpc_id = aws_vpc.elitegate.id

  tags = merge(local.common_tags, {
    Name = "elitegate-igw"
  })
}
