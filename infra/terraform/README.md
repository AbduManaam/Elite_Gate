# EliteGate Terraform — Step 3

This starter package creates the safe Terraform foundation before AWS application resources are defined.

## Order

1. Review and edit `bootstrap/terraform.tfvars.example`.
2. Copy it to `bootstrap/terraform.tfvars`.
3. Run the bootstrap stack to create the S3 state bucket.
4. Copy `environments/production/backend.hcl.example` to `backend.hcl`.
5. Replace the bucket name and region in `backend.hcl`.
6. Initialize the production stack with the remote backend.
7. Run formatting, validation, and planning.
8. Do not run `terraform apply` for the production stack until the full infrastructure is reviewed.

## Bootstrap commands

```powershell
cd infra\terraform\bootstrap
Copy-Item terraform.tfvars.example terraform.tfvars
terraform init
terraform fmt -recursive
terraform validate
terraform plan -out bootstrap.tfplan
terraform apply bootstrap.tfplan
```

The bootstrap apply creates only the Terraform state bucket. It does not create EliteGate EC2, RDS, Redis, ALB, or other application infrastructure.

## Production foundation commands

```powershell
cd ..\environments\production
Copy-Item backend.hcl.example backend.hcl
terraform init -backend-config=backend.hcl
terraform fmt -recursive
terraform validate
terraform plan
```

At this point, the production stack contains provider configuration and input definitions only. Application resources will be added in the next Terraform batches.
