# ---------------------------------------------------------------------------
# IAM role and instance profile for EC2
# ---------------------------------------------------------------------------

resource "aws_iam_role" "ec2" {
  name        = "elitegate-production-ec2-role"
  description = "IAM role assumed by EliteGate production EC2 instances"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"

    Statement = [
      {
        Sid    = "AllowEC2AssumeRole"
        Effect = "Allow"

        Principal = {
          Service = "ec2.amazonaws.com"
        }

        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-ec2-role"
    Component = "Compute"
  })
}

resource "aws_iam_instance_profile" "ec2" {
  name = "elitegate-production-ec2-instance-profile"
  role = aws_iam_role.ec2.name

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-ec2-instance-profile"
    Component = "Compute"
  })
}

# ---------------------------------------------------------------------------
# EC2 Secrets & SSM Read Access IAM Policy
# ---------------------------------------------------------------------------

resource "aws_iam_policy" "ec2_secrets_policy" {
  name        = "elitegate-production-ec2-secrets-policy"
  description = "Grants EC2 instance least-privilege read permissions for EliteGate secrets and SSM parameters"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AllowSecretsManagerRead"
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue",
          "secretsmanager:DescribeSecret"
        ]
        Resource = [
          "arn:aws:secretsmanager:${var.aws_region}:${data.aws_caller_identity.current.account_id}:secret:elitegate/production/*"
        ]
      },
      {
        Sid    = "AllowProjectJWTSecretCreate"
        Effect = "Allow"

        Action = [
          "secretsmanager:CreateSecret"
        ]

        Resource = "*"

        Condition = {
          StringLike = {
            "secretsmanager:Name" = "elitegate/production/projects/*/jwt/hs256"
          }
        }
      },
      {
        Sid    = "AllowProjectJWTSecretManagement"
        Effect = "Allow"

        Action = [
          "secretsmanager:PutSecretValue",
          "secretsmanager:DeleteSecret",
          "secretsmanager:RestoreSecret"
        ]

        Resource = [
          "arn:aws:secretsmanager:${var.aws_region}:${data.aws_caller_identity.current.account_id}:secret:elitegate/production/projects/*"
        ]
      },
      {
        Sid    = "AllowSSMParameterRead"
        Effect = "Allow"
        Action = [
          "ssm:GetParameter",
          "ssm:GetParameters",
          "ssm:GetParametersByPath"
        ]
        Resource = [
          "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter/elitegate/production",
          "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter/elitegate/production/*"
        ]
      },
      {
        Sid    = "AllowKMSDecrypt"
        Effect = "Allow"
        Action = [
          "kms:Decrypt",
          "kms:DescribeKey"
        ]
        Resource = [
          "arn:aws:kms:${var.aws_region}:${data.aws_caller_identity.current.account_id}:alias/aws/secretsmanager",
          "arn:aws:kms:${var.aws_region}:${data.aws_caller_identity.current.account_id}:alias/aws/ssm"
        ]
      }
    ]
  })

  tags = merge(local.common_tags, {
    Name = "elitegate-production-ec2-secrets-policy"
  })
}

resource "aws_iam_role_policy_attachment" "ec2_secrets" {
  role       = aws_iam_role.ec2.name
  policy_arn = aws_iam_policy.ec2_secrets_policy.arn
}

resource "aws_iam_role_policy_attachment" "ec2_ssm_core" {
  role       = aws_iam_role.ec2.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}
resource "aws_iam_role_policy_attachment" "ec2_ecr_read_only" {
  role       = aws_iam_role.ec2.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

