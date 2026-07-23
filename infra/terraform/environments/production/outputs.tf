output "configuration_summary" {
  description = "Non-sensitive values selected for the production stack."
  value = {
    aws_region          = var.aws_region
    project_name        = var.project_name
    environment         = var.environment
    vpc_cidr            = var.vpc_cidr
    admin_instance_type = var.admin_instance_type
    admin_port          = var.admin_port
  }
}

output "ecr_admin_repository_url" {
  description = "URL of the EliteGate Admin ECR repository"
  value       = aws_ecr_repository.admin.repository_url
}

output "ecr_gateway_repository_url" {
  description = "URL of the EliteGate Gateway ECR repository"
  value       = aws_ecr_repository.gateway.repository_url
}

output "ecr_admin_repository_arn" {
  description = "ARN of the EliteGate Admin ECR repository"
  value       = aws_ecr_repository.admin.arn
}

output "ecr_gateway_repository_arn" {
  description = "ARN of the EliteGate Gateway ECR repository"
  value       = aws_ecr_repository.gateway.arn
}

output "ec2_iam_role_name" {
  description = "Name of the IAM role used by EliteGate production EC2 instances"
  value       = aws_iam_role.ec2.name
}

output "ec2_iam_role_arn" {
  description = "ARN of the IAM role used by EliteGate production EC2 instances"
  value       = aws_iam_role.ec2.arn
}

output "ec2_instance_profile_name" {
  description = "Name of the IAM instance profile for EliteGate production EC2 instances"
  value       = aws_iam_instance_profile.ec2.name
}

output "ec2_instance_profile_arn" {
  description = "ARN of the IAM instance profile for EliteGate production EC2 instances"
  value       = aws_iam_instance_profile.ec2.arn
}

output "elitegate_ec2_instance_id" {
  description = "ID of the EliteGate production EC2 instance"
  value       = aws_instance.elitegate_app.id
}

output "elitegate_ec2_private_ip" {
  description = "Private IPv4 address of the EliteGate production EC2 instance"
  value       = aws_instance.elitegate_app.private_ip
}

output "rds_endpoint" {
  description = "Connection endpoint for the EliteGate RDS PostgreSQL instance"
  value       = aws_db_instance.postgres.endpoint
}

output "rds_port" {
  description = "Port of the EliteGate RDS PostgreSQL instance"
  value       = aws_db_instance.postgres.port
}

output "rds_identifier" {
  description = "Identifier of the EliteGate RDS PostgreSQL instance"
  value       = aws_db_instance.postgres.identifier
}

output "rds_arn" {
  description = "ARN of the EliteGate RDS PostgreSQL instance"
  value       = aws_db_instance.postgres.arn
}

output "rds_db_subnet_group_name" {
  description = "Name of the DB subnet group for EliteGate RDS"
  value       = aws_db_subnet_group.postgres.name
}

output "redis_primary_endpoint" {
  description = "Primary endpoint address for EliteGate ElastiCache Redis"
  value       = aws_elasticache_replication_group.redis.primary_endpoint_address
}

output "redis_port" {
  description = "Port number of the EliteGate ElastiCache Redis instance"
  value       = aws_elasticache_replication_group.redis.port
}

output "redis_replication_group_id" {
  description = "Replication Group ID of the EliteGate ElastiCache Redis instance"
  value       = aws_elasticache_replication_group.redis.id
}

output "redis_subnet_group_name" {
  description = "Name of the ElastiCache subnet group for EliteGate Redis"
  value       = aws_elasticache_subnet_group.redis.name
}

output "alb_arn" {
  description = "ARN of the EliteGate production Application Load Balancer"
  value       = aws_lb.elitegate.arn
}

output "alb_dns_name" {
  description = "AWS-generated DNS name of the EliteGate production Application Load Balancer"
  value       = aws_lb.elitegate.dns_name
}

output "alb_zone_id" {
  description = "Canonical hosted zone ID of the EliteGate production Application Load Balancer"
  value       = aws_lb.elitegate.zone_id
}

output "alb_target_group_arn" {
  description = "ARN of the EliteGate application target group"
  value       = aws_lb_target_group.gateway.arn
}

output "alb_https_listener_arn" {
  description = "ARN of the EliteGate production HTTPS listener"
  value       = aws_lb_listener.https.arn
}

output "cloudwatch_admin_log_group_name" {
  description = "Name of the CloudWatch Log Group for EliteGate Admin application logs"
  value       = aws_cloudwatch_log_group.admin.name
}

output "cloudwatch_admin_log_group_arn" {
  description = "ARN of the CloudWatch Log Group for EliteGate Admin application logs"
  value       = aws_cloudwatch_log_group.admin.arn
}

output "cloudwatch_gateway_log_group_name" {
  description = "Name of the CloudWatch Log Group for EliteGate Gateway application logs"
  value       = aws_cloudwatch_log_group.gateway.name
}

output "cloudwatch_gateway_log_group_arn" {
  description = "ARN of the CloudWatch Log Group for EliteGate Gateway application logs"
  value       = aws_cloudwatch_log_group.gateway.arn
}

output "ec2_high_cpu_alarm_name" {
  description = "Alarm name for EC2 high CPU utilization"
  value       = aws_cloudwatch_metric_alarm.ec2_high_cpu.alarm_name
}

output "ec2_status_check_alarm_name" {
  description = "Alarm name for EC2 status check failure"
  value       = aws_cloudwatch_metric_alarm.ec2_status_check_failed.alarm_name
}

output "rds_high_cpu_alarm_name" {
  description = "Alarm name for RDS high CPU utilization"
  value       = aws_cloudwatch_metric_alarm.rds_high_cpu.alarm_name
}

output "rds_low_storage_alarm_name" {
  description = "Alarm name for RDS low free storage"
  value       = aws_cloudwatch_metric_alarm.rds_low_free_storage.alarm_name
}

output "redis_high_cpu_alarm_name" {
  description = "Alarm name for ElastiCache Redis high engine CPU utilization"
  value       = aws_cloudwatch_metric_alarm.redis_high_engine_cpu.alarm_name
}

output "redis_evictions_alarm_name" {
  description = "Alarm name for ElastiCache Redis key evictions"
  value       = aws_cloudwatch_metric_alarm.redis_evictions.alarm_name
}

output "secrets_database_arn" {
  description = "ARN of the Secrets Manager secret for RDS PostgreSQL database credentials"
  value       = aws_secretsmanager_secret.database.arn
}

output "secrets_redis_arn" {
  description = "ARN of the Secrets Manager secret for ElastiCache Redis credentials"
  value       = aws_secretsmanager_secret.redis.arn
}

output "secrets_jwt_arn" {
  description = "ARN of the Secrets Manager secret for JWT authentication"
  value       = aws_secretsmanager_secret.jwt.arn
}

output "secrets_oauth_arn" {
  description = "ARN of the Secrets Manager secret for Google OAuth"
  value       = aws_secretsmanager_secret.oauth.arn
}

output "secrets_smtp_arn" {
  description = "ARN of the Secrets Manager secret for SMTP credentials"
  value       = aws_secretsmanager_secret.smtp.arn
}

output "ssm_param_environment_name" {
  description = "SSM Parameter name for application environment"
  value       = aws_ssm_parameter.app_environment.name
}

output "ssm_param_gateway_port_name" {
  description = "SSM Parameter name for Gateway service port"
  value       = aws_ssm_parameter.gateway_port.name
}

output "github_cd_role_arn" {
  description = "IAM role ARN used by GitHub Actions CD"
  value       = aws_iam_role.github_cd.arn
}







