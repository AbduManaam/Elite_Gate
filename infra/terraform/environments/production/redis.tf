# ---------------------------------------------------------------------------
# Redis Auth Token Generator
# ---------------------------------------------------------------------------

resource "random_password" "redis_auth_token" {
  length           = 32
  special          = true
  override_special = "!&#$^<>-" # Safe subset of printable non-alphanumeric characters allowed by ElastiCache
}

# ---------------------------------------------------------------------------
# ElastiCache Subnet Group (Private App Subnets A & B)
# ---------------------------------------------------------------------------

resource "aws_elasticache_subnet_group" "redis" {
  name        = "elitegate-production-redis-subnet-group"
  description = "Subnet group for EliteGate production ElastiCache Redis"
  subnet_ids = [
    aws_subnet.private_app_a.id,
    aws_subnet.private_app_b.id
  ]

  tags = merge(local.common_tags, {
    Name = "elitegate-production-redis-subnet-group"
    Tier = "Cache"
  })
}

# ---------------------------------------------------------------------------
# Single-Node ElastiCache Redis Replication Group
# ---------------------------------------------------------------------------

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id = "elitegate-production-redis"
  description          = "ElastiCache Redis replication group for EliteGate production"

  # Engine Configuration (Redis 7.1 parameterized)
  engine         = "redis"
  engine_version = var.redis_engine_version
  node_type      = var.redis_node_type
  port           = 6379

  # Parameter Group Family (redis7 handles version 7.x)
  parameter_group_name = "default.redis7"

  # Topology - Single Primary Node
  num_cache_clusters         = 1
  automatic_failover_enabled = false
  multi_az_enabled           = false

  # Security & Networking
  subnet_group_name  = aws_elasticache_subnet_group.redis.name
  security_group_ids = [aws_security_group.redis.id]

  # Encryption
  transit_encryption_enabled = true
  at_rest_encryption_enabled = true
  auth_token                 = random_password.redis_auth_token.result

  # Maintenance & Updates
  apply_immediately  = false
  maintenance_window = "sun:05:00-sun:06:00" # UTC - choose window during lowest expected traffic

  tags = merge(local.common_tags, {
    Name = "elitegate-production-redis"
    Tier = "Cache"
  })
}
