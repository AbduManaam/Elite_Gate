resource "aws_eip" "nat_a" {
  domain = "vpc"

  tags = merge(local.common_tags, {
    Name = "elitegate-nat-eip-a"
  })
}

resource "aws_nat_gateway" "elitegate_public_a" {
  allocation_id = aws_eip.nat_a.id
  subnet_id     = aws_subnet.public_a.id

  tags = merge(local.common_tags, {
    Name = "elitegate-nat-gateway-a"
  })

  depends_on = [
    aws_internet_gateway.elitegate
  ]
}
