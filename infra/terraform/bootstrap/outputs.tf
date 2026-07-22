output "state_bucket_name" {
  description = "S3 bucket used by the main Terraform configuration."
  value       = aws_s3_bucket.terraform_state.id
}

output "production_backend_example" {
  description = "Example backend settings for the production stack."
  value = {
    bucket       = aws_s3_bucket.terraform_state.id
    key          = "elitegate/production/terraform.tfstate"
    region       = var.aws_region
    encrypt      = true
    use_lockfile = true
  }
}
