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

# Public subnet route-table associations

resource "aws_route_table_association" "public_a" {
  subnet_id      = aws_subnet.public_a.id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table_association" "public_b" {
  subnet_id      = aws_subnet.public_b.id
  route_table_id = aws_route_table.public.id
}

# Private application subnet route-table associations

resource "aws_route_table_association" "private_app_a" {
  subnet_id      = aws_subnet.private_app_a.id
  route_table_id = aws_route_table.private.id
}

resource "aws_route_table_association" "private_app_b" {
  subnet_id      = aws_subnet.private_app_b.id
  route_table_id = aws_route_table.private.id
}
