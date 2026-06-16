// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func SLADuration() validator.Object { return slaDurationValidator{} }

type slaDurationValidator struct{}

func (v slaDurationValidator) Description(_ context.Context) string {
	return "time must fit its unit: minutes 1-59, hours 1-23, days >= 1"
}

func (v slaDurationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v slaDurationValidator) ValidateObject(_ context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	attrs := req.ConfigValue.Attributes()
	timeVal, _ := attrs["time"].(types.Int64)
	unitVal, _ := attrs["unit"].(types.String)
	if timeVal.IsNull() || timeVal.IsUnknown() || unitVal.IsNull() || unitVal.IsUnknown() {
		return
	}
	t := timeVal.ValueInt64()
	var maxTime int64
	switch unitVal.ValueString() {
	case "minutes":
		maxTime = 59
	case "hours":
		maxTime = 23
	case "days":
		maxTime = 0
	default:
		return
	}
	if t < 1 || (maxTime > 0 && t > maxTime) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid SLA duration",
			fmt.Sprintf("time=%d is out of range for unit %q (minutes 1-59, hours 1-23, days >= 1)", t, unitVal.ValueString()),
		)
	}
}
