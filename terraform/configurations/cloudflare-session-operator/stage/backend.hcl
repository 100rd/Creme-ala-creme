bucket         = "creme-terraform-state"
key            = "cloudflare-session-operator/stage/terraform.tfstate"
region         = "eu-central-1"
encrypt        = true
dynamodb_table = "terraform-locks"
