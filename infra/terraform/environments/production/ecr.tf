# ---------------------------------------------------------------------------
# Elastic Container Registry (ECR) repositories
# ---------------------------------------------------------------------------

resource "aws_ecr_repository" "admin" {
  name                 = "elitegate-admin"
  image_tag_mutability = "IMMUTABLE"
  force_delete         = false

  encryption_configuration {
    encryption_type = "AES256"
  }

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = merge(local.common_tags, {
    Name      = "elitegate-admin"
    Component = "Admin"
  })
}

resource "aws_ecr_repository" "gateway" {
  name                 = "elitegate-gateway"
  image_tag_mutability = "IMMUTABLE"
  force_delete         = false

  encryption_configuration {
    encryption_type = "AES256"
  }

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = merge(local.common_tags, {
    Name      = "elitegate-gateway"
    Component = "Gateway"
  })
}
