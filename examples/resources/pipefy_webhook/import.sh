# Import an existing Webhook using the format pipe_id/webhook_id
terraform import pipefy_webhook.example "<PIPE_ID>/<WEBHOOK_ID>"

# Note: headers and filters are not read back from the API, so after import the
# first plan shows an in-place update that re-sends the values from your config.
