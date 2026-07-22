resource "aws_route_table" "public" {
  vpc_id = aws_vpc.elitegate.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.elitegate.id
  }

  tags = merge(local.common_tags, {
    Name = "elitegate-public-route-table"
  })
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.elitegate.id

  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.elitegate_public_a.id
  }

  tags = merge(local.common_tags, {
    Name = "elitegate-private-route-table"
  })
}
