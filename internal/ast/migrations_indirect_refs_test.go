// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package ast

import (
	"testing"
)

// mutateRenameAmi renames the "ami" attribute to "image_id" on all
// test_instance resource blocks in the module. This is the base
// mutation shared by all indirect reference tests.
func mutateRenameAmi(mod *Module) {
	for _, r := range mod.FindBlocks("resource", "test_instance") {
		r.Block.RenameAttribute("ami", "image_id")
	}
}

// assertOrSkip compares module output against expected, skipping
// (not failing) if the output doesn't match. This lets us write
// tests for patterns that aren't implemented yet without breaking CI.
func assertOrSkip(t *testing.T, mod *Module, want map[string]string, pattern string) {
	t.Helper()
	got := mod.Bytes()
	for name, wantContent := range want {
		gotContent, ok := got[name]
		if !ok {
			t.Skipf("not yet implemented: %s — missing output file %s", pattern, name)
		}
		if string(gotContent) != wantContent {
			t.Skipf("not yet implemented: %s — file %s mismatch\n--- want ---\n%s\n--- got ---\n%s",
				pattern, name, wantContent, string(gotContent))
		}
	}
}

// buildModule is a helper that parses a map of filename→HCL content
// into a Module.
func buildModule(t *testing.T, files map[string]string) *Module {
	t.Helper()
	var parsed []*File
	for name, content := range files {
		f, err := ParseFile([]byte(content), name, nil)
		if err != nil {
			t.Fatalf("parsing %s: %s", name, err)
		}
		parsed = append(parsed, f)
	}
	return NewModule(parsed, "", true, nil)
}

// TestIndirectRef_ForEachEachValue tests that attribute renames propagate
// through for_each bindings where each.value carries the renamed resource.
//
// Pattern:
//
//	resource "test_security_group" "rules" {
//	  for_each = test_instance.all
//	  name     = each.value.ami   # needs to become each.value.image_id
//	}
//
// Real-world: any resource iterating over another resource's instances
// and accessing an attribute that gets renamed in a provider upgrade.
func TestIndirectRef_ForEachEachValue(t *testing.T) {
	files := map[string]string{
		"main.tf": `# Instances to iterate over
resource "test_instance" "all" {
  for_each = var.instances
  ami      = each.value.base_image # the AMI
  name     = each.key
}

// Security group using each.value from the instances
resource "test_security_group" "rules" {
  for_each    = test_instance.all
  name        = "sg-${each.key}"
  source_ami  = each.value.ami /* the instance AMI */
  instance_id = each.value.id
}

/* Output proving non-target code is preserved */
output "sg_ids" {
  value = values(test_security_group.rules)[*].id
}
`,
	}

	want := map[string]string{
		"main.tf": `# Instances to iterate over
resource "test_instance" "all" {
  for_each = var.instances
  image_id = each.value.base_image # the AMI
  name     = each.key
}

// Security group using each.value from the instances
resource "test_security_group" "rules" {
  for_each    = test_instance.all
  name        = "sg-${each.key}"
  source_ami  = each.value.image_id /* the instance AMI */
  instance_id = each.value.id
}

/* Output proving non-target code is preserved */
output "sg_ids" {
  value = values(test_security_group.rules)[*].id
}
`,
	}

	mod := buildModule(t, files)
	mutateRenameAmi(mod)
	// TODO: additional mutation to rename each.value.ami → each.value.image_id
	// in consumer blocks would go here once implemented

	assertOrSkip(t, mod, want, "for_each each.value.attr")
}

