# CI/CD setup — payment-service

This repository ships two GitHub Actions workflows:

| File | Trigger | Purpose |
|------|---------|---------|
| `workflows/ci.yml` | Pull request + push to `main` | lint, vet, gofmt, golangci-lint, `go mod tidy` drift, unit tests with race + coverage, govulncheck, Docker build (no push) |
| `workflows/cd.yml` | Push to `main`, manual dispatch | Build & push image to Artifact Registry, deploy to Cloud Run `dev → staging → prod` with canary 10→50→100% and manual approval gates |

## Required repository configuration

### 1. Repository variables (`Settings → Secrets and variables → Actions → Variables`)

| Variable | Required | Default | Description |
|----------|:--------:|---------|-------------|
| `GCP_PROJECT_ID` | ✅ | — | GCP project hosting Artifact Registry + Cloud Run |
| `GCP_REGION` | optional | `asia-southeast2` | Cloud Run + Artifact Registry region |
| `ARTIFACT_REGISTRY_REPO` | optional | `parkirpintar` | Artifact Registry repo name |
| `WIF_PROVIDER` | ✅ | — | Full WIF provider id, e.g. `projects/123/locations/global/workloadIdentityPools/gha/providers/github` |
| `WIF_SERVICE_ACCOUNT` | ✅ | — | Service account email the GitHub job impersonates |
| `SERVICE_URL_DEV` | optional | — | Public URL of the dev Cloud Run revision (e.g. `https://payment-service-dev-abc.a.run.app`). When set, the smoke job probes `/healthz`. |

> No static service-account keys. Authentication uses Workload Identity Federation
> (WIF) so jobs assume the GCP service account via OIDC.

### 2. Environments (`Settings → Environments`)

Create three environments. Set protection rules:

| Environment | Required reviewers | Wait timer | Branch policy |
|-------------|--------------------|-----------:|----------------|
| `dev`       | 0 | 0 | `main` only |
| `staging`   | 1 | 0 | `main` only |
| `prod`      | 2 | 0 | `main` only |

Manual-approval gates are enforced by these environment settings — the workflow
itself just declares the environment name; GitHub blocks until reviewers approve.

### 3. GCP-side prerequisites (one-time)

```bash
# Variables
PROJECT_ID=<your-project>
REGION=asia-southeast2
POOL=gha
PROVIDER=github
REPO_OWNER=pintarparkir          # GitHub org / user
REPO_NAME=payment-service        # Repository name
SA_NAME=payment-deployer

# Artifact Registry
gcloud artifacts repositories create parkirpintar \
  --repository-format=docker --location=$REGION --project=$PROJECT_ID

# Workload Identity Pool + Provider
gcloud iam workload-identity-pools create $POOL --location=global --project=$PROJECT_ID
gcloud iam workload-identity-pools providers create-oidc $PROVIDER \
  --location=global --workload-identity-pool=$POOL --project=$PROJECT_ID \
  --issuer-uri="https://token.actions.githubusercontent.com" \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository,attribute.ref=assertion.ref" \
  --attribute-condition="assertion.repository=='$REPO_OWNER/$REPO_NAME'"

# Service account + binding
gcloud iam service-accounts create $SA_NAME --project=$PROJECT_ID
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:$SA_NAME@$PROJECT_ID.iam.gserviceaccount.com" \
  --role=roles/run.admin
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:$SA_NAME@$PROJECT_ID.iam.gserviceaccount.com" \
  --role=roles/artifactregistry.writer
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:$SA_NAME@$PROJECT_ID.iam.gserviceaccount.com" \
  --role=roles/iam.serviceAccountUser

# Bind the GHA-OIDC subject to that service account
PROJECT_NUMBER=$(gcloud projects describe $PROJECT_ID --format='value(projectNumber)')
gcloud iam service-accounts add-iam-policy-binding \
  $SA_NAME@$PROJECT_ID.iam.gserviceaccount.com \
  --role=roles/iam.workloadIdentityUser \
  --member="principalSet://iam.googleapis.com/projects/$PROJECT_NUMBER/locations/global/workloadIdentityPools/$POOL/attribute.repository/$REPO_OWNER/$REPO_NAME"
```

Then copy the WIF provider resource name into the `WIF_PROVIDER` repo variable:

```
projects/$PROJECT_NUMBER/locations/global/workloadIdentityPools/$POOL/providers/$PROVIDER
```

And the service account email into `WIF_SERVICE_ACCOUNT`:

```
$SA_NAME@$PROJECT_ID.iam.gserviceaccount.com
```

### 4. Cloud Run services (one-time, per environment)

Cloud Run services are created on first `gcloud run deploy`. The CD workflow uses
the service names `<repo>-dev`, `<repo>-staging`, `<repo>-prod`. Bootstrap with:

```bash
gcloud run deploy payment-service-dev \
  --image=gcr.io/cloudrun/hello \
  --region=$REGION --project=$PROJECT_ID \
  --no-allow-unauthenticated --no-traffic
# Repeat for -staging, -prod.
```

(The hello image is just a placeholder so the service object exists; the workflow
will replace it on first real deploy.)

## Quality gates (current state)

| Gate | Threshold | Blocking? |
|------|-----------|----------|
| Lint (golangci-lint) | 0 errors | ✅ |
| gofmt / `go vet` | clean | ✅ |
| `go mod tidy` drift | empty diff | ✅ |
| Unit tests | 100% pass | ✅ |
| Unit-test coverage (usecase) | ≥ 80% | ⚠️ warn-only (flips to blocking after T-008) |
| Unit-test coverage (repo) | ≥ 60% | ⚠️ warn-only |
| govulncheck | 0 HIGH/CRITICAL | ✅ |
| Container vuln scan (Artifact Registry / Container Analysis) | 0 CRITICAL | informational |
| Manual approval (staging → prod) | 2 reviewers | ✅ (via Environment protection) |

## Rollback

Canary stages publish a tagged revision (`prod-<sha>`, etc.). To roll back:

```bash
# Find the previous good tag (e.g. prod-<prevsha>)
gcloud run services describe payment-service-prod --region=$REGION \
  --format='value(status.traffic)'

# Point all traffic back at it
gcloud run services update-traffic payment-service-prod \
  --region=$REGION \
  --to-tags=prod-<prevsha>=100
```
