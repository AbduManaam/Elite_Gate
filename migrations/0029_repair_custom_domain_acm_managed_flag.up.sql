UPDATE custom_domains
SET
    certificate_managed_by_elitegate = TRUE,
    provisioning_status = 'attaching_certificate',
    provisioning_error = NULL,
    next_retry_at = NOW(),
    updated_at = NOW()
WHERE certificate_arn IS NOT NULL
  AND certificate_managed_by_elitegate = FALSE
  AND provisioning_status = 'failed';