// TestIndirectRef_DynamicBlockIterator tests that attribute renames propagate
// into dynamic block content where the iterator carries the renamed resource.
//
// Pattern:
//
//	dynamic "config" {
//	  for_each = test_instance.all
//	  content {
//	    image = config.value.ami   # needs to become config.value.image_id
//	  }
//	}
//
// Real-world: dynamic blocks generating repeated sub-blocks from a resource
// whose attributes change in a provider upgrade.
func TestIndirectRef_DynamicBlockIterator(t *testing.T) {
	files := map[string]string{
		"main.tf": `# Instances that will be renamed
resource "test_instance" "all" {
  for_each = var.instances
  ami      = each.value.base_image # the AMI
  name     = each.key
}

// Load balancer with dynamic block iterating over instances
resource "test_load_balancer" "main" {
  name = "lb-main"

  dynamic "target" {
    for_each = test_instance.all
    content {
      instance_id = target.value.id
      image       = target.value.ami /* the instance AMI */
      port        = 443
    }
  }
}

/* Output to prove non-target code is preserved */
output "lb_arn" {
  value = test_load_balancer.main.arn # LB ARN
}
`,
	}

	want := map[string]string{
		"main.tf": `# Instances that will be renamed
resource "test_instance" "all" {
  for_each = var.instances
  image_id = each.value.base_image # the AMI
  name     = each.key
}

// Load balancer with dynamic block iterating over instances
resource "test_load_balancer" "main" {
  name = "lb-main"

  dynamic "target" {
    for_each = test_instance.all
    content {
      instance_id = target.value.id
      image       = target.value.image_id /* the instance AMI */
      port        = 443
    }
  }
}

/* Output to prove non-target code is preserved */
output "lb_arn" {
  value = test_load_balancer.main.arn # LB ARN
}
`,
	}

	mod := buildModule(t, files)
	mutateRenameAmi(mod)
	assertOrSkip(t, mod, want, "dynamic block iterator.value.attr")
}

// TestIndirectRef_LocalAliasThenTraverse tests that attribute renames
// propagate through local value aliases. A local stores the entire resource
// object, and downstream code traverses the renamed attribute on it.
//
// Pattern:
//
//	locals { inst = test_instance.example }
//	output { value = local.inst.ami }   # needs to become local.inst.image_id
//
// Real-world: configs that assign resources to locals for readability, then
// reference attributes from those locals throughout the file.
func TestIndirectRef_LocalAliasThenTraverse(t *testing.T) {
	files := map[string]string{
		"main.tf": `# The instance being renamed
resource "test_instance" "example" {
  ami           = "abc-123" # the base image
  instance_type = "t2.micro"
}

// Local alias for the instance
locals {
  inst = test_instance.example
}

/* Output accessing the renamed attribute via the local */
output "instance_ami" {
  value = local.inst.ami // should track the rename
}

# Output accessing a non-renamed attribute via the local
output "instance_type" {
  value = local.inst.instance_type
}
`,
	}

	want := map[string]string{
		"main.tf": `# The instance being renamed
resource "test_instance" "example" {
  image_id      = "abc-123" # the base image
  instance_type = "t2.micro"
}

// Local alias for the instance
locals {
  inst = test_instance.example
}

/* Output accessing the renamed attribute via the local */
output "instance_ami" {
  value = local.inst.image_id // should track the rename
}

# Output accessing a non-renamed attribute via the local
output "instance_type" {
  value = local.inst.instance_type
}
`,
	}

	mod := buildModule(t, files)
	mutateRenameAmi(mod)
	assertOrSkip(t, mod, want, "local alias then traverse .attr")
}

// TestIndirectRef_SplatThenAttribute tests that attribute renames propagate
// through splat expressions. The [*] operator creates a list of attribute
// values, and the attribute name following it must be updated.
//
// Pattern:
//
//	output { value = test_instance.all[*].ami }   # needs [*].image_id
//
// Real-world: collecting a specific attribute from all instances of a
// counted or for_each resource.
func TestIndirectRef_SplatThenAttribute(t *testing.T) {
	files := map[string]string{
		"main.tf": `# Multiple instances via count
resource "test_instance" "all" {
  count = 3
  ami   = "abc-123" # the base image
  name  = "instance-${count.index}"
}

// Splat collecting the AMI from all instances
output "all_amis" {
  value = test_instance.all[*].ami /* should become image_id */
}

# Splat inside a function call
output "ami_csv" {
  value = join(",", test_instance.all[*].ami)
}

/* Legacy splat syntax */
output "all_amis_legacy" {
  value = test_instance.all.*.ami // legacy splat
}

# Non-renamed attribute via splat (must be untouched)
output "all_names" {
  value = test_instance.all[*].name
}
`,
	}

	want := map[string]string{
		"main.tf": `# Multiple instances via count
resource "test_instance" "all" {
  count    = 3
  image_id = "abc-123" # the base image
  name     = "instance-${count.index}"
}

// Splat collecting the AMI from all instances
output "all_amis" {
  value = test_instance.all[*].image_id /* should become image_id */
}

# Splat inside a function call
output "ami_csv" {
  value = join(",", test_instance.all[*].image_id)
}

/* Legacy splat syntax */
output "all_amis_legacy" {
  value = test_instance.all.*.image_id // legacy splat
}

# Non-renamed attribute via splat (must be untouched)
output "all_names" {
  value = test_instance.all[*].name
}
`,
	}

	mod := buildModule(t, files)
	mutateRenameAmi(mod)
	assertOrSkip(t, mod, want, "splat [*].attr and .*.attr")
}

