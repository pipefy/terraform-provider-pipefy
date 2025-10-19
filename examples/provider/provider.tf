# Using a static token

provider "pipefy" {
  token = "my_token"
}

# Using a service account

provider "pipefy" {
  client_id     = "your_client_id"
  client_secret = "your_client_secret"
}

# Using a service account on Single Tenants

provider "pipefy" {
  client_id     = "your_client_id"
  client_secret = "your_client_secret"
  token_url     = "https://<your_single_tenant_domain>/oauth/token"
  endpoint      = "https://<your_single_tenant_domain>/graphql"
}
