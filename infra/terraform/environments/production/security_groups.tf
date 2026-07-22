# ---------------------------------------------------------------------------
# Security groups
# ---------------------------------------------------------------------------

resource "aws_security_group" "alb" {
  name        = "elitegate-alb-sg"
  description = "Security group for the EliteGate public Application Load Balancer"
  vpc_id      = aws_vpc.elitegate.id

  tags = merge(local.common_tags, {
    Name = "elitegate-alb-sg"
    Tier = "LoadBalancer"
  })
}

resource "aws_security_group" "ec2" {
  name        = "elitegate-ec2-sg"
  description = "Security group for EliteGate EC2 application workloads"
  vpc_id      = aws_vpc.elitegate.id

  tags = merge(local.common_tags, {
    Name = "elitegate-ec2-sg"
    Tier = "Application"
  })
}

resource "aws_security_group" "rds" {
  name        = "elitegate-rds-sg"
  description = "Security group for EliteGate RDS PostgreSQL"
  vpc_id      = aws_vpc.elitegate.id

  tags = merge(local.common_tags, {
    Name = "elitegate-rds-sg"
    Tier = "Database"
  })
}

resource "aws_security_group" "redis" {
  name        = "elitegate-redis-sg"
  description = "Security group for EliteGate ElastiCache Redis"
  vpc_id      = aws_vpc.elitegate.id

  tags = merge(local.common_tags, {
    Name = "elitegate-redis-sg"
    Tier = "Cache"
  })
}

# ---------------------------------------------------------------------------
# ALB ingress
# ---------------------------------------------------------------------------

resource "aws_vpc_security_group_ingress_rule" "alb_http_ipv4" {
  security_group_id = aws_security_group.alb.id

  description = "Allow public HTTP traffic"
  ip_protocol = "tcp"
  from_port   = 80
  to_port     = 80
  cidr_ipv4   = "0.0.0.0/0"
}

resource "aws_vpc_security_group_ingress_rule" "alb_https_ipv4" {
  security_group_id = aws_security_group.alb.id

  description = "Allow public HTTPS traffic"
  ip_protocol = "tcp"
  from_port   = 443
  to_port     = 443
  cidr_ipv4   = "0.0.0.0/0"
}

# ---------------------------------------------------------------------------
# ALB egress to application workloads
# ---------------------------------------------------------------------------

resource "aws_vpc_security_group_egress_rule" "alb_to_ec2_gateway" {
  security_group_id = aws_security_group.alb.id

  description                  = "Allow ALB traffic to the EliteGate gateway service"
  ip_protocol                  = "tcp"
  from_port                    = var.gateway_port
  to_port                      = var.gateway_port
  referenced_security_group_id = aws_security_group.ec2.id
}

resource "aws_vpc_security_group_egress_rule" "alb_to_ec2_admin" {
  security_group_id = aws_security_group.alb.id

  description                  = "Allow ALB traffic to the EliteGate admin service"
  ip_protocol                  = "tcp"
  from_port                    = var.admin_port
  to_port                      = var.admin_port
  referenced_security_group_id = aws_security_group.ec2.id
}

# ---------------------------------------------------------------------------
# EC2 ingress from ALB
# ---------------------------------------------------------------------------

resource "aws_vpc_security_group_ingress_rule" "ec2_gateway_from_alb" {
  security_group_id = aws_security_group.ec2.id

  description                  = "Allow gateway traffic from the ALB"
  ip_protocol                  = "tcp"
  from_port                    = var.gateway_port
  to_port                      = var.gateway_port
  referenced_security_group_id = aws_security_group.alb.id
}

resource "aws_vpc_security_group_ingress_rule" "ec2_admin_from_alb" {
  security_group_id = aws_security_group.ec2.id

  description                  = "Allow admin traffic from the ALB"
  ip_protocol                  = "tcp"
  from_port                    = var.admin_port
  to_port                      = var.admin_port
  referenced_security_group_id = aws_security_group.alb.id
}

# ---------------------------------------------------------------------------
# EC2 outbound access
# ---------------------------------------------------------------------------

resource "aws_vpc_security_group_egress_rule" "ec2_all_ipv4" {
  security_group_id = aws_security_group.ec2.id

  description = "Allow required outbound traffic from application workloads"
  ip_protocol = "-1"
  cidr_ipv4   = "0.0.0.0/0"
}

# ---------------------------------------------------------------------------
# RDS ingress from application workloads
# ---------------------------------------------------------------------------

resource "aws_vpc_security_group_ingress_rule" "rds_postgresql_from_ec2" {
  security_group_id = aws_security_group.rds.id

  description                  = "Allow PostgreSQL traffic from application workloads"
  ip_protocol                  = "tcp"
  from_port                    = 5432
  to_port                      = 5432
  referenced_security_group_id = aws_security_group.ec2.id
}

# ---------------------------------------------------------------------------
# Redis ingress from application workloads
# ---------------------------------------------------------------------------

resource "aws_vpc_security_group_ingress_rule" "redis_from_ec2" {
  security_group_id = aws_security_group.redis.id

  description                  = "Allow Redis traffic from application workloads"
  ip_protocol                  = "tcp"
  from_port                    = 6379
  to_port                      = 6379
  referenced_security_group_id = aws_security_group.ec2.id
}