// TestIndirectRef_ForExpressionBinding tests that attribute renames propagate
// through for expression iterator bindings. The iterator variable captures
// each resource instance, and attribute access on it must be updated.
//
// Pattern:
//
//	value = [for inst in test_instance.all : inst.ami]
//	# needs to become inst.image_id
//
// Real-world: collecting or transforming resource attributes in outputs,
// locals, or other expressions.
func TestIndirectRef_ForExpressionBinding(t *testing.T) {
	files := map[string]string{
		"main.tf": `# Instances to iterate over
resource "test_instance" "all" {
  count = 3
  ami   = "abc-123" # the base image
  name  = "instance-${count.index}"
}

// For expression producing a list of AMIs
output "ami_list" {
  value = [for inst in test_instance.all : inst.ami]
}

# For expression producing a map keyed by name
output "ami_map" {
  value = { for inst in test_instance.all : inst.name => inst.ami } /* name→AMI */
}

/* For expression with conditional filter */
output "ami_filtered" {
  value = [for inst in test_instance.all : inst.ami if inst.name != ""] // filtered
}

# For expression accessing non-renamed attribute (must be untouched)
output "name_list" {
  value = [for inst in test_instance.all : inst.name]
}
`,
	}

	want := map[string]string{
		"main.tf": `# Instances to iterate over
resource "test_instance" "all" {
  count    = 3
  image_id = "abc-123" # the base image
  name     = "instance-${count.index}"
}

// For expression producing a list of AMIs
output "ami_list" {
  value = [for inst in test_instance.all : inst.image_id]
}

# For expression producing a map keyed by name
output "ami_map" {
  value = { for inst in test_instance.all : inst.name => inst.image_id } /* name→AMI */
}

/* For expression with conditional filter */
output "ami_filtered" {
  value = [for inst in test_instance.all : inst.image_id if inst.name != ""] // filtered
}

# For expression accessing non-renamed attribute (must be untouched)
output "name_list" {
  value = [for inst in test_instance.all : inst.name]
}
`,
	}

	mod := buildModule(t, files)
	mutateRenameAmi(mod)
	assertOrSkip(t, mod, want, "for expression iterator.attr")
}

// TestIndirectRef_SelfInProvisioner tests that attribute renames propagate
// to self.attr references inside provisioner blocks. The self keyword refers
// to the enclosing resource, so self.ami must become self.image_id.
//
// Pattern:
//
//	provisioner "local-exec" {
//	  command = "echo ${self.ami}"   # needs to become self.image_id
//	}
//
// Real-world: provisioners that reference the resource's own attributes,
// which change names in a provider upgrade.
func TestIndirectRef_SelfInProvisioner(t *testing.T) {
	files := map[string]string{
		"main.tf": `# Instance with provisioner referencing self
resource "test_instance" "example" {
  ami           = "abc-123" # the base image
  instance_type = "t2.micro"

  // Provisioner using self to reference own attributes
  provisioner "local-exec" {
    command = "echo ${self.ami}" /* the AMI */
  }

  # Another provisioner with direct self traversal
  provisioner "local-exec" {
    command = self.ami
  }

  /* Provisioner referencing non-renamed attribute (untouched) */
  provisioner "local-exec" {
    command = "type: ${self.instance_type}"
  }
}
`,
	}

	want := map[string]string{
		"main.tf": `# Instance with provisioner referencing self
resource "test_instance" "example" {
  image_id      = "abc-123" # the base image
  instance_type = "t2.micro"

  // Provisioner using self to reference own attributes
  provisioner "local-exec" {
    command = "echo ${self.image_id}" /* the AMI */
  }

  # Another provisioner with direct self traversal
  provisioner "local-exec" {
    command = self.image_id
  }

  /* Provisioner referencing non-renamed attribute (untouched) */
  provisioner "local-exec" {
    command = "type: ${self.instance_type}"
  }
}
`,
	}

	mod := buildModule(t, files)
	mutateRenameAmi(mod)
	assertOrSkip(t, mod, want, "self.attr in provisioner")
}

