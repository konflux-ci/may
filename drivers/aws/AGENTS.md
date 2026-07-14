# MAY AWS Driver

MAY AWS Driver manages AWS Instances for MAY.

The driver intended to perform all kind of operations with EC2 instances in Amazon cloud. It should support two modes or operations - static (long-running instances) and dynamic (one-time instances). This includes, but not limited to - on demand EC2 instances creation, accessibility verification, and disposal.

## Commands

| Action | Command |
|--------|---------|
| test | `make test` |
| e2e | `make test-e2e` (isolated Kind) |
| run local | `make run` |
| CRD/types changed | `make manifests generate` |
| lint | `make lint` / `make lint-fix` |

## Project Layout

- `config/` — manifests
- `internal/controller/` — controllers
- `internal/config/` — internal configuration structs and parsers
- `internal/client/` — EC2 client constructors (OpenShift SA web-identity auth)

## Key Conventions

- Do not edit `**/zz_generated.*`, `config/crd/bases/*`, `config/rbac/role.yaml`, `PROJECT` — regenerate with `make manifests` / `make generate`.
- Keep `// +kubebuilder:scaffold:*` markers.
- Do not move files.

## Gotchas

- `Host` type is defined in `../../may`.

## AWS authentication (standalone OpenShift)

The driver does not store or read AWS credentials from CRs or Secrets. It calls
the EC2 API using credentials resolved by the AWS SDK from the controller pod
environment.

### How it works

1. The controller runs as the `controller-manager` ServiceAccount.
2. A projected ServiceAccount token (audience `sts.amazonaws.com`) is mounted
   into the pod.
3. `AWS_ROLE_ARN` and `AWS_WEB_IDENTITY_TOKEN_FILE` point the SDK at that token
   and the target IAM role.
4. AWS STS validates the token against an IAM OIDC provider and returns
   short-lived credentials for EC2 API calls.

This is web-identity federation (the same STS API EKS IRSA uses), but OpenShift
is not EKS: nothing injects credentials automatically. The platform team wires
the OIDC trust in AWS and the token projection in the Deployment.

### AWS setup

1. Register the OpenShift service-account OIDC issuer as an IAM OIDC provider.
2. Create an IAM role with `AssumeRoleWithWebIdentity` trust, conditioned on:
   `system:serviceaccount:<controller-namespace>:controller-manager`
3. Attach a least-privilege policy for the EC2 actions the driver needs.

### OpenShift setup

1. Deploy with the `controller-manager` ServiceAccount (see `config/rbac/service_account.yaml`).
2. Enable `config/manager/aws_web_identity_patch.yaml` in the manager kustomization.
3. Set `AWS_ROLE_ARN` in that patch to the IAM role ARN for the environment.

The patch also sets `AWS_EC2_METADATA_DISABLED=true` so the SDK cannot fall
back to the node instance metadata service if web-identity env vars are wrong.
When both web-identity env vars are set, client creation verifies the token file
exists before calling the EC2 API.

Local development can use exported `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`
or an `AWS_PROFILE` instead of web identity.

## Local development

No driver code or manifest changes are required. `make run` uses the same
`LoadDefaultConfig` path; only how credentials are supplied differs.

### What you do not need locally

- `config/manager/aws_web_identity_patch.yaml` (projected SA token, `AWS_ROLE_ARN`)
- OpenShift OIDC / IAM web-identity trust setup
- Any AWS credential Secrets or host annotations

### What you do need

1. **AWS credentials** in your shell or shared config (pick one):
   - `aws configure` then `export AWS_PROFILE=your-profile`, or
   - `export AWS_ACCESS_KEY_ID=...` and `export AWS_SECRET_ACCESS_KEY=...`
     (and `AWS_SESSION_TOKEN` if using temporary credentials)
2. **A reachable cluster** via `KUBECONFIG` (Kind, OpenShift, etc.).
3. **Host CRs with a region annotation** — region still comes from the CR, not
   from env: `aws.may.konflux-ci.dev/region: us-east-1`

### Run

```sh
make run
```

### Gotchas

- Unset `AWS_WEB_IDENTITY_TOKEN_FILE` and `AWS_ROLE_ARN` in your shell if you
  copied them from a cluster pod; otherwise the SDK may try web identity before
  or instead of your local keys (depending on `AWS_PROFILE`).
- `AWS_PROFILE` takes precedence over static env vars when set.
- Use an IAM user or role with EC2 permissions scoped for dev; same API surface
  as production, no special "dev mode" in the driver.
