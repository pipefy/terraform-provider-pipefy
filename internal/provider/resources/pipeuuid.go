// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/pipefy/terraform-provider-pipefy/internal/provider/client"
)

const getPipeUUIDQuery = "query GetPipeUuid_tf($id:ID!){ pipe(id:$id){ uuid } }"

func resolvePipeUUID(ctx context.Context, api *client.ApiClient, pipeID string) (string, error) {
	var output struct {
		Pipe *struct {
			UUID string `json:"uuid"`
		} `json:"pipe"`
	}
	if err := api.DoGraphQL(ctx, getPipeUUIDQuery, map[string]any{"id": pipeID}, &output); err != nil {
		return "", fmt.Errorf("resolve pipe %q UUID: %w", pipeID, err)
	}
	if output.Pipe == nil || output.Pipe.UUID == "" {
		return "", fmt.Errorf("resolve pipe %q UUID: expected a non-empty pipe.uuid", pipeID)
	}
	return output.Pipe.UUID, nil
}
