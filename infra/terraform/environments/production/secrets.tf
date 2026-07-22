# ---------------------------------------------------------------------------
# Data Source: AWS Caller Identity (Strict Account ID Scoping)
# ---------------------------------------------------------------------------

data "aws_caller_identity" "current" {}

# ---------------------------------------------------------------------------
# Secret Password Generators for JWT & OAuth State
# ---------------------------------------------------------------------------

resource "random_password" "jwt_secret" {
  length  = 64
  special = false
}

resource "random_password" "oauth_state_secret" {
  length  = 32
  special = false
}

# ---------------------------------------------------------------------------
# AWS Secrets Manager - Database Credentials (Managed Secret Version)
# ---------------------------------------------------------------------------

resource "aws_secretsmanager_secret" "database" {
  name                    = "elitegate/production/database/postgres"
  description             = "PostgreSQL primary database connection credentials for EliteGate production"
  recovery_window_in_days = 7

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-database-secret"
    Component = "Database"
  })
}

resource "aws_secretsmanager_secret_version" "database" {
  secret_id = aws_secretsmanager_secret.database.id
  secret_string = jsonencode({
    username = var.database_username
    password = random_password.db_password.result
    engine   = "postgres"
    host     = aws_db_instance.postgres.address
    port     = aws_db_instance.postgres.port
    dbname   = var.database_name
  })
}

# ---------------------------------------------------------------------------
# AWS Secrets Manager - Redis Credentials (Managed Secret Version)
# ---------------------------------------------------------------------------

resource "aws_secretsmanager_secret" "redis" {
  name                    = "elitegate/production/redis/cache"
  description             = "ElastiCache Redis authentication token and endpoint for EliteGate production"
  recovery_window_in_days = 7

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-redis-secret"
    Component = "Cache"
  })
}

resource "aws_secretsmanager_secret_version" "redis" {
  secret_id = aws_secretsmanager_secret.redis.id
  secret_string = jsonencode({
    auth_token       = random_password.redis_auth_token.result
    primary_endpoint = aws_elasticache_replication_group.redis.primary_endpoint_address
    port             = aws_elasticache_replication_group.redis.port
  })
}

# ---------------------------------------------------------------------------
# AWS Secrets Manager - JWT Signing Secret (Managed Secret Version)
# ---------------------------------------------------------------------------

resource "aws_secretsmanager_secret" "jwt" {
  name                    = "elitegate/production/auth/jwt"
  description             = "JWT authentication signing secret for EliteGate"
  recovery_window_in_days = 7

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-jwt-secret"
    Component = "Auth"
  })
}

resource "aws_secretsmanager_secret_version" "jwt" {
  secret_id = aws_secretsmanager_secret.jwt.id
  secret_string = jsonencode({
    jwt_secret = random_password.jwt_secret.result
  })
}

# ---------------------------------------------------------------------------
# AWS Secrets Manager - OAuth Container (External Secret - Populated via CLI)
# ---------------------------------------------------------------------------

resource "aws_secretsmanager_secret" "oauth" {
  name                    = "elitegate/production/auth/oauth"
  description             = "Google OAuth client credentials and state secret container for EliteGate"
  recovery_window_in_days = 7

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-oauth-secret"
    Component = "Auth"
  })
}

# Initial placeholder secret version containing auto-generated OAuth state secret
resource "aws_secretsmanager_secret_version" "oauth" {
  secret_id = aws_secretsmanager_secret.oauth.id
  secret_string = jsonencode({
    google_client_id     = ""
    google_client_secret = ""
    oauth_state_secret   = random_password.oauth_state_secret.result
    google_redirect_url  = var.root_domain_name != "" ? "https://${var.api_subdomain}.${var.root_domain_name}/admin/google/callback" : "http://localhost:9090/admin/google/callback"
  })

  lifecycle {
    ignore_changes = [secret_string] # Keeps externally updated client secrets intact during terraform apply
  }
}

# ---------------------------------------------------------------------------
# AWS Secrets Manager - SMTP Container (External Secret - Populated via CLI)
# ---------------------------------------------------------------------------

resource "aws_secretsmanager_secret" "smtp" {
  name                    = "elitegate/production/email/smtp"
  description             = "SMTP server credentials container for outbound system emails"
  recovery_window_in_days = 7

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-smtp-secret"
    Component = "Email"
  })
}

resource "aws_secretsmanager_secret_version" "smtp" {
  secret_id = aws_secretsmanager_secret.smtp.id
  secret_string = jsonencode({
    smtp_username = ""
    smtp_password = ""
  })

  lifecycle {
    ignore_changes = [secret_string] # Keeps externally updated passwords intact during terraform apply
  }
}

# ---------------------------------------------------------------------------
# AWS Systems Manager (SSM) Parameter Store - Application Configurations
# ---------------------------------------------------------------------------

resource "aws_ssm_parameter" "app_environment" {
  name        = "/elitegate/production/app/environment"
  description = "Deployment environment name"
  type        = "String"
  value       = var.environment

  tags = merge(local.common_tags, {
    Name = "elitegate-production-param-environment"
  })
}

resource "aws_ssm_parameter" "aws_region" {
  name        = "/elitegate/production/app/aws_region"
  description = "AWS region for EliteGate production stack"
  type        = "String"
  value       = var.aws_region

  tags = merge(local.common_tags, {
    Name = "elitegate-production-param-region"
  })
}

resource "aws_ssm_parameter" "gateway_port" {
  name        = "/elitegate/production/gateway/port"
  description = "Listening port for EliteGate Gateway service"
  type        = "String"
  value       = tostring(var.gateway_port)

  tags = merge(local.common_tags, {
    Name = "elitegate-production-param-gateway-port"
  })
}

resource "aws_ssm_parameter" "admin_port" {
  name        = "/elitegate/production/admin/port"
  description = "Listening port for EliteGate Admin service"
  type        = "String"
  value       = tostring(var.admin_port)

  tags = merge(local.common_tags, {
    Name = "elitegate-production-param-admin-port"
  })
}

# ---------------------------------------------------------------------------
# SSM Parameter Store - SMTP Configuration Flags (Non-Sensitive)
# ---------------------------------------------------------------------------

resource "aws_ssm_parameter" "smtp_enabled" {
  name        = "/elitegate/production/email/enabled"
  description = "Enable or disable SMTP email dispatch"
  type        = "String"
  value       = "true"

  tags = merge(local.common_tags, {
    Name = "elitegate-production-param-smtp-enabled"
  })
}

resource "aws_ssm_parameter" "smtp_host" {
  name        = "/elitegate/production/email/host"
  description = "SMTP host address"
  type        = "String"
  value       = var.smtp_host

  tags = merge(local.common_tags, {
    Name = "elitegate-production-param-smtp-host"
  })
}

resource "aws_ssm_parameter" "smtp_port" {
  name        = "/elitegate/production/email/port"
  description = "SMTP host port"
  type        = "String"
  value       = tostring(var.smtp_port)

  tags = merge(local.common_tags, {
    Name = "elitegate-production-param-smtp-port"
  })
}

resource "aws_ssm_parameter" "smtp_from_email" {
  name        = "/elitegate/production/email/from_email"
  description = "Sender email address for system emails"
  type        = "String"
  value       = var.smtp_from_email != "" ? var.smtp_from_email : "noreply@elitegate.io"

  tags = merge(local.common_tags, {
    Name = "elitegate-production-param-smtp-from-email"
  })
}
