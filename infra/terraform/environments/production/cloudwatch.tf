# ---------------------------------------------------------------------------
# CloudWatch Log Groups (Application Logs)
# ---------------------------------------------------------------------------

resource "aws_cloudwatch_log_group" "admin" {
  name              = "/elitegate/production/admin"
  retention_in_days = var.cloudwatch_log_retention_days

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-admin-logs"
    Component = "Observability"
    Service   = "Admin"
  })
}

resource "aws_cloudwatch_log_group" "gateway" {
  name              = "/elitegate/production/gateway"
  retention_in_days = var.cloudwatch_log_retention_days

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-gateway-logs"
    Component = "Observability"
    Service   = "Gateway"
  })
}

# ---------------------------------------------------------------------------
# EC2 Application Server Metric Alarms
# ---------------------------------------------------------------------------

resource "aws_cloudwatch_metric_alarm" "ec2_high_cpu" {
  alarm_name          = "elitegate-production-ec2-high-cpu"
  alarm_description   = "Triggers when average EC2 CPU utilization is >= 80% for 15 minutes."
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 3
  metric_name         = "CPUUtilization"
  namespace           = "AWS/EC2"
  period              = 300
  statistic           = "Average"
  threshold           = var.ec2_high_cpu_threshold
  treat_missing_data  = "missing"

  dimensions = {
    InstanceId = aws_instance.elitegate_app.id
  }

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-ec2-high-cpu"
    Component = "Observability"
    Resource  = "EC2"
  })
}

resource "aws_cloudwatch_metric_alarm" "ec2_status_check_failed" {
  alarm_name          = "elitegate-production-ec2-status-check-failed"
  alarm_description   = "Triggers when EC2 combined status check fails (system or instance level)."
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 2
  metric_name         = "StatusCheckFailed"
  namespace           = "AWS/EC2"
  period              = 60
  statistic           = "Maximum"
  threshold           = 1
  treat_missing_data  = "breaching"

  dimensions = {
    InstanceId = aws_instance.elitegate_app.id
  }

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-ec2-status-check-failed"
    Component = "Observability"
    Resource  = "EC2"
  })
}

# ---------------------------------------------------------------------------
# RDS PostgreSQL Database Metric Alarms
# ---------------------------------------------------------------------------

resource "aws_cloudwatch_metric_alarm" "rds_high_cpu" {
  alarm_name          = "elitegate-production-rds-high-cpu"
  alarm_description   = "Triggers when average RDS PostgreSQL CPU utilization is >= 80% for 15 minutes."
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 3
  metric_name         = "CPUUtilization"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = var.rds_high_cpu_threshold
  treat_missing_data  = "missing"

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.postgres.identifier
  }

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-rds-high-cpu"
    Component = "Observability"
    Resource  = "RDS"
  })
}

resource "aws_cloudwatch_metric_alarm" "rds_low_free_storage" {
  alarm_name          = "elitegate-production-rds-low-storage"
  alarm_description   = "Triggers when RDS PostgreSQL free storage space drops below 5 GiB."
  comparison_operator = "LessThanOrEqualToThreshold"
  evaluation_periods  = 2
  metric_name         = "FreeStorageSpace"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = var.rds_low_free_storage_threshold_bytes
  treat_missing_data  = "missing"

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.postgres.identifier
  }

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-rds-low-storage"
    Component = "Observability"
    Resource  = "RDS"
  })
}

resource "aws_cloudwatch_metric_alarm" "rds_low_freeable_memory" {
  alarm_name          = "elitegate-production-rds-low-memory"
  alarm_description   = "Triggers when RDS PostgreSQL freeable memory drops below threshold."
  comparison_operator = "LessThanOrEqualToThreshold"
  evaluation_periods  = 3
  metric_name         = "FreeableMemory"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = var.rds_low_freeable_memory_threshold_bytes
  treat_missing_data  = "missing"

  dimensions = {
    DBInstanceIdentifier = aws_db_instance.postgres.identifier
  }

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-rds-low-memory"
    Component = "Observability"
    Resource  = "RDS"
  })
}

# ---------------------------------------------------------------------------
# ElastiCache Redis Metric Alarms
# ---------------------------------------------------------------------------

resource "aws_cloudwatch_metric_alarm" "redis_high_engine_cpu" {
  alarm_name          = "elitegate-production-redis-high-engine-cpu"
  alarm_description   = "Triggers when Redis single-threaded EngineCPUUtilization is >= 80% for 15 minutes."
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 3
  metric_name         = "EngineCPUUtilization"
  namespace           = "AWS/ElastiCache"
  period              = 300
  statistic           = "Average"
  threshold           = var.redis_high_engine_cpu_threshold
  treat_missing_data  = "missing"

  dimensions = {
    CacheClusterId = aws_elasticache_replication_group.redis.id
  }

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-redis-high-engine-cpu"
    Component = "Observability"
    Resource  = "ElastiCache"
  })
}

resource "aws_cloudwatch_metric_alarm" "redis_low_freeable_memory" {
  alarm_name          = "elitegate-production-redis-low-memory"
  alarm_description   = "Triggers when ElastiCache Redis freeable memory drops below threshold."
  comparison_operator = "LessThanOrEqualToThreshold"
  evaluation_periods  = 3
  metric_name         = "FreeableMemory"
  namespace           = "AWS/ElastiCache"
  period              = 300
  statistic           = "Average"
  threshold           = var.redis_low_freeable_memory_threshold_bytes
  treat_missing_data  = "missing"

  dimensions = {
    CacheClusterId = aws_elasticache_replication_group.redis.id
  }

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-redis-low-memory"
    Component = "Observability"
    Resource  = "ElastiCache"
  })
}

resource "aws_cloudwatch_metric_alarm" "redis_evictions" {
  alarm_name          = "elitegate-production-redis-evictions"
  alarm_description   = "Triggers when ElastiCache Redis performs key evictions due to memory pressure."
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 1
  metric_name         = "Evictions"
  namespace           = "AWS/ElastiCache"
  period              = 300
  statistic           = "Sum"
  threshold           = 1
  treat_missing_data  = "notBreaching"

  dimensions = {
    CacheClusterId = aws_elasticache_replication_group.redis.id
  }

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-redis-evictions"
    Component = "Observability"
    Resource  = "ElastiCache"
  })
}
