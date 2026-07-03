# The first component must be the parent pipe id: the relation is read from the
# parent's childrenRelations, so a child pipe id will not resolve.
terraform import pipefy_pipe_relation.orders_to_fulfillment "<PARENT_PIPE_ID>/<RELATION_ID>"
