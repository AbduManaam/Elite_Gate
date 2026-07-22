# ---------------------------------------------------------------------------
# Amazon Linux 2023 AMI Data Source (x86_64)
# ---------------------------------------------------------------------------

data "aws_ami" "amazon_linux_2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-2023.*-x86_64"]
  }

  filter {
    name   = "architecture"
    values = ["x86_64"]
  }

  filter {
    name   = "root-device-type"
    values = ["ebs"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# ---------------------------------------------------------------------------
# EliteGate Production EC2 Application Server
# ---------------------------------------------------------------------------

resource "aws_instance" "elitegate_app" {
  ami           = data.aws_ami.amazon_linux_2023.id
  instance_type = var.ec2_instance_type

  subnet_id                   = aws_subnet.private_app_a.id
  associate_public_ip_address = false

  vpc_security_group_ids = [
    aws_security_group.ec2.id
  ]

  iam_instance_profile = aws_iam_instance_profile.ec2.name

  user_data = templatefile(
    "${path.module}/templates/ec2_user_data.sh.tftpl",
    {
      aws_region = var.aws_region
    }
  )
  user_data_replace_on_change = true

  monitoring                           = var.enable_detailed_monitoring
  instance_initiated_shutdown_behavior = "stop"

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
    instance_metadata_tags      = "disabled"
  }

  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.ec2_root_volume_size
    encrypted             = true
    delete_on_termination = true

    tags = merge(local.common_tags, {
      Name = "elitegate-production-app-root-volume-a"
    })
  }

  tags = merge(local.common_tags, {
    Name = "elitegate-production-app-server-a"
    Tier = "Application"
    Role = "EliteGateServer"
  })
}
