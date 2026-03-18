// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform/internal/ast"
)

// TestIntegration_AWS_v3to4_s3_rename tests a realistic v3→v4 migration:
// aws_s3_bucket_object → aws_s3_object with reference rewriting.
func TestIntegration_AWS_v3to4_s3_rename(t *testing.T) {
	tfSrc := `resource "aws_s3_bucket_object" "config" {
  bucket = aws_s3_bucket.main.id
  key    = "config.json"
}

output "object_id" {
  value = aws_s3_bucket_object.config.id
}
`
	migration := &Migration{
		Name:        "v3to4/rename_s3_bucket_object",
		Description: "Rename aws_s3_bucket_object to aws_s3_object",
		Match:       Match{BlockType: "resource", Label: "aws_s3_bucket_object"},
		Actions: []Action{
			{Action: "rename_resource", To: "aws_s3_object"},
		},
	}

	f, err := ast.ParseFile([]byte(tfSrc), "main.tf", nil)
	if err != nil {
		t.Fatal(err)
	}
	mod := ast.NewModule([]*ast.File{f}, "", true, nil)

	if err := Execute(migration, mod); err != nil {
		t.Fatal(err)
	}

	got := string(mod.Bytes()["main.tf"])
	if !strings.Contains(got, `resource "aws_s3_object" "config"`) {
		t.Errorf("expected resource renamed:\n%s", got)
	}
	if !strings.Contains(got, "aws_s3_object.config.id") {
		t.Errorf("expected references renamed:\n%s", got)
	}
	if strings.Contains(got, "aws_s3_bucket_object") {
		t.Errorf("expected no remaining aws_s3_bucket_object:\n%s", got)
	}
}

// TestIntegration_AWS_v4to5_multi exercises a realistic v4→v5 migration
// combining attribute renames and removals on aws_instance.
func TestIntegration_AWS_v4to5_multi(t *testing.T) {
	tfSrc := `resource "aws_instance" "web" {
  ami                 = "ami-123"
  instance_type       = "t2.micro"
  vpc_classic_link_id = "vpc-abc"
}

resource "aws_instance" "api" {
  ami           = var.api_ami
  instance_type = "t3.small"
}
`
	migrations := []*Migration{
		{
			Name:  "v4to5/remove_ec2_classic",
			Match: Match{BlockType: "resource", Label: "aws_instance"},
			Actions: []Action{
				{Action: "remove_attribute", Name: "vpc_classic_link_id"},
			},
		},
	}

	f, err := ast.ParseFile([]byte(tfSrc), "main.tf", nil)
	if err != nil {
		t.Fatal(err)
	}
	mod := ast.NewModule([]*ast.File{f}, "", true, nil)

	for _, m := range migrations {
		if err := Execute(m, mod); err != nil {
			t.Fatal(err)
		}
	}

	got := string(mod.Bytes()["main.tf"])
	if strings.Contains(got, "vpc_classic_link_id") {
		t.Errorf("expected vpc_classic_link_id removed:\n%s", got)
	}
	if !strings.Contains(got, "ami") {
		t.Errorf("expected ami preserved:\n%s", got)
	}
}

// TestIntegration_fullPipeline tests discover → filter → execute → write.
func TestIntegration_fullPipeline(t *testing.T) {
	// Set up migration files
	migDir := t.TempDir()
	v3to4 := filepath.Join(migDir, "v3to4")
	os.MkdirAll(v3to4, 0755)

	writeJSON(t, filepath.Join(v3to4, "rename_attr.json"), &Migration{
		Name:  "v3to4/rename_attr",
		Match: Match{BlockType: "resource", Label: "aws_instance"},
		Actions: []Action{
			{Action: "rename_attribute", From: "ami", To: "image_id"},
		},
	})

	// Set up .tf files
	tfDir := t.TempDir()
	os.WriteFile(filepath.Join(tfDir, "main.tf"), []byte(`resource "aws_instance" "web" {
  ami           = "ami-123"
  instance_type = "t2.micro"
}
`), 0644)

	// Discover
	migrations, err := DiscoverMigrations(migDir)
	if err != nil {
		t.Fatal(err)
	}
	migrations = FilterMigrations(migrations, "v3to4/*")
	if len(migrations) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(migrations))
	}

	// Load module
	entries, _ := os.ReadDir(tfDir)
	var files []*ast.File
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}
		src, _ := os.ReadFile(filepath.Join(tfDir, e.Name()))
		f, err := ast.ParseFile(src, filepath.Join(tfDir, e.Name()), nil)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	mod := ast.NewModule(files, tfDir, true, nil)

	// Execute
	for _, m := range migrations {
		if err := Execute(m, mod); err != nil {
			t.Fatal(err)
		}
	}

	// Write back
	for filename, content := range mod.Bytes() {
		os.WriteFile(filename, content, 0644)
	}

	// Verify
	result, _ := os.ReadFile(filepath.Join(tfDir, "main.tf"))
	got := string(result)
	if !strings.Contains(got, "image_id") {
		t.Errorf("expected image_id in output:\n%s", got)
	}
	if strings.Contains(got, "  ami ") {
		t.Errorf("expected ami renamed:\n%s", got)
	}
}
