# ---------------------------------------------------------------------------
# Database Subnet Group (Private DB Subnets A & B)
# ---------------------------------------------------------------------------

resource "aws_db_subnet_group" "postgres" {
  name        = "elitegate-production-db-subnet-group"
  description = "Database subnet group for EliteGate production RDS PostgreSQL"
  subnet_ids = [
    aws_subnet.private_db_a.id,
    aws_subnet.private_db_b.id
  ]

  tags = merge(local.common_tags, {
    Name = "elitegate-production-db-subnet-group"
    Tier = "Database"
  })
}

# ---------------------------------------------------------------------------
# Master Database Password & Final Snapshot Suffix Generators
# ---------------------------------------------------------------------------

resource "random_password" "db_password" {
  length           = 32
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

resource "random_id" "snapshot_suffix" {
  byte_length = 4
}

# ---------------------------------------------------------------------------
# Managed RDS PostgreSQL Database Instance
# ---------------------------------------------------------------------------

resource "aws_db_instance" "postgres" {
  identifier     = "elitegate-production-db"
  engine         = "postgres"
  engine_version = var.database_engine_version
  instance_class = var.database_instance_class

  allocated_storage     = 20
  max_allocated_storage = 100
  storage_type          = "gp3"
  storage_encrypted     = true

  db_name  = var.database_name
  username = var.database_username
  password = random_password.db_password.result

  db_subnet_group_name   = aws_db_subnet_group.postgres.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  publicly_accessible = false
  multi_az            = false

  backup_retention_period    = var.database_backup_retention_days
  backup_window              = "03:00-04:00"
  maintenance_window         = "Sun:04:30-Sun:05:30"
  auto_minor_version_upgrade = true

  deletion_protection       = true
  skip_final_snapshot       = false
  final_snapshot_identifier = "elitegate-production-db-final-${random_id.snapshot_suffix.hex}"

  tags = merge(local.common_tags, {
    Name = "elitegate-production-db"
    Tier = "Database"
  })
}
