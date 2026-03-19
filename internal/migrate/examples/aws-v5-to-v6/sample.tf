# Sample Terraform configuration demonstrating AWS provider v5 patterns
# that need migration to v6.
#
# Run: terraform migrate run -migrations-dir=. "v5to6/*"

# --- CPU Options Restructure ---
# v6 moves cpu_core_count and cpu_threads_per_core into a cpu_options block.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-6-upgrade.html.markdown#resource-aws_instance

resource "aws_instance" "compute" {
  ami                  = "ami-0c55b159cbfafe1f0"
  instance_type        = "c5.xlarge"
  cpu_core_count       = 2
  cpu_threads_per_core = 1
}

resource "aws_instance" "dynamic_compute" {
  ami                  = var.ami_id
  instance_type        = var.instance_type
  cpu_core_count       = var.cpu_cores
  cpu_threads_per_core = var.env == "prod" ? 2 : 1
}

# --- Batch Compute Environment Name Rename ---
# v6 renames compute_environment_name to name.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-6-upgrade.html.markdown#resource-aws_batch_compute_environment

resource "aws_batch_compute_environment" "batch" {
  compute_environment_name = "my-batch-env"
  type                     = "MANAGED"

  compute_resources {
    type      = "FARGATE"
    max_vcpus = 16
  }
}

# --- S3 Bucket Region Rename ---
# v6 renames 'region' to 'bucket_region'.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-6-upgrade.html.markdown#resource-aws_s3_bucket

resource "aws_s3_bucket" "data" {
  bucket = "my-data-bucket"
  region = "us-east-1"
}

# --- OpsWorks Removal ---
# v6 removes all aws_opsworks_* resources (service discontinued).
# The remove_resource migration removes the block and adds FIXME comments
# to any files that reference it.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-6-upgrade.html.markdown#removal-of-aws_opsworks_-resources

resource "aws_opsworks_stack" "legacy" {
  name   = "my-legacy-stack"
  region = "us-east-1"

  service_role_arn             = aws_iam_role.opsworks.arn
  default_instance_profile_arn = aws_iam_instance_profile.opsworks.arn
}

resource "aws_iam_role" "opsworks" {
  name               = "opsworks-role"
  assume_role_policy = "{}"
}

resource "aws_iam_instance_profile" "opsworks" {
  name = "opsworks-profile"
  role = aws_iam_role.opsworks.name
}

# This output references the opsworks stack and will get a FIXME comment
output "stack_id" {
  value = aws_opsworks_stack.legacy.id
}

# --- Launch Template GPU Removal ---
# v6 removes elastic_gpu_specifications (service discontinued).
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-6-upgrade.html.markdown#resource-aws_launch_template

resource "aws_launch_template" "gpu" {
  name          = "gpu-template"
  instance_type = "p3.2xlarge"
  image_id      = "ami-gpu-123"

  elastic_gpu_specifications {
    type = "eg1.medium"
  }
}

variable "ami_id" {
  type    = string
  default = "ami-0c55b159cbfafe1f0"
}

variable "instance_type" {
  type    = string
  default = "c5.xlarge"
}

variable "cpu_cores" {
  type    = number
  default = 2
}

variable "env" {
  type    = string
  default = "dev"
}
