// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package ast

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// TestMigrationPatterns_ExtractToResource exercises extraction of a nested block
// from one resource into a new standalone resource, wired back to the parent.
//
// Real-world examples:
//   - AWS v3→v4: aws_s3_bucket.versioning {} → new aws_s3_bucket_versioning resource
//   - AWS v3→v4: aws_s3_bucket.logging {} → new aws_s3_bucket_logging resource
func TestMigrationPatterns_ExtractToResource(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		mutate func(t *testing.T, mod *Module)
		want   map[string]string
	}{
		{
			name: "extract_nested_block_to_new_resource",
			files: map[string]string{
				"main.tf": `# S3 bucket with versioning enabled
resource "test_bucket" "main" {
  name = "my-bucket" # the bucket name

  versioning {
    enabled = true
  }
}

/* Output to prove surrounding code is preserved */
output "bucket_id" {
  value = test_bucket.main.id
}
`,
			},
			mutate: func(t *testing.T, mod *Module) {
				for _, r := range mod.FindBlocks("resource", "test_bucket") {
					nested := r.Block.NestedBlocks("versioning")
					if len(nested) == 0 {
						continue
					}
					// Read attributes from nested block before removing it
					attrs := nested[0].Attributes()

					// Remove nested block from parent
					r.Block.RemoveBlock("versioning")

					// Create new standalone resource
					labels := r.Block.Labels()
					newBlock := r.File.AddBlock("resource", []string{"test_bucket_versioning", labels[1]})

					// Wire to parent via traversal
					newBlock.SetAttributeTraversal("bucket", hcl.Traversal{
						hcl.TraverseRoot{Name: "test_bucket"},
						hcl.TraverseAttr{Name: labels[1]},
						hcl.TraverseAttr{Name: "id"},
					})

					// Copy attributes from the extracted nested block
					for name, expr := range attrs {
						newBlock.SetAttributeRaw(name, expr.BuildTokens(nil))
					}
				}
			},
			want: map[string]string{
				"main.tf": `# S3 bucket with versioning enabled
resource "test_bucket" "main" {
  name = "my-bucket" # the bucket name

}

/* Output to prove surrounding code is preserved */
output "bucket_id" {
  value = test_bucket.main.id
}
resource "test_bucket_versioning" "main" {
  bucket  = test_bucket.main.id
  enabled = true
}
`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var files []*File
			for name, content := range tc.files {
				f, err := ParseFile([]byte(content), name, nil)
				if err != nil {
					t.Fatalf("parsing %s: %s", name, err)
				}
				files = append(files, f)
			}
			mod := NewModule(files, "", true, nil)

			tc.mutate(t, mod)

			got := mod.Bytes()
			for name, wantContent := range tc.want {
				gotContent, ok := got[name]
				if !ok {
					t.Errorf("missing output file %s", name)
					continue
				}
				if string(gotContent) != wantContent {
					t.Errorf("file %s mismatch\n--- want ---\n%s\n--- got ---\n%s", name, wantContent, string(gotContent))
				}
			}
		})
	}
}

