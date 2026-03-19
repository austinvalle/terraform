# Sample Terraform configuration demonstrating AWS provider v4 patterns
# that need migration to v5.
#
# Run: terraform migrate run -migrations-dir=. "v4to5/*"

# --- EC2-Classic Attribute Removal ---
# v5 removes all EC2-Classic support. These attributes no longer exist.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-5-upgrade.html.markdown#ec2-classic-resource-and-data-source-removal

resource "aws_instance" "web" {
  ami                                = "ami-0c55b159cbfafe1f0"
  instance_type                      = "t3.micro"
  security_groups                    = ["default"]
  vpc_classic_link_id                = "vpc-abc123"
  vpc_classic_link_security_groups   = ["sg-abc123"]
  vpc_security_group_ids             = [aws_security_group.web.id]
}

resource "aws_security_group" "web" {
  name = "web-sg"
}

# --- Autoscaling Attachment Rename ---
# v5 renames alb_target_group_arn to lb_target_group_arn.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-5-upgrade.html.markdown#resource-aws_autoscaling_attachment

resource "aws_autoscaling_attachment" "asg_alb" {
  autoscaling_group_name = "my-asg"
  alb_target_group_arn   = "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/my-tg/abc123"
}

# --- Elasticache Replication Group ---
# v5 renames several attributes and removes the cluster_mode block.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-5-upgrade.html.markdown#resource-aws_elasticache_replication_group

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id          = "my-redis"
  replication_group_description = "Production Redis cluster"
  node_type                     = "cache.r6g.large"
  number_cache_clusters         = 3
  availability_zones            = ["us-east-1a", "us-east-1b", "us-east-1c"]

  cluster_mode {
    num_node_groups         = 3
    replicas_per_node_group = 2
  }
}

# --- DB Instance Name Rename ---
# v5 renames 'name' to 'db_name' on aws_db_instance.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-5-upgrade.html.markdown#resource-aws_db_instance

resource "aws_db_instance" "postgres" {
  identifier     = "my-postgres"
  engine         = "postgres"
  engine_version = "14.1"
  instance_class = "db.t3.micro"
  name           = "myappdb"
  username       = "admin"
  password       = var.db_password
}

# --- DB Security Group Removal (EC2-Classic) ---
# v5 removes aws_db_security_group entirely.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-5-upgrade.html.markdown#ec2-classic-resource-and-data-source-removal

resource "aws_db_security_group" "legacy" {
  name = "legacy-db-sg"
}

resource "aws_db_instance" "legacy_db" {
  identifier     = "legacy"
  engine         = "mysql"
  instance_class = "db.t3.micro"
  name           = "legacydb"
  username       = "admin"
  password       = var.db_password
  # This reference will get a FIXME comment when the security group is removed
  db_security_groups = [aws_db_security_group.legacy.name]
}

# --- OpenSearch Kibana Rename ---
# v5 renames kibana_endpoint to dashboard_endpoint.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-5-upgrade.html.markdown#resource-aws_opensearch_domain

resource "aws_opensearch_domain" "search" {
  domain_name    = "my-search"
  engine_version = "OpenSearch_2.3"
}

output "search_dashboard" {
  value = aws_opensearch_domain.search.kibana_endpoint
}

# --- RDS Cluster Engine Now Required ---
# v5 removes the default for engine on aws_rds_cluster.
# The add_attribute action only sets it if not already present.
# See: https://github.com/hashicorp/terraform-provider-aws/blob/main/website/docs/guides/version-5-upgrade.html.markdown#resource-aws_rds_cluster

resource "aws_rds_cluster" "aurora" {
  cluster_identifier = "my-aurora"
  master_username    = "admin"
  master_password    = var.db_password
}

resource "aws_rds_cluster" "aurora_mysql" {
  cluster_identifier = "my-aurora-mysql"
  engine             = "aurora-mysql"
  master_username    = "admin"
  master_password    = var.db_password
}

variable "db_password" {
  type      = string
  sensitive = true
}