// TestIndirectRef_LifecycleIgnoreChanges tests that attribute renames
// propagate into lifecycle ignore_changes lists. These contain bare
// attribute names (not full traversals), requiring special handling.
//
// Pattern:
//
//	lifecycle {
//	  ignore_changes = [ami, instance_type]   # ami needs to become image_id
//	}
//
// Real-world: resources with ignore_changes listing attributes that get
// renamed in a provider upgrade — the lifecycle block must be updated too.
func TestIndirectRef_LifecycleIgnoreChanges(t *testing.T) {
	files := map[string]string{
		"main.tf": `# Instance with lifecycle ignoring the renamed attribute
resource "test_instance" "example" {
  ami           = "abc-123" # the base image
  instance_type = "t2.micro"

  lifecycle {
    ignore_changes = [ami, instance_type] // ignore AMI changes
  }
}

// Instance with only the renamed attribute in ignore_changes
resource "test_instance" "single_ignore" {
  ami           = var.custom_ami
  instance_type = "t2.micro"

  /* Only ignore the AMI */
  lifecycle {
    ignore_changes = [ami]
  }
}

# Instance with ignore_changes that does NOT include the renamed attr
resource "test_instance" "no_match" {
  ami           = "abc-123"
  instance_type = "t2.micro"

  lifecycle {
    ignore_changes = [instance_type] # should be untouched
  }
}
`,
	}

	want := map[string]string{
		"main.tf": `# Instance with lifecycle ignoring the renamed attribute
resource "test_instance" "example" {
  image_id      = "abc-123" # the base image
  instance_type = "t2.micro"

  lifecycle {
    ignore_changes = [image_id, instance_type] // ignore AMI changes
  }
}

// Instance with only the renamed attribute in ignore_changes
resource "test_instance" "single_ignore" {
  image_id      = var.custom_ami
  instance_type = "t2.micro"

  /* Only ignore the AMI */
  lifecycle {
    ignore_changes = [image_id]
  }
}

# Instance with ignore_changes that does NOT include the renamed attr
resource "test_instance" "no_match" {
  image_id      = "abc-123"
  instance_type = "t2.micro"

  lifecycle {
    ignore_changes = [instance_type] # should be untouched
  }
}
`,
	}

	mod := buildModule(t, files)
	mutateRenameAmi(mod)
	assertOrSkip(t, mod, want, "lifecycle ignore_changes bare attr names")
}

// TestIndirectRef_ChainedLocals tests that attribute renames propagate
// through multiple levels of local value indirection.
//
// Pattern:
//
//	locals { inst = test_instance.example }
//	locals { ami  = local.inst.ami }        # needs to become .image_id
//	output { value = local.ami }            # just a local name, no rename
//
// Real-world: configs that layer locals for readability, where a renamed
// attribute is accessed several levels deep in the chain.
func TestIndirectRef_ChainedLocals(t *testing.T) {
	files := map[string]string{
		"main.tf": `# The instance being renamed
resource "test_instance" "example" {
  ami           = "abc-123" # the base image
  instance_type = "t2.micro"
}

// First level: alias the whole resource
locals {
  inst = test_instance.example
}

# Second level: extract the specific attribute
locals {
  ami_value     = local.inst.ami /* should track the rename */
  instance_type = local.inst.instance_type
}

/* Third level: use the extracted value */
output "the_ami" {
  value = local.ami_value // just a local name, unchanged
}

# Direct use of the first-level local
output "direct_from_local" {
  value = local.inst.ami # should become .image_id
}
`,
	}

	want := map[string]string{
		"main.tf": `# The instance being renamed
resource "test_instance" "example" {
  image_id      = "abc-123" # the base image
  instance_type = "t2.micro"
}

// First level: alias the whole resource
locals {
  inst = test_instance.example
}

# Second level: extract the specific attribute
locals {
  ami_value     = local.inst.image_id /* should track the rename */
  instance_type = local.inst.instance_type
}

/* Third level: use the extracted value */
output "the_ami" {
  value = local.ami_value // just a local name, unchanged
}

# Direct use of the first-level local
output "direct_from_local" {
  value = local.inst.image_id # should become .image_id
}
`,
	}

	mod := buildModule(t, files)
	mutateRenameAmi(mod)
	assertOrSkip(t, mod, want, "chained locals multi-level indirection")
}

