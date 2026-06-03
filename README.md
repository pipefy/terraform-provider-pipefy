<div align="center">
  <img
    src="docs/images/pipefy-developers-banner.png"
    alt="Pipefy Developers: Where developers orchestrate intelligence"
    width="100%"
  />
</div>

# Terraform Provider for Pipefy

[![Release](https://img.shields.io/github/v/release/pipefy/terraform-provider-pipefy.svg)](https://github.com/pipefy/terraform-provider-pipefy/releases)
[![Terraform Registry](https://img.shields.io/badge/terraform-registry-623CE4?logo=terraform)](https://registry.terraform.io/providers/pipefy/pipefy/latest)
[![License](https://img.shields.io/badge/license-MPL--2.0-blue.svg)](LICENSE)
[![Tests](https://github.com/pipefy/terraform-provider-pipefy/actions/workflows/test.yml/badge.svg)](https://github.com/pipefy/terraform-provider-pipefy/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/pipefy/terraform-provider-pipefy)](https://goreportcard.com/report/github.com/pipefy/terraform-provider-pipefy)

Manage [Pipefy](https://www.pipefy.com) as code. This provider lets you create and manage pipes, phases, fields, and automations through the Pipefy GraphQL API. It is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework).

## Documentation

- Provider reference, resources, and data sources: [Terraform Registry](https://registry.terraform.io/providers/pipefy/pipefy/latest/docs)
- Runnable configurations: [`examples/`](./examples)

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24 (only to build the provider from source)

## Using the provider

Declare the provider and pin a version:

```terraform
terraform {
  required_providers {
    pipefy = {
      source  = "pipefy/pipefy"
      version = "0.0.7-pre"
    }
  }
}
```

The provider is in a pre-1.0 prerelease line. Pin an exact version, since Terraform does not select prerelease versions through range constraints such as `~>`. Check the [releases](https://github.com/pipefy/terraform-provider-pipefy/releases) for the latest.

### Authentication

Configure either a static API token or a service account (OAuth client credentials). One of the two is required.

```terraform
# Static token
provider "pipefy" {
  token = "your_api_token"
}

# Service account
provider "pipefy" {
  client_id     = "your_client_id"
  client_secret = "your_client_secret"
}
```

Every attribute can also be supplied through environment variables, so credentials stay out of your configuration:

| Attribute       | Environment variable   | Default                              |
| --------------- | ---------------------- | ------------------------------------ |
| `token`         | `PIPEFY_TOKEN`         | -                                    |
| `client_id`     | `PIPEFY_CLIENT_ID`     | -                                    |
| `client_secret` | `PIPEFY_CLIENT_SECRET` | -                                    |
| `token_url`     | `PIPEFY_TOKEN_URL`     | `https://app.pipefy.com/oauth/token` |
| `endpoint`      | -                      | `https://api.pipefy.com/graphql`     |

For a single-tenant deployment, point `endpoint` and `token_url` at your domain:

```terraform
provider "pipefy" {
  client_id     = "your_client_id"
  client_secret = "your_client_secret"
  token_url     = "https://<your_single_tenant_domain>/oauth/token"
  endpoint      = "https://<your_single_tenant_domain>/graphql"
}
```

### Example

```terraform
resource "pipefy_pipe" "example" {
  name            = "Example Pipe"
  organization_id = "<ORG_ID>"
}

resource "pipefy_phase" "backlog" {
  pipe_id = pipefy_pipe.example.id
  name    = "Backlog"
}

resource "pipefy_field" "title" {
  phase_id = pipefy_phase.backlog.id
  type     = "short_text"
  label    = "Title"
  required = true
}

resource "pipefy_field" "priority" {
  phase_id = pipefy_phase.backlog.id
  type     = "select"
  label    = "Priority"
  options  = ["Low", "Medium", "High"]
}
```

The provider ships the `pipefy_pipe`, `pipefy_phase`, `pipefy_field`, and `pipefy_automation` resources (all importable) and the `pipefy_pipe` and `pipefy_phase` data sources. See [`examples/`](./examples) for automation and import examples.

## Developing the provider

Build and install the provider into `$GOPATH/bin`:

```shell
make install
```

Regenerate the registry documentation after changing any resource or data source schema:

```shell
make generate
```

Format and lint before opening a pull request:

```shell
make fmt
make lint
```

See [AGENTS.md](./AGENTS.md) for the full development guide, including project layout and how to add a resource.

## Testing

```shell
make test      # unit tests
make testacc   # acceptance tests
```

Acceptance tests run against the live Pipefy API and create real resources, which may incur cost. They require `PIPEFY_TOKEN` (or `PIPEFY_CLIENT_ID` and `PIPEFY_CLIENT_SECRET`) to be set.

## Contributing

Issues and pull requests are welcome. Please review the [Code of Conduct](./.github/CODE_OF_CONDUCT.md) and run `make fmt`, `make lint`, and `make test` before submitting.

## Support

This provider is open source and maintained by Pipefy on a best-effort basis. For bugs and feature requests, open a [GitHub issue](https://github.com/pipefy/terraform-provider-pipefy/issues). It is not covered by your Pipefy commercial support agreement or SLA.

## License

Distributed under the Mozilla Public License 2.0. See [LICENSE](./LICENSE).
