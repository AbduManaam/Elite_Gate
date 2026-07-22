# ---------------------------------------------------------------------------
# Application Load Balancer (Public / Internet-Facing)
# ---------------------------------------------------------------------------

resource "aws_lb" "elitegate" {
  name               = "elitegate-production-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = [aws_subnet.public_a.id, aws_subnet.public_b.id]

  enable_deletion_protection = true
  drop_invalid_header_fields = true

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-alb"
    Component = "LoadBalancing"
    Tier      = "Public"
  })
}

# ---------------------------------------------------------------------------
# Target Group (EliteGate Gateway Application)
# ---------------------------------------------------------------------------

resource "aws_lb_target_group" "gateway" {
  name        = "elitegate-production-gateway-tg"
  port        = var.gateway_port
  protocol    = "HTTP"
  target_type = "instance"
  vpc_id      = aws_vpc.elitegate.id

  health_check {
    enabled             = true
    protocol            = "HTTP"
    path                = var.alb_health_check_path
    port                = "traffic-port"
    matcher             = "200" # Strict matcher representing direct HTTP 200 OK from health endpoint
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  deregistration_delay = 30

  tags = merge(local.common_tags, {
    Name      = "elitegate-production-gateway-tg"
    Component = "LoadBalancing"
    Tier      = "Application"
  })
}

# ---------------------------------------------------------------------------
# Target Group Attachment (Register EC2 App Server)
# ---------------------------------------------------------------------------

resource "aws_lb_target_group_attachment" "gateway" {
  target_group_arn = aws_lb_target_group.gateway.arn
  target_id        = aws_instance.elitegate_app.id
  port             = var.gateway_port
}

# ---------------------------------------------------------------------------
# HTTP Listener (Port 80) - Redirects to HTTPS (Explicit Path Preservation)
# ---------------------------------------------------------------------------

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.elitegate.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"

    redirect {
      protocol    = "HTTPS"
      port        = "443"
      host        = "#{host}"
      path        = "/#{path}"
      query       = "#{query}"
      status_code = "HTTP_301"
    }
  }
}

# ---------------------------------------------------------------------------
# HTTPS Listener (Port 443) - Terminates TLS & Forwards to TG
# ---------------------------------------------------------------------------

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.elitegate.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.alb_certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.gateway.arn
  }
}
