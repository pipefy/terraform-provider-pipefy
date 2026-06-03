# AGENTS.md

Development guide for `terraform-provider-pipefy`, aimed at both humans and AI coding agents. `CLAUDE.md` is a symlink to this file.

## Project overview

A Terraform provider that manages [Pipefy](https://www.pipefy.com) resources through the Pipefy GraphQL API (`https://api.pipefy.com/graphql`). It is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) and speaks Terraform plugin protocol 6.0.

Module path: `github.com/pipefy/terraform-provider-pipefy`. Registry source: `pipefy/pipefy`.

It exposes four resources (`pipefy_pipe`, `pipefy_phase`, `pipefy_field`, `pipefy_automation`, all importable) and two data sources (`pipefy_pipe`, `pipefy_phase`).

## Setup and requirements

- Go >= 1.24 (pinned to 1.24.0 in `.tool-versions` and `go.mod`)
- Terraform >= 1.0
- `golangci-lint` for linting

```shell
git clone https://github.com/pipefy/terraform-provider-pipefy
cd terraform-provider-pipefy
go mod download
make build
```

## Essential commands

All tasks run through the `GNUmakefile`. Use these targets rather than ad hoc `go` invocations.

| Command         | What it does                                                       |
| --------------- | ------------------------------------------------------------------ |
| `make build`    | Compile all packages (`go build -v ./...`)                         |
| `make install`  | Build and install the provider into `$GOPATH/bin`                  |
| `make generate` | Regenerate registry docs via tfplugindocs (`cd tools; go generate ./...`) |
| `make fmt`      | Format Go code (`gofmt -s -w -e .`)                                |
| `make lint`     | Run `golangci-lint run`                                            |
| `make test`     | Run unit tests                                                     |
| `make testacc`  | Run acceptance tests (`TF_ACC=1`) against the live API             |

The `docs/` directory is generated. Run `make generate` after any schema change; never hand-edit files under `docs/`.

## Project layout

```
internal/provider/
  provider.go              # Provider config, auth, resource/data-source registration
  resources/               # pipefy_pipe, pipefy_phase, pipefy_field, pipefy_automation
  datasources/             # pipefy_pipe, pipefy_phase
  client/api_client.go     # Pipefy GraphQL HTTP client
  locks/                   # Mutex helpers for serializing API mutations
examples/                  # Example .tf per resource and data source (feeds the docs)
docs/                      # Generated reference docs (do not edit by hand)
tools/                     # tfplugindocs tooling for `make generate`
main.go                    # Provider entrypoint
```

## Conventions

- Terraform attribute names are snake_case; resource type names are prefixed `pipefy_`.
- Register every resource and data source in `provider.go` via the `Resources()` and `DataSources()` constructor lists.
- Each resource has a matching example at `examples/resources/<type>/resource.tf` and an `import.sh`; data sources have `examples/data-sources/<type>/data-source.tf`. These examples are embedded into the generated docs.
- Auth lives entirely in `provider.go`'s `Configure`. The provider accepts a static `token` (env `PIPEFY_TOKEN`) or a service account `client_id` + `client_secret` (env `PIPEFY_CLIENT_ID` / `PIPEFY_CLIENT_SECRET`); exactly one mode must be configured.

## How to add a resource

1. Implement schema, `Create`, `Read`, `Update`, `Delete`, and `ImportState` in `internal/provider/resources/resource_<name>.go`.
2. Add its constructor to the `Resources()` list in `internal/provider/provider.go`.
3. Add `examples/resources/pipefy_<name>/resource.tf` and `import.sh`.
4. Add an acceptance test `internal/provider/resource_<name>_test.go`.
5. Run `make generate` to refresh `docs/`, then `make testacc` to verify.

## Testing

- Unit tests: `make test`.
- Acceptance tests are gated on `TF_ACC=1` (use `make testacc`). They create real Pipefy resources and may incur cost.
- Export credentials before running acceptance tests: `PIPEFY_TOKEN`, or `PIPEFY_CLIENT_ID` and `PIPEFY_CLIENT_SECRET`.

## Gotchas

- One of the two auth modes is required; the provider errors at configure time if neither is set.
- Single-tenant deployments must set both `endpoint` and `token_url` to the tenant domain.
- The provider struct is still named `ScaffoldingProvider` internally (leftover from the HashiCorp template); the user-facing type name is `pipefy`.
- `examples/` still contains template directories named `scaffolding_example`; they are not part of the provider and can be ignored.

## Never

- Commit API tokens, client secrets, or other credentials.
- Hand-edit generated files under `docs/`; change the schema and run `make generate` instead.