// TestMigrationPatterns_MoveAttributeToBlock exercises moving a top-level attribute
// into a nested block.
//
// Real-world examples:
//   - AWS v6: aws_instance.cpu_core_count → aws_instance.cpu_options { core_count }
//   - AWS v6: aws_instance.cpu_threads_per_core → aws_instance.cpu_options { threads_per_core }
func TestMigrationPatterns_MoveAttributeToBlock(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		mutate func(t *testing.T, mod *Module)
		want   map[string]string
	}{
		{
			name: "move_attribute_into_nested_block",
			files: map[string]string{
				"main.tf": `# Instance with top-level CPU attributes
resource "test_instance" "web" {
  ami                  = "abc-123" # the base image
  instance_type        = "c5.xlarge"
  cpu_core_count       = 2
  cpu_threads_per_core = 1
}
`,
			},
			mutate: func(t *testing.T, mod *Module) {
				for _, r := range mod.FindBlocks("resource", "test_instance") {
					// Read expressions before removing
					coreExpr := r.Block.GetAttributeExpression("cpu_core_count")
					threadsExpr := r.Block.GetAttributeExpression("cpu_threads_per_core")
					if coreExpr == nil && threadsExpr == nil {
						continue
					}

					// Create nested block
					cpuOpts := r.Block.AddBlock("cpu_options")

					// Move attributes
					if coreExpr != nil {
						cpuOpts.SetAttributeRaw("core_count", coreExpr.BuildTokens(nil))
						r.Block.RemoveAttribute("cpu_core_count")
					}
					if threadsExpr != nil {
						cpuOpts.SetAttributeRaw("threads_per_core", threadsExpr.BuildTokens(nil))
						r.Block.RemoveAttribute("cpu_threads_per_core")
					}
				}
			},
			want: map[string]string{
				"main.tf": `# Instance with top-level CPU attributes
resource "test_instance" "web" {
  ami           = "abc-123" # the base image
  instance_type = "c5.xlarge"
  cpu_options {
    core_count       = 2
    threads_per_core = 1
  }
}
`,
			},
		},
		{
			name: "move_attribute_with_expression_value",
			files: map[string]string{
				"main.tf": `resource "test_instance" "dynamic" {
  ami                  = var.ami_id
  instance_type        = var.instance_type
  cpu_core_count       = var.cpu_cores
  cpu_threads_per_core = var.env == "prod" ? 2 : 1
}
`,
			},
			mutate: func(t *testing.T, mod *Module) {
				for _, r := range mod.FindBlocks("resource", "test_instance") {
					coreExpr := r.Block.GetAttributeExpression("cpu_core_count")
					threadsExpr := r.Block.GetAttributeExpression("cpu_threads_per_core")
					if coreExpr == nil && threadsExpr == nil {
						continue
					}
					cpuOpts := r.Block.AddBlock("cpu_options")
					if coreExpr != nil {
						cpuOpts.SetAttributeRaw("core_count", coreExpr.BuildTokens(nil))
						r.Block.RemoveAttribute("cpu_core_count")
					}
					if threadsExpr != nil {
						cpuOpts.SetAttributeRaw("threads_per_core", threadsExpr.BuildTokens(nil))
						r.Block.RemoveAttribute("cpu_threads_per_core")
					}
				}
			},
			want: map[string]string{
				"main.tf": `resource "test_instance" "dynamic" {
  ami           = var.ami_id
  instance_type = var.instance_type
  cpu_options {
    core_count       = var.cpu_cores
    threads_per_core = var.env == "prod" ? 2 : 1
  }
}
`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var files []*File
			for name, content := range tc.files {
				f, err := ParseFile([]byte(content), name, nil)
				if err != nil {
					t.Fatalf("parsing %s: %s", name, err)
				}
				files = append(files, f)
			}
			mod := NewModule(files, "", true, nil)

			tc.mutate(t, mod)

			got := mod.Bytes()
			for name, wantContent := range tc.want {
				gotContent, ok := got[name]
				if !ok {
					t.Errorf("missing output file %s", name)
					continue
				}
				if string(gotContent) != wantContent {
					t.Errorf("file %s mismatch\n--- want ---\n%s\n--- got ---\n%s", name, wantContent, string(gotContent))
				}
			}
		})
	}
}

// TestMigrationPatterns_FlattenBlock exercises inlining nested block attributes
// into the parent block.
//
// Real-world examples:
//   - AWS v5: aws_elasticache_replication_group.cluster_mode { num_node_groups, replicas_per_node_group }
//     → top-level num_node_groups, replicas_per_node_group
func TestMigrationPatterns_FlattenBlock(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		mutate func(t *testing.T, mod *Module)
		want   map[string]string
	}{
		{
			name: "flatten_nested_block_to_parent",
			files: map[string]string{
				"main.tf": `# Replication group with cluster_mode block
resource "test_replication_group" "main" {
  description = "my cluster" # the description

  cluster_mode {
    num_node_groups         = 3
    replicas_per_node_group = 2
  }
}
`,
			},
			mutate: func(t *testing.T, mod *Module) {
				for _, r := range mod.FindBlocks("resource", "test_replication_group") {
					nested := r.Block.NestedBlocks("cluster_mode")
					if len(nested) == 0 {
						continue
					}
					// Read specific attributes by name for deterministic order
					attrNames := []string{"num_node_groups", "replicas_per_node_group"}
					exprs := make(map[string]*hclwrite.Expression)
					for _, name := range attrNames {
						if expr := nested[0].GetAttributeExpression(name); expr != nil {
							exprs[name] = expr
						}
					}

					// Remove the nested block
					r.Block.RemoveBlock("cluster_mode")

					// Add attributes to parent in deterministic order
					for _, name := range attrNames {
						if expr, ok := exprs[name]; ok {
							r.Block.SetAttributeRaw(name, expr.BuildTokens(nil))
						}
					}
				}
			},
			want: map[string]string{
				"main.tf": `# Replication group with cluster_mode block
resource "test_replication_group" "main" {
  description = "my cluster" # the description

  num_node_groups         = 3
  replicas_per_node_group = 2
}
`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var files []*File
			for name, content := range tc.files {
				f, err := ParseFile([]byte(content), name, nil)
				if err != nil {
					t.Fatalf("parsing %s: %s", name, err)
				}
				files = append(files, f)
			}
			mod := NewModule(files, "", true, nil)

			tc.mutate(t, mod)

			got := mod.Bytes()
			for name, wantContent := range tc.want {
				gotContent, ok := got[name]
				if !ok {
					t.Errorf("missing output file %s", name)
					continue
				}
				if string(gotContent) != wantContent {
					t.Errorf("file %s mismatch\n--- want ---\n%s\n--- got ---\n%s", name, wantContent, string(gotContent))
				}
			}
		})
	}
}

