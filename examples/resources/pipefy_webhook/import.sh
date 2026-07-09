# Import an existing Webhook using the format pipe_id/webhook_id
terraform import pipefy_webhook.example "<PIPE_ID>/<WEBHOOK_ID>"

# Note: headers is not read back from the API (it is sensitive), so after import
# the first plan shows an in-place update that re-sends it from your config.
# filters is refreshed from the API, so it imports without a follow-up change.
