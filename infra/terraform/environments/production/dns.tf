data "aws_route53_zone" "elitegateway" {
  name         = "elitegateway.site."
  private_zone = false
}

resource "aws_route53_record" "api" {
  zone_id = data.aws_route53_zone.elitegateway.zone_id
  name    = "api.elitegateway.site"
  type    = "A"

  alias {
    name                   = aws_lb.elitegate.dns_name
    zone_id                = aws_lb.elitegate.zone_id
    evaluate_target_health = true
  }
}

resource "aws_route53_record" "frontend_app" {
  zone_id = data.aws_route53_zone.elitegateway.zone_id
  name    = "app.elitegateway.site"
  type    = "CNAME"
  ttl     = 300
  records = ["080689a018702fe4.vercel-dns-017.com."]
}
