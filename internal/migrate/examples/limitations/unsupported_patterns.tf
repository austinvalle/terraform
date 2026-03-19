# Patterns that CANNOT be automated with the current migration system.
# Each section describes what would be needed to support it.

# ============================================================================
# LIMITATION 1: S3 Lifecycle Rule Extraction with Structural Rewriting
# ============================================================================
# The v3→v4 S3 lifecycle_rule extraction is the most complex migration.
# The nested block structure changes significantly: attribute names change,
# values change type (bool→string), and sub-blocks are restructured.
#
# What would be needed: a "transform" action that can rewrite nested block
# structure and map values (e.g., enabled=true → status="Enabled").
#
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-4-upgrade.html.markdown#s3-bucket-refactor

resource "aws_s3_bucket" "with_lifecycle" {
  bucket = "my-bucket"

  lifecycle_rule {
    id      = "expire-old"
    enabled = true              # v4 changes to: status = "Enabled"
    prefix  = "logs/"           # v4 changes to: filter { prefix = "logs/" }

    expiration {
      days = 90
    }

    noncurrent_version_expiration {
      days = 30                 # v4 renames to: noncurrent_days = 30
    }

    transition {
      days          = 30
      storage_class = "STANDARD_IA"
    }
  }
}

# ============================================================================
# LIMITATION 2: Dynamic Blocks with for_each
# ============================================================================
# When resources use dynamic blocks, the iterator variable creates indirect
# references that can't be tracked by simple token-level prefix matching.
# The migration system doesn't understand that `each.value.ami` refers to
# the same thing as `var.instances["web"].ami`.
#
# What would be needed: expression-level analysis that understands for_each
# iterator bindings and can rename attributes inside dynamic block content.

resource "aws_instance" "dynamic_fleet" {
  for_each = var.instances

  ami           = each.value.ami              # If "ami" is renamed to "image_id",
  instance_type = each.value.instance_type    # this can't be auto-updated because
                                               # the rename_attribute action only
                                               # touches the block's own attributes,
                                               # not the map values feeding for_each.
}

variable "instances" {
  type = map(object({
    ami           = string   # This key name won't be updated
    instance_type = string
  }))
  default = {
    web = {
      ami           = "ami-web123"
      instance_type = "t3.micro"
    }
  }
}

# ============================================================================
# LIMITATION 3: Conditional Resource Creation with count
# ============================================================================
# When a removed resource is conditionally created with count, the migration
# system removes it but can't update count-dependent references like
# aws_db_security_group.legacy[0].name. The reference rewriting works at the
# resource type level but doesn't handle indexed references specially.
#
# What would be needed: reference-aware removal that can find and comment
# out indexed references (resource.name[0].attr, resource.name[*].attr).

resource "aws_db_security_group" "legacy" {
  count = var.use_classic ? 1 : 0
  name  = "legacy-sg"
}

resource "aws_db_instance" "uses_classic" {
  identifier     = "my-db"
  engine         = "mysql"
  instance_class = "db.t3.micro"

  # This indexed reference won't be caught by remove_resource's
  # ReferencesPrefix check because it looks for "aws_db_security_group"
  # as a traversal root, but this expression uses a complex index operation.
  db_security_groups = var.use_classic ? [aws_db_security_group.legacy[0].name] : []
}

variable "use_classic" {
  type    = bool
  default = false
}

# ============================================================================
# LIMITATION 4: Provider Configuration Changes
# ============================================================================
# v5 renames several provider-level attributes. While rename_attribute works
# on provider blocks, authentication precedence changes and new required
# fields can't be expressed as simple migrations.
#
# What would be needed: provider blocks matched via {"block_type": "provider",
# "label": "aws"} work with rename_attribute. But behavioral changes like
# auth credential precedence reordering are not expressible.
#
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-5-upgrade.html.markdown#changes-to-authentication

provider "aws" {
  region = "us-east-1"

  # v5 renames these (automatable):
  # shared_credentials_file → shared_credentials_files
  # s3_force_path_style → s3_use_path_style
  shared_credentials_file = "~/.aws/credentials"
  s3_force_path_style     = true

  # v5 changes auth credential resolution order (NOT automatable):
  # Previously: env vars > shared credentials > IAM role
  # Now: env vars > shared credentials > SSO > IAM role
  # If your config relied on the old precedence, no migration can fix this.
}

# ============================================================================
# LIMITATION 5: Value Type Changes Requiring Expression Rewriting
# ============================================================================
# Some v6 changes convert between types in ways that require understanding
# the full expression, not just pattern-matching a literal value.
#
# replace_value can handle: true → "Enabled", "" → null, 0 → true
# replace_value CANNOT handle: expressions, variables, or interpolations.
#
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-6-upgrade.html.markdown

resource "aws_wafv2_web_acl" "example" {
  name  = "my-acl"
  scope = "REGIONAL"

  # replace_value can fix this literal: true → false
  enable_machine_learning = true

  # But this conditional can't be pattern-matched:
  cloudwatch_metrics_enabled = var.enable_metrics ? true : false

  visibility_config {
    sampled_requests_enabled   = true
    cloudwatch_metrics_enabled = true
    metric_name                = "my-metric"
  }
}

# ============================================================================
# LIMITATION 6: Cross-Resource Wiring After Extraction
# ============================================================================
# When extract_to_resource pulls a nested block into a new resource, it creates
# a basic wiring attribute (e.g., bucket = aws_s3_bucket.X.id). But if other
# resources reference attributes of the *extracted* nested block through the
# parent, those references break silently.
#
# Example: if another resource references aws_s3_bucket.main.versioning[0].enabled,
# that reference won't exist after extraction. The migration system doesn't
# rewrite these cross-resource nested attribute references.
#
# What would be needed: deep reference analysis that tracks nested block
# attribute paths and rewrites them to point at the new standalone resource.

resource "aws_s3_bucket" "main" {
  bucket = "my-bucket"
  versioning {
    enabled = true
  }
}

# After extraction, this reference path breaks because
# aws_s3_bucket.main.versioning no longer exists.
output "versioning_status" {
  value = aws_s3_bucket.main.versioning[0].enabled
}

# ============================================================================
# LIMITATION 7: Import Commands Required After Extraction
# ============================================================================
# After extract_to_resource creates new standalone resources, Terraform will
# see them as new resources to create. The user must run terraform import
# for each extracted resource to adopt existing infrastructure.
#
# This is a fundamental limitation: HCL migration only handles the config
# files, not the state. Users need to run:
#   terraform import aws_s3_bucket_versioning.main <bucket-name>
#
# What would be needed: integration with terraform state commands, or
# generating a shell script of import commands alongside the migration.
