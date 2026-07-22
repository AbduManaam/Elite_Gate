variable "aws_region" {
  description = "AWS region for EliteGate production infrastructure."
  type        = string
  default     = "ap-south-1"
}

variable "project_name" {
  description = "Project name used in resource names and tags."
  type        = string
  default     = "elitegate"
}

variable "environment" {
  description = "Deployment environment."
  type        = string
  default     = "production"

  validation {
    condition     = contains(["development", "staging", "production"], var.environment)
    error_message = "environment must be development, staging, or production."
  }
}

variable "vpc_cidr" {
  description = "CIDR block for the EliteGate VPC."
  type        = string
  default     = "10.20.0.0/16"
}

variable "public_subnet_cidrs" {
  description = "CIDR blocks for public subnets used by the ALB."
  type        = list(string)
  default     = ["10.20.0.0/24", "10.20.1.0/24"]
}

variable "private_app_subnet_cidrs" {
  description = "CIDR blocks reserved for private application resources."
  type        = list(string)
  default     = ["10.20.10.0/24", "10.20.11.0/24"]
}

variable "private_data_subnet_cidrs" {
  description = "CIDR blocks for RDS and ElastiCache."
  type        = list(string)
  default     = ["10.20.20.0/24", "10.20.21.0/24"]
}

variable "admin_instance_type" {
  description = "EC2 instance type for the Admin service and dynamic gateway Docker containers."
  type        = string
  default     = "t3.small"
}

variable "gateway_port" {
  description = "Host port exposed by the EliteGate Gateway container."
  type        = number
  default     = 8080
}

variable "admin_port" {
  description = "Host port exposed by the EliteGate Admin container."
  type        = number
  default     = 9090
}

variable "database_name" {
  description = "Initial PostgreSQL database name."
  type        = string
  default     = "elitegate"
}

variable "database_username" {
  description = "PostgreSQL administrator username. The password will be generated later."
  type        = string
  default     = "elitegate_admin"
  sensitive   = true
}

variable "database_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t4g.micro"
}

variable "redis_node_type" {
  description = "ElastiCache node type."
  type        = string
  default     = "cache.t4g.micro"
}

variable "root_domain_name" {
  description = "EliteGate-owned root domain, for example elitegate.io. Leave empty until Route 53 is ready."
  type        = string
  default     = ""
}

variable "api_subdomain" {
  description = "Subdomain used by the Admin API."
  type        = string
  default     = "api"
}

variable "vercel_frontend_url" {
  description = "Production Vercel frontend URL used later for CORS and OAuth redirects."
  type        = string
  default     = ""
}

variable "allowed_ssh_cidr" {
  description = "Single trusted CIDR allowed to use SSH. Prefer SSM and leave this empty."
  type        = string
  default     = ""
}

variable "ec2_instance_type" {
  description = "EC2 instance type for the EliteGate application server"
  type        = string
  default     = "t3.small"
}

variable "ec2_root_volume_size" {
  description = "Root EBS volume size in GiB for the EliteGate EC2 server"
  type        = number
  default     = 30

  validation {
    condition     = var.ec2_root_volume_size >= 20
    error_message = "The EC2 root volume must be at least 20 GiB."
  }
}

variable "enable_detailed_monitoring" {
  description = "Enable detailed EC2 monitoring"
  type        = bool
  default     = true
}

variable "database_engine_version" {
  description = "PostgreSQL engine version for Amazon RDS."
  type        = string
  default     = "16"
}

variable "database_backup_retention_days" {
  description = "Number of days to retain automated RDS database backups."
  type        = number
  default     = 7
}

variable "redis_engine_version" {
  description = "Redis engine version for Amazon ElastiCache."
  type        = string
  default     = "7.1"
}

variable "alb_certificate_arn" {
  description = "ARN of the validated ACM certificate used by the EliteGate production ALB HTTPS listener."
  type        = string

  validation {
    condition = can(regex(
      "^arn:aws:acm:ap-south-1:[0-9]{12}:certificate/[A-Za-z0-9-]+$",
      var.alb_certificate_arn
    ))
    error_message = "alb_certificate_arn must be a valid ACM certificate ARN from ap-south-1."
  }
}

variable "alb_health_check_path" {
  description = "Unauthenticated HTTP path used by the ALB to check application readiness."
  type        = string
  default     = "/health"

  validation {
    condition     = startswith(var.alb_health_check_path, "/")
    error_message = "alb_health_check_path must begin with '/'."
  }
}

variable "cloudwatch_log_retention_days" {
  description = "Number of days EliteGate application logs are retained in CloudWatch."
  type        = number
  default     = 30

  validation {
    condition     = contains([1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653], var.cloudwatch_log_retention_days)
    error_message = "The CloudWatch log retention value must be supported by AWS CloudWatch Logs."
  }
}

variable "ec2_high_cpu_threshold" {
  description = "Average EC2 CPU percentage threshold that triggers an alarm."
  type        = number
  default     = 80
}

variable "rds_high_cpu_threshold" {
  description = "Average RDS CPU percentage threshold that triggers an alarm."
  type        = number
  default     = 80
}

variable "rds_low_free_storage_threshold_bytes" {
  description = "RDS free storage threshold in bytes (default 5 GiB)."
  type        = number
  default     = 5368709120
}

variable "rds_low_freeable_memory_threshold_bytes" {
  description = "RDS freeable memory threshold in bytes (default 256 MiB)."
  type        = number
  default     = 268435456
}

variable "redis_high_engine_cpu_threshold" {
  description = "Average Redis engine CPU percentage threshold that triggers an alarm."
  type        = number
  default     = 80
}

variable "redis_low_freeable_memory_threshold_bytes" {
  description = "ElastiCache Redis freeable memory threshold in bytes (default 64 MiB)."
  type        = number
  default     = 67108864
}

variable "smtp_host" {
  description = "SMTP host server address."
  type        = string
  default     = "smtp.gmail.com"
}

variable "smtp_port" {
  description = "SMTP server port."
  type        = number
  default     = 587
}

variable "smtp_from_email" {
  description = "Sender email address for system emails."
  type        = string
  default     = "noreply@elitegate.io"
}