// TestMigrationPatterns_RemoveResourceWithRefWarnings exercises removing a resource
// block and adding FIXME comments to all files that reference it.
//
// Real-world examples:
//   - AWS v6: aws_opsworks_* resources removed (17 resources)
//   - AWS v5: aws_db_security_group, aws_redshift_security_group removed (EC2-Classic)
func TestMigrationPatterns_RemoveResourceWithRefWarnings(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		mutate func(t *testing.T, mod *Module)
		want   map[string]string
	}{
		{
			name: "remove_resource_and_warn_at_reference_sites",
			files: map[string]string{
				"main.tf": `# OpsWorks stack definition
resource "test_opsworks_stack" "main" {
  name   = "my-stack" # the stack name
  region = "us-east-1"
}

// A non-related resource in the same file
resource "test_instance" "web" {
  ami           = "abc-123"
  instance_type = "t2.micro"
}
`,
				"references.tf": `# Layer referencing the stack
resource "test_opsworks_layer" "app" {
  stack_id = test_opsworks_stack.main.id
  name     = "app-layer"
}

output "stack_id" {
  value = test_opsworks_stack.main.id
}
`,
				"unrelated.tf": `# File with no references to the removed resource
resource "test_bucket" "logs" {
  name = "logs-bucket"
}
`,
			},
			mutate: func(t *testing.T, mod *Module) {
				results := mod.FindBlocks("resource", "test_opsworks_stack")
				if len(results) == 0 {
					return
				}

				// Collect which files reference this resource
				prefix := hcl.Traversal{hcl.TraverseRoot{Name: "test_opsworks_stack"}}

				// Track files that have references (deduplicate)
				warned := make(map[string]bool)
				for _, f := range mod.Files() {
					if f.ReferencesPrefix(prefix) {
						if !warned[f.Filename()] {
							f.AppendComment("FIXME: test_opsworks_stack has been removed. Update references manually.")
							warned[f.Filename()] = true
						}
					}
				}

				// Remove the resource blocks
				for _, r := range results {
					labels := r.Block.Labels()
					r.File.RemoveBlock("resource", append([]string{"test_opsworks_stack"}, labels[1:]...))
				}
			},
			want: map[string]string{
				"main.tf": `
// A non-related resource in the same file
resource "test_instance" "web" {
  ami           = "abc-123"
  instance_type = "t2.micro"
}
`,
				"references.tf": `# Layer referencing the stack
resource "test_opsworks_layer" "app" {
  stack_id = test_opsworks_stack.main.id
  name     = "app-layer"
}

output "stack_id" {
  value = test_opsworks_stack.main.id
}

# FIXME: test_opsworks_stack has been removed. Update references manually.
`,
				"unrelated.tf": `# File with no references to the removed resource
resource "test_bucket" "logs" {
  name = "logs-bucket"
}
`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var files []*File
			for name, content := range tc.files {
				f, err := ParseFile([]byte(content), name, nil)
				if err != nil {
					t.Fatalf("parsing %s: %s", name, err)
				}
				files = append(files, f)
			}
			mod := NewModule(files, "", true, nil)

			tc.mutate(t, mod)

			got := mod.Bytes()
			for name, wantContent := range tc.want {
				gotContent, ok := got[name]
				if !ok {
					t.Errorf("missing output file %s", name)
					continue
				}
				if string(gotContent) != wantContent {
					t.Errorf("file %s mismatch\n--- want ---\n%s\n--- got ---\n%s", name, wantContent, string(gotContent))
				}
			}
			// Verify no extra files appeared
			for name := range got {
				if _, ok := tc.want[name]; !ok {
					t.Errorf("unexpected output file %s", name)
				}
			}
		})
	}
}