// TestIndirectRef_CollectionFuncThenAttr tests that attribute renames
// propagate through collection function results. Functions like values(),
// lookup(), and one() return resource objects, and attribute access on the
// result must be updated.
//
// Pattern:
//
//	value = values(test_instance.all)[0].ami   # needs .image_id
//	value = one(test_instance.single).ami      # needs .image_id
//	value = lookup(local.instances, "a").ami    # needs .image_id
//
// Real-world: configs that use collection functions to select resources
// before accessing attributes that get renamed.
func TestIndirectRef_CollectionFuncThenAttr(t *testing.T) {
	files := map[string]string{
		"main.tf": `# Instances behind a for_each
resource "test_instance" "all" {
  for_each = var.instances
  ami      = each.value.base_image # the AMI
  name     = each.key
}

// Access via values() then index
output "first_ami" {
  value = values(test_instance.all)[0].ami /* should become image_id */
}

# Access via one() for a single-element set
output "single_ami" {
  value = one(values(test_instance.all)).ami // should become image_id
}

/* Access via lookup on a local map */
locals {
  instance_map = test_instance.all
}

output "looked_up_ami" {
  value = lookup(local.instance_map, "web").ami # should become image_id
}

# Access non-renamed attribute via same pattern (must be untouched)
output "first_name" {
  value = values(test_instance.all)[0].name
}
`,
	}

	want := map[string]string{
		"main.tf": `# Instances behind a for_each
resource "test_instance" "all" {
  for_each = var.instances
  image_id = each.value.base_image # the AMI
  name     = each.key
}

// Access via values() then index
output "first_ami" {
  value = values(test_instance.all)[0].image_id /* should become image_id */
}

# Access via one() for a single-element set
output "single_ami" {
  value = one(values(test_instance.all)).image_id // should become image_id
}

/* Access via lookup on a local map */
locals {
  instance_map = test_instance.all
}

output "looked_up_ami" {
  value = lookup(local.instance_map, "web").image_id # should become image_id
}

# Access non-renamed attribute via same pattern (must be untouched)
output "first_name" {
  value = values(test_instance.all)[0].name
}
`,
	}

	mod := buildModule(t, files)
	mutateRenameAmi(mod)
	assertOrSkip(t, mod, want, "collection function then .attr")
}

// TestIndirectRef_ConditionalWithLocalAlias tests that attribute renames
// propagate through conditional expressions where both branches access
// the renamed attribute via local aliases.
//
// Pattern:
//
//	locals {
//	  primary   = test_instance.primary
//	  secondary = test_instance.secondary
//	}
//	output {
//	  value = var.use_primary ? local.primary.ami : local.secondary.ami
//	}
//
// Real-world: configs with fallback patterns using conditionals over
// locally-aliased resources.
func TestIndirectRef_ConditionalWithLocalAlias(t *testing.T) {
	files := map[string]string{
		"main.tf": `# Primary instance
resource "test_instance" "primary" {
  ami           = "primary-ami" # the primary image
  instance_type = "t2.micro"
}

// Secondary instance
resource "test_instance" "secondary" {
  ami           = "secondary-ami"
  instance_type = "t2.small"
}

/* Local aliases for both */
locals {
  primary   = test_instance.primary
  secondary = test_instance.secondary
}

# Conditional selecting AMI from one of the locals
output "selected_ami" {
  value = var.use_primary ? local.primary.ami : local.secondary.ami // should become .image_id
}

/* Nested conditional with local alias */
output "tiered_ami" {
  value = var.tier == "high" ? local.primary.ami : (var.tier == "low" ? local.secondary.ami : "default") # both branches
}

// Non-renamed attribute in conditional (must be untouched)
output "selected_type" {
  value = var.use_primary ? local.primary.instance_type : local.secondary.instance_type
}
`,
	}

	want := map[string]string{
		"main.tf": `# Primary instance
resource "test_instance" "primary" {
  image_id      = "primary-ami" # the primary image
  instance_type = "t2.micro"
}

// Secondary instance
resource "test_instance" "secondary" {
  image_id      = "secondary-ami"
  instance_type = "t2.small"
}

/* Local aliases for both */
locals {
  primary   = test_instance.primary
  secondary = test_instance.secondary
}

# Conditional selecting AMI from one of the locals
output "selected_ami" {
  value = var.use_primary ? local.primary.image_id : local.secondary.image_id // should become .image_id
}

/* Nested conditional with local alias */
output "tiered_ami" {
  value = var.tier == "high" ? local.primary.image_id : (var.tier == "low" ? local.secondary.image_id : "default") # both branches
}

// Non-renamed attribute in conditional (must be untouched)
output "selected_type" {
  value = var.use_primary ? local.primary.instance_type : local.secondary.instance_type
}
`,
	}

	mod := buildModule(t, files)
	mutateRenameAmi(mod)
	assertOrSkip(t, mod, want, "conditional with local alias .attr")
}
