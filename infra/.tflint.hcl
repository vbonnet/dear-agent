# tflint configuration for the infra/ OpenTofu root and its modules.
#
# Only the bundled terraform ruleset is enabled. The cloud provider rulesets
# (aws, azurerm, google) are irrelevant here: this root manages GitHub
# repositories and rulesets, nothing else.
config {
  call_module_type = "all"
}

plugin "terraform" {
  enabled = true
  preset  = "recommended"
}

# The GitHub provider is pinned in providers.tf, and required_version lives
# there too, so the deprecated-interpolation and unpinned-source rules below
# are the ones that actually earn their keep on this root.
rule "terraform_required_version" {
  enabled = true
}

rule "terraform_required_providers" {
  enabled = true
}

rule "terraform_unused_declarations" {
  enabled = true
}

rule "terraform_documented_variables" {
  enabled = true
}

rule "terraform_documented_outputs" {
  enabled = true
}

rule "terraform_naming_convention" {
  enabled = true
  format  = "snake_case"
}
