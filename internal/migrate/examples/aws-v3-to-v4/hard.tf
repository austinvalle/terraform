variable "acl_policy" {
  type = string
}

locals {
  versioning     = var.acl_policy == "public"
  sse_algorithms = ["aws:mks"]
}

resource "aws_s3_bucket" "assets" {
  bucket = "my-app-assets"

  versioning {
    enabled = locals.versioning
  }

  dynamic "server_side_encryption_configuration" {
    for_each = locals.sse_algorithms
    content {
      rule {
        apply_server_side_encryption_by_default {
          sse_algorithm = each.key
        }
      }
    }
  }

  acl = var.acl_policy
}

resource "aws_s3_bucket" "another" {
  bucket = "foo"

  // We want the same SSE as with the other bucket
  dynamic "server_side_encription_configuration" {
    for_each = toset(aws_s3_bucket.assets.server_side_encryption_configuration.*.rule.apply_server_side_encryption_by_default.sse_algorithm)

    content {
      rule {
        apply_server_side_encryption_by_default {
          sse_algorithm = each.key
        }
      }
    }
  }
}

resource "aws_instance" "name" {
  for_each = toset([aws_s3_bucket.assets, aws_s3_bucket.another])

  name = each.value.server_side_encription_configuration.rule.apply_server_side_encryption_by_default.sse_algorithm
}

output "sse_policies" {
  value = aws_s3_bucket.assets.server_side_encryption_configuration.*.rule.apply_server_side_encryption_by_default.sse_algorithm
}